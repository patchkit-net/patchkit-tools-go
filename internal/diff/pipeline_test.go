package diff

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/patchkit-net/patchkit-tools-go/internal/native"
)

func TestRun_InMemoryDeltas(t *testing.T) {
	tmpDir := t.TempDir()

	// Create content directory with files
	contentDir := filepath.Join(tmpDir, "content")
	os.MkdirAll(filepath.Join(contentDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(contentDir, "same.txt"), []byte(
		"This file exists in both versions and has been modified. "+
			"Adding enough content to make meaningful blocks for librsync."+
			"More content to fill the blocks up to a useful size for testing."),
		0644)
	os.WriteFile(filepath.Join(contentDir, "new_file.txt"), []byte("this is a brand new file"), 0644)
	os.WriteFile(filepath.Join(contentDir, "subdir", "nested.txt"), []byte(
		"Nested file that was also modified between versions. "+
			"Needs to be long enough for librsync to generate meaningful blocks."+
			"Adding more text here to pad the file to a reasonable size for testing."),
		0644)

	// Create signatures from a "previous version"
	prevDir := filepath.Join(tmpDir, "prev")
	os.MkdirAll(filepath.Join(prevDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(prevDir, "same.txt"), []byte(
		"This file exists in both versions and was the original. "+
			"Adding enough content to make meaningful blocks for librsync."+
			"More content to fill the blocks up to a useful size for testing."),
		0644)
	os.WriteFile(filepath.Join(prevDir, "removed.txt"), []byte("this file was removed"), 0644)
	os.WriteFile(filepath.Join(prevDir, "subdir", "nested.txt"), []byte(
		"Nested file from the previous version of the application. "+
			"Needs to be long enough for librsync to generate meaningful blocks."+
			"Adding more text here to pad the file to a reasonable size for testing."),
		0644)

	// Generate signatures for the "previous version"
	sigsDir := filepath.Join(tmpDir, "sigs")
	rsync := native.NewRsync()
	for _, relPath := range []string{"same.txt", "removed.txt", "subdir/nested.txt"} {
		srcPath := filepath.Join(prevDir, relPath)
		sigPath := filepath.Join(sigsDir, relPath)
		os.MkdirAll(filepath.Dir(sigPath), 0755)
		if err := rsync.Signature(srcPath, sigPath, 256); err != nil {
			t.Fatalf("generating signature for %s: %v", relPath, err)
		}
	}

	// Run the diff pipeline
	result, err := Run(context.Background(), &Config{
		ContentDir:    contentDir,
		SignaturesDir: sigsDir,
		TempDir:       tmpDir,
		Algorithm:     native.AlgorithmLibrsync,
		Workers:       2,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Verify summary
	if len(result.Summary.AddedFiles) != 1 || result.Summary.AddedFiles[0] != "new_file.txt" {
		t.Errorf("AddedFiles = %v, want [new_file.txt]", result.Summary.AddedFiles)
	}
	if len(result.Summary.RemovedFiles) != 1 || result.Summary.RemovedFiles[0] != "removed.txt" {
		t.Errorf("RemovedFiles = %v, want [removed.txt]", result.Summary.RemovedFiles)
	}
	if len(result.Summary.ModifiedFiles) != 2 {
		t.Errorf("ModifiedFiles = %v, want 2 entries", result.Summary.ModifiedFiles)
	}

	// Verify delta entries
	if len(result.DeltaFiles) != 3 {
		t.Fatalf("DeltaFiles has %d entries, want 3", len(result.DeltaFiles))
	}

	// Added file should have FilePath set (points to original content)
	newFileEntry := result.DeltaFiles["new_file.txt"]
	if newFileEntry.FilePath == "" {
		t.Error("added file entry should have FilePath set")
	}
	if newFileEntry.Data != nil {
		t.Error("added file entry should not have Data set")
	}

	// Modified files should have Data set (in-memory delta)
	sameEntry := result.DeltaFiles["same.txt"]
	if sameEntry.Data == nil {
		t.Fatal("modified file 'same.txt' should have Data set")
	}
	if sameEntry.FilePath != "" {
		t.Error("modified file 'same.txt' should not have FilePath set")
	}
	if len(sameEntry.Data) == 0 {
		t.Error("modified file 'same.txt' has empty delta data")
	}
	if sameEntry.Mode == 0 {
		t.Error("modified file 'same.txt' should have Mode set")
	}

	nestedEntry := result.DeltaFiles["subdir/nested.txt"]
	if nestedEntry.Data == nil {
		t.Fatal("modified file 'subdir/nested.txt' should have Data set")
	}
	if len(nestedEntry.Data) == 0 {
		t.Error("modified file 'subdir/nested.txt' has empty delta data")
	}

	t.Logf("same.txt delta: %d bytes", len(sameEntry.Data))
	t.Logf("subdir/nested.txt delta: %d bytes", len(nestedEntry.Data))
}

func TestRun_AllAdded(t *testing.T) {
	tmpDir := t.TempDir()

	contentDir := filepath.Join(tmpDir, "content")
	os.MkdirAll(contentDir, 0755)
	os.WriteFile(filepath.Join(contentDir, "a.txt"), []byte("file a"), 0644)
	os.WriteFile(filepath.Join(contentDir, "b.txt"), []byte("file b"), 0644)

	sigsDir := filepath.Join(tmpDir, "sigs")
	os.MkdirAll(sigsDir, 0755) // empty sigs dir

	result, err := Run(context.Background(), &Config{
		ContentDir:    contentDir,
		SignaturesDir: sigsDir,
		TempDir:       tmpDir,
		Algorithm:     native.AlgorithmLibrsync,
		Workers:       2,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if len(result.Summary.AddedFiles) != 2 {
		t.Errorf("AddedFiles = %v, want 2 entries", result.Summary.AddedFiles)
	}
	if len(result.DeltaFiles) != 2 {
		t.Errorf("DeltaFiles has %d entries, want 2", len(result.DeltaFiles))
	}

	// All entries should be file-based
	for name, entry := range result.DeltaFiles {
		if entry.FilePath == "" {
			t.Errorf("added file %s should have FilePath", name)
		}
		if entry.Data != nil {
			t.Errorf("added file %s should not have Data", name)
		}
	}
}
