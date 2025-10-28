package models

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/kris-hansen/comanda/utils/fileutil"
	"github.com/kris-hansen/comanda/utils/retry"
	openai "github.com/sashabaranov/go-openai"
)

// DeepseekProvider handles Deepseek family of models
type DeepseekProvider struct {
	apiKey  string
	config  ModelConfig
	verbose bool
	mu      sync.Mutex
}

// NewDeepseekProvider creates a new Deepseek provider instance
func NewDeepseekProvider() *DeepseekProvider {
	return &DeepseekProvider{
		config: ModelConfig{
			Temperature:         0.7,
			MaxTokens:           2000,
			MaxCompletionTokens: 2000,
			TopP:                1.0,
		},
	}
}

// Name returns the provider name
func (d *DeepseekProvider) Name() string {
	return "deepseek"
}

// debugf prints debug information if verbose mode is enabled (thread-safe)
func (d *DeepseekProvider) debugf(format string, args ...interface{}) {
	if d.verbose {
		d.mu.Lock()
		defer d.mu.Unlock()
		log.Printf("[DEBUG][Deepseek] "+format+"\n", args...)
	}
}

// SupportsModel checks if the given model name is supported by Deepseek
func (d *DeepseekProvider) SupportsModel(modelName string) bool {
	d.debugf("Checking if model is supported: %s", modelName)
	modelName = strings.ToLower(modelName)

	// Register Deepseek model families if not already done
	registry := GetRegistry()
	if len(registry.GetFamilies("deepseek")) == 0 {
		registry.RegisterFamilies("deepseek", []string{
			"deepseek-",
		})
	}

	// Special case for deepseek-r1: only support if API key is configured
	if strings.HasPrefix(modelName, "deepseek-r1") {
		if d.apiKey == "" {
			d.debugf("Deepseek API key not configured; not claiming support for model %s", modelName)
			return false
		}
		d.debugf("Deepseek API key configured; supporting model %s", modelName)
		return true
	}

	// Use the central model registry for validation
	for _, prefix := range registry.GetFamilies("deepseek") {
		if strings.HasPrefix(modelName, prefix) {
			d.debugf("Model %s is supported (matches prefix %s)", modelName, prefix)
			return true
		}
	}

	// Also check exact matches in the registry
	for _, model := range registry.GetModels("deepseek") {
		if modelName == model {
			d.debugf("Model %s is supported (exact match)", modelName)
			return true
		}
	}

	d.debugf("Model %s is not supported (no matching prefix or exact match)", modelName)
	return false
}

// Configure sets up the provider with necessary credentials
func (d *DeepseekProvider) Configure(apiKey string) error {
	d.debugf("Configuring Deepseek provider")
	if apiKey == "" {
		return fmt.Errorf("API key is required for Deepseek provider")
	}
	d.apiKey = apiKey
	d.debugf("API key configured successfully")
	return nil
}

// createChatCompletionRequest creates a ChatCompletionRequest with the appropriate parameters
func (d *DeepseekProvider) createChatCompletionRequest(modelName string, messages []openai.ChatCompletionMessage) openai.ChatCompletionRequest {
	req := openai.ChatCompletionRequest{
		Model:    modelName,
		Messages: messages,
	}

	// deepseek-reasoner doesn't support temperature parameter
	if !strings.HasSuffix(modelName, "reasoner") {
		req.MaxTokens = d.config.MaxTokens
		req.Temperature = float32(d.config.Temperature)
		req.TopP = float32(d.config.TopP)
	}

	return req
}

// SendPrompt sends a prompt to the specified model and returns the response
func (d *DeepseekProvider) SendPrompt(modelName string, prompt string) (string, error) {
	d.debugf("Preparing to send prompt to model: %s", modelName)
	d.debugf("Prompt length: %d characters", len(prompt))

	if d.apiKey == "" {
		return "", fmt.Errorf("Deepseek provider not configured: missing API key")
	}

	if !d.SupportsModel(modelName) {
		return "", fmt.Errorf("invalid Deepseek model: %s", modelName)
	}

	d.debugf("Model validation passed, preparing API call")

	config := openai.DefaultConfig(d.apiKey)
	config.BaseURL = "https://api.deepseek.com/v1"
	client := openai.NewClientWithConfig(config)

	// Use retry mechanism for API calls
	result, err := retry.WithRetry(
		func() (interface{}, error) {
			messages := []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			}

			req := d.createChatCompletionRequest(modelName, messages)
			resp, err := client.CreateChatCompletion(context.Background(), req)

			if err != nil {
				return "", fmt.Errorf("Deepseek API error: %v", err)
			}

			if len(resp.Choices) == 0 {
				return "", fmt.Errorf("no response choices returned from Deepseek")
			}

			return resp.Choices[0].Message.Content, nil
		},
		retry.Is429Error,
		retry.DefaultRetryConfig,
	)

	if err != nil {
		return "", err
	}

	response := result.(string)
	d.debugf("API call completed, response length: %d characters", len(response))

	return response, nil
}

