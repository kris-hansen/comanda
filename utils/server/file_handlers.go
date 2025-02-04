package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/kris-hansen/comanda/utils/config"
)

// handleListFiles returns a list of files with detailed metadata
func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !checkAuth(s.config, w, r) {
		return
	}

	config.VerboseLog("Listing files in data directory")
	config.DebugLog("Scanning directory: %s", s.config.DataDir)

	files, err := s.listFilesWithMetadata(s.config.DataDir)
	if err != nil {
		config.VerboseLog("Error listing files: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ListResponse{
			Success: false,
			Error:   fmt.Sprintf("Error listing files: %v", err),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ListResponse{
		Success: true,
		Files:   files,
	})
}

// listFilesWithMetadata returns detailed information about files in a directory
func (s *Server) listFilesWithMetadata(dir string) ([]FileInfo, error) {
	var files []FileInfo

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the directory itself
		if path == dir {
			return nil
		}

		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		// For YAML files, determine if they require STDIN
		methods := ""
		if strings.HasSuffix(info.Name(), ".yaml") {
			content, err := os.ReadFile(path)
			if err == nil {
				if hasStdinInput(content) {
					methods = "POST"
				} else {
					methods = "GET"
				}
			}
		}

		files = append(files, FileInfo{
			Name:       info.Name(),
			Path:       relPath,
			Size:       info.Size(),
			IsDir:      info.IsDir(),
			CreatedAt:  info.ModTime(), // Note: CreatedAt falls back to ModTime on some systems
			ModifiedAt: info.ModTime(),
			Methods:    methods,
		})

		return nil
	})

	return files, err
}

// handleFileOperation handles file operations (create, update, delete)
func (s *Server) handleFileOperation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !checkAuth(s.config, w, r) {
		return
	}

	var filePath string
	var content string

	if r.Method == http.MethodDelete {
		filePath = r.URL.Query().Get("path")
		// For delete operations, empty path is not allowed
		if filePath == "" {
			config.VerboseLog("Empty path parameter")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(FileResponse{
				Success: false,
				Error:   "Path parameter is required",
			})
			return
		}
	} else {
		var req FileRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			config.VerboseLog("Error decoding request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(FileResponse{
				Success: false,
				Error:   "Invalid request format",
			})
			return
		}
		filePath = req.Path
		content = req.Content

		// For create/update operations, empty path means create in root with default name
		if filePath == "" {
			filePath = "file.txt"
		}
	}

	// Validate path before any cleaning or manipulation
	if strings.Contains(filePath, "../") || strings.Contains(filePath, "..\\") {
		config.VerboseLog("Path traversal attempt: %s", filePath)
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   "Invalid file path: access denied",
		})
		return
	}

	// Clean the path and check for empty result
	cleanPath := filepath.Clean(filePath)
	if cleanPath == "." {
		cleanPath = "file.txt"
	}

	// Validate cleaned path
	fullPath, err := s.validatePath(cleanPath)
	if err != nil {
		config.VerboseLog("Invalid path: %v", err)
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   "Invalid file path: access denied",
		})
		return
	}

	switch r.Method {
	case http.MethodPost:
		s.handleCreateFile(w, r, fullPath, content)
	case http.MethodPut:
		s.handleUpdateFile(w, r, fullPath, content)
	case http.MethodDelete:
		s.handleDeleteFile(w, r, fullPath)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   "Method not allowed",
		})
	}
}

// handleCreateFile handles file creation
func (s *Server) handleCreateFile(w http.ResponseWriter, r *http.Request, path string, content string) {
	// Check if file already exists first
	if _, err := os.Stat(path); err == nil {
		config.VerboseLog("File already exists: %s", path)
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   "File already exists",
		})
		return
	} else if !os.IsNotExist(err) {
		// Handle other errors
		config.VerboseLog("Error checking file: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   fmt.Sprintf("Error checking file: %v", err),
		})
		return
	}

	// Create directories if they don't exist
	dirPath := filepath.Dir(path)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		config.VerboseLog("Error creating directories: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   fmt.Sprintf("Error creating directories: %v", err),
		})
		return
	}

	// Write the file
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		config.VerboseLog("Error writing file: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   fmt.Sprintf("Error writing file: %v", err),
		})
		return
	}

	// Get file info for response
	fileInfo, err := s.getFileInfo(path)
	if err != nil {
		config.VerboseLog("Error getting file info: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   fmt.Sprintf("Error getting file info: %v", err),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(FileResponse{
		Success: true,
		Message: "File created successfully",
		File:    fileInfo,
	})
}

