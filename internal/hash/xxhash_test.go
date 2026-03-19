package hash

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// buildSanityBuffer creates the reference xxHash test buffer.
// From xxHash reference: sanityBuffer[i] = (byte)(i * PRIME32)
// where PRIME32 = 2654435761 (0x9E3779B1)
func buildSanityBuffer(size int) []byte {
	const prime32 uint32 = 0x9E3779B1
	buf := make([]byte, size)
	for i := 0; i < size; i++ {
		buf[i] = byte(uint32(i) * prime32)
	}
	return buf
}

// Known test vectors for xxh32.
// Reference: https://github.com/Cyan4973/xxHash/blob/dev/cli/xsum_sanity_check.c
func TestXXH32Bytes_emptyWithSeed0(t *testing.T) {
	got := XXH32Bytes([]byte{}, 0)
	want := uint32(0x02CC5D05)
	if got != want {
		t.Errorf("XXH32({}, 0) = 0x%08X, want 0x%08X", got, want)
	}
}

func TestXXH32Bytes_referenceVectors(t *testing.T) {
	// Reference test vectors from xxHash using the sanity buffer.
	// PRIME32 used as seed = 2654435761 = 0x9E3779B1
	const primeSeed uint32 = 0x9E3779B1
	sanity := buildSanityBuffer(256)

	tests := []struct {
		name string
		data []byte
		seed uint32
		want uint32
	}{
		{"0 bytes seed 0", sanity[:0], 0, 0x02CC5D05},
		{"0 bytes seed PRIME", sanity[:0], primeSeed, 0x36B78AE7},
		{"1 byte seed 0", sanity[:1], 0, 0xCF65B03E},
		{"1 byte seed PRIME", sanity[:1], primeSeed, 0xB4545AA4},
		{"14 bytes seed 0", sanity[:14], 0, 0x92772A5E},
		{"14 bytes seed PRIME", sanity[:14], primeSeed, 0x71DBA2FB},
		{"222 bytes seed 0", sanity[:222], 0, 0x01D080E9},
		{"222 bytes seed PRIME", sanity[:222], primeSeed, 0x25D0FE33},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := XXH32Bytes(tt.data, tt.seed)
			if got != tt.want {
				t.Errorf("XXH32(sanity[:%d], 0x%X) = 0x%08X, want 0x%08X", len(tt.data), tt.seed, got, tt.want)
			}
		})
	}
}

func TestXXH32Bytes_consistency(t *testing.T) {
	// Same input should always produce same output
	data := []byte("Hello, World! This is a test string for xxhash.")
	h1 := XXH32Bytes(data, 42)
	h2 := XXH32Bytes(data, 42)
	if h1 != h2 {
		t.Errorf("inconsistent: 0x%08X != 0x%08X", h1, h2)
	}

	// Different seed should produce different output
	h3 := XXH32Bytes(data, 0)
	if h1 == h3 {
		t.Error("different seeds produced same hash")
	}
}

func TestXXH32Reader_matchesBytes(t *testing.T) {
	data := []byte("Test data for streaming vs one-shot comparison")
	seed := uint32(42)

	expected := XXH32Bytes(data, seed)

	got, err := XXH32Reader(bytes.NewReader(data), seed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("Reader result 0x%08X != Bytes result 0x%08X", got, expected)
	}
}

func TestXXH32Reader_largeData(t *testing.T) {
	// Test with data larger than the internal buffer (>16 bytes, forces multi-block processing)
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte(i)
	}

	expected := XXH32Bytes(data, 42)
	got, err := XXH32Reader(bytes.NewReader(data), 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("Reader result 0x%08X != Bytes result 0x%08X", got, expected)
	}
}

func TestXXH32File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testfile.bin")
	data := []byte("file content for xxhash test")
	os.WriteFile(path, data, 0644)

	expected := XXH32Bytes(data, 42)
	got, err := XXH32File(path, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != expected {
		t.Errorf("File result 0x%08X != Bytes result 0x%08X", got, expected)
	}
}

func TestXXH32File_notFound(t *testing.T) {
	_, err := XXH32File("/nonexistent/file", 42)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
