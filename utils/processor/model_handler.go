package processor

import (
	"fmt"
	"strings"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/models"
)

// validateModel checks if the specified model is supported and has the required capabilities
func (p *Processor) validateModel(modelNames []string, inputs []string) error {
	if len(modelNames) == 0 {
		return fmt.Errorf("no model specified")
	}

	// Special case: if the only model is "NA", skip validation
	if len(modelNames) == 1 && modelNames[0] == "NA" {
		p.debugf("Model is NA, skipping provider validation")
		return nil
	}

	p.debugf("Validating %d model(s)", len(modelNames))
	for _, modelName := range modelNames {
		p.debugf("Starting validation for model: %s", modelName)
		p.debugf("Attempting provider detection for model: %s", modelName)
		provider := models.DetectProvider(modelName)
		p.debugf("Provider detection result for %s: found=%v", modelName, provider != nil)
		if provider == nil {
			errMsg := fmt.Sprintf("unsupported model: %s (no provider found)", modelName)
			p.debugf("Validation failed: %s", errMsg)
			return fmt.Errorf(errMsg)
		}

		// Check if the provider actually supports this model
		p.debugf("Checking if provider %s supports model %s", provider.Name(), modelName)
		if !provider.SupportsModel(modelName) {
			errMsg := fmt.Sprintf("unsupported model: %s (provider %s does not support it)", modelName, provider.Name())
			p.debugf("Validation failed: %s", errMsg)
			return fmt.Errorf(errMsg)
		}
		p.debugf("Provider %s confirmed support for model %s", provider.Name(), modelName)

		// Get provider name
		providerName := provider.Name()

		// Get model configuration from environment
		p.debugf("Getting model configuration for %s from provider %s", modelName, providerName)
		modelConfig, err := p.envConfig.GetModelConfig(providerName, modelName)
		if err != nil {
			// Check if the error is specifically "model not found" after provider support was confirmed
			if strings.Contains(err.Error(), fmt.Sprintf("model %s not found for provider %s", modelName, providerName)) {
				errMsg := fmt.Sprintf("model %s is supported by provider %s but is not enabled in your configuration. Use 'comanda configure' to add it.", modelName, providerName)
				p.debugf("Configuration error: %s", errMsg)
				return fmt.Errorf(errMsg)
			}
			// Otherwise, return the original configuration error
			errMsg := fmt.Sprintf("failed to get model configuration for %s: %v", modelName, err)
			p.debugf("Configuration error: %s", errMsg)
			return fmt.Errorf(errMsg)
		}
		p.debugf("Successfully retrieved model configuration for %s", modelName)

		// Check if model has required capabilities based on input types
		for _, input := range inputs {
			if input == "NA" || input == "STDIN" {
				continue
			}

			// Check for file mode support if input is a document file
			if p.validator.IsDocumentFile(input) && !modelConfig.HasMode(config.FileMode) {
				return fmt.Errorf("model %s does not support file processing", modelName)
			}

			// Check for vision mode support if input is an image file
			if p.validator.IsImageFile(input) && !modelConfig.HasMode(config.VisionMode) {
				return fmt.Errorf("model %s does not support image processing", modelName)
			}

			// For text files, ensure model supports text mode
			if !p.validator.IsDocumentFile(input) && !p.validator.IsImageFile(input) && !modelConfig.HasMode(config.TextMode) {
				return fmt.Errorf("model %s does not support text processing", modelName)
			}
		}

		provider.SetVerbose(p.verbose)
		// Store provider by provider name instead of model name
		p.providers[provider.Name()] = provider
		p.debugf("Model %s is supported by provider %s", modelName, provider.Name())
	}
	return nil
}

// configureProviders sets up all detected providers with API keys
func (p *Processor) configureProviders() error {
	p.debugf("Configuring providers")

	for providerName, provider := range p.providers {
		p.debugf("Configuring provider %s", providerName)

		// Handle Ollama provider separately since it doesn't need an API key
		if providerName == "ollama" {
			if err := provider.Configure(""); err != nil {
				return fmt.Errorf("failed to configure provider %s: %w", providerName, err)
			}
			p.debugf("Successfully configured local provider %s", providerName)
			continue
		}

		var providerConfig *config.Provider
		var err error

		switch providerName {
		case "anthropic":
			providerConfig, err = p.envConfig.GetProviderConfig("anthropic")
		case "openai":
			providerConfig, err = p.envConfig.GetProviderConfig("openai")
		case "google":
			providerConfig, err = p.envConfig.GetProviderConfig("google")
		case "xai":
			providerConfig, err = p.envConfig.GetProviderConfig("xai")
		case "deepseek":
			providerConfig, err = p.envConfig.GetProviderConfig("deepseek")
		default:
			return fmt.Errorf("unknown provider: %s", providerName)
		}

		if err != nil {
			return fmt.Errorf("failed to get config for provider %s: %w", providerName, err)
		}

		if providerConfig.APIKey == "" {
			return fmt.Errorf("missing API key for provider %s", providerName)
		}

		p.debugf("Found API key for provider %s", providerName)

		if err := provider.Configure(providerConfig.APIKey); err != nil {
			return fmt.Errorf("failed to configure provider %s: %w", providerName, err)
		}

		p.debugf("Successfully configured provider %s", providerName)
	}
	return nil
}

// GetModelProvider returns the provider for the specified model
func (p *Processor) GetModelProvider(modelName string) models.Provider {
	// Special case: if model is "NA", return nil since no provider is needed
	if modelName == "NA" {
		return nil
	}

	provider := models.DetectProvider(modelName)
	if provider == nil {
		return nil
	}
	return p.providers[provider.Name()]
}
