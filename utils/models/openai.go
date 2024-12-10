package models

import (
	"context"
	"fmt"
	"strings"

	"github.com/kris-hansen/comanda/utils/fileutil"
	openai "github.com/sashabaranov/go-openai"
)

// OpenAIProvider handles OpenAI family of models
type OpenAIProvider struct {
	apiKey  string
	config  ModelConfig
	verbose bool
}

// NewOpenAIProvider creates a new OpenAI provider instance
func NewOpenAIProvider() *OpenAIProvider {
	return &OpenAIProvider{
		config: ModelConfig{
			Temperature: 0.7,
			MaxTokens:   2000,
			TopP:        1.0,
		},
	}
}

// Name returns the provider name
func (o *OpenAIProvider) Name() string {
	return "openai"
}

// debugf prints debug information if verbose mode is enabled
func (o *OpenAIProvider) debugf(format string, args ...interface{}) {
	if o.verbose {
		fmt.Printf("[DEBUG][OpenAI] "+format+"\n", args...)
	}
}

// SupportsModel checks if the given model name is supported by OpenAI
func (o *OpenAIProvider) SupportsModel(modelName string) bool {
	o.debugf("Checking if model is supported: %s", modelName)
	modelName = strings.ToLower(modelName)

	// Accept any model name that starts with our known prefixes
	validPrefixes := []string{
		"gpt-",
		"o1-",
	}

	for _, prefix := range validPrefixes {
		if strings.HasPrefix(modelName, prefix) {
			o.debugf("Model %s is supported (matches prefix %s)", modelName, prefix)
			return true
		}
	}

	o.debugf("Model %s is not supported (no matching prefix)", modelName)
	return false
}

// Configure sets up the provider with necessary credentials
func (o *OpenAIProvider) Configure(apiKey string) error {
	o.debugf("Configuring OpenAI provider")
	if apiKey == "" {
		return fmt.Errorf("API key is required for OpenAI provider")
	}
	o.apiKey = apiKey
	o.debugf("API key configured successfully")
	return nil
}

// SendPrompt sends a prompt to the specified model and returns the response
func (o *OpenAIProvider) SendPrompt(modelName string, prompt string) (string, error) {
	o.debugf("Preparing to send prompt to model: %s", modelName)
	o.debugf("Prompt length: %d characters", len(prompt))

	if o.apiKey == "" {
		return "", fmt.Errorf("OpenAI provider not configured: missing API key")
	}

	if !o.SupportsModel(modelName) {
		return "", fmt.Errorf("invalid OpenAI model: %s", modelName)
	}

	o.debugf("Model validation passed, preparing API call")
	o.debugf("Using configuration: Temperature=%.2f, MaxTokens=%d, TopP=%.2f",
		o.config.Temperature, o.config.MaxTokens, o.config.TopP)

	client := openai.NewClient(o.apiKey)

	// Check if this is a vision input by looking for base64 image data
	if strings.HasPrefix(modelName, "gpt-4") && strings.Contains(prompt, ";base64,") {
		return o.handleVisionPrompt(client, prompt, modelName)
	}

	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: modelName,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			},
			Temperature: float32(o.config.Temperature),
			MaxTokens:   o.config.MaxTokens,
			TopP:        float32(o.config.TopP),
		},
	)

	if err != nil {
		return "", fmt.Errorf("OpenAI API error: %v", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned from OpenAI")
	}

	response := resp.Choices[0].Message.Content
	o.debugf("API call completed, response length: %d characters", len(response))

	return response, nil
}

