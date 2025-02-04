package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
)

func TestHandleFileUpload(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "comanda-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create server instance
	server := &Server{
		mux: http.NewServeMux(),
		config: &ServerConfig{
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
		name           string
		filename       string
		content        string
		path           string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Valid file upload",
			filename:       "test.txt",
			content:        "test content",
			path:           "uploaded.txt",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Upload to subdirectory",
			filename:       "test.txt",
			content:        "test content",
			path:           "subdir/test.txt",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Path traversal attempt",
			filename:       "test.txt",
			content:        "test content",
			path:           "../test.txt",
			expectedStatus: http.StatusForbidden,
			expectedError:  "Invalid file path: access denied",
		},
		{
			name:           "Empty path (use filename)",
			filename:       "test.txt",
			content:        "test content",
			path:           "",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create multipart form
			var b bytes.Buffer
			w := multipart.NewWriter(&b)

			// Add file
			fw, err := w.CreateFormFile("file", tt.filename)
			if err != nil {
				t.Fatal(err)
			}
			_, err = io.Copy(fw, strings.NewReader(tt.content))
			if err != nil {
				t.Fatal(err)
			}

			// Add path if provided
			if tt.path != "" {
				if err := w.WriteField("path", tt.path); err != nil {
					t.Fatal(err)
				}
			}

			w.Close()

			// Create request
			req := httptest.NewRequest("POST", "/files/upload", &b)
			req.Header.Set("Content-Type", w.FormDataContentType())
			req.Header.Set("Authorization", "Bearer test-token")
			rec := httptest.NewRecorder()

			// Handle request using the server's mux
			server.mux.ServeHTTP(rec, req)

			// Check status code
			if rec.Code != tt.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatus, rec.Code)
			}

			var response FileUploadResponse
			if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
				t.Fatal(err)
			}

			if tt.expectedError != "" {
				if response.Error != tt.expectedError {
					t.Errorf("Expected error '%s', got '%s'", tt.expectedError, response.Error)
				}
				return
			}

			// For successful uploads, verify file exists with correct content
			expectedPath := tt.path
			if expectedPath == "" {
				expectedPath = tt.filename
			}
			fullPath := filepath.Join(tempDir, expectedPath)
			content, err := os.ReadFile(fullPath)
			if err != nil {
				t.Fatalf("Failed to read uploaded file: %v", err)
			}

			if string(content) != tt.content {
				t.Errorf("Expected content '%s', got '%s'", tt.content, string(content))
			}

			// Verify file info in response
			if response.File.Path != expectedPath {
				t.Errorf("Expected path '%s', got '%s'", expectedPath, response.File.Path)
			}
		})
	}

	// Test missing file
	t.Run("Missing file", func(t *testing.T) {
		var b bytes.Buffer
		w := multipart.NewWriter(&b)
		w.Close()

		req := httptest.NewRequest("POST", "/files/upload", &b)
		req.Header.Set("Content-Type", w.FormDataContentType())
		req.Header.Set("Authorization", "Bearer test-token")
		rec := httptest.NewRecorder()

		server.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, rec.Code)
		}

		var response FileUploadResponse
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}

		if response.Error != "No file provided" {
			t.Errorf("Expected error 'No file provided', got '%s'", response.Error)
		}
	})
}
