package pack1

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	bufferSize = 5 * 1024 * 1024 // 5MB read chunks
)

// EncryptionKey builds the pack1 encryption key string for a given app secret
// and version ID. This must match Ruby's Version.encryption_key:
//
//	"\x08\x07\x18\x24" + Base64.encode64(app_secret + vid.to_s).strip
//
// The returned string is meant to be passed to NewPacker, which SHA256-hashes
// it to derive the AES-256 key.
func EncryptionKey(appSecret string, vid int) string {
	raw := fmt.Sprintf("%s%d", appSecret, vid)
	return "\x08\x07\x18\x24" + base64.StdEncoding.EncodeToString([]byte(raw))
}

// Magic bytes at the start of a Pack1 archive.
var magic = []byte{'P', 'a', 'c', 'k', '1', 0x01, 0x02, 0x03, 0x04}

// FileEntry represents a file in the Pack1 metadata.
type FileEntry struct {
	Name   string `json:"name"`
	Type   string `json:"type"` // "regular", "directory", "symlink"
	Target string `json:"target,omitempty"`
	Mode   string `json:"mode"`
	Offset int64  `json:"offset"`
	Size   int64  `json:"size"`  // encrypted size in archive
	USize  int64  `json:"usize"` // original uncompressed size
}

// Metadata is the Pack1 archive metadata stored as a separate JSON file.
type Metadata struct {
	Version     string      `json:"version"`
	Encryption  string      `json:"encryption"`
	Compression string      `json:"compression"`
	IV          string      `json:"iv"`
	Files       []FileEntry `json:"files"`
}

// Packer creates Pack1 archives (gzip + AES-256-CBC).
type Packer struct {
	key    []byte
	iv     []byte
	offset int64
}

// NewPacker creates a new Pack1 packer with the given encryption key.
// The key is SHA256-hashed to produce a 32-byte AES-256 key.
func NewPacker(password string) (*Packer, error) {
	keyHash := sha256.Sum256([]byte(password))

	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, fmt.Errorf("failed to generate IV: %w", err)
	}

	return &Packer{
		key:    keyHash[:],
		iv:     iv,
		offset: int64(len(magic)),
	}, nil
}

// Result contains the output of packing.
type Result struct {
	ArchivePath  string
	MetadataPath string
	Metadata     *Metadata
}

// PackDir creates a Pack1 archive from a directory.
func (p *Packer) PackDir(sourceDir, archivePath, metadataPath string) (*Result, error) {
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		return nil, err
	}
	defer archiveFile.Close()

	// Write magic
	if _, err := archiveFile.Write(magic); err != nil {
		return nil, err
	}

	meta := &Metadata{
		Version:     "1.1",
		Encryption:  "aes",
		Compression: "gzip",
		IV:          base64.StdEncoding.EncodeToString(p.iv),
	}

	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)

		if relPath == "." {
			return nil
		}

		if info.IsDir() {
			meta.Files = append(meta.Files, FileEntry{
				Name: relPath,
				Type: "directory",
				Mode: fmt.Sprintf("%o", info.Mode()),
			})
			return nil
		}

		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			meta.Files = append(meta.Files, FileEntry{
				Name:   relPath,
				Type:   "symlink",
				Target: target,
				Mode:   fmt.Sprintf("%o", info.Mode()),
			})
			return nil
		}

		entry, err := p.packFile(archiveFile, path, relPath, info)
		if err != nil {
			return err
		}
		meta.Files = append(meta.Files, *entry)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Write metadata JSON
	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(metadataPath, metaJSON, 0644); err != nil {
		return nil, err
	}

	return &Result{
		ArchivePath:  archivePath,
		MetadataPath: metadataPath,
		Metadata:     meta,
	}, nil
}

// PackFiles creates a Pack1 archive from specific files.
// entries maps archive entry name to source file path.
func (p *Packer) PackFiles(entries map[string]string, archivePath, metadataPath string) (*Result, error) {
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		return nil, err
	}
	defer archiveFile.Close()

	if _, err := archiveFile.Write(magic); err != nil {
		return nil, err
	}

	meta := &Metadata{
		Version:     "1.1",
		Encryption:  "aes",
		Compression: "gzip",
		IV:          base64.StdEncoding.EncodeToString(p.iv),
	}

	for entryName, sourcePath := range entries {
		info, err := os.Stat(sourcePath)
		if err != nil {
			return nil, err
		}

		entry, err := p.packFile(archiveFile, sourcePath, entryName, info)
		if err != nil {
			return nil, err
		}
		meta.Files = append(meta.Files, *entry)
	}

	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(metadataPath, metaJSON, 0644); err != nil {
		return nil, err
	}

	return &Result{
		ArchivePath:  archivePath,
		MetadataPath: metadataPath,
		Metadata:     meta,
	}, nil
}

// DeltaEntry holds the data for a single diff archive entry.
// Exactly one of FilePath or Data is set.
type DeltaEntry struct {
	FilePath string
	Data     []byte
	Mode     os.FileMode
}

