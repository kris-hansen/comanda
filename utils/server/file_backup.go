package server

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kris-hansen/comanda/utils/config"
)

// handleFileBackup handles creating a backup of files
func (s *Server) handleFileBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if !checkAuth(s.config, w, r) {
		return
	}

	// Create backup directory if it doesn't exist
	backupDir := filepath.Join(s.config.DataDir, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		config.VerboseLog("Error creating backup directory: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(BackupResponse{
			Success: false,
			Error:   fmt.Sprintf("Error creating backup directory: %v", err),
		})
		return
	}

	// Create backup file
	timestamp := time.Now().Format("20060102-150405")
	backupName := fmt.Sprintf("backup-%s.zip", timestamp)
	backupPath := filepath.Join(backupDir, backupName)

	zipFile, err := os.Create(backupPath)
	if err != nil {
		config.VerboseLog("Error creating backup file: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(BackupResponse{
			Success: false,
			Error:   fmt.Sprintf("Error creating backup file: %v", err),
		})
		return
	}
	defer zipFile.Close()

	// Create zip writer
	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Walk through data directory
	err = filepath.Walk(s.config.DataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip backup directory
		if path == backupDir || strings.HasPrefix(path, backupDir) {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(s.config.DataDir, path)
		if err != nil {
			return err
		}

		// Skip if this is the root directory
		if relPath == "." {
			return nil
		}

		// Create zip entry
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		header.Name = relPath
		header.Method = zip.Deflate

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			_, err = io.Copy(writer, file)
			if err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		config.VerboseLog("Error creating backup: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(BackupResponse{
			Success: false,
			Error:   fmt.Sprintf("Error creating backup: %v", err),
		})
		return
	}

	json.NewEncoder(w).Encode(BackupResponse{
		Success:  true,
		Message:  "Backup created successfully",
		Filename: backupName,
	})
}

// handleFileRestore handles restoring files from a backup
func (s *Server) handleFileRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if !checkAuth(s.config, w, r) {
		return
	}

	var req RestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.VerboseLog("Error decoding request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(BackupResponse{
			Success: false,
			Error:   "Invalid request format",
		})
		return
	}

	// Validate backup name
	if req.Backup == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(BackupResponse{
			Success: false,
			Error:   "Backup name is required",
		})
		return
	}

	// Check for path traversal or absolute paths in backup name
	if strings.Contains(req.Backup, "..") || strings.Contains(req.Backup, "/") || strings.Contains(req.Backup, "\\") || filepath.IsAbs(req.Backup) {
		config.VerboseLog("Invalid backup path: %s", req.Backup)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(BackupResponse{
			Success: false,
			Error:   "Invalid backup path",
		})
		return
	}

	// Get full backup path
	fullBackupPath := filepath.Join(s.config.DataDir, "backups", req.Backup)

	// Check if the backup exists
	if _, err := os.Stat(fullBackupPath); os.IsNotExist(err) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(BackupResponse{
			Success: false,
			Error:   "Backup file not found",
		})
		return
	}

	// Open zip file
	zipReader, err := zip.OpenReader(fullBackupPath)
	if err != nil {
		config.VerboseLog("Error opening backup file: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(BackupResponse{
			Success: false,
			Error:   fmt.Sprintf("Error opening backup file: %v", err),
		})
		return
	}
	defer zipReader.Close()

	// Extract files
	for _, file := range zipReader.File {
		// Validate file path
		fullPath, err := s.validatePath(file.Name)
		if err != nil {
			config.VerboseLog("Path traversal attempt: %s", file.Name)
			w.WriteHeader(http.StatusForbidden)
			json.NewEncoder(w).Encode(BackupResponse{
				Success: false,
				Error:   "Invalid file path: access denied",
			})
			return
		}

		if file.FileInfo().IsDir() {
			os.MkdirAll(fullPath, file.Mode())
			continue
		}

		// Create directory for file if needed
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			config.VerboseLog("Error creating directory: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(BackupResponse{
				Success: false,
				Error:   fmt.Sprintf("Error creating directory: %v", err),
			})
			return
		}

		// Create file
		outFile, err := os.OpenFile(fullPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			config.VerboseLog("Error creating file: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(BackupResponse{
				Success: false,
				Error:   fmt.Sprintf("Error creating file: %v", err),
			})
			return
		}

		// Open zip file
		rc, err := file.Open()
		if err != nil {
			outFile.Close()
			config.VerboseLog("Error opening zip file: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(BackupResponse{
				Success: false,
				Error:   fmt.Sprintf("Error opening zip file: %v", err),
			})
			return
		}

		// Copy content
		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()

		if err != nil {
			config.VerboseLog("Error copying file: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(BackupResponse{
				Success: false,
				Error:   fmt.Sprintf("Error copying file: %v", err),
			})
			return
		}
	}

	json.NewEncoder(w).Encode(BackupResponse{
		Success: true,
		Message: "Files restored successfully",
	})
}
