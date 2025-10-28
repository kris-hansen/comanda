package models

import (
	"context"
	"fmt"
	"log"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kris-hansen/comanda/utils/fileutil"
	"github.com/kris-hansen/comanda/utils/retry"
	openai "github.com/sashabaranov/go-openai"
)

// VLLMProvider handles vLLM locally-served models
type VLLMProvider struct {
	verbose  bool
	endpoint string
	mu       sync.Mutex
}

// NewVLLMProvider creates a new vLLM provider instance
func NewVLLMProvider() *VLLMProvider {
	endpoint := os.Getenv("VLLM_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}
	return &VLLMProvider{
		endpoint: endpoint,
	}
}

// Name returns the provider name
func (v *VLLMProvider) Name() string {
	return "vllm"
}

// debugf prints debug information if verbose mode is enabled (thread-safe)
func (v *VLLMProvider) debugf(format string, args ...interface{}) {
	if v.verbose {
		v.mu.Lock()
		defer v.mu.Unlock()
		log.Printf("[DEBUG][vLLM] "+format+"\n", args...)
	}
}

// SupportsModel for VLLMProvider. Similar to Ollama, vLLM can support any model
// that users have loaded on their vLLM server. The actual validation happens
// when checking what models are available on the running vLLM instance.
func (v *VLLMProvider) SupportsModel(modelName string) bool {
	v.debugf("Checking if model is supported: %s", modelName)

	// vLLM can potentially support any model that users have loaded
	// The actual validation happens by checking the running vLLM server
	v.debugf("vLLM provider can support model: %s (will check server availability)", modelName)
	return true
}

// Configure sets up the provider. Since vLLM is a local service that doesn't use API keys,
// we accept "LOCAL" as a special API key value to indicate it's properly configured.
func (v *VLLMProvider) Configure(apiKey string) error {
	v.debugf("Configuring vLLM provider")
	if apiKey != "LOCAL" {
		return fmt.Errorf("invalid API key for vLLM: must be 'LOCAL' to indicate local service")
	}
	return nil
}

// SendPrompt sends a prompt to the specified model and returns the response
func (v *VLLMProvider) SendPrompt(modelName string, prompt string) (string, error) {
	v.debugf("Preparing to send prompt to model: %s", modelName)
	v.debugf("Prompt length: %d characters", len(prompt))

	// Use the OpenAI-compatible client
	config := openai.DefaultConfig("")
	config.BaseURL = v.endpoint + "/v1"
	client := openai.NewClientWithConfig(config)

	// Use retry mechanism for API calls
	result, err := retry.WithRetry(
		func() (interface{}, error) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			resp, err := client.CreateChatCompletion(
				ctx,
				openai.ChatCompletionRequest{
					Model: modelName,
					Messages: []openai.ChatCompletionMessage{
						{
							Role:    openai.ChatMessageRoleUser,
							Content: prompt,
						},
					},
				},
			)

			if err != nil {
				v.debugf("Error calling vLLM API: %v", err)
				// Check if it's a rate limit error
				if strings.Contains(err.Error(), "429") {
					return "", fmt.Errorf("API request failed with status 429: %v", err)
				}
				return "", fmt.Errorf("error calling vLLM API: %v (is vLLM server running?)", err)
			}

			if len(resp.Choices) == 0 {
				return "", fmt.Errorf("no response choices returned from vLLM")
			}

			responseText := resp.Choices[0].Message.Content
			v.debugf("API call completed, response length: %d characters", len(responseText))
			return responseText, nil
		},
		retry.Is429Error,
		retry.DefaultRetryConfig,
	)

	if err != nil {
		return "", err
	}

	return result.(string), nil
}

// SendPromptWithFile sends a prompt along with a file to the specified model and returns the response
func (v *VLLMProvider) SendPromptWithFile(modelName string, prompt string, file FileInput) (string, error) {
	v.debugf("Preparing to send prompt with file to model: %s", modelName)
	v.debugf("File path: %s", file.Path)

	// Read the file content with size check - do this outside the retry loop
	fileData, err := fileutil.SafeReadFile(file.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	// For vLLM, we'll use a similar approach to Ollama - combine file content with prompt
	// Note: Some vLLM models support vision/multimodal, but for simplicity we'll use text concatenation
	fileContent := string(fileData)
	combinedPrompt := fmt.Sprintf("File content:\n%s\n\nUser prompt: %s", fileContent, prompt)

	return v.SendPrompt(modelName, combinedPrompt)
}

// ValidateModel checks if the specific vLLM model is valid
func (v *VLLMProvider) ValidateModel(modelName string) bool {
	v.debugf("Validating model: %s", modelName)

	// Use the central model registry for validation
	isValid := GetRegistry().ValidateModel("vllm", modelName)

	// If not found in registry, fall back to SupportsModel for backward compatibility
	if !isValid {
		isValid = v.SupportsModel(modelName)
	}

	if isValid {
		v.debugf("Model %s validation succeeded", modelName)
	} else {
		v.debugf("Model %s validation failed", modelName)
	}

	return isValid
}

// SetVerbose enables or disables verbose mode
func (v *VLLMProvider) SetVerbose(verbose bool) {
	v.verbose = verbose
}

// getVLLMEndpoint returns the configured vLLM endpoint
func (v *VLLMProvider) getVLLMEndpoint() string {
	return v.endpoint
}

// checkVLLMServerHealth checks if the vLLM server is running and responsive
func (v *VLLMProvider) checkVLLMServerHealth() error {
	client := &http.Client{Timeout: 5 * time.Second}

	// Try to fetch models to verify server is running
	resp, err := client.Get(v.endpoint + "/v1/models")
	if err != nil {
		return fmt.Errorf("vLLM server not reachable at %s: %v", v.endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vLLM server returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}
