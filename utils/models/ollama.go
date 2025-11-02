package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kris-hansen/comanda/utils/fileutil"
	"github.com/kris-hansen/comanda/utils/retry"
)

// OllamaProvider handles Ollama family of models
type OllamaProvider struct {
	verbose bool
	mu      sync.Mutex
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

// debugf prints debug information if verbose mode is enabled (thread-safe)
func (o *OllamaProvider) debugf(format string, args ...interface{}) {
	if o.verbose {
		o.mu.Lock()
		defer o.mu.Unlock()
		log.Printf("[DEBUG][Ollama] "+format+"\n", args...)
	}
}

// SupportsModel for OllamaProvider. Since we now have proper provider ordering in DetectProvider,
// Ollama can accept any model name and let the actual local availability check determine if it exists.
func (o *OllamaProvider) SupportsModel(modelName string) bool {
	o.debugf("Checking if model is supported: %s", modelName)

	// Ollama can potentially support any model that users have pulled locally
	// The actual validation happens in isModelAvailableLocally() or checkOllamaModelExists()
	o.debugf("Ollama provider can support model: %s (will check local availability)", modelName)
	return true
}

// Configure sets up the provider. Since Ollama is a local service that doesn't use API keys,
// we accept "LOCAL" as a special API key value to indicate it's properly configured.
// Note: The original implementation checked for "LOCAL". This seems unnecessary now
// as configuration might not be needed if we dynamically check models.
// However, keeping the Configure method might be required by the Provider interface.
// Let's keep the check for now, but it might be removable later if configure isn't called.
func (o *OllamaProvider) Configure(apiKey string) error {
	o.debugf("Configuring Ollama provider")
	if apiKey != "LOCAL" {
		return fmt.Errorf("invalid API key for Ollama: must be 'LOCAL' to indicate local service")
	}
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

	// Use retry mechanism for API calls
	result, err := retry.WithRetry(
		func() (interface{}, error) {
			client := &http.Client{Timeout: 30 * time.Second} // Add a 30-second timeout
			resp, err := client.Post("http://localhost:11434/api/generate", "application/json", bytes.NewBuffer(jsonData))
			if err != nil {
				o.debugf("Error calling Ollama API: %v", err)
				return "", fmt.Errorf("error calling Ollama API: %v (is Ollama running?)", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				bodyBytes, _ := io.ReadAll(resp.Body)
				o.debugf("Ollama API returned non-200 status: %d, body: %s", resp.StatusCode, string(bodyBytes))

				// Check for rate limit errors (429)
				if resp.StatusCode == http.StatusTooManyRequests {
					return "", fmt.Errorf("API request failed with status 429: %s", string(bodyBytes))
				}

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

			return fullResponse.String(), nil
		},
		retry.Is429Error,
		retry.DefaultRetryConfig,
	)

	if err != nil {
		return "", err
	}

	response := result.(string)
	o.debugf("API call completed, response length: %d characters", len(response))
	return response, nil
}

// SendPromptWithFile sends a prompt along with a file to the specified model and returns the response
func (o *OllamaProvider) SendPromptWithFile(modelName string, prompt string, file FileInput) (string, error) {
	o.debugf("Preparing to send prompt with file to model: %s", modelName)
	o.debugf("File path: %s", file.Path)

	// Read the file content with size check - do this outside the retry loop
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

	// Use retry mechanism for API calls
	result, err := retry.WithRetry(
		func() (interface{}, error) {
			client := &http.Client{Timeout: 30 * time.Second} // Add a 30-second timeout
			resp, err := client.Post("http://localhost:11434/api/generate", "application/json", bytes.NewBuffer(jsonData))
			if err != nil {
				return "", fmt.Errorf("error calling Ollama API: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				bodyBytes, _ := io.ReadAll(resp.Body)

				// Check for rate limit errors (429)
				if resp.StatusCode == http.StatusTooManyRequests {
					return "", fmt.Errorf("API request failed with status 429: %s", string(bodyBytes))
				}

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

			return fullResponse.String(), nil
		},
		retry.Is429Error,
		retry.DefaultRetryConfig,
	)

	if err != nil {
		return "", err
	}

	response := result.(string)
	o.debugf("API call completed, response length: %d characters", len(response))
	return response, nil
}

// ValidateModel checks if the specific Ollama model variant is valid
func (o *OllamaProvider) ValidateModel(modelName string) bool {
	o.debugf("Validating model: %s", modelName)

	// Use the central model registry for validation
	isValid := GetRegistry().ValidateModel("ollama", modelName)

	// If not found in registry, fall back to SupportsModel for backward compatibility
	if !isValid {
		isValid = o.SupportsModel(modelName)
	}

	if isValid {
		o.debugf("Model %s validation succeeded", modelName)
	} else {
		o.debugf("Model %s validation failed", modelName)
	}

	return isValid
}

// SetVerbose enables or disables verbose mode
func (o *OllamaProvider) SetVerbose(verbose bool) {
	o.verbose = verbose
}
