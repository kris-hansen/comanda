package models

import (
	"fmt"
	"strings"
)

// AnthropicProvider handles Anthropic family of models
type AnthropicProvider struct {
	apiKey  string
	config  ModelConfig
	verbose bool
}

// NewAnthropicProvider creates a new Anthropic provider instance
func NewAnthropicProvider() *AnthropicProvider {
	return &AnthropicProvider{
		config: ModelConfig{
			Temperature: 0.7,
			MaxTokens:   2000,
			TopP:        1.0,
		},
	}
}

// debugf prints debug information if verbose mode is enabled
func (a *AnthropicProvider) debugf(format string, args ...interface{}) {
	if a.verbose {
		fmt.Printf("[DEBUG][Anthropic] "+format+"\n", args...)
	}
}

// Name returns the provider name
func (a *AnthropicProvider) Name() string {
	return "anthropic"
}

// SupportsModel checks if the given model name is supported by Anthropic
func (a *AnthropicProvider) SupportsModel(modelName string) bool {
	a.debugf("Checking if model is supported: %s", modelName)
	modelName = strings.ToLower(modelName)
	isSupported := strings.HasPrefix(modelName, "claude-")
	a.debugf("Model %s support result: %v", modelName, isSupported)
	return isSupported
}

// Configure sets up the provider with necessary credentials
func (a *AnthropicProvider) Configure(apiKey string) error {
	a.debugf("Configuring Anthropic provider")
	if apiKey == "" {
		return fmt.Errorf("API key is required for Anthropic provider")
	}
	a.apiKey = apiKey
	a.debugf("API key configured successfully")
	return nil
}

// SendPrompt sends a prompt to the specified model and returns the response
func (a *AnthropicProvider) SendPrompt(modelName string, prompt string) (string, error) {
	a.debugf("Preparing to send prompt to model: %s", modelName)
	a.debugf("Prompt length: %d characters", len(prompt))

	if a.apiKey == "" {
		return "", fmt.Errorf("Anthropic provider not configured: missing API key")
	}

	if !a.ValidateModel(modelName) {
		return "", fmt.Errorf("invalid Anthropic model: %s", modelName)
	}

	a.debugf("Model validation passed, preparing API call")
	a.debugf("Using configuration: Temperature=%.2f, MaxTokens=%d, TopP=%.2f",
		a.config.Temperature, a.config.MaxTokens, a.config.TopP)

	// TODO: Implement actual Anthropic API call
	// This would use the Anthropic API client to send the request
	// For now, return a placeholder response
	response := fmt.Sprintf("Response from %s: %s", modelName, prompt)
	a.debugf("API call completed, response length: %d characters", len(response))

	return response, nil
}

// ValidateModel checks if the specific Anthropic model variant is valid
func (a *AnthropicProvider) ValidateModel(modelName string) bool {
	a.debugf("Validating model: %s", modelName)
	validModels := []string{
		"claude-3-opus",
		"claude-3-sonnet",
		"claude-3-5-haiku",
		"claude-3-5-sonnet",
	}

	modelName = strings.ToLower(modelName)
	// Check exact matches
	for _, valid := range validModels {
		if modelName == valid {
			a.debugf("Found exact model match: %s", modelName)
			return true
		}
	}

	// Check model families
	modelFamilies := []string{
		"claude-",
	}

	for _, family := range modelFamilies {
		if strings.HasPrefix(modelName, family) {
			a.debugf("Model %s matches family: %s", modelName, family)
			return true
		}
	}

	a.debugf("Model %s validation failed - no matches found", modelName)
	return false
}

// SetConfig updates the provider configuration
func (a *AnthropicProvider) SetConfig(config ModelConfig) {
	a.debugf("Updating provider configuration")
	a.debugf("Old config: Temperature=%.2f, MaxTokens=%d, TopP=%.2f",
		a.config.Temperature, a.config.MaxTokens, a.config.TopP)
	a.config = config
	a.debugf("New config: Temperature=%.2f, MaxTokens=%d, TopP=%.2f",
		a.config.Temperature, a.config.MaxTokens, a.config.TopP)
}

// GetConfig returns the current provider configuration
func (a *AnthropicProvider) GetConfig() ModelConfig {
	return a.config
}

// SetVerbose enables or disables verbose mode
func (a *AnthropicProvider) SetVerbose(verbose bool) {
	a.verbose = verbose
}
