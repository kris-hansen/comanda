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
	"gopkg.in/yaml.v3"
)

func TestHandleEncryptEnv(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "comanda-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test environment file
	envPath := filepath.Join(tempDir, ".env")
	testConfig := &config.EnvConfig{
		Providers: map[string]*config.Provider{
			"test": {
				APIKey: "test-key",
			},
		},
	}
	if err := config.SaveEnvConfig(envPath, testConfig); err != nil {
		t.Fatal(err)
	}

	// Set environment variable for test
	os.Setenv("COMANDA_ENV", envPath)
	defer os.Unsetenv("COMANDA_ENV")

	// Create server instance
	server := &Server{
		config: &ServerConfig{
			DataDir:     tempDir,
			BearerToken: "test-token",
			Enabled:     true,
		},
		envConfig: testConfig,
	}

	// Create test request body
	body := EnvironmentRequest{
		Password: "test-password",
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}

	// Create test request
	req := httptest.NewRequest("POST", "/env/encrypt", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Call handler
	server.handleEncryptEnv(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	var response EnvironmentResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	if !response.Success {
		t.Error("Expected success to be true")
	}

	// Verify file was encrypted
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if !config.IsEncrypted(content) {
		t.Error("Expected file to be encrypted")
	}
}

func TestHandleDecryptEnv(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "comanda-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create a test environment file
	envPath := filepath.Join(tempDir, ".env")
	testConfig := &config.EnvConfig{
		Providers: map[string]*config.Provider{
			"test": {
				APIKey: "test-key",
			},
		},
	}
	if err := config.SaveEnvConfig(envPath, testConfig); err != nil {
		t.Fatal(err)
	}

	// Encrypt the file first
	password := "test-password"
	if err := config.EncryptConfig(envPath, password); err != nil {
		t.Fatal(err)
	}

	// Set environment variable for test
	os.Setenv("COMANDA_ENV", envPath)
	defer os.Unsetenv("COMANDA_ENV")

	// Create server instance
	server := &Server{
		config: &ServerConfig{
			DataDir:     tempDir,
			BearerToken: "test-token",
			Enabled:     true,
		},
		envConfig: testConfig,
	}

	// Create test request body
	body := EnvironmentRequest{
		Password: password,
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}

	// Create test request
	req := httptest.NewRequest("POST", "/env/decrypt", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Call handler
	server.handleDecryptEnv(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	var response EnvironmentResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	if !response.Success {
		t.Error("Expected success to be true")
	}

	// Verify file was decrypted
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatal(err)
	}
	if config.IsEncrypted(content) {
		t.Error("Expected file to be decrypted")
	}

	// Verify decrypted content matches original using YAML unmarshaling
	var decryptedConfig config.EnvConfig
	if err := yaml.Unmarshal(content, &decryptedConfig); err != nil {
		t.Fatal(err)
	}

	if decryptedConfig.Providers["test"].APIKey != testConfig.Providers["test"].APIKey {
		t.Error("Decrypted content does not match original")
	}
}

func TestHandleEncryptEnvWithoutPassword(t *testing.T) {
	// Create server instance
	server := &Server{
		config: &ServerConfig{
			BearerToken: "test-token",
			Enabled:     true,
		},
	}

	// Create test request with empty password
	body := EnvironmentRequest{
		Password: "",
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}

	// Create test request
	req := httptest.NewRequest("POST", "/env/encrypt", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Call handler
	server.handleEncryptEnv(w, req)

	// Check response
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, w.Code)
	}

	var response EnvironmentResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	if response.Success {
		t.Error("Expected success to be false")
	}

	if response.Error != "Password is required" {
		t.Errorf("Expected error message 'Password is required', got '%s'", response.Error)
	}
}
