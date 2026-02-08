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
