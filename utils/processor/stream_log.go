package processor

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/kris-hansen/comanda/utils/tui"
)

// StreamLogger handles real-time logging to a file for monitoring long-running operations
type StreamLogger struct {
	file     *os.File
	mu       sync.Mutex
	enabled  bool
	callback func(line string) // optional sink invoked with each formatted log line
}

// NewStreamLogger creates a new stream logger that writes to the specified file
func NewStreamLogger(path string) (*StreamLogger, error) {
	if path == "" {
		return &StreamLogger{enabled: false}, nil
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open stream log file: %w", err)
	}

	return &StreamLogger{
		file:    file,
		enabled: true,
	}, nil
}

// NewCallbackStreamLogger creates a stream logger that delivers each formatted
// line to a callback instead of (or in addition to) a file. Lines use the same
// format as the file-based log, so existing tail/parse tooling works unchanged.
func NewCallbackStreamLogger(cb func(line string)) *StreamLogger {
	return &StreamLogger{
		enabled:  cb != nil,
		callback: cb,
	}
}

// SetCallback attaches a line callback to an existing logger
func (s *StreamLogger) SetCallback(cb func(line string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callback = cb
	if cb != nil {
		s.enabled = true
	}
}

// Close closes the stream log file
func (s *StreamLogger) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.callback = nil
	if s.file != nil {
		return s.file.Close()
	}
	return nil
}

// IsEnabled returns whether stream logging is enabled
func (s *StreamLogger) IsEnabled() bool {
	return s.enabled
}

// Log writes a message to the stream log with timestamp
func (s *StreamLogger) Log(format string, args ...interface{}) {
	if !s.enabled {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.file == nil && s.callback == nil {
		return
	}

	timestamp := time.Now().Format("15:04:05")
	message := fmt.Sprintf(format, args...)
	line := fmt.Sprintf("[%s] %s", timestamp, message)
	if s.file != nil {
		fmt.Fprintf(s.file, "%s\n", line)
		_ = s.file.Sync() // Flush immediately for tail -f
	}
	if s.callback != nil {
		s.callback(line)
	}
}

// LogSection writes a section header to the stream log
func (s *StreamLogger) LogSection(title string) {
	if !s.enabled {
		return
	}
	separator := tui.DoubleSeparator(64)
	s.Log("%s", separator)
	s.Log("  %s", title)
	s.Log("%s", separator)
}

// LogIteration writes iteration start info
func (s *StreamLogger) LogIteration(current, total int, loopName string) {
	if !s.enabled {
		return
	}
	s.Log("")
	s.Log("───────────────────────────────────────────────────────────────")
	s.Log("  ITERATION %d/%d - %s", current, total, loopName)
	s.Log("───────────────────────────────────────────────────────────────")
}

// LogOutput writes the output from an iteration (truncated if too long)
func (s *StreamLogger) LogOutput(output string, maxLines int) {
	if !s.enabled {
		return
	}

	lines := splitLines(output)
	if len(lines) > maxLines {
		s.Log("OUTPUT (%d lines, showing first %d):", len(lines), maxLines)
		for i := 0; i < maxLines; i++ {
			s.Log("  %s", lines[i])
		}
		s.Log("  ... (%d more lines)", len(lines)-maxLines)
	} else {
		s.Log("OUTPUT:")
		for _, line := range lines {
			s.Log("  %s", line)
		}
	}
}

// LogThinking writes model thinking/reasoning content
func (s *StreamLogger) LogThinking(thinking string) {
	if !s.enabled || thinking == "" {
		return
	}
	s.Log("THINKING:")
	lines := splitLines(thinking)
	for _, line := range lines {
		s.Log("  │ %s", line)
	}
}

// LogExit writes exit reason
func (s *StreamLogger) LogExit(reason string) {
	if !s.enabled {
		return
	}
	s.Log("")
	s.Log("★ EXIT: %s", reason)
}

// LogError writes an error message
func (s *StreamLogger) LogError(err error) {
	if !s.enabled {
		return
	}
	s.Log("✖ ERROR: %v", err)
}

// LogContextUsage writes context usage info
func (s *StreamLogger) LogContextUsage(usedTokens, thresholdTokens, windowTokens int, percentage float64) {
	if !s.enabled {
		return
	}
	bar := s.contextBar(percentage)
	s.Log("CONTEXT: %s %.1f%% (%dk/%dk tokens, %dk window)",
		bar, percentage, usedTokens/1000, thresholdTokens/1000, windowTokens/1000)
}

// contextBar creates a visual progress bar for context usage
func (s *StreamLogger) contextBar(percentage float64) string {
	width := 20
	filled := int(percentage / 100 * float64(width))
	if filled > width {
		filled = width
	}
	bar := ""
	for i := 0; i < width; i++ {
		if i < filled {
			if percentage >= 90 {
				bar += "█" // Full block for danger zone
			} else if percentage >= 70 {
				bar += "▓" // Dark shade for warning
			} else {
				bar += "▒" // Medium shade for normal
			}
		} else {
			bar += "░"
		}
	}
	return "[" + bar + "]"
}

// splitLines splits a string into lines
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
