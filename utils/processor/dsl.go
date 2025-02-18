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
	progress   ProgressWriter    // Progress writer for streaming updates
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

// SetProgressWriter sets the progress writer for streaming updates
func (p *Processor) SetProgressWriter(w ProgressWriter) {
	p.progress = w
	p.spinner.SetProgressWriter(w)
}

// SetLastOutput sets the last output value, useful for initializing with STDIN data
func (p *Processor) SetLastOutput(output string) {
	p.lastOutput = output
}

// LastOutput returns the last output value
func (p *Processor) LastOutput() string {
	return p.lastOutput
}

// debugf prints debug information if verbose mode is enabled
func (p *Processor) debugf(format string, args ...interface{}) {
	if p.verbose {
		fmt.Printf("[DEBUG][DSL] "+format+"\n", args...)
	}
}

// emitProgress sends a progress update if a progress writer is configured
func (p *Processor) emitProgress(msg string, step *StepInfo) {
	if p.progress != nil {
		p.progress.WriteProgress(ProgressUpdate{
			Type:    ProgressStep,
			Message: msg,
			Step:    step,
		})
	}
}

// emitError sends an error update if a progress writer is configured
func (p *Processor) emitError(err error) {
	if p.progress != nil {
		p.progress.WriteProgress(ProgressUpdate{
			Type:  ProgressError,
			Error: err,
		})
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
	if len(p.config.Steps) == 0 {
		err := fmt.Errorf("no steps defined in DSL configuration")
		p.debugf("Validation error: %v", err)
		p.emitError(err)
		return fmt.Errorf("validation failed: %w", err)
	}

	p.debugf("Initial validation passed: found %d steps", len(p.config.Steps))

	// First validate all steps before processing
	p.spinner.Start("Validating DSL configuration")
	p.debugf("Starting step validation for %d steps", len(p.config.Steps))
	p.debugf("Step details:")
	for i, step := range p.config.Steps {
		p.debugf("Step %d: name=%s model=%v action=%v", i+1, step.Name, step.Config.Model, step.Config.Action)
	}

	// Validate model names before proceeding
	p.debugf("Beginning model validation phase")
	for _, step := range p.config.Steps {
		modelNames := p.NormalizeStringSlice(step.Config.Model)
		p.debugf("Normalized model names for step %s: %v", step.Name, modelNames)
		if err := p.validateModel(modelNames, []string{"STDIN"}); err != nil {
			p.debugf("Model validation failed for step %s: %v", step.Name, err)
			return fmt.Errorf("model validation failed for step %s: %w", step.Name, err)
		}
		p.debugf("Model validation passed for step %s", step.Name)
	}
	p.debugf("Model validation phase completed successfully")
	for i, step := range p.config.Steps {
		p.debugf("Validating step %d/%d: %s", i+1, len(p.config.Steps), step.Name)
		p.debugf("Step config: input=%v model=%v action=%v output=%v", step.Config.Input, step.Config.Model, step.Config.Action, step.Config.Output)

		if err := p.validateStepConfig(step.Name, step.Config); err != nil {
			p.spinner.Stop()
			errMsg := fmt.Sprintf("Validation failed for step '%s': %v", step.Name, err)
			p.debugf("Step validation error: %s", errMsg)
			p.emitError(fmt.Errorf(errMsg))
			return fmt.Errorf("validation error: %w", err)
		}
		p.debugf("Successfully validated step: %s", step.Name)
	}
	p.spinner.Stop()
	p.debugf("All steps validated successfully")

	// Process steps in order with detailed logging
	defer func() {
		if r := recover(); r != nil {
			p.debugf("Panic during step processing: %v", r)
			p.emitError(fmt.Errorf("internal error: %v", r))
		}
	}()

	for stepIndex, step := range p.config.Steps {
		stepInfo := &StepInfo{
			Name:   step.Name,
			Model:  fmt.Sprintf("%v", step.Config.Model),
			Action: fmt.Sprintf("%v", step.Config.Action),
		}
		stepMsg := fmt.Sprintf("Processing step %d/%d: %s", stepIndex+1, len(p.config.Steps), step.Name)
		p.emitProgress(stepMsg, stepInfo)
		p.spinner.Start(stepMsg)
		p.debugf("Starting step processing: index=%d name=%s", stepIndex, step.Name)
		p.debugf("Step details: input=%v model=%v action=%v output=%v", step.Config.Input, step.Config.Model, step.Config.Action, step.Config.Output)

		// Handle input based on type with error context
		var inputs []string
		p.debugf("Processing input configuration for step: %s", step.Name)
		switch v := step.Config.Input.(type) {
		case map[string]interface{}:
			// Check for database input
			if _, hasDB := v["database"]; hasDB {
				p.spinner.Stop()
				p.spinner.Start("Processing database input")
				p.debugf("Processing database input for step: %s", step.Name)
				if err := p.handleDatabaseInput(v); err != nil {
					p.spinner.Stop()
					errMsg := fmt.Sprintf("Database input processing failed for step '%s': %v", step.Name, err)
					p.debugf("Database error: %s", errMsg)
					p.emitError(fmt.Errorf(errMsg))
					return fmt.Errorf("database input error: %w", err)
				}
				p.debugf("Successfully processed database input")
				// Create a temporary file with the database output
				tmpFile, err := os.CreateTemp("", "comanda-db-*.txt")
				if err != nil {
					p.spinner.Stop()
					p.emitError(err)
					return fmt.Errorf("failed to create temp file for database output: %w", err)
				}
				tmpPath := tmpFile.Name()
				defer os.Remove(tmpPath)

				if _, err := tmpFile.WriteString(p.lastOutput); err != nil {
					tmpFile.Close()
					p.spinner.Stop()
					p.emitError(err)
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
					p.emitError(err)
					return fmt.Errorf("failed to process scraping input: %w", err)
				}
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
				// Initialize empty input if none provided
				if p.lastOutput == "" {
					p.lastOutput = ""
					p.debugf("No previous output available, using empty input")
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
					p.emitError(err)
					err = fmt.Errorf("failed to create temp file for STDIN: %w", err)
					fmt.Printf("Error in step '%s': %v\n", step.Name, err)
					return err
				}
				tmpPath := tmpFile.Name()
				defer os.Remove(tmpPath)

				if _, err := tmpFile.WriteString(p.lastOutput); err != nil {
					tmpFile.Close()
					p.spinner.Stop()
					p.emitError(err)
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
				p.emitError(err)
				err = fmt.Errorf("input processing error in step %s: %w", step.Name, err)
				fmt.Printf("Error: %v\n", err)
				return err
			}
			p.spinner.Stop()
		}

		// Skip model validation and provider configuration if model is NA
		if !(len(modelNames) == 1 && modelNames[0] == "NA") {
			// Validate model for this step with detailed logging
			p.spinner.Start("Validating model configuration")
			p.debugf("Validating models for step '%s': models=%v inputs=%v", step.Name, modelNames, inputs)
			if err := p.validateModel(modelNames, inputs); err != nil {
				p.spinner.Stop()
				errMsg := fmt.Sprintf("Model validation failed for step '%s': %v (models=%v)", step.Name, err, modelNames)
				p.debugf("Model validation error: %s", errMsg)
				p.emitError(fmt.Errorf(errMsg))
				return fmt.Errorf("model validation error: %w", err)
			}
			p.debugf("Model validation successful for step: %s", step.Name)
			p.spinner.Stop()

			// Configure providers with detailed logging
			p.spinner.Start("Configuring model providers")
			p.debugf("Configuring providers for step '%s'", step.Name)
			if err := p.configureProviders(); err != nil {
				p.spinner.Stop()
				errMsg := fmt.Sprintf("Provider configuration failed for step '%s': %v", step.Name, err)
				p.debugf("Provider configuration error: %s", errMsg)
				p.emitError(fmt.Errorf(errMsg))
				return fmt.Errorf("provider configuration error: %w", err)
			}
			p.debugf("Provider configuration successful for step: %s", step.Name)
			p.spinner.Stop()
		}

		// Process actions with detailed logging
		p.spinner.Start("Processing actions")
		p.debugf("Processing actions for step '%s'", step.Name)

		// Substitute variables in actions
		substitutedActions := make([]string, len(actions))
		for i, action := range actions {
			original := action
			substituted := p.substituteVariables(action)
			substitutedActions[i] = substituted
			if original != substituted {
				p.debugf("Variable substitution: original='%s' substituted='%s'", original, substituted)
			}
		}

		p.debugf("Executing actions: models=%v actions=%v", modelNames, substitutedActions)
		response, err := p.processActions(modelNames, substitutedActions)
		if err != nil {
			p.spinner.Stop()
			errMsg := fmt.Sprintf("Action processing failed for step '%s': %v (models=%v actions=%v)",
				step.Name, err, modelNames, substitutedActions)
			p.debugf("Action processing error: %s", errMsg)
			p.emitError(fmt.Errorf(errMsg))
			return fmt.Errorf("action processing error: %w", err)
		}
		p.debugf("Successfully processed actions for step: %s", step.Name)
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
				p.debugf("Processing database output for step '%s'", step.Name)
				if err := p.handleDatabaseOutput(response, v); err != nil {
					p.spinner.Stop()
					errMsg := fmt.Sprintf("Database output processing failed for step '%s': %v (config=%v)",
						step.Name, err, v)
					p.debugf("Database output error: %s", errMsg)
					p.emitError(fmt.Errorf(errMsg))
					return fmt.Errorf("database output error: %w", err)
				}
				p.debugf("Successfully processed database output for step: %s", step.Name)
				handled = true
			}
		}

		// Handle regular output if not already handled
		if !handled {
			outputs := p.NormalizeStringSlice(step.Config.Output)
			p.debugf("Processing regular output for step '%s': model=%s outputs=%v",
				step.Name, modelNames[0], outputs)
			if err := p.handleOutput(modelNames[0], response, outputs); err != nil {
				p.spinner.Stop()
				errMsg := fmt.Sprintf("Output processing failed for step '%s': %v (model=%s outputs=%v)",
					step.Name, err, modelNames[0], outputs)
				p.debugf("Output processing error: %s", errMsg)
				p.emitError(fmt.Errorf(errMsg))
				return fmt.Errorf("output handling error: %w", err)
			}
			p.debugf("Successfully processed output for step: %s", step.Name)
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
