package models

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kris-hansen/comanda/utils/fileutil"
	"github.com/kris-hansen/comanda/utils/retry"
	openai "github.com/sashabaranov/go-openai"
)

// MoonshotProvider handles Moonshot family of models
type MoonshotProvider struct {
	apiKey  string
	config  ModelConfig
	verbose bool
	mu      sync.Mutex
}

// NewMoonshotProvider creates a new Moonshot provider instance
func NewMoonshotProvider() *MoonshotProvider {
	return &MoonshotProvider{
		config: ModelConfig{
			Temperature:         0.3, // Default to 0.3 as recommended in the Moonshot documentation
			MaxTokens:           2000,
			MaxCompletionTokens: 2000,
			TopP:                1.0,
		},
	}
}

// Name returns the provider name
func (o *MoonshotProvider) Name() string {
	return "moonshot"
}

// debugf prints debug information if verbose mode is enabled (thread-safe)
func (o *MoonshotProvider) debugf(format string, args ...interface{}) {
	if o.verbose {
		o.mu.Lock()
		defer o.mu.Unlock()
		log.Printf("[DEBUG][Moonshot] "+format+"\n", args...)
	}
}

// SupportsModel checks if the given model name is supported by Moonshot
func (o *MoonshotProvider) SupportsModel(modelName string) bool {
	o.debugf("Checking if model is supported: %s", modelName)
	modelName = strings.ToLower(modelName)

	// Register Moonshot model families if not already done
	registry := GetRegistry()
	if len(registry.GetFamilies("moonshot")) == 0 {
		registry.RegisterFamilies("moonshot", []string{
			"moonshot-", // Standard Moonshot models
		})
	}

	// Use the central model registry for validation
	for _, prefix := range registry.GetFamilies("moonshot") {
		if strings.HasPrefix(modelName, prefix) {
			o.debugf("Model %s is supported (matches prefix %s)", modelName, prefix)
			return true
		}
	}

	// Also check exact matches in the registry
	for _, model := range registry.GetModels("moonshot") {
		if modelName == model {
			o.debugf("Model %s is supported (exact match)", modelName)
			return true
		}
	}

	o.debugf("Model %s is not supported (no matching prefix or exact match)", modelName)
	return false
}

// Configure sets up the provider with necessary credentials
func (o *MoonshotProvider) Configure(apiKey string) error {
	o.debugf("Configuring Moonshot provider")
	if apiKey == "" {
		return fmt.Errorf("API key is required for Moonshot provider")
	}
	o.apiKey = apiKey
	o.debugf("API key configured successfully")
	return nil
}

// createChatCompletionRequest creates a ChatCompletionRequest with the appropriate parameters
func (o *MoonshotProvider) createChatCompletionRequest(modelName string, messages []openai.ChatCompletionMessage) openai.ChatCompletionRequest {
	req := openai.ChatCompletionRequest{
		Model:    modelName,
		Messages: messages,
	}

	// Moonshot API only supports temperature in range [0, 1]
	temperature := float32(o.config.Temperature)
	if temperature > 1.0 {
		temperature = 1.0
	}

	req.MaxTokens = o.config.MaxTokens
	req.Temperature = temperature
	req.TopP = float32(o.config.TopP)
	o.debugf("Using configured parameters: Temperature=%.2f, TopP=%.2f", temperature, o.config.TopP)

	return req
}

