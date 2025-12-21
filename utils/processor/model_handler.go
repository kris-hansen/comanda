package processor

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/models"
)

// --- Ollama specific types (copied from ollama.go for local check) ---

// OllamaTagsResponse represents the top-level structure of Ollama's /api/tags response
type OllamaTagsResponse struct {
	Models []OllamaModelTag `json:"models"`
}

// OllamaModelTag represents the details of a single model tag from /api/tags
type OllamaModelTag struct {
	Name string `json:"name"`
}

// --- Helper function to check local Ollama models ---

// checkOllamaModelExists queries the local Ollama instance to see if a model tag exists.
func checkOllamaModelExists(modelName string) (bool, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("http://localhost:11434/api/tags")
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return false, fmt.Errorf("failed to connect to Ollama at http://localhost:11434 to verify model '%s'. Is Ollama running?", modelName)
		}
		return false, fmt.Errorf("error calling Ollama /api/tags to verify model '%s': %v", modelName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return false, fmt.Errorf("Ollama /api/tags returned non-OK status %d while verifying model '%s': %s", resp.StatusCode, modelName, string(bodyBytes))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Errorf("error reading Ollama /api/tags response body while verifying model '%s': %v", modelName, err)
	}

	var tagsResponse OllamaTagsResponse
	if err := json.Unmarshal(bodyBytes, &tagsResponse); err != nil {
		return false, fmt.Errorf("error unmarshaling Ollama /api/tags response while verifying model '%s': %v. Body: %s", modelName, err, string(bodyBytes))
	}

	modelNameLower := strings.ToLower(modelName)
	for _, model := range tagsResponse.Models {
		modelFullName := strings.ToLower(model.Name)

		// First check exact match
		if modelFullName == modelNameLower {
			return true, nil // Model found (exact match)
		}

		// Then check if the requested model matches the base name (before :tag)
		// e.g., "gpt-oss" should match "gpt-oss:latest"
		if strings.Contains(modelFullName, ":") {
			baseName := strings.Split(modelFullName, ":")[0]
			if baseName == modelNameLower {
				return true, nil // Model found (tag match)
			}
		}

		// Also check if the full model name starts with the requested name
		// e.g., "llama3" should match "llama3.2:latest"
		if strings.HasPrefix(modelFullName, modelNameLower) {
			// Make sure we're not matching partial names unintentionally
			nextChar := modelFullName[len(modelNameLower):]
			if strings.HasPrefix(nextChar, ":") || strings.HasPrefix(nextChar, ".") {
				return true, nil // Model found (prefix match)
			}
		}
	}

	// Model not found in the list
	// Construct a helpful error message suggesting how to pull the model
	availableModels := make([]string, len(tagsResponse.Models))
	for i, m := range tagsResponse.Models {
		availableModels[i] = m.Name
	}
	errMsg := fmt.Sprintf("model tag '%s' not found in local Ollama instance. Available models: %v. Try running 'ollama pull %s'", modelName, availableModels, modelName)
	return false, fmt.Errorf("%s", errMsg)
}

