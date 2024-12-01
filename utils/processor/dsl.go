package processor

import (
	"flag"
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
	providers  map[string]models.Provider
	verbose    bool
	lastOutput string
	spinner    *Spinner
	variables  map[string]string // Store variables from STDIN
}

// isTestMode checks if the code is running in test mode
func isTestMode() bool {
	return flag.Lookup("test.v") != nil
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
		spinner:   NewSpinner(),
		variables: make(map[string]string),
	}

	// Disable spinner in test environments
	if isTestMode() {
		p.spinner.Disable()
	}

	p.debugf("Creating new validator with default extensions")
	return p
}

// SetLastOutput sets the last output value, useful for initializing with STDIN data
func (p *Processor) SetLastOutput(output string) {
	p.lastOutput = output
}

// debugf prints debug information if verbose mode is enabled
func (p *Processor) debugf(format string, args ...interface{}) {
	if p.verbose {
		fmt.Printf("[DEBUG][DSL] "+format+"\n", args...)
	}
}

// parseVariableAssignment checks for "as $varname" syntax and returns the variable name
func (p *Processor) parseVariableAssignment(input string) (string, string) {
	parts := strings.Split(input, " as $")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return input, ""
}

// substituteVariables replaces variable references with their values
func (p *Processor) substituteVariables(text string) string {
	for name, value := range p.variables {
		text = strings.ReplaceAll(text, "$"+name, value)
	}
	return text
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
		errors = append(errors, "model is required (can be NA or a valid model name)")
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

	if len(p.config.Steps) == 0 {
		return fmt.Errorf("no steps defined in DSL configuration")
	}

	// First validate all steps before processing
	p.spinner.Start("Validating DSL configuration")
	for _, step := range p.config.Steps {
		if err := p.validateStepConfig(step.Name, step.Config); err != nil {
			p.spinner.Stop()
			fmt.Printf("Error: %v\n", err)
			return err
		}
	}
	p.spinner.Stop()

	// Process steps in order
	for stepIndex, step := range p.config.Steps {
		stepMsg := fmt.Sprintf("Processing step %d/%d: %s", stepIndex+1, len(p.config.Steps), step.Name)
		p.spinner.Start(stepMsg)
		p.debugf("Processing step: %s", step.Name)

		// Handle input based on type
		var inputs []string
		switch v := step.Config.Input.(type) {
		case map[string]interface{}:
			// Check for database input
			if _, hasDB := v["database"]; hasDB {
				p.spinner.Stop()
				p.spinner.Start("Processing database input")
				if err := p.handleDatabaseInput(v); err != nil {
					p.spinner.Stop()
					return fmt.Errorf("failed to process database input: %w", err)
				}
				// Create a temporary file with the database output
				tmpFile, err := os.CreateTemp("", "comanda-db-*.txt")
				if err != nil {
					p.spinner.Stop()
					return fmt.Errorf("failed to create temp file for database output: %w", err)
				}
				tmpPath := tmpFile.Name()
				defer os.Remove(tmpPath)

				if _, err := tmpFile.WriteString(p.lastOutput); err != nil {
					tmpFile.Close()
					p.spinner.Stop()
					return fmt.Errorf("failed to write database output to temp file: %w", err)
				}
				tmpFile.Close()

				// Set the input to the temp file path
				inputs = []string{tmpPath}
				p.spinner.Stop()
			} else if url, ok := v["url"].(string); ok {
				// Handle scraping configuration
				p.spinner.Stop()
				p.spinner.Start(fmt.Sprintf("Scraping content from %s", url))
				if err := p.handler.ProcessScrape(url, v); err != nil {
					p.spinner.Stop()
					return fmt.Errorf("failed to process scraping input: %w", err)
				}
				inputs = []string{url}
				p.spinner.Stop()
			} else {
				inputs = p.NormalizeStringSlice(step.Config.Input)
			}
		default:
			inputs = p.NormalizeStringSlice(step.Config.Input)
		}

		modelNames := p.NormalizeStringSlice(step.Config.Model)
		actions := p.NormalizeStringSlice(step.Config.Action)

		p.debugf("Step configuration:")
		p.debugf("- Inputs: %v", inputs)
		p.debugf("- Models: %v", modelNames)
		p.debugf("- Actions: %v", actions)

		// Handle STDIN specially
		if len(inputs) == 1 {
			input := inputs[0]
			if strings.HasPrefix(input, "STDIN") {
				if p.lastOutput == "" {
					p.spinner.Stop()
					err := fmt.Errorf("STDIN specified but no previous output available")
					fmt.Printf("Error in step '%s': %v\n", step.Name, err)
					return err
				}

				// Check for variable assignment
				_, varName := p.parseVariableAssignment(input)
				if varName != "" {
					p.variables[varName] = p.lastOutput
				}

				p.spinner.Start("Processing STDIN input")
				// Create a temporary file with .txt extension for the STDIN content
				tmpFile, err := os.CreateTemp("", "comanda-stdin-*.txt")
				if err != nil {
					p.spinner.Stop()
					err = fmt.Errorf("failed to create temp file for STDIN: %w", err)
					fmt.Printf("Error in step '%s': %v\n", step.Name, err)
					return err
				}
				tmpPath := tmpFile.Name()
				defer os.Remove(tmpPath)

				if _, err := tmpFile.WriteString(p.lastOutput); err != nil {
					tmpFile.Close()
					p.spinner.Stop()
					err = fmt.Errorf("failed to write to temp file: %w", err)
					fmt.Printf("Error in step '%s': %v\n", step.Name, err)
					return err
				}
				tmpFile.Close()
				p.spinner.Stop()

				// Update inputs to use the temporary file
				inputs = []string{tmpPath}
			}
		}

		// Process inputs for this step
		if len(inputs) > 0 {
			p.spinner.Start("Processing input files")
			p.debugf("Processing inputs for step %s...", step.Name)
			if err := p.processInputs(inputs); err != nil {
				p.spinner.Stop()
				err = fmt.Errorf("input processing error in step %s: %w", step.Name, err)
				fmt.Printf("Error: %v\n", err)
				return err
			}
			p.spinner.Stop()
		}

		// Skip model validation and provider configuration if model is NA
		if !(len(modelNames) == 1 && modelNames[0] == "NA") {
			// Validate model for this step
			p.spinner.Start("Validating model configuration")
			if err := p.validateModel(modelNames, inputs); err != nil {
				p.spinner.Stop()
				err = fmt.Errorf("model validation error in step %s: %w", step.Name, err)
				fmt.Printf("Error: %v\n", err)
				return err
			}
			p.spinner.Stop()

			// Configure providers if needed
			p.spinner.Start("Configuring model providers")
			if err := p.configureProviders(); err != nil {
				p.spinner.Stop()
				err = fmt.Errorf("provider configuration error in step %s: %w", step.Name, err)
				fmt.Printf("Error: %v\n", err)
				return err
			}
			p.spinner.Stop()
		}

		// Process actions for this step
		p.spinner.Start("Processing actions")
		// Substitute variables in actions
		substitutedActions := make([]string, len(actions))
		for i, action := range actions {
			substitutedActions[i] = p.substituteVariables(action)
		}
		response, err := p.processActions(modelNames, substitutedActions)
		if err != nil {
			p.spinner.Stop()
			err = fmt.Errorf("action processing error in step %s: %w", step.Name, err)
			fmt.Printf("Error: %v\n", err)
			return err
		}
		p.spinner.Stop()

		// Store the response for potential use as STDIN in next step
		p.lastOutput = response

		// Handle output for this step
		p.spinner.Start("Handling output")

		// Handle output based on type
		var handled bool
		switch v := step.Config.Output.(type) {
		case map[string]interface{}:
			if _, hasDB := v["database"]; hasDB {
				if err := p.handleDatabaseOutput(response, v); err != nil {
					p.spinner.Stop()
					err = fmt.Errorf("database output error in step %s: %w", step.Name, err)
					fmt.Printf("Error: %v\n", err)
					return err
				}
				handled = true
			}
		}

		// Handle regular output if not already handled
		if !handled {
			outputs := p.NormalizeStringSlice(step.Config.Output)
			if err := p.handleOutput(modelNames[0], response, outputs); err != nil {
				p.spinner.Stop()
				err = fmt.Errorf("output handling error in step %s: %w", step.Name, err)
				fmt.Printf("Error: %v\n", err)
				return err
			}
		}

		p.spinner.Stop()

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
