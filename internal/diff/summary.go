package diff

import "encoding/json"

// Summary describes the result of a diff operation.
type Summary struct {
	Size              int64    `json:"size"`
	UncompressedSize  int64    `json:"uncompressed_size"`
	CompressionMethod string   `json:"compression_method"`
	EncryptionMethod  string   `json:"encryption_method"`
	AddedFiles        []string `json:"added_files"`
	ModifiedFiles     []string `json:"modified_files"`
	RemovedFiles      []string `json:"removed_files"`
	UnchangedFiles    []string `json:"unchanged_files"`
}

// JSON returns the summary as a JSON byte slice.
// Nil slices are replaced with empty slices so JSON encodes [] not null.
func (s *Summary) JSON() ([]byte, error) {
	cp := *s
	if cp.AddedFiles == nil {
		cp.AddedFiles = []string{}
	}
	if cp.ModifiedFiles == nil {
		cp.ModifiedFiles = []string{}
	}
	if cp.RemovedFiles == nil {
		cp.RemovedFiles = []string{}
	}
	if cp.UnchangedFiles == nil {
		cp.UnchangedFiles = []string{}
	}
	return json.Marshal(&cp)
}

// JSONIndent returns the summary as a formatted JSON byte slice.
func (s *Summary) JSONIndent() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// TotalFiles returns the total number of files across all categories.
func (s *Summary) TotalFiles() int {
	return len(s.AddedFiles) + len(s.ModifiedFiles) + len(s.RemovedFiles) + len(s.UnchangedFiles)
}

// HasChanges returns true if there are any added, modified, or removed files.
func (s *Summary) HasChanges() bool {
	return len(s.AddedFiles) > 0 || len(s.ModifiedFiles) > 0 || len(s.RemovedFiles) > 0
}
