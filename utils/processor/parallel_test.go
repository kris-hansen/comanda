package processor

import (
	"testing"

	"github.com/kris-hansen/comanda/utils/models"
)

// TestParallelProcessing tests the parallel processing functionality
func TestParallelProcessing(t *testing.T) {
	// Store the original function
	originalDetectProvider := models.DetectProvider

	// Override with test version that uses the mock providers
	models.DetectProvider = func(modelName string) models.Provider {
		return NewMockProvider("openai")
	}

	// Restore the original function after the test
	defer func() {
		models.DetectProvider = originalDetectProvider
	}()

	// Create a DSLConfig with parallel steps
	config := DSLConfig{
		ParallelSteps: map[string][]Step{
			"parallel-process": {
				{
					Name: "step_one",
					Config: StepConfig{
						Input:  []string{"NA"},
						Model:  []string{"gpt-4o-mini"},
						Action: []string{"test action 1"},
						Output: []string{"STDOUT"},
					},
				},
				{
					Name: "step_two",
					Config: StepConfig{
						Input:  []string{"NA"},
						Model:  []string{"gpt-4o-mini"},
						Action: []string{"test action 2"},
						Output: []string{"STDOUT"},
					},
				},
			},
		},
		Steps: []Step{
			{
				Name: "step_three",
				Config: StepConfig{
					Input:  []string{"NA"},
					Model:  []string{"gpt-4o-mini"},
					Action: []string{"test action 3"},
					Output: []string{"STDOUT"},
				},
			},
		},
	}

	// Create a processor with the test config
	processor := NewProcessor(&config, createTestEnvConfig(), createTestServerConfig(), false, "")

	// Run the processor
	err := processor.Process()
	if err != nil {
		t.Errorf("Process() returned unexpected error: %v", err)
	}
}

// TestParallelDependencyValidation tests the dependency validation for parallel steps
func TestParallelDependencyValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      DSLConfig
		expectError bool
	}{
		{
			name: "valid parallel steps - no dependencies",
			config: DSLConfig{
				ParallelSteps: map[string][]Step{
					"parallel-process": {
						{
							Name: "step_one",
							Config: StepConfig{
								Input:  []string{"NA"},
								Model:  []string{"gpt-4o-mini"},
								Action: []string{"test action 1"},
								Output: []string{"output1.txt"},
							},
						},
						{
							Name: "step_two",
							Config: StepConfig{
								Input:  []string{"NA"},
								Model:  []string{"claude-3-5-haiku-latest"},
								Action: []string{"test action 2"},
								Output: []string{"output2.txt"},
							},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "invalid parallel steps - dependency between parallel steps",
			config: DSLConfig{
				ParallelSteps: map[string][]Step{
					"parallel-process": {
						{
							Name: "step_one",
							Config: StepConfig{
								Input:  []string{"NA"},
								Model:  []string{"gpt-4o-mini"},
								Action: []string{"test action 1"},
								Output: []string{"output1.txt"},
							},
						},
						{
							Name: "step_two",
							Config: StepConfig{
								Input:  []string{"output1.txt"}, // Depends on output from step_one
								Model:  []string{"claude-3-5-haiku-latest"},
								Action: []string{"test action 2"},
								Output: []string{"output2.txt"},
							},
						},
					},
				},
			},
			expectError: true,
		},
		{
			name: "invalid parallel steps - same output file",
			config: DSLConfig{
				ParallelSteps: map[string][]Step{
					"parallel-process": {
						{
							Name: "step_one",
							Config: StepConfig{
								Input:  []string{"NA"},
								Model:  []string{"gpt-4o-mini"},
								Action: []string{"test action 1"},
								Output: []string{"same_output.txt"},
							},
						},
						{
							Name: "step_two",
							Config: StepConfig{
								Input:  []string{"NA"},
								Model:  []string{"claude-3-5-haiku-latest"},
								Action: []string{"test action 2"},
								Output: []string{"same_output.txt"}, // Same output file as step_one
							},
						},
					},
				},
			},
			expectError: true,
		},
		{
			name: "valid sequential step depending on parallel step output",
			config: DSLConfig{
				ParallelSteps: map[string][]Step{
					"parallel-process": {
						{
							Name: "step_one",
							Config: StepConfig{
								Input:  []string{"NA"},
								Model:  []string{"gpt-4o-mini"},
								Action: []string{"test action 1"},
								Output: []string{"output1.txt"},
							},
						},
					},
				},
				Steps: []Step{
					{
						Name: "step_two",
						Config: StepConfig{
							Input:  []string{"output1.txt"}, // Depends on output from parallel step
							Model:  []string{"gpt-4o"},
							Action: []string{"test action 2"},
							Output: []string{"output2.txt"},
						},
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := NewProcessor(&tt.config, createTestEnvConfig(), createTestServerConfig(), false, "")
			err := processor.validateDependencies()

			if tt.expectError && err == nil {
				t.Error("validateDependencies() expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("validateDependencies() unexpected error: %v", err)
			}
		})
	}
}

// Store the original validateModel function
var validateModel = func(p *Processor, modelNames []string, inputs []string) error {
	return p.validateModel(modelNames, inputs)
}

// TestParallelExecution tests that steps are actually executed in parallel
func TestParallelExecution(t *testing.T) {
	// Skip this test for now as it requires more complex setup
	t.Skip("Skipping parallel execution test")
}
