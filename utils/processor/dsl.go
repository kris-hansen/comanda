package processor

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/input"
	"github.com/kris-hansen/comanda/utils/models"
)

// Processor handles the DSL processing pipeline
type Processor struct {
	config       *DSLConfig
	envConfig    *config.EnvConfig
	serverConfig *config.ServerConfig // Add server config
	handler      *input.Handler
	validator    *input.Validator
	providers    map[string]models.Provider
	verbose      bool
	lastOutput   string
	spinner      *Spinner
	variables    map[string]string // Store variables from STDIN
	progress     ProgressWriter    // Progress writer for streaming updates
	runtimeDir   string            // Runtime directory for file operations
}

// isTestMode checks if the code is running in test mode
func isTestMode() bool {
	return flag.Lookup("test.v") != nil
}

// NewProcessor creates a new DSL processor
func NewProcessor(config *DSLConfig, envConfig *config.EnvConfig, serverConfig *config.ServerConfig, verbose bool, runtimeDir ...string) *Processor {
	// Default runtime directory to empty string if not provided
	rd := ""
	if len(runtimeDir) > 0 {
		rd = runtimeDir[0]
	}

	p := &Processor{
		config:       config,
		envConfig:    envConfig,
		serverConfig: serverConfig, // Store server config
		handler:      input.NewHandler(),
		validator:    input.NewValidator(nil),
		providers:    make(map[string]models.Provider),
		verbose:      verbose,
		spinner:      NewSpinner(),
		variables:    make(map[string]string),
		runtimeDir:   rd,
	}

	// Disable spinner in test environments
	if isTestMode() {
		p.spinner.Disable()
	}

	p.debugf("Creating new validator with default extensions")
	if rd != "" {
		p.debugf("Using runtime directory: %s", rd)
	}
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

// emitProgressWithMetrics sends a progress update with performance metrics
func (p *Processor) emitProgressWithMetrics(msg string, step *StepInfo, metrics *PerformanceMetrics) {
	if p.progress != nil {
		p.progress.WriteProgress(ProgressUpdate{
			Type:               ProgressStep,
			Message:            msg,
			Step:               step,
			PerformanceMetrics: metrics,
		})
	}
}

// emitParallelProgress sends a progress update for a parallel step
func (p *Processor) emitParallelProgress(msg string, step *StepInfo, parallelID string) {
	if p.progress != nil {
		p.progress.WriteProgress(ProgressUpdate{
			Type:       ProgressParallelStep,
			Message:    msg,
			Step:       step,
			IsParallel: true,
			ParallelID: parallelID,
		})
	}
}

// emitParallelProgressWithMetrics sends a progress update for a parallel step with performance metrics
func (p *Processor) emitParallelProgressWithMetrics(msg string, step *StepInfo, parallelID string, metrics *PerformanceMetrics) {
	if p.progress != nil {
		p.progress.WriteProgress(ProgressUpdate{
			Type:               ProgressParallelStep,
			Message:            msg,
			Step:               step,
			IsParallel:         true,
			ParallelID:         parallelID,
			PerformanceMetrics: metrics,
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

// validateDependencies checks for dependencies between steps and ensures parallel steps don't depend on each other
func (p *Processor) validateDependencies() error {
	// Build a map of output files produced by each step
	outputFiles := make(map[string]string) // file -> step name

	// Track dependencies between steps
	dependencies := make(map[string][]string) // step name -> dependencies

	// First, collect outputs from parallel steps
	for groupName, steps := range p.config.ParallelSteps {
		p.debugf("Checking dependencies for parallel group: %s", groupName)

		// Track outputs within this parallel group to check for dependencies
		parallelOutputs := make(map[string]string) // file -> step name

		for _, step := range steps {
			outputs := p.NormalizeStringSlice(step.Config.Output)
			for _, output := range outputs {
				if output != "STDOUT" {
					// Check if this output is already produced by another parallel step
					if producerStep, exists := parallelOutputs[output]; exists {
						return fmt.Errorf("parallel step '%s' and '%s' both produce the same output file '%s', which creates a conflict",
							step.Name, producerStep, output)
					}

					// Add to parallel outputs map
					parallelOutputs[output] = step.Name

					// Add to global outputs map
					outputFiles[output] = step.Name
				}
			}

			// Check if this parallel step depends on outputs from other parallel steps
			inputs := p.NormalizeStringSlice(step.Config.Input)
			for _, input := range inputs {
				if input != "NA" && input != "STDIN" {
					// Check if this input is an output from another parallel step
					if producerStep, exists := parallelOutputs[input]; exists {
						return fmt.Errorf("parallel step '%s' depends on output '%s' from parallel step '%s', which is not allowed",
							step.Name, input, producerStep)
					}
				}
			}
		}
	}

	// Now check regular steps
	for _, step := range p.config.Steps {
		// Check inputs for dependencies
		inputs := p.NormalizeStringSlice(step.Config.Input)
		var stepDependencies []string

		for _, input := range inputs {
			if input != "NA" && input != "STDIN" {
				// Check if this input is an output from another step
				if producerStep, exists := outputFiles[input]; exists {
					stepDependencies = append(stepDependencies, producerStep)
				}
			}
		}

		// Store dependencies for this step
		if len(stepDependencies) > 0 {
			dependencies[step.Name] = stepDependencies
		}

		// Add this step's outputs to the map
		outputs := p.NormalizeStringSlice(step.Config.Output)
		for _, output := range outputs {
			if output != "STDOUT" {
				outputFiles[output] = step.Name
			}
		}
	}

	// Check for circular dependencies
	for stepName, deps := range dependencies {
		visited := make(map[string]bool)
		if err := p.checkCircularDependencies(stepName, deps, dependencies, visited); err != nil {
			return err
		}
	}

	return nil
}

// checkCircularDependencies performs a depth-first search to detect circular dependencies
func (p *Processor) checkCircularDependencies(
	currentStep string,
	dependencies []string,
	allDependencies map[string][]string,
	visited map[string]bool,
) error {
	if visited[currentStep] {
		return fmt.Errorf("circular dependency detected involving step '%s'", currentStep)
	}

	visited[currentStep] = true

	for _, dep := range dependencies {
		if nextDeps, exists := allDependencies[dep]; exists {
			if err := p.checkCircularDependencies(dep, nextDeps, allDependencies, visited); err != nil {
				return err
			}
		}
	}

	// Remove from visited when backtracking
	visited[currentStep] = false

	return nil
}

// Process executes the DSL processing pipeline
func (p *Processor) Process() error {
	// Check if we have any steps to process
	if len(p.config.Steps) == 0 && len(p.config.ParallelSteps) == 0 {
		err := fmt.Errorf("no steps defined in DSL configuration")
		p.debugf("Validation error: %v", err)
		p.emitError(err)
		return fmt.Errorf("validation failed: %w", err)
	}

	p.debugf("Initial validation passed: found %d sequential steps and %d parallel step groups",
		len(p.config.Steps), len(p.config.ParallelSteps))

	// First validate all steps before processing
	p.spinner.Start("Validating DSL configuration")

	// Validate sequential steps
	p.debugf("Starting sequential step validation for %d steps", len(p.config.Steps))
	for i, step := range p.config.Steps {
		p.debugf("Step %d: name=%s model=%v action=%v", i+1, step.Name, step.Config.Model, step.Config.Action)

		// Validate step configuration
		if err := p.validateStepConfig(step.Name, step.Config); err != nil {
			p.spinner.Stop()
			errMsg := fmt.Sprintf("Validation failed for step '%s': %v", step.Name, err)
			p.debugf("Step validation error: %s", errMsg)
			p.emitError(fmt.Errorf(errMsg))
			return fmt.Errorf("validation error: %w", err)
		}

		// Validate model names
		modelNames := p.NormalizeStringSlice(step.Config.Model)
		p.debugf("Normalized model names for step %s: %v", step.Name, modelNames)
		if err := p.validateModel(modelNames, []string{"STDIN"}); err != nil {
			p.debugf("Model validation failed for step %s: %v", step.Name, err)
			return fmt.Errorf("model validation failed for step %s: %w", step.Name, err)
		}

		p.debugf("Successfully validated step: %s", step.Name)
	}

	// Validate parallel steps
	for groupName, steps := range p.config.ParallelSteps {
		p.debugf("Starting parallel step validation for group '%s' with %d steps", groupName, len(steps))

		for i, step := range steps {
			p.debugf("Parallel step %d: name=%s model=%v action=%v", i+1, step.Name, step.Config.Model, step.Config.Action)

			// Validate step configuration
			if err := p.validateStepConfig(step.Name, step.Config); err != nil {
				p.spinner.Stop()
				errMsg := fmt.Sprintf("Validation failed for parallel step '%s': %v", step.Name, err)
				p.debugf("Parallel step validation error: %s", errMsg)
				p.emitError(fmt.Errorf(errMsg))
				return fmt.Errorf("validation error: %w", err)
			}

			// Validate model names
			modelNames := p.NormalizeStringSlice(step.Config.Model)
			p.debugf("Normalized model names for parallel step %s: %v", step.Name, modelNames)
			if err := p.validateModel(modelNames, []string{"STDIN"}); err != nil {
				p.debugf("Model validation failed for parallel step %s: %v", step.Name, err)
				return fmt.Errorf("model validation failed for parallel step %s: %w", step.Name, err)
			}

			p.debugf("Successfully validated parallel step: %s", step.Name)
		}
	}

	// Validate dependencies between steps
	p.debugf("Validating dependencies between steps")
	if err := p.validateDependencies(); err != nil {
		p.spinner.Stop()
		errMsg := fmt.Sprintf("Dependency validation failed: %v", err)
		p.debugf("Dependency validation error: %s", errMsg)
		p.emitError(fmt.Errorf(errMsg))
		return fmt.Errorf("dependency validation error: %w", err)
	}

	p.spinner.Stop()
	p.debugf("All steps validated successfully")

	// Process steps with detailed logging and error handling
	defer func() {
		if r := recover(); r != nil {
			p.debugf("Panic during step processing: %v", r)
			p.emitError(fmt.Errorf("internal error: %v", r))
		}
	}()

	// Store results from parallel steps for use in sequential steps
	parallelResults := make(map[string]string)

	// Process parallel steps first if any
	for groupName, steps := range p.config.ParallelSteps {
		p.spinner.Start(fmt.Sprintf("Processing parallel step group: %s", groupName))
		p.debugf("Starting parallel processing for group '%s' with %d steps", groupName, len(steps))

		// Create channels for collecting results and errors
		type stepResult struct {
			name   string
			output string
		}

		resultChan := make(chan stepResult, len(steps))
		errorChan := make(chan error, len(steps))

		// Use a WaitGroup to wait for all goroutines to complete
		var wg sync.WaitGroup

		// Launch a goroutine for each parallel step
		for _, step := range steps {
			wg.Add(1)

			// Create a copy of the step for the goroutine to avoid race conditions
			stepCopy := step

			go func() {
				defer wg.Done()

				p.debugf("Starting goroutine for parallel step: %s", stepCopy.Name)

				// Process the step
				response, err := p.processStep(stepCopy, true, groupName)
				if err != nil {
					p.debugf("Error in parallel step '%s': %v", stepCopy.Name, err)
					errorChan <- fmt.Errorf("error in parallel step '%s': %w", stepCopy.Name, err)
					return
				}

				// Send the result to the result channel
				resultChan <- stepResult{
					name:   stepCopy.Name,
					output: response,
				}

				p.debugf("Completed parallel step: %s", stepCopy.Name)
			}()
		}

		// Wait for all goroutines to complete in a separate goroutine
		go func() {
			wg.Wait()
			close(resultChan)
			close(errorChan)
		}()

		// Check for errors
		for err := range errorChan {
			p.spinner.Stop()
			p.emitError(err)
			return err
		}

		// Collect results
		for result := range resultChan {
			p.debugf("Collected result from parallel step: %s", result.name)
			parallelResults[result.name] = result.output
		}

		p.spinner.Stop()
		p.debugf("Completed all parallel steps in group: %s", groupName)
	}

	// Process sequential steps
	for stepIndex, step := range p.config.Steps {
		stepInfo := &StepInfo{
			Name:   step.Name,
			Model:  fmt.Sprintf("%v", step.Config.Model),
			Action: fmt.Sprintf("%v", step.Config.Action),
		}

		stepMsg := fmt.Sprintf("Processing step %d/%d: %s", stepIndex+1, len(p.config.Steps), step.Name)
		p.emitProgress(stepMsg, stepInfo)
		p.spinner.Start(stepMsg)

		// Process the step
		response, err := p.processStep(step, false, "")
		if err != nil {
			p.spinner.Stop()
			errMsg := fmt.Sprintf("Error processing step '%s': %v", step.Name, err)
			p.debugf("Step processing error: %s", errMsg)
			p.emitError(fmt.Errorf(errMsg))
			return fmt.Errorf("step processing error: %w", err)
		}

		// Store the response for potential use as STDIN in next step
		p.lastOutput = response

		p.spinner.Stop()
		p.debugf("Successfully processed step: %s", step.Name)

		// Clear the handler's contents for the next step
		p.handler = input.NewHandler()
	}

	p.debugf("DSL processing completed successfully")
	return nil
}

// processStep handles the processing of a single step (used for both sequential and parallel processing)
func (p *Processor) processStep(step Step, isParallel bool, parallelID string) (string, error) {
	// Create performance metrics for this step
	metrics := &PerformanceMetrics{}
	startTime := time.Now()

	// Check if this is an openai-responses step
	if step.Config.Type == "openai-responses" {
		return p.processResponsesStep(step, isParallel, parallelID)
	}

	// Create a new handler for this step to avoid conflicts in parallel processing
	stepHandler := input.NewHandler()
	p.handler = stepHandler

	stepInfo := &StepInfo{
		Name:   step.Name,
		Model:  fmt.Sprintf("%v", step.Config.Model),
		Action: fmt.Sprintf("%v", step.Config.Action),
	}

	// Emit progress update based on whether this is a parallel step
	if isParallel {
		stepMsg := fmt.Sprintf("Processing parallel step: %s", step.Name)
		p.emitParallelProgress(stepMsg, stepInfo, parallelID)
		p.debugf("Starting parallel step processing: name=%s", step.Name)
	} else {
		stepMsg := fmt.Sprintf("Processing step: %s", step.Name)
		p.emitProgress(stepMsg, stepInfo)
		p.debugf("Starting step processing: name=%s", step.Name)
	}

	p.debugf("Step details: input=%v model=%v action=%v output=%v",
		step.Config.Input, step.Config.Model, step.Config.Action, step.Config.Output)

	// Handle input based on type with error context
	var inputs []string
	inputStartTime := time.Now()
	p.debugf("Processing input configuration for step: %s", step.Name)
	switch v := step.Config.Input.(type) {
	case map[string]interface{}:
		// Check for database input
		if _, hasDB := v["database"]; hasDB {
			p.debugf("Processing database input for step: %s", step.Name)
			if err := p.handleDatabaseInput(v); err != nil {
				errMsg := fmt.Sprintf("Database input processing failed for step '%s': %v", step.Name, err)
				p.debugf("Database error: %s", errMsg)
				return "", fmt.Errorf("database input error: %w", err)
			}
			p.debugf("Successfully processed database input")
			// Create a temporary file with the database output
			tmpFile, err := os.CreateTemp("", "comanda-db-*.txt")
			if err != nil {
				return "", fmt.Errorf("failed to create temp file for database output: %w", err)
			}
			tmpPath := tmpFile.Name()
			defer os.Remove(tmpPath)

			if _, err := tmpFile.WriteString(p.lastOutput); err != nil {
				tmpFile.Close()
				return "", fmt.Errorf("failed to write database output to temp file: %w", err)
			}
			tmpFile.Close()

			// Set the input to the temp file path
			inputs = []string{tmpPath}
		} else if url, ok := v["url"].(string); ok {
			// Handle scraping configuration
			p.debugf("Scraping content from %s for step: %s", url, step.Name)
			if err := p.handler.ProcessScrape(url, v); err != nil {
				return "", fmt.Errorf("failed to process scraping input: %w", err)
			}
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

			p.debugf("Processing STDIN input for step: %s", step.Name)
			// Create a temporary file with .txt extension for the STDIN content
			tmpFile, err := os.CreateTemp("", "comanda-stdin-*.txt")
			if err != nil {
				err = fmt.Errorf("failed to create temp file for STDIN: %w", err)
				fmt.Printf("Error in step '%s': %v\n", step.Name, err)
				return "", err
			}
			tmpPath := tmpFile.Name()
			defer os.Remove(tmpPath)

			if _, err := tmpFile.WriteString(p.lastOutput); err != nil {
				tmpFile.Close()
				err = fmt.Errorf("failed to write to temp file: %w", err)
				fmt.Printf("Error in step '%s': %v\n", step.Name, err)
				return "", err
			}
			tmpFile.Close()

			// Update inputs to use the temporary file
			inputs = []string{tmpPath}
		}
	}

	// Process inputs for this step
	if len(inputs) > 0 {
		p.debugf("Processing inputs for step %s...", step.Name)
		if err := p.processInputs(inputs); err != nil {
			err = fmt.Errorf("input processing error in step %s: %w", step.Name, err)
			fmt.Printf("Error: %v\n", err)
			return "", err
		}
	}

	// Record input processing time
	metrics.InputProcessingTime = time.Since(inputStartTime).Milliseconds()
	p.debugf("Input processing completed in %d ms", metrics.InputProcessingTime)

	// Start model processing time tracking
	modelStartTime := time.Now()

	// Skip model validation and provider configuration if model is NA
	if !(len(modelNames) == 1 && modelNames[0] == "NA") {
		// Validate model for this step with detailed logging
		p.debugf("Validating models for step '%s': models=%v inputs=%v", step.Name, modelNames, inputs)
		if err := p.validateModel(modelNames, inputs); err != nil {
			errMsg := fmt.Sprintf("Model validation failed for step '%s': %v (models=%v)", step.Name, err, modelNames)
			p.debugf("Model validation error: %s", errMsg)
			return "", fmt.Errorf("model validation error: %w", err)
		}
		p.debugf("Model validation successful for step: %s", step.Name)

		// Configure providers with detailed logging
		p.debugf("Configuring providers for step '%s'", step.Name)
		if err := p.configureProviders(); err != nil {
			errMsg := fmt.Sprintf("Provider configuration failed for step '%s': %v", step.Name, err)
			p.debugf("Provider configuration error: %s", errMsg)
			return "", fmt.Errorf("provider configuration error: %w", err)
		}
		p.debugf("Provider configuration successful for step: %s", step.Name)
	}

	// Process actions with detailed logging
	p.debugf("Processing actions for step '%s'", step.Name)

	// Record model processing time
	metrics.ModelProcessingTime = time.Since(modelStartTime).Milliseconds()
	p.debugf("Model processing completed in %d ms", metrics.ModelProcessingTime)

	// Start action processing time tracking
	actionStartTime := time.Now()

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
		errMsg := fmt.Sprintf("Action processing failed for step '%s': %v (models=%v actions=%v)",
			step.Name, err, modelNames, substitutedActions)
		p.debugf("Action processing error: %s", errMsg)
		return "", fmt.Errorf("action processing error: %w", err)
	}
	p.debugf("Successfully processed actions for step: %s", step.Name)

	// Record action processing time
	metrics.ActionProcessingTime = time.Since(actionStartTime).Milliseconds()
	p.debugf("Action processing completed in %d ms", metrics.ActionProcessingTime)

	// Start output processing time tracking
	outputStartTime := time.Now()

	// Handle output for this step
	p.debugf("Handling output for step: %s", step.Name)

	// Handle output based on type
	var handled bool
	switch v := step.Config.Output.(type) {
	case map[string]interface{}:
		if _, hasDB := v["database"]; hasDB {
			p.debugf("Processing database output for step '%s'", step.Name)
			if err := p.handleDatabaseOutput(response, v); err != nil {
				errMsg := fmt.Sprintf("Database output processing failed for step '%s': %v (config=%v)",
					step.Name, err, v)
				p.debugf("Database output error: %s", errMsg)
				return "", fmt.Errorf("database output error: %w", err)
			}

			// For database outputs, we still want to show performance metrics in STDOUT
			if p.verbose {
				fmt.Printf("\nPerformance Metrics for step '%s':\n"+
					"- Input processing: %d ms\n"+
					"- Model processing: %d ms\n"+
					"- Action processing: %d ms\n"+
					"- Database output: (in progress)\n"+
					"- Total processing: (in progress)\n",
					step.Name,
					metrics.InputProcessingTime,
					metrics.ModelProcessingTime,
					metrics.ActionProcessingTime)
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
		if err := p.handleOutput(modelNames[0], response, outputs, metrics); err != nil {
			errMsg := fmt.Sprintf("Output processing failed for step '%s': %v (model=%s outputs=%v)",
				step.Name, err, modelNames[0], outputs)
			p.debugf("Output processing error: %s", errMsg)
			return "", fmt.Errorf("output handling error: %w", err)
		}
		p.debugf("Successfully processed output for step: %s", step.Name)
	}

	// Record output processing time
	metrics.OutputProcessingTime = time.Since(outputStartTime).Milliseconds()
	p.debugf("Output processing completed in %d ms", metrics.OutputProcessingTime)

	// Calculate total processing time
	metrics.TotalProcessingTime = time.Since(startTime).Milliseconds()

	// Log performance metrics
	p.debugf("Step '%s' performance metrics:", step.Name)
	p.debugf("- Input processing: %d ms", metrics.InputProcessingTime)
	p.debugf("- Model processing: %d ms", metrics.ModelProcessingTime)
	p.debugf("- Action processing: %d ms", metrics.ActionProcessingTime)
	p.debugf("- Output processing: %d ms", metrics.OutputProcessingTime)
	p.debugf("- Total processing: %d ms", metrics.TotalProcessingTime)

	// Emit progress update with performance metrics
	if isParallel {
		p.emitParallelProgressWithMetrics(
			fmt.Sprintf("Completed parallel step: %s (in %d ms)", step.Name, metrics.TotalProcessingTime),
			stepInfo,
			parallelID,
			metrics)
	} else {
		p.emitProgressWithMetrics(
			fmt.Sprintf("Completed step: %s (in %d ms)", step.Name, metrics.TotalProcessingTime),
			stepInfo,
			metrics)
	}

	return response, nil
}

// getCurrentStepConfig returns the configuration for the current step being processed
func (p *Processor) getCurrentStepConfig() StepConfig {
	// If we're not processing a step yet, return an empty config with default values
	if p.config == nil || (len(p.config.Steps) == 0 && len(p.config.ParallelSteps) == 0) {
		return StepConfig{
			BatchMode: "individual", // Default to individual mode for safety
		}
	}

	// For now, just return the first step's config
	// In a more complete implementation, this would track the current step being processed
	if len(p.config.Steps) > 0 {
		return p.config.Steps[0].Config
	}

	// If we only have parallel steps, return the first one
	for _, steps := range p.config.ParallelSteps {
		if len(steps) > 0 {
			return steps[0].Config
		}
	}

	// Fallback to default config
	return StepConfig{
		BatchMode: "individual", // Default to individual mode for safety
	}
}

// GetProcessedInputs returns all processed input contents
func (p *Processor) GetProcessedInputs() []*input.Input {
	return p.handler.GetInputs()
}