// SendPrompt sends a prompt to the specified model and returns the response
func (o *MoonshotProvider) SendPrompt(modelName string, prompt string) (string, error) {
	o.debugf("Preparing to send prompt to model: %s", modelName)
	o.debugf("Prompt length: %d characters", len(prompt))

	if o.apiKey == "" {
		return "", fmt.Errorf("Moonshot provider not configured: missing API key")
	}

	if !o.SupportsModel(modelName) {
		return "", fmt.Errorf("invalid Moonshot model: %s", modelName)
	}

	o.debugf("Model validation passed, preparing API call")

	// Create a custom client with the Moonshot base URL
	config := openai.DefaultConfig(o.apiKey)
	config.BaseURL = "https://api.moonshot.ai/v1"
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

			req := o.createChatCompletionRequest(modelName, messages)
			resp, err := client.CreateChatCompletion(context.Background(), req)

			if err != nil {
				return "", fmt.Errorf("Moonshot API error: %v", err)
			}

			if len(resp.Choices) == 0 {
				return "", fmt.Errorf("no response choices returned from Moonshot")
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
	o.debugf("API call completed, response length: %d characters", len(response))

	return response, nil
}

// SendPromptWithFile sends a prompt along with a file to the specified model and returns the response
func (o *MoonshotProvider) SendPromptWithFile(modelName string, prompt string, file FileInput) (string, error) {
	o.debugf("Preparing to send prompt with file to model: %s", modelName)
	o.debugf("File path: %s", file.Path)

	if o.apiKey == "" {
		return "", fmt.Errorf("Moonshot provider not configured: missing API key")
	}

	if !o.SupportsModel(modelName) {
		return "", fmt.Errorf("invalid Moonshot model: %s", modelName)
	}

	// Read the file content with size check - do this outside the retry loop
	fileData, err := fileutil.SafeReadFile(file.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	// Create a custom client with the Moonshot base URL
	config := openai.DefaultConfig(o.apiKey)
	config.BaseURL = "https://api.moonshot.ai/v1"
	client := openai.NewClientWithConfig(config)

	// Include the file content as part of the prompt
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

			req := o.createChatCompletionRequest(modelName, messages)
			resp, err := client.CreateChatCompletion(context.Background(), req)

			if err != nil {
				return "", fmt.Errorf("Moonshot API error: %v", err)
			}

			if len(resp.Choices) == 0 {
				return "", fmt.Errorf("no response choices returned from Moonshot")
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
	o.debugf("API call completed, response length: %d characters", len(response))

	return response, nil
}

// ValidateModel checks if the specific Moonshot model variant is valid
func (o *MoonshotProvider) ValidateModel(modelName string) bool {
	o.debugf("Validating model: %s", modelName)

	// Use the central model registry for validation
	isValid := GetRegistry().ValidateModel("moonshot", modelName)

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

// SetConfig updates the provider configuration
func (o *MoonshotProvider) SetConfig(config ModelConfig) {
	o.debugf("Updating provider configuration")
	o.debugf("Old config: Temperature=%.2f, MaxTokens=%d, MaxCompletionTokens=%d, TopP=%.2f",
		o.config.Temperature, o.config.MaxTokens, o.config.MaxCompletionTokens, o.config.TopP)
	o.config = config
	o.debugf("New config: Temperature=%.2f, MaxTokens=%d, MaxCompletionTokens=%d, TopP=%.2f",
		o.config.Temperature, o.config.MaxTokens, o.config.MaxCompletionTokens, o.config.TopP)
}

// GetConfig returns the current provider configuration
func (o *MoonshotProvider) GetConfig() ModelConfig {
	return o.config
}

// SetVerbose enables or disables verbose mode
func (o *MoonshotProvider) SetVerbose(verbose bool) {
	o.verbose = verbose
}

// prepareResponsesRequestBody prepares the request body for the Responses API
func (o *MoonshotProvider) prepareResponsesRequestBody(config ResponsesConfig) (map[string]interface{}, error) {
	// Build the request body
	requestBody := map[string]interface{}{
		"model": config.Model,
		"input": config.Input,
	}

	// Add stream parameter if specified
	if config.Stream {
		requestBody["stream"] = true
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
		// Ensure temperature is within [0, 1] for Moonshot
		temperature := config.Temperature
		if temperature > 1.0 {
			temperature = 1.0
		}
		requestBody["temperature"] = temperature
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
		// For the Responses API, the response_format parameter has moved to text.format
		// Check if the ResponseFormat has a "type" key with value "text"
		if responseType, ok := config.ResponseFormat["type"].(string); ok && responseType == "text" {
			// Create a text object with the format
			requestBody["text"] = map[string]interface{}{
				"format": map[string]interface{}{
					"type": "text",
				},
			}
		} else if responseType, ok := config.ResponseFormat["type"].(string); ok && responseType == "json_object" {
			// For JSON format
			requestBody["text"] = map[string]interface{}{
				"format": map[string]interface{}{
					"type": "json_object",
				},
			}
		} else {
			// For other formats, create a proper format object
			requestBody["text"] = map[string]interface{}{
				"format": config.ResponseFormat,
			}
		}
		// Log the formatted request body for debugging
		textFormatBytes, _ := json.Marshal(requestBody["text"])
		o.debugf("Using text format in request body: %s", string(textFormatBytes))
	}

	return requestBody, nil
}

// SendPromptWithResponses sends a prompt using the Moonshot Responses API
func (o *MoonshotProvider) SendPromptWithResponses(config ResponsesConfig) (string, error) {
	o.debugf("Preparing to send prompt using Responses API with model: %s", config.Model)

	if o.apiKey == "" {
		return "", fmt.Errorf("Moonshot provider not configured: missing API key")
	}

	if !o.SupportsModel(config.Model) {
		return "", fmt.Errorf("invalid Moonshot model: %s", config.Model)
	}

	// Prepare request body
	requestBody, err := o.prepareResponsesRequestBody(config)
	if err != nil {
		return "", fmt.Errorf("failed to prepare request body: %w", err)
	}

	// Convert request body to JSON
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Create HTTP request with context for timeout
	timeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Use our generic retry mechanism instead of custom implementation
	result, err := retry.WithRetry(
		func() (interface{}, error) {
			req, err := http.NewRequestWithContext(ctx, "POST", "https://api.moonshot.ai/v1/responses", bytes.NewBuffer(jsonData))
			if err != nil {
				return nil, fmt.Errorf("failed to create HTTP request: %w", err)
			}

			// Set headers
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", o.apiKey))

			// Send request
			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				return nil, fmt.Errorf("failed to send HTTP request: %w", err)
			}
			defer resp.Body.Close()

			// Read response body
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to read response body: %w", err)
			}

			// Check for rate limit errors (429)
			if resp.StatusCode == http.StatusTooManyRequests {
				return nil, fmt.Errorf("API request failed with status 429: %s", string(body))
			}

			// Check for error status code
			if resp.StatusCode != http.StatusOK {
				// Don't retry on 4xx errors (client errors) except 429
				if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
					return nil, fmt.Errorf("Moonshot API error: %s (status code: %d)", string(body), resp.StatusCode)
				}

				return nil, fmt.Errorf("Moonshot API error: %s (status code: %d)", string(body), resp.StatusCode)
			}

			// Parse response
			var responseData map[string]interface{}
			if err := json.Unmarshal(body, &responseData); err != nil {
				return nil, fmt.Errorf("failed to parse response body: %w", err)
			}

			return responseData, nil
		},
		retry.Is429Error,
		retry.DefaultRetryConfig,
	)

	if err != nil {
		return "", err
	}

	responseData := result.(map[string]interface{})

	// Extract output text
	output, err := o.extractOutputText(responseData)
	if err != nil {
		return "", fmt.Errorf("failed to extract output text: %w", err)
	}

	o.debugf("API call completed, response length: %d characters", len(output))

	return output, nil
}

// SendPromptWithResponsesStream sends a prompt using the Moonshot Responses API with streaming
func (o *MoonshotProvider) SendPromptWithResponsesStream(config ResponsesConfig, handler ResponsesStreamHandler) error {
	o.debugf("Preparing to send prompt using Responses API with streaming for model: %s", config.Model)

	if o.apiKey == "" {
		return fmt.Errorf("Moonshot provider not configured: missing API key")
	}

	if !o.SupportsModel(config.Model) {
		return fmt.Errorf("invalid Moonshot model: %s", config.Model)
	}

	// Force streaming to be enabled
	config.Stream = true

	// Prepare request body
	requestBody, err := o.prepareResponsesRequestBody(config)
	if err != nil {
		return fmt.Errorf("failed to prepare request body: %w", err)
	}

	// Convert request body to JSON
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Create HTTP request with context for timeout
	timeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.moonshot.ai/v1/responses", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", o.apiKey))
	req.Header.Set("Accept", "text/event-stream")

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Check for error status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Moonshot API error: %s (status code: %d)", string(body), resp.StatusCode)
	}

	// Process the stream using a scanner
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		// Skip "data: " prefix
		if strings.HasPrefix(line, "data: ") {
			line = strings.TrimPrefix(line, "data: ")
		}

		// Skip "[DONE]" message
		if line == "[DONE]" {
			break
		}

		// Parse the event
		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			handler.OnError(fmt.Errorf("failed to parse event: %w", err))
			continue
		}

		// Process based on event type
		eventType, ok := event["type"].(string)
		if !ok {
			continue
		}

		switch eventType {
		case "response.created":
			if resp, ok := event["response"].(map[string]interface{}); ok {
				handler.OnResponseCreated(resp)
			}
		case "response.in_progress":
			if resp, ok := event["response"].(map[string]interface{}); ok {
				handler.OnResponseInProgress(resp)
			}
		case "response.output_item.added":
			if index, ok := event["output_index"].(float64); ok {
				if item, ok := event["item"].(map[string]interface{}); ok {
					handler.OnOutputItemAdded(int(index), item)
				}
			}
		case "response.output_text.delta":
			itemID, _ := event["item_id"].(string)
			index, _ := event["output_index"].(float64)
			contentIndex, _ := event["content_index"].(float64)
			delta, _ := event["delta"].(string)
			handler.OnOutputTextDelta(itemID, int(index), int(contentIndex), delta)
		case "response.completed":
			if resp, ok := event["response"].(map[string]interface{}); ok {
				handler.OnResponseCompleted(resp)
			}
			return nil // End streaming
		case "error":
			message, _ := event["message"].(string)
			handler.OnError(fmt.Errorf("stream error: %s", message))
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stream read error: %w", err)
	}

	return nil
}

