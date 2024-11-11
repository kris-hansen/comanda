package processor

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/input"
	"github.com/kris-hansen/comanda/utils/models"
)

// StepConfig represents the configuration for a single step
type StepConfig struct {
	Input      interface{} `yaml:"input"`       // Can be string or []string
	Model      interface{} `yaml:"model"`       // Can be string or []string
	Action     interface{} `yaml:"action"`      // Can be string or []string
	Output     interface{} `yaml:"output"`      // Can be string or []string
	NextAction interface{} `yaml:"next-action"` // Can be string or []string
}

// DSLConfig represents the structure of the DSL configuration
type DSLConfig map[string]StepConfig

// Processor handles the DSL processing pipeline
type Processor struct {
	config     *DSLConfig
	envConfig  *config.EnvConfig
	handler    *input.Handler
	validator  *input.Validator
	providers  map[string]models.Provider // Key is provider name, not model name
	verbose    bool
	lastOutput string
}

// NewProcessor creates a new DSL processor
func NewProcessor(config *DSLConfig, envConfig *config.EnvConfig, verbose bool) *Processor {
	return &Processor{
		config:    config,
		envConfig: envConfig,
		handler:   input.NewHandler(),
		validator: input.NewValidator(nil),
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

// validateStepConfig checks if all required fields are present in a step
func (p *Processor) validateStepConfig(stepName string, config StepConfig) error {
	var errors []string

	// Check input field exists (can be empty or NA, but must be present)
	if config.Input == nil {
		errors = append(errors, "input tag is required (can be NA or empty, but the tag must be present)")
	}

	// Check model field
	modelNames := p.NormalizeStringSlice(config.Model)
	if len(modelNames) == 0 {
		errors = append(errors, "model is required and must specify a valid model name")
	}

	// Check action field
	actions := p.NormalizeStringSlice(config.Action)
	if len(actions) == 0 {
		errors = append(errors, "action is required")
	}

	// Check output field
	outputs := p.NormalizeStringSlice(config.Output)
	if len(outputs) == 0 {
		errors = append(errors, "output is required (can be STDOUT for console output)")
	}

	if len(errors) > 0 {
		return fmt.Errorf("validation errors in step '%s':\n- %s", stepName, strings.Join(errors, "\n- "))
	}

	return nil
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

	if len(*p.config) == 0 {
		return fmt.Errorf("no steps defined in DSL configuration")
	}

	// First validate all steps before processing
	for stepName, stepConfig := range *p.config {
		if err := p.validateStepConfig(stepName, stepConfig); err != nil {
			return err
		}
	}

	for stepName, stepConfig := range *p.config {
		p.debugf("Processing step: %s", stepName)

		inputs := p.NormalizeStringSlice(stepConfig.Input)
		modelNames := p.NormalizeStringSlice(stepConfig.Model)
		actions := p.NormalizeStringSlice(stepConfig.Action)

		p.debugf("Step configuration:")
		p.debugf("- Inputs: %v", inputs)
		p.debugf("- Models: %v", modelNames)
		p.debugf("- Actions: %v", actions)

		// Handle STDIN specially
		if len(inputs) == 1 && inputs[0] == "STDIN" {
			if p.lastOutput == "" {
				return fmt.Errorf("STDIN specified but no previous output available")
			}
			// Create a temporary file with .txt extension for the STDIN content
			tmpFile, err := os.CreateTemp("", "comanda-stdin-*.txt")
			if err != nil {
				return fmt.Errorf("failed to create temp file for STDIN: %w", err)
			}
			tmpPath := tmpFile.Name()
			defer os.Remove(tmpPath)

			if _, err := tmpFile.WriteString(p.lastOutput); err != nil {
				tmpFile.Close()
				return fmt.Errorf("failed to write to temp file: %w", err)
			}
			tmpFile.Close()

			// Update inputs to use the temporary file
			inputs = []string{tmpPath}
		}

		// Process inputs for this step
		if len(inputs) != 1 || inputs[0] != "NA" {
			p.debugf("Processing inputs for step %s...", stepName)
			if err := p.processInputs(inputs); err != nil {
				return fmt.Errorf("input processing error in step %s: %w", stepName, err)
			}
		}

		// Validate model for this step
		if err := p.validateModel(modelNames); err != nil {
			return fmt.Errorf("model validation error in step %s: %w", stepName, err)
		}

		// Configure providers if needed
		if err := p.configureProviders(); err != nil {
			return fmt.Errorf("provider configuration error in step %s: %w", stepName, err)
		}

		// Process actions for this step
		response, err := p.processActions(modelNames, actions)
		if err != nil {
			return fmt.Errorf("action processing error in step %s: %w", stepName, err)
		}

		// Store the response for potential use as STDIN in next step
		p.lastOutput = response

		// Handle output for this step
		outputs := p.NormalizeStringSlice(stepConfig.Output)
		if err := p.handleOutput(modelNames[0], response, outputs); err != nil {
			return fmt.Errorf("output handling error in step %s: %w", stepName, err)
		}

		// Clear the handler's contents for the next step
		p.handler = input.NewHandler()
	}

	p.debugf("DSL processing completed successfully")
	return nil
}

// isSpecialInput checks if the input is a special type (e.g., screenshot)
func (p *Processor) isSpecialInput(input string) bool {
	specialInputs := []string{"screenshot", "NA", "STDIN"}
	for _, special := range specialInputs {
		if input == special {
			return true
		}
	}
	return false
}

// isURL checks if the input string is a valid URL
func (p *Processor) isURL(input string) bool {
	u, err := url.Parse(input)
	if err != nil {
		return false
	}
	return u.Scheme != "" && u.Host != ""
}

// fetchURL retrieves content from a URL and saves it to a temporary file
func (p *Processor) fetchURL(urlStr string) (string, error) {
	p.debugf("Fetching content from URL: %s", urlStr)

	resp, err := http.Get(urlStr)
	if err != nil {
		return "", fmt.Errorf("failed to fetch URL %s: %w", urlStr, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch URL %s: status code %d", urlStr, resp.StatusCode)
	}

	// Create a temporary file with an appropriate extension based on Content-Type
	ext := ".txt"
	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "html") {
		ext = ".html"
	} else if strings.Contains(contentType, "json") {
		ext = ".json"
	}

	tmpFile, err := os.CreateTemp("", "comanda-url-*"+ext)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file for URL content: %w", err)
	}
	tmpPath := tmpFile.Name()

	_, err = io.Copy(tmpFile, resp.Body)
	tmpFile.Close()
	if err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to write URL content to file: %w", err)
	}

	p.debugf("URL content saved to temporary file: %s", tmpPath)
	return tmpPath, nil
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
			if inputPath == "STDIN" {
				p.debugf("Skipping STDIN input as it's handled in Process()")
				continue
			}
			p.debugf("Processing special input: %s", inputPath)
			if err := p.handler.ProcessPath(inputPath); err != nil {
				return fmt.Errorf("error processing special input %s: %w", inputPath, err)
			}
			continue
		}

		// Check if input is a URL
		if p.isURL(inputPath) {
			tmpPath, err := p.fetchURL(inputPath)
			if err != nil {
				return err
			}
			defer os.Remove(tmpPath)
			inputPath = tmpPath
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
	if len(modelNames) == 0 {
		return fmt.Errorf("no model specified")
	}

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

// processActions handles the action section of the DSL
func (p *Processor) processActions(modelNames []string, actions []string) (string, error) {
	if len(modelNames) == 0 {
		return "", fmt.Errorf("no model specified for actions")
	}

	// For now, use the first model specified
	modelName := modelNames[0]

	// Get provider by detecting it from the model name
	provider := models.DetectProvider(modelName)
	if provider == nil {
		return "", fmt.Errorf("provider not found for model: %s", modelName)
	}

	// Use the configured provider instance
	configuredProvider := p.providers[provider.Name()]
	if configuredProvider == nil {
		return "", fmt.Errorf("provider %s not configured", provider.Name())
	}

	p.debugf("Using model %s with provider %s", modelName, configuredProvider.Name())
	p.debugf("Processing %d action(s)", len(actions))

	// Get all input contents
	inputContents := string(p.handler.GetAllContents())
	var finalResponse string

	for i, action := range actions {
		p.debugf("Processing action %d/%d: %s", i+1, len(actions), action)

		// Use consistent format for all text inputs
		fullPrompt := fmt.Sprintf("Input:\n%s\nAction: %s", inputContents, action)
		p.debugf("Prepared prompt with input and action")

		response, err := configuredProvider.SendPrompt(modelName, fullPrompt)
		if err != nil {
			return "", fmt.Errorf("failed to process action with model %s: %w", modelName, err)
		}
		finalResponse = response
	}

	return finalResponse, nil
}

// handleOutput processes the model's response according to the output configuration
func (p *Processor) handleOutput(modelName string, response string, outputs []string) error {
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
	provider := models.DetectProvider(modelName)
	if provider == nil {
		return nil
	}
	return p.providers[provider.Name()]
}
