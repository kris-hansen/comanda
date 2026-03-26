package tui

import (
	"io"
	"strings"
	"sync"
)

// DebugWriter captures debug output and forwards it to a channel for TUI display.
// It implements io.Writer so it can be used with log.SetOutput().
type DebugWriter struct {
	mu       sync.Mutex
	lines    []string
	maxLines int
	onChange func([]string)
	original io.Writer
}

// NewDebugWriter creates a new DebugWriter that captures output.
// original is the fallback writer (e.g., os.Stderr) for when TUI is not active.
// maxLines limits how many lines are kept in the buffer.
func NewDebugWriter(original io.Writer, maxLines int) *DebugWriter {
	if maxLines <= 0 {
		maxLines = 500
	}
	return &DebugWriter{
		lines:    make([]string, 0, maxLines),
		maxLines: maxLines,
		original: original,
	}
}

// SetOnChange sets a callback that's called whenever new lines are added.
func (w *DebugWriter) SetOnChange(fn func([]string)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onChange = fn
}

// Write implements io.Writer. It captures the output and optionally forwards to original.
func (w *DebugWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	text := string(p)
	newLines := strings.Split(strings.TrimRight(text, "\n"), "\n")

	for _, line := range newLines {
		if line != "" {
			w.lines = append(w.lines, line)
		}
	}

	// Trim to max lines
	if len(w.lines) > w.maxLines {
		w.lines = w.lines[len(w.lines)-w.maxLines:]
	}

	// Notify listener
	if w.onChange != nil {
		// Send a copy to avoid race conditions
		linesCopy := make([]string, len(w.lines))
		copy(linesCopy, w.lines)
		w.onChange(linesCopy)
	}

	return len(p), nil
}

// Lines returns a copy of all captured lines.
func (w *DebugWriter) Lines() []string {
	w.mu.Lock()
	defer w.mu.Unlock()

	linesCopy := make([]string, len(w.lines))
	copy(linesCopy, w.lines)
	return linesCopy
}

// LastN returns the last n lines.
func (w *DebugWriter) LastN(n int) []string {
	w.mu.Lock()
	defer w.mu.Unlock()

	if n >= len(w.lines) {
		linesCopy := make([]string, len(w.lines))
		copy(linesCopy, w.lines)
		return linesCopy
	}

	start := len(w.lines) - n
	linesCopy := make([]string, n)
	copy(linesCopy, w.lines[start:])
	return linesCopy
}

// Clear empties the buffer.
func (w *DebugWriter) Clear() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lines = w.lines[:0]
}

// Original returns the original writer for passthrough.
func (w *DebugWriter) Original() io.Writer {
	return w.original
}
