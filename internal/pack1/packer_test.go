package pack1

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestPackDir_createsArchiveAndMetadata(t *testing.T) {
	// Create source directory
	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "test.txt"), []byte("hello pack1"), 0644)
	os.MkdirAll(filepath.Join(srcDir, "sub"), 0755)
	os.WriteFile(filepath.Join(srcDir, "sub", "nested.txt"), []byte("nested content"), 0644)

	outDir := t.TempDir()
	archivePath := filepath.Join(outDir, "test.pack1")
	metaPath := filepath.Join(outDir, "test.pack1.meta")

	packer, err := NewPacker("test-password")
	if err != nil {
		t.Fatalf("NewPacker() error: %v", err)
	}

	result, err := packer.PackDir(srcDir, archivePath, metaPath)
	if err != nil {
		t.Fatalf("PackDir() error: %v", err)
	}

	// Verify archive exists and has magic header
	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("failed to read archive: %v", err)
	}

	if len(data) < len(magic) {
		t.Fatal("archive too small")
	}
	for i, b := range magic {
		if data[i] != b {
			t.Fatalf("magic byte %d: got 0x%02X, want 0x%02X", i, data[i], b)
		}
	}

	// Verify metadata
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("failed to read metadata: %v", err)
	}

	var meta Metadata
	if err := json.Unmarshal(metaData, &meta); err != nil {
		t.Fatalf("failed to parse metadata: %v", err)
	}

	if meta.Version != "1.1" {
		t.Errorf("version = %q, want %q", meta.Version, "1.1")
	}
	if meta.Encryption != "aes" {
		t.Errorf("encryption = %q, want %q", meta.Encryption, "aes")
	}
	if meta.Compression != "gzip" {
		t.Errorf("compression = %q, want %q", meta.Compression, "gzip")
	}
	if meta.IV == "" {
		t.Error("IV is empty")
	}

	// Should have directory + 2 regular files
	if len(meta.Files) != 3 {
		t.Errorf("expected 3 file entries, got %d", len(meta.Files))
	}

	// Verify result paths
	if result.ArchivePath != archivePath {
		t.Errorf("ArchivePath = %q, want %q", result.ArchivePath, archivePath)
	}
}

func TestPackDir_regularFileHasNonZeroSize(t *testing.T) {
	srcDir := t.TempDir()
	os.WriteFile(filepath.Join(srcDir, "data.bin"), []byte("some binary data for encryption test"), 0644)

	outDir := t.TempDir()
	packer, err := NewPacker("key123")
	if err != nil {
		t.Fatalf("NewPacker() error: %v", err)
	}

	result, err := packer.PackDir(srcDir, filepath.Join(outDir, "out.pack1"), filepath.Join(outDir, "out.meta"))
	if err != nil {
		t.Fatalf("PackDir() error: %v", err)
	}

	// Find the regular file entry
	for _, f := range result.Metadata.Files {
		if f.Type == "regular" {
			if f.Size <= 0 {
				t.Errorf("encrypted size should be > 0, got %d", f.Size)
			}
			if f.USize <= 0 {
				t.Errorf("uncompressed size should be > 0, got %d", f.USize)
			}
			if f.Offset < int64(len(magic)) {
				t.Errorf("offset %d should be >= magic length %d", f.Offset, len(magic))
			}
		}
	}
}

func TestPackFiles(t *testing.T) {
	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "a.txt")
	os.WriteFile(file1, []byte("content A"), 0644)

	outDir := t.TempDir()
	packer, err := NewPacker("password")
	if err != nil {
		t.Fatalf("NewPacker() error: %v", err)
	}

	result, err := packer.PackFiles(
		map[string]string{"files/a.txt": file1},
		filepath.Join(outDir, "out.pack1"),
		filepath.Join(outDir, "out.meta"),
	)
	if err != nil {
		t.Fatalf("PackFiles() error: %v", err)
	}

	if len(result.Metadata.Files) != 1 {
		t.Errorf("expected 1 file, got %d", len(result.Metadata.Files))
	}
}

