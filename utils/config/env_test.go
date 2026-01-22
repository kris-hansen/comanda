package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestEncryptionDecryption(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.env")

	// Test data
	testConfig := &EnvConfig{
		Providers: map[string]*Provider{
			"test": {
				APIKey: "test-key",
				Models: []Model{
					{Name: "test-model", Type: "test"},
				},
			},
		},
	}

	// Save the test config
	if err := SaveEnvConfig(testFile, testConfig); err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Test encryption
	password := "test-password"
	if err := EncryptConfig(testFile, password); err != nil {
		t.Fatalf("Failed to encrypt config: %v", err)
	}

	// Verify file is encrypted
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read encrypted file: %v", err)
	}
	if !IsEncrypted(data) {
		t.Error("File should be marked as encrypted")
	}

	// Test decryption
	decrypted, err := DecryptConfig(data, password)
	if err != nil {
		t.Fatalf("Failed to decrypt config: %v", err)
	}

	// Parse decrypted config
	var loadedConfig EnvConfig
	if err := yaml.Unmarshal(decrypted, &loadedConfig); err != nil {
		t.Fatalf("Failed to parse decrypted config: %v", err)
	}

	// Verify decrypted content
	if loadedConfig.Providers["test"].APIKey != testConfig.Providers["test"].APIKey {
		t.Error("Decrypted config does not match original")
	}

	// Test wrong password
	if _, err := DecryptConfig(data, "wrong-password"); err == nil {
		t.Error("Decryption should fail with wrong password")
	}
}

func TestLoadEncryptedEnvConfig(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.env")

	// Test data
	testConfig := &EnvConfig{
		Providers: map[string]*Provider{
			"test": {
				APIKey: "test-key",
				Models: []Model{
					{Name: "test-model", Type: "test"},
				},
			},
		},
	}

	// Save and encrypt the test config
	if err := SaveEnvConfig(testFile, testConfig); err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	password := "test-password"
	if err := EncryptConfig(testFile, password); err != nil {
		t.Fatalf("Failed to encrypt config: %v", err)
	}

	// Test loading encrypted config
	loadedConfig, err := LoadEncryptedEnvConfig(testFile, password)
	if err != nil {
		t.Fatalf("Failed to load encrypted config: %v", err)
	}

	// Verify loaded config
	if loadedConfig.Providers["test"].APIKey != testConfig.Providers["test"].APIKey {
		t.Error("Loaded config does not match original")
	}

	// Test loading with wrong password
	if _, err := LoadEncryptedEnvConfig(testFile, "wrong-password"); err == nil {
		t.Error("Loading should fail with wrong password")
	}
}

func TestIsEncrypted(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		expected bool
	}{
		{
			name:     "Encrypted data",
			data:     []byte("ENCRYPTED:test"),
			expected: true,
		},
		{
			name:     "Unencrypted data",
			data:     []byte("test"),
			expected: false,
		},
		{
			name:     "Empty data",
			data:     []byte{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsEncrypted(tt.data); got != tt.expected {
				t.Errorf("IsEncrypted() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDeriveKey(t *testing.T) {
	tests := []struct {
		name     string
		password string
	}{
		{
			name:     "Simple password",
			password: "test",
		},
		{
			name:     "Empty password",
			password: "",
		},
		{
			name:     "Long password",
			password: strings.Repeat("a", 100),
		},
		{
			name:     "Special characters",
			password: "!@#$%^&*()",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := deriveKey(tt.password)

			// Verify key length (AES-256 requires 32 bytes)
			if len(key) != 32 {
				t.Errorf("Key length = %v, want 32", len(key))
			}

			// Verify deterministic behavior
			key2 := deriveKey(tt.password)
			if string(key) != string(key2) {
				t.Error("Key derivation is not deterministic")
			}

			// If password is not empty, verify different passwords produce different keys
			if tt.password != "" {
				differentKey := deriveKey(tt.password + "different")
				if string(key) == string(differentKey) {
					t.Error("Different passwords produced same key")
				}
			}
		})
	}
}

func TestLoadEnvConfigWithPassword(t *testing.T) {
	// Create a temporary test file
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.env")

	// Test data
	testConfig := &EnvConfig{
		Providers: map[string]*Provider{
			"test": {
				APIKey: "test-key",
				Models: []Model{
					{Name: "test-model", Type: "test"},
				},
			},
		},
	}

	// Test unencrypted config
	if err := SaveEnvConfig(testFile, testConfig); err != nil {
		t.Fatalf("Failed to save test config: %v", err)
	}

	// Load unencrypted config
	loadedConfig, err := LoadEnvConfig(testFile)
	if err != nil {
		t.Fatalf("Failed to load unencrypted config: %v", err)
	}

	// Verify loaded config
	if loadedConfig.Providers["test"].APIKey != testConfig.Providers["test"].APIKey {
		t.Error("Loaded unencrypted config does not match original")
	}

	// Test non-existent file
	nonExistentFile := filepath.Join(tmpDir, "nonexistent.env")
	if _, err := LoadEnvConfig(nonExistentFile); err == nil {
		t.Error("Loading non-existent file should fail")
	}

	// Test invalid YAML
	invalidFile := filepath.Join(tmpDir, "invalid.env")
	if err := os.WriteFile(invalidFile, []byte("invalid: yaml: content"), 0644); err != nil {
		t.Fatalf("Failed to create invalid test file: %v", err)
	}
	if _, err := LoadEnvConfig(invalidFile); err == nil {
		t.Error("Loading invalid YAML should fail")
	}
}

func TestIsAgenticToolsAllowed(t *testing.T) {
	tests := []struct {
		name     string
		config   *EnvConfig
		expected bool
	}{
		{
			name:     "nil security config defaults to true",
			config:   &EnvConfig{Security: nil},
			expected: true,
		},
		{
			name:     "explicitly enabled",
			config:   &EnvConfig{Security: &SecurityConfig{AllowAgenticTools: true}},
			expected: true,
		},
		{
			name:     "explicitly disabled",
			config:   &EnvConfig{Security: &SecurityConfig{AllowAgenticTools: false}},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.IsAgenticToolsAllowed()
			if result != tt.expected {
				t.Errorf("IsAgenticToolsAllowed() = %v, want %v", result, tt.expected)
			}
		})
	}
}
