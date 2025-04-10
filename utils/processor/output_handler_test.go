package processor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
)

func TestHandleOutput(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "comanda-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Get absolute path for DataDir
	absDataDir, err := filepath.Abs(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Test cases
	tests := []struct {
		name            string
		serverEnabled   bool
		outputs         []string
		response        string
		expectedFiles   map[string]string // map[filepath]expectedContent
		expectInDataDir bool
	}{
		{
			name:          "CLI mode - simple output",
			serverEnabled: false,
			outputs:       []string{"test.txt"},
			response:      "test content",
			expectedFiles: map[string]string{
				"test.txt": "test content",
			},
			expectInDataDir: false,
		},
		{
			name:          "CLI mode - nested output",
			serverEnabled: false,
			outputs:       []string{"subdir/test.txt"},
			response:      "nested content",
			expectedFiles: map[string]string{
				"subdir/test.txt": "nested content",
			},
			expectInDataDir: false,
		},
		{
			name:          "Server mode - simple output",
			serverEnabled: true,
			outputs:       []string{"test.txt"},
			response:      "test content",
			expectedFiles: map[string]string{
				"test.txt": "test content",
			},
			expectInDataDir: true,
		},
		{
			name:          "Server mode - nested output",
			serverEnabled: true,
			outputs:       []string{"subdir/test.txt"},
			response:      "nested content",
			expectedFiles: map[string]string{
				"subdir/test.txt": "nested content",
			},
			expectInDataDir: true,
		},
		{
			name:          "Server mode - multiple outputs",
			serverEnabled: true,
			outputs:       []string{"test1.txt", "subdir/test2.txt"},
			response:      "multi content",
			expectedFiles: map[string]string{
				"test1.txt":        "multi content",
				"subdir/test2.txt": "multi content",
			},
			expectInDataDir: true,
		},
		{
			name:          "Server mode - STDOUT and file",
			serverEnabled: true,
			outputs:       []string{"STDOUT", "test.txt"},
			response:      "mixed content",
			expectedFiles: map[string]string{
				"test.txt": "mixed content",
			},
			expectInDataDir: true,
		},
		{
			name:          "Server mode with runtime directory",
			serverEnabled: true,
			outputs:       []string{"test.txt"},
			response:      "runtime content",
			expectedFiles: map[string]string{
				"runtime/test.txt": "runtime content",
			},
			expectInDataDir: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create working directory for CLI mode tests
			if !tt.expectInDataDir {
				if err := os.Chdir(tempDir); err != nil {
					t.Fatalf("Failed to change working directory: %v", err)
				}
			}

			// Create server config based on test case
			var serverConfig *config.ServerConfig
			if tt.serverEnabled {
				serverConfig = &config.ServerConfig{
					Enabled: true,
					DataDir: absDataDir,
				}
			}

			// Create processor with test configuration
			proc := &Processor{
				serverConfig: serverConfig,
				verbose:      true,
			}

			// Set runtime directory for the runtime directory test
			if tt.name == "Server mode with runtime directory" {
				proc.runtimeDir = "runtime"
				// Create runtime directory
				if err := os.MkdirAll(filepath.Join(absDataDir, "runtime"), 0755); err != nil {
					t.Fatalf("Failed to create runtime directory: %v", err)
				}
			}

			// Handle output
			if err := proc.handleOutput("test-model", tt.response, tt.outputs, nil); err != nil {
				t.Fatalf("handleOutput failed: %v", err)
			}

			// Verify files were created in the correct location with correct content
			for filePath, expectedContent := range tt.expectedFiles {
				var fullPath string
				if tt.expectInDataDir {
					fullPath = filepath.Join(absDataDir, filePath)
				} else {
					fullPath = filepath.Join(tempDir, filePath)
				}

				// Read the file
				content, err := os.ReadFile(fullPath)
				if err != nil {
					t.Errorf("Failed to read file %s: %v", fullPath, err)
					continue
				}

				// Check content
				if string(content) != expectedContent {
					t.Errorf("File %s has wrong content. Expected %q, got %q", fullPath, expectedContent, string(content))
				}

				// If in server mode, verify file is within DataDir
				if tt.expectInDataDir {
					rel, err := filepath.Rel(absDataDir, fullPath)
					if err != nil {
						t.Errorf("Failed to get relative path: %v", err)
					}
					if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
						t.Errorf("File %s was created outside DataDir", fullPath)
					}
				}
			}
		})
	}
}
