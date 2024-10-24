package processor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/input"
	"github.com/kris-hansen/comanda/utils/models"
)

// DSLConfig represents the structure of the DSL configuration
type DSLConfig struct {
	Input      interface{} `yaml:"input"`       // Can be string or []string
	Model      interface{} `yaml:"model"`       // Can be string or []string
	Action     interface{} `yaml:"action"`      // Can be string or []string
	Output     interface{} `yaml:"output"`      // Can be string or []string
	NextAction interface{} `yaml:"next-action"` // Can be string or []string
}

// Processor handles the DSL processing pipeline
type Processor struct {
	config    *DSLConfig
	envConfig *config.EnvConfig
	handler   *input.Handler
	validator *input.Validator
	providers map[string]models.Provider
	verbose   bool
}

// NewProcessor creates a new DSL processor
func NewProcessor(config *DSLConfig, envConfig *config.EnvConfig, verbose bool) *Processor {
	return &Processor{
		config:    config,
		envConfig: envConfig,
		handler:   input.NewHandler(),
		validator: input.NewValidator(nil), // Use built-in text and image extensions
		providers: make(map[string]models.Provider),
		verbose:   verbose,
	}
}

// debugf prints debug information if verbose mode is enabled
func (p *Processor) debugf(format string, args ...interface{}) {
	if p.verbose {
		fmt.Printf("[DEBUG][DSL] "+format+"\n", args...)
	}
}

// NormalizeStringSlice converts interface{} to []string
func (p *Processor) NormalizeStringSlice(val interface{}) []string {
	p.debugf("Normalizing value type: %T", val)
	if val == nil {
		p.debugf("Received nil value, returning empty string slice")
		return []string{}
	}

	switch v := val.(type) {
	case []interface{}:
		result := make([]string, len(v))
		for i, item := range v {
			if str, ok := item.(string); ok {
				result[i] = str
			}
		}
		p.debugf("Converted []interface{} to []string: %v", result)
		return result
	case []string:
		p.debugf("Value already []string: %v", v)
		return v
	case string:
		p.debugf("Converting single string to []string: %v", v)
		return []string{v}
	default:
		p.debugf("Unsupported type, returning empty string slice")
		return []string{}
	}
}

// Process executes the DSL processing pipeline
func (p *Processor) Process() error {
	p.debugf("Starting DSL processing")
	p.debugf("Parsing DSL configuration sections")

	inputs := p.NormalizeStringSlice(p.config.Input)
	modelNames := p.NormalizeStringSlice(p.config.Model)
	actions := p.NormalizeStringSlice(p.config.Action)

	p.debugf("Processing with configuration:")
	p.debugf("- Inputs: %v", inputs)
	p.debugf("- Models: %v", modelNames)
	p.debugf("- Actions: %v", actions)

	// Skip input processing if it's "NA"
	if len(inputs) != 1 || inputs[0] != "NA" {
		p.debugf("Processing inputs...")
		if err := p.processInputs(inputs); err != nil {
			return fmt.Errorf("input processing error: %w", err)
		}
	} else {
		p.debugf("Skipping input processing (NA specified)")
	}

	// Detect and validate model
	p.debugf("Starting model validation phase")
	if err := p.validateModel(modelNames); err != nil {
		return fmt.Errorf("model validation error: %w", err)
	}

	// Configure providers
	p.debugf("Starting provider configuration phase")
	if err := p.configureProviders(); err != nil {
		return fmt.Errorf("provider configuration error: %w", err)
	}

	// Process actions
	p.debugf("Starting action processing phase")
	if err := p.processActions(modelNames, actions); err != nil {
		return fmt.Errorf("action processing error: %w", err)
	}

	p.debugf("DSL processing completed successfully")
	return nil
}

// isSpecialInput checks if the input is a special type (e.g., screenshot)
func (p *Processor) isSpecialInput(input string) bool {
	specialInputs := []string{"screenshot", "NA"}
	for _, special := range specialInputs {
		if input == special {
			return true
		}
	}
	return false
}

// processInputs handles the input section of the DSL
func (p *Processor) processInputs(inputs []string) error {
	p.debugf("Processing %d input(s)", len(inputs))
	for _, inputPath := range inputs {
		// Skip empty input
		if inputPath == "" {
			p.debugf("Skipping empty input")
			continue
		}

		p.debugf("Processing input path: %s", inputPath)

		// Handle special inputs first
		if p.isSpecialInput(inputPath) {
			if inputPath == "NA" {
				p.debugf("Skipping NA input")
				continue
			}
			p.debugf("Processing special input: %s", inputPath)
			if err := p.handler.ProcessPath(inputPath); err != nil {
				return fmt.Errorf("error processing special input %s: %w", inputPath, err)
			}
			continue
		}

		// Handle regular file inputs
		if err := p.processRegularInput(inputPath); err != nil {
			return err
		}
	}
	return nil
}

// processRegularInput handles regular file and directory inputs
func (p *Processor) processRegularInput(inputPath string) error {
	// Check if the path exists
	if _, err := os.Stat(inputPath); err != nil {
		if os.IsNotExist(err) {
			// Only try glob if the path contains glob characters
			if containsGlobChar(inputPath) {
				matches, err := filepath.Glob(inputPath)
				if err != nil {
					return fmt.Errorf("error processing glob pattern %s: %w", inputPath, err)
				}
				if len(matches) == 0 {
					return fmt.Errorf("no files found matching pattern: %s", inputPath)
				}
				for _, match := range matches {
					if err := p.processFile(match); err != nil {
						return err
					}
				}
				return nil
			}
			return fmt.Errorf("path does not exist: %s", inputPath)
		}
		return fmt.Errorf("error accessing path %s: %w", inputPath, err)
	}

	return p.processFile(inputPath)
}