// validateModel checks if the specified model is supported and has the required capabilities
func (p *Processor) validateModel(modelNames []string, inputs []string) error {
	if len(modelNames) == 0 {
		return fmt.Errorf("no model specified")
	}

	// Special case: if the only model is "NA", skip validation
	if len(modelNames) == 1 && modelNames[0] == "NA" {
		p.debugf("Model is NA, skipping provider validation")
		return nil
	}

	p.debugf("Validating %d model(s)", len(modelNames))
	for _, modelName := range modelNames {
		p.debugf("Starting validation for model: %s", modelName)
		p.debugf("Attempting provider detection for model: %s", modelName)
		provider := models.DetectProvider(modelName)
		p.debugf("Provider detection result for %s: found=%v", modelName, provider != nil)
		if provider == nil {
			// Check if this is a Claude Code model - give specific error about missing CLI
			if models.NewClaudeCodeProvider().SupportsModel(modelName) {
				errMsg := fmt.Sprintf("model %s requires Claude Code CLI, but 'claude' binary not found. Install Claude Code from https://claude.ai/download or ensure it's in your PATH", modelName)
				p.debugf("Validation failed: %s", errMsg)
				return fmt.Errorf("%s", errMsg)
			}
			// Check if this is a Gemini CLI model - give specific error about missing CLI
			if models.NewGeminiCLIProvider().SupportsModel(modelName) {
				errMsg := fmt.Sprintf("model %s requires Gemini CLI, but 'gemini' binary not found. Install Gemini CLI via 'npm install -g @google/gemini-cli' or ensure it's in your PATH", modelName)
				p.debugf("Validation failed: %s", errMsg)
				return fmt.Errorf("%s", errMsg)
			}
			// Check if this is an OpenAI Codex model - give specific error about missing CLI
			if models.NewOpenAICodexProvider().SupportsModel(modelName) {
				errMsg := fmt.Sprintf("model %s requires OpenAI Codex CLI, but 'codex' binary not found. Install OpenAI Codex CLI via 'npm install -g @openai/codex' or ensure it's in your PATH", modelName)
				p.debugf("Validation failed: %s", errMsg)
				return fmt.Errorf("%s", errMsg)
			}
			errMsg := fmt.Sprintf("unsupported model: %s (no provider found)", modelName)
			p.debugf("Validation failed: %s", errMsg)
			return fmt.Errorf("%s", errMsg)
		}

		// Check if the provider actually supports this model
		p.debugf("Checking if provider %s supports model %s", provider.Name(), modelName)
		if !provider.SupportsModel(modelName) {
			errMsg := fmt.Sprintf("unsupported model: %s (provider %s does not support it)", modelName, provider.Name())
			p.debugf("Validation failed: %s", errMsg)
			return fmt.Errorf("%s", errMsg)
		}
		p.debugf("Provider %s confirmed support for model %s", provider.Name(), modelName)

		// Get provider name
		providerName := provider.Name()

		// --- Add Ollama specific local check ---
		if providerName == "ollama" {
			p.debugf("Performing local check for Ollama model tag: %s", modelName)
			exists, err := checkOllamaModelExists(modelName)
			if err != nil {
				// Error occurred during check (e.g., connection refused, API error)
				p.debugf("Ollama local check failed for %s: %v", modelName, err)
				return fmt.Errorf("Ollama check failed: %w", err) // Wrap the specific error
			}
			if !exists {
				// Model tag specifically not found in the list from /api/tags
				// The error from checkOllamaModelExists already contains the helpful message
				p.debugf("Ollama model tag %s not found locally.", modelName)
				// The error from checkOllamaModelExists includes the suggestion to pull
				return fmt.Errorf("model tag '%s' not found locally via Ollama API", modelName)
			}
			p.debugf("Ollama model tag %s confirmed to exist locally.", modelName)
		}
		// --- End Ollama specific check ---

		// --- Skip envConfig checks for Claude Code provider ---
		// Claude Code uses the local 'claude' binary and doesn't require API key configuration
		if providerName == "claude-code" {
			p.debugf("Skipping envConfig check for claude-code provider (uses local binary)")
			provider.SetVerbose(p.verbose)
			p.providers[provider.Name()] = provider
			p.debugf("Model %s is supported by provider %s", modelName, provider.Name())
			continue
		}
		// --- End Claude Code specific check ---

		// --- Skip envConfig checks for Gemini CLI provider ---
		// Gemini CLI uses the local 'gemini' binary and doesn't require API key configuration here
		if providerName == "gemini-cli" {
			p.debugf("Skipping envConfig check for gemini-cli provider (uses local binary)")
			provider.SetVerbose(p.verbose)
			p.providers[provider.Name()] = provider
			p.debugf("Model %s is supported by provider %s", modelName, provider.Name())
			continue
		}
		// --- End Gemini CLI specific check ---

		// --- Skip envConfig checks for OpenAI Codex provider ---
		// OpenAI Codex uses the local 'codex' binary and doesn't require API key configuration here
		if providerName == "openai-codex" {
			p.debugf("Skipping envConfig check for openai-codex provider (uses local binary)")
			provider.SetVerbose(p.verbose)
			p.providers[provider.Name()] = provider
			p.debugf("Model %s is supported by provider %s", modelName, provider.Name())
			continue
		}
		// --- End OpenAI Codex specific check ---

		// Get model configuration from environment
		p.debugf("Getting model configuration for %s from provider %s", modelName, providerName)
		modelConfig, err := p.envConfig.GetModelConfig(providerName, modelName)
		if err != nil {
			// Check if the error is specifically "model not found" after provider support was confirmed
			if strings.Contains(err.Error(), fmt.Sprintf("model %s not found for provider %s", modelName, providerName)) {
				errMsg := fmt.Sprintf("model %s is supported by provider %s but is not enabled in your configuration. Use 'comanda configure' to add it.", modelName, providerName)
				p.debugf("Configuration error: %s", errMsg)
				return fmt.Errorf("%s", errMsg)
			}
			// Otherwise, return the original configuration error
			errMsg := fmt.Sprintf("failed to get model configuration for %s: %v", modelName, err)
			p.debugf("Configuration error: %s", errMsg)
			return fmt.Errorf("%s", errMsg)
		}
		p.debugf("Successfully retrieved model configuration for %s", modelName)

		// Check if model has required capabilities based on input types
		for _, input := range inputs {
			if input == "NA" || input == "STDIN" {
				continue
			}

			// Check for file mode support if input is a document file
			if p.validator.IsDocumentFile(input) && !modelConfig.HasMode(config.FileMode) {
				return fmt.Errorf("model %s does not support file processing", modelName)
			}

			// Check for vision mode support if input is an image file
			if p.validator.IsImageFile(input) && !modelConfig.HasMode(config.VisionMode) {
				return fmt.Errorf("model %s does not support image processing", modelName)
			}

			// For text files, ensure model supports text mode
			if !p.validator.IsDocumentFile(input) && !p.validator.IsImageFile(input) && !modelConfig.HasMode(config.TextMode) {
				return fmt.Errorf("model %s does not support text processing", modelName)
			}
		}

		provider.SetVerbose(p.verbose)
		// Store provider by provider name instead of model name
		p.providers[provider.Name()] = provider
		p.debugf("Model %s is supported by provider %s", modelName, provider.Name())
	}
	return nil
}

