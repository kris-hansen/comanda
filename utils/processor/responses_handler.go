package processor

import (
	"fmt"
	"strings"

	"github.com/kris-hansen/comanda/utils/models"
)

// processResponsesStep handles the openai-responses step type
func (p *Processor) processResponsesStep(step Step, isParallel bool, parallelID string) (string, error) {
	p.debugf("Processing openai-responses step: %s", step.Name)

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

	// Send the request
	return responsesProvider.SendPromptWithResponses(config)
}
