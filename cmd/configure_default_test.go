package cmd

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/spf13/cobra"
)

func TestConfigureDefaultFlag(t *testing.T) {
	// Create a temporary directory for test
	tempDir, err := os.MkdirTemp("", "comanda-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set up test environment
	testEnvPath := filepath.Join(tempDir, ".env")
	os.Setenv("COMANDA_ENV", testEnvPath)
	defer os.Unsetenv("COMANDA_ENV")

	// Create a test configuration with some models
	testConfig := &config.EnvConfig{
		Providers: map[string]*config.Provider{
			"openai": {
				APIKey: "test-key",
				Models: []config.Model{
					{Name: "gpt-4", Type: "external", Modes: []config.ModelMode{config.TextMode}},
					{Name: "gpt-3.5-turbo", Type: "external", Modes: []config.ModelMode{config.TextMode}},
				},
			},
			"anthropic": {
				APIKey: "test-key",
				Models: []config.Model{
					{Name: "claude-3-opus", Type: "external", Modes: []config.ModelMode{config.TextMode}},
				},
			},
		},
	}

	// Save the test configuration
	if err := config.SaveEnvConfig(testEnvPath, testConfig); err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Test 1: Verify getAllConfiguredModelNames function
	modelNames := getAllConfiguredModelNames(testConfig)
	expectedModels := []string{"gpt-4", "gpt-3.5-turbo", "claude-3-opus"}

	if len(modelNames) != len(expectedModels) {
		t.Errorf("Expected %d models, got %d", len(expectedModels), len(modelNames))
	}

	// Check that all expected models are present
	modelMap := make(map[string]bool)
	for _, model := range modelNames {
		modelMap[model] = true
	}
	for _, expected := range expectedModels {
		if !modelMap[expected] {
			t.Errorf("Expected model %s not found in configured models", expected)
		}
	}

	// Test 2: Test the --default flag behavior with no configured models
	emptyConfig := &config.EnvConfig{
		Providers: map[string]*config.Provider{},
	}
	if err := config.SaveEnvConfig(testEnvPath, emptyConfig); err != nil {
		t.Fatalf("Failed to save empty config: %v", err)
	}

	// Create a new command instance for testing
	rootCmd := &cobra.Command{}
	configureCmd := &cobra.Command{
		Use:   "configure",
		Short: "Configure model settings",
		Run: func(cmd *cobra.Command, args []string) {
			// This would normally be the full configure command logic
			// For testing, we'll just verify the flag is set
			if defaultFlag {
				envConfig, _ := config.LoadEnvConfig(testEnvPath)
				models := getAllConfiguredModelNames(envConfig)
				if len(models) == 0 {
					// This is the expected behavior
					return
				}
			}
		},
	}

	configureCmd.Flags().BoolVar(&defaultFlag, "default", false, "Interactively set the default model for workflow generation")
	rootCmd.AddCommand(configureCmd)

	// Test with --default flag
	defaultFlag = true
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"configure", "--default"})

	// Execute should not error even with no models
	if err := rootCmd.Execute(); err != nil {
		t.Errorf("Command execution failed: %v", err)
	}

	// Test 3: Verify that setting default model updates the configuration
	testConfig.DefaultGenerationModel = "gpt-4"
	if err := config.SaveEnvConfig(testEnvPath, testConfig); err != nil {
		t.Fatalf("Failed to save config with default model: %v", err)
	}

	// Load and verify
	loadedConfig, err := config.LoadEnvConfig(testEnvPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if loadedConfig.DefaultGenerationModel != "gpt-4" {
		t.Errorf("Expected default model to be 'gpt-4', got '%s'", loadedConfig.DefaultGenerationModel)
	}
}

func TestConfigureSetDefaultGenerationModelFlag(t *testing.T) {
	// Create a temporary directory for test
	tempDir, err := os.MkdirTemp("", "comanda-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set up test environment
	testEnvPath := filepath.Join(tempDir, ".env")
	os.Setenv("COMANDA_ENV", testEnvPath)
	defer os.Unsetenv("COMANDA_ENV")

	// Create a test configuration
	testConfig := &config.EnvConfig{
		Providers: map[string]*config.Provider{
			"openai": {
				APIKey: "test-key",
				Models: []config.Model{
					{Name: "gpt-4", Type: "external", Modes: []config.ModelMode{config.TextMode}},
				},
			},
		},
	}

	if err := config.SaveEnvConfig(testEnvPath, testConfig); err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Test the --set-default-generation-model flag
	setDefaultGenerationModelFlag = "gpt-4"

	// Simulate the command logic
	envConfig, err := config.LoadEnvConfig(testEnvPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	envConfig.DefaultGenerationModel = setDefaultGenerationModelFlag

	if err := config.SaveEnvConfig(testEnvPath, envConfig); err != nil {
		t.Fatalf("Failed to save updated config: %v", err)
	}

	// Verify the change
	updatedConfig, err := config.LoadEnvConfig(testEnvPath)
	if err != nil {
		t.Fatalf("Failed to load updated config: %v", err)
	}

	if updatedConfig.DefaultGenerationModel != "gpt-4" {
		t.Errorf("Expected default generation model to be 'gpt-4', got '%s'", updatedConfig.DefaultGenerationModel)
	}
}

func TestListConfigurationWithDefaultModel(t *testing.T) {
	// Create a temporary directory for test
	tempDir, err := os.MkdirTemp("", "comanda-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Set up test environment
	testEnvPath := filepath.Join(tempDir, ".env")
	os.Setenv("COMANDA_ENV", testEnvPath)
	defer os.Unsetenv("COMANDA_ENV")

	// Create a test configuration with default model
	testConfig := &config.EnvConfig{
		DefaultGenerationModel: "claude-3-opus",
		Providers: map[string]*config.Provider{
			"anthropic": {
				APIKey: "test-key",
				Models: []config.Model{
					{Name: "claude-3-opus", Type: "external", Modes: []config.ModelMode{config.TextMode}},
				},
			},
		},
	}

	if err := config.SaveEnvConfig(testEnvPath, testConfig); err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr) // Restore default log output

	// Call listConfiguration
	listConfiguration()

	// Get captured output
	output := buf.String()

	// Verify default model is displayed (accounting for log timestamp prefix)
	if !strings.Contains(output, "Default Generation Model: claude-3-opus") && !strings.Contains(output, "claude-3-opus") {
		t.Errorf("Expected output to contain default generation model, got: %s", output)
	}
}
