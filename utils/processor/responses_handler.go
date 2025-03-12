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

	// Debug the full response
	fmt.Println("\n==== DEBUG: Full Response Structure ====")
	fmt.Println(string(responseBytes))
	fmt.Println("==== END FULL RESPONSE ====\n")

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

	// Debug the output array
	fmt.Println("\n==== DEBUG: Output Array Structure ====")
	for i, item := range outputArray {
		itemBytes, _ := json.Marshal(item)
		fmt.Printf("Item %d: %s\n", i, string(itemBytes))
	}
	fmt.Println("==== END DEBUG ====\n")

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
			fmt.Println("\nDEBUG: Skipping web_search_call item")
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
						fmt.Printf("\nDEBUG: Extracted text from output_text: %d characters\n", len(text))

						// Collect annotations if present
						if annotationsArray, ok := contentMap["annotations"].([]interface{}); ok && len(annotationsArray) > 0 {
							fmt.Printf("\nDEBUG: Found %d annotations\n", len(annotationsArray))
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
				fmt.Printf("\nDEBUG: Extracted text from item: %d characters\n", len(text))
			} else if content, ok := itemMap["content"].(string); ok {
				result.WriteString(content)
				result.WriteString("\n")
				fmt.Printf("\nDEBUG: Extracted content from item: %d characters\n", len(content))
			}
		}
	}

	// If we didn't extract any text, try a more aggressive approach
	if result.Len() == 0 {
		fmt.Println("\nDEBUG: No text extracted using standard approach, trying recursive extraction")
		extractedText := p.recursiveExtractText(parsedResponse)
		if extractedText != "" {
			result.WriteString(extractedText)
			fmt.Printf("\nDEBUG: Extracted text using recursive approach: %d characters\n", len(extractedText))
		}
	}

	// If we still didn't extract any text, return the entire response as a string
	if result.Len() == 0 {
		fmt.Println("\nDEBUG: No text extracted, returning entire response")
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

	fmt.Printf("\nDEBUG: Total extracted text: %d characters\n", result.Len())
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
		Step:       &StepInfo{Name: step.Name},
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

	// Configure providers
	if err := p.configureProviders(); err != nil {
		return "", fmt.Errorf("provider configuration error: %w", err)
	}

	// Use the configured provider instance
	configuredProvider := p.providers[provider.Name()]
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

	if len(inputs) > 0 {
		if err := p.processInputs(inputs); err != nil {
			return "", fmt.Errorf("input processing error in step %s: %w", step.Name, err)
		}
	}

	// Get processed inputs
	processedInputs := p.handler.GetInputs()
	if len(processedInputs) == 0 {
		return "", fmt.Errorf("no inputs provided for openai-responses step")
	}

	// Combine input contents
	var inputContents []string
	for _, input := range processedInputs {
		inputContents = append(inputContents, string(input.Contents))
	}
	combinedInput := strings.Join(inputContents, "\n\n")

	// Get actions
	actions := p.NormalizeStringSlice(step.Config.Action)
	if len(actions) == 0 {
		return "", fmt.Errorf("no actions provided for openai-responses step")
	}

	// Combine actions
	combinedAction := strings.Join(actions, "\n")

	// Create the final prompt
	prompt := fmt.Sprintf("Input:\n%s\n\nAction: %s", combinedInput, combinedAction)

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

	// If response format is specified, add it
	if step.Config.ResponseFormat != nil {
		config.ResponseFormat = step.Config.ResponseFormat
	}

	// Send progress update
	p.sendProgressUpdate(ProgressUpdate{
		Type:       ProgressStep,
		Message:    fmt.Sprintf("Sending request to %s", modelName),
		Step:       &StepInfo{Name: step.Name, Model: modelName, Action: combinedAction},
		IsParallel: isParallel,
		ParallelID: parallelID,
	})

	var response string
	var err error

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
		responseID, err := p.extractResponseID(response)
		if err == nil && responseID != "" {
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
		Step:               &StepInfo{Name: step.Name, Model: modelName, Action: combinedAction},
		IsParallel:         isParallel,
		ParallelID:         parallelID,
		PerformanceMetrics: metrics,
	})

	// Get outputs
	outputs := p.NormalizeStringSlice(step.Config.Output)
	p.debugf("Processing output for responses step '%s': model=%s outputs=%v",
		step.Name, modelName, outputs)

	// Print debug info to console
	fmt.Println("\n\n==== DEBUG OUTPUT ====")
	fmt.Println("About to handle output for responses step:", step.Name)
	fmt.Println("Output destinations:", outputs)
	fmt.Println("Response length:", len(response), "characters")

	// Extract the text from the response using our improved extraction function
	var responseData map[string]interface{}
	if err := json.Unmarshal([]byte(response), &responseData); err != nil {
		fmt.Printf("Error parsing response: %v\n", err)
		// If we can't parse the response, use it as-is
		if err := p.handleOutput(modelName, response, outputs, metrics); err != nil {
			errMsg := fmt.Sprintf("Output processing failed for responses step '%s': %v (model=%s outputs=%v)",
				step.Name, err, modelName, outputs)
			p.debugf("Output processing error: %s", errMsg)
			fmt.Printf("\nERROR: %s\n", errMsg)
			return "", fmt.Errorf("output handling error: %w", err)
		}
	} else {
		// Extract text using our improved function
		extractedText, err := p.extractOutputTextFromResponse(responseData)
		if err != nil {
			fmt.Printf("Error extracting text: %v\n", err)
			// If extraction fails, use the original response
			if err := p.handleOutput(modelName, response, outputs, metrics); err != nil {
				errMsg := fmt.Sprintf("Output processing failed for responses step '%s': %v (model=%s outputs=%v)",
					step.Name, err, modelName, outputs)
				p.debugf("Output processing error: %s", errMsg)
				fmt.Printf("\nERROR: %s\n", errMsg)
				return "", fmt.Errorf("output handling error: %w", err)
			}
		} else {
			// Use the extracted text
			fmt.Printf("Successfully extracted text: %d characters\n", len(extractedText))
			if err := p.handleOutput(modelName, extractedText, outputs, metrics); err != nil {
				errMsg := fmt.Sprintf("Output processing failed for responses step '%s': %v (model=%s outputs=%v)",
					step.Name, err, modelName, outputs)
				p.debugf("Output processing error: %s", errMsg)
				fmt.Printf("\nERROR: %s\n", errMsg)
				return "", fmt.Errorf("output handling error: %w", err)
			}
		}
	}
	fmt.Println("==== END DEBUG OUTPUT ====\n\n")
	p.debugf("Successfully processed output for responses step: %s", step.Name)
	fmt.Printf("\nDEBUG: Successfully processed output for responses step: %s\n", step.Name)

	return response, nil
}
