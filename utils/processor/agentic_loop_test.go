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
		// New test cases for end-of-output detection
		{
			name:       "DONE at end of sentence exits",
			output:     "There is nothing further to document. DONE",
			shouldExit: true,
		},
		{
			name:       "DONE at end with period exits",
			output:     "All tasks completed. DONE.",
			shouldExit: true,
		},
		{
			name:       "COMPLETE at end of output exits",
			output:     "All work has been finished. COMPLETE",
			shouldExit: true,
		},
		{
			name:       "FINISHED at end of output exits",
			output:     "The implementation is ready. FINISHED",
			shouldExit: true,
		},
		{
			name:       "DONE in middle of sentence does NOT exit",
			output:     "I am not DONE yet, there is more work",
			shouldExit: false,
		},
		{
			name:       "multiline with DONE at end of last line exits",
			output:     "Step 1 complete.\nStep 2 complete.\nAll work DONE",
			shouldExit: true,
		},
		{
			name:       "multiline with DONE in middle continues",
			output:     "I thought I was DONE but\nactually there's more to do",
			shouldExit: false,
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

func TestExpandAllowedPathsWithOutputDirs(t *testing.T) {
	p := &Processor{
		variables: make(map[string]string),
	}

	tests := []struct {
		name         string
		allowedPaths []string
		steps        []Step
		wantMinLen   int // Minimum expected length (since paths are absolute)
	}{
		{
			name:         "no steps - paths unchanged",
			allowedPaths: []string{"/tmp/src"},
			steps:        []Step{},
			wantMinLen:   1,
		},
		{
			name:         "step with file output adds directory",
			allowedPaths: []string{"/tmp/src"},
			steps: []Step{
				{
					Name: "test",
					Config: StepConfig{
						Output: "./docs/output.md",
					},
				},
			},
			wantMinLen: 2, // Original path + docs directory
		},
		{
			name:         "step with STDOUT output - no addition",
			allowedPaths: []string{"/tmp/src"},
			steps: []Step{
				{
					Name: "test",
					Config: StepConfig{
						Output: "STDOUT",
					},
				},
			},
			wantMinLen: 1,
		},
		{
			name:         "step with variable output - no addition",
			allowedPaths: []string{"/tmp/src"},
			steps: []Step{
				{
					Name: "test",
					Config: StepConfig{
						Output: "$RESULT",
					},
				},
			},
			wantMinLen: 1,
		},
		{
			name:         "multiple steps with different outputs",
			allowedPaths: []string{"/tmp/src"},
			steps: []Step{
				{
					Name: "step1",
					Config: StepConfig{
						Output: "./docs/file1.md",
					},
				},
				{
					Name: "step2",
					Config: StepConfig{
						Output: "./output/file2.txt",
					},
				},
			},
			wantMinLen: 3, // Original + docs + output
		},
		{
			name:         "deduplicates same directory",
			allowedPaths: []string{"/tmp/src"},
			steps: []Step{
				{
					Name: "step1",
					Config: StepConfig{
						Output: "./docs/file1.md",
					},
				},
				{
					Name: "step2",
					Config: StepConfig{
						Output: "./docs/file2.md",
					},
				},
			},
			wantMinLen: 2, // Original + docs (deduped)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.expandAllowedPathsWithOutputDirs(tt.allowedPaths, tt.steps)
			if len(result) < tt.wantMinLen {
				t.Errorf("expandAllowedPathsWithOutputDirs() returned %d paths, want at least %d", len(result), tt.wantMinLen)
			}
		})
	}
}

func TestExtractOutputPath(t *testing.T) {
	p := &Processor{
		variables: make(map[string]string),
	}

	tests := []struct {
		name   string
		output interface{}
		want   string
	}{
		{
			name:   "nil output",
			output: nil,
			want:   "",
		},
		{
			name:   "STDOUT",
			output: "STDOUT",
			want:   "",
		},
		{
			name:   "NA",
			output: "NA",
			want:   "",
		},
		{
			name:   "variable reference",
			output: "$RESULT",
			want:   "",
		},
		{
			name:   "relative path",
			output: "./docs/output.md",
			want:   "./docs/output.md",
		},
		{
			name:   "absolute path",
			output: "/tmp/output.md",
			want:   "/tmp/output.md",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.extractOutputPath(tt.output)
			// For file paths, we just check they're non-empty when expected
			if tt.want == "" && result != "" {
				t.Errorf("extractOutputPath() = %q, want empty", result)
			}
			if tt.want != "" && result == "" {
				t.Errorf("extractOutputPath() = empty, want non-empty")
			}
		})
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
