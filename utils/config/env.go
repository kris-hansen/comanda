package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ModelMode represents the supported modes for a model
type ModelMode string

const (
	TextMode   ModelMode = "text"
	VisionMode ModelMode = "vision"
	MultiMode  ModelMode = "multi"
)

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

// ServerConfig represents the server configuration
type ServerConfig struct {
	Port        int    `yaml:"port"`
	BearerToken string `yaml:"bearer_token,omitempty"`
	Enabled     bool   `yaml:"enabled"`
	DataDir     string `yaml:"data_dir"`
}

// EnvConfig represents the complete environment configuration
type EnvConfig struct {
	Providers map[string]*Provider `yaml:"providers"` // Changed to store pointers to Provider
	Server    *ServerConfig        `yaml:"server,omitempty"`
}

// Verbose indicates whether verbose logging is enabled
var Verbose bool

// debugLog prints debug information if verbose mode is enabled
func debugLog(format string, args ...interface{}) {
	if Verbose {
		fmt.Printf("[DEBUG] "+format+"\n", args...)
	}
}

// GetEnvPath returns the environment file path from COMANDA_ENV or the default
func GetEnvPath() string {
	if envPath := os.Getenv("COMANDA_ENV"); envPath != "" {
		debugLog("Using environment file from COMANDA_ENV: %s", envPath)
		return envPath
	}
	debugLog("Using default environment file: .env")
	return ".env"
}

// LoadEnvConfig loads the environment configuration from .env file
func LoadEnvConfig(path string) (*EnvConfig, error) {
	debugLog("Attempting to load environment configuration from: %s", path)

	data, err := os.ReadFile(path)
	if err != nil {
		debugLog("Error reading environment file: %v", err)
		return nil, fmt.Errorf("error reading env file: %w", err)
	}

	var config EnvConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		debugLog("Error parsing environment file: %v", err)
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

	debugLog("Successfully loaded environment configuration")
	return &config, nil
}

// SaveEnvConfig saves the environment configuration to .env file
func SaveEnvConfig(path string, config *EnvConfig) error {
	debugLog("Attempting to save environment configuration to: %s", path)

	data, err := yaml.Marshal(config)
	if err != nil {
		debugLog("Error marshaling environment config: %v", err)
		return fmt.Errorf("error marshaling env config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		debugLog("Error writing environment file: %v", err)
		return fmt.Errorf("error writing env file: %w", err)
	}

	debugLog("Successfully saved environment configuration")
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
			DataDir: "data",
		}
	}
	return c.Server
}

// UpdateServerConfig updates the server configuration
func (c *EnvConfig) UpdateServerConfig(config ServerConfig) {
	if c.Server == nil {
		c.Server = &ServerConfig{}
	}
	c.Server.Port = config.Port
	c.Server.BearerToken = config.BearerToken
	c.Server.Enabled = config.Enabled
	c.Server.DataDir = config.DataDir
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
	validModes := []ModelMode{TextMode, VisionMode, MultiMode}
	for _, validMode := range validModes {
		if mode == validMode {
			return true
		}
	}
	return false
}

// GetSupportedModes returns all supported model modes
func GetSupportedModes() []ModelMode {
	return []ModelMode{TextMode, VisionMode, MultiMode}
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

	for _, model := range provider.Models {
		if model.Name == modelName {
			return &model, nil
		}
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