// SendPromptWithFile sends a prompt along with a file to the specified model and returns the response
func (d *DeepseekProvider) SendPromptWithFile(modelName string, prompt string, file FileInput) (string, error) {
	d.debugf("Preparing to send prompt with file to model: %s", modelName)
	d.debugf("File path: %s", file.Path)

	if d.apiKey == "" {
		return "", fmt.Errorf("Deepseek provider not configured: missing API key")
	}

	if !d.SupportsModel(modelName) {
		return "", fmt.Errorf("invalid Deepseek model: %s", modelName)
	}

	// Read the file content with size check - do this outside the retry loop
	fileData, err := fileutil.SafeReadFile(file.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	config := openai.DefaultConfig(d.apiKey)
	config.BaseURL = "https://api.deepseek.com/v1"
	client := openai.NewClientWithConfig(config)

	// For image files, handle them using vision capabilities
	if strings.HasPrefix(file.MimeType, "image/") {
		return d.handleFileAsVisionWithRetry(client, prompt, fileData, file.MimeType, modelName)
	}

	// For other files, include the content as part of the prompt
	fileContent := string(fileData)
	combinedPrompt := fmt.Sprintf("File content:\n%s\n\nUser prompt: %s", fileContent, prompt)

	// Use retry mechanism for API calls
	result, err := retry.WithRetry(
		func() (interface{}, error) {
			messages := []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: combinedPrompt,
				},
			}

			req := d.createChatCompletionRequest(modelName, messages)
			resp, err := client.CreateChatCompletion(context.Background(), req)

			if err != nil {
				return "", fmt.Errorf("Deepseek API error: %v", err)
			}

			if len(resp.Choices) == 0 {
				return "", fmt.Errorf("no response choices returned from Deepseek")
			}

			return resp.Choices[0].Message.Content, nil
		},
		retry.Is429Error,
		retry.DefaultRetryConfig,
	)

	if err != nil {
		return "", err
	}

	response := result.(string)
	d.debugf("API call completed, response length: %d characters", len(response))

	return response, nil
}

// handleFileAsVisionWithRetry processes a file as a vision model request with retry logic
func (d *DeepseekProvider) handleFileAsVisionWithRetry(client *openai.Client, prompt string, fileData []byte, mimeType string, modelName string) (string, error) {
	// Use retry mechanism for API calls
	result, err := retry.WithRetry(
		func() (interface{}, error) {
			return d.handleFileAsVision(client, prompt, fileData, mimeType, modelName)
		},
		retry.Is429Error,
		retry.DefaultRetryConfig,
	)

	if err != nil {
		return "", err
	}

	return result.(string), nil
}

// handleFileAsVision processes a file as a vision model request
func (d *DeepseekProvider) handleFileAsVision(client *openai.Client, prompt string, fileData []byte, mimeType string, modelName string) (string, error) {
	// Convert file data to base64 string with proper data URI prefix
	base64Data := fmt.Sprintf("data:%s;base64,%s", mimeType, string(fileData))

	// Create the message content with text and image parts
	content := []openai.ChatMessagePart{
		{
			Type: openai.ChatMessagePartTypeText,
			Text: prompt,
		},
		{
			Type: openai.ChatMessagePartTypeImageURL,
			ImageURL: &openai.ChatMessageImageURL{
				URL: base64Data,
			},
		},
	}

	messages := []openai.ChatCompletionMessage{
		{
			Role:         openai.ChatMessageRoleUser,
			MultiContent: content,
		},
	}

	req := d.createChatCompletionRequest(modelName, messages)
	resp, err := client.CreateChatCompletion(context.Background(), req)

	if err != nil {
		return "", fmt.Errorf("Deepseek Vision API error: %v", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned from Deepseek Vision")
	}

	return resp.Choices[0].Message.Content, nil
}

// SetConfig updates the provider configuration
func (d *DeepseekProvider) SetConfig(config ModelConfig) {
	d.debugf("Updating provider configuration")
	d.debugf("Old config: Temperature=%.2f, MaxTokens=%d, MaxCompletionTokens=%d, TopP=%.2f",
		d.config.Temperature, d.config.MaxTokens, d.config.MaxCompletionTokens, d.config.TopP)
	d.config = config
	d.debugf("New config: Temperature=%.2f, MaxTokens=%d, MaxCompletionTokens=%d, TopP=%.2f",
		d.config.Temperature, d.config.MaxTokens, d.config.MaxCompletionTokens, d.config.TopP)
}

// GetConfig returns the current provider configuration
func (d *DeepseekProvider) GetConfig() ModelConfig {
	return d.config
}

// ValidateModel checks if the specific Deepseek model variant is valid
func (d *DeepseekProvider) ValidateModel(modelName string) bool {
	d.debugf("Validating model: %s", modelName)

	// Use the central model registry for validation
	isValid := GetRegistry().ValidateModel("deepseek", modelName)

	// If not found in registry, fall back to SupportsModel for backward compatibility
	if !isValid {
		isValid = d.SupportsModel(modelName)
	}

	if isValid {
		d.debugf("Model %s validation succeeded", modelName)
	} else {
		d.debugf("Model %s validation failed", modelName)
	}

	return isValid
}

// SetVerbose enables or disables verbose mode
func (d *DeepseekProvider) SetVerbose(verbose bool) {
	d.verbose = verbose
}