// PackDeltaEntries creates a Pack1 archive from DeltaEntry sources.
// Entries with Data set are written from memory; entries with FilePath are read from disk.
func (p *Packer) PackDeltaEntries(entries map[string]DeltaEntry, archivePath, metadataPath string) (*Result, error) {
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		return nil, err
	}
	defer archiveFile.Close()

	if _, err := archiveFile.Write(magic); err != nil {
		return nil, err
	}

	meta := &Metadata{
		Version:     "1.1",
		Encryption:  "aes",
		Compression: "gzip",
		IV:          base64.StdEncoding.EncodeToString(p.iv),
	}

	for entryName, entry := range entries {
		var fe *FileEntry
		if entry.Data != nil {
			fe, err = p.packReader(archiveFile, bytes.NewReader(entry.Data), entryName, entry.Mode)
		} else {
			info, statErr := os.Stat(entry.FilePath)
			if statErr != nil {
				return nil, statErr
			}
			fe, err = p.packFile(archiveFile, entry.FilePath, entryName, info)
		}
		if err != nil {
			return nil, err
		}
		meta.Files = append(meta.Files, *fe)
	}

	metaJSON, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(metadataPath, metaJSON, 0644); err != nil {
		return nil, err
	}

	return &Result{
		ArchivePath:  archivePath,
		MetadataPath: metadataPath,
		Metadata:     meta,
	}, nil
}

func (p *Packer) packReader(w io.Writer, src io.Reader, entryName string, mode os.FileMode) (*FileEntry, error) {
	startOffset := p.offset
	var totalUncompressed int64

	block, err := aes.NewCipher(p.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	cw := &countingWriter{w: w}
	stream := cipher.NewCBCEncrypter(block, p.iv)
	encWriter := &cbcWriter{w: cw, block: stream, blockSize: aes.BlockSize}

	gzWriter := gzip.NewWriter(encWriter)

	buf := make([]byte, bufferSize)
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			totalUncompressed += int64(n)
			if _, err := gzWriter.Write(buf[:n]); err != nil {
				return nil, err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}

	if err := gzWriter.Close(); err != nil {
		return nil, err
	}

	if err := encWriter.Close(); err != nil {
		return nil, err
	}

	encryptedSize := cw.count
	p.offset += int64(encryptedSize)

	return &FileEntry{
		Name:   entryName,
		Type:   "regular",
		Mode:   fmt.Sprintf("%o", mode),
		Offset: startOffset,
		Size:   int64(encryptedSize),
		USize:  totalUncompressed,
	}, nil
}

func (p *Packer) packFile(w io.Writer, sourcePath, entryName string, info os.FileInfo) (*FileEntry, error) {
	src, err := os.Open(sourcePath)
	if err != nil {
		return nil, err
	}
	defer src.Close()

	startOffset := p.offset
	var totalUncompressed int64

	// Create encryption stream
	block, err := aes.NewCipher(p.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	// Use a counting writer to track encrypted output size
	cw := &countingWriter{w: w}
	stream := cipher.NewCBCEncrypter(block, p.iv)
	encWriter := &cbcWriter{w: cw, block: stream, blockSize: aes.BlockSize}

	// Compress then encrypt
	gzWriter := gzip.NewWriter(encWriter)

	buf := make([]byte, bufferSize)
	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			totalUncompressed += int64(n)
			if _, err := gzWriter.Write(buf[:n]); err != nil {
				return nil, err
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}

	// Finalize gzip
	if err := gzWriter.Close(); err != nil {
		return nil, err
	}

	// Finalize CBC padding
	if err := encWriter.Close(); err != nil {
		return nil, err
	}

	encryptedSize := cw.count
	p.offset += int64(encryptedSize)

	return &FileEntry{
		Name:   entryName,
		Type:   "regular",
		Mode:   fmt.Sprintf("%o", info.Mode()),
		Offset: startOffset,
		Size:   int64(encryptedSize),
		USize:  totalUncompressed,
	}, nil
}

// cbcWriter wraps a CBC encrypter as an io.WriteCloser.
// It buffers data until a full block is available, then encrypts and writes.
type cbcWriter struct {
	w         io.Writer
	block     cipher.BlockMode
	blockSize int
	buf       []byte
}

func (cw *cbcWriter) Write(p []byte) (int, error) {
	cw.buf = append(cw.buf, p...)
	total := len(p)

	for len(cw.buf) >= cw.blockSize {
		// Encrypt one block at a time
		blockData := cw.buf[:cw.blockSize]
		cw.block.CryptBlocks(blockData, blockData)
		if _, err := cw.w.Write(blockData); err != nil {
			return 0, err
		}
		cw.buf = cw.buf[cw.blockSize:]
	}

	return total, nil
}

func (cw *cbcWriter) Close() error {
	// PKCS7 padding
	padLen := cw.blockSize - len(cw.buf)%cw.blockSize
	for i := 0; i < padLen; i++ {
		cw.buf = append(cw.buf, byte(padLen))
	}

	cw.block.CryptBlocks(cw.buf, cw.buf)
	_, err := cw.w.Write(cw.buf)
	cw.buf = nil
	return err
}

// countingWriter wraps a writer and counts bytes written.
type countingWriter struct {
	w     io.Writer
	count int
}

func (cw *countingWriter) Write(p []byte) (int, error) {
	n, err := cw.w.Write(p)
	cw.count += n
	return n, err
}
