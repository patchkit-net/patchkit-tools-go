//go:build !cgo_turbopatch

package native

import "fmt"

// StubTurboPatch is a no-op implementation when turbopatch CGo is not available.
type StubTurboPatch struct{}

// NewTurboPatch returns a stub that returns errors when called.
func NewTurboPatch() TurboPatch {
	return &StubTurboPatch{}
}

// TurboPatchAvailable returns false when turbopatch CGo bindings are not compiled in.
func TurboPatchAvailable() bool {
	return false
}

func (tp *StubTurboPatch) Delta2(oldSigPath, newSigPath, newFilePath, deltaOutputPath string, blockSize int64) error {
	return fmt.Errorf("turbopatch is not available: build with -tags cgo_turbopatch")
}
