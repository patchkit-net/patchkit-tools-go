package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/patchkit-net/patchkit-tools-go/internal/api"
	"github.com/patchkit-net/patchkit-tools-go/internal/exitcode"
)

// exitErr is a simple error that carries an exit code.
type exitErr struct {
	code int
}

func (e *exitErr) Error() string {
	return fmt.Sprintf("exit code %d", e.code)
}

func exitError(code int) error {
	return &exitErr{code: code}
}

func exitCodeFromError(err error) int {
	if errors.Is(err, context.Canceled) {
		return exitcode.Interrupted
	}
	if apiErr, ok := err.(*api.APIError); ok {
		return apiErr.ExitCode()
	}
	if _, ok := err.(*api.JobError); ok {
		return exitcode.ProcessingError
	}
	if _, ok := err.(*api.PublishError); ok {
		return exitcode.ProcessingError
	}
	return exitcode.GeneralError
}
