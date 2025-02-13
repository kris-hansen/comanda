package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
)

func TestHandleFileDownload(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "comanda-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files
	files := map[string]string{
		"test.txt":        "test content",
		"subdir/test.txt": "subdirectory content",
		"test.json":       `{"key": "value"}`,
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
		mux: http.NewServeMux(),
		config: &config.ServerConfig{
			DataDir:     tempDir,
			BearerToken: "test-token",
			Enabled:     true,
		},
		envConfig: &config.EnvConfig{},
	}

	// Register routes
	server.routes()

	// Test cases
	tests := []struct {
		name            string
		path            string
		expectedStatus  int
		expectedError   string
		expectedContent string
	}{
		{
			name:            "Valid file download",
			path:            "test.txt",
			expectedStatus:  http.StatusOK,
			expectedContent: "test content",
		},
		{
			name:            "Download from subdirectory",
			path:            "subdir/test.txt",
			expectedStatus:  http.StatusOK,
			expectedContent: "subdirectory content",
		},
		{
			name:           "Nonexistent file",
			path:           "nonexistent.txt",
			expectedStatus: http.StatusNotFound,
			expectedError:  "File not found",
		},
		{
			name:           "Path traversal attempt",
			path:           "../test.txt",
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
			// Create request
			req := httptest.NewRequest("GET", "/files/download?path="+tt.path, nil)
			req.Header.Set("Authorization", "Bearer test-token")
			rec := httptest.NewRecorder()

			// Handle request using the server's mux
			server.mux.ServeHTTP(rec, req)

			// Check status code
			if rec.Code != tt.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatus, rec.Code)
			}

			if tt.expectedError != "" {
				var response FileResponse
				if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
					t.Fatal(err)
				}
				if response.Error != tt.expectedError {
					t.Errorf("Expected error '%s', got '%s'", tt.expectedError, response.Error)
				}
				return
			}

			// For successful downloads
			if tt.expectedContent != "" {
				// Check Content-Type header
				contentType := rec.Header().Get("Content-Type")
				if contentType != "application/octet-stream" {
					t.Errorf("Expected Content-Type 'application/octet-stream', got '%s'", contentType)
				}

				// Check Content-Disposition header
				contentDisposition := rec.Header().Get("Content-Disposition")
				expectedFilename := filepath.Base(tt.path)
				expectedDisposition := "attachment; filename=" + expectedFilename
				if contentDisposition != expectedDisposition {
					t.Errorf("Expected Content-Disposition '%s', got '%s'", expectedDisposition, contentDisposition)
				}

				// Check file content
				content, err := io.ReadAll(rec.Body)
				if err != nil {
					t.Fatal(err)
				}
				if string(content) != tt.expectedContent {
					t.Errorf("Expected content '%s', got '%s'", tt.expectedContent, string(content))
				}
			}
		})
	}

	// Test directory download attempt
	t.Run("Directory download attempt", func(t *testing.T) {
		// Create a test directory
		dirPath := filepath.Join(tempDir, "testdir")
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			t.Fatal(err)
		}

		req := httptest.NewRequest("GET", "/files/download?path=testdir", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()

		server.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, rec.Code)
		}

		var response FileResponse
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}

		expectedError := "Cannot download a directory"
		if response.Error != expectedError {
			t.Errorf("Expected error '%s', got '%s'", expectedError, response.Error)
		}
	})
}
