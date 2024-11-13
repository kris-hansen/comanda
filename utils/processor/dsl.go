package processor

import (
	"fmt"
	"os"
	"strings"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/input"
	"github.com/kris-hansen/comanda/utils/models"
)

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
	p := &Processor{
		config:    config,
		envConfig: envConfig,
		handler:   input.NewHandler(),
		validator: input.NewValidator(nil),
		providers: make(map[string]models.Provider),
		verbose:   verbose,
	}
	p.debugf("Creating new validator with default extensions")
	return p
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

// Process executes the DSL processing pipeline
func (p *Processor) Process() error {
	p.debugf("Starting DSL processing")

	if len(*p.config) == 0 {
		return fmt.Errorf("no steps defined in DSL configuration")
	}

	// First validate all steps before processing
	for stepName, stepConfig := range *p.config {
		if err := p.validateStepConfig(stepName, stepConfig); err != nil {
			fmt.Printf("Error: %v\n", err)
			return err
		}
	}

	for stepName, stepConfig := range *p.config {
		p.debugf("Processing step: %s", stepName)

		// Handle input based on type
		var inputs []string
		switch v := stepConfig.Input.(type) {
		case map[string]interface{}:
			// Handle scraping configuration
			if url, ok := v["url"].(string); ok {
				if err := p.handler.ProcessScrape(url, v); err != nil {
					return fmt.Errorf("failed to process scraping input: %w", err)
				}
				inputs = []string{url}
			} else {
				inputs = p.NormalizeStringSlice(stepConfig.Input)
			}
		default:
			inputs = p.NormalizeStringSlice(stepConfig.Input)
		}

		modelNames := p.NormalizeStringSlice(stepConfig.Model)
		actions := p.NormalizeStringSlice(stepConfig.Action)

		p.debugf("Step configuration:")
		p.debugf("- Inputs: %v", inputs)
		p.debugf("- Models: %v", modelNames)
		p.debugf("- Actions: %v", actions)

		// Handle STDIN specially
		if len(inputs) == 1 && inputs[0] == "STDIN" {
			if p.lastOutput == "" {
				err := fmt.Errorf("STDIN specified but no previous output available")
				fmt.Printf("Error in step '%s': %v\n", stepName, err)
				return err
			}
			// Create a temporary file with .txt extension for the STDIN content
			tmpFile, err := os.CreateTemp("", "comanda-stdin-*.txt")
			if err != nil {
				err = fmt.Errorf("failed to create temp file for STDIN: %w", err)
				fmt.Printf("Error in step '%s': %v\n", stepName, err)
				return err
			}
			tmpPath := tmpFile.Name()
			defer os.Remove(tmpPath)

			if _, err := tmpFile.WriteString(p.lastOutput); err != nil {
				tmpFile.Close()
				err = fmt.Errorf("failed to write to temp file: %w", err)
				fmt.Printf("Error in step '%s': %v\n", stepName, err)
				return err
			}
			tmpFile.Close()

			// Update inputs to use the temporary file
			inputs = []string{tmpPath}
		}

		// Process inputs for this step
		if len(inputs) != 1 || inputs[0] != "NA" {
			p.debugf("Processing inputs for step %s...", stepName)
			if err := p.processInputs(inputs); err != nil {
				err = fmt.Errorf("input processing error in step %s: %w", stepName, err)
				fmt.Printf("Error: %v\n", err)
				return err
			}
		}

		// Validate model for this step
		if err := p.validateModel(modelNames, inputs); err != nil {
			err = fmt.Errorf("model validation error in step %s: %w", stepName, err)
			fmt.Printf("Error: %v\n", err)
			return err
		}

		// Configure providers if needed
		if err := p.configureProviders(); err != nil {
			err = fmt.Errorf("provider configuration error in step %s: %w", stepName, err)
			fmt.Printf("Error: %v\n", err)
			return err
		}

		// Process actions for this step
		response, err := p.processActions(modelNames, actions)
		if err != nil {
			err = fmt.Errorf("action processing error in step %s: %w", stepName, err)
			fmt.Printf("Error: %v\n", err)
			return err
		}

		// Store the response for potential use as STDIN in next step
		p.lastOutput = response

		// Handle output for this step
		outputs := p.NormalizeStringSlice(stepConfig.Output)
		if err := p.handleOutput(modelNames[0], response, outputs); err != nil {
			err = fmt.Errorf("output handling error in step %s: %w", stepName, err)
			fmt.Printf("Error: %v\n", err)
			return err
		}

		// Clear the handler's contents for the next step
		p.handler = input.NewHandler()
	}

	p.debugf("DSL processing completed successfully")
	return nil
}

// GetProcessedInputs returns all processed input contents
func (p *Processor) GetProcessedInputs() []*input.Input {
	return p.handler.GetInputs()
}
