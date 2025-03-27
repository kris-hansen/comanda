package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"

	openai "github.com/sashabaranov/go-openai"
)

// OllamaModel represents the structure of a model returned by the Ollama API
type OllamaModel struct {
	Name    string `json:"name"`
	ModTime string `json:"modified_at"`
	Size    int64  `json:"size"`
}

// GetOpenAIModels fetches the list of available models from the OpenAI API.
func GetOpenAIModels(apiKey string) ([]string, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required for OpenAI")
	}
	client := openai.NewClient(apiKey)
	models, err := client.ListModels(context.Background())
	if err != nil {
		return nil, fmt.Errorf("error fetching OpenAI models: %v", err)
	}

	var allModels []string
	for _, model := range models.Models {
		allModels = append(allModels, model.ID)
	}

	return allModels, nil
}

// GetAnthropicModels returns a hardcoded list of known Anthropic models.
func GetAnthropicModels() []string {
	// This list should be updated periodically based on Anthropic's offerings.
	return []string{
		"claude-3-5-sonnet-20241022",
		"claude-3-5-sonnet-latest",
		"claude-3-5-haiku-latest",
		"claude-3-7-sonnet-20250219",
		"claude-3-7-sonnet-latest",
		"claude-3-5-haiku-20241022",
	}
}

// GetXAIModels returns a hardcoded list of known X.AI models.
func GetXAIModels() []string {
	// This list should be updated periodically based on X.AI's offerings.
	return []string{
		"grok-beta",
		"grok-vision-beta",
	}
}

// GetDeepseekModels returns a hardcoded list of known Deepseek models.
func GetDeepseekModels() []string {
	// This list should be updated periodically based on Deepseek's offerings.
	return []string{
		"deepseek-chat",
		"deepseek-coder",
		"deepseek-vision",
		"deepseek-reasoner",
	}
}

// GetGoogleModels returns a hardcoded list of known Google models.
func GetGoogleModels() []string {
	// This list should be updated periodically based on Google's offerings.
	// List based on user input and existing models, matching utils/models/google.go
	return []string{
		// From user input
		"gemini-2.5-pro-exp-03-25",
		"gemini-2.0-flash",
		"gemini-2.0-flash-lite",
		"gemini-1.5-flash",
		"gemini-1.5-flash-8b",
		"gemini-1.5-pro",
		"gemini-embedding-exp",

		// Existing models not explicitly in user list but kept for compatibility/completeness
		"gemini-1.0-pro",
		"gemini-2.0-flash-exp",                // Experimental version
		"gemini-2.0-flash-001",                // Specific version
		"gemini-2.0-pro-exp-02-05",            // Experimental version
		"gemini-2.0-flash-lite-preview-02-05", // Preview version
		"gemini-2.0-flash-thinking-exp-01-21", // Experimental version
		"aqa",                                 // Attributed Question Answering model
	}
}

// CheckOllamaInstalled checks if the Ollama CLI is installed and runnable.
func CheckOllamaInstalled() bool {
	cmd := exec.Command("ollama", "list")
	if err := cmd.Run(); err != nil {
		// Consider logging the error if verbose/debug is enabled
		return false
	}
	return true
}

// GetOllamaModels fetches the list of locally available models from the Ollama API.
func GetOllamaModels() ([]OllamaModel, error) {
	// Check if Ollama is running first
	if !CheckOllamaInstalled() {
		return nil, fmt.Errorf("Ollama is not installed or not running")
	}

	resp, err := http.Get("http://localhost:11434/api/tags")
	if err != nil {
		return nil, fmt.Errorf("error connecting to Ollama API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var response struct {
		Models []OllamaModel `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("error decoding Ollama response: %v", err)
	}

	return response.Models, nil
}

// GetAvailableModels retrieves the list of available models for a given provider.
// For providers like OpenAI and Ollama, it requires the API key or connection.
// For others, it returns a hardcoded list.
func GetAvailableModels(providerName string, apiKey string) ([]string, error) {
	switch providerName {
	case "openai":
		return GetOpenAIModels(apiKey)
	case "anthropic":
		return GetAnthropicModels(), nil
	case "google":
		return GetGoogleModels(), nil
	case "xai":
		return GetXAIModels(), nil
	case "deepseek":
		return GetDeepseekModels(), nil
	case "ollama":
		ollamaModels, err := GetOllamaModels()
		if err != nil {
			return nil, err
		}
		modelNames := make([]string, len(ollamaModels))
		for i, m := range ollamaModels {
			modelNames[i] = m.Name
		}
		return modelNames, nil
	default:
		return nil, fmt.Errorf("unknown provider: %s", providerName)
	}
}
