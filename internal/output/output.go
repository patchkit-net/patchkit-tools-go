package output

import (
	"os"

	"golang.org/x/term"
)

// Mode represents the output mode.
type Mode string

const (
	ModeText Mode = "text"
	ModeJSON Mode = "json"
)

// Outputter is the interface for all output operations.
type Outputter interface {
	// Info prints an informational message (text mode: stderr, json/quiet: suppressed).
	Info(msg string)

	// Infof prints a formatted informational message.
	Infof(format string, args ...interface{})

	// Warn prints a warning message.
	Warn(msg string)

	// Warnf prints a formatted warning message.
	Warnf(format string, args ...interface{})

	// Error prints an error message with optional suggestion.
	Error(err error, suggestion string)

	// Result outputs the final result (text: formatted, json: to stdout).
	Result(v interface{})

	// StartProgress begins a progress bar for a stage.
	StartProgress(stage string, total int64)

	// UpdateProgress updates the current progress.
	UpdateProgress(current int64)

	// UpdateProgressMessage updates the progress bar message.
	UpdateProgressMessage(msg string)

	// EndProgress completes the current progress bar.
	EndProgress()

	// IsTerminal returns true if stdout is a terminal.
	IsTerminal() bool
}

// New creates an Outputter based on the given mode.
func New(mode Mode, quiet bool, progressEvents bool) Outputter {
	switch mode {
	case ModeJSON:
		return NewJSONOutput(progressEvents)
	default:
		if quiet {
			return NewQuietOutput()
		}
		return NewTextOutput()
	}
}

// IsTerminal checks if file descriptor is a terminal.
func IsStdoutTerminal() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// IsStderrTerminal checks if stderr is a terminal.
func IsStderrTerminal() bool {
	return term.IsTerminal(int(os.Stderr.Fd()))
}
