package content

import (
	"compress/flate"
	"io"
)

// newFlateWriter creates a flate writer with a specific compression level.
func newFlateWriter(w io.Writer, level int) (io.WriteCloser, error) {
	return flate.NewWriter(w, level)
}
