package native

import (
	"encoding/binary"
	"fmt"
	"os"
)

// readSigBlockLen reads the block length from a librsync signature file header.
// The signature format is: 4 bytes magic + 4 bytes block_len (big-endian) + 4 bytes strong_len.
func readSigBlockLen(sigPath string) (int, error) {
	f, err := os.Open(sigPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	// Skip magic (4 bytes)
	if _, err := f.Seek(4, 0); err != nil {
		return 0, fmt.Errorf("failed to seek past magic: %w", err)
	}

	// Read block_len (4 bytes, big-endian per librsync spec)
	var blockLen uint32
	if err := binary.Read(f, binary.BigEndian, &blockLen); err != nil {
		return 0, fmt.Errorf("failed to read block_len: %w", err)
	}

	if blockLen == 0 {
		return 2048, nil // default block size
	}

	return int(blockLen), nil
}
