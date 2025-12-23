package config

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// ModelMode represents the supported modes for a model
type ModelMode string

const (
	TextMode   ModelMode = "text"
	VisionMode ModelMode = "vision"
	MultiMode  ModelMode = "multi"
	FileMode   ModelMode = "file" // Added for file handling support
)

// DatabaseType represents supported database types
type DatabaseType string

const (
	PostgreSQL DatabaseType = "postgres"
)

// DatabaseConfig represents a database connection configuration
type DatabaseConfig struct {
	Type     DatabaseType `yaml:"type"`
	Host     string       `yaml:"host"`
	Port     int          `yaml:"port"`
	User     string       `yaml:"user"`
	Password string       `yaml:"password"`
	Database string       `yaml:"database"`
}

// Model represents a single model configuration
type Model struct {
	Name  string      `yaml:"name"`
	Type  string      `yaml:"type"`
	Modes []ModelMode `yaml:"modes"`
}

// Provider represents a provider's configuration
type Provider struct {
	APIKey string  `yaml:"api_key"`
	Models []Model `yaml:"models"`
}

// EnvConfig represents the complete environment configuration
type EnvConfig struct {
	Providers              map[string]*Provider      `yaml:"providers"` // Changed to store pointers to Provider
	Server                 *ServerConfig             `yaml:"server,omitempty"`
	Databases              map[string]DatabaseConfig `yaml:"databases,omitempty"` // Added database configurations
	DefaultGenerationModel string                    `yaml:"default_generation_model,omitempty"`
	MemoryFile             string                    `yaml:"memory_file,omitempty"` // Path to COMANDA.md memory file
}

// Verbose indicates whether verbose logging is enabled
var Verbose bool

// Debug indicates whether debug logging is enabled
var Debug bool

// DebugLog prints debug information if debug mode is enabled
func DebugLog(format string, args ...interface{}) {
	if Debug {
		log.Printf("[DEBUG] "+format+"\n", args...)
	}
}

// VerboseLog prints verbose information if verbose mode is enabled
func VerboseLog(format string, args ...interface{}) {
	if Verbose {
		log.Printf("[VERBOSE] "+format+"\n", args...)
	}
}

// GetComandaDir returns the path to the .comanda directory in the user's home
func GetComandaDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(homeDir, ".comanda"), nil
}

// EnsureComandaDir creates the ~/.comanda directory if it doesn't exist
func EnsureComandaDir() (string, error) {
	comandaDir, err := GetComandaDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(comandaDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create .comanda directory: %w", err)
	}
	return comandaDir, nil
}

// GetEnvPath returns the environment file path, checking in order:
// 1. COMANDA_ENV environment variable (explicit override)
// 2. ~/.comanda/config.yaml (preferred default)
// 3. .env in current directory (legacy fallback)
func GetEnvPath() string {
	// 1. Check for explicit override via environment variable
	if envPath := os.Getenv("COMANDA_ENV"); envPath != "" {
		DebugLog("Using environment file from COMANDA_ENV: %s", envPath)
		return envPath
	}

	// 2. Check for ~/.comanda/config.yaml (preferred default)
	comandaDir, err := GetComandaDir()
	if err == nil {
		configPath := filepath.Join(comandaDir, "config.yaml")
		if _, err := os.Stat(configPath); err == nil {
			DebugLog("Using config file: %s", configPath)
			return configPath
		}
	}

	// 3. Check for legacy .env file in current directory
	if _, err := os.Stat(".env"); err == nil {
		DebugLog("Using legacy environment file: .env")
		return ".env"
	}

	// 4. Default to new location (will be created on first configure)
	if comandaDir != "" {
		return filepath.Join(comandaDir, "config.yaml")
	}

	// Fallback if home dir not available
	DebugLog("Using default environment file: .env")
	return ".env"
}

// PromptPassword prompts the user for a password securely
func PromptPassword(prompt string) (string, error) {
	log.Printf("%s", prompt)
	password, err := term.ReadPassword(int(syscall.Stdin))
	log.Printf("\n") // Add newline after password input
	if err != nil {
		return "", fmt.Errorf("error reading password: %w", err)
	}
	return string(password), nil
}

// deriveKey derives an AES-256 key from a password using SHA-256
func deriveKey(password string) []byte {
	hash := sha256.Sum256([]byte(password))
	return hash[:]
}

