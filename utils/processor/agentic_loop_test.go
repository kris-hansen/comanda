package processor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kris-hansen/comanda/utils/models"
)

func TestCheckExitCondition_LLMDecides(t *testing.T) {
	p := &Processor{
		variables:    make(map[string]string),
		cliVariables: make(map[string]string),
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
		variables:    make(map[string]string),
		cliVariables: make(map[string]string),
	}

	loopCtx := &LoopContext{
		Iteration:      3,
		PreviousOutput: "test output",
		StartTime:      time.Now().Add(-10 * time.Second),
	}

	steps := []Step{{Name: "analyze", Config: StepConfig{Action: "initial prompt"}}}
	loopCtx.CurrentActions = map[string]string{"analyze": "refined prompt"}

	p.setLoopVariables(loopCtx, steps, 10)

	if p.variables["loop.iteration"] != "3" {
		t.Errorf("loop.iteration = %q, want %q", p.variables["loop.iteration"], "3")
	}

	if p.variables["loop.previous_output"] != "test output" {
		t.Errorf("loop.previous_output = %q, want %q", p.variables["loop.previous_output"], "test output")
	}

	if p.variables["loop.total_iterations"] != "10" {
		t.Errorf("loop.total_iterations = %q, want %q", p.variables["loop.total_iterations"], "10")
	}

	if p.variables["loop.current_prompt"] != "refined prompt" {
		t.Errorf("loop.current_prompt = %q, want %q", p.variables["loop.current_prompt"], "refined prompt")
	}

	if p.cliVariables["loop.iteration"] != "3" {
		t.Errorf("cli loop.iteration = %q, want %q", p.cliVariables["loop.iteration"], "3")
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

	loopCtx := &LoopContext{CurrentActions: make(map[string]string)}
	_, err := p.executeLoopSteps(loopCtx, &AgenticLoopConfig{}, []Step{}, "input")
	if err == nil {
		t.Error("executeLoopSteps() should return error for empty steps")
	}
}

func TestRefineLoopStepPrompt_UpdatesCurrentAction(t *testing.T) {
	mockProvider := &CustomMockProvider{
		MockProvider: *NewMockProvider("openai"),
		responses: map[string]string{
			"refining the prompt for the next iteration": "Improved prompt for iteration 2",
		},
	}
	if err := mockProvider.Configure("test-key"); err != nil {
		t.Fatalf("Configure() failed: %v", err)
	}

	originalDetect := models.DetectProvider
	models.DetectProvider = func(modelName string) models.Provider {
		return mockProvider
	}
	defer func() { models.DetectProvider = originalDetect }()

	p := &Processor{
		variables:    make(map[string]string),
		cliVariables: make(map[string]string),
		providers: map[string]models.Provider{
			"openai": mockProvider,
		},
	}

	loopCtx := &LoopContext{
		Iteration:      1,
		PreviousOutput: "initial result",
		CurrentActions: map[string]string{"improve": "Initial prompt"},
	}
	loopConfig := &AgenticLoopConfig{
		PromptImprovement: &PromptImprovementConfig{Enabled: true},
	}
	step := Step{
		Name: "improve",
		Config: StepConfig{
			Model:  "gpt-4o-mini",
			Action: "Initial prompt",
		},
	}

	if err := p.refineLoopStepPrompt(loopCtx, loopConfig, step, "improve", "latest output"); err != nil {
		t.Fatalf("refineLoopStepPrompt() failed: %v", err)
	}

	if got := loopCtx.CurrentActions["improve"]; got != "Improved prompt for iteration 2" {
		t.Fatalf("current action = %q, want %q", got, "Improved prompt for iteration 2")
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

func TestInferDefaultAllowedPaths(t *testing.T) {
	p := &Processor{}

	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	expectedCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	// Test with workflow file in a subdirectory. Defaults should still use cwd.
	workflowDir := filepath.Join(tmpDir, ".comanda")
	if err := os.MkdirAll(workflowDir, 0755); err != nil {
		t.Fatal(err)
	}
	workflowFile := filepath.Join(workflowDir, "test.yaml")
	if err := os.WriteFile(workflowFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	paths := p.inferDefaultAllowedPaths(workflowFile)
	if len(paths) != 1 {
		t.Errorf("Expected 1 path, got %d", len(paths))
	}
	if paths[0] != expectedCwd {
		t.Errorf("Expected %s, got %s", expectedCwd, paths[0])
	}

	// Test with empty workflow file
	paths = p.inferDefaultAllowedPaths("")
	if len(paths) != 1 {
		t.Errorf("Expected 1 path for cwd fallback, got %d", len(paths))
	}
	if paths[0] != expectedCwd {
		t.Errorf("Expected %s, got %s", expectedCwd, paths[0])
	}
}

func TestExpandWithCommonProjectDirs(t *testing.T) {
	p := &Processor{}

	// Create temp dir with some common subdirs
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "src"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "test"), 0755)
	os.MkdirAll(filepath.Join(tmpDir, "docs"), 0755)

	// Expand from base path
	result := p.expandWithCommonProjectDirs([]string{tmpDir})

	// Should have original + expanded paths
	if len(result) < 4 {
		t.Errorf("Expected at least 4 paths (base + 3 subdirs), got %d", len(result))
	}

	// Verify specific paths are present
	pathSet := make(map[string]bool)
	for _, p := range result {
		pathSet[p] = true
	}

	expectedPaths := []string{
		tmpDir,
		filepath.Join(tmpDir, "src"),
		filepath.Join(tmpDir, "test"),
		filepath.Join(tmpDir, "docs"),
	}

	for _, expected := range expectedPaths {
		if !pathSet[expected] {
			t.Errorf("Expected path %s not found in result", expected)
		}
	}
}

func TestFormatPathAccessError(t *testing.T) {
	allowedPaths := []string{"/home/user/project/src", "/home/user/project/docs"}
	failedPath := "/home/user/project/build/output.txt"

	errMsg := FormatPathAccessError(failedPath, allowedPaths)

	// Should contain the failed path
	if !strings.Contains(errMsg, failedPath) {
		t.Error("Error message should contain failed path")
	}

	// Should list allowed paths
	for _, p := range allowedPaths {
		if !strings.Contains(errMsg, p) {
			t.Errorf("Error message should contain allowed path: %s", p)
		}
	}

	// Should contain suggestion
	if !strings.Contains(errMsg, "Suggestion") {
		t.Error("Error message should contain a suggestion")
	}
}
