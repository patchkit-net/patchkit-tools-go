package diff

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"golang.org/x/sync/errgroup"

	"github.com/patchkit-net/patchkit-tools-go/internal/hash"
	"github.com/patchkit-net/patchkit-tools-go/internal/native"
)

const xxhashSeed uint32 = 42

// ProgressFn is called when a file has been processed.
type ProgressFn func(current, total int, fileName string)

// Config configures the diff pipeline.
type Config struct {
	// ContentDir is the directory containing the new version's files.
	ContentDir string

	// SignaturesDir is the directory containing the previous version's signatures.
	SignaturesDir string

	// TempDir is a directory for temporary files during delta computation
	// (used by turbopatch for intermediate signatures).
	TempDir string

	// Algorithm specifies the delta algorithm to use.
	Algorithm native.DeltaAlgorithm

	// Workers is the number of parallel delta workers. 0 means use GOMAXPROCS.
	Workers int

	// PreviousHashes maps relative file paths to their xxh32 hex hashes from
	// the previous diff. Used to detect unchanged files.
	PreviousHashes map[string]string

	// ProgressFn is called after each file is processed.
	ProgressFn ProgressFn
}

// DeltaEntry holds the data for a single entry in the diff archive.
// Exactly one of FilePath or Data is set.
type DeltaEntry struct {
	// FilePath is set for added files — points to the original content file on disk.
	FilePath string

	// Data is set for modified files — contains the delta bytes in memory.
	Data []byte

	// Mode is the file permission mode of the source file.
	Mode os.FileMode
}

// Result contains the diff pipeline output.
type Result struct {
	// Summary describes the file changes.
	Summary *Summary

	// DeltaFiles maps relative file paths to their DeltaEntry.
	// For added files, FilePath points to the original content.
	// For modified files, Data contains the in-memory delta.
	// Unchanged files have no entry.
	DeltaFiles map[string]DeltaEntry
}

// Run executes the diff pipeline.
// Deltas for modified files are computed in parallel and stored in memory,
// avoiding temporary files on disk (which can be corrupted by AV software).
func Run(ctx context.Context, cfg *Config) (*Result, error) {
	// List content files
	contentFiles, err := listRelativeFiles(cfg.ContentDir)
	if err != nil {
		return nil, fmt.Errorf("listing content files: %w", err)
	}

	// List signature files
	sigFiles, err := listRelativeFiles(cfg.SignaturesDir)
	if err != nil {
		return nil, fmt.Errorf("listing signature files: %w", err)
	}

	// Classify files
	contentSet := toSet(contentFiles)
	sigSet := toSet(sigFiles)

	var added, modified, removed []string
	for _, f := range contentFiles {
		if sigSet[f] {
			modified = append(modified, f)
		} else {
			added = append(added, f)
		}
	}
	for _, f := range sigFiles {
		if !contentSet[f] {
			removed = append(removed, f)
		}
	}

	sort.Strings(added)
	sort.Strings(modified)
	sort.Strings(removed)

	deltaFiles := make(map[string]DeltaEntry)
	var unchangedFiles []string
	totalFiles := len(added) + len(modified)
	processed := 0

	// Process added files - reference the original content on disk
	for _, f := range added {
		contentPath := filepath.Join(cfg.ContentDir, filepath.FromSlash(f))
		info, err := os.Stat(contentPath)
		if err != nil {
			return nil, fmt.Errorf("stat added file %s: %w", f, err)
		}
		deltaFiles[f] = DeltaEntry{
			FilePath: contentPath,
			Mode:     info.Mode(),
		}
		processed++
		if cfg.ProgressFn != nil {
			cfg.ProgressFn(processed, totalFiles, f)
		}
	}

	// Process modified files in parallel — deltas go to memory
	type deltaResult struct {
		file      string
		data      []byte
		mode      os.FileMode
		unchanged bool
	}

	results := make(chan deltaResult, len(modified))
	deltaBuilder := NewDeltaBuilder(cfg.Algorithm, cfg.TempDir)

	workers := cfg.Workers
	if workers <= 0 {
		workers = 4
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(workers)

	for _, f := range modified {
		f := f
		g.Go(func() error {
			if gctx.Err() != nil {
				return gctx.Err()
			}

			contentPath := filepath.Join(cfg.ContentDir, filepath.FromSlash(f))
			sigPath := filepath.Join(cfg.SignaturesDir, filepath.FromSlash(f))

			// Check if file is unchanged via xxhash
			if cfg.PreviousHashes != nil {
				if prevHash, ok := cfg.PreviousHashes[f]; ok {
					currentHash, err := hash.XXH32File(contentPath, xxhashSeed)
					if err == nil {
						currentHex := fmt.Sprintf("%x", currentHash)
						if currentHex == prevHash {
							results <- deltaResult{file: f, unchanged: true}
							return nil
						}
					}
				}
			}

			// Get source file mode for metadata
			info, err := os.Stat(contentPath)
			if err != nil {
				return fmt.Errorf("stat %s: %w", f, err)
			}

			// Compute delta to in-memory buffer
			var buf bytes.Buffer
			if err := deltaBuilder.BuildDeltaToWriter(sigPath, contentPath, &buf); err != nil {
				return fmt.Errorf("building delta for %s: %w", f, err)
			}

			results <- deltaResult{file: f, data: buf.Bytes(), mode: info.Mode()}
			return nil
		})
	}

	// Collect results in a goroutine
	go func() {
		g.Wait()
		close(results)
	}()

	for dr := range results {
		if dr.unchanged {
			unchangedFiles = append(unchangedFiles, dr.file)
		} else {
			deltaFiles[dr.file] = DeltaEntry{
				Data: dr.data,
				Mode: dr.mode,
			}
		}
		processed++
		if cfg.ProgressFn != nil {
			cfg.ProgressFn(processed, totalFiles, dr.file)
		}
	}

	// Check for errors from the errgroup
	if err := g.Wait(); err != nil {
		return nil, err
	}

	sort.Strings(unchangedFiles)

	return &Result{
		Summary: &Summary{
			AddedFiles:     added,
			ModifiedFiles:  modified,
			RemovedFiles:   removed,
			UnchangedFiles: unchangedFiles,
		},
		DeltaFiles: deltaFiles,
	}, nil
}

func listRelativeFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	return files, err
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}
