package cli

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/patchkit-net/patchkit-tools-go/internal/api"
	"github.com/patchkit-net/patchkit-tools-go/internal/config"
	"github.com/patchkit-net/patchkit-tools-go/internal/exitcode"
)

// resolveLockTimeout returns the effective lock timeout.
// Explicit --lock-timeout flag wins; otherwise the config value is used.
func resolveLockTimeout(cmd *cobra.Command, cfg *config.Config) (time.Duration, error) {
	if cmd.Flags().Changed("lock-timeout") {
		v, _ := cmd.Flags().GetString("lock-timeout")
		d, err := time.ParseDuration(v)
		if err != nil {
			return 0, fmt.Errorf("invalid --lock-timeout: %w", err)
		}
		return d, nil
	}
	return cfg.LockTimeout, nil
}

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
