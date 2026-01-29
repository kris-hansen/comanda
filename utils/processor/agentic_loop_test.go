package processor

import (
	"testing"
	"time"
)

func TestCheckExitCondition_LLMDecides(t *testing.T) {
	p := &Processor{
		variables: make(map[string]string),
	}

	tests := []struct {
		name       string
		output     string
		shouldExit bool
	}{
		{
			name:       "DONE exits",
			output:     "DONE",
			shouldExit: true,
		},
		{
			name:       "done lowercase exits",
			output:     "done",
			shouldExit: true,
		},
		{
			name:       "COMPLETE exits",
			output:     "COMPLETE",
			shouldExit: true,
		},
		{
			name:       "FINISHED exits",
			output:     "FINISHED",
			shouldExit: true,
		},
		{
			name:       "TASK_COMPLETE exits",
			output:     "TASK_COMPLETE",
			shouldExit: true,
		},
		{
			name:       "TASK COMPLETE exits",
			output:     "TASK COMPLETE",
			shouldExit: true,
		},
		{
			name:       "regular output continues",
			output:     "Here is the next step of the analysis",
			shouldExit: false,
		},
		{
			name:       "DONE with whitespace exits",
			output:     "  DONE  ",
			shouldExit: true,
		},
	}

	config := &AgenticLoopConfig{
		ExitCondition: "llm_decides",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldExit, _ := p.checkExitCondition(config, tt.output)
			if shouldExit != tt.shouldExit {
				t.Errorf("checkExitCondition() = %v, want %v", shouldExit, tt.shouldExit)
			}
		})
	}
}

func TestCheckExitCondition_PatternMatch(t *testing.T) {
	p := &Processor{
		variables: make(map[string]string),
	}

	tests := []struct {
		name        string
		exitPattern string
		output      string
		shouldExit  bool
	}{
		{
			name:        "pattern matches",
			exitPattern: "SATISFIED",
			output:      "I am SATISFIED with this result",
			shouldExit:  true,
		},
		{
			name:        "pattern not found",
			exitPattern: "SATISFIED",
			output:      "Still working on it",
			shouldExit:  false,
		},
		{
			name:        "regex pattern matches",
			exitPattern: `CODE_REVIEW_\d+_DONE`,
			output:      "CODE_REVIEW_123_DONE",
			shouldExit:  true,
		},
		{
			name:        "empty pattern never matches",
			exitPattern: "",
			output:      "anything",
			shouldExit:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &AgenticLoopConfig{
				ExitCondition: "pattern_match",
				ExitPattern:   tt.exitPattern,
			}
			shouldExit, _ := p.checkExitCondition(config, tt.output)
			if shouldExit != tt.shouldExit {
				t.Errorf("checkExitCondition() = %v, want %v", shouldExit, tt.shouldExit)
			}
		})
	}
}

func TestBuildIterationContext(t *testing.T) {
	p := &Processor{
		variables: make(map[string]string),
	}

	tests := []struct {
		name          string
		history       []LoopIteration
		contextWindow int
		previousOut   string
		wantContains  []string
	}{
		{
			name:          "empty history",
			history:       []LoopIteration{},
			contextWindow: 5,
			previousOut:   "initial input",
			wantContains:  []string{"initial input"},
		},
		{
			name: "single iteration in history",
			history: []LoopIteration{
				{Index: 1, Output: "first output"},
			},
			contextWindow: 5,
			previousOut:   "current",
			wantContains:  []string{"Iteration 1", "first output", "current"},
		},
		{
			name: "context window limits history",
			history: []LoopIteration{
				{Index: 1, Output: "first"},
				{Index: 2, Output: "second"},
				{Index: 3, Output: "third"},
				{Index: 4, Output: "fourth"},
			},
			contextWindow: 2,
			previousOut:   "current",
			wantContains:  []string{"third", "fourth", "current"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loopCtx := &LoopContext{
				History:        tt.history,
				PreviousOutput: tt.previousOut,
			}
			result := p.buildIterationContext(loopCtx, tt.contextWindow)
			for _, want := range tt.wantContains {
				if !contains(result, want) {
					t.Errorf("buildIterationContext() missing expected content %q", want)
				}
			}
		})
	}
}

func TestSetLoopVariables(t *testing.T) {
	p := &Processor{
		variables: make(map[string]string),
	}

	loopCtx := &LoopContext{
		Iteration:      3,
		PreviousOutput: "test output",
		StartTime:      time.Now().Add(-10 * time.Second),
	}

	p.setLoopVariables(loopCtx, 10)

	if p.variables["loop.iteration"] != "3" {
		t.Errorf("loop.iteration = %q, want %q", p.variables["loop.iteration"], "3")
	}

	if p.variables["loop.previous_output"] != "test output" {
		t.Errorf("loop.previous_output = %q, want %q", p.variables["loop.previous_output"], "test output")
	}

	if p.variables["loop.total_iterations"] != "10" {
		t.Errorf("loop.total_iterations = %q, want %q", p.variables["loop.total_iterations"], "10")
	}

	// Elapsed seconds should be approximately 10
	elapsed := p.variables["loop.elapsed_seconds"]
	if elapsed == "" {
		t.Error("loop.elapsed_seconds is empty")
	}
}

func TestAgenticLoopConfigDefaults(t *testing.T) {
	// Test that defaults are correctly applied
	if DefaultMaxIterations != 10 {
		t.Errorf("DefaultMaxIterations = %d, want 10", DefaultMaxIterations)
	}

	// Default timeout is 0 (no timeout, rely on max_iterations instead)
	if DefaultTimeoutSeconds != 0 {
		t.Errorf("DefaultTimeoutSeconds = %d, want 0", DefaultTimeoutSeconds)
	}

	if DefaultContextWindow != 5 {
		t.Errorf("DefaultContextWindow = %d, want 5", DefaultContextWindow)
	}

	// Default checkpoint interval for stateful loops
	if DefaultCheckpointInterval != 5 {
		t.Errorf("DefaultCheckpointInterval = %d, want 5", DefaultCheckpointInterval)
	}
}

func TestExecuteLoopSteps_NoSteps(t *testing.T) {
	p := &Processor{
		variables: make(map[string]string),
	}

	_, err := p.executeLoopSteps([]Step{}, "input")
	if err == nil {
		t.Error("executeLoopSteps() should return error for empty steps")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