// processFile handles a single file input
func (p *Processor) processFile(path string) error {
	p.debugf("Validating path: %s", path)
	if err := p.validator.ValidatePath(path); err != nil {
		return err
	}

	// Add file extension validation
	if err := p.validator.ValidateFileExtension(path); err != nil {
		return err
	}

	p.debugf("Processing file: %s", path)
	if err := p.handler.ProcessPath(path); err != nil {
		return err
	}
	p.debugf("Successfully processed file: %s", path)
	return nil
}

// containsGlobChar checks if a path contains glob characters
func containsGlobChar(path string) bool {
	return strings.ContainsAny(path, "*?[]")
}

// validateModel checks if the specified model is supported
func (p *Processor) validateModel(modelNames []string) error {
	p.debugf("Validating %d model(s)", len(modelNames))
	for _, modelName := range modelNames {
		p.debugf("Detecting provider for model: %s", modelName)
		provider := models.DetectProvider(modelName)
		if provider == nil {
			return fmt.Errorf("unsupported model: %s", modelName)
		}

		// Check if the provider actually supports this model
		if !provider.SupportsModel(modelName) {
			return fmt.Errorf("unsupported model: %s", modelName)
		}

		provider.SetVerbose(p.verbose)
		p.providers[modelName] = provider
		p.debugf("Model %s is supported by provider %s", modelName, provider.Name())
	}
	return nil
}

// configureProviders sets up all detected providers with API keys
func (p *Processor) configureProviders() error {
	configuredProviders := make(map[string]bool)
	p.debugf("Configuring providers for %d model(s)", len(p.providers))

	for modelName, provider := range p.providers {
		providerName := provider.Name()
		// Skip if provider already configured
		if configuredProviders[providerName] {
			p.debugf("Provider %s already configured, skipping", providerName)
			continue
		}

		p.debugf("Configuring provider %s for model %s", providerName, modelName)

		// Handle Ollama provider separately since it doesn't need an API key
		if providerName == "Ollama" {
			if err := provider.Configure(""); err != nil {
				return fmt.Errorf("failed to configure provider %s: %w", providerName, err)
			}
			p.debugf("Successfully configured local provider %s", providerName)
			configuredProviders[providerName] = true
			continue
		}

		var providerConfig *config.Provider
		var err error

		switch providerName {
		case "Anthropic":
			providerConfig, err = p.envConfig.GetProviderConfig("anthropic")
		case "OpenAI":
			providerConfig, err = p.envConfig.GetProviderConfig("openai")
		default:
			return fmt.Errorf("unknown provider for model: %s", modelName)
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
		configuredProviders[providerName] = true
	}
	return nil
}

// processActions handles the action section of the DSL
func (p *Processor) processActions(modelNames []string, actions []string) error {
	if len(modelNames) == 0 {
		return fmt.Errorf("no model specified for actions")
	}

	// For now, use the first model specified
	modelName := modelNames[0]
	provider := p.providers[modelName]
	if provider == nil {
		return fmt.Errorf("provider not found for model: %s", modelName)
	}

	p.debugf("Using model %s with provider %s", modelName, provider.Name())
	p.debugf("Processing %d action(s)", len(actions))

	// Get all input contents
	inputContents := string(p.handler.GetAllContents())

	for i, action := range actions {
		p.debugf("Processing action %d/%d: %s", i+1, len(actions), action)

		// For vision models, put the action before the input
		if modelName == "gpt-4o" || modelName == "gpt-4o-mini" {
			fullPrompt := fmt.Sprintf("%s\nInput:\n%s", action, inputContents)
			p.debugf("Prepared vision prompt with action first")
			response, err := provider.SendPrompt(modelName, fullPrompt)
			if err != nil {
				return fmt.Errorf("failed to process action with model %s: %w", modelName, err)
			}
			if err := p.handleOutput(modelName, response); err != nil {
				return err
			}
			continue
		}

		// For non-vision models, keep the original format
		fullPrompt := fmt.Sprintf("Input:\n%s\nAction: %s", inputContents, action)
		response, err := provider.SendPrompt(modelName, fullPrompt)
		if err != nil {
			return fmt.Errorf("failed to process action with model %s: %w", modelName, err)
		}
		if err := p.handleOutput(modelName, response); err != nil {
			return err
		}
	}

	return nil
}

// handleOutput processes the model's response according to the output configuration
func (p *Processor) handleOutput(modelName string, response string) error {
	outputs := p.NormalizeStringSlice(p.config.Output)
	p.debugf("Handling %d output(s)", len(outputs))
	for _, output := range outputs {
		p.debugf("Processing output: %s", output)
		if output == "STDOUT" {
			fmt.Printf("\nResponse from %s:\n%s\n", modelName, response)
			p.debugf("Response written to STDOUT")
		} else {
			// Write to file
			p.debugf("Writing response to file: %s", output)
			if err := os.WriteFile(output, []byte(response), 0644); err != nil {
				return fmt.Errorf("failed to write response to file %s: %w", output, err)
			}
			p.debugf("Response successfully written to file: %s", output)
		}
	}
	return nil
}

// GetProcessedInputs returns all processed input contents
func (p *Processor) GetProcessedInputs() []*input.Input {
	return p.handler.GetInputs()
}

// GetModelProvider returns the provider for the specified model
func (p *Processor) GetModelProvider(modelName string) models.Provider {
	return p.providers[modelName]
}
