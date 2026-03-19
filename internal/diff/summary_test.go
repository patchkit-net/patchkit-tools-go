package diff

import (
	"encoding/json"
	"testing"
)

func TestSummary_JSON(t *testing.T) {
	s := &Summary{
		AddedFiles:     []string{"new.txt"},
		ModifiedFiles:  []string{"changed.txt"},
		RemovedFiles:   []string{"old.txt"},
		UnchangedFiles: []string{"same.txt"},
	}

	data, err := s.JSON()
	if err != nil {
		t.Fatalf("JSON() error: %v", err)
	}

	var parsed Summary
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if len(parsed.AddedFiles) != 1 || parsed.AddedFiles[0] != "new.txt" {
		t.Errorf("AddedFiles = %v, want [new.txt]", parsed.AddedFiles)
	}
}

func TestSummary_TotalFiles(t *testing.T) {
	s := &Summary{
		AddedFiles:     []string{"a"},
		ModifiedFiles:  []string{"b", "c"},
		RemovedFiles:   []string{"d"},
		UnchangedFiles: []string{"e"},
	}
	if s.TotalFiles() != 5 {
		t.Errorf("TotalFiles() = %d, want 5", s.TotalFiles())
	}
}

func TestSummary_HasChanges(t *testing.T) {
	empty := &Summary{}
	if empty.HasChanges() {
		t.Error("empty summary should not have changes")
	}

	withAdded := &Summary{AddedFiles: []string{"new.txt"}}
	if !withAdded.HasChanges() {
		t.Error("summary with added files should have changes")
	}
}