// localProviders lists providers that use local binaries and don't require API keys.
// Add new local provider names here as needed.
var localProviders = map[string]bool{
	"ollama":       true,
	"claude-code":  true,
	"gemini-cli":   true,
	"openai-codex": true,
}

// isLocalProvider checks if a provider uses local configuration (no API key needed)
func isLocalProvider(name string) bool {
	return localProviders[name]
}

// configureProviders sets up all detected providers with API keys
func (p *Processor) configureProviders() error {
	p.debugf("Configuring providers")

	for providerName, provider := range p.providers {
		p.debugf("Configuring provider %s", providerName)

		// Handle local providers (no API key needed, use "LOCAL" configuration)
		if isLocalProvider(providerName) {
			if err := provider.Configure("LOCAL"); err != nil {
				return fmt.Errorf("failed to configure provider %s: %w", providerName, err)
			}
			p.debugf("Successfully configured local provider %s", providerName)
			continue
		}

		var providerConfig *config.Provider
		var err error

		switch providerName {
		case "anthropic":
			providerConfig, err = p.envConfig.GetProviderConfig("anthropic")
		case "openai":
			providerConfig, err = p.envConfig.GetProviderConfig("openai")
		case "google":
			providerConfig, err = p.envConfig.GetProviderConfig("google")
		case "xai":
			providerConfig, err = p.envConfig.GetProviderConfig("xai")
		case "deepseek":
			providerConfig, err = p.envConfig.GetProviderConfig("deepseek")
		case "moonshot":
			providerConfig, err = p.envConfig.GetProviderConfig("moonshot")
		default:
			return fmt.Errorf("unknown provider: %s", providerName)
		}

		if err != nil {
			return fmt.Errorf("failed to get config for provider %s: %w", providerName, err)
		}

		if providerConfig.APIKey == "" {
			return fmt.Errorf("missing API key for provider %s", providerName)
		}

		p.debugf("Found API key for provider %s", providerName)

		if err := provider.Configure(providerConfig.APIKey); err != nil {
			return fmt.Errorf("failed to configure provider %s: %w", providerName, err)
		}

		p.debugf("Successfully configured provider %s", providerName)
	}
	return nil
}

// GetModelProvider returns the provider for the specified model
func (p *Processor) GetModelProvider(modelName string) models.Provider {
	// Special case: if model is "NA", return nil since no provider is needed
	if modelName == "NA" {
		return nil
	}

	provider := models.DetectProvider(modelName)
	if provider == nil {
		return nil
	}
	return p.providers[provider.Name()]
}
