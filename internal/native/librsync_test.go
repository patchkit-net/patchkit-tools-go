package native

import (
	"bytes"
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

func TestPureGoRsync_DeltaToWriter(t *testing.T) {
	tmpDir := t.TempDir()

	// Create basis and new files
	basisPath := filepath.Join(tmpDir, "basis.txt")
	basisData := []byte("Hello, World! This is the original file content for librsync testing. " +
		"It needs to be long enough to have meaningful blocks for signature generation. " +
		"Adding more content here to ensure we have multiple blocks to work with.")
	os.WriteFile(basisPath, basisData, 0644)

	newPath := filepath.Join(tmpDir, "new.txt")
	newData := []byte("Hello, World! This is the MODIFIED file content for librsync testing. " +
		"It needs to be long enough to have meaningful blocks for signature generation. " +
		"Adding more content here to ensure we have multiple blocks to work with. Extra data appended.")
	os.WriteFile(newPath, newData, 0644)

	// Generate signature
	sigPath := filepath.Join(tmpDir, "basis.sig")
	rsync := NewRsync()
	if err := rsync.Signature(basisPath, sigPath, 256); err != nil {
		t.Fatalf("Signature() error: %v", err)
	}

	// Generate delta to file (reference)
	deltaPath := filepath.Join(tmpDir, "delta.bin")
	if err := rsync.Delta(sigPath, newPath, deltaPath); err != nil {
		t.Fatalf("Delta() error: %v", err)
	}
	fileDelta, err := os.ReadFile(deltaPath)
	if err != nil {
		t.Fatalf("reading delta file: %v", err)
	}

	// Generate delta to writer (streaming)
	var buf bytes.Buffer
	if err := rsync.DeltaToWriter(sigPath, newPath, &buf); err != nil {
		t.Fatalf("DeltaToWriter() error: %v", err)
	}

	// Both should produce identical output
	if !bytes.Equal(fileDelta, buf.Bytes()) {
		t.Errorf("DeltaToWriter output (%d bytes) differs from Delta file output (%d bytes)",
			buf.Len(), len(fileDelta))
	}

	if buf.Len() == 0 {
		t.Fatal("DeltaToWriter produced empty output")
	}
}

func TestPureGoRsync_DeltaToWriter_LargeFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a large basis file (1MB)
	basisPath := filepath.Join(tmpDir, "basis.bin")
	basisData := make([]byte, 1024*1024)
	for i := range basisData {
		basisData[i] = byte(i % 256)
	}
	os.WriteFile(basisPath, basisData, 0644)

	// Create a slightly modified version
	newPath := filepath.Join(tmpDir, "new.bin")
	newData := make([]byte, len(basisData))
	copy(newData, basisData)
	// Modify a few blocks in the middle
	for i := 512 * 1024; i < 512*1024+4096; i++ {
		newData[i] = byte((i * 7) % 256)
	}
	os.WriteFile(newPath, newData, 0644)

	// Generate signature
	sigPath := filepath.Join(tmpDir, "basis.sig")
	rsync := NewRsync()
	if err := rsync.Signature(basisPath, sigPath, 2048); err != nil {
		t.Fatalf("Signature() error: %v", err)
	}

	// Stream delta to memory
	var buf bytes.Buffer
	if err := rsync.DeltaToWriter(sigPath, newPath, &buf); err != nil {
		t.Fatalf("DeltaToWriter() error: %v", err)
	}

	// Delta should be much smaller than the full file
	if buf.Len() == 0 {
		t.Fatal("DeltaToWriter produced empty output")
	}
	if int64(buf.Len()) >= int64(len(newData)) {
		t.Errorf("delta (%d bytes) should be smaller than new file (%d bytes)", buf.Len(), len(newData))
	}
	t.Logf("1MB file delta: %d bytes (%.1f%% of original)", buf.Len(), float64(buf.Len())/float64(len(newData))*100)
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
