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

	// Get absolute path for DataDir
	absDataDir, err := filepath.Abs(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create server instance
	server := &Server{
		config: &config.ServerConfig{
			DataDir:     absDataDir,
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
		expectedPath   string
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
			expectedPath:   "file.txt", // Should create file.txt in DataDir root
		},
		{
			name: "Deep nested directory",
			request: FileRequest{
				Path:    "very/deep/nested/path/test.txt",
				Content: "nested content",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "File in DataDir root",
			request: FileRequest{
				Path:    "root-file.txt",
				Content: "root content",
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

				// Verify file is within DataDir
				rel, err := filepath.Rel(tempDir, filePath)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(rel, "..") {
					t.Errorf("File was created outside DataDir: %s", filePath)
				}

				// For empty path, verify file.txt is created in DataDir root
				if tt.request.Path == "" {
					expectedLocation := filepath.Join(tempDir, "file.txt")
					if filePath != expectedLocation {
						t.Errorf("Expected file at %s, got %s", expectedLocation, filePath)
					}
				}

				// For nested paths, verify directory structure was created
				if strings.Contains(tt.request.Path, "/") {
					dir := filepath.Dir(filePath)
					if _, err := os.Stat(dir); err != nil {
						t.Errorf("Directory structure was not created properly: %v", err)
					}
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

func TestHandleGetFileContent(t *testing.T) {
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

	// Create server instance
	server := &Server{
		config: &config.ServerConfig{
			DataDir:     absDataDir,
			BearerToken: "test-token",
			Enabled:     true,
		},
		envConfig: &config.EnvConfig{},
	}

	// Create test files in DataDir
	files := map[string]string{
		"test.txt":           "test content",
		"subdir/nested.txt":  "nested content",
		"deep/path/file.txt": "deep content",
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

	// Create a file outside DataDir for testing access restrictions
	outsideFile := filepath.Join(filepath.Dir(tempDir), "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("outside content"), 0644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outsideFile)

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectedError  string
		expectedBody   string
	}{
		{
			name:           "Get file from root of DataDir",
			path:           "test.txt",
			expectedStatus: http.StatusOK,
			expectedBody:   "test content",
		},
		{
			name:           "Get file from subdirectory",
			path:           "subdir/nested.txt",
			expectedStatus: http.StatusOK,
			expectedBody:   "nested content",
		},
		{
			name:           "Get file from deep path",
			path:           "deep/path/file.txt",
			expectedStatus: http.StatusOK,
			expectedBody:   "deep content",
		},
		{
			name:           "Path traversal attempt",
			path:           "../outside.txt",
			expectedStatus: http.StatusForbidden,
			expectedError:  "Invalid file path: access denied",
		},
		{
			name:           "Nested path traversal attempt",
			path:           "subdir/../../outside.txt",
			expectedStatus: http.StatusForbidden,
			expectedError:  "Invalid file path: access denied",
		},
		{
			name:           "Absolute path attempt",
			path:           outsideFile,
			expectedStatus: http.StatusForbidden,
			expectedError:  "Invalid file path: access denied",
		},
		{
			name:           "Nonexistent file",
			path:           "nonexistent.txt",
			expectedStatus: http.StatusNotFound,
			expectedError:  "File not found",
		},
		{
			name:           "Empty path",
			path:           "",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "path parameter is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test request
			req := httptest.NewRequest("GET", "/files/content?path="+tt.path, nil)
			req.Header.Set("Authorization", "Bearer test-token")
			w := httptest.NewRecorder()

			// Call handler
			server.handleGetFileContent(w, req)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatus, w.Code)
			}

			// For error cases, check error message
			if tt.expectedError != "" {
				var response FileResponse
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Fatal(err)
				}
				if response.Error != tt.expectedError {
					t.Errorf("Expected error '%s', got '%s'", tt.expectedError, response.Error)
				}
			} else {
				// For success cases, check file content
				content := w.Body.String()
				if content != tt.expectedBody {
					t.Errorf("Expected content '%s', got '%s'", tt.expectedBody, content)
				}
			}
		})
	}
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

	// Get absolute path for DataDir
	absDataDir, err := filepath.Abs(tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create server instance
	server := &Server{
		config: &config.ServerConfig{
			DataDir:     absDataDir,
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
			expectedError:  "path parameter is required",
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