// SendPromptWithFile sends a prompt along with a file to the specified model and returns the response
func (o *OpenAIProvider) SendPromptWithFile(modelName string, prompt string, file FileInput) (string, error) {
	o.debugf("Preparing to send prompt with file to model: %s", modelName)
	o.debugf("File path: %s", file.Path)

	if o.apiKey == "" {
		return "", fmt.Errorf("OpenAI provider not configured: missing API key")
	}

	if !o.SupportsModel(modelName) {
		return "", fmt.Errorf("invalid OpenAI model: %s", modelName)
	}

	// Read the file content with size check
	fileData, err := fileutil.SafeReadFile(file.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	client := openai.NewClient(o.apiKey)

	// For GPT-4 Vision, handle image files
	if strings.HasPrefix(modelName, "gpt-4") && strings.HasPrefix(file.MimeType, "image/") {
		return o.handleFileAsVision(client, prompt, fileData, file.MimeType, modelName)
	}

	// For other files, include the content as part of the prompt
	fileContent := string(fileData)
	combinedPrompt := fmt.Sprintf("File content:\n%s\n\nUser prompt: %s", fileContent, prompt)

	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: modelName,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: combinedPrompt,
				},
			},
			Temperature: float32(o.config.Temperature),
			MaxTokens:   o.config.MaxTokens,
			TopP:        float32(o.config.TopP),
		},
	)

	if err != nil {
		return "", fmt.Errorf("OpenAI API error: %v", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned from OpenAI")
	}

	response := resp.Choices[0].Message.Content
	o.debugf("API call completed, response length: %d characters", len(response))

	return response, nil
}

// handleFileAsVision processes a file as a vision model request
func (o *OpenAIProvider) handleFileAsVision(client *openai.Client, prompt string, fileData []byte, mimeType string, modelName string) (string, error) {
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

	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: modelName,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:         openai.ChatMessageRoleUser,
					MultiContent: content,
				},
			},
			MaxTokens: o.config.MaxTokens,
		},
	)

	if err != nil {
		return "", fmt.Errorf("OpenAI Vision API error: %v", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned from OpenAI Vision")
	}

	return resp.Choices[0].Message.Content, nil
}

// handleVisionPrompt processes a vision model request with image data
func (o *OpenAIProvider) handleVisionPrompt(client *openai.Client, prompt string, modelName string) (string, error) {
	// Split the prompt into text and base64 image data
	parts := strings.Split(prompt, "Action: ")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid vision prompt format")
	}

	// Extract image data from the input section
	inputParts := strings.Split(parts[0], "Input:\n")
	if len(inputParts) != 2 {
		return "", fmt.Errorf("invalid input format in vision prompt")
	}

	imageData := strings.TrimSpace(inputParts[1])
	action := strings.TrimSpace(parts[1])

	o.debugf("Vision prompt image data length: %d", len(imageData))
	o.debugf("Vision prompt action length: %d", len(action))

	// Check if image data is properly formatted
	if !strings.HasPrefix(imageData, "data:image/") && !strings.Contains(imageData, ";base64,") {
		imageData = fmt.Sprintf("data:image/png;base64,%s", imageData)
	}

	// Create the message content with text and image parts
	content := []openai.ChatMessagePart{
		{
			Type: openai.ChatMessagePartTypeText,
			Text: action,
		},
		{
			Type: openai.ChatMessagePartTypeImageURL,
			ImageURL: &openai.ChatMessageImageURL{
				URL: imageData,
			},
		},
	}

	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: modelName,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:         openai.ChatMessageRoleUser,
					MultiContent: content,
				},
			},
			MaxTokens: o.config.MaxTokens,
		},
	)

	if err != nil {
		return "", fmt.Errorf("OpenAI Vision API error: %v", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned from OpenAI Vision")
	}

	return resp.Choices[0].Message.Content, nil
}

// ValidateModel checks if the specific OpenAI model variant is valid
func (o *OpenAIProvider) ValidateModel(modelName string) bool {
	return o.SupportsModel(modelName)
}

// SetConfig updates the provider configuration
func (o *OpenAIProvider) SetConfig(config ModelConfig) {
	o.debugf("Updating provider configuration")
	o.debugf("Old config: Temperature=%.2f, MaxTokens=%d, TopP=%.2f",
		o.config.Temperature, o.config.MaxTokens, o.config.TopP)
	o.config = config
	o.debugf("New config: Temperature=%.2f, MaxTokens=%d, TopP=%.2f",
		o.config.Temperature, o.config.MaxTokens, o.config.TopP)
}

// GetConfig returns the current provider configuration
func (o *OpenAIProvider) GetConfig() ModelConfig {
	return o.config
}

// SetVerbose enables or disables verbose mode
func (o *OpenAIProvider) SetVerbose(verbose bool) {
	o.verbose = verbose
}
