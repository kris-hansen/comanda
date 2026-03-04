package tui

import (
	"testing"
	"time"
)

func TestNewProgressReporter(t *testing.T) {
	reporter := NewProgressReporter()
	if reporter == nil {
		t.Fatal("NewProgressReporter returned nil")
	}
	defer reporter.Close()

	if reporter.pid == 0 {
		t.Error("Reporter PID not set")
	}
}

func TestProgressReporterSubscribe(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	ch := reporter.Subscribe()
	if ch == nil {
		t.Fatal("Subscribe returned nil channel")
	}

	if len(reporter.subscribers) != 1 {
		t.Errorf("Expected 1 subscriber, got %d", len(reporter.subscribers))
	}
}

func TestProgressReporterEmit(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	ch := reporter.Subscribe()

	// Emit an event
	reporter.Emit(ProgressEvent{
		Type:    "test",
		Message: "test message",
	})

	// Should receive the event
	select {
	case event := <-ch:
		if event.Type != "test" {
			t.Errorf("Expected type 'test', got %q", event.Type)
		}
		if event.Message != "test message" {
			t.Errorf("Expected message 'test message', got %q", event.Message)
		}
		if event.Timestamp.IsZero() {
			t.Error("Timestamp not set")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for event")
	}
}

func TestProgressReporterStepStart(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	ch := reporter.Subscribe()

	reporter.StepStart("test-step")

	select {
	case event := <-ch:
		if event.Type != "step_start" {
			t.Errorf("Expected type 'step_start', got %q", event.Type)
		}
		if event.StepName != "test-step" {
			t.Errorf("Expected step name 'test-step', got %q", event.StepName)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for event")
	}
}

func TestProgressReporterStepEnd(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	ch := reporter.Subscribe()

	reporter.StepEnd("test-step", nil)

	select {
	case event := <-ch:
		if event.Type != "step_end" {
			t.Errorf("Expected type 'step_end', got %q", event.Type)
		}
		if event.StepName != "test-step" {
			t.Errorf("Expected step name 'test-step', got %q", event.StepName)
		}
		if event.Error != nil {
			t.Errorf("Expected nil error, got %v", event.Error)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for event")
	}
}

func TestProgressReporterLoopIteration(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	ch := reporter.Subscribe()

	reporter.LoopIteration("review-loop", 3, 10)

	select {
	case event := <-ch:
		if event.Type != "loop_iter" {
			t.Errorf("Expected type 'loop_iter', got %q", event.Type)
		}
		if event.LoopName != "review-loop" {
			t.Errorf("Expected loop name 'review-loop', got %q", event.LoopName)
		}
		if event.Iteration != 3 {
			t.Errorf("Expected iteration 3, got %d", event.Iteration)
		}
		if event.MaxIter != 10 {
			t.Errorf("Expected max iter 10, got %d", event.MaxIter)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for event")
	}
}

func TestProgressReporterTokenUpdate(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	ch := reporter.Subscribe()

	reporter.TokenUpdate(50000, 100000)

	select {
	case event := <-ch:
		if event.Type != "tokens" {
			t.Errorf("Expected type 'tokens', got %q", event.Type)
		}
		if event.TokensUsed != 50000 {
			t.Errorf("Expected tokens used 50000, got %d", event.TokensUsed)
		}
		if event.TokensAvail != 100000 {
			t.Errorf("Expected tokens avail 100000, got %d", event.TokensAvail)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for event")
	}
}

func TestProgressReporterComplete(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	ch := reporter.Subscribe()

	reporter.Complete(nil)

	select {
	case event := <-ch:
		if event.Type != "complete" {
			t.Errorf("Expected type 'complete', got %q", event.Type)
		}
		if event.Error != nil {
			t.Errorf("Expected nil error, got %v", event.Error)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for event")
	}
}

func TestProgressReporterUnsubscribe(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	ch := reporter.Subscribe()

	if len(reporter.subscribers) != 1 {
		t.Errorf("Expected 1 subscriber, got %d", len(reporter.subscribers))
	}

	reporter.Unsubscribe(ch)

	if len(reporter.subscribers) != 0 {
		t.Errorf("Expected 0 subscribers after unsubscribe, got %d", len(reporter.subscribers))
	}
}

func TestProgressReporterGetResourceUsage(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	cpu, mem := reporter.GetResourceUsage()

	// CPU might be 0 on first call (needs time to measure)
	// Memory should be > 0 for a running process
	if mem < 0 {
		t.Errorf("Memory usage should be >= 0, got %f", mem)
	}

	t.Logf("Resource usage - CPU: %.2f%%, Memory: %.2f MB", cpu, mem)
}

func TestProgressReporterMultipleSubscribers(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	ch1 := reporter.Subscribe()
	ch2 := reporter.Subscribe()

	if len(reporter.subscribers) != 2 {
		t.Errorf("Expected 2 subscribers, got %d", len(reporter.subscribers))
	}

	// Emit event
	reporter.ToolCall("test tool call")

	// Both subscribers should receive it
	for i, ch := range []chan ProgressEvent{ch1, ch2} {
		select {
		case event := <-ch:
			if event.Type != "tool_call" {
				t.Errorf("Subscriber %d: expected type 'tool_call', got %q", i, event.Type)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("Subscriber %d: timeout waiting for event", i)
		}
	}
}

func TestProgressReporterCollectProcessTree(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	// collectProcessTree should at least return the current process
	procs := reporter.collectProcessTree(reporter.proc)

	if len(procs) < 1 {
		t.Errorf("Expected at least 1 process in tree, got %d", len(procs))
	}

	// First process should be the one we passed in
	if procs[0].Pid != reporter.proc.Pid {
		t.Errorf("First process should be the root process")
	}

	t.Logf("Process tree contains %d process(es)", len(procs))
}

func TestProgressReporterGetResourceUsageWithChildren(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	// Get resource usage - should aggregate from process tree
	cpu, mem := reporter.GetResourceUsage()

	// Memory should be positive for any running process
	if mem <= 0 {
		t.Errorf("Expected positive memory usage, got %f MB", mem)
	}

	// CPU can be 0 on first call, that's okay
	t.Logf("Aggregated resource usage - CPU: %.2f%%, Memory: %.2f MB", cpu, mem)
}