// IsEncrypted checks if the file content is encrypted
func IsEncrypted(data []byte) bool {
	return strings.HasPrefix(string(data), "ENCRYPTED:")
}

// EncryptConfig encrypts the configuration file
func EncryptConfig(path string, password string) error {
	DebugLog("Attempting to encrypt configuration at: %s", path)

	// Read the original file
	plaintext, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	// Generate a random nonce
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("error generating nonce: %w", err)
	}

	// Create cipher
	key := deriveKey(password)
	block, err := aes.NewCipher(key)
	if err != nil {
		return fmt.Errorf("error creating cipher: %w", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("error creating GCM: %w", err)
	}

	// Encrypt the data
	ciphertext := aesgcm.Seal(nil, nonce, plaintext, nil)

	// Combine nonce and ciphertext
	encrypted := append(nonce, ciphertext...)

	// Encode as base64 and add prefix
	encodedData := "ENCRYPTED:" + base64.StdEncoding.EncodeToString(encrypted)

	// Write the encrypted data back to the file
	if err := os.WriteFile(path, []byte(encodedData), 0644); err != nil {
		return fmt.Errorf("error writing encrypted file: %w", err)
	}

	DebugLog("Successfully encrypted configuration")
	return nil
}

// DecryptConfig decrypts the configuration data
func DecryptConfig(data []byte, password string) ([]byte, error) {
	DebugLog("Attempting to decrypt configuration data")

	// Remove the "ENCRYPTED:" prefix
	encodedData := strings.TrimPrefix(string(data), "ENCRYPTED:")

	// Decode from base64
	encrypted, err := base64.StdEncoding.DecodeString(encodedData)
	if err != nil {
		return nil, fmt.Errorf("error decoding base64: %w", err)
	}

	// Extract nonce and ciphertext
	if len(encrypted) < 12 {
		return nil, fmt.Errorf("invalid encrypted data")
	}
	nonce := encrypted[:12]
	ciphertext := encrypted[12:]

	// Create cipher
	key := deriveKey(password)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("error creating cipher: %w", err)
	}

	aesgcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("error creating GCM: %w", err)
	}

	// Decrypt the data
	plaintext, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("error decrypting data (wrong password?): %w", err)
	}

	DebugLog("Successfully decrypted configuration")
	return plaintext, nil
}

// LoadEnvConfig loads the environment configuration from .env file
func LoadEnvConfig(path string) (*EnvConfig, error) {
	DebugLog("Attempting to load environment configuration from: %s", path)

	data, err := os.ReadFile(path)
	if err != nil {
		DebugLog("Error reading environment file: %v", err)
		return nil, fmt.Errorf("error reading env file: %w", err)
	}

	if IsEncrypted(data) {
		return nil, fmt.Errorf("encrypted configuration detected: password required")
	}

	var config EnvConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		DebugLog("Error parsing environment file: %v", err)
		return nil, fmt.Errorf("error parsing env file: %w", err)
	}

	// Convert any non-pointer providers to pointers
	if config.Providers != nil {
		for name, provider := range config.Providers {
			if provider != nil {
				// Ensure we're storing a pointer
				config.Providers[name] = provider
			}
		}
	}

	// Initialize databases map if nil
	if config.Databases == nil {
		config.Databases = make(map[string]DatabaseConfig)
	}

	DebugLog("Successfully loaded environment configuration")
	return &config, nil
}

// LoadEncryptedEnvConfig loads an encrypted environment configuration
func LoadEncryptedEnvConfig(path string, password string) (*EnvConfig, error) {
	DebugLog("Attempting to load encrypted environment configuration from: %s", path)

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading env file: %w", err)
	}

	if !IsEncrypted(data) {
		return nil, fmt.Errorf("configuration is not encrypted")
	}

	decrypted, err := DecryptConfig(data, password)
	if err != nil {
		return nil, err
	}

	var config EnvConfig
	if err := yaml.Unmarshal(decrypted, &config); err != nil {
		return nil, fmt.Errorf("error parsing decrypted config: %w", err)
	}

	// Convert any non-pointer providers to pointers
	if config.Providers != nil {
		for name, provider := range config.Providers {
			if provider != nil {
				config.Providers[name] = provider
			}
		}
	}

	// Initialize databases map if nil
	if config.Databases == nil {
		config.Databases = make(map[string]DatabaseConfig)
	}

	return &config, nil
}

