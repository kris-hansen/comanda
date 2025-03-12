package models

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
			Temperature:         0.7,
			MaxTokens:           2000,
			MaxCompletionTokens: 2000,
			TopP:                1.0,
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

// isNewModelSeries checks if the model is part of the newer series (4o or o1)
func (o *OpenAIProvider) isNewModelSeries(modelName string) bool {
	modelName = strings.ToLower(modelName)
	return strings.Contains(modelName, "4o") || strings.HasPrefix(modelName, "o1-")
}

// createChatCompletionRequest creates a ChatCompletionRequest with the appropriate parameters
func (o *OpenAIProvider) createChatCompletionRequest(modelName string, messages []openai.ChatCompletionMessage) openai.ChatCompletionRequest {
	req := openai.ChatCompletionRequest{
		Model:    modelName,
		Messages: messages,
	}

	if o.isNewModelSeries(modelName) {
		// New model series (4o and o1) have fixed parameters
		req.MaxCompletionTokens = o.config.MaxCompletionTokens
		req.Temperature = 1.0
		req.TopP = 1.0
		req.PresencePenalty = 0.0
		req.FrequencyPenalty = 0.0
		o.debugf("Using fixed parameters for new model series: Temperature=1.0, TopP=1.0, PresencePenalty=0.0, FrequencyPenalty=0.0")
	} else {
		// Legacy models use configurable parameters
		req.MaxTokens = o.config.MaxTokens
		req.Temperature = float32(o.config.Temperature)
		req.TopP = float32(o.config.TopP)
		o.debugf("Using configured parameters for legacy model: Temperature=%.2f, TopP=%.2f", o.config.Temperature, o.config.TopP)
	}

	return req
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

	client := openai.NewClient(o.apiKey)

	// Check if this is a vision input by looking for base64 image data
	if strings.HasPrefix(modelName, "gpt-4") && strings.Contains(prompt, ";base64,") {
		return o.handleVisionPrompt(client, prompt, modelName)
	}

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: prompt,
		},
	}

	req := o.createChatCompletionRequest(modelName, messages)
	resp, err := client.CreateChatCompletion(context.Background(), req)

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

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: combinedPrompt,
		},
	}

	req := o.createChatCompletionRequest(modelName, messages)
	resp, err := client.CreateChatCompletion(context.Background(), req)

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

	messages := []openai.ChatCompletionMessage{
		{
			Role:         openai.ChatMessageRoleUser,
			MultiContent: content,
		},
	}

	req := o.createChatCompletionRequest(modelName, messages)
	resp, err := client.CreateChatCompletion(context.Background(), req)

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

	messages := []openai.ChatCompletionMessage{
		{
			Role:         openai.ChatMessageRoleUser,
			MultiContent: content,
		},
	}

	req := o.createChatCompletionRequest(modelName, messages)
	resp, err := client.CreateChatCompletion(context.Background(), req)

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
	o.debugf("Old config: Temperature=%.2f, MaxTokens=%d, MaxCompletionTokens=%d, TopP=%.2f",
		o.config.Temperature, o.config.MaxTokens, o.config.MaxCompletionTokens, o.config.TopP)
	o.config = config
	o.debugf("New config: Temperature=%.2f, MaxTokens=%d, MaxCompletionTokens=%d, TopP=%.2f",
		o.config.Temperature, o.config.MaxTokens, o.config.MaxCompletionTokens, o.config.TopP)
}

// GetConfig returns the current provider configuration
func (o *OpenAIProvider) GetConfig() ModelConfig {
	return o.config
}

// SetVerbose enables or disables verbose mode
func (o *OpenAIProvider) SetVerbose(verbose bool) {
	o.verbose = verbose
}

