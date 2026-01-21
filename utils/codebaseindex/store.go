package codebaseindex

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	encryptedPrefix = "COMANDA_ENCRYPTED_V1:"
)

// writeOutput writes the index content to the appropriate location(s)
func (m *Manager) writeOutput(content string) (string, error) {
	// Determine output path
	outputPath := m.determineOutputPath()

	// Ensure directory exists
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Handle encryption if enabled
	if m.config.Encrypt {
		if m.config.EncryptionKey == "" {
			return "", fmt.Errorf("encryption enabled but no encryption key provided")
		}

		encPath := outputPath + ".enc"
		if err := encryptToFile([]byte(content), m.config.EncryptionKey, encPath); err != nil {
			return "", fmt.Errorf("failed to encrypt output: %w", err)
		}

		return encPath, nil
	}

	// Write plaintext
	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write output: %w", err)
	}

	// Also write to config location if requested
	if m.config.Store == StoreBoth || m.config.Store == StoreConfig {
		configPath := m.getConfigStorePath()
		if configPath != outputPath {
			configDir := filepath.Dir(configPath)
			if err := os.MkdirAll(configDir, 0755); err != nil {
				m.logf("Warning: failed to create config directory: %v", err)
			} else {
				if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
					m.logf("Warning: failed to write to config store: %v", err)
				}
			}
		}
	}

	return outputPath, nil
}

// determineOutputPath determines the output path based on configuration
func (m *Manager) determineOutputPath() string {
	// Use custom path if specified
	if m.config.OutputPath != "" {
		if filepath.IsAbs(m.config.OutputPath) {
			return m.config.OutputPath
		}
		return filepath.Join(m.config.Root, m.config.OutputPath)
	}

	// Use default based on store location
	switch m.config.Store {
	case StoreConfig:
		return m.getConfigStorePath()
	default: // StoreRepo or StoreBoth
		return filepath.Join(m.config.Root, ".comanda", m.config.RepoFileSlug+"_INDEX.md")
	}
}

// getConfigStorePath returns the path in the user's config directory
func (m *Manager) getConfigStorePath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// Fall back to repo location
		return filepath.Join(m.config.Root, ".comanda", m.config.RepoFileSlug+"_INDEX.md")
	}
	return filepath.Join(homeDir, ".comanda", m.config.RepoFileSlug+"_INDEX.md")
}

// encryptToFile encrypts content and writes to file with .enc extension
func encryptToFile(content []byte, password string, outputPath string) error {
	// Derive key from password using SHA-256
	key := sha256.Sum256([]byte(password))

	// Generate random nonce
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Create cipher
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return fmt.Errorf("failed to create cipher: %w", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("failed to create GCM: %w", err)
	}

	// Encrypt
	ciphertext := aesgcm.Seal(nil, nonce, content, nil)

	// Combine nonce and ciphertext
	encrypted := append(nonce, ciphertext...)

	// Encode as base64 and add prefix
	encodedData := encryptedPrefix + base64.StdEncoding.EncodeToString(encrypted)

	// Write to file
	if err := os.WriteFile(outputPath, []byte(encodedData), 0644); err != nil {
		return fmt.Errorf("failed to write encrypted file: %w", err)
	}

	return nil
}

// DecryptFromFile decrypts content from an encrypted file
func DecryptFromFile(inputPath string, password string) ([]byte, error) {
	// Read encrypted file
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return Decrypt(data, password)
}

// Decrypt decrypts encrypted data
func Decrypt(data []byte, password string) ([]byte, error) {
	// Check for prefix
	dataStr := string(data)
	if len(dataStr) < len(encryptedPrefix) {
		return nil, fmt.Errorf("invalid encrypted data: missing prefix")
	}

	if dataStr[:len(encryptedPrefix)] != encryptedPrefix {
		return nil, fmt.Errorf("invalid encrypted data: wrong prefix")
	}

	// Remove prefix and decode base64
	encodedData := dataStr[len(encryptedPrefix):]
	encrypted, err := base64.StdEncoding.DecodeString(encodedData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}

	// Extract nonce and ciphertext
	if len(encrypted) < 12 {
		return nil, fmt.Errorf("invalid encrypted data: too short")
	}
	nonce := encrypted[:12]
	ciphertext := encrypted[12:]

	// Derive key from password
	key := sha256.Sum256([]byte(password))

	// Create cipher
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt
	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt (wrong password?): %w", err)
	}

	return plaintext, nil
}

// IsEncrypted checks if data appears to be encrypted
func IsEncrypted(data []byte) bool {
	return len(data) >= len(encryptedPrefix) && string(data[:len(encryptedPrefix)]) == encryptedPrefix
}
