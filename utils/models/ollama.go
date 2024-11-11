package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// OllamaProvider handles Ollama family of models
type OllamaProvider struct {
	verbose bool
}

// OllamaRequest represents the request structure for Ollama API
type OllamaRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// OllamaResponse represents the response structure from Ollama API
type OllamaResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// NewOllamaProvider creates a new Ollama provider instance
func NewOllamaProvider() *OllamaProvider {
	return &OllamaProvider{}
}

// Name returns the provider name
func (o *OllamaProvider) Name() string {
	return "ollama"
}

// debugf prints debug information if verbose mode is enabled
func (o *OllamaProvider) debugf(format string, args ...interface{}) {
	if o.verbose {
		fmt.Printf("[DEBUG][Ollama] "+format+"\n", args...)
	}
}

// SupportsModel checks if the given model name is supported by Ollama
func (o *OllamaProvider) SupportsModel(modelName string) bool {
	o.debugf("Checking if model is supported: %s", modelName)

	// Ollama typically supports models like:
	// - llama2
	// - codellama
	// - mistral
	// - neural-chat
	// But should not support models with known prefixes from other providers
	modelName = strings.ToLower(modelName)

	// Don't support models from other providers
	knownOtherPrefixes := []string{
		"gpt-",    // OpenAI
		"claude-", // Anthropic
		"gemini-", // Google
	}

	for _, prefix := range knownOtherPrefixes {
		if strings.HasPrefix(modelName, prefix) {
			o.debugf("Model %s belongs to another provider (prefix: %s)", modelName, prefix)
			return false
		}
	}

	o.debugf("Model %s may be supported by Ollama", modelName)
	return true
}

// Configure sets up the provider (no API key needed for Ollama)
func (o *OllamaProvider) Configure(apiKey string) error {
	o.debugf("Configuring Ollama provider")
	return nil
}

// SendPrompt sends a prompt to the specified model and returns the response
func (o *OllamaProvider) SendPrompt(modelName string, prompt string) (string, error) {
	o.debugf("Preparing to send prompt to model: %s", modelName)
	o.debugf("Prompt length: %d characters", len(prompt))

	reqBody := OllamaRequest{
		Model:  modelName,
		Prompt: prompt,
		Stream: false,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("error marshaling request: %v", err)
	}

	resp, err := http.Post("http://localhost:11434/api/generate", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("error calling Ollama API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Ollama API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	// Read and accumulate all responses
	var fullResponse strings.Builder
	decoder := json.NewDecoder(resp.Body)
	for {
		var ollamaResp OllamaResponse
		if err := decoder.Decode(&ollamaResp); err != nil {
			if err == io.EOF {
				break
			}
			return "", fmt.Errorf("error decoding response: %v", err)
		}
		fullResponse.WriteString(ollamaResp.Response)
		if ollamaResp.Done {
			break
		}
	}

	result := fullResponse.String()
	o.debugf("API call completed, response length: %d characters", len(result))
	return result, nil
}

// SetVerbose enables or disables verbose mode
func (o *OllamaProvider) SetVerbose(verbose bool) {
	o.verbose = verbose
}
