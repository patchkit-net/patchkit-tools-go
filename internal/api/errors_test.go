package api

import (
	"fmt"
	"testing"

	"github.com/patchkit-net/patchkit-tools-go/internal/exitcode"
)

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *APIError
		want string
	}{
		{
			name: "with message",
			err:  &APIError{StatusCode: 404, Message: "not found"},
			want: "API error 404: not found",
		},
		{
			name: "without message",
			err:  &APIError{StatusCode: 500, Status: "Internal Server Error", URL: "https://api.patchkit.net/1/apps/abc"},
			want: "API error 500: Internal Server Error (url: https://api.patchkit.net/1/apps/abc)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAPIError_ExitCode(t *testing.T) {
	tests := []struct {
		statusCode int
		want       int
	}{
		{401, exitcode.AuthError},
		{403, exitcode.AuthError},
		{404, exitcode.NotFound},
		{409, exitcode.Conflict},
		{500, exitcode.NetworkError},
		{502, exitcode.NetworkError},
		{503, exitcode.NetworkError},
		{400, exitcode.GeneralError},
		{422, exitcode.GeneralError},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("status_%d", tt.statusCode), func(t *testing.T) {
			err := &APIError{StatusCode: tt.statusCode}
			if got := err.ExitCode(); got != tt.want {
				t.Errorf("ExitCode() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		method string
		want   bool
	}{
		{"nil error", nil, "GET", false},
		{"network error GET", &NetworkError{Err: fmt.Errorf("conn reset")}, "GET", true},
		{"network error POST", &NetworkError{Err: fmt.Errorf("conn reset")}, "POST", true},
		{"5xx on GET", &APIError{StatusCode: 500}, "GET", true},
		{"5xx on POST", &APIError{StatusCode: 500}, "POST", false},
		{"5xx on PUT", &APIError{StatusCode: 502}, "PUT", false},
		{"4xx on GET", &APIError{StatusCode: 404}, "GET", false},
		{"4xx on POST", &APIError{StatusCode: 400}, "POST", false},
		{"random error", fmt.Errorf("some error"), "GET", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err, tt.method); got != tt.want {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.want)
			}
		})
	}
}