// handleUpdateFile handles file updates
func (s *Server) handleUpdateFile(w http.ResponseWriter, r *http.Request, path string, content string) {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		config.VerboseLog("File not found: %s", path)
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   "File not found",
		})
		return
	}

	// Write the file
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		config.VerboseLog("Error writing file: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   fmt.Sprintf("Error writing file: %v", err),
		})
		return
	}

	// Get file info for response
	fileInfo, err := s.getFileInfo(path)
	if err != nil {
		config.VerboseLog("Error getting file info: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   fmt.Sprintf("Error getting file info: %v", err),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(FileResponse{
		Success: true,
		Message: "File updated successfully",
		File:    fileInfo,
	})
}

// handleDeleteFile handles file deletion
func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request, path string) {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		config.VerboseLog("File not found: %s", path)
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   "File not found",
		})
		return
	}

	// Delete the file
	if err := os.Remove(path); err != nil {
		config.VerboseLog("Error deleting file: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   fmt.Sprintf("Error deleting file: %v", err),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(FileResponse{
		Success: true,
		Message: "File deleted successfully",
	})
}

// handleGetFileContent handles retrieving file content
func (s *Server) handleGetFileContent(w http.ResponseWriter, r *http.Request) {
	if !checkAuth(s.config, w, r) {
		return
	}

	// Get path from query parameters
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		config.VerboseLog("Missing path parameter")
		config.DebugLog("Content request failed: no path provided")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   "path parameter is required",
		})
		return
	}

	config.DebugLog("Processing content request for file: %s", filePath)

	// Validate path
	fullPath, err := s.validatePath(filePath)
	if err != nil {
		config.VerboseLog("Invalid path: %v", err)
		config.DebugLog("Path validation failed: %v", err)
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   "Invalid file path: access denied",
		})
		return
	}

	config.DebugLog("Validated path: %s", fullPath)

	// Check if file exists and get info
	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			config.VerboseLog("File not found: %s", fullPath)
			config.DebugLog("File not found at path: %s", fullPath)
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(FileResponse{
				Success: false,
				Error:   "File not found",
			})
			return
		}
		config.VerboseLog("Error accessing file: %v", err)
		config.DebugLog("File access error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   fmt.Sprintf("Error accessing file: %v", err),
		})
		return
	}

	// Don't allow directory content retrieval
	if fileInfo.IsDir() {
		config.VerboseLog("Cannot retrieve content of directory: %s", fullPath)
		config.DebugLog("Directory content request rejected: %s", fullPath)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   "Cannot retrieve content of a directory",
		})
		return
	}

	// Read file content
	content, err := os.ReadFile(fullPath)
	if err != nil {
		config.VerboseLog("Error reading file: %v", err)
		config.DebugLog("File read error: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   fmt.Sprintf("Error reading file: %v", err),
		})
		return
	}

	config.DebugLog("Successfully read file content, size: %d bytes", len(content))

	// Set content type header based on file extension
	contentType := "text/plain"
	if strings.HasSuffix(fullPath, ".json") {
		contentType = "application/json"
	} else if strings.HasSuffix(fullPath, ".yaml") || strings.HasSuffix(fullPath, ".yml") {
		contentType = "application/yaml"
	}
	w.Header().Set("Content-Type", contentType)

	// Write content directly to response
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

