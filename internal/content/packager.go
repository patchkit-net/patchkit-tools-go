package content

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultCompressionLevel = 2

// Packager creates ZIP content packages from a directory.
type Packager struct {
	compressionLevel int
}

// NewPackager creates a content packager with default compression level 2.
func NewPackager() *Packager {
	return &Packager{compressionLevel: defaultCompressionLevel}
}

// PackDir creates a ZIP archive from all files in sourceDir.
// Files are stored with paths relative to sourceDir.
func (p *Packager) PackDir(sourceDir, outputPath string) error {
	outFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	zw := zip.NewWriter(outFile)
	defer zw.Close()

	// Register custom compressor with our compression level
	zw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return newFlateWriter(w, p.compressionLevel)
	})

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories - ZIP creates them implicitly
		if info.IsDir() {
			return nil
		}

		// Skip symlinks
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		// Normalize to forward slashes for ZIP
		relPath = filepath.ToSlash(relPath)

		return p.addFile(zw, path, relPath, info)
	})
}

// PackFiles creates a ZIP archive from specific files.
// entries maps archive entry name to source file path.
func (p *Packager) PackFiles(entries map[string]string, outputPath string) error {
	outFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	zw := zip.NewWriter(outFile)
	defer zw.Close()

	zw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return newFlateWriter(w, p.compressionLevel)
	})

	for entryName, sourcePath := range entries {
		info, err := os.Stat(sourcePath)
		if err != nil {
			return err
		}
		if err := p.addFile(zw, sourcePath, entryName, info); err != nil {
			return err
		}
	}

	return nil
}

// DeltaEntry holds the data for a single diff archive entry.
// Exactly one of FilePath or Data is set.
type DeltaEntry struct {
	FilePath string
	Data     []byte
	Mode     os.FileMode
}

// PackDeltaEntries creates a ZIP archive from DeltaEntry sources.
// Entries with Data set are written from memory; entries with FilePath are read from disk.
func (p *Packager) PackDeltaEntries(entries map[string]DeltaEntry, outputPath string) error {
	outFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	zw := zip.NewWriter(outFile)
	defer zw.Close()

	zw.RegisterCompressor(zip.Deflate, func(w io.Writer) (io.WriteCloser, error) {
		return newFlateWriter(w, p.compressionLevel)
	})

	for entryName, entry := range entries {
		if entry.Data != nil {
			if err := p.addFromMemory(zw, entryName, entry.Data, entry.Mode); err != nil {
				return err
			}
		} else {
			info, err := os.Stat(entry.FilePath)
			if err != nil {
				return err
			}
			if err := p.addFile(zw, entry.FilePath, entryName, info); err != nil {
				return err
			}
		}
	}

	return nil
}

func (p *Packager) addFromMemory(zw *zip.Writer, entryName string, data []byte, mode os.FileMode) error {
	header := &zip.FileHeader{
		Name:               entryName,
		Method:             zip.Deflate,
		Modified:           time.Now(),
		UncompressedSize64: uint64(len(data)),
	}
	header.SetMode(mode)

	w, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}

	_, err = io.Copy(w, bytes.NewReader(data))
	return err
}

func (p *Packager) addFile(zw *zip.Writer, sourcePath, entryName string, info os.FileInfo) error {
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = entryName
	header.Method = zip.Deflate

	w, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}

	f, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(w, f)
	return err
}

// ListFiles returns all regular files in a directory with paths relative to dir.
func ListFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	return files, err
}

// NormalizeEntryName normalizes a file path for use as a ZIP entry name.
func NormalizeEntryName(path string) string {
	return strings.TrimPrefix(filepath.ToSlash(path), "/")
}
