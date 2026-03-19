package output

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/schollz/progressbar/v3"
)

// TextOutput implements human-readable text output with colors and progress bars.
type TextOutput struct {
	bar *progressbar.ProgressBar
}

func NewTextOutput() *TextOutput {
	return &TextOutput{}
}

func (t *TextOutput) Info(msg string) {
	fmt.Fprintln(os.Stderr, msg)
}

func (t *TextOutput) Infof(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func (t *TextOutput) Warn(msg string) {
	c := color.New(color.FgYellow)
	c.Fprintf(os.Stderr, "Warning: %s\n", msg)
}

func (t *TextOutput) Warnf(format string, args ...interface{}) {
	t.Warn(fmt.Sprintf(format, args...))
}

func (t *TextOutput) Error(err error, suggestion string) {
	c := color.New(color.FgRed)
	c.Fprintf(os.Stderr, "Error: %s\n", err.Error())
	if suggestion != "" {
		fmt.Fprintf(os.Stderr, "\n%s\n", suggestion)
	}
}

func (t *TextOutput) Result(v interface{}) {
	switch val := v.(type) {
	case string:
		fmt.Println(val)
	case fmt.Stringer:
		fmt.Println(val.String())
	default:
		// For structs, try a simple key-value display
		data, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			fmt.Println(v)
		} else {
			fmt.Println(string(data))
		}
	}
}

func (t *TextOutput) StartProgress(stage string, total int64) {
	if !IsStderrTerminal() {
		fmt.Fprintf(os.Stderr, "%s...\n", stage)
		return
	}

	t.bar = progressbar.NewOptions64(total,
		progressbar.OptionSetDescription(stage+"..."),
		progressbar.OptionSetWriter(os.Stderr),
		progressbar.OptionShowBytes(total > 1024),
		progressbar.OptionSetWidth(20),
		progressbar.OptionThrottle(100),
		progressbar.OptionShowCount(),
		progressbar.OptionClearOnFinish(),
		progressbar.OptionSetPredictTime(true),
		progressbar.OptionFullWidth(),
		progressbar.OptionSetRenderBlankState(true),
	)
}

func (t *TextOutput) UpdateProgress(current int64) {
	if t.bar != nil {
		t.bar.Set64(current)
	}
}

func (t *TextOutput) UpdateProgressMessage(msg string) {
	if t.bar != nil {
		t.bar.Describe(msg)
	}
}

func (t *TextOutput) EndProgress() {
	if t.bar != nil {
		t.bar.Finish()
		t.bar = nil
	}
}

func (t *TextOutput) IsTerminal() bool {
	return IsStdoutTerminal()
}

// QuietOutput suppresses all output except errors.
type QuietOutput struct{}

func NewQuietOutput() *QuietOutput {
	return &QuietOutput{}
}

func (q *QuietOutput) Info(msg string)                          {}
func (q *QuietOutput) Infof(format string, args ...interface{}) {}
func (q *QuietOutput) Warn(msg string)                          {}
func (q *QuietOutput) Warnf(format string, args ...interface{}) {}
func (q *QuietOutput) Error(err error, suggestion string) {
	fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
}
func (q *QuietOutput) Result(v interface{})             {}
func (q *QuietOutput) StartProgress(stage string, total int64) {}
func (q *QuietOutput) UpdateProgress(current int64)     {}
func (q *QuietOutput) UpdateProgressMessage(msg string) {}
func (q *QuietOutput) EndProgress()                     {}
func (q *QuietOutput) IsTerminal() bool                 { return IsStdoutTerminal() }
