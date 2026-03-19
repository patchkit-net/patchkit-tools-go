package output

import (
	"encoding/json"
	"fmt"
	"os"
)

// JSONOutput implements JSON output mode.
// Informational messages go to stderr, results go to stdout.
type JSONOutput struct {
	progressEvents bool
	currentStage   string
}

func NewJSONOutput(progressEvents bool) *JSONOutput {
	return &JSONOutput{progressEvents: progressEvents}
}

func (j *JSONOutput) Info(msg string) {
	fmt.Fprintln(os.Stderr, msg)
}

func (j *JSONOutput) Infof(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func (j *JSONOutput) Warn(msg string) {
	fmt.Fprintf(os.Stderr, "Warning: %s\n", msg)
}

func (j *JSONOutput) Warnf(format string, args ...interface{}) {
	j.Warn(fmt.Sprintf(format, args...))
}

func (j *JSONOutput) Error(err error, suggestion string) {
	errObj := map[string]interface{}{
		"error":   errorCode(err),
		"message": err.Error(),
	}
	data, _ := json.Marshal(errObj)
	fmt.Fprintln(os.Stdout, string(data))
}

func (j *JSONOutput) Result(v interface{}) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling result: %v\n", err)
		return
	}
	fmt.Fprintln(os.Stdout, string(data))
}

func (j *JSONOutput) StartProgress(stage string, total int64) {
	j.currentStage = stage
	if j.progressEvents {
		event := map[string]interface{}{
			"type":  "stage_start",
			"stage": stage,
		}
		if total > 0 {
			event["total_bytes"] = total
		}
		data, _ := json.Marshal(event)
		fmt.Fprintln(os.Stderr, string(data))
	}
}

func (j *JSONOutput) UpdateProgress(current int64) {
	if j.progressEvents {
		event := map[string]interface{}{
			"type":  "progress",
			"stage": j.currentStage,
			"bytes": current,
		}
		data, _ := json.Marshal(event)
		fmt.Fprintln(os.Stderr, string(data))
	}
}

func (j *JSONOutput) UpdateProgressMessage(msg string) {
	// JSON mode doesn't show progress messages inline
}

func (j *JSONOutput) EndProgress() {
	if j.progressEvents {
		event := map[string]interface{}{
			"type":  "stage_complete",
			"stage": j.currentStage,
		}
		data, _ := json.Marshal(event)
		fmt.Fprintln(os.Stderr, string(data))
	}
	j.currentStage = ""
}

func (j *JSONOutput) IsTerminal() bool {
	return IsStdoutTerminal()
}

func errorCode(err error) string {
	// Map error types to error code strings
	switch err.(type) {
	default:
		return "general_error"
	}
}
