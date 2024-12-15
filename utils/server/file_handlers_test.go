package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
)

func TestHandleCreateFile(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "comanda-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create server instance
	server := &Server{
		config: &ServerConfig{
			DataDir:     tempDir,
			BearerToken: "test-token",
			Enabled:     true,
		},
		envConfig: &config.EnvConfig{},
	}

	// Test cases
	tests := []struct {
		name           string
		request        FileRequest
		expectedStatus int
		expectedError  string
	}{
		{
			name: "Valid file creation",
			request: FileRequest{
				Path:    "test.yaml",
				Content: "test content",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Create file in subdirectory",
			request: FileRequest{
				Path:    "subdir/test.yaml",
				Content: "test content",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Path traversal attempt with ..",
			request: FileRequest{
				Path:    "../test.yaml",
				Content: "test content",
			},
			expectedStatus: http.StatusForbidden,
			expectedError:  "Invalid file path: access denied",
		},
		{
			name: "Path traversal attempt with /../",
			request: FileRequest{
				Path:    "subdir/../outside.yaml",
				Content: "test content",
			},
			expectedStatus: http.StatusForbidden,
			expectedError:  "Invalid file path: access denied",
		},
		{
			name: "Path traversal attempt with /../../",
			request: FileRequest{
				Path:    "subdir/../../outside.yaml",
				Content: "test content",
			},
			expectedStatus: http.StatusForbidden,
			expectedError:  "Invalid file path: access denied",
		},
		{
			name: "Absolute path attempt",
			request: FileRequest{
				Path:    "/etc/test.yaml",
				Content: "test content",
			},
			expectedStatus: http.StatusForbidden,
			expectedError:  "Invalid file path: access denied",
		},
		{
			name: "Empty path",
			request: FileRequest{
				Path:    "",
				Content: "test content",
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request body
			body, err := json.Marshal(tt.request)
			if err != nil {
				t.Fatal(err)
			}

			// Create test request
			req := httptest.NewRequest("POST", "/files", bytes.NewBuffer(body))
			req.Header.Set("Authorization", "Bearer test-token")
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			// Call handler
			server.handleFileOperation(w, req)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatus, w.Code)
			}

			var response FileResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatal(err)
			}

			// Check error message if expected
			if tt.expectedError != "" {
				if response.Error != tt.expectedError {
					t.Errorf("Expected error '%s', got '%s'", tt.expectedError, response.Error)
				}

				// Verify file was not created for error cases
				if _, err := os.Stat(filepath.Join(tempDir, tt.request.Path)); !os.IsNotExist(err) {
					t.Error("File should not have been created")
				}

				// For path traversal attempts, verify no file was created outside data directory
				if strings.Contains(tt.request.Path, "..") {
					parentPath := filepath.Join(filepath.Dir(tempDir), "outside.yaml")
					if _, err := os.Stat(parentPath); !os.IsNotExist(err) {
						t.Error("File was created outside data directory")
						os.Remove(parentPath)
					}
				}
			} else {
				// For empty path, verify file was created in data directory
				var filePath string
				if tt.request.Path == "" {
					filePath = filepath.Join(tempDir, "file.txt")
				} else {
					filePath = filepath.Join(tempDir, tt.request.Path)
				}

				// Verify file was created with correct content
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatal(err)
				}
				if string(content) != tt.request.Content {
					t.Errorf("Expected content '%s', got '%s'", tt.request.Content, string(content))
				}

				// Verify file info in response
				expectedPath := tt.request.Path
				if expectedPath == "" {
					expectedPath = "file.txt"
				}
				if response.File.Path != expectedPath {
					t.Errorf("Expected path '%s', got '%s'", expectedPath, response.File.Path)
				}
			}
		})
	}

	// Test creating an existing file
	t.Run("Create existing file", func(t *testing.T) {
		// Create a file first
		existingPath := "existing.yaml"
		existingFile := filepath.Join(tempDir, existingPath)
		if err := os.WriteFile(existingFile, []byte("initial content"), 0644); err != nil {
			t.Fatal(err)
		}

		req := FileRequest{
			Path:    existingPath,
			Content: "new content",
		}
		body, _ := json.Marshal(req)
		httpReq := httptest.NewRequest("POST", "/files", bytes.NewBuffer(body))
		httpReq.Header.Set("Authorization", "Bearer test-token")
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleFileOperation(w, httpReq)

		if w.Code != http.StatusConflict {
			t.Errorf("Expected status code %d, got %d", http.StatusConflict, w.Code)
		}

		var response FileResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}

		if response.Error != "File already exists" {
			t.Errorf("Expected error 'File already exists', got '%s'", response.Error)
		}

		// Verify original content wasn't changed
		content, err := os.ReadFile(existingFile)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != "initial content" {
			t.Error("Original file content was modified")
		}
	})
}

func TestHandleDeleteFile(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "comanda-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files
	files := map[string]string{
		"test.yaml":        "test content",
		"subdir/test.yaml": "test content",
	}

	for path, content := range files {
		fullPath := filepath.Join(tempDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Create server instance
	server := &Server{
		config: &ServerConfig{
			DataDir:     tempDir,
			BearerToken: "test-token",
			Enabled:     true,
		},
		envConfig: &config.EnvConfig{},
	}

	// Test cases
	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Valid delete",
			path:           "test.yaml",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Delete file in subdirectory",
			path:           "subdir/test.yaml",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Delete nonexistent file",
			path:           "nonexistent.yaml",
			expectedStatus: http.StatusNotFound,
			expectedError:  "File not found",
		},
		{
			name:           "Path traversal attempt",
			path:           "../test.yaml",
			expectedStatus: http.StatusForbidden,
			expectedError:  "Invalid file path: access denied",
		},
		{
			name:           "Path traversal attempt with /../",
			path:           "subdir/../outside.yaml",
			expectedStatus: http.StatusForbidden,
			expectedError:  "Invalid file path: access denied",
		},
		{
			name:           "Absolute path attempt",
			path:           "/etc/test.yaml",
			expectedStatus: http.StatusForbidden,
			expectedError:  "Invalid file path: access denied",
		},
		{
			name:           "Empty path",
			path:           "",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Path parameter is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("DELETE", "/files?path="+tt.path, nil)
			req.Header.Set("Authorization", "Bearer test-token")
			w := httptest.NewRecorder()

			// Call handler
			server.handleFileOperation(w, req)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatus, w.Code)
			}

			var response FileResponse
			if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
				t.Fatal(err)
			}

			// Check error message if expected
			if tt.expectedError != "" {
				if response.Error != tt.expectedError {
					t.Errorf("Expected error '%s', got '%s'", tt.expectedError, response.Error)
				}

				// For path traversal attempts, verify no file was deleted outside data directory
				if strings.Contains(tt.path, "..") {
					parentPath := filepath.Join(filepath.Dir(tempDir), "test.yaml")
					if _, err := os.Stat(parentPath); err == nil {
						t.Error("File outside data directory was affected")
					}
				}
			} else {
				// Verify file was deleted
				if _, err := os.Stat(filepath.Join(tempDir, tt.path)); !os.IsNotExist(err) {
					t.Error("Expected file to be deleted")
				}
			}
		})
	}
}
