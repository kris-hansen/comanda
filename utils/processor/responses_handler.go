package processor

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/kris-hansen/comanda/utils/models"
)

// responsesStreamHandler implements the ResponsesStreamHandler interface
type responsesStreamHandler struct {
	processor      *Processor
	stepName       string
	isParallel     bool
	parallelID     string
	responseBuffer *strings.Builder
	currentText    strings.Builder
}

// OnResponseCreated handles the response.created event
func (h *responsesStreamHandler) OnResponseCreated(response map[string]interface{}) {
	h.processor.debugf("[%s] Response created", h.stepName)

	// Send progress update
	h.processor.sendProgressUpdate(ProgressUpdate{
		Type:       ProgressStep,
		Message:    "Response created",
		Step:       &StepInfo{Name: h.stepName},
		IsParallel: h.isParallel,
		ParallelID: h.parallelID,
	})

	// Extract response ID if available
	if id, ok := response["id"].(string); ok && id != "" {
		// Store in variables map
		h.processor.variables[h.stepName+".response_id"] = id
		h.processor.debugf("[%s] Stored response ID: %s", h.stepName, id)
	}
}

// OnResponseInProgress handles the response.in_progress event
func (h *responsesStreamHandler) OnResponseInProgress(response map[string]interface{}) {
	h.processor.debugf("[%s] Response in progress", h.stepName)

	// Send progress update
	h.processor.sendProgressUpdate(ProgressUpdate{
		Type:       ProgressStep,
		Message:    "Processing...",
		Step:       &StepInfo{Name: h.stepName},
		IsParallel: h.isParallel,
		ParallelID: h.parallelID,
	})
}

// OnOutputItemAdded handles the response.output_item.added event
func (h *responsesStreamHandler) OnOutputItemAdded(index int, item map[string]interface{}) {
	h.processor.debugf("[%s] Output item added at index %d", h.stepName, index)

	// Send progress update
	h.processor.sendProgressUpdate(ProgressUpdate{
		Type:       ProgressStep,
		Message:    fmt.Sprintf("Generating output item %d", index+1),
		Step:       &StepInfo{Name: h.stepName},
		IsParallel: h.isParallel,
		ParallelID: h.parallelID,
	})
}

// OnOutputTextDelta handles the response.output_text.delta event
func (h *responsesStreamHandler) OnOutputTextDelta(itemID string, index int, contentIndex int, delta string) {
	// Append to current text
	h.currentText.WriteString(delta)

	// Only send progress updates periodically to avoid flooding
	if h.currentText.Len()%100 == 0 {
		h.processor.debugf("[%s] Received %d characters", h.stepName, h.currentText.Len())

		// Send progress update
		h.processor.sendProgressUpdate(ProgressUpdate{
			Type:       ProgressStep,
			Message:    fmt.Sprintf("Received %d characters", h.currentText.Len()),
			Step:       &StepInfo{Name: h.stepName},
			IsParallel: h.isParallel,
			ParallelID: h.parallelID,
		})
	}
}

// OnResponseCompleted handles the response.completed event
func (h *responsesStreamHandler) OnResponseCompleted(response map[string]interface{}) {
	h.processor.debugf("[%s] Response completed", h.stepName)

	// Extract the final text from the response
	output, err := h.processor.extractOutputTextFromResponse(response)
	if err == nil {
		h.responseBuffer.WriteString(output)
	} else {
		// If extraction fails, use the accumulated text
		h.responseBuffer.WriteString(h.currentText.String())
	}

	// Send progress update
	h.processor.sendProgressUpdate(ProgressUpdate{
		Type:       ProgressComplete,
		Message:    "Response completed",
		Step:       &StepInfo{Name: h.stepName},
		IsParallel: h.isParallel,
		ParallelID: h.parallelID,
	})
}

// OnError handles stream errors
func (h *responsesStreamHandler) OnError(err error) {
	h.processor.debugf("[%s] Stream error: %v", h.stepName, err)

	// Send progress update
	h.processor.sendProgressUpdate(ProgressUpdate{
		Type:       ProgressError,
		Message:    fmt.Sprintf("Stream error: %v", err),
		Error:      err,
		Step:       &StepInfo{Name: h.stepName},
		IsParallel: h.isParallel,
		ParallelID: h.parallelID,
	})
}