// LoadEnvConfigWithPassword attempts to load the config, prompting for password if encrypted
func LoadEnvConfigWithPassword(path string) (*EnvConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &EnvConfig{
				Providers: make(map[string]*Provider),
				Databases: make(map[string]DatabaseConfig),
			}, nil
		}
		return nil, fmt.Errorf("error reading env file: %w", err)
	}

	if IsEncrypted(data) {
		password, err := PromptPassword("Enter decryption password: ")
		if err != nil {
			return nil, err
		}
		return LoadEncryptedEnvConfig(path, password)
	}

	return LoadEnvConfig(path)
}

// SaveEnvConfig saves the environment configuration to .env file
func SaveEnvConfig(path string, config *EnvConfig) error {
	DebugLog("Attempting to save environment configuration to: %s", path)

	data, err := yaml.Marshal(config)
	if err != nil {
		DebugLog("Error marshaling environment config: %v", err)
		return fmt.Errorf("error marshaling env config: %w", err)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			DebugLog("Error creating directory %s: %v", dir, err)
			return fmt.Errorf("error creating directory: %w", err)
		}
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		DebugLog("Error writing environment file: %v", err)
		return fmt.Errorf("error writing env file: %w", err)
	}

	DebugLog("Successfully saved environment configuration")
	return nil
}

// GenerateBearerToken generates a secure random bearer token
func GenerateBearerToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// GetServerConfig retrieves the server configuration
func (c *EnvConfig) GetServerConfig() *ServerConfig {
	if c.Server == nil {
		c.Server = &ServerConfig{
			Port:    8080,
			Enabled: false,
			DataDir: "data", // Initial default, will be made absolute
			CORS: CORS{
				Enabled:        true,
				AllowedOrigins: []string{"*"},
				AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
				AllowedHeaders: []string{"Authorization", "Content-Type"},
				MaxAge:         3600,
			},
		}
	}

	// Make DataDir absolute if it's not already
	if !filepath.IsAbs(c.Server.DataDir) {
		// Get the directory containing the .env file
		envPath := GetEnvPath()
		envDir := filepath.Dir(envPath)

		// Make DataDir absolute relative to the .env file location
		c.Server.DataDir = filepath.Join(envDir, c.Server.DataDir)

		// Create the directory if it doesn't exist
		if err := os.MkdirAll(c.Server.DataDir, 0755); err != nil {
			// Log error but don't fail - the server will handle directory creation as needed
			VerboseLog("Error creating DataDir: %v", err)
		}
	}

	return c.Server
}

// UpdateServerConfig updates the server configuration
func (c *EnvConfig) UpdateServerConfig(serverConfig ServerConfig) {
	if c.Server == nil {
		c.Server = &ServerConfig{}
	}
	c.Server.Port = serverConfig.Port
	c.Server.BearerToken = serverConfig.BearerToken
	c.Server.Enabled = serverConfig.Enabled
	c.Server.DataDir = serverConfig.DataDir
	c.Server.CORS = serverConfig.CORS
}

// GetProviderConfig retrieves configuration for a specific provider
func (c *EnvConfig) GetProviderConfig(providerName string) (*Provider, error) {
	provider, exists := c.Providers[providerName]
	if !exists {
		return nil, fmt.Errorf("provider %s not found in configuration", providerName)
	}
	if provider == nil {
		return nil, fmt.Errorf("provider %s configuration is nil", providerName)
	}
	return provider, nil
}

// AddProvider adds or updates a provider configuration
func (c *EnvConfig) AddProvider(name string, provider Provider) {
	if c.Providers == nil {
		c.Providers = make(map[string]*Provider)
	}
	// Store a pointer to the provider
	providerCopy := provider
	c.Providers[name] = &providerCopy
}

// ValidateModelMode checks if a mode is valid
func ValidateModelMode(mode ModelMode) bool {
	validModes := []ModelMode{TextMode, VisionMode, MultiMode, FileMode}
	for _, validMode := range validModes {
		if mode == validMode {
			return true
		}
	}
	return false
}

// GetSupportedModes returns all supported model modes
func GetSupportedModes() []ModelMode {
	return []ModelMode{TextMode, VisionMode, MultiMode, FileMode}
}