func TestEncryptionKey(t *testing.T) {
	// Verify EncryptionKey matches Ruby's Version.encryption_key output:
	//   "\x08\x07\x18\x24" + Base64.encode64("589e266ead17e0fd829ce30d3ccccb1b" + "7").strip
	// Ruby Base64.encode64 for inputs < 60 base64 chars produces the same output
	// as Go's base64.StdEncoding.EncodeToString (no mid-string newlines).
	key := EncryptionKey("589e266ead17e0fd829ce30d3ccccb1b", 7)

	b64Part := base64.StdEncoding.EncodeToString([]byte("589e266ead17e0fd829ce30d3ccccb1b7"))
	want := "\x08\x07\x18\x24" + b64Part

	if key != want {
		t.Errorf("EncryptionKey() = %q, want %q", key, want)
	}
}

func TestEncryptionKey_matchesRubySHA256(t *testing.T) {
	// The AES key derived from the encryption key string must match
	// Ruby's Digest::SHA256.digest(key_string).
	// Ruby known value for secret "589e266ead17e0fd829ce30d3ccccb1b", vid 7:
	//   SHA256 hex = 08b9dbd5373bc0788dd6907c6c44d641fcf9315232175cdc48f9c41f01d1a0a2
	key := EncryptionKey("589e266ead17e0fd829ce30d3ccccb1b", 7)
	hash := sha256.Sum256([]byte(key))

	wantHex := "08b9dbd5373bc0788dd6907c6c44d641fcf9315232175cdc48f9c41f01d1a0a2"
	gotHex := ""
	for _, b := range hash {
		gotHex += sprintf02x(b)
	}

	if gotHex != wantHex {
		t.Errorf("SHA256(EncryptionKey()) = %s, want %s", gotHex, wantHex)
	}
}

func sprintf02x(b byte) string {
	const hex = "0123456789abcdef"
	return string([]byte{hex[b>>4], hex[b&0x0f]})
}

// TestRubyCompatibility_packAndDecrypt packs a file using EncryptionKey and
// verifies it can be decrypted using the same key derivation that Ruby uses
// (single SHA256 of the raw key string). This proves the Go packer produces
// archives compatible with Ruby's Pack1Unpacker.
func TestRubyCompatibility_packAndDecrypt(t *testing.T) {
	content := []byte("hello from Go packer, verifying Ruby compatibility")

	// Create source file
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "test.txt"), content, 0644); err != nil {
		t.Fatal(err)
	}

	// Pack using EncryptionKey (same key derivation as Ruby)
	encKey := EncryptionKey("abc123secret", 5)
	packer, err := NewPacker(encKey)
	if err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	archivePath := filepath.Join(outDir, "test.pack1")
	metaPath := filepath.Join(outDir, "test.pack1.meta")

	result, err := packer.PackDir(srcDir, archivePath, metaPath)
	if err != nil {
		t.Fatal(err)
	}

	// Read metadata
	var meta Metadata
	metaBytes, _ := os.ReadFile(metaPath)
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatal(err)
	}

	// Find the regular file entry
	var entry *FileEntry
	for i := range result.Metadata.Files {
		if result.Metadata.Files[i].Type == "regular" {
			entry = &result.Metadata.Files[i]
			break
		}
	}
	if entry == nil {
		t.Fatal("no regular file in metadata")
	}

	// Decrypt using Ruby-compatible key derivation: SHA256(raw_key_string)
	aesKey := sha256.Sum256([]byte(encKey))
	iv, err := base64.StdEncoding.DecodeString(meta.IV)
	if err != nil {
		t.Fatalf("decode IV: %v", err)
	}

	archiveData, _ := os.ReadFile(archivePath)
	encrypted := archiveData[entry.Offset : entry.Offset+entry.Size]

	// AES-256-CBC decrypt
	block, err := aes.NewCipher(aesKey[:])
	if err != nil {
		t.Fatal(err)
	}
	decrypter := cipher.NewCBCDecrypter(block, iv)
	decrypted := make([]byte, len(encrypted))
	decrypter.CryptBlocks(decrypted, encrypted)

	// Remove PKCS7 padding
	padLen := int(decrypted[len(decrypted)-1])
	if padLen > aes.BlockSize || padLen == 0 {
		t.Fatalf("invalid padding: %d", padLen)
	}
	decrypted = decrypted[:len(decrypted)-padLen]

	// Gzip decompress
	gz, err := gzip.NewReader(bytes.NewReader(decrypted))
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	decompressed, err := io.ReadAll(gz)
	gz.Close()
	if err != nil {
		t.Fatalf("gzip read: %v", err)
	}

	if !bytes.Equal(decompressed, content) {
		t.Errorf("decrypted content = %q, want %q", decompressed, content)
	}
}
