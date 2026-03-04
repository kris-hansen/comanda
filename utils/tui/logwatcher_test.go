package tui

import (
	"os"
	"testing"
	"time"
)

func TestNewLogWatcher(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	watcher := NewLogWatcher("/tmp/test.log", reporter)
	if watcher == nil {
		t.Fatal("NewLogWatcher returned nil")
	}
	if watcher.path != "/tmp/test.log" {
		t.Errorf("Expected path '/tmp/test.log', got %q", watcher.path)
	}
}

func TestLogWatcherParseLine(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	ch := reporter.Subscribe()
	watcher := NewLogWatcher("", reporter)

	tests := []struct {
		name     string
		line     string
		wantType string
	}{
		{
			name:     "iteration",
			line:     "[12:34:56] ITERATION 3/10 - review-loop",
			wantType: "loop_iter",
		},
		{
			name:     "context usage",
			line:     "[12:34:56] CONTEXT: ████████░░ 75.5% (75k/100k tokens)",
			wantType: "tokens",
		},
		{
			name:     "tool call read",
			line:     "[12:34:56] → Read file.txt",
			wantType: "tool_call",
		},
		{
			name:     "tool call write",
			line:     "[12:34:56] → Write output.txt",
			wantType: "tool_call",
		},
		{
			name:     "step start",
			line:     "[12:34:56] Starting step: analyze",
			wantType: "step_start",
		},
		{
			name:     "step complete",
			line:     "[12:34:56] Completed step: analyze",
			wantType: "step_end",
		},
		{
			name:     "error",
			line:     "[12:34:56] Error: connection timeout",
			wantType: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear any pending events
			select {
			case <-ch:
			default:
			}

			watcher.parseLine(tt.line)

			select {
			case event := <-ch:
				if event.Type != tt.wantType {
					t.Errorf("parseLine(%q) emitted type %q, want %q", tt.line, event.Type, tt.wantType)
				}
			case <-time.After(100 * time.Millisecond):
				t.Errorf("parseLine(%q) did not emit expected event type %q", tt.line, tt.wantType)
			}
		})
	}
}

func TestLogWatcherParseLineIgnoresDecorative(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	ch := reporter.Subscribe()
	watcher := NewLogWatcher("", reporter)

	// These lines should be ignored
	lines := []string{
		"",
		"───────────────────────────────────────────────────────────────",
		"═══════════════════════════════════════════════════════════════",
		"  indented text",
	}

	for _, line := range lines {
		watcher.parseLine(line)

		select {
		case event := <-ch:
			// output events are OK for non-decorative lines
			if line != "" && event.Type != "output" {
				t.Errorf("parseLine(%q) should not emit event, got %q", line, event.Type)
			}
		case <-time.After(10 * time.Millisecond):
			// No event is expected - this is good
		}
	}
}

func TestLogWatcherStartStop(t *testing.T) {
	// Create a temp file
	tmpFile, err := os.CreateTemp("", "logwatcher-test-*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	reporter := NewProgressReporter()
	defer reporter.Close()

	watcher := NewLogWatcher(tmpFile.Name(), reporter)

	// Start should not block
	watcher.Start()

	// Give it a moment to start
	time.Sleep(50 * time.Millisecond)

	// Stop should complete
	done := make(chan struct{})
	go func() {
		watcher.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Good
	case <-time.After(2 * time.Second):
		t.Error("Stop timed out")
	}
}

func TestLogWatcherWatchesFile(t *testing.T) {
	// Create a temp file
	tmpFile, err := os.CreateTemp("", "logwatcher-test-*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	reporter := NewProgressReporter()
	defer reporter.Close()

	ch := reporter.Subscribe()
	watcher := NewLogWatcher(tmpFile.Name(), reporter)
	watcher.Start()
	defer watcher.Stop()

	// Write to the file
	time.Sleep(50 * time.Millisecond) // Let watcher start
	tmpFile.WriteString("[12:34:56] ITERATION 1/5 - test-loop\n")
	tmpFile.Sync()

	// Should receive the event
	select {
	case event := <-ch:
		if event.Type != "loop_iter" {
			t.Errorf("Expected type 'loop_iter', got %q", event.Type)
		}
		if event.LoopName != "test-loop" {
			t.Errorf("Expected loop name 'test-loop', got %q", event.LoopName)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Timeout waiting for event from file")
	}

	tmpFile.Close()
}

func TestLogWatcherIterationParsing(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	ch := reporter.Subscribe()
	watcher := NewLogWatcher("", reporter)

	watcher.parseLine("[12:34:56] ITERATION 7/15 - code-review-loop")

	select {
	case event := <-ch:
		if event.Iteration != 7 {
			t.Errorf("Expected iteration 7, got %d", event.Iteration)
		}
		if event.MaxIter != 15 {
			t.Errorf("Expected max iter 15, got %d", event.MaxIter)
		}
		if event.LoopName != "code-review-loop" {
			t.Errorf("Expected loop name 'code-review-loop', got %q", event.LoopName)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for event")
	}
}

func TestLogWatcherContextParsing(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	ch := reporter.Subscribe()
	watcher := NewLogWatcher("", reporter)

	watcher.parseLine("[12:34:56] CONTEXT: ████████░░ 80% (80k/100k tokens, 128k window)")

	select {
	case event := <-ch:
		if event.TokensUsed != 80000 {
			t.Errorf("Expected tokens used 80000, got %d", event.TokensUsed)
		}
		if event.TokensAvail != 100000 {
			t.Errorf("Expected tokens avail 100000, got %d", event.TokensAvail)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for event")
	}
}
