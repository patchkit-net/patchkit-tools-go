package diff

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/patchkit-net/patchkit-tools-go/internal/native"
)

const turbopatchBlockSize int64 = 128 * 1024 * 1024 // 128MB

// DeltaBuilder computes file deltas using the configured algorithm.
type DeltaBuilder struct {
	rsync      native.Rsync
	turbopatch native.TurboPatch
	algorithm  native.DeltaAlgorithm
	tempDir    string
}

// NewDeltaBuilder creates a delta builder with the given algorithm.
func NewDeltaBuilder(algorithm native.DeltaAlgorithm, tempDir string) *DeltaBuilder {
	return &DeltaBuilder{
		rsync:      native.NewRsync(),
		turbopatch: native.NewTurboPatch(),
		algorithm:  algorithm,
		tempDir:    tempDir,
	}
}

// BuildDelta generates a delta file for a modified file.
// sigPath: path to the old version's signature file
// contentPath: path to the new version's content file
// deltaOutputPath: where to write the delta
func (db *DeltaBuilder) BuildDelta(sigPath, contentPath, deltaOutputPath string) error {
	switch db.algorithm {
	case native.AlgorithmLibrsync:
		return db.rsync.Delta(sigPath, contentPath, deltaOutputPath)

	case native.AlgorithmTurbopatch:
		return db.buildTurbopatchDelta(sigPath, contentPath, deltaOutputPath)

	default:
		return fmt.Errorf("unknown delta algorithm: %s", db.algorithm)
	}
}

// BuildDeltaToWriter generates a delta and writes it to the provided writer.
// For librsync (pure Go), this streams directly without temp files.
// For turbopatch (CGo only), this falls back to a temp file.
func (db *DeltaBuilder) BuildDeltaToWriter(sigPath, contentPath string, out io.Writer) error {
	switch db.algorithm {
	case native.AlgorithmLibrsync:
		return db.rsync.DeltaToWriter(sigPath, contentPath, out)

	case native.AlgorithmTurbopatch:
		// Turbopatch is CGo-only and requires file paths.
		// Generate to a temp file, then copy to writer.
		tmpFile, err := os.CreateTemp(db.tempDir, "tp-delta-*")
		if err != nil {
			return fmt.Errorf("creating temp file for turbopatch delta: %w", err)
		}
		tmpPath := tmpFile.Name()
		tmpFile.Close()
		defer os.Remove(tmpPath)

		if err := db.buildTurbopatchDelta(sigPath, contentPath, tmpPath); err != nil {
			return err
		}

		f, err := os.Open(tmpPath)
		if err != nil {
			return fmt.Errorf("reading turbopatch delta: %w", err)
		}
		defer f.Close()

		_, err = io.Copy(out, f)
		return err

	default:
		return fmt.Errorf("unknown delta algorithm: %s", db.algorithm)
	}
}

func (db *DeltaBuilder) buildTurbopatchDelta(sigPath, contentPath, deltaOutputPath string) error {
	if !native.TurboPatchAvailable() {
		return fmt.Errorf("turbopatch algorithm requested but not available")
	}

	// Read block size from old signature
	blockLen, err := native.ReadSignatureBlockLen(sigPath)
	if err != nil {
		return fmt.Errorf("failed to read signature block length: %w", err)
	}

	// Generate new signature for the new content file.
	// Use CreateTemp for unique naming to avoid collisions when parallel workers
	// process files with the same basename from different directories.
	tmpSigFile, err := os.CreateTemp(db.tempDir, filepath.Base(contentPath)+"-*.newsig")
	if err != nil {
		return fmt.Errorf("creating temp sig file: %w", err)
	}
	newSigPath := tmpSigFile.Name()
	tmpSigFile.Close()
	defer os.Remove(newSigPath)

	if err := db.rsync.Signature(contentPath, newSigPath, blockLen); err != nil {
		return fmt.Errorf("failed to generate new signature: %w", err)
	}

	// Compute turbopatch delta: old_sig + new_sig + new_content -> delta
	return db.turbopatch.Delta2(sigPath, newSigPath, contentPath, deltaOutputPath, turbopatchBlockSize)
}
