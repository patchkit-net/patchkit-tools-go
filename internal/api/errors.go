package api

import (
	"fmt"

	"github.com/patchkit-net/patchkit-tools-go/internal/exitcode"
)

// APIError represents an error response from the PatchKit API.
type APIError struct {
	URL        string `json:"url,omitempty"`
	StatusCode int    `json:"status_code"`
	Status     string `json:"status,omitempty"`
	Body       string `json:"body,omitempty"`
	Message    string `json:"message,omitempty"`
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("API error %d: %s (url: %s)", e.StatusCode, e.Status, e.URL)
}

func (e *APIError) ExitCode() int {
	switch {
	case e.StatusCode == 401 || e.StatusCode == 403:
		return exitcode.AuthError
	case e.StatusCode == 404:
		return exitcode.NotFound
	case e.StatusCode == 409:
		return exitcode.Conflict
	case e.StatusCode >= 500:
		return exitcode.NetworkError
	default:
		return exitcode.GeneralError
	}
}

// NetworkError represents a network-level error (connection refused, timeout, etc.).
type NetworkError struct {
	Err     error
	URL     string
	Attempt int
	MaxTry  int
}

func (e *NetworkError) Error() string {
	return fmt.Sprintf("network error (attempt %d/%d): %s (url: %s)", e.Attempt, e.MaxTry, e.Err, e.URL)
}

func (e *NetworkError) Unwrap() error {
	return e.Err
}

// JobError represents a server-side processing job failure.
type JobError struct {
	Message string
}

func (e *JobError) Error() string {
	return fmt.Sprintf("processing error: %s", e.Message)
}

func (e *JobError) ExitCode() int {
	return exitcode.ProcessingError
}

// PublishError represents a failure during version publishing.
type PublishError struct {
	Message string
}

func (e *PublishError) Error() string {
	return fmt.Sprintf("publish error: %s", e.Message)
}

func (e *PublishError) ExitCode() int {
	return exitcode.ProcessingError
}

// IsRetryable returns true if the error is a transient network or server error
// that should be retried.
func IsRetryable(err error, method string) bool {
	if err == nil {
		return false
	}

	// Network errors are always retryable
	if _, ok := err.(*NetworkError); ok {
		return true
	}

	// API errors: only 5xx on GET
	if apiErr, ok := err.(*APIError); ok {
		if apiErr.StatusCode >= 500 && method == "GET" {
			return true
		}
	}

	return false
}

// IsServerError returns true if the error is a 5xx server error.
func IsServerError(err error) bool {
	if apiErr, ok := err.(*APIError); ok {
		return apiErr.StatusCode >= 500
	}
	return false
}