// handleFileUpload handles file uploads via multipart/form-data
func (s *Server) handleFileUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(FileUploadResponse{
			Success: false,
			Error:   "Method not allowed",
		})
		return
	}

	// Parse multipart form with 32MB limit
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		config.VerboseLog("Error parsing multipart form: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(FileUploadResponse{
			Success: false,
			Error:   "Error parsing form data",
		})
		return
	}

	// Get file from form
	file, header, err := r.FormFile("file")
	if err != nil {
		config.VerboseLog("Error getting file from form: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(FileUploadResponse{
			Success: false,
			Error:   "No file provided",
		})
		return
	}
	defer file.Close()

	// Get path from form or use filename
	filePath := r.FormValue("path")
	if filePath == "" {
		filePath = header.Filename
	}

	// Validate path
	fullPath, err := s.validatePath(filePath)
	if err != nil {
		config.VerboseLog("Invalid path: %v", err)
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(FileUploadResponse{
			Success: false,
			Error:   fmt.Sprintf("Invalid file path: %v", err),
		})
		return
	}

	// Create directories if they don't exist
	dirPath := filepath.Dir(fullPath)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		config.VerboseLog("Error creating directories: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(FileUploadResponse{
			Success: false,
			Error:   fmt.Sprintf("Error creating directories: %v", err),
		})
		return
	}

	// Create destination file
	dst, err := os.Create(fullPath)
	if err != nil {
		config.VerboseLog("Error creating file: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(FileUploadResponse{
			Success: false,
			Error:   fmt.Sprintf("Error creating file: %v", err),
		})
		return
	}
	defer dst.Close()

	// Copy file contents
	if _, err := io.Copy(dst, file); err != nil {
		config.VerboseLog("Error copying file: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(FileUploadResponse{
			Success: false,
			Error:   fmt.Sprintf("Error saving file: %v", err),
		})
		return
	}

	// Get file info for response
	fileInfo, err := s.getFileInfo(fullPath)
	if err != nil {
		config.VerboseLog("Error getting file info: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(FileUploadResponse{
			Success: false,
			Error:   fmt.Sprintf("Error getting file info: %v", err),
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(FileUploadResponse{
		Success: true,
		Message: "File uploaded successfully",
		File:    fileInfo,
	})
}

// handleFileDownload handles file downloads
func (s *Server) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   "Method not allowed",
		})
		return
	}

	// Get path from query parameters
	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		config.VerboseLog("Missing path parameter")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   "path parameter is required",
		})
		return
	}

	// Validate path
	fullPath, err := s.validatePath(filePath)
	if err != nil {
		config.VerboseLog("Invalid path: %v", err)
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   fmt.Sprintf("Invalid file path: %v", err),
		})
		return
	}

	// Check if file exists and get info
	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			config.VerboseLog("File not found: %s", fullPath)
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(FileResponse{
				Success: false,
				Error:   "File not found",
			})
			return
		}
		config.VerboseLog("Error accessing file: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   fmt.Sprintf("Error accessing file: %v", err),
		})
		return
	}

	// Don't allow directory downloads
	if fileInfo.IsDir() {
		config.VerboseLog("Cannot download directory: %s", fullPath)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   "Cannot download a directory",
		})
		return
	}

	// Open the file
	file, err := os.Open(fullPath)
	if err != nil {
		config.VerboseLog("Error opening file: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   fmt.Sprintf("Error opening file: %v", err),
		})
		return
	}
	defer file.Close()

	// Set headers for file download
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filepath.Base(filePath)))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))

	// Stream the file
	if _, err := io.Copy(w, file); err != nil {
		config.VerboseLog("Error streaming file: %v", err)
		// Can't write error response here as headers are already sent
		return
	}
}

// getFileInfo returns detailed information about a file
func (s *Server) getFileInfo(path string) (FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return FileInfo{}, err
	}

	relPath, err := filepath.Rel(s.config.DataDir, path)
	if err != nil {
		return FileInfo{}, err
	}

	// For YAML files, determine if they require STDIN
	methods := ""
	if strings.HasSuffix(info.Name(), ".yaml") {
		content, err := os.ReadFile(path)
		if err == nil {
			if hasStdinInput(content) {
				methods = "POST"
			} else {
				methods = "GET"
			}
		}
	}

	return FileInfo{
		Name:       info.Name(),
		Path:       relPath,
		Size:       info.Size(),
		IsDir:      info.IsDir(),
		CreatedAt:  info.ModTime(), // Note: CreatedAt falls back to ModTime on some systems
		ModifiedAt: info.ModTime(),
		Methods:    methods,
	}, nil
}
