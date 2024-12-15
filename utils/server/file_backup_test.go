package server

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
)

func TestFileBackupAndRestore(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "comanda-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create test files
	files := map[string]string{
		"test1.yaml":            "content1",
		"test2.yaml":            "content2",
		"subdir/test3.yaml":     "content3",
		"deep/nested/test.yaml": "nested content",
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

	var backupFilename string

	// Test backup creation
	t.Run("Create Backup", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/files/backup", nil)
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()

		server.handleFileBackup(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}

		var response BackupResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}

		if !response.Success {
			t.Error("Expected success to be true")
		}

		if !strings.HasPrefix(response.Filename, "backup-") || !strings.HasSuffix(response.Filename, ".zip") {
			t.Errorf("Invalid backup filename format: %s", response.Filename)
		}

		backupFilename = response.Filename

		// Verify backup file exists
		backupPath := filepath.Join(tempDir, "backups", backupFilename)
		if _, err := os.Stat(backupPath); os.IsNotExist(err) {
			t.Error("Backup file was not created")
		}

		// Verify backup contents
		reader, err := zip.OpenReader(backupPath)
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()

		foundFiles := make(map[string]bool)
		for _, file := range reader.File {
			foundFiles[file.Name] = true

			if content, ok := files[file.Name]; ok {
				rc, err := file.Open()
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(rc)
				rc.Close()
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != content {
					t.Errorf("File content mismatch for %s", file.Name)
				}
			}
		}

		for path := range files {
			if !foundFiles[path] {
				t.Errorf("File %s not found in backup", path)
			}
		}
	})

	// Test restore with path traversal attempt
	t.Run("Restore with Path Traversal", func(t *testing.T) {
		// Create a malicious zip file
		maliciousZip := filepath.Join(tempDir, "backups", "malicious.zip")
		zipFile, err := os.Create(maliciousZip)
		if err != nil {
			t.Fatal(err)
		}

		zipWriter := zip.NewWriter(zipFile)
		writer, err := zipWriter.Create("../outside.txt")
		if err != nil {
			t.Fatal(err)
		}
		writer.Write([]byte("malicious content"))
		zipWriter.Close()
		zipFile.Close()

		// Try to restore the malicious backup
		req := RestoreRequest{
			Backup: "malicious.zip",
		}
		body, _ := json.Marshal(req)
		httpReq := httptest.NewRequest("POST", "/files/restore", bytes.NewBuffer(body))
		httpReq.Header.Set("Authorization", "Bearer test-token")
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleFileRestore(w, httpReq)

		if w.Code != http.StatusForbidden {
			t.Errorf("Expected status code %d, got %d", http.StatusForbidden, w.Code)
		}

		var response BackupResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}

		if response.Success {
			t.Error("Expected restore to fail")
		}

		if !strings.Contains(response.Error, "Invalid file path") {
			t.Errorf("Expected error about invalid path, got: %s", response.Error)
		}

		// Verify no files were created outside data directory
		outsidePath := filepath.Join(filepath.Dir(tempDir), "outside.txt")
		if _, err := os.Stat(outsidePath); !os.IsNotExist(err) {
			t.Error("File was created outside data directory")
			os.Remove(outsidePath)
		}
	})

	// Test restore with invalid backup path
	t.Run("Restore with Invalid Backup Path", func(t *testing.T) {
		tests := []struct {
			name           string
			backup         string
			expectedStatus int
			expectedError  string
		}{
			{
				name:           "Path traversal in backup name",
				backup:         "../malicious.zip",
				expectedStatus: http.StatusBadRequest,
				expectedError:  "Invalid backup path",
			},
			{
				name:           "Absolute path in backup name",
				backup:         "/etc/malicious.zip",
				expectedStatus: http.StatusBadRequest,
				expectedError:  "Invalid backup path",
			},
			{
				name:           "Empty backup name",
				backup:         "",
				expectedStatus: http.StatusBadRequest,
				expectedError:  "Backup name is required",
			},
			{
				name:           "Nonexistent backup",
				backup:         "nonexistent.zip",
				expectedStatus: http.StatusNotFound,
				expectedError:  "Backup file not found",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				req := RestoreRequest{
					Backup: tt.backup,
				}
				body, _ := json.Marshal(req)
				httpReq := httptest.NewRequest("POST", "/files/restore", bytes.NewBuffer(body))
				httpReq.Header.Set("Authorization", "Bearer test-token")
				httpReq.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()

				server.handleFileRestore(w, httpReq)

				if w.Code != tt.expectedStatus {
					t.Errorf("Expected status code %d, got %d", tt.expectedStatus, w.Code)
				}

				var response BackupResponse
				if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
					t.Fatal(err)
				}

				if response.Success {
					t.Error("Expected restore to fail")
				}

				if response.Error != tt.expectedError {
					t.Errorf("Expected error '%s', got '%s'", tt.expectedError, response.Error)
				}
			})
		}
	})

	// Test restore with valid backup
	t.Run("Restore Valid Backup", func(t *testing.T) {
		// Delete all test files first
		for path := range files {
			os.Remove(filepath.Join(tempDir, path))
		}

		// Restore from backup
		req := RestoreRequest{
			Backup: backupFilename,
		}
		body, _ := json.Marshal(req)
		httpReq := httptest.NewRequest("POST", "/files/restore", bytes.NewBuffer(body))
		httpReq.Header.Set("Authorization", "Bearer test-token")
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleFileRestore(w, httpReq)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}

		var response BackupResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}

		if !response.Success {
			t.Errorf("Restore failed: %s", response.Error)
		}

		// Verify all files were restored with correct content
		for path, expectedContent := range files {
			content, err := os.ReadFile(filepath.Join(tempDir, path))
			if err != nil {
				t.Errorf("Error reading restored file %s: %v", path, err)
				continue
			}
			if string(content) != expectedContent {
				t.Errorf("Restored content mismatch for %s", path)
			}
		}
	})
}
