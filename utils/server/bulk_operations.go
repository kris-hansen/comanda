package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/kris-hansen/comanda/utils/config"
)

// handleBulkFileOperation handles bulk file operations
func (s *Server) handleBulkFileOperation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !checkAuth(s.config, w, r) {
		return
	}

	switch r.Method {
	case http.MethodPost:
		s.handleBulkCreate(w, r)
	case http.MethodPut:
		s.handleBulkUpdate(w, r)
	case http.MethodDelete:
		s.handleBulkDelete(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Method not allowed",
		})
	}
}

// handleBulkCreate handles bulk file creation
func (s *Server) handleBulkCreate(w http.ResponseWriter, r *http.Request) {
	var req BulkFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.VerboseLog("Error decoding request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(BulkFileResponse{
			Success: false,
			Error:   "Invalid request format",
		})
		return
	}

	results := make([]FileResult, 0, len(req.Files))
	success := true

	for _, file := range req.Files {
		result := FileResult{
			Path:    file.Path,
			Success: true,
		}

		// Validate path
		fullPath, err := s.validatePath(file.Path)
		if err != nil {
			result.Success = false
			result.Error = "Invalid file path: access denied"
			success = false
			results = append(results, result)
			continue
		}

		// Create directories if they don't exist
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("Error creating directories: %v", err)
			success = false
			results = append(results, result)
			continue
		}

		// Check if file already exists
		if _, err := os.Stat(fullPath); err == nil {
			result.Success = false
			result.Error = "File already exists"
			success = false
			results = append(results, result)
			continue
		}

		// Write the file
		if err := os.WriteFile(fullPath, []byte(file.Content), 0644); err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("Error writing file: %v", err)
			success = false
		}

		results = append(results, result)
	}

	response := BulkFileResponse{
		Success: success,
		Results: results,
	}
	if !success {
		response.Message = "Some files failed to be created"
	} else {
		response.Message = "All files created successfully"
	}

	json.NewEncoder(w).Encode(response)
}

// handleBulkUpdate handles bulk file updates
func (s *Server) handleBulkUpdate(w http.ResponseWriter, r *http.Request) {
	var req BulkFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.VerboseLog("Error decoding request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(BulkFileResponse{
			Success: false,
			Error:   "Invalid request format",
		})
		return
	}

	results := make([]FileResult, 0, len(req.Files))
	success := true

	for _, file := range req.Files {
		result := FileResult{
			Path:    file.Path,
			Success: true,
		}

		// Validate path
		fullPath, err := s.validatePath(file.Path)
		if err != nil {
			result.Success = false
			result.Error = "Invalid file path: access denied"
			success = false
			results = append(results, result)
			continue
		}

		// Check if file exists
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			result.Success = false
			result.Error = "File not found"
			success = false
			results = append(results, result)
			continue
		}

		// Write the file
		if err := os.WriteFile(fullPath, []byte(file.Content), 0644); err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("Error writing file: %v", err)
			success = false
		}

		results = append(results, result)
	}

	response := BulkFileResponse{
		Success: success,
		Results: results,
	}
	if !success {
		response.Message = "Some files failed to be updated"
	} else {
		response.Message = "All files updated successfully"
	}

	json.NewEncoder(w).Encode(response)
}

// handleBulkDelete handles bulk file deletion
func (s *Server) handleBulkDelete(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Files []string `json:"files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.VerboseLog("Error decoding request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(BulkFileResponse{
			Success: false,
			Error:   "Invalid request format",
		})
		return
	}

	results := make([]FileResult, 0, len(req.Files))
	success := true

	for _, path := range req.Files {
		result := FileResult{
			Path:    path,
			Success: true,
		}

		// Validate path
		fullPath, err := s.validatePath(path)
		if err != nil {
			result.Success = false
			result.Error = "Invalid file path: access denied"
			success = false
			results = append(results, result)
			continue
		}

		// Check if file exists
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			result.Success = false
			result.Error = "File not found"
			success = false
			results = append(results, result)
			continue
		}

		// Delete file
		if err := os.Remove(fullPath); err != nil {
			result.Success = false
			result.Error = fmt.Sprintf("Error deleting file: %v", err)
			success = false
		}

		results = append(results, result)
	}

	response := BulkFileResponse{
		Success: success,
		Results: results,
	}
	if !success {
		response.Message = "Some files failed to be deleted"
	} else {
		response.Message = "All files deleted successfully"
	}

	json.NewEncoder(w).Encode(response)
}
