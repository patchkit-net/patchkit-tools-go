package diff

import (
	"fmt"
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
		if !native.TurboPatchAvailable() {
			return fmt.Errorf("turbopatch algorithm requested but not available")
		}

		// Read block size from old signature
		blockLen, err := native.ReadSignatureBlockLen(sigPath)
		if err != nil {
			return fmt.Errorf("failed to read signature block length: %w", err)
		}

		// Generate new signature for the new content file
		newSigPath := filepath.Join(db.tempDir, filepath.Base(contentPath)+".newsig")
		if err := db.rsync.Signature(contentPath, newSigPath, blockLen); err != nil {
			return fmt.Errorf("failed to generate new signature: %w", err)
		}
		defer os.Remove(newSigPath)

		// Compute turbopatch delta: old_sig + new_sig + new_content -> delta
		return db.turbopatch.Delta2(sigPath, newSigPath, contentPath, deltaOutputPath, turbopatchBlockSize)

	default:
		return fmt.Errorf("unknown delta algorithm: %s", db.algorithm)
	}
}
