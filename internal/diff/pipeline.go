package diff

import (
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

	// OutputDir is the directory where delta files will be written.
	OutputDir string

	// TempDir is a directory for temporary files during delta computation.
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

// Result contains the diff pipeline output.
type Result struct {
	// Summary describes the file changes.
	Summary *Summary

	// DeltaFiles maps relative file paths to their delta file paths in OutputDir.
	// For added files, the value is the original content file path.
	// For modified files with no changes (unchanged), no entry exists.
	DeltaFiles map[string]string
}

// Run executes the diff pipeline.
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

	// Create output directory
	if err := os.MkdirAll(cfg.OutputDir, 0755); err != nil {
		return nil, err
	}

	deltaFiles := make(map[string]string)
	var unchangedFiles []string
	totalFiles := len(added) + len(modified)
	processed := 0

	// Process added files - just reference the original content
	for _, f := range added {
		deltaFiles[f] = filepath.Join(cfg.ContentDir, filepath.FromSlash(f))
		processed++
		if cfg.ProgressFn != nil {
			cfg.ProgressFn(processed, totalFiles, f)
		}
	}

	// Process modified files in parallel
	type deltaResult struct {
		file      string
		deltaPath string
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

			// Compute delta
			deltaPath := filepath.Join(cfg.OutputDir, filepath.FromSlash(f)+".delta")

			// Ensure parent directory exists
			if err := os.MkdirAll(filepath.Dir(deltaPath), 0755); err != nil {
				return err
			}

			if err := deltaBuilder.BuildDelta(sigPath, contentPath, deltaPath); err != nil {
				return fmt.Errorf("building delta for %s: %w", f, err)
			}

			results <- deltaResult{file: f, deltaPath: deltaPath}
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
			deltaFiles[dr.file] = dr.deltaPath
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
