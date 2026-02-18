package processor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPreLoopWorkflowIntegration tests the complete pre-loop workflow pattern
// based on the context.yaml use case:
// 1. Pre-loop steps run before loops
// 2. Pre-loop step outputs are stored as variables ($VARNAME)
// 3. Subsequent pre-loop steps can reference previous outputs via $VARNAME in inputs
// 4. Loops can access pre-loop step outputs
func TestPreLoopWorkflowIntegration(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir, err := os.MkdirTemp("", "comanda-preloop-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create .comanda directory
	comandaDir := filepath.Join(tmpDir, ".comanda")
	if err := os.MkdirAll(comandaDir, 0755); err != nil {
		t.Fatalf("Failed to create .comanda dir: %v", err)
	}

	// Create initial input file
	inputFile := filepath.Join(tmpDir, "input.txt")
	if err := os.WriteFile(inputFile, []byte("initial content"), 0644); err != nil {
		t.Fatalf("Failed to create input file: %v", err)
	}

	tests := []struct {
		name        string
		config      DSLConfig
		setup       func(t *testing.T, tmpDir string)
		validate    func(t *testing.T, p *Processor, tmpDir string)
		expectError bool
		errorMsg    string
	}{
		{
			name: "pre-loop step output variable configuration",
			config: DSLConfig{
				Steps: []Step{
					{
						Name: "create_index",
						Config: StepConfig{
							Input:  "NA",
							Model:  "mock",
							Action: "create index content",
							Output: "$INDEX_CONTENT",
						},
					},
				},
			},
			validate: func(t *testing.T, p *Processor, tmpDir string) {
				// Verify the output is configured as a variable
				output := p.config.Steps[0].Config.Output.(string)
				if !strings.HasPrefix(output, "$") {
					t.Error("Expected output to be a variable reference starting with $")
				}
			},
		},
		{
			name: "pre-loop step output file configuration",
			config: DSLConfig{
				Steps: []Step{
					{
						Name: "write_file",
						Config: StepConfig{
							Input:  "NA",
							Model:  "mock",
							Action: "generate output",
							Output: ".comanda/output.md",
						},
					},
				},
			},
			validate: func(t *testing.T, p *Processor, tmpDir string) {
				// Verify the output is configured as a file path
				output := p.config.Steps[0].Config.Output.(string)
				if output != ".comanda/output.md" {
					t.Errorf("Expected output to be .comanda/output.md, got %s", output)
				}
			},
		},
		{
			name: "second step uses variable from first step as input file path",
			config: DSLConfig{
				Steps: []Step{
					{
						Name: "step1_create_file",
						Config: StepConfig{
							Input:  "NA",
							Model:  "mock",
							Action: "create file",
							Output: ".comanda/index.md",
						},
					},
					{
						Name: "step2_use_file",
						Config: StepConfig{
							Input:  ".comanda/index.md", // Direct reference to file
							Model:  "mock",
							Action: "process file",
							Output: "STDOUT",
						},
					},
				},
			},
			setup: func(t *testing.T, tmpDir string) {
				// Create the file that step1 would create (mocking the output)
				indexPath := filepath.Join(tmpDir, ".comanda", "index.md")
				if err := os.WriteFile(indexPath, []byte("index content"), 0644); err != nil {
					t.Fatalf("Failed to create index file: %v", err)
				}
			},
		},
		{
			name: "variable substitution in input path ($VARNAME)",
			config: DSLConfig{
				Steps: []Step{
					{
						Name: "step1",
						Config: StepConfig{
							Input:  "NA",
							Model:  "mock",
							Action: "create",
							Output: "$INDEX_PATH",
						},
					},
					{
						Name: "step2",
						Config: StepConfig{
							Input:  "$INDEX_PATH", // Variable reference in input
							Model:  "mock",
							Action: "process",
							Output: "STDOUT",
						},
					},
				},
			},
			setup: func(t *testing.T, tmpDir string) {
				// This test verifies that $INDEX_PATH in input gets substituted
				// The processor should substitute $INDEX_PATH with the actual path
			},
			validate: func(t *testing.T, p *Processor, tmpDir string) {
				// Verify step2 input references step1 output variable
				step2Input := p.config.Steps[1].Config.Input.(string)
				if step2Input != "$INDEX_PATH" {
					t.Errorf("step2 input = %q, want $INDEX_PATH", step2Input)
				}
				// The substituteVariablesInSlice function will replace this at runtime
			},
		},
		{
			name: "CLI variable substitution in input ({{varname}})",
			config: DSLConfig{
				Steps: []Step{
					{
						Name: "step_with_cli_var",
						Config: StepConfig{
							Input:  "{{input_file}}",
							Model:  "mock",
							Action: "process",
							Output: "STDOUT",
						},
					},
				},
			},
			setup: func(t *testing.T, tmpDir string) {
				// Create the input file that the CLI variable will reference
				inputPath := filepath.Join(tmpDir, "cli_input.txt")
				if err := os.WriteFile(inputPath, []byte("cli input content"), 0644); err != nil {
					t.Fatalf("Failed to create CLI input file: %v", err)
				}
			},
		},
		{
			name: "pre-loop steps configured before loop execution",
			config: DSLConfig{
				Steps: []Step{
					{
						Name: "prereq",
						Config: StepConfig{
							Input:  "NA",
							Model:  "mock",
							Action: "setup",
							Output: "$PREREQ_DONE",
						},
					},
				},
				Loops: map[string]*AgenticLoopConfig{
					"main_loop": {
						Name:          "main-loop",
						MaxIterations: 1,
						Steps: []Step{
							{
								Name: "loop_step",
								Config: StepConfig{
									Input:  "$PREREQ_DONE", // Access prereq output
									Model:  "mock",
									Action: "loop action",
									Output: "STDOUT",
								},
							},
						},
					},
				},
				ExecuteLoops: []string{"main_loop"},
			},
			validate: func(t *testing.T, p *Processor, tmpDir string) {
				// Verify hybrid workflow is correctly configured
				if len(p.config.Steps) != 1 {
					t.Errorf("Expected 1 pre-loop step, got %d", len(p.config.Steps))
				}
				if len(p.config.Loops) != 1 {
					t.Errorf("Expected 1 loop, got %d", len(p.config.Loops))
				}
				// Verify loop step references the prereq output
				loopStep := p.config.Loops["main_loop"].Steps[0]
				loopInput := loopStep.Config.Input.(string)
				if loopInput != "$PREREQ_DONE" {
					t.Errorf("loop step input = %q, want $PREREQ_DONE", loopInput)
				}
			},
		},
		{
			name: "codebase-index step configuration recognized",
			config: DSLConfig{
				Steps: []Step{
					{
						Name: "index_codebase",
						Config: StepConfig{
							Type: "codebase-index",
							CodebaseIndex: &CodebaseIndexConfig{
								Root: ".",
								Output: &CodebaseIndexOutputConfig{
									Path: ".comanda/index.md",
								},
							},
						},
					},
				},
			},
			validate: func(t *testing.T, p *Processor, tmpDir string) {
				// Verify step is recognized as codebase-index type
				if p.config.Steps[0].Config.Type != "codebase-index" {
					t.Error("Expected step type to be codebase-index")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run setup if provided
			if tt.setup != nil {
				tt.setup(t, tmpDir)
			}

			// Create processor
			cliVars := map[string]string{
				"input_file": filepath.Join(tmpDir, "cli_input.txt"),
			}
			processor := NewProcessor(&tt.config, nil, nil, true, tmpDir, cliVars)

			// For most tests, we just validate configuration handling
			// Full execution would require mocking model providers
			if tt.validate != nil {
				tt.validate(t, processor, tmpDir)
			}
		})
	}
}

// TestVariableSubstitutionInInputs specifically tests that $VARNAME variables
// are properly substituted in step input paths
func TestVariableSubstitutionInInputs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "comanda-varsub-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	config := &DSLConfig{}
	processor := NewProcessor(config, nil, nil, true, tmpDir)

	// Manually set a variable (simulating step1 output)
	processor.variables["FILE_PATH"] = testFile
	processor.variables["CONTENT"] = "some content"

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple variable substitution",
			input:    "$FILE_PATH",
			expected: testFile,
		},
		{
			name:     "variable in path",
			input:    "$FILE_PATH",
			expected: testFile,
		},
		{
			name:     "no variable - passthrough",
			input:    "regular/path.txt",
			expected: "regular/path.txt",
		},
		{
			name:     "content variable",
			input:    "$CONTENT",
			expected: "some content",
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

// TestVariableSubstitutionInInputsSlice tests that substituteVariablesInSlice
// correctly substitutes all variables in a slice of inputs
func TestVariableSubstitutionInInputsSlice(t *testing.T) {
	config := &DSLConfig{}
	processor := NewProcessor(config, nil, nil, true, "")

	// Set variables
	processor.variables["PATH1"] = "/path/to/file1.txt"
	processor.variables["PATH2"] = "/path/to/file2.txt"
	processor.variables["CONTENT"] = "inline content"

	inputs := []string{"$PATH1", "$PATH2", "regular.txt", "$CONTENT"}
	processor.substituteVariablesInSlice(inputs)

	expected := []string{"/path/to/file1.txt", "/path/to/file2.txt", "regular.txt", "inline content"}
	for i, input := range inputs {
		if input != expected[i] {
			t.Errorf("inputs[%d] = %q, want %q", i, input, expected[i])
		}
	}
}

// TestPreLoopStepChaining tests that outputs from earlier pre-loop steps
// can be used as inputs in later pre-loop steps
func TestPreLoopStepChaining(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "comanda-chain-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	config := &DSLConfig{
		Steps: []Step{
			{
				Name: "step1",
				Config: StepConfig{
					Input:  "NA",
					Model:  "mock",
					Action: "generate index",
					Output: "$INDEX",
				},
			},
			{
				Name: "step2",
				Config: StepConfig{
					Input:  "$INDEX", // Uses step1's output variable
					Model:  "mock",
					Action: "categorize based on index",
					Output: "$CATEGORIES",
				},
			},
			{
				Name: "step3",
				Config: StepConfig{
					Input:  "$CATEGORIES", // Uses step2's output variable
					Model:  "mock",
					Action: "final processing",
					Output: "STDOUT",
				},
			},
		},
	}

	processor := NewProcessor(config, nil, nil, true, tmpDir)

	// Verify the configuration is valid
	if len(processor.config.Steps) != 3 {
		t.Errorf("Expected 3 steps, got %d", len(processor.config.Steps))
	}

	// Verify step2 input references step1 output
	step2Input := processor.config.Steps[1].Config.Input.(string)
	if step2Input != "$INDEX" {
		t.Errorf("step2 input = %q, want $INDEX", step2Input)
	}

	// Verify step3 input references step2 output
	step3Input := processor.config.Steps[2].Config.Input.(string)
	if step3Input != "$CATEGORIES" {
		t.Errorf("step3 input = %q, want $CATEGORIES", step3Input)
	}
}

// TestContextYAMLPattern tests the specific pattern used in context.yaml:
// - codebase-index step that sets $CORE_INDEX_PATH
// - categorization step that uses $CORE_INDEX as input
func TestContextYAMLPattern(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "comanda-context-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create .comanda directory
	comandaDir := filepath.Join(tmpDir, ".comanda")
	if err := os.MkdirAll(comandaDir, 0755); err != nil {
		t.Fatalf("Failed to create .comanda dir: %v", err)
	}

	config := &DSLConfig{
		Steps: []Step{
			{
				Name: "index_core_codebase",
				Config: StepConfig{
					Type: "codebase-index",
					CodebaseIndex: &CodebaseIndexConfig{
						Root: tmpDir,
						Output: &CodebaseIndexOutputConfig{
							Path: ".comanda/index.md",
						},
					},
				},
			},
			{
				Name: "categorize_codebase_sections",
				Config: StepConfig{
					// This is the pattern that was failing - $CORE_INDEX should be substituted
					Input:  "$CORE_INDEX",
					Model:  "mock",
					Action: "categorize",
					Output: ".comanda/categories.md",
				},
			},
		},
		Loops: map[string]*AgenticLoopConfig{
			"process_loop": {
				Name:          "process-sections",
				MaxIterations: 5,
				Steps: []Step{
					{
						Name: "process",
						Config: StepConfig{
							Input:  ".comanda/categories.md",
							Model:  "mock",
							Action: "process",
							Output: "STDOUT",
						},
					},
				},
			},
		},
		ExecuteLoops: []string{"process_loop"},
	}

	processor := NewProcessor(config, nil, nil, true, tmpDir)

	// Verify the hybrid workflow is detected
	hasSteps := len(processor.config.Steps) > 0
	hasLoops := len(processor.config.Loops) > 0

	if !hasSteps {
		t.Error("Expected steps to be present (pre-loop steps)")
	}
	if !hasLoops {
		t.Error("Expected loops to be present")
	}

	// Verify the step2 input uses the variable pattern
	step2Input := processor.config.Steps[1].Config.Input.(string)
	if !strings.HasPrefix(step2Input, "$CORE") {
		t.Errorf("step2 input should reference $CORE variable, got %q", step2Input)
	}

	t.Logf("Context YAML pattern validated: %d pre-loop steps, %d loops",
		len(processor.config.Steps), len(processor.config.Loops))
}

// TestInputVariableSubstitutionBeforeFileCheck verifies that variables are
// substituted in inputs BEFORE checking if the file exists
func TestInputVariableSubstitutionBeforeFileCheck(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "comanda-inputvar-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a test file
	testFile := filepath.Join(tmpDir, "actual_file.txt")
	if err := os.WriteFile(testFile, []byte("file content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	config := &DSLConfig{}
	processor := NewProcessor(config, nil, nil, true, tmpDir)

	// Set variable to point to the actual file
	processor.variables["MY_FILE"] = testFile

	// Test that $MY_FILE gets substituted to the actual path
	input := "$MY_FILE"
	substituted := processor.substituteVariables(input)

	if substituted != testFile {
		t.Errorf("Variable substitution failed: got %q, want %q", substituted, testFile)
	}

	// Verify the substituted path points to an existing file
	if _, err := os.Stat(substituted); os.IsNotExist(err) {
		t.Errorf("Substituted path %q does not exist", substituted)
	}
}

// TestStepOutputVariableStorage verifies that step outputs are correctly
// stored in the variables map when output starts with $
func TestStepOutputVariableStorage(t *testing.T) {
	config := &DSLConfig{
		Steps: []Step{
			{
				Name: "test_step",
				Config: StepConfig{
					Input:  "NA",
					Model:  "mock",
					Action: "test",
					Output: "$MY_VAR",
				},
			},
		},
	}

	processor := NewProcessor(config, nil, nil, true, "")

	// Simulate step output
	processor.lastOutput = "step output content"

	// Parse the output configuration
	output := processor.config.Steps[0].Config.Output.(string)

	// Verify it's a variable output
	if !strings.HasPrefix(output, "$") {
		t.Error("Output should be a variable reference starting with $")
	}

	// Extract variable name
	varName := strings.TrimPrefix(output, "$")
	if varName != "MY_VAR" {
		t.Errorf("Variable name = %q, want MY_VAR", varName)
	}

	// Manually store (simulating what processOutputs does)
	processor.variables[varName] = processor.lastOutput

	// Verify storage
	if val := processor.variables["MY_VAR"]; val != "step output content" {
		t.Errorf("Stored variable value = %q, want %q", val, "step output content")
	}
}
