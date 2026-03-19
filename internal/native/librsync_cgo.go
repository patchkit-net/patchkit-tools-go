//go:build cgo_librsync

package native

/*
#cgo LDFLAGS: -lrsync
#include <stdlib.h>

// Forward declarations matching librsync's whole.h
typedef int rs_result;
extern rs_result rs_rdiff_sig(char *basis_name, char *sig_name, long block_len);
extern rs_result rs_rdiff_delta(char *sig_name, char *new_name, char *delta_name);
*/
import "C"

import (
	"fmt"
	"io"
	"os"
	"unsafe"
)

// CGoRsync implements Rsync using CGo bindings to librsync.
type CGoRsync struct{}

// NewRsync creates a new CGo Rsync implementation.
func NewRsync() Rsync {
	return &CGoRsync{}
}

func (r *CGoRsync) Signature(filePath, sigOutputPath string, blockLen int) error {
	cBasis := C.CString(filePath)
	defer C.free(unsafe.Pointer(cBasis))

	cSig := C.CString(sigOutputPath)
	defer C.free(unsafe.Pointer(cSig))

	result := C.rs_rdiff_sig(cBasis, cSig, C.long(blockLen))
	if result != 0 {
		return fmt.Errorf("rs_rdiff_sig failed with code %d", result)
	}
	return nil
}

func (r *CGoRsync) Delta(sigPath, newFilePath, deltaOutputPath string) error {
	cSig := C.CString(sigPath)
	defer C.free(unsafe.Pointer(cSig))

	cNew := C.CString(newFilePath)
	defer C.free(unsafe.Pointer(cNew))

	cDelta := C.CString(deltaOutputPath)
	defer C.free(unsafe.Pointer(cDelta))

	result := C.rs_rdiff_delta(cSig, cNew, cDelta)
	if result != 0 {
		return fmt.Errorf("rs_rdiff_delta failed with code %d", result)
	}
	return nil
}

func (r *CGoRsync) DeltaToWriter(sigPath, newFilePath string, out io.Writer) error {
	// CGo bindings require file paths, so use a temp file and copy to writer.
	tmpFile, err := os.CreateTemp("", "pkt-delta-*")
	if err != nil {
		return fmt.Errorf("creating temp file for delta: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	if err := r.Delta(sigPath, newFilePath, tmpPath); err != nil {
		return err
	}

	f, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("reading delta temp file: %w", err)
	}
	defer f.Close()

	_, err = io.Copy(out, f)
	return err
}
