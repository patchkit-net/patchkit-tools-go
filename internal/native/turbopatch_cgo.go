//go:build cgo_turbopatch

package native

/*
#cgo LDFLAGS: -lturbopatch
#include <stdlib.h>
#include <stdint.h>

// Forward declarations matching turbopatch Rust FFI exports
extern int tp_delta2(const char *old_sig_path, const char *new_sig_path,
                     const char *new_file_path, const char *target_path,
                     uint64_t block_size);
extern int tp_patch(const char *source_path, const char *delta_path);
*/
import "C"

import (
	"fmt"
	"unsafe"
)

// CGoTurboPatch implements TurboPatch using CGo bindings.
type CGoTurboPatch struct{}

// NewTurboPatch creates a new CGo TurboPatch implementation.
func NewTurboPatch() TurboPatch {
	return &CGoTurboPatch{}
}

// TurboPatchAvailable returns true when turbopatch CGo bindings are compiled in.
func TurboPatchAvailable() bool {
	return true
}

func (tp *CGoTurboPatch) Delta2(oldSigPath, newSigPath, newFilePath, deltaOutputPath string, blockSize int64) error {
	cOldSig := C.CString(oldSigPath)
	defer C.free(unsafe.Pointer(cOldSig))

	cNewSig := C.CString(newSigPath)
	defer C.free(unsafe.Pointer(cNewSig))

	cNewFile := C.CString(newFilePath)
	defer C.free(unsafe.Pointer(cNewFile))

	cTarget := C.CString(deltaOutputPath)
	defer C.free(unsafe.Pointer(cTarget))

	result := C.tp_delta2(cOldSig, cNewSig, cNewFile, cTarget, C.uint64_t(blockSize))
	if result != 0 {
		return fmt.Errorf("tp_delta2 failed with code %d", result)
	}
	return nil
}
