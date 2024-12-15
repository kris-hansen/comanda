package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/kris-hansen/comanda/utils/config"
)

// handleEncryptEnv handles environment file encryption
func (s *Server) handleEncryptEnv(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !checkAuth(s.config, w, r) {
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(EnvironmentResponse{
			Success: false,
			Error:   "Invalid request format",
		})
		return
	}

	if req.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(EnvironmentResponse{
			Success: false,
			Error:   "Password is required",
		})
		return
	}

	// Get environment file path
	envPath := config.GetEnvPath()

	// Create directory if it doesn't exist
	if err := os.MkdirAll(s.config.DataDir, 0755); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(EnvironmentResponse{
			Success: false,
			Error:   fmt.Sprintf("Error creating directory: %v", err),
		})
		return
	}

	// Encrypt the file
	if err := config.EncryptConfig(envPath, req.Password); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(EnvironmentResponse{
			Success: false,
			Error:   fmt.Sprintf("Error encrypting file: %v", err),
		})
		return
	}

	json.NewEncoder(w).Encode(EnvironmentResponse{
		Success: true,
		Message: "Environment file encrypted successfully",
	})
}

// handleDecryptEnv handles environment file decryption
func (s *Server) handleDecryptEnv(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !checkAuth(s.config, w, r) {
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(EnvironmentResponse{
			Success: false,
			Error:   "Invalid request format",
		})
		return
	}

	if req.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(EnvironmentResponse{
			Success: false,
			Error:   "Password is required",
		})
		return
	}

	// Get environment file path
	envPath := config.GetEnvPath()

	// Read encrypted file
	data, err := os.ReadFile(envPath)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(EnvironmentResponse{
			Success: false,
			Error:   fmt.Sprintf("Error reading file: %v", err),
		})
		return
	}

	// Verify file is encrypted
	if !config.IsEncrypted(data) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(EnvironmentResponse{
			Success: false,
			Error:   "File is not encrypted",
		})
		return
	}

	// Decrypt the data
	decrypted, err := config.DecryptConfig(data, req.Password)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(EnvironmentResponse{
			Success: false,
			Error:   fmt.Sprintf("Error decrypting file: %v", err),
		})
		return
	}

	// Write decrypted data back to file
	if err := os.WriteFile(envPath, decrypted, 0644); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(EnvironmentResponse{
			Success: false,
			Error:   fmt.Sprintf("Error writing file: %v", err),
		})
		return
	}

	json.NewEncoder(w).Encode(EnvironmentResponse{
		Success: true,
		Message: "Environment file decrypted successfully",
	})
}
