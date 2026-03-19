//go:build !cgo_librsync

package native

import (
	"io"
	"os"

	"github.com/balena-os/librsync-go"
)

// PureGoRsync implements Rsync using a pure Go librsync library.
type PureGoRsync struct{}

// NewRsync creates a new pure Go Rsync implementation.
func NewRsync() Rsync {
	return &PureGoRsync{}
}

func (r *PureGoRsync) Signature(filePath, sigOutputPath string, blockLen int) error {
	inFile, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer inFile.Close()

	outFile, err := os.Create(sigOutputPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	if blockLen <= 0 {
		blockLen = 2048
	}

	_, err = librsync.Signature(inFile, outFile, uint32(blockLen), 0, librsync.BLAKE2_SIG_MAGIC)
	return err
}

func (r *PureGoRsync) Delta(sigPath, newFilePath, deltaOutputPath string) error {
	outFile, err := os.Create(deltaOutputPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	return r.DeltaToWriter(sigPath, newFilePath, outFile)
}

func (r *PureGoRsync) DeltaToWriter(sigPath, newFilePath string, out io.Writer) error {
	sig, err := librsync.ReadSignatureFile(sigPath)
	if err != nil {
		return err
	}

	newFile, err := os.Open(newFilePath)
	if err != nil {
		return err
	}
	defer newFile.Close()

	return librsync.Delta(sig, newFile, out)
}