// extractOutputTextFromResponse extracts the output text from a response object
func (p *Processor) extractOutputTextFromResponse(responseData map[string]interface{}) (string, error) {
	// Convert to JSON and back to ensure consistent handling
	responseBytes, err := json.Marshal(responseData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal response: %w", err)
	}

	// Parse the response
	var parsedResponse map[string]interface{}
	if err := json.Unmarshal(responseBytes, &parsedResponse); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	// Check if output exists
	output, ok := parsedResponse["output"]
	if !ok {
		// If no output field, check for other fields that might contain the response
		if text, ok := parsedResponse["text"].(string); ok {
			return text, nil
		}
		if content, ok := parsedResponse["content"].(string); ok {
			return content, nil
		}
		if message, ok := parsedResponse["message"].(string); ok {
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
		outputBytes, _ := json.Marshal(output)
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
						// Collect annotations if present
						if annotationsArray, ok := contentMap["annotations"].([]interface{}); ok && len(annotationsArray) > 0 {
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
			} else if content, ok := itemMap["content"].(string); ok {
				result.WriteString(content)
				result.WriteString("\n")
			}
		}
	}

	// If we didn't extract any text, try a more aggressive approach
	if result.Len() == 0 {
		extractedText := p.recursiveExtractText(parsedResponse)
		if extractedText != "" {
			result.WriteString(extractedText)
		}
	}

	// If we still didn't extract any text, return the entire response as a string
	if result.Len() == 0 {
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

	return result.String(), nil
}

// recursiveExtractText recursively searches for text content in a nested structure
func (p *Processor) recursiveExtractText(data interface{}) string {
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
			text := p.recursiveExtractText(value)
			if text != "" {
				result.WriteString(text)
			}
		}
	case []interface{}:
		// Recursively search array elements
		for _, item := range v {
			text := p.recursiveExtractText(item)
			if text != "" {
				result.WriteString(text)
			}
		}
	}

	return result.String()
}

// extractResponseID extracts the response ID from a response string or object
func (p *Processor) extractResponseID(response string) (string, error) {
	// Try to parse as JSON first
	var responseData map[string]interface{}
	if err := json.Unmarshal([]byte(response), &responseData); err != nil {
		// Not valid JSON, try to extract using regex
		re := regexp.MustCompile(`"id":\s*"(resp_[^"]+)"`)
		matches := re.FindStringSubmatch(response)
		if len(matches) > 1 {
			return matches[1], nil
		}
		return "", fmt.Errorf("could not extract response ID")
	}

	// Extract from parsed JSON
	if id, ok := responseData["id"].(string); ok {
		return id, nil
	}

	return "", fmt.Errorf("response ID not found in JSON")
}

// sendProgressUpdate sends a progress update
func (p *Processor) sendProgressUpdate(update ProgressUpdate) {
	if update.IsParallel {
		if update.Type == ProgressComplete {
			p.emitParallelProgressWithMetrics(update.Message, update.Step, update.ParallelID, update.PerformanceMetrics)
		} else if update.Type == ProgressError {
			p.emitError(update.Error)
		} else {
			p.emitParallelProgress(update.Message, update.Step, update.ParallelID)
		}
	} else {
		if update.Type == ProgressComplete {
			p.emitProgressWithMetrics(update.Message, update.Step, update.PerformanceMetrics)
		} else if update.Type == ProgressError {
			p.emitError(update.Error)
		} else {
			p.emitProgress(update.Message, update.Step)
		}
	}
}

