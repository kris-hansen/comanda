package processor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
)

func TestProcessInputs(t *testing.T) {
	// Create temporary test files
	tmpDir := t.TempDir()

	validFile1 := filepath.Join(tmpDir, "test1.txt")
	validFile2 := filepath.Join(tmpDir, "test2.txt")
	invalidFile := filepath.Join(tmpDir, "test.invalid")

	// Create runtime directory and file
	runtimeDir := filepath.Join(tmpDir, "runtime")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatalf("Failed to create runtime directory: %v", err)
	}
	runtimeFile := filepath.Join(runtimeDir, "runtime.txt")

	// Create test files
	if err := os.WriteFile(validFile1, []byte("test content 1"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	if err := os.WriteFile(validFile2, []byte("test content 2"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	if err := os.WriteFile(invalidFile, []byte("invalid content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	if err := os.WriteFile(runtimeFile, []byte("runtime content"), 0644); err != nil {
		t.Fatalf("Failed to create runtime file: %v", err)
	}

	tests := []struct {
		name          string
		inputs        []string
		expectErr     bool
		serverEnabled bool
		runtimeDir    string
	}{
		{
			name:      "single valid file",
			inputs:    []string{validFile1},
			expectErr: false,
		},
		{
			name:      "multiple valid files",
			inputs:    []string{validFile1, validFile2},
			expectErr: false,
		},
		{
			name:      "invalid file extension",
			inputs:    []string{invalidFile},
			expectErr: true,
		},
		{
			name:      "non-existent file",
			inputs:    []string{filepath.Join(tmpDir, "nonexistent.txt")},
			expectErr: true,
		},
		{
			name:      "empty input list",
			inputs:    []string{},
			expectErr: false,
		},
		{
			name:      "NA input",
			inputs:    []string{"NA"},
			expectErr: false,
		},
		{
			name:      "glob pattern",
			inputs:    []string{filepath.Join(tmpDir, "*.txt")},
			expectErr: false,
		},
		{
			name:          "runtime directory file",
			inputs:        []string{"runtime.txt"},
			expectErr:     false,
			serverEnabled: true,
			runtimeDir:    "runtime",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create server config based on test case
			var serverConfig *config.ServerConfig
			if tt.serverEnabled {
				serverConfig = &config.ServerConfig{
					Enabled: true,
					DataDir: tmpDir,
				}
			} else {
				serverConfig = &config.ServerConfig{
					Enabled: true,
				}
			}

			processor := NewProcessor(&DSLConfig{}, createTestEnvConfig(), serverConfig, false, tt.runtimeDir)

			err := processor.processInputs(tt.inputs)
			if (err != nil) != tt.expectErr {
				t.Errorf("processInputs() error = %v, expectErr %v", err, tt.expectErr)
			}

			if !tt.expectErr && len(tt.inputs) > 0 && tt.inputs[0] != "NA" {
				inputs := processor.GetProcessedInputs()
				if len(inputs) == 0 {
					t.Error("No inputs were processed")
				}
			}
		})
	}
}

func TestGetProcessedInputs(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	processor := NewProcessor(&DSLConfig{}, createTestEnvConfig(), &config.ServerConfig{Enabled: true}, false, "")

	// Process a test file
	err := processor.processInputs([]string{testFile})
	if err != nil {
		t.Fatalf("Failed to process input: %v", err)
	}

	// Get processed inputs
	inputs := processor.GetProcessedInputs()
	if len(inputs) != 1 {
		t.Errorf("Expected 1 processed input, got %d", len(inputs))
	}

	if string(inputs[0].Contents) != "test content" {
		t.Errorf("Expected content 'test content', got '%s'", string(inputs[0].Contents))
	}
}
