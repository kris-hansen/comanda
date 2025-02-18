package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kris-hansen/comanda/utils/fileutil"
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

	// Ollama supports specific model families
	modelName = strings.ToLower(modelName)
	o.debugf("Checking model name: %s", modelName)

	// List of known Ollama model prefixes
	ollamaPrefixes := []string{
		"llama2",
		"codellama",
		"mistral",
		"neural-chat",
		"dolphin",
		"orca",
		"vicuna",
		"nous",
		"wizard",
		"stable",
		"phi",
		"openchat",
		"solar",
		"yi",
		"qwen",
		"mixtral",
		"deepseek-r1",
	}

	// Check if model starts with any known Ollama prefix
	for _, prefix := range ollamaPrefixes {
		if strings.HasPrefix(modelName, prefix) {
			o.debugf("Model %s is supported by Ollama (matches prefix: %s)", modelName, prefix)
			return true
		}
	}

	o.debugf("Model %s is not supported by Ollama (no matching prefix)", modelName)
	return false
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

	o.debugf("Sending request to Ollama API: %s", string(jsonData))
	resp, err := http.Post("http://localhost:11434/api/generate", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		o.debugf("Error calling Ollama API: %v", err)
		return "", fmt.Errorf("error calling Ollama API: %v (is Ollama running?)", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		o.debugf("Ollama API returned non-200 status: %d, body: %s", resp.StatusCode, string(bodyBytes))
		return "", fmt.Errorf("Ollama API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}
	o.debugf("Ollama API request successful, reading response")

	// Read and accumulate all responses
	var fullResponse strings.Builder
	decoder := json.NewDecoder(resp.Body)
	for {
		var ollamaResp OllamaResponse
		if err := decoder.Decode(&ollamaResp); err != nil {
			if err == io.EOF {
				break
			}
			o.debugf("Error decoding response: %v", err)
			return "", fmt.Errorf("error decoding response: %v", err)
		}
		o.debugf("Received response chunk: done=%v length=%d", ollamaResp.Done, len(ollamaResp.Response))
		fullResponse.WriteString(ollamaResp.Response)
		if ollamaResp.Done {
			break
		}
	}

	result := fullResponse.String()
	o.debugf("API call completed, response length: %d characters", len(result))
	return result, nil
}

// SendPromptWithFile sends a prompt along with a file to the specified model and returns the response
func (o *OllamaProvider) SendPromptWithFile(modelName string, prompt string, file FileInput) (string, error) {
	o.debugf("Preparing to send prompt with file to model: %s", modelName)
	o.debugf("File path: %s", file.Path)

	// Read the file content with size check
	fileData, err := fileutil.SafeReadFile(file.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	// Combine file content with the prompt
	fileContent := string(fileData)
	combinedPrompt := fmt.Sprintf("File content:\n%s\n\nUser prompt: %s", fileContent, prompt)

	reqBody := OllamaRequest{
		Model:  modelName,
		Prompt: combinedPrompt,
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
