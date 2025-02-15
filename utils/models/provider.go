package models

import (
	"log"
)

// ModelConfig represents configuration options for model calls
type ModelConfig struct {
	Temperature         float64
	MaxTokens           int
	MaxCompletionTokens int
	TopP                float64
}

// FileInput represents a file to be processed by the model
type FileInput struct {
	Path     string
	MimeType string
}

// Provider represents a model provider (e.g., Anthropic, OpenAI)
type Provider interface {
	Name() string
	SupportsModel(modelName string) bool
	SendPrompt(modelName string, prompt string) (string, error)
	SendPromptWithFile(modelName string, prompt string, file FileInput) (string, error)
	Configure(apiKey string) error
	SetVerbose(verbose bool)
}

// DetectProviderFunc is the type for the provider detection function
type DetectProviderFunc func(modelName string) Provider

// DetectProvider determines the appropriate provider based on the model name
var DetectProvider DetectProviderFunc = defaultDetectProvider

// defaultDetectProvider is the default implementation of DetectProvider
func defaultDetectProvider(modelName string) Provider {
	log.Printf("[DEBUG][Provider] Attempting to detect provider for model: %s", modelName)

	// Order providers from most specific to most general
	providers := []Provider{
		NewGoogleProvider(),    // Handles gemini- models
		NewAnthropicProvider(), // Handles claude- models
		NewXAIProvider(),       // Handles grok- models
		NewDeepseekProvider(),  // Handles deepseek- models
		NewOpenAIProvider(),    // Handles gpt- models
		NewOllamaProvider(),    // Handles remaining models
	}

	for _, provider := range providers {
		//log.Printf("[DEBUG][Provider] Checking if %s supports model %s", provider.Name(), modelName)
		if provider.SupportsModel(modelName) {
			log.Printf("[DEBUG][Provider] Found provider %s for model %s", provider.Name(), modelName)
			return provider
		}
	}
	log.Printf("[DEBUG][Provider] No provider found for model %s", modelName)
	return nil
}
