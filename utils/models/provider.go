package models

import (
	"github.com/kris-hansen/comanda/utils/config"
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

// ResponsesConfig represents configuration for OpenAI Responses API
type ResponsesConfig struct {
	Model              string
	Input              string
	Instructions       string
	PreviousResponseID string
	MaxOutputTokens    int
	Temperature        float64
	TopP               float64
	Stream             bool
	Tools              []map[string]interface{}
	ResponseFormat     map[string]interface{}
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

// ResponsesStreamHandler defines callbacks for streaming responses
type ResponsesStreamHandler interface {
	OnResponseCreated(response map[string]interface{})
	OnResponseInProgress(response map[string]interface{})
	OnOutputItemAdded(index int, item map[string]interface{})
	OnOutputTextDelta(itemID string, index int, contentIndex int, delta string)
	OnResponseCompleted(response map[string]interface{})
	OnError(err error)
}

// ResponsesProvider extends Provider with Responses API capabilities
type ResponsesProvider interface {
	Provider
	SendPromptWithResponses(config ResponsesConfig) (string, error)
	SendPromptWithResponsesStream(config ResponsesConfig, handler ResponsesStreamHandler) error
}

// DetectProviderFunc is the type for the provider detection function
type DetectProviderFunc func(modelName string) Provider

// DetectProvider determines the appropriate provider based on the model name
var DetectProvider DetectProviderFunc = defaultDetectProvider

// defaultDetectProvider is the default implementation of DetectProvider
func defaultDetectProvider(modelName string) Provider {
	config.DebugLog("[Provider] Attempting to detect provider for model: %s", modelName)

	// Order providers from most specific to most general
	providers := []Provider{
		NewGoogleProvider(),    // Handles gemini- models
		NewAnthropicProvider(), // Handles claude- models
		NewXAIProvider(),       // Handles grok- models
		NewDeepseekProvider(),  // Handles deepseek- models
		NewMoonshotProvider(),  // Handles moonshot- models
		NewOpenAIProvider(),    // Handles gpt- models
		NewOllamaProvider(),    // Handles remaining models
	}

	for _, provider := range providers {
		if provider.SupportsModel(modelName) {
			config.DebugLog("[Provider] Found provider %s for model %s", provider.Name(), modelName)
			return provider
		}
	}
	config.DebugLog("[Provider] No provider found for model %s", modelName)
	return nil
}
