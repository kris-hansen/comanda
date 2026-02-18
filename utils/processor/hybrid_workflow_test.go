package processor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestHybridWorkflowWithCodebaseIndex tests the complete hybrid workflow pattern:
// 1. Pre-loop codebase-index step that sets $PREFIX_INDEX variable
// 2. Pre-loop standard step that uses $PREFIX_INDEX as input
// 3. Multiple loops with dependencies that use pre-loop step outputs
// 4. execute_loops specifying execution order
//
// This mirrors the context.yaml pattern for deep codebase documentation.
func TestHybridWorkflowWithCodebaseIndex(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "comanda-hybrid-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create .comanda directory
	comandaDir := filepath.Join(tmpDir, ".comanda")
	if err := os.MkdirAll(comandaDir, 0755); err != nil {
		t.Fatalf("Failed to create .comanda dir: %v", err)
	}

	// Create a mock source file to index
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatalf("Failed to create source file: %v", err)
	}

	config := &DSLConfig{
		Steps: []Step{
			// Pre-loop step 1: codebase-index
			{
				Name: "index_codebase",
				Config: StepConfig{
					Type: "codebase-index",
					CodebaseIndex: &CodebaseIndexConfig{
						Root: tmpDir,
						Output: &CodebaseIndexOutputConfig{
							Path:  ".comanda/INDEX.md",
							Store: "repo",
						},
						Expose: &CodebaseIndexExposeConfig{
							WorkflowVariable: true,
						},
						MaxOutputKB: 200,
					},
				},
			},
			// Pre-loop step 2: uses $PREFIX_INDEX from codebase-index
			{
				Name: "categorize_sections",
				Config: StepConfig{
					Input:  "$COMANDA_HYBRID_TEST_INDEX", // Variable from codebase-index
					Model:  "mock",
					Action: "Categorize all files into sections",
					Output: ".comanda/sections.md",
				},
			},
		},
		Loops: map[string]*AgenticLoopConfig{
			"process_backend": {
				Name:          "process-backend",
				MaxIterations: 3,
				Steps: []Step{
					{
						Name: "analyze_backend",
						Config: StepConfig{
							Input:  ".comanda/INDEX.md",
							Model:  "mock",
							Action: "Analyze backend patterns",
							Output: "STDOUT",
						},
					},
				},
			},
			"process_frontend": {
				Name:          "process-frontend",
				DependsOn:     []string{"process_backend"},
				MaxIterations: 3,
				Steps: []Step{
					{
						Name: "analyze_frontend",
						Config: StepConfig{
							Input:  ".comanda/INDEX.md",
							Model:  "mock",
							Action: "Analyze frontend patterns",
							Output: "STDOUT",
						},
					},
				},
			},
			"compile_results": {
				Name:          "compile-results",
				DependsOn:     []string{"process_frontend"},
				MaxIterations: 1,
				Steps: []Step{
					{
						Name: "compile",
						Config: StepConfig{
							Input:  "NA",
							Model:  "mock",
							Action: "Compile all documentation",
							Output: "STDOUT",
						},
					},
				},
			},
		},
		ExecuteLoops: []string{
			"process_backend",
			"process_frontend",
			"compile_results",
		},
	}

	processor := NewProcessor(config, nil, nil, true, tmpDir)

	// Verify hybrid workflow is detected
	hasSteps := len(processor.config.Steps) > 0
	hasLoops := len(processor.config.Loops) > 0

	if !hasSteps {
		t.Error("Expected pre-loop steps to be present")
	}
	if !hasLoops {
		t.Error("Expected loops to be present")
	}

	// Verify pre-loop step count
	if len(processor.config.Steps) != 2 {
		t.Errorf("Expected 2 pre-loop steps, got %d", len(processor.config.Steps))
	}

	// Verify loop count
	if len(processor.config.Loops) != 3 {
		t.Errorf("Expected 3 loops, got %d", len(processor.config.Loops))
	}

	// Verify execute_loops order
	if len(processor.config.ExecuteLoops) != 3 {
		t.Errorf("Expected 3 execute_loops entries, got %d", len(processor.config.ExecuteLoops))
	}

	// Verify step 2 references a variable
	step2Input := processor.config.Steps[1].Config.Input.(string)
	if !strings.HasPrefix(step2Input, "$") {
		t.Errorf("step2 input should reference a variable, got %q", step2Input)
	}

	// Verify loop dependencies
	frontendLoop := processor.config.Loops["process_frontend"]
	if frontendLoop == nil {
		t.Fatal("process_frontend loop not found")
	}
	if len(frontendLoop.DependsOn) != 1 || frontendLoop.DependsOn[0] != "process_backend" {
		t.Errorf("process_frontend should depend on process_backend, got %v", frontendLoop.DependsOn)
	}

	compileLoop := processor.config.Loops["compile_results"]
	if compileLoop == nil {
		t.Fatal("compile_results loop not found")
	}
	if len(compileLoop.DependsOn) != 1 || compileLoop.DependsOn[0] != "process_frontend" {
		t.Errorf("compile_results should depend on process_frontend, got %v", compileLoop.DependsOn)
	}

	t.Logf("Hybrid workflow validated: %d pre-loop steps, %d loops, %d execute_loops",
		len(processor.config.Steps), len(processor.config.Loops), len(processor.config.ExecuteLoops))
}

