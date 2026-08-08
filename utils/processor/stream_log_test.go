package processor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStreamLogger(t *testing.T) {
	// Create temp file
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	// Create logger
	logger, err := NewStreamLogger(logPath)
	if err != nil {
		t.Fatalf("Failed to create stream logger: %v", err)
	}
	defer logger.Close()

	// Test logging
	logger.Log("Test message %d", 1)
	logger.LogSection("Test Section")
	logger.LogIteration(1, 10, "test-loop")
	logger.LogOutput("Line 1\nLine 2\nLine 3", 10)
	logger.LogExit("completed")

	// Close to flush
	logger.Close()

	// Read and verify
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logStr := string(content)

	// Check for expected content
	checks := []string{
		"Test message 1",
		"Test Section",
		"ITERATION 1/10",
		"test-loop",
		"Line 1",
		"Line 2",
		"EXIT: completed",
	}

	for _, check := range checks {
		if !strings.Contains(logStr, check) {
			t.Errorf("Log should contain '%s', got:\n%s", check, logStr)
		}
	}
}

func TestStreamLoggerDisabled(t *testing.T) {
	// Empty path should create disabled logger
	logger, err := NewStreamLogger("")
	if err != nil {
		t.Fatalf("Failed to create disabled logger: %v", err)
	}

	if logger.IsEnabled() {
		t.Error("Logger with empty path should be disabled")
	}

	// These should not panic
	logger.Log("test")
	logger.LogSection("test")
	logger.LogIteration(1, 10, "test")
	logger.Close()
}

func TestStreamLoggerCallback(t *testing.T) {
	var lines []string
	logger := NewCallbackStreamLogger(func(line string) {
		lines = append(lines, line)
	})

	if !logger.IsEnabled() {
		t.Fatal("Callback logger should be enabled")
	}

	logger.Log("Test message %d", 1)
	logger.LogIteration(3, 10, "build")
	logger.LogExit("completed")
	logger.Close()

	joined := strings.Join(lines, "\n")
	checks := []string{
		"Test message 1",
		"ITERATION 3/10 - build",
		"EXIT: completed",
	}
	for _, check := range checks {
		if !strings.Contains(joined, check) {
			t.Errorf("Callback lines should contain '%s', got:\n%s", check, joined)
		}
	}

	// Lines should carry the same [HH:MM:SS] prefix as the file format
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "[") {
		t.Errorf("Callback lines should be timestamp-prefixed, got %q", lines)
	}

	// After Close, the callback must no longer fire
	count := len(lines)
	logger.Log("after close")
	if len(lines) != count {
		t.Error("Callback should not fire after Close")
	}
}

func TestStreamLoggerFileAndCallback(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "both.log")

	logger, err := NewStreamLogger(logPath)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	var lines []string
	logger.SetCallback(func(line string) {
		lines = append(lines, line)
	})

	logger.Log("shared line")
	logger.Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}
	if !strings.Contains(string(content), "shared line") {
		t.Error("File should contain the logged line")
	}
	if len(lines) != 1 || !strings.Contains(lines[0], "shared line") {
		t.Errorf("Callback should receive the same line, got %q", lines)
	}
}

func TestStreamLoggerOutputTruncation(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "truncate.log")

	logger, err := NewStreamLogger(logPath)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	// Create output with many lines
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, "Line content here")
	}
	output := strings.Join(lines, "\n")

	// Log with max 5 lines
	logger.LogOutput(output, 5)
	logger.Close()

	content, _ := os.ReadFile(logPath)
	logStr := string(content)

	// Should mention truncation
	if !strings.Contains(logStr, "100 lines") {
		t.Errorf("Should mention total line count")
	}
	if !strings.Contains(logStr, "more lines") {
		t.Errorf("Should mention truncation")
	}
}
