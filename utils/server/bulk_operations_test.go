package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
)

func TestHandleBulkFileOperations(t *testing.T) {
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

	// Test bulk create
	t.Run("Bulk Create", func(t *testing.T) {
		req := BulkFileRequest{
			Files: []FileRequest{
				{Path: "test1.yaml", Content: "content1"},
				{Path: "test2.yaml", Content: "content2"},
				{Path: "subdir/test3.yaml", Content: "content3"},
				{Path: "../outside.yaml", Content: "bad content"},     // Path traversal attempt
				{Path: "/etc/test.yaml", Content: "bad content"},      // Absolute path attempt
				{Path: "subdir/../test4.yaml", Content: "content4"},   // Path traversal attempt
				{Path: "./test5.yaml", Content: "content5"},           // Current directory
				{Path: "deep/nested/test6.yaml", Content: "content6"}, // Deep nesting
			},
		}

		body, _ := json.Marshal(req)
		httpReq := httptest.NewRequest("POST", "/files/bulk", bytes.NewBuffer(body))
		httpReq.Header.Set("Authorization", "Bearer test-token")
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleBulkFileOperation(w, httpReq)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}

		var response BulkFileResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}

		if response.Success {
			t.Error("Expected success to be false due to invalid paths")
		}

		// Verify results
		expectedResults := map[string]struct {
			success bool
			content string
			error   string
		}{
			"test1.yaml":             {true, "content1", ""},
			"test2.yaml":             {true, "content2", ""},
			"subdir/test3.yaml":      {true, "content3", ""},
			"../outside.yaml":        {false, "", "Invalid file path: access denied"},
			"/etc/test.yaml":         {false, "", "Invalid file path: access denied"},
			"subdir/../test4.yaml":   {false, "", "Invalid file path: access denied"},
			"./test5.yaml":           {true, "content5", ""},
			"deep/nested/test6.yaml": {true, "content6", ""},
		}

		// Verify each result
		for _, result := range response.Results {
			expected := expectedResults[result.Path]
			if result.Success != expected.success {
				t.Errorf("Path %s: expected success=%v got %v", result.Path, expected.success, result.Success)
			}

			if !result.Success {
				if result.Error != expected.error {
					t.Errorf("Path %s: expected error '%s' got '%s'", result.Path, expected.error, result.Error)
				}
			} else {
				// Verify file was created with correct content
				content, err := os.ReadFile(filepath.Join(tempDir, result.Path))
				if err != nil {
					t.Errorf("Error reading file %s: %v", result.Path, err)
					continue
				}
				if string(content) != expected.content {
					t.Errorf("Path %s: expected content '%s' got '%s'", result.Path, expected.content, string(content))
				}
			}
		}

		// Verify no files were created outside data directory
		outsidePath := filepath.Join(filepath.Dir(tempDir), "outside.yaml")
		if _, err := os.Stat(outsidePath); !os.IsNotExist(err) {
			t.Error("File was created outside data directory")
			os.Remove(outsidePath)
		}
	})

	// Test bulk update
	t.Run("Bulk Update", func(t *testing.T) {
		// Create some files for updating
		filesToCreate := map[string]string{
			"test1.yaml":        "original1",
			"test2.yaml":        "original2",
			"subdir/test3.yaml": "original3",
		}
		for path, content := range filesToCreate {
			fullPath := filepath.Join(tempDir, path)
			os.MkdirAll(filepath.Dir(fullPath), 0755)
			os.WriteFile(fullPath, []byte(content), 0644)
		}

		req := BulkFileRequest{
			Files: []FileRequest{
				{Path: "test1.yaml", Content: "updated1"},
				{Path: "test2.yaml", Content: "updated2"},
				{Path: "subdir/test3.yaml", Content: "updated3"},
				{Path: "nonexistent.yaml", Content: "new content"},
				{Path: "../test.yaml", Content: "bad content"},
				{Path: "/etc/test.yaml", Content: "bad content"},
			},
		}

		body, _ := json.Marshal(req)
		httpReq := httptest.NewRequest("PUT", "/files/bulk", bytes.NewBuffer(body))
		httpReq.Header.Set("Authorization", "Bearer test-token")
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleBulkFileOperation(w, httpReq)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}

		var response BulkFileResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}

		if response.Success {
			t.Error("Expected success to be false due to invalid paths and nonexistent files")
		}

		// Verify results
		expectedResults := map[string]struct {
			success bool
			content string
			error   string
		}{
			"test1.yaml":        {true, "updated1", ""},
			"test2.yaml":        {true, "updated2", ""},
			"subdir/test3.yaml": {true, "updated3", ""},
			"nonexistent.yaml":  {false, "", "File not found"},
			"../test.yaml":      {false, "", "Invalid file path: access denied"},
			"/etc/test.yaml":    {false, "", "Invalid file path: access denied"},
		}

		// Verify each result
		for _, result := range response.Results {
			expected := expectedResults[result.Path]
			if result.Success != expected.success {
				t.Errorf("Path %s: expected success=%v got %v", result.Path, expected.success, result.Success)
			}

			if !result.Success {
				if result.Error != expected.error {
					t.Errorf("Path %s: expected error '%s' got '%s'", result.Path, expected.error, result.Error)
				}
			} else {
				// Verify file was updated with correct content
				content, err := os.ReadFile(filepath.Join(tempDir, result.Path))
				if err != nil {
					t.Errorf("Error reading file %s: %v", result.Path, err)
					continue
				}
				if string(content) != expected.content {
					t.Errorf("Path %s: expected content '%s' got '%s'", result.Path, expected.content, string(content))
				}
			}
		}
	})

	// Test bulk delete
	t.Run("Bulk Delete", func(t *testing.T) {
		// Create some files for deletion
		filesToCreate := map[string]string{
			"test1.yaml":        "content1",
			"test2.yaml":        "content2",
			"subdir/test3.yaml": "content3",
		}
		for path, content := range filesToCreate {
			fullPath := filepath.Join(tempDir, path)
			os.MkdirAll(filepath.Dir(fullPath), 0755)
			os.WriteFile(fullPath, []byte(content), 0644)
		}

		req := struct {
			Files []string `json:"files"`
		}{
			Files: []string{
				"test1.yaml",
				"test2.yaml",
				"subdir/test3.yaml",
				"nonexistent.yaml",
				"../test.yaml",
				"/etc/test.yaml",
				"subdir/../test2.yaml", // Path traversal attempt
			},
		}

		body, _ := json.Marshal(req)
		httpReq := httptest.NewRequest("DELETE", "/files/bulk", bytes.NewBuffer(body))
		httpReq.Header.Set("Authorization", "Bearer test-token")
		httpReq.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handleBulkFileOperation(w, httpReq)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
		}

		var response BulkFileResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatal(err)
		}

		if response.Success {
			t.Error("Expected success to be false due to invalid paths and nonexistent files")
		}

		// Verify results
		expectedResults := map[string]struct {
			success bool
			error   string
		}{
			"test1.yaml":           {true, ""},
			"test2.yaml":           {true, ""},
			"subdir/test3.yaml":    {true, ""},
			"nonexistent.yaml":     {false, "File not found"},
			"../test.yaml":         {false, "Invalid file path: access denied"},
			"/etc/test.yaml":       {false, "Invalid file path: access denied"},
			"subdir/../test2.yaml": {false, "Invalid file path: access denied"},
		}

		// Verify each result
		for _, result := range response.Results {
			expected := expectedResults[result.Path]
			if result.Success != expected.success {
				t.Errorf("Path %s: expected success=%v got %v", result.Path, expected.success, result.Success)
			}

			if !result.Success && result.Error != expected.error {
				t.Errorf("Path %s: expected error '%s' got '%s'", result.Path, expected.error, result.Error)
			}

			if result.Success {
				// Verify file was deleted
				if _, err := os.Stat(filepath.Join(tempDir, result.Path)); !os.IsNotExist(err) {
					t.Errorf("File %s was not deleted", result.Path)
				}
			}
		}
	})
}