// SendPromptWithResponses sends a prompt using the OpenAI Responses API
func (o *OpenAIProvider) SendPromptWithResponses(config ResponsesConfig) (string, error) {
	o.debugf("Preparing to send prompt using Responses API with model: %s", config.Model)

	if o.apiKey == "" {
		return "", fmt.Errorf("OpenAI provider not configured: missing API key")
	}

	if !o.SupportsModel(config.Model) {
		return "", fmt.Errorf("invalid OpenAI model: %s", config.Model)
	}

	// Build the request body
	requestBody := map[string]interface{}{
		"model": config.Model,
		"input": config.Input,
	}

	// Add optional parameters if provided
	if config.Instructions != "" {
		requestBody["instructions"] = config.Instructions
	}

	if config.PreviousResponseID != "" {
		requestBody["previous_response_id"] = config.PreviousResponseID
	}

	if config.MaxOutputTokens > 0 {
		requestBody["max_output_tokens"] = config.MaxOutputTokens
	}

	if config.Temperature > 0 {
		requestBody["temperature"] = config.Temperature
	}

	if config.TopP > 0 {
		requestBody["top_p"] = config.TopP
	}

	if len(config.Tools) > 0 {
		// Format tools correctly for the API
		var formattedTools []map[string]interface{}

		// Debug the tools
		toolsBytes, _ := json.Marshal(config.Tools)
		o.debugf("Tools: %s", string(toolsBytes))

		// Process each tool
		for _, toolMap := range config.Tools {
			o.debugf("Processing tool: %v", toolMap)

			// Get the tool type
			toolTypeRaw, ok := toolMap["type"]
			if !ok {
				o.debugf("Tool has no type: %v", toolMap)
				continue
			}

			toolType, ok := toolTypeRaw.(string)
			if !ok {
				o.debugf("Tool type is not a string: %v", toolTypeRaw)
				continue
			}

			// Process based on tool type
			if toolType == "function" {
				functionRaw, ok := toolMap["function"]
				if !ok {
					o.debugf("Function tool has no function field: %v", toolMap)
					continue
				}

				function, ok := functionRaw.(map[string]interface{})
				if !ok {
					o.debugf("Function field is not a map: %v", functionRaw)
					continue
				}

				// Extract the name from the function object
				name, ok := function["name"].(string)
				if !ok {
					o.debugf("Function has no name field: %v", function)
					continue
				}

				// Add the formatted function tool with name at the top level
				formattedTools = append(formattedTools, map[string]interface{}{
					"type":     "function",
					"name":     name,
					"function": function,
				})
				o.debugf("Added function tool: %s", name)
			} else {
				// Other tool types (like web_search)
				formattedTools = append(formattedTools, toolMap)
				o.debugf("Added tool of type %s", toolType)
			}
		}

		// Add the formatted tools to the request
		requestBody["tools"] = formattedTools
		o.debugf("Final formatted tools: %v", formattedTools)
	}

	if config.ResponseFormat != nil {
		requestBody["text"] = config.ResponseFormat
	}

	// Convert request body to JSON
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", "https://api.openai.com/v1/responses", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", o.apiKey))

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for error status code
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("OpenAI API error: %s (status code: %d)", string(body), resp.StatusCode)
	}

	// Parse response
	var responseData map[string]interface{}
	if err := json.Unmarshal(body, &responseData); err != nil {
		return "", fmt.Errorf("failed to parse response body: %w", err)
	}

	// Extract output text
	output, err := o.extractOutputText(responseData)
	if err != nil {
		return "", fmt.Errorf("failed to extract output text: %w", err)
	}

	o.debugf("API call completed, response length: %d characters", len(output))

	return output, nil
}

// extractOutputText extracts the output text from the Responses API response
func (o *OpenAIProvider) extractOutputText(responseData map[string]interface{}) (string, error) {
	// Check if output exists
	output, ok := responseData["output"]
	if !ok {
		return "", fmt.Errorf("response does not contain output field")
	}

	// Output is an array of content items
	outputArray, ok := output.([]interface{})
	if !ok {
		return "", fmt.Errorf("output field is not an array")
	}

	// Process each output item
	var result strings.Builder

	for _, item := range outputArray {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		// Check if this is a message
		if itemType, ok := itemMap["type"].(string); ok && itemType == "message" {
			// Get content array
			content, ok := itemMap["content"].([]interface{})
			if !ok {
				continue
			}

			// Process each content item
			for _, contentItem := range content {
				contentMap, ok := contentItem.(map[string]interface{})
				if !ok {
					continue
				}

				// Check if this is output_text
				if contentType, ok := contentMap["type"].(string); ok && contentType == "output_text" {
					if text, ok := contentMap["text"].(string); ok {
						result.WriteString(text)
						result.WriteString("\n")
					}
				}
			}
		}
	}

	return result.String(), nil
}
