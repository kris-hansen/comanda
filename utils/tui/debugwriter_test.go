package tui

import (
	"bytes"
	"testing"
)

func TestDebugWriter_Write(t *testing.T) {
	original := &bytes.Buffer{}
	w := NewDebugWriter(original, 100)

	// Write some content
	n, err := w.Write([]byte("[DEBUG] test message\n"))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != 21 {
		t.Errorf("Expected 21 bytes written, got %d", n)
	}

	lines := w.Lines()
	if len(lines) != 1 {
		t.Errorf("Expected 1 line, got %d", len(lines))
	}
	if lines[0] != "[DEBUG] test message" {
		t.Errorf("Expected '[DEBUG] test message', got '%s'", lines[0])
	}
}

func TestDebugWriter_MaxLines(t *testing.T) {
	original := &bytes.Buffer{}
	w := NewDebugWriter(original, 5)

	// Write more lines than maxLines
	for i := 0; i < 10; i++ {
		w.Write([]byte("line\n"))
	}

	lines := w.Lines()
	if len(lines) != 5 {
		t.Errorf("Expected 5 lines (max), got %d", len(lines))
	}
}

func TestDebugWriter_LastN(t *testing.T) {
	original := &bytes.Buffer{}
	w := NewDebugWriter(original, 100)

	w.Write([]byte("line1\nline2\nline3\nline4\nline5\n"))

	last3 := w.LastN(3)
	if len(last3) != 3 {
		t.Errorf("Expected 3 lines, got %d", len(last3))
	}
	if last3[0] != "line3" || last3[1] != "line4" || last3[2] != "line5" {
		t.Errorf("Unexpected lines: %v", last3)
	}
}

func TestDebugWriter_OnChange(t *testing.T) {
	original := &bytes.Buffer{}
	w := NewDebugWriter(original, 100)

	var callbackCalled bool
	var receivedLines []string

	w.SetOnChange(func(lines []string) {
		callbackCalled = true
		receivedLines = lines
	})

	w.Write([]byte("test line\n"))

	if !callbackCalled {
		t.Error("OnChange callback was not called")
	}
	if len(receivedLines) != 1 {
		t.Errorf("Expected 1 line in callback, got %d", len(receivedLines))
	}
}

func TestDebugWriter_Clear(t *testing.T) {
	original := &bytes.Buffer{}
	w := NewDebugWriter(original, 100)

	w.Write([]byte("line1\nline2\n"))
	if len(w.Lines()) != 2 {
		t.Errorf("Expected 2 lines before clear")
	}

	w.Clear()
	if len(w.Lines()) != 0 {
		t.Errorf("Expected 0 lines after clear, got %d", len(w.Lines()))
	}
}
