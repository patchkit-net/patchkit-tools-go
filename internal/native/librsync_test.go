package native

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPureGoRsync_SignatureAndDelta(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a basis file
	basisPath := filepath.Join(tmpDir, "basis.txt")
	basisData := []byte("Hello, World! This is the original file content for librsync testing. " +
		"It needs to be long enough to have meaningful blocks for signature generation. " +
		"Adding more content here to ensure we have multiple blocks to work with.")
	os.WriteFile(basisPath, basisData, 0644)

	// Generate signature
	sigPath := filepath.Join(tmpDir, "basis.sig")
	rsync := NewRsync()
	err := rsync.Signature(basisPath, sigPath, 256)
	if err != nil {
		t.Fatalf("Signature() error: %v", err)
	}

	// Verify signature file exists and is non-empty
	sigInfo, err := os.Stat(sigPath)
	if err != nil {
		t.Fatalf("signature file not found: %v", err)
	}
	if sigInfo.Size() == 0 {
		t.Fatal("signature file is empty")
	}

	// Create a new file (slightly different from basis)
	newPath := filepath.Join(tmpDir, "new.txt")
	newData := []byte("Hello, World! This is the MODIFIED file content for librsync testing. " +
		"It needs to be long enough to have meaningful blocks for signature generation. " +
		"Adding more content here to ensure we have multiple blocks to work with. Extra data appended.")
	os.WriteFile(newPath, newData, 0644)

	// Generate delta
	deltaPath := filepath.Join(tmpDir, "delta.bin")
	err = rsync.Delta(sigPath, newPath, deltaPath)
	if err != nil {
		t.Fatalf("Delta() error: %v", err)
	}

	// Verify delta file exists and is non-empty
	deltaInfo, err := os.Stat(deltaPath)
	if err != nil {
		t.Fatalf("delta file not found: %v", err)
	}
	if deltaInfo.Size() == 0 {
		t.Fatal("delta file is empty")
	}

	// Delta should be smaller than the full new file
	if deltaInfo.Size() >= int64(len(newData)) {
		t.Logf("warning: delta (%d bytes) is not smaller than new file (%d bytes)", deltaInfo.Size(), len(newData))
	}
}

func TestReadSignatureBlockLen(t *testing.T) {
	tmpDir := t.TempDir()

	basisPath := filepath.Join(tmpDir, "file.txt")
	os.WriteFile(basisPath, []byte("test content for signature block length reading"), 0644)

	sigPath := filepath.Join(tmpDir, "file.sig")
	rsync := NewRsync()
	err := rsync.Signature(basisPath, sigPath, 512)
	if err != nil {
		t.Fatalf("Signature() error: %v", err)
	}

	blockLen, err := ReadSignatureBlockLen(sigPath)
	if err != nil {
		t.Fatalf("ReadSignatureBlockLen() error: %v", err)
	}
	if blockLen != 512 {
		t.Errorf("block length = %d, want 512", blockLen)
	}
}