// AddModelToProvider adds a model to a specific provider
func (c *EnvConfig) AddModelToProvider(providerName string, model Model) error {
	provider, exists := c.Providers[providerName]
	if !exists {
		return fmt.Errorf("provider %s not found", providerName)
	}

	// Check if model already exists
	for _, m := range provider.Models {
		if m.Name == model.Name {
			return fmt.Errorf("model %s already exists for provider %s", model.Name, providerName)
		}
	}

	// Validate modes
	for _, mode := range model.Modes {
		if !ValidateModelMode(mode) {
			return fmt.Errorf("invalid model mode: %s", mode)
		}
	}

	provider.Models = append(provider.Models, model)
	return nil
}

// GetModelConfig retrieves configuration for a specific model
func (c *EnvConfig) GetModelConfig(providerName, modelName string) (*Model, error) {
	provider, err := c.GetProviderConfig(providerName)
	if err != nil {
		return nil, err
	}

	var exactMatch *Model
	var baseMatches []*Model

	for _, model := range provider.Models {
		// Check for exact match first
		if model.Name == modelName {
			exactMatch = &model
			break
		}

		// Check if this model matches the base name (before any tag)
		modelBaseName := strings.Split(model.Name, ":")[0]
		if modelBaseName == modelName {
			baseMatches = append(baseMatches, &model)
		}
	}

	// Return exact match if found
	if exactMatch != nil {
		return exactMatch, nil
	}

	// If no exact match but exactly one base name match, use that
	if len(baseMatches) == 1 {
		return baseMatches[0], nil
	}

	// If multiple base name matches, it's ambiguous
	if len(baseMatches) > 1 {
		var modelNames []string
		for _, match := range baseMatches {
			modelNames = append(modelNames, match.Name)
		}
		return nil, fmt.Errorf("ambiguous model name %s for provider %s, matches: %v", modelName, providerName, modelNames)
	}

	return nil, fmt.Errorf("model %s not found for provider %s", modelName, providerName)
}

// UpdateAPIKey updates the API key for a specific provider
func (c *EnvConfig) UpdateAPIKey(providerName, apiKey string) error {
	provider, exists := c.Providers[providerName]
	if !exists {
		return fmt.Errorf("provider %s not found", providerName)
	}

	provider.APIKey = apiKey
	return nil
}

// HasMode checks if a model supports a specific mode
func (m *Model) HasMode(mode ModelMode) bool {
	for _, supportedMode := range m.Modes {
		if supportedMode == mode {
			return true
		}
	}
	return false
}

// UpdateModelModes updates the modes for a specific model
func (c *EnvConfig) UpdateModelModes(providerName, modelName string, modes []ModelMode) error {
	provider, exists := c.Providers[providerName]
	if !exists {
		return fmt.Errorf("provider %s not found", providerName)
	}

	for i, model := range provider.Models {
		if model.Name == modelName {
			// Validate all modes before updating
			for _, mode := range modes {
				if !ValidateModelMode(mode) {
					return fmt.Errorf("invalid model mode: %s", mode)
				}
			}
			provider.Models[i].Modes = modes
			return nil
		}
	}

	return fmt.Errorf("model %s not found for provider %s", modelName, providerName)
}

// GetDatabaseConfig retrieves configuration for a specific database
func (c *EnvConfig) GetDatabaseConfig(name string) (*DatabaseConfig, error) {
	if c.Databases == nil {
		return nil, fmt.Errorf("no databases configured")
	}

	db, exists := c.Databases[name]
	if !exists {
		return nil, fmt.Errorf("database %s not found in configuration", name)
	}

	return &db, nil
}

// AddDatabase adds or updates a database configuration
func (c *EnvConfig) AddDatabase(name string, config DatabaseConfig) {
	if c.Databases == nil {
		c.Databases = make(map[string]DatabaseConfig)
	}
	c.Databases[name] = config
}

// GetConnectionString returns a connection string for the specified database
func (c *DatabaseConfig) GetConnectionString() string {
	switch c.Type {
	case PostgreSQL:
		return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
			c.User, c.Password, c.Host, c.Port, c.Database)
	default:
		return ""
	}
}

// GetAllConfiguredModels returns a list of all configured models across all providers
func (c *EnvConfig) GetAllConfiguredModels() []string {
	var models []string

	if c.Providers == nil {
		return models
	}

	for _, provider := range c.Providers {
		if provider == nil {
			continue
		}
		for _, model := range provider.Models {
			models = append(models, model.Name)
		}
	}

	return models
}
