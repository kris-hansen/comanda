package models

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

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

// OllamaTagsResponse represents the response from Ollama's /api/tags endpoint
type OllamaTagsResponse struct {
	Models []OllamaModelTag `json:"models"`
}

// OllamaModelTag represents a single model tag from Ollama
type OllamaModelTag struct {
	Name string `json:"name"`
}

// isModelAvailableLocally checks if a model is available in the local Ollama instance
func isModelAvailableLocally(modelName string) bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil {
		config.DebugLog("[Provider] Failed to connect to Ollama: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		config.DebugLog("[Provider] Ollama API returned status %d", resp.StatusCode)
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		config.DebugLog("[Provider] Failed to read Ollama response: %v", err)
		return false
	}

	var tagsResponse OllamaTagsResponse
	if err := json.Unmarshal(body, &tagsResponse); err != nil {
		config.DebugLog("[Provider] Failed to parse Ollama response: %v", err)
		return false
	}

	modelNameLower := strings.ToLower(modelName)
	for _, model := range tagsResponse.Models {
		modelFullName := strings.ToLower(model.Name)

		// First check exact match
		if modelFullName == modelNameLower {
			config.DebugLog("[Provider] Found local model (exact match): %s", modelName)
			return true
		}

		// Then check if the requested model matches the base name (before :tag)
		// e.g., "gpt-oss" should match "gpt-oss:latest"
		if strings.Contains(modelFullName, ":") {
			baseName := strings.Split(modelFullName, ":")[0]
			if baseName == modelNameLower {
				config.DebugLog("[Provider] Found local model (tag match): %s -> %s", modelName, model.Name)
				return true
			}
		}

		// Also check if the full model name starts with the requested name
		// e.g., "llama3" should match "llama3.2:latest"
		if strings.HasPrefix(modelFullName, modelNameLower) {
			// Make sure we're not matching partial names unintentionally
			nextChar := modelFullName[len(modelNameLower):]
			if strings.HasPrefix(nextChar, ":") || strings.HasPrefix(nextChar, ".") {
				config.DebugLog("[Provider] Found local model (prefix match): %s -> %s", modelName, model.Name)
				return true
			}
		}
	}

	config.DebugLog("[Provider] Model %s not found locally", modelName)
	return false
}

// VLLMModelsResponse represents the response from vLLM's /v1/models endpoint
type VLLMModelsResponse struct {
	Data []VLLMModelInfo `json:"data"`
}

// VLLMModelInfo represents a single model from vLLM
type VLLMModelInfo struct {
	ID string `json:"id"`
}

// isModelAvailableOnVLLM checks if a model is available on the local vLLM instance
func isModelAvailableOnVLLM(modelName string) bool {
	// Get vLLM endpoint from environment or use default
	endpoint := "http://localhost:8000"
	// Note: endpoint customization can be done via VLLM_ENDPOINT environment variable
	// which is read in the VLLMProvider itself

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(endpoint + "/v1/models")
	if err != nil {
		config.DebugLog("[Provider] Failed to connect to vLLM: %v", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		config.DebugLog("[Provider] vLLM API returned status %d", resp.StatusCode)
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		config.DebugLog("[Provider] Failed to read vLLM response: %v", err)
		return false
	}

	var modelsResponse VLLMModelsResponse
	if err := json.Unmarshal(body, &modelsResponse); err != nil {
		config.DebugLog("[Provider] Failed to parse vLLM response: %v", err)
		return false
	}

	modelNameLower := strings.ToLower(modelName)
	for _, model := range modelsResponse.Data {
		modelIDLower := strings.ToLower(model.ID)

		// Check for exact match
		if modelIDLower == modelNameLower {
			config.DebugLog("[Provider] Found vLLM model (exact match): %s", modelName)
			return true
		}
	}

	config.DebugLog("[Provider] Model %s not found on vLLM server", modelName)
	return false
}

// DetectProviderFunc is the type for the provider detection function
type DetectProviderFunc func(modelName string) Provider

// DetectProvider determines the appropriate provider based on the model name
var DetectProvider DetectProviderFunc = defaultDetectProvider

// defaultDetectProvider is the default implementation of DetectProvider
func defaultDetectProvider(modelName string) Provider {
	config.DebugLog("[Provider] Attempting to detect provider for model: %s", modelName)

	// First, check local providers (Ollama and vLLM)
	// This prioritizes local models over third-party providers

	// Check Ollama
	ollamaProvider := NewOllamaProvider()
	if ollamaProvider.SupportsModel(modelName) {
		// Check if the model actually exists locally
		if isModelAvailableLocally(modelName) {
			config.DebugLog("[Provider] Found local Ollama provider for model %s", modelName)
			return ollamaProvider
		}
	}

	// Check vLLM
	vllmProvider := NewVLLMProvider()
	if vllmProvider.SupportsModel(modelName) {
		// Check if the model actually exists on vLLM server
		if isModelAvailableOnVLLM(modelName) {
			config.DebugLog("[Provider] Found local vLLM provider for model %s", modelName)
			return vllmProvider
		}
	}

	// Check Claude Code (local CLI)
	claudeCodeProvider := NewClaudeCodeProvider()
	if claudeCodeProvider.SupportsModel(modelName) {
		// Check if the claude binary is available
		if IsClaudeCodeAvailable() {
			config.DebugLog("[Provider] Found local Claude Code provider for model %s", modelName)
			return claudeCodeProvider
		}
		// Model is a claude-code model but binary not found - return nil to give clear error
		// rather than falling through to Ollama which would give a confusing error
		config.DebugLog("[Provider] Model %s requires Claude Code CLI but 'claude' binary not found in PATH", modelName)
		return nil
	}

	// Check Gemini CLI (local CLI)
	geminiCLIProvider := NewGeminiCLIProvider()
	if geminiCLIProvider.SupportsModel(modelName) {
		// Check if the gemini binary is available
		if IsGeminiCLIAvailable() {
			config.DebugLog("[Provider] Found local Gemini CLI provider for model %s", modelName)
			return geminiCLIProvider
		}
		// Model is a gemini-cli model but binary not found - return nil to give clear error
		config.DebugLog("[Provider] Model %s requires Gemini CLI but 'gemini' binary not found in PATH", modelName)
		return nil
	}

	// Order third-party providers from most specific to most general
	providers := []Provider{
		NewGoogleProvider(),    // Handles gemini- models
		NewAnthropicProvider(), // Handles claude- models
		NewXAIProvider(),       // Handles grok- models
		NewDeepseekProvider(),  // Handles deepseek- models
		NewMoonshotProvider(),  // Handles moonshot- models
		NewOpenAIProvider(),    // Handles gpt- models
	}

	for _, provider := range providers {
		if provider.SupportsModel(modelName) {
			config.DebugLog("[Provider] Found provider %s for model %s", provider.Name(), modelName)
			return provider
		}
	}

	// If no third-party provider supports it, fall back to Ollama as a catch-all
	config.DebugLog("[Provider] No third-party provider found, using Ollama as fallback for model %s", modelName)
	return ollamaProvider
}