// processResponsesStep handles the openai-responses step type
func (p *Processor) processResponsesStep(step Step, isParallel bool, parallelID string) (string, error) {
	p.debugf("Processing openai-responses step: %s", step.Name)

	startTime := time.Now()

	// Send initial progress update
	p.sendProgressUpdate(ProgressUpdate{
		Type:       ProgressStep,
		Message:    fmt.Sprintf("Starting responses step: %s", step.Name),
		Step:       &StepInfo{Name: step.Name, Instructions: step.Config.Instructions},
		IsParallel: isParallel,
		ParallelID: parallelID,
	})

	// Get the model name
	modelNames := p.NormalizeStringSlice(step.Config.Model)
	if len(modelNames) == 0 {
		return "", fmt.Errorf("no model specified for openai-responses step")
	}
	modelName := modelNames[0]

	// Get the OpenAI provider
	provider := models.DetectProvider(modelName)
	if provider == nil || provider.Name() != "openai" {
		return "", fmt.Errorf("openai-responses step requires an OpenAI model, got: %s", modelName)
	}

	// Check if this is a model that requires the responses API
	isResponsesAPIModel := strings.HasPrefix(modelName, "o1-pro") ||
		strings.HasPrefix(modelName, "o3-") ||
		strings.HasPrefix(modelName, "o4-")

	// Log a warning if a model requires the responses API but doesn't have a response format
	if isResponsesAPIModel && step.Config.ResponseFormat == nil {
		p.debugf("Warning: Model %s requires the responses API but no response_format is specified", modelName)
	} else if step.Config.ResponseFormat != nil {
		responseFormatBytes, _ := json.Marshal(step.Config.ResponseFormat)
		p.debugf("Response format for model %s: %s", modelName, string(responseFormatBytes))
	}

	// Add more detailed logging about the response format
	if step.Config.ResponseFormat != nil {
		p.debugf("Response format type: %T", step.Config.ResponseFormat)
		p.debugf("Response format value: %+v", step.Config.ResponseFormat)

		// Log each key-value pair in the response format
		// Get all keys
		var keys []string
		for k := range step.Config.ResponseFormat {
			keys = append(keys, k)
		}
		p.debugf("Response format keys: %v", keys)

		// Log each key-value pair
		for k, v := range step.Config.ResponseFormat {
			p.debugf("Response format key '%s' has value: %v (type: %T)", k, v, v)
		}
	} else {
		p.debugf("No response_format specified for model %s", modelName)
	}

	// Configure providers
	if err := p.configureProviders(); err != nil {
		return "", fmt.Errorf("provider configuration error: %w", err)
	}

	// Use the configured provider instance
	configuredProvider, err := p.getProviderForModel(modelName)
	if err != nil {
		return "", fmt.Errorf("failed to get provider for model %s: %w", modelName, err)
	}
	if configuredProvider == nil {
		return "", fmt.Errorf("OpenAI provider not configured")
	}

	// Check if the provider implements ResponsesProvider interface
	responsesProvider, ok := configuredProvider.(models.ResponsesProvider)
	if !ok {
		return "", fmt.Errorf("provider %s does not support Responses API", provider.Name())
	}

	// Process input files
	inputs := p.NormalizeStringSlice(step.Config.Input)
	p.debugf("Processing inputs for step %s: %v", step.Name, inputs)

	// Check if input is NA, which means we should skip input processing
	isNAInput := len(inputs) == 1 && inputs[0] == "NA"
	p.debugf("Input is NA: %v", isNAInput)

	if len(inputs) > 0 && !isNAInput {
		if err := p.processInputs(inputs); err != nil {
			return "", fmt.Errorf("input processing error in step %s: %w", step.Name, err)
		}
	}

	// Get processed inputs
	processedInputs := p.handler.GetInputs()
	if len(processedInputs) == 0 && !isNAInput {
		return "", fmt.Errorf("no inputs provided for openai-responses step")
	}

	// Combine input contents
	var inputContents []string
	for _, input := range processedInputs {
		inputContents = append(inputContents, string(input.Contents))
	}
	combinedInput := strings.Join(inputContents, "\n\n")

	// If input is NA, use a default input
	if isNAInput {
		combinedInput = "Please follow the instructions."
		p.debugf("Using default input for NA input: %s", combinedInput)
	}

	// Get actions
	actions := p.NormalizeStringSlice(step.Config.Action)

	// For openai-responses type, instructions can be used instead of actions
	if len(actions) == 0 && step.Config.Instructions == "" {
		return "", fmt.Errorf("no actions or instructions provided for openai-responses step")
	}

	// Create the final prompt
	var prompt string
	if len(actions) > 0 {
		// Combine actions
		combinedAction := strings.Join(actions, "\n")
		prompt = fmt.Sprintf("Input:\n%s\n\nAction: %s", combinedInput, combinedAction)
		p.debugf("Using action-based prompt format")
	} else {
		// Use instructions-only format
		prompt = combinedInput
		p.debugf("Using instructions-only prompt format (no actions)")
	}

	// Create ResponsesConfig
	config := models.ResponsesConfig{
		Model:              modelName,
		Input:              prompt,
		Instructions:       step.Config.Instructions,
		PreviousResponseID: step.Config.PreviousResponseID,
		MaxOutputTokens:    step.Config.MaxOutputTokens,
		Temperature:        step.Config.Temperature,
		TopP:               step.Config.TopP,
		Stream:             step.Config.Stream,
		Tools:              step.Config.Tools,
	}

	// Log the configuration details for debugging
	p.debugf("ResponsesConfig details:")
	p.debugf("- Model: %s", config.Model)
	p.debugf("- Instructions: %s", config.Instructions)
	p.debugf("- MaxOutputTokens: %d", config.MaxOutputTokens)
	p.debugf("- Temperature: %f", config.Temperature)
	p.debugf("- TopP: %f", config.TopP)
	p.debugf("- Stream: %v", config.Stream)
	p.debugf("- Has Tools: %v", config.Tools != nil)
	p.debugf("- Has PreviousResponseID: %v", config.PreviousResponseID != "")

	// Note: For models that are known to be slow (o1-pro, o3, o4),
	// the timeout is handled in the OpenAI provider's SendPromptWithResponsesStream
	// and SendPromptWithResponses methods, which use a 5-minute timeout
	// for the context when making API requests.

	// If response format is specified, add it
	if step.Config.ResponseFormat != nil {
		config.ResponseFormat = step.Config.ResponseFormat
		responseFormatBytes, _ := json.Marshal(config.ResponseFormat)
		p.debugf("Setting response_format: %s", string(responseFormatBytes))
	} else {
		p.debugf("No response_format specified in config")
	}

	// Send progress update
	var actionInfo string
	if len(actions) > 0 {
		actionInfo = strings.Join(actions, "\n")
	} else {
		actionInfo = "(using instructions only)"
	}

	p.sendProgressUpdate(ProgressUpdate{
		Type:       ProgressStep,
		Message:    fmt.Sprintf("Sending request to %s", modelName),
		Step:       &StepInfo{Name: step.Name, Model: modelName, Action: actionInfo, Instructions: step.Config.Instructions},
		IsParallel: isParallel,
		ParallelID: parallelID,
	})

	var response string

	// Check if streaming is enabled
	if step.Config.Stream {
		p.debugf("[%s] Using streaming mode", step.Name)

		// Create a buffer to collect the response
		var responseBuffer strings.Builder

		// Create a stream handler
		streamHandler := &responsesStreamHandler{
			processor:      p,
			stepName:       step.Name,
			isParallel:     isParallel,
			parallelID:     parallelID,
			responseBuffer: &responseBuffer,
		}

		// Send the request with streaming
		err = responsesProvider.SendPromptWithResponsesStream(config, streamHandler)
		if err != nil {
			return "", fmt.Errorf("streaming error: %w", err)
		}

		response = responseBuffer.String()
	} else {
		// Non-streaming path
		response, err = responsesProvider.SendPromptWithResponses(config)
		if err != nil {
			return "", err
		}

		// Extract and store the response ID for potential future use
		responseID, extractErr := p.extractResponseID(response)
		if extractErr == nil && responseID != "" {
			// Store in variables map
			p.variables[step.Name+".response_id"] = responseID
			p.debugf("[%s] Stored response ID: %s", step.Name, responseID)
		}
	}

	// Calculate performance metrics
	elapsedTime := time.Since(startTime)
	metrics := &PerformanceMetrics{
		TotalProcessingTime: elapsedTime.Milliseconds(),
	}

	// Send completion progress update
	p.sendProgressUpdate(ProgressUpdate{
		Type:               ProgressComplete,
		Message:            fmt.Sprintf("Completed responses step: %s", step.Name),
		Step:               &StepInfo{Name: step.Name, Model: modelName, Action: actionInfo, Instructions: step.Config.Instructions},
		IsParallel:         isParallel,
		ParallelID:         parallelID,
		PerformanceMetrics: metrics,
	})

	// Get outputs
	outputs := p.NormalizeStringSlice(step.Config.Output)
	p.debugf("Processing output for responses step '%s': model=%s outputs=%v",
		step.Name, modelName, outputs)

	// Extract the text from the response using our improved extraction function
	var responseData map[string]interface{}
	if err := json.Unmarshal([]byte(response), &responseData); err != nil {
		// If we can't parse the response, use it as-is
		if err := p.handleOutput(modelName, response, outputs, metrics); err != nil {
			errMsg := fmt.Sprintf("Output processing failed for responses step '%s': %v (model=%s outputs=%v)",
				step.Name, err, modelName, outputs)
			p.debugf("Output processing error: %s", errMsg)
			return "", fmt.Errorf("output handling error: %w", err)
		}
	} else {
		// Extract text using our improved function
		extractedText, err := p.extractOutputTextFromResponse(responseData)
		if err != nil {
			// If extraction fails, use the original response
			if err := p.handleOutput(modelName, response, outputs, metrics); err != nil {
				errMsg := fmt.Sprintf("Output processing failed for responses step '%s': %v (model=%s outputs=%v)",
					step.Name, err, modelName, outputs)
				p.debugf("Output processing error: %s", errMsg)
				return "", fmt.Errorf("output handling error: %w", err)
			}
		} else {
			// Use the extracted text
			if err := p.handleOutput(modelName, extractedText, outputs, metrics); err != nil {
				errMsg := fmt.Sprintf("Output processing failed for responses step '%s': %v (model=%s outputs=%v)",
					step.Name, err, modelName, outputs)
				p.debugf("Output processing error: %s", errMsg)
				return "", fmt.Errorf("output handling error: %w", err)
			}
		}
	}
	p.debugf("Successfully processed output for responses step: %s", step.Name)

	return response, nil
}
