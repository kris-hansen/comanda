package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/processor"
	"gopkg.in/yaml.v3"
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

		// For YAML files, always use POST method
		methods := ""
		if strings.HasSuffix(info.Name(), ".yaml") {
			methods = "POST" // All YAML files are processed via POST
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

// handleYAMLUpload handles YAML file uploads via JSON payload
func (s *Server) handleYAMLUpload(w http.ResponseWriter, r *http.Request) {
	if !checkAuth(s.config, w, r) {
		return
	}

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   "Method not allowed",
		})
		return
	}

	var req YAMLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.VerboseLog("Error decoding request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   "Invalid request format",
		})
		return
	}

	// Generate a unique filename for the YAML content
	filename := fmt.Sprintf("upload_%d.yaml", time.Now().UnixNano())
	fullPath, err := s.validatePath(filename)
	if err != nil {
		config.VerboseLog("Invalid path: %v", err)
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(FileResponse{
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
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   fmt.Sprintf("Error creating directories: %v", err),
		})
		return
	}

	// Write the YAML content to file
	if err := os.WriteFile(fullPath, []byte(req.Content), 0644); err != nil {
		config.VerboseLog("Error writing file: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   fmt.Sprintf("Error writing file: %v", err),
		})
		return
	}

	// Get file info for response
	fileInfo, err := s.getFileInfo(fullPath)
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
		Message: "YAML file uploaded successfully",
		File:    fileInfo,
	})
}

// handleYAMLProcess handles YAML file processing
func (s *Server) handleYAMLProcess(w http.ResponseWriter, r *http.Request) {
	if !checkAuth(s.config, w, r) {
		return
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "Method not allowed",
		})
		return
	}

	var req YAMLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.VerboseLog("Error decoding request: %v", err)
		if req.Streaming {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			sseWriter := &sseWriter{w: w, f: w.(http.Flusher)}
			sseWriter.Write([]byte("Error decoding request"))
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   "Invalid request format",
			})
		}
		return
	}

	// First unmarshal into a map to preserve step names
	var rawConfig map[string]processor.StepConfig
	if err := yaml.Unmarshal([]byte(req.Content), &rawConfig); err != nil {
		if req.Streaming {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			flusher, ok := w.(http.Flusher)
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(ProcessResponse{
					Success: false,
					Error:   "Streaming is not supported",
				})
				return
			}
			sseWriter := &sseWriter{w: w, f: flusher}
			sseWriter.SendError(fmt.Errorf("Error parsing YAML: %v", err))
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   fmt.Sprintf("Error parsing YAML: %v", err),
			})
		}
		return
	}

	// Convert map to ordered Steps slice
	var dslConfig processor.DSLConfig
	for name, config := range rawConfig {
		dslConfig.Steps = append(dslConfig.Steps, processor.Step{
			Name:   name,
			Config: config,
		})
	}

	// Create processor instance with validation enabled
	proc := processor.NewProcessor(&dslConfig, s.envConfig, true)

	// Set input if provided
	if req.Input != "" {
		proc.SetLastOutput(req.Input)
	}

	// Check Accept header for streaming
	if r.Header.Get("Accept") == "text/event-stream" {
		req.Streaming = true
	}

	if req.Streaming {
		// Set up SSE streaming
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")

		flusher, ok := w.(http.Flusher)
		if !ok {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   "Streaming is not supported",
			})
			return
		}

		// Create SSE writer
		sseWriter := &sseWriter{w: w, f: flusher}

		// Create progress channel and writer
		progressChan := make(chan processor.ProgressUpdate)
		progressWriter := processor.NewChannelProgressWriter(progressChan)

		// Set up processor with progress writer
		proc.SetProgressWriter(progressWriter)

		// Run the processor in a goroutine
		processDone := make(chan error)
		go func() {
			processDone <- proc.Process()
		}()

		// Start heartbeat ticker
		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()

		// Handle events
		for {
			select {
			case <-r.Context().Done():
				// Client disconnected
				return
			case err := <-processDone:
				if err != nil {
					sseWriter.SendError(err)
				} else {
					sseWriter.SendComplete("Processing complete")
				}
				return
			case update := <-progressChan:
				switch update.Type {
				case processor.ProgressSpinner:
					sseWriter.SendSpinner(update.Message)
				case processor.ProgressStep:
					// Map specific progress messages to complete events
					if update.Message == "DSL processing completed successfully" {
						sseWriter.SendComplete(update.Message)
					} else {
						sseWriter.SendProgress(update.Message)
					}
				case processor.ProgressComplete:
					sseWriter.SendComplete(update.Message)
				case processor.ProgressError:
					sseWriter.SendError(update.Error)
				}
			case <-heartbeat.C:
				sseWriter.SendHeartbeat()
			}
		}
	}

	// Non-streaming behavior
	var buf bytes.Buffer
	pipeReader, pipeWriter, _ := os.Pipe()
	filterWriter := &filteringWriter{
		output: pipeWriter,
		debug:  os.Stdout,
	}

	originalLogOutput := log.Writer()
	log.SetOutput(filterWriter)

	config.DebugLog("Starting DSL processing")

	err := proc.Process()

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		io.Copy(&buf, pipeReader)
	}()

	log.SetOutput(originalLogOutput)
	pipeWriter.Close()
	wg.Wait()

	finalOutput := proc.LastOutput()

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   fmt.Sprintf("Error processing YAML: %v", err),
			Output:  finalOutput,
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ProcessResponse{
		Success: true,
		Message: "YAML processed successfully",
		Output:  finalOutput,
	})
}

// getFileInfo returns metadata about a file
func (s *Server) getFileInfo(path string) (FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return FileInfo{}, err
	}

	// Get relative path from data directory
	relPath, err := filepath.Rel(s.config.DataDir, path)
	if err != nil {
		return FileInfo{}, err
	}

	// For YAML files, always use POST method
	methods := ""
	if strings.HasSuffix(info.Name(), ".yaml") {
		methods = "POST" // All YAML files are processed via POST
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

	// Set appropriate headers
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filepath.Base(fullPath)))

	// Copy file contents to response
	if _, err := io.Copy(w, file); err != nil {
		config.VerboseLog("Error copying file: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(FileResponse{
			Success: false,
			Error:   fmt.Sprintf("Error copying file: %v", err),
		})
		return
	}
}
