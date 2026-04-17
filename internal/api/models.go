package api

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// IntOrFalse handles API fields that return either an int or false.
// PatchKit API uses false when no version is being processed/published,
// or the version ID (int) when one is active.
type IntOrFalse struct {
	Value int
	Set   bool
}

func (f *IntOrFalse) UnmarshalJSON(data []byte) error {
	// JSON null means "no version" — same semantics as false.
	// This check is required because Go's encoding/json treats null into a
	// non-pointer int as a no-op (nil error, value untouched), which would
	// otherwise misread "processing_version": null as Set=true, Value=0.
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		f.Value = 0
		f.Set = false
		return nil
	}
	// Try int first
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		f.Value = i
		f.Set = true
		return nil
	}
	// Fall back to bool (false means not set)
	f.Value = 0
	f.Set = false
	return nil
}

func (f IntOrFalse) MarshalJSON() ([]byte, error) {
	if f.Set {
		return json.Marshal(f.Value)
	}
	return json.Marshal(false)
}

// App represents a PatchKit application.
type App struct {
	ID                         int            `json:"id"`
	Name                       string         `json:"name"`
	Secret                     string         `json:"secret"`
	Platform                   string         `json:"platform"`
	IsChannel                  bool           `json:"is_channel"`
	AllowChannelDirectPublish  bool           `json:"allow_channel_direct_publish"`
	DiffAlgorithm              string         `json:"diff_algorithm"`
	ProcessingVersion          IntOrFalse     `json:"processing_version"`
	PublishingVersion          IntOrFalse     `json:"publishing_version"`
	ParentGroup                *ParentGroup   `json:"parent_group,omitempty"`
	PublishedVersions          int            `json:"published_versions_count"`
	DraftVersions              int            `json:"draft_versions_count"`
}

// ParentGroup represents the parent group of a channel app.
type ParentGroup struct {
	Secret string `json:"secret"`
	Name   string `json:"name"`
}

// ProcessingMessage represents a processing message from the server.
type ProcessingMessage struct {
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// Version represents a PatchKit version.
type Version struct {
	ID                   int                 `json:"id"`
	Label                string              `json:"label"`
	Changelog            string              `json:"changelog"`
	Published            bool                `json:"published"`
	Draft                bool                `json:"draft"`
	PendingPublish       bool                `json:"pending_publish"`
	PublishProgress      float64             `json:"publish_progress"`
	HasProcessingError   bool                `json:"has_processing_error"`
	ProcessingMessages   []ProcessingMessage `json:"processing_messages"`
	ProcessingProgress   float64             `json:"processing_progress"`
	PublishWhenProcessed bool                `json:"publish_when_processed"`
	CanBeImported        bool                `json:"can_be_imported"`
}

// JobStatus represents the status of a background processing job.
type JobStatus struct {
	GUID          string  `json:"guid"`
	Finished      bool    `json:"finished"`
	Pending       bool    `json:"pending"`
	Progress      float64 `json:"progress"`
	Status        int     `json:"status"`
	StatusMessage string  `json:"status_message"`
}

// GlobalLock represents a global processing lock.
type GlobalLock struct {
	Status        string `json:"status"` // "allow" or "deny"
	QueuePosition int    `json:"queue_position"`
}

// Upload represents a created upload session.
type Upload struct {
	ID StringOrInt `json:"id"`
}

// StringOrInt handles JSON fields that can be either a string or an integer,
// storing the value as a string either way.
type StringOrInt string

func (s *StringOrInt) UnmarshalJSON(data []byte) error {
	// Try string first
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = StringOrInt(str)
		return nil
	}
	// Try int
	var num int64
	if err := json.Unmarshal(data, &num); err == nil {
		*s = StringOrInt(fmt.Sprintf("%d", num))
		return nil
	}
	return fmt.Errorf("cannot unmarshal %s as string or int", string(data))
}

func (s StringOrInt) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(s))
}

func (s StringOrInt) String() string {
	return string(s)
}

// ChunkURL represents a presigned URL for uploading a chunk.
type ChunkURL struct {
	URL string `json:"url"`
}

// SignaturesInfo represents signature download information.
type SignaturesInfo struct {
	URL  string `json:"url"`
	Size int64  `json:"size"`
}

// ContentSummary represents version content file hashes.
type ContentSummary struct {
	Files map[string]string `json:"files"`
}

// Pack1KeyResponse represents the response from the pack1_key endpoint.
type Pack1KeyResponse struct {
	Key string `json:"key"`
}

// VersionCreateResponse represents the response from creating a version.
type VersionCreateResponse struct {
	ID int `json:"id"`
}

// UploadResponse represents the response from uploading content/diff.
type UploadResponse struct {
	JobGUID string `json:"job_guid"`
}

// ImportResponse represents the response from importing a version.
type ImportResponse struct {
	JobGUID string `json:"job_guid"`
}

// LinkResponse represents the response from linking a channel version.
type LinkResponse struct {
	JobGUID string `json:"job_guid"`
}
