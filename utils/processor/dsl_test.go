package processor

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/models"
	"gopkg.in/yaml.v3"
)

func createTestServerConfig() *config.ServerConfig {
	return &config.ServerConfig{
		Enabled: true,
	}
}

func TestNormalizeStringSlice(t *testing.T) {
	processor := NewProcessor(&DSLConfig{}, createTestEnvConfig(), createTestServerConfig(), false, "")

	tests := []struct {
		name     string
		input    interface{}
		expected []string
	}{
		{
			name:     "nil input",
			input:    nil,
			expected: []string{},
		},
		{
			name:     "single string",
			input:    "test",
			expected: []string{"test"},
		},
		{
			name:     "string slice",
			input:    []string{"test1", "test2"},
			expected: []string{"test1", "test2"},
		},
		{
			name:     "interface slice",
			input:    []interface{}{"test1", "test2"},
			expected: []string{"test1", "test2"},
		},
		{
			name:     "empty interface slice",
			input:    []interface{}{},
			expected: []string{},
		},
		{
			name:     "mixed type interface slice - only strings extracted",
			input:    []interface{}{"test1", 42, "test2"},
			expected: []string{"test1", "", "test2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.NormalizeStringSlice(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("NormalizeStringSlice() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNewProcessor(t *testing.T) {
	config := &DSLConfig{}
	envConfig := createTestEnvConfig()
	verbose := true

	processor := NewProcessor(config, envConfig, createTestServerConfig(), verbose, "")

	if processor == nil {
		t.Error("NewProcessor() returned nil")
	}

	if processor.config != config {
		t.Error("NewProcessor() did not set config correctly")
	}

	if processor.envConfig != envConfig {
		t.Error("NewProcessor() did not set envConfig correctly")
	}

	if processor.verbose != verbose {
		t.Error("NewProcessor() did not set verbose correctly")
	}

	if processor.handler == nil {
		t.Error("NewProcessor() did not initialize handler")
	}

	if processor.validator == nil {
		t.Error("NewProcessor() did not initialize validator")
	}

	if processor.providers == nil {
		t.Error("NewProcessor() did not initialize providers map")
	}
}

func TestSubstituteCLIVariables(t *testing.T) {
	cliVars := map[string]string{
		"filename":     "/path/to/file.txt",
		"project_name": "myproject",
		"output_dir":   "results",
	}
	processor := NewProcessor(&DSLConfig{}, createTestEnvConfig(), createTestServerConfig(), false, "", cliVars)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no variables",
			input:    "plain text",
			expected: "plain text",
		},
		{
			name:     "single variable without spaces",
			input:    "tool: grep -E 'error' {{filename}}",
			expected: "tool: grep -E 'error' /path/to/file.txt",
		},
		{
			name:     "single variable with spaces",
			input:    "tool: grep -E 'error' {{ filename }}",
			expected: "tool: grep -E 'error' /path/to/file.txt",
		},
		{
			name:     "multiple variables",
			input:    "Analyze {{project_name}} and save to {{output_dir}}/results.txt",
			expected: "Analyze myproject and save to results/results.txt",
		},
		{
			name:     "undefined variable stays unchanged",
			input:    "{{undefined_var}} should remain",
			expected: "{{undefined_var}} should remain",
		},
		{
			name:     "mixed variables",
			input:    "{{filename}} and {{ project_name }} and {{undefined}}",
			expected: "/path/to/file.txt and myproject and {{undefined}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.SubstituteCLIVariables(tt.input)
			if result != tt.expected {
				t.Errorf("SubstituteCLIVariables(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewProcessorWithCLIVariables(t *testing.T) {
	cliVars := map[string]string{
		"test_var": "test_value",
	}
	processor := NewProcessor(&DSLConfig{}, createTestEnvConfig(), createTestServerConfig(), false, "", cliVars)

	if processor.cliVariables == nil {
		t.Error("NewProcessor() did not initialize cliVariables")
	}

	if processor.cliVariables["test_var"] != "test_value" {
		t.Errorf("NewProcessor() did not set cliVariables correctly, got %v", processor.cliVariables)
	}
}

func TestNewProcessorWithoutCLIVariables(t *testing.T) {
	processor := NewProcessor(&DSLConfig{}, createTestEnvConfig(), createTestServerConfig(), false, "")

	if processor.cliVariables == nil {
		t.Error("NewProcessor() did not initialize cliVariables to empty map")
	}

	if len(processor.cliVariables) != 0 {
		t.Errorf("NewProcessor() should have empty cliVariables, got %v", processor.cliVariables)
	}
}

func TestValidateStepConfig(t *testing.T) {
	processor := NewProcessor(&DSLConfig{}, createTestEnvConfig(), createTestServerConfig(), false, "")

	tests := []struct {
		name          string
		stepName      string
		config        StepConfig
		expectedError string
	}{
		{
			name:     "valid config",
			stepName: "test_step",
			config: StepConfig{
				Input:  "test.txt",
				Model:  "gpt-4o-mini",
				Action: "analyze",
				Output: "STDOUT",
			},
			expectedError: "",
		},
		{
			name:     "missing input tag",
			stepName: "test_step",
			config: StepConfig{
				Model:  "gpt-4o-mini",
				Action: "analyze",
				Output: "STDOUT",
			},
			expectedError: "input tag is required",
		},
		{
			name:     "missing model",
			stepName: "test_step",
			config: StepConfig{
				Input:  "test.txt",
				Action: "analyze",
				Output: "STDOUT",
			},
			expectedError: "model is required",
		},
		{
			name:     "missing action",
			stepName: "test_step",
			config: StepConfig{
				Input:  "test.txt",
				Model:  "gpt-4o-mini",
				Output: "STDOUT",
			},
			expectedError: "action is required",
		},
		{
			name:     "missing output",
			stepName: "test_step",
			config: StepConfig{
				Input:  "test.txt",
				Model:  "gpt-4o-mini",
				Action: "analyze",
			},
			expectedError: "output is required",
		},
		{
			name:     "empty input allowed",
			stepName: "test_step",
			config: StepConfig{
				Input:  "",
				Model:  "gpt-4o-mini",
				Action: "analyze",
				Output: "STDOUT",
			},
			expectedError: "",
		},
		{
			name:     "NA input allowed",
			stepName: "test_step",
			config: StepConfig{
				Input:  "NA",
				Model:  "gpt-4o-mini",
				Action: "analyze",
				Output: "STDOUT",
			},
			expectedError: "",
		},
		{
			name:     "codebase-index step type does not require standard fields",
			stepName: "index_step",
			config: StepConfig{
				Type: "codebase-index",
				CodebaseIndex: &CodebaseIndexConfig{
					Root: ".",
				},
			},
			expectedError: "",
		},
		{
			name:     "codebase_index block does not require standard fields",
			stepName: "index_step",
			config: StepConfig{
				CodebaseIndex: &CodebaseIndexConfig{
					Root: "./my-project",
				},
			},
			expectedError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := processor.validateStepConfig(tt.stepName, tt.config)
			if tt.expectedError == "" {
				if err != nil {
					t.Errorf("validateStepConfig() returned unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Error("validateStepConfig() expected error but got none")
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("validateStepConfig() error = %v, want error containing %v", err, tt.expectedError)
				}
			}
		})
	}
}

func TestBuildCodebaseIndexConfigEncryptionKey(t *testing.T) {
	processor := NewProcessor(&DSLConfig{}, createTestEnvConfig(), createTestServerConfig(), false, "")

	// Test that encryption key is picked up from environment
	t.Run("encryption key from env var", func(t *testing.T) {
		// Set env var
		t.Setenv("COMANDA_INDEX_KEY", "test-secret-key")

		stepConfig := StepConfig{
			CodebaseIndex: &CodebaseIndexConfig{
				Root: ".",
				Output: &CodebaseIndexOutputConfig{
					Encrypt: true,
				},
			},
		}

		config := processor.buildCodebaseIndexConfig(stepConfig)

		if config.EncryptionKey != "test-secret-key" {
			t.Errorf("EncryptionKey = %q, want %q", config.EncryptionKey, "test-secret-key")
		}
	})

	// Test that encryption key is empty when encrypt is false
	t.Run("no encryption key when encrypt false", func(t *testing.T) {
		t.Setenv("COMANDA_INDEX_KEY", "test-secret-key")

		stepConfig := StepConfig{
			CodebaseIndex: &CodebaseIndexConfig{
				Root: ".",
				Output: &CodebaseIndexOutputConfig{
					Encrypt: false,
				},
			},
		}

		config := processor.buildCodebaseIndexConfig(stepConfig)

		if config.EncryptionKey != "" {
			t.Errorf("EncryptionKey should be empty when encrypt=false, got %q", config.EncryptionKey)
		}
	})

	// Test that encryption key is empty when env var not set and config not set
	t.Run("empty encryption key when neither set", func(t *testing.T) {
		// Ensure env var is not set
		t.Setenv("COMANDA_INDEX_KEY", "")

		stepConfig := StepConfig{
			CodebaseIndex: &CodebaseIndexConfig{
				Root: ".",
				Output: &CodebaseIndexOutputConfig{
					Encrypt: true,
				},
			},
		}

		config := processor.buildCodebaseIndexConfig(stepConfig)

		if config.EncryptionKey != "" {
			t.Errorf("EncryptionKey should be empty when neither set, got %q", config.EncryptionKey)
		}
	})

	// Test that encryption key falls back to config when env var not set
	t.Run("encryption key from config when env var not set", func(t *testing.T) {
		// Ensure env var is not set
		t.Setenv("COMANDA_INDEX_KEY", "")

		// Create a processor with config that has IndexEncryptionKey
		envCfg := createTestEnvConfig()
		envCfg.IndexEncryptionKey = "config-secret-key"
		procWithKey := NewProcessor(&DSLConfig{}, envCfg, createTestServerConfig(), false, "")

		stepConfig := StepConfig{
			CodebaseIndex: &CodebaseIndexConfig{
				Root: ".",
				Output: &CodebaseIndexOutputConfig{
					Encrypt: true,
				},
			},
		}

		config := procWithKey.buildCodebaseIndexConfig(stepConfig)

		if config.EncryptionKey != "config-secret-key" {
			t.Errorf("EncryptionKey = %q, want %q", config.EncryptionKey, "config-secret-key")
		}
	})

	// Test that env var takes precedence over config
	t.Run("env var takes precedence over config", func(t *testing.T) {
		t.Setenv("COMANDA_INDEX_KEY", "env-secret-key")

		// Create a processor with config that has IndexEncryptionKey
		envCfg := createTestEnvConfig()
		envCfg.IndexEncryptionKey = "config-secret-key"
		procWithKey := NewProcessor(&DSLConfig{}, envCfg, createTestServerConfig(), false, "")

		stepConfig := StepConfig{
			CodebaseIndex: &CodebaseIndexConfig{
				Root: ".",
				Output: &CodebaseIndexOutputConfig{
					Encrypt: true,
				},
			},
		}

		config := procWithKey.buildCodebaseIndexConfig(stepConfig)

		if config.EncryptionKey != "env-secret-key" {
			t.Errorf("EncryptionKey = %q, want %q (env var should take precedence)", config.EncryptionKey, "env-secret-key")
		}
	})
}

func TestProcess(t *testing.T) {
	tests := []struct {
		name        string
		config      DSLConfig
		expectError bool
	}{
		{
			name:        "empty config",
			config:      DSLConfig{},
			expectError: true,
		},
		{
			name: "single step with missing model",
			config: DSLConfig{
				Steps: []Step{
					{
						Name: "step_one",
						Config: StepConfig{
							Action: []string{"test action"},
							Output: []string{"STDOUT"},
						},
					},
				},
			},
			expectError: true,
		},
		{
			name: "valid single step",
			config: DSLConfig{
				Steps: []Step{
					{
						Name: "step_one",
						Config: StepConfig{
							Input:  []string{"NA"},
							Model:  []string{"gpt-4o-mini"},
							Action: []string{"test action"},
							Output: []string{"STDOUT"},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "input file exists as output in later step",
			config: DSLConfig{
				Steps: []Step{
					{
						Name: "step_one",
						Config: StepConfig{
							Input:  []string{"future_output.txt"},
							Model:  []string{"gpt-4o-mini"},
							Action: []string{"test action"},
							Output: []string{"STDOUT"},
						},
					},
					{
						Name: "step_two",
						Config: StepConfig{
							Input:  []string{"NA"},
							Model:  []string{"gpt-4o-mini"},
							Action: []string{"generate"},
							Output: []string{"future_output.txt"},
						},
					},
				},
			},
			expectError: false,
		},
		{
			name: "non-existent input file with no matching output",
			config: DSLConfig{
				Steps: []Step{
					{
						Name: "step_one",
						Config: StepConfig{
							Input:  []string{"nonexistent.txt"},
							Model:  []string{"gpt-4o-mini"},
							Action: []string{"test action"},
							Output: []string{"STDOUT"},
						},
					},
				},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := NewProcessor(&tt.config, createTestEnvConfig(), createTestServerConfig(), false, "")
			err := processor.Process()

			if tt.expectError && err == nil {
				t.Error("Process() expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Process() unexpected error: %v", err)
			}
		})
	}
}

func TestChunkingWithTemplateVariables(t *testing.T) {
	// Create a temporary test file with enough content to chunk
	tmpfile, err := os.CreateTemp("", "chunk-test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	// Write 100 lines to ensure chunking happens
	content := strings.Repeat("This is line content for testing chunking.\n", 100)
	if _, err := tmpfile.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	// Create a temporary output directory
	outputDir, err := os.MkdirTemp("", "chunk-output-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(outputDir)

	config := DSLConfig{
		Steps: []Step{
			{
				Name: "chunk_test",
				Config: StepConfig{
					Input: tmpfile.Name(),
					Chunk: &ChunkConfig{
						By:        "lines",
						Size:      30,
						Overlap:   5,
						MaxChunks: 10,
					},
					BatchMode: "individual",
					Model:     "gpt-4o-mini",
					Action:    "Summarize chunk {{ chunk_index }} of {{ total_chunks }}",
					Output:    filepath.Join(outputDir, "output_chunk_{{ chunk_index }}.txt"),
				},
			},
		},
	}

	processor := NewProcessor(&config, createTestEnvConfig(), createTestServerConfig(), true, "")

	// Mock the provider to return predictable output
	mockProvider := &MockProvider{}
	processor.providers["openai"] = mockProvider

	err = processor.Process()
	if err != nil {
		t.Fatalf("Process() failed: %v", err)
	}

	// Verify that multiple chunk files were created with proper names
	files, err := filepath.Glob(filepath.Join(outputDir, "output_chunk_*.txt"))
	if err != nil {
		t.Fatalf("Failed to glob output files: %v", err)
	}

	if len(files) < 2 {
		t.Errorf("Expected at least 2 chunk files, got %d. Files: %v", len(files), files)
	}

	// Verify that files have numeric indices, not template literals
	for _, file := range files {
		basename := filepath.Base(file)
		if strings.Contains(basename, "{{") || strings.Contains(basename, "}}") {
			t.Errorf("File still contains template literal: %s", basename)
		}

		// Verify the file matches the pattern output_chunk_N.txt where N is a number
		if !strings.HasPrefix(basename, "output_chunk_") || !strings.HasSuffix(basename, ".txt") {
			t.Errorf("File doesn't match expected pattern: %s", basename)
		}
	}

	// Verify specific files exist
	expectedFiles := []string{
		filepath.Join(outputDir, "output_chunk_0.txt"),
		filepath.Join(outputDir, "output_chunk_1.txt"),
		filepath.Join(outputDir, "output_chunk_2.txt"),
	}

	for _, expectedFile := range expectedFiles {
		if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
			t.Errorf("Expected file not found: %s", expectedFile)
		}
	}
}

func TestBatchModeIndividualWithTemplateVariables(t *testing.T) {
	// Create temporary test files to simulate wildcard expansion
	tmpDir, err := os.MkdirTemp("", "batch-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create multiple input files (simulating requirements_chunk_*.txt)
	for i := 0; i < 3; i++ {
		tmpfile, err := os.CreateTemp(tmpDir, fmt.Sprintf("requirements_chunk_%d-*.txt", i))
		if err != nil {
			t.Fatal(err)
		}
		content := fmt.Sprintf("Requirement %d: This is test content for file %d\n", i, i)
		if _, err := tmpfile.Write([]byte(content)); err != nil {
			tmpfile.Close()
			t.Fatal(err)
		}
		tmpfile.Close()
	}

	// Create a single additional file (simulating test_inventory.txt)
	inventoryFile, err := os.CreateTemp(tmpDir, "test_inventory-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := inventoryFile.Write([]byte("Test inventory content\n")); err != nil {
		inventoryFile.Close()
		t.Fatal(err)
	}
	inventoryFile.Close()

	// Create output directory
	outputDir, err := os.MkdirTemp("", "batch-output-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(outputDir)

	// Get all requirement files
	reqFiles, err := filepath.Glob(filepath.Join(tmpDir, "requirements_chunk_*"))
	if err != nil {
		t.Fatal(err)
	}

	// Build input list (all requirement files + inventory file)
	var inputs []interface{}
	for _, f := range reqFiles {
		inputs = append(inputs, f)
	}
	inputs = append(inputs, inventoryFile.Name())

	config := DSLConfig{
		Steps: []Step{
			{
				Name: "batch_test",
				Config: StepConfig{
					Input:     inputs,
					BatchMode: "individual", // Key: using batch_mode individual without chunk: block
					Model:     "gpt-4o-mini",
					Action:    "Process this file",
					Output:    filepath.Join(outputDir, "coverage_chunk_{{ chunk_index }}.txt"),
				},
			},
		},
	}

	processor := NewProcessor(&config, createTestEnvConfig(), createTestServerConfig(), true, "")

	// Mock the provider
	mockProvider := &MockProvider{}
	processor.providers["openai"] = mockProvider

	err = processor.Process()
	if err != nil {
		t.Fatalf("Process() failed: %v", err)
	}

	// Verify that multiple output files were created with proper names (not literal template)
	files, err := filepath.Glob(filepath.Join(outputDir, "coverage_chunk_*.txt"))
	if err != nil {
		t.Fatalf("Failed to glob output files: %v", err)
	}

	if len(files) != len(inputs) {
		t.Errorf("Expected %d output files (one per input), got %d. Files: %v", len(inputs), len(files), files)
	}

	// Verify that files have numeric indices, not template literals
	for _, file := range files {
		basename := filepath.Base(file)
		if strings.Contains(basename, "{{") || strings.Contains(basename, "}}") {
			t.Errorf("File still contains template literal: %s", basename)
		}

		// Verify the file matches the pattern coverage_chunk_N.txt where N is a number
		if !strings.HasPrefix(basename, "coverage_chunk_") || !strings.HasSuffix(basename, ".txt") {
			t.Errorf("File doesn't match expected pattern: %s", basename)
		}
	}

	// Verify specific files exist
	for i := 0; i < len(inputs); i++ {
		expectedFile := filepath.Join(outputDir, fmt.Sprintf("coverage_chunk_%d.txt", i))
		if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
			t.Errorf("Expected file not found: %s", expectedFile)
		}
	}

	// Verify that NO file exists with literal template syntax
	literalFile := filepath.Join(outputDir, "coverage_chunk_{{ chunk_index }}.txt")
	if _, err := os.Stat(literalFile); !os.IsNotExist(err) {
		t.Errorf("File with literal template syntax should NOT exist: %s", literalFile)
	}
}

func TestDebugf(t *testing.T) {
	tests := []struct {
		name    string
		verbose bool
	}{
		{
			name:    "verbose mode enabled",
			verbose: true,
		},
		{
			name:    "verbose mode disabled",
			verbose: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := NewProcessor(&DSLConfig{}, createTestEnvConfig(), createTestServerConfig(), tt.verbose, "")
			// Note: This test only verifies that debugf doesn't panic
			// In a real scenario, you might want to capture stdout and verify the output
			processor.debugf("test message %s", "arg")
		})
	}
}

// CustomMockProvider extends MockProvider with custom response handling
type CustomMockProvider struct {
	MockProvider
	responses map[string]string
}

func (m *CustomMockProvider) SendPrompt(model, prompt string) (string, error) {
	// First check if we have a custom response for this prompt
	for key, response := range m.responses {
		if strings.Contains(prompt, key) {
			return response, nil
		}
	}

	// Fall back to the parent implementation
	return "mock response", nil
}

// Override SendPromptWithFile to use our custom responses
func (m *CustomMockProvider) SendPromptWithFile(model, prompt string, file models.FileInput) (string, error) {
	// First check if we have a custom response for this prompt
	for key, response := range m.responses {
		if strings.Contains(prompt, key) {
			return response, nil
		}
	}

	// Fall back to the parent implementation
	return "mock response", nil
}

func TestProcessWithDefer(t *testing.T) {
	// Create a temporary defer.yaml file for the test
	deferYAML := `
determine_poem_type:
  input: STDIN
  model: gpt-4o-mini
  action: "Analyze the input poem"
  output: STDOUT

defer:
  analyze_haiku:
    input: STDIN
    model: gpt-4o-mini
    action: "This is the haiku analysis."
    output: STDOUT
`
	// Load the DSL config from the string
	var dslConfig DSLConfig
	if err := yaml.Unmarshal([]byte(deferYAML), &dslConfig); err != nil {
		t.Fatalf("Failed to unmarshal yaml: %v", err)
	}

	// Create a custom mock provider for this test
	customMockProvider := &CustomMockProvider{
		MockProvider: *NewMockProvider("openai"),
		responses: map[string]string{
			"Analyze the input poem":      `{"step":"analyze_haiku","input":"a test haiku"}`,
			"This is the haiku analysis.": "Haiku analysis complete.",
		},
	}
	customMockProvider.Configure("test-key")

	// Store the original DetectProvider function
	originalDetect := models.DetectProvider

	// Override the DetectProvider function to return our custom mock provider
	models.DetectProvider = func(modelName string) models.Provider {
		return customMockProvider
	}

	// Restore the original function when the test is done
	defer func() { models.DetectProvider = originalDetect }()

	// Create a processor with the default environment config
	processor := NewProcessor(&dslConfig, createTestEnvConfig(), createTestServerConfig(), true, "")
	processor.SetLastOutput("An old silent pond...") // Initial STDIN

	// Process the workflow
	err := processor.Process()
	if err != nil {
		t.Fatalf("Process() failed: %v", err)
	}

	// Check the final output
	expectedOutput := "Haiku analysis complete."
	if processor.LastOutput() != expectedOutput {
		t.Errorf("Expected final output to be '%s', but got '%s'", expectedOutput, processor.LastOutput())
	}
}

func TestUnmarshalYAMLWithDuplicateDeferSteps(t *testing.T) {
	// Define a YAML with duplicate step names in the defer block
	deferYAML := `
determine_poem_type:
  input: STDIN
  model: gpt-4o-mini
  action: "Analyze the input poem"
  output: STDOUT

defer:
  analyze_haiku:
    input: STDIN
    model: gpt-4o-mini
    action: "This is the haiku analysis."
    output: STDOUT
  analyze_haiku:
    input: STDIN
    model: gpt-4o-mini
    action: "This is another haiku analysis."
    output: STDOUT
`
	// Try to unmarshal the YAML
	var dslConfig DSLConfig
	err := yaml.Unmarshal([]byte(deferYAML), &dslConfig)

	// Verify that an error was returned
	if err == nil {
		t.Error("Expected error for duplicate step names in defer block, but got nil")
	} else if !strings.Contains(err.Error(), "already defined") {
		t.Errorf("Expected error message to contain 'already defined', but got: %v", err)
	}
}

func TestProcessWithUncalledDefer(t *testing.T) {
	// Define a YAML where the deferred step should NOT be called
	deferYAML := `
determine_poem_type:
  input: STDIN
  model: gpt-4o-mini
  action: "Analyze the input poem"
  output: STDOUT

defer:
  analyze_sonnet:
    input: STDIN
    model: gpt-4o-mini
    action: "This is the sonnet analysis."
    output: STDOUT
`
	var dslConfig DSLConfig
	if err := yaml.Unmarshal([]byte(deferYAML), &dslConfig); err != nil {
		t.Fatalf("Failed to unmarshal yaml: %v", err)
	}

	// Create a custom mock provider for this test
	customMockProvider := &CustomMockProvider{
		MockProvider: *NewMockProvider("openai"),
		responses: map[string]string{
			"Analyze the input poem":       "Just a regular string output.",
			"This is the sonnet analysis.": "THIS_SHOULD_NOT_BE_RETURNED",
		},
	}
	customMockProvider.Configure("test-key")

	// Store the original DetectProvider function
	originalDetect := models.DetectProvider

	// Override the DetectProvider function to return our custom mock provider
	models.DetectProvider = func(modelName string) models.Provider {
		return customMockProvider
	}

	// Restore the original function when the test is done
	defer func() { models.DetectProvider = originalDetect }()

	// Create a processor with the default environment config
	processor := NewProcessor(&dslConfig, createTestEnvConfig(), createTestServerConfig(), true, "")
	processor.SetLastOutput("An old silent pond...")

	err := processor.Process()
	if err != nil {
		t.Fatalf("Process() failed: %v", err)
	}

	// The final output should be from the first step, as the defer step is never called.
	expectedOutput := "Just a regular string output."
	if processor.LastOutput() != expectedOutput {
		t.Errorf("Expected final output to be '%s', but got '%s'", expectedOutput, processor.LastOutput())
	}
}

func TestIsURL(t *testing.T) {
	processor := NewProcessor(&DSLConfig{}, createTestEnvConfig(), createTestServerConfig(), false, "")

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "valid http URL",
			input:    "http://example.com",
			expected: true,
		},
		{
			name:     "valid https URL",
			input:    "https://example.com/path?query=value",
			expected: true,
		},
		{
			name:     "invalid URL - no scheme",
			input:    "example.com",
			expected: false,
		},
		{
			name:     "invalid URL - empty string",
			input:    "",
			expected: false,
		},
		{
			name:     "invalid URL - file path",
			input:    "/path/to/file.txt",
			expected: false,
		},
		{
			name:     "invalid URL - relative path",
			input:    "path/to/file.txt",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.isURL(tt.input)
			if result != tt.expected {
				t.Errorf("isURL() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFetchURL(t *testing.T) {
	processor := NewProcessor(&DSLConfig{}, createTestEnvConfig(), createTestServerConfig(), false, "")

	// Create test server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/text":
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("Hello, World!"))
		case "/html":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html><body>Hello, World!</body></html>"))
		case "/json":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"message": "Hello, World!"}`))
		case "/error":
			http.Error(w, "Not Found", http.StatusNotFound)
		}
	}))
	defer ts.Close()

	tests := []struct {
		name        string
		url         string
		expectError bool
		contentType string
	}{
		{
			name:        "fetch text content",
			url:         ts.URL + "/text",
			expectError: false,
			contentType: "text/plain",
		},
		{
			name:        "fetch HTML content",
			url:         ts.URL + "/html",
			expectError: false,
			contentType: "text/html",
		},
		{
			name:        "fetch JSON content",
			url:         ts.URL + "/json",
			expectError: false,
			contentType: "application/json",
		},
		{
			name:        "fetch error response",
			url:         ts.URL + "/error",
			expectError: true,
		},
		{
			name:        "invalid URL",
			url:         "http://invalid.url.that.does.not.exist",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpPath, err := processor.fetchURL(tt.url)
			if tt.expectError {
				if err == nil {
					t.Error("fetchURL() expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("fetchURL() unexpected error: %v", err)
				}
				if tmpPath == "" {
					t.Error("fetchURL() returned empty path")
				}
				// Clean up temporary file
				if tmpPath != "" {
					if err := processor.processFile(tmpPath); err != nil {
						t.Errorf("Failed to process fetched file: %v", err)
					}
				}
			}
		})
	}
}
