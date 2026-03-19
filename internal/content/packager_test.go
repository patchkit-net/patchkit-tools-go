package content

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestPackDir(t *testing.T) {
	// Create source directory with some files
	srcDir := t.TempDir()
	os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(srcDir, "hello.txt"), []byte("hello world"), 0644)
	os.WriteFile(filepath.Join(srcDir, "subdir", "nested.txt"), []byte("nested file"), 0644)

	// Pack
	outputPath := filepath.Join(t.TempDir(), "output.zip")
	p := NewPackager()
	err := p.PackDir(srcDir, outputPath)
	if err != nil {
		t.Fatalf("PackDir() error: %v", err)
	}

	// Verify ZIP contents
	zr, err := zip.OpenReader(outputPath)
	if err != nil {
		t.Fatalf("failed to open ZIP: %v", err)
	}
	defer zr.Close()

	fileNames := make(map[string]bool)
	for _, f := range zr.File {
		fileNames[f.Name] = true
	}

	if !fileNames["hello.txt"] {
		t.Error("missing hello.txt in ZIP")
	}
	if !fileNames["subdir/nested.txt"] {
		t.Error("missing subdir/nested.txt in ZIP")
	}
	if len(zr.File) != 2 {
		t.Errorf("expected 2 files, got %d", len(zr.File))
	}
}

func TestPackFiles(t *testing.T) {
	tmpDir := t.TempDir()
	file1 := filepath.Join(tmpDir, "a.txt")
	file2 := filepath.Join(tmpDir, "b.txt")
	os.WriteFile(file1, []byte("file a"), 0644)
	os.WriteFile(file2, []byte("file b"), 0644)

	outputPath := filepath.Join(t.TempDir(), "output.zip")
	p := NewPackager()
	err := p.PackFiles(map[string]string{
		"data/a.txt": file1,
		"data/b.txt": file2,
	}, outputPath)
	if err != nil {
		t.Fatalf("PackFiles() error: %v", err)
	}

	zr, err := zip.OpenReader(outputPath)
	if err != nil {
		t.Fatalf("failed to open ZIP: %v", err)
	}
	defer zr.Close()

	if len(zr.File) != 2 {
		t.Errorf("expected 2 files, got %d", len(zr.File))
	}
}

func TestListFiles(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "a", "b"), 0755)
	os.WriteFile(filepath.Join(dir, "root.txt"), []byte("r"), 0644)
	os.WriteFile(filepath.Join(dir, "a", "mid.txt"), []byte("m"), 0644)
	os.WriteFile(filepath.Join(dir, "a", "b", "deep.txt"), []byte("d"), 0644)

	files, err := ListFiles(dir)
	if err != nil {
		t.Fatalf("ListFiles() error: %v", err)
	}
	if len(files) != 3 {
		t.Errorf("expected 3 files, got %d: %v", len(files), files)
	}
}