// extractOutputText extracts the output text from the Responses API response
func (o *MoonshotProvider) extractOutputText(responseData map[string]interface{}) (string, error) {
	// Debug the full response
	responseBytes, _ := json.MarshalIndent(responseData, "", "  ")
	o.debugf("Full response: %s", string(responseBytes))

	// Check if output exists
	output, ok := responseData["output"]
	if !ok {
		// If no output field, check for other fields that might contain the response
		if text, ok := responseData["text"].(string); ok {
			return text, nil
		}
		if content, ok := responseData["content"].(string); ok {
			return content, nil
		}
		if message, ok := responseData["message"].(string); ok {
			return message, nil
		}

		// Return the entire response as a string if we can't find a specific field
		return string(responseBytes), nil
	}

	// Output is an array of content items
	outputArray, ok := output.([]interface{})
	if !ok {
		// If output is not an array, try to convert it to a string
		if outputStr, ok := output.(string); ok {
			return outputStr, nil
		}

		// If output is a map, try to extract text from it
		if outputMap, ok := output.(map[string]interface{}); ok {
			if text, ok := outputMap["text"].(string); ok {
				return text, nil
			}
		}

		// Return the output as a JSON string
		outputBytes, _ := json.MarshalIndent(output, "", "  ")
		return string(outputBytes), nil
	}

	// Process each output item
	var result strings.Builder
	var annotations []map[string]interface{}

	// First pass: collect all text content and annotations
	for _, item := range outputArray {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		// Skip web_search_call items
		if itemType, ok := itemMap["type"].(string); ok && itemType == "web_search_call" {
			o.debugf("Skipping web_search_call item")
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
						o.debugf("Extracted text from output_text: %d characters", len(text))

						// Collect annotations if present
						if annotationsArray, ok := contentMap["annotations"].([]interface{}); ok && len(annotationsArray) > 0 {
							o.debugf("Found %d annotations", len(annotationsArray))
							for _, anno := range annotationsArray {
								if annoMap, ok := anno.(map[string]interface{}); ok {
									annotations = append(annotations, annoMap)
								}
							}
						}
					}
				}
			}
		} else {
			// Try to extract text from the item
			if text, ok := itemMap["text"].(string); ok {
				result.WriteString(text)
				result.WriteString("\n")
				o.debugf("Extracted text from item: %d characters", len(text))
			} else if content, ok := itemMap["content"].(string); ok {
				result.WriteString(content)
				result.WriteString("\n")
				o.debugf("Extracted content from item: %d characters", len(content))
			}
		}
	}

	// If we didn't extract any text, try a more aggressive approach
	if result.Len() == 0 {
		o.debugf("No text extracted using standard approach, trying recursive extraction")
		extractedText := o.recursiveExtractText(responseData)
		if extractedText != "" {
			result.WriteString(extractedText)
			o.debugf("Extracted text using recursive approach: %d characters", len(extractedText))
		}
	}

	// If we still didn't extract any text, return the entire response as a string
	if result.Len() == 0 {
		o.debugf("No text extracted, returning entire response")
		return string(responseBytes), nil
	}

	// Add annotations as footnotes if present
	if len(annotations) > 0 {
		result.WriteString("\n\n## References\n")
		for i, anno := range annotations {
			annoType, _ := anno["type"].(string)
			if annoType == "url_citation" {
				url, _ := anno["url"].(string)
				title, _ := anno["title"].(string)
				result.WriteString(fmt.Sprintf("%d. [%s](%s)\n", i+1, title, url))
			}
		}
	}

	o.debugf("Total extracted text: %d characters", result.Len())
	return result.String(), nil
}

// recursiveExtractText recursively searches for text content in a nested structure
func (o *MoonshotProvider) recursiveExtractText(data interface{}) string {
	var result strings.Builder

	switch v := data.(type) {
	case map[string]interface{}:
		// Check for common text fields
		if text, ok := v["text"].(string); ok {
			result.WriteString(text)
			result.WriteString("\n")
			return result.String()
		}

		// Check for content field
		if content, ok := v["content"].(string); ok {
			result.WriteString(content)
			result.WriteString("\n")
			return result.String()
		}

		// Recursively search all fields
		for _, value := range v {
			text := o.recursiveExtractText(value)
			if text != "" {
				result.WriteString(text)
			}
		}
	case []interface{}:
		// Recursively search array elements
		for _, item := range v {
			text := o.recursiveExtractText(item)
			if text != "" {
				result.WriteString(text)
			}
		}
	}

	return result.String()
}