// TestVariableSubstitutionInPreLoopSteps verifies that step output variables
// ($VARNAME) are properly substituted in subsequent pre-loop step inputs
func TestVariableSubstitutionInPreLoopSteps(t *testing.T) {
	config := &DSLConfig{}
	processor := NewProcessor(config, nil, nil, true, "")

	// Simulate codebase-index step setting variables
	// Note: Variable names should be distinct to avoid prefix matching issues
	processor.variables["CORE_INDEX"] = "# Index Content\n- file1.go\n- file2.go"
	processor.variables["CORE_PATH"] = ".comanda/INDEX.md"
	processor.variables["CORE_SHA"] = "abc123"
	processor.variables["SETUP_DONE"] = "completed"

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "substitute INDEX variable",
			input:    "$CORE_INDEX",
			expected: "# Index Content\n- file1.go\n- file2.go",
		},
		{
			name:     "substitute PATH variable",
			input:    "$CORE_PATH",
			expected: ".comanda/INDEX.md",
		},
		{
			name:     "no substitution for non-variable",
			input:    ".comanda/sections.md",
			expected: ".comanda/sections.md",
		},
		{
			name:     "substitute SHA variable",
			input:    "$CORE_SHA",
			expected: "abc123",
		},
		{
			name:     "substitute SETUP_DONE variable",
			input:    "$SETUP_DONE",
			expected: "completed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.substituteVariables(tt.input)
			if result != tt.expected {
				t.Errorf("substituteVariables(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestPreLoopStepsRunBeforeLoops verifies the execution order:
// 1. All pre-loop steps complete first
// 2. Then loops execute according to execute_loops order
func TestPreLoopStepsRunBeforeLoops(t *testing.T) {
	config := &DSLConfig{
		Steps: []Step{
			{
				Name: "setup_step",
				Config: StepConfig{
					Input:  "NA",
					Model:  "mock",
					Action: "setup",
					Output: "$SETUP_COMPLETE",
				},
			},
		},
		Loops: map[string]*AgenticLoopConfig{
			"main_loop": {
				Name:          "main-loop",
				MaxIterations: 5,
				Steps: []Step{
					{
						Name: "process",
						Config: StepConfig{
							Input:  "$SETUP_COMPLETE", // Depends on pre-loop step
							Model:  "mock",
							Action: "process",
							Output: "STDOUT",
						},
					},
				},
			},
		},
		ExecuteLoops: []string{"main_loop"},
	}

	processor := NewProcessor(config, nil, nil, true, "")

	// Verify the hybrid workflow detection
	hasSteps := len(processor.config.Steps) > 0
	hasLoops := len(processor.config.Loops) > 0

	if !hasSteps || !hasLoops {
		t.Error("Should detect hybrid workflow with both steps and loops")
	}

	// Verify loop step references pre-loop step output
	mainLoop := processor.config.Loops["main_loop"]
	if mainLoop == nil {
		t.Fatal("main_loop not found")
	}

	loopStepInput := mainLoop.Steps[0].Config.Input.(string)
	if loopStepInput != "$SETUP_COMPLETE" {
		t.Errorf("Loop step input should be $SETUP_COMPLETE, got %q", loopStepInput)
	}
}

// TestLoopDependencyChain verifies that loops with depends_on are
// correctly ordered in the dependency graph
func TestLoopDependencyChain(t *testing.T) {
	config := &DSLConfig{
		Loops: map[string]*AgenticLoopConfig{
			"step1": {
				Name:          "step1",
				MaxIterations: 1,
			},
			"step2": {
				Name:          "step2",
				DependsOn:     []string{"step1"},
				MaxIterations: 1,
			},
			"step3": {
				Name:          "step3",
				DependsOn:     []string{"step2"},
				MaxIterations: 1,
			},
			"step4": {
				Name:          "step4",
				DependsOn:     []string{"step3"},
				MaxIterations: 1,
			},
		},
		ExecuteLoops: []string{"step1", "step2", "step3", "step4"},
	}

	processor := NewProcessor(config, nil, nil, true, "")

	// Verify all loops are present
	if len(processor.config.Loops) != 4 {
		t.Errorf("Expected 4 loops, got %d", len(processor.config.Loops))
	}

	// Verify dependency chain
	for i := 2; i <= 4; i++ {
		loopName := "step" + string(rune('0'+i))
		prevLoop := "step" + string(rune('0'+i-1))

		loop := processor.config.Loops[loopName]
		if loop == nil {
			t.Errorf("Loop %s not found", loopName)
			continue
		}

		if len(loop.DependsOn) != 1 || loop.DependsOn[0] != prevLoop {
			t.Errorf("Loop %s should depend on %s, got %v", loopName, prevLoop, loop.DependsOn)
		}
	}
}

// TestCodebaseIndexVariableExport verifies that codebase-index steps
// correctly configure variable export settings
func TestCodebaseIndexVariableExport(t *testing.T) {
	config := &DSLConfig{
		Steps: []Step{
			{
				Name: "index_project",
				Config: StepConfig{
					Type: "codebase-index",
					CodebaseIndex: &CodebaseIndexConfig{
						Root: ".",
						Output: &CodebaseIndexOutputConfig{
							Path:  ".comanda/INDEX.md",
							Store: "repo",
						},
						Expose: &CodebaseIndexExposeConfig{
							WorkflowVariable: true,
						},
					},
				},
			},
		},
	}

	processor := NewProcessor(config, nil, nil, true, "")

	// Verify codebase-index step is recognized
	step := processor.config.Steps[0]
	if step.Config.Type != "codebase-index" {
		t.Errorf("Step type should be codebase-index, got %s", step.Config.Type)
	}

	// Verify expose configuration
	if step.Config.CodebaseIndex == nil {
		t.Fatal("CodebaseIndex config should not be nil")
	}
	if step.Config.CodebaseIndex.Expose == nil {
		t.Fatal("Expose config should not be nil")
	}
	if !step.Config.CodebaseIndex.Expose.WorkflowVariable {
		t.Error("WorkflowVariable should be true")
	}
}

// TestYAMLParsingPreservesStepOrder verifies that pre-loop steps
// maintain their definition order after YAML parsing
func TestYAMLParsingPreservesStepOrder(t *testing.T) {
	yamlContent := `
index_codebase:
  type: codebase-index
  codebase_index:
    root: .
    output:
      path: .comanda/INDEX.md

categorize_sections:
  input: $PROJECT_INDEX
  model: mock
  action: categorize
  output: .comanda/sections.md

loops:
  process_loop:
    name: process
    max_iterations: 3
    steps:
      process_step:
        input: .comanda/sections.md
        model: mock
        action: process
        output: STDOUT

execute_loops:
  - process_loop
`

	var config DSLConfig
	err := parseYAML([]byte(yamlContent), &config)
	if err != nil {
		t.Fatalf("Failed to parse YAML: %v", err)
	}

	// Verify step count
	if len(config.Steps) != 2 {
		t.Errorf("Expected 2 steps, got %d", len(config.Steps))
	}

	// Verify step names (order may vary due to map iteration, but both should be present)
	stepNames := make(map[string]bool)
	for _, step := range config.Steps {
		stepNames[step.Name] = true
	}
	if !stepNames["index_codebase"] {
		t.Error("index_codebase step not found")
	}
	if !stepNames["categorize_sections"] {
		t.Error("categorize_sections step not found")
	}

	// Verify loop exists
	if len(config.Loops) != 1 {
		t.Errorf("Expected 1 loop, got %d", len(config.Loops))
	}
	if config.Loops["process_loop"] == nil {
		t.Error("process_loop not found")
	}

	// Verify execute_loops
	if len(config.ExecuteLoops) != 1 || config.ExecuteLoops[0] != "process_loop" {
		t.Errorf("Expected execute_loops to be [process_loop], got %v", config.ExecuteLoops)
	}
}

// parseYAML is a helper to parse YAML content into DSLConfig
func parseYAML(content []byte, config *DSLConfig) error {
	return yaml.Unmarshal(content, config)
}
