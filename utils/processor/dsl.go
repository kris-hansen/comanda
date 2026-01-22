package processor

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/kris-hansen/comanda/utils/chunker"
	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/input"
	"github.com/kris-hansen/comanda/utils/models"
	"gopkg.in/yaml.v3"
)

// GenerateStepConfig defines the configuration for a generate step
type GenerateStepConfig struct {
	Model        interface{} `yaml:"model"`
	Action       interface{} `yaml:"action"`
	Output       string      `yaml:"output"`
	ContextFiles []string    `yaml:"context_files"`
}

// ProcessStepConfig defines the configuration for a process step
type ProcessStepConfig struct {
	WorkflowFile   string                 `yaml:"workflow_file"`
	Inputs         map[string]interface{} `yaml:"inputs"`
	CaptureOutputs []string               `yaml:"capture_outputs"`
}

// Processor handles the DSL processing pipeline
type Processor struct {
	config               *DSLConfig
	envConfig            *config.EnvConfig
	serverConfig         *config.ServerConfig // Add server config
	handler              *input.Handler
	validator            *input.Validator
	providers            map[string]models.Provider
	verbose              bool
	lastOutput           string
	spinner              *Spinner
	variables            map[string]string  // Store variables from STDIN
	cliVariables         map[string]string  // CLI-provided variables for {{var}} substitution
	progress             ProgressWriter     // Progress writer for streaming updates
	runtimeDir           string             // Runtime directory for file operations
	memory               *MemoryManager     // Memory manager for COMANDA.md file
	externalMemory       string             // External memory context (e.g., from OpenAI messages)
	mu                   sync.Mutex         // Mutex for thread-safe debug logging
	currentAgenticConfig *AgenticLoopConfig // Current agentic loop config (set during agentic loop execution)
}

// setAgenticConfig sets the current agentic config (thread-safe)
func (p *Processor) setAgenticConfig(config *AgenticLoopConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.currentAgenticConfig = config
}

// getAgenticConfig returns the current agentic config (thread-safe)
func (p *Processor) getAgenticConfig() *AgenticLoopConfig {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.currentAgenticConfig
}

// UnmarshalYAML is a custom unmarshaler for DSLConfig to handle mixed types at the root level
func (c *DSLConfig) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("expected a mapping node but got %v", node.Kind)
	}

	c.Steps = []Step{}
	c.ParallelSteps = make(map[string][]Step)
	c.Defer = make(map[string]StepConfig)
	c.AgenticLoops = make(map[string]*AgenticLoopConfig)

	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		stepName := keyNode.Value

		switch stepName {
		case "parallel":
			var parallelSteps map[string][]Step
			if err := valueNode.Decode(&parallelSteps); err != nil {
				return fmt.Errorf("failed to decode parallel steps: %w", err)
			}
			c.ParallelSteps = parallelSteps
		case "defer":
			var deferredSteps map[string]StepConfig
			if err := valueNode.Decode(&deferredSteps); err != nil {
				return fmt.Errorf("failed to decode deferred steps: %w", err)
			}

			// Check for duplicate step names before assigning
			for stepName := range deferredSteps {
				if _, exists := c.Defer[stepName]; exists {
					return fmt.Errorf("duplicate step name '%s' found in defer block: step names must be unique", stepName)
				}
			}

			// Assign deferred steps to the config
			c.Defer = deferredSteps
		case "agentic-loop":
			// Parse agentic loop block with config and steps
			if err := c.parseAgenticLoopBlock(valueNode); err != nil {
				return fmt.Errorf("failed to decode agentic loop: %w", err)
			}
		default:
			// Try to decode as a standard step config first
			var stepConfig StepConfig
			stepErr := valueNode.Decode(&stepConfig)

			// If it fails or if the valueNode is a mapping that contains nested steps,
			// check if this is a parallel step group
			if stepErr != nil || c.isParallelStepGroup(valueNode) {
				var parallelSteps map[string]StepConfig
				if err := valueNode.Decode(&parallelSteps); err != nil {
					// If we can't decode as parallel steps either, return the original step error
					if stepErr != nil {
						return fmt.Errorf("failed to decode step '%s': %w", stepName, stepErr)
					}
					return fmt.Errorf("failed to decode parallel step group '%s': %w", stepName, err)
				}

				// Convert map[string]StepConfig to []Step
				var steps []Step
				for subStepName, subStepConfig := range parallelSteps {
					steps = append(steps, Step{Name: subStepName, Config: subStepConfig})
				}
				c.ParallelSteps[stepName] = steps
			} else {
				// It's a regular step
				c.Steps = append(c.Steps, Step{Name: stepName, Config: stepConfig})
			}
		}
	}

	return nil
}

// isParallelStepGroup checks if a YAML node represents a parallel step group
// by examining if it contains nested mappings that look like step configurations
func (c *DSLConfig) isParallelStepGroup(node *yaml.Node) bool {
	if node.Kind != yaml.MappingNode {
		return false
	}

	// Check if all the values in this mapping are themselves mappings
	// which would indicate nested step configurations
	for i := 1; i < len(node.Content); i += 2 {
		valueNode := node.Content[i]
		if valueNode.Kind != yaml.MappingNode {
			return false
		}

		// Check if this nested mapping has step-like keys
		hasStepKeys := false
		for j := 0; j < len(valueNode.Content); j += 2 {
			keyNode := valueNode.Content[j]
			key := keyNode.Value
			if key == "input" || key == "model" || key == "action" || key == "output" ||
				key == "generate" || key == "process" || key == "type" || key == "agentic_loop" {
				hasStepKeys = true
				break
			}
		}
		if !hasStepKeys {
			return false
		}
	}

	return true
}

// parseAgenticLoopBlock parses an agentic-loop block from YAML
// The block can have a "config" section with loop settings and a "steps" section with sub-steps
func (c *DSLConfig) parseAgenticLoopBlock(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("agentic-loop must be a mapping")
	}

	// Parse the agentic loop structure
	loopConfig := &AgenticLoopConfig{}
	var stepsNode *yaml.Node

	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]
		key := keyNode.Value

		switch key {
		case "config":
			// Decode the config section
			if err := valueNode.Decode(loopConfig); err != nil {
				return fmt.Errorf("failed to decode agentic loop config: %w", err)
			}
		case "steps":
			stepsNode = valueNode
		default:
			return fmt.Errorf("unknown key '%s' in agentic-loop block, expected 'config' or 'steps'", key)
		}
	}

	// Parse steps if present
	if stepsNode != nil {
		if stepsNode.Kind != yaml.MappingNode {
			return fmt.Errorf("agentic-loop steps must be a mapping")
		}

		for i := 0; i < len(stepsNode.Content); i += 2 {
			keyNode := stepsNode.Content[i]
			valueNode := stepsNode.Content[i+1]
			stepName := keyNode.Value

			var stepConfig StepConfig
			if err := valueNode.Decode(&stepConfig); err != nil {
				return fmt.Errorf("failed to decode agentic loop step '%s': %w", stepName, err)
			}

			loopConfig.Steps = append(loopConfig.Steps, Step{Name: stepName, Config: stepConfig})
		}
	}

	// Store the loop config with a default name
	c.AgenticLoops["agentic-loop"] = loopConfig
	return nil
}

// isTestMode checks if the code is running in test mode
func isTestMode() bool {
	return flag.Lookup("test.v") != nil
}

// NewProcessor creates a new DSL processor
func NewProcessor(dslConfig *DSLConfig, envConfig *config.EnvConfig, serverConfig *config.ServerConfig, verbose bool, runtimeDir string, cliVariables ...map[string]string) *Processor {
	// Get CLI variables if provided
	cliVars := make(map[string]string)
	if len(cliVariables) > 0 && cliVariables[0] != nil {
		cliVars = cliVariables[0]
	}

	p := &Processor{
		config:       dslConfig,
		envConfig:    envConfig,
		serverConfig: serverConfig, // Store server config
		handler:      input.NewHandler(),
		validator:    input.NewValidator(nil),
		providers:    make(map[string]models.Provider),
		verbose:      verbose,
		spinner:      NewSpinner(),
		variables:    make(map[string]string),
		cliVariables: cliVars,
		runtimeDir:   runtimeDir, // Store runtime directory
	}

	// Store runtime directory as-is (relative or empty)
	if runtimeDir != "" {
		p.debugf("Processor initialized with runtime directory: %s", runtimeDir)
	} else {
		p.debugf("Processor initialized without a specific runtime directory.")
	}

	// Log server configuration
	if p.serverConfig != nil {
		p.debugf("Server configuration:")
		p.debugf("- Enabled: %v", p.serverConfig.Enabled)
		p.debugf("- DataDir: %s", p.serverConfig.DataDir)
	} else {
		p.debugf("No server configuration provided")
	}

	// Initialize memory manager
	memoryPath := config.GetMemoryPath(envConfig)
	if memoryPath != "" {
		memoryMgr, err := NewMemoryManager(memoryPath)
		if err != nil {
			// Provide detailed diagnostic information about the failure
			p.debugf("Warning: Failed to initialize memory manager")
			p.debugf("  Memory file path: %s", memoryPath)
			p.debugf("  Error: %v", err)

			// Check if file exists and provide additional context
			if fileInfo, statErr := os.Stat(memoryPath); statErr != nil {
				if os.IsNotExist(statErr) {
					p.debugf("  Reason: Memory file does not exist")
				} else if os.IsPermission(statErr) {
					p.debugf("  Reason: Permission denied accessing memory file")
				} else {
					p.debugf("  Additional error: %v", statErr)
				}
			} else {
				p.debugf("  File exists: true, Size: %d bytes", fileInfo.Size())
				if !fileInfo.Mode().IsRegular() {
					p.debugf("  Reason: Path is not a regular file")
				}
			}
			p.debugf("  Memory features will be disabled for this session")
		} else {
			p.memory = memoryMgr
			p.debugf("Memory manager initialized with file: %s", memoryPath)
		}
	} else {
		p.debugf("No memory file configured")
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

// SetMemoryContext sets external memory context (e.g., from OpenAI chat messages)
// This context is used alongside or instead of file-based memory
func (p *Processor) SetMemoryContext(context string) {
	p.externalMemory = context
}

// GetMemoryFilePath returns the path to the memory file, or empty string if not configured
func (p *Processor) GetMemoryFilePath() string {
	if p.memory == nil {
		return ""
	}
	return p.memory.GetFilePath()
}

// getGlobalToolConfig returns the global tool configuration from envConfig, converted to processor.ToolConfig
func (p *Processor) getGlobalToolConfig() *ToolConfig {
	if p.envConfig == nil {
		return nil
	}
	globalConfig := p.envConfig.GetToolConfig()
	if globalConfig == nil {
		return nil
	}
	// Convert from config.ToolConfig to processor.ToolConfig
	return &ToolConfig{
		Allowlist: globalConfig.Allowlist,
		Denylist:  globalConfig.Denylist,
		Timeout:   globalConfig.Timeout,
	}
}

// debugf prints debug information if verbose mode is enabled (thread-safe)
func (p *Processor) debugf(format string, args ...interface{}) {
	if p.verbose {
		p.mu.Lock()
		defer p.mu.Unlock()
		log.Printf("[DEBUG][DSL] "+format+"\n", args...)
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

// SubstituteCLIVariables replaces {{varname}} with CLI-provided values
func (p *Processor) SubstituteCLIVariables(text string) string {
	for name, value := range p.cliVariables {
		// Support both {{varname}} and {{ varname }} syntax
		text = strings.ReplaceAll(text, "{{"+name+"}}", value)
		text = strings.ReplaceAll(text, "{{ "+name+" }}", value)
	}
	return text
}

// substituteCLIVariablesInSlice applies CLI variable substitution to all elements in a slice
func (p *Processor) substituteCLIVariablesInSlice(items []string) {
	for i, item := range items {
		items[i] = p.SubstituteCLIVariables(item)
	}
}

// validateStepConfig checks if all required fields are present in a step
func (p *Processor) validateStepConfig(stepName string, config StepConfig) error {
	var errors []string

	isGenerateStep := config.Generate != nil
	isProcessStep := config.Process != nil
	isCodebaseIndexStep := config.Type == "codebase-index" || config.CodebaseIndex != nil
	isStandardStep := !isGenerateStep && !isProcessStep && !isCodebaseIndexStep && config.Type != "openai-responses" // Standard steps are not generate, process, codebase-index, or openai-responses
	isOpenAIResponsesStep := config.Type == "openai-responses"

	// Ensure a step is of one type only
	typeCount := 0
	if isStandardStep {
		typeCount++
	}
	if isGenerateStep {
		typeCount++
	}
	if isProcessStep {
		typeCount++
	}
	if isOpenAIResponsesStep { // This is a specific type of standard step, handled slightly differently
		// No increment here as it's a specialization of standard
	}

	if typeCount > 1 {
		errors = append(errors, "a step can only be one type: standard, generate, or process")
	}
	if isGenerateStep && (config.Input != nil || config.Model != nil || config.Action != nil || config.Output != nil) {
		// Allow Input: NA for generate steps if they don't need prior step's output
		if val, ok := config.Input.(string); !ok || val != "NA" {
			// errors = append(errors, "a 'generate' step should not contain 'input', 'model', 'action', or 'output' fields at the top level, unless Input is 'NA'")
		}
	}
	if isProcessStep && (config.Input != nil || config.Model != nil || config.Action != nil || config.Output != nil) {
		// Allow Input: NA for process steps
		if val, ok := config.Input.(string); !ok || val != "NA" {
			// errors = append(errors, "a 'process' step should not contain 'input', 'model', 'action', or 'output' fields at the top level, unless Input is 'NA'")
		}
	}

	if isStandardStep {
		if config.Input == nil {
			errors = append(errors, "input tag is required for standard steps (can be NA or empty, but the tag must be present)")
		}
		modelNames := p.NormalizeStringSlice(config.Model)
		if len(modelNames) == 0 {
			errors = append(errors, "model is required for standard steps (can be NA or a valid model name)")
		}
		actions := p.NormalizeStringSlice(config.Action)
		if len(actions) == 0 {
			errors = append(errors, "action is required for standard steps")
		}
		outputs := p.NormalizeStringSlice(config.Output)
		if len(outputs) == 0 {
			errors = append(errors, "output is required for standard steps (can be STDOUT for console output)")
		}
	} else if isOpenAIResponsesStep {
		// Validation specific to openai-responses type
		// For example, 'instructions' might be required instead of 'action'
		if config.Instructions == "" {
			// errors = append(errors, "'instructions' is required for 'openai-responses' type steps")
		}
		// Other openai-responses specific validations...
	} else if isGenerateStep {
		if config.Generate.Action == nil {
			errors = append(errors, "'action' is required within the 'generate' configuration")
		}
		if config.Generate.Output == "" {
			errors = append(errors, "'output' (filename) is required within the 'generate' configuration")
		}
	} else if isProcessStep {
		if config.Process.WorkflowFile == "" {
			errors = append(errors, "'workflow_file' is required within the 'process' configuration")
		}
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
			p.emitError(fmt.Errorf("%s", errMsg))
			return fmt.Errorf("validation error: %w", err)
		}

		// Validate model names only for standard steps (not generate, process, openai-responses, or codebase-index)
		isCodebaseIndex := step.Config.Type == "codebase-index" || step.Config.CodebaseIndex != nil
		if step.Config.Generate == nil && step.Config.Process == nil && step.Config.Type != "openai-responses" && !isCodebaseIndex {
			modelNames := p.NormalizeStringSlice(step.Config.Model)
			p.debugf("Normalized model names for step %s: %v", step.Name, modelNames)
			if err := p.validateModel(modelNames, []string{"STDIN"}); err != nil { // STDIN is a placeholder here
				p.spinner.Stop()
				errMsg := fmt.Sprintf("Model validation failed for step '%s': %v", step.Name, err)
				p.debugf("Model validation error: %s", errMsg)
				p.emitError(fmt.Errorf("%s", errMsg))
				return fmt.Errorf("model validation failed for step %s: %w", step.Name, err)
			}
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
				p.emitError(fmt.Errorf("%s", errMsg))
				return fmt.Errorf("validation error: %w", err)
			}

			// Validate model names only for standard steps (not generate, process, openai-responses, or codebase-index)
			isCodebaseIndex := step.Config.Type == "codebase-index" || step.Config.CodebaseIndex != nil
			if step.Config.Generate == nil && step.Config.Process == nil && step.Config.Type != "openai-responses" && !isCodebaseIndex {
				modelNames := p.NormalizeStringSlice(step.Config.Model)
				p.debugf("Normalized model names for parallel step %s: %v", step.Name, modelNames)
				if err := p.validateModel(modelNames, []string{"STDIN"}); err != nil { // STDIN is a placeholder
					p.spinner.Stop()
					errMsg := fmt.Sprintf("Model validation failed for parallel step '%s': %v", step.Name, err)
					p.debugf("Model validation error: %s", errMsg)
					p.emitError(fmt.Errorf("%s", errMsg))
					return fmt.Errorf("model validation failed for parallel step %s: %w", step.Name, err)
				}
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
		p.emitError(fmt.Errorf("%s", errMsg))
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

	// Process agentic loops
	for loopName, loopConfig := range p.config.AgenticLoops {
		p.debugf("Starting agentic loop: %s", loopName)
		p.spinner.Start(fmt.Sprintf("Processing agentic loop: %s", loopName))

		output, err := p.processAgenticLoop(loopName, loopConfig, p.lastOutput)
		if err != nil {
			p.spinner.Stop()
			p.emitError(err)
			return fmt.Errorf("agentic loop error: %w", err)
		}

		p.lastOutput = output
		p.spinner.Stop()
		p.debugf("Completed agentic loop: %s", loopName)
	}

	// Process sequential steps
	for stepIndex, step := range p.config.Steps {
		stepInfo := &StepInfo{
			Name:   step.Name,
			Model:  fmt.Sprintf("%v", step.Config.Model),
			Action: fmt.Sprintf("%v", step.Config.Action),
		}
		if step.Config.Generate != nil {
			stepInfo.Model = fmt.Sprintf("%v", step.Config.Generate.Model)
			stepInfo.Action = fmt.Sprintf("%v", step.Config.Generate.Action)
		} else if step.Config.Process != nil {
			stepInfo.Action = fmt.Sprintf("Process workflow: %s", step.Config.Process.WorkflowFile)
			stepInfo.Model = "N/A"
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
			p.emitError(fmt.Errorf("%s", errMsg))
			return fmt.Errorf("step processing error: %w", err)
		}

		// Store the response for potential use as STDIN in next step
		p.lastOutput = response

		p.spinner.Stop()
		p.debugf("Successfully processed step: %s", step.Name)

		// Check for deferred step execution
		if err := p.handleDeferredStep(); err != nil {
			return err
		}

		// Clear the handler's contents for the next step
		p.handler = input.NewHandler()
	}

	p.debugf("DSL processing completed successfully")
	return nil
}

// tryDispatchSpecialStep attempts to dispatch special step types (generate, process, agentic loop, codebase-index).
// Returns (result, handled, error). If handled is true, the step was processed.
func (p *Processor) tryDispatchSpecialStep(step Step, isParallel bool, parallelID string, metrics *PerformanceMetrics, startTime time.Time) (string, bool, error) {
	// Check if this is an openai-responses step
	if step.Config.Type == "openai-responses" {
		result, err := p.processResponsesStep(step, isParallel, parallelID)
		return result, true, err
	}

	// Handle generate step
	if step.Config.Generate != nil {
		result, err := p.processGenerateStep(step, isParallel, parallelID, metrics, startTime)
		return result, true, err
	}

	// Handle process step
	if step.Config.Process != nil {
		result, err := p.processProcessStep(step, isParallel, parallelID, metrics, startTime)
		return result, true, err
	}

	// Handle inline agentic loop step
	if step.Config.AgenticLoop != nil {
		result, err := p.processInlineAgenticLoop(step)
		return result, true, err
	}

	// Handle codebase-index step
	if step.Config.Type == "codebase-index" || step.Config.CodebaseIndex != nil {
		result, err := p.processCodebaseIndexStep(step, isParallel, parallelID)
		return result, true, err
	}

	return "", false, nil
}

// processStep handles the processing of a single step (used for both sequential and parallel processing)
func (p *Processor) processStep(step Step, isParallel bool, parallelID string) (string, error) {
	// Create performance metrics for this step
	metrics := &PerformanceMetrics{}
	startTime := time.Now()

	// Try to dispatch special step types first
	if result, handled, err := p.tryDispatchSpecialStep(step, isParallel, parallelID, metrics, startTime); handled {
		return result, err
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

	// Apply CLI variable substitution to inputs and actions
	p.substituteCLIVariablesInSlice(inputs)
	p.substituteCLIVariablesInSlice(actions)

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
				log.Printf("Error in step '%s': %v\n", step.Name, err)
				return "", err
			}
			tmpPath := tmpFile.Name()
			defer os.Remove(tmpPath)

			if _, err := tmpFile.WriteString(p.lastOutput); err != nil {
				tmpFile.Close()
				err = fmt.Errorf("failed to write to temp file: %w", err)
				log.Printf("Error in step '%s': %v\n", step.Name, err)
				return "", err
			}
			tmpFile.Close()

			// Update inputs to use the temporary file
			inputs = []string{tmpPath}
		}
	}

	// Handle tool input (e.g., "tool: ls -la" or "tool: STDIN|grep pattern")
	if len(inputs) == 1 && IsToolInput(inputs[0]) {
		p.debugf("Processing tool input for step: %s", step.Name)

		// Parse the tool command
		command, usesStdin, err := ParseToolInput(inputs[0])
		if err != nil {
			return "", fmt.Errorf("failed to parse tool input for step '%s': %w", step.Name, err)
		}

		// Create tool executor with merged global + step-level configuration
		stepToolConfig := &ToolConfig{}
		if step.Config.ToolConfig != nil {
			stepToolConfig.Allowlist = step.Config.ToolConfig.Allowlist
			stepToolConfig.Denylist = step.Config.ToolConfig.Denylist
			stepToolConfig.Timeout = step.Config.ToolConfig.Timeout
		}
		toolConfig := MergeToolConfigs(p.getGlobalToolConfig(), stepToolConfig)
		executor := NewToolExecutor(toolConfig, p.verbose, p.debugf)

		// Prepare stdin content if needed
		var stdinContent string
		if usesStdin {
			stdinContent = p.lastOutput
			p.debugf("Tool input uses STDIN, providing %d bytes of content", len(stdinContent))
		}

		// Execute the tool
		stdout, stderr, err := executor.Execute(command, stdinContent)
		if err != nil {
			// Include stderr in the error message for debugging
			if stderr != "" {
				return "", fmt.Errorf("tool execution failed for step '%s': %w\nstderr: %s", step.Name, err, stderr)
			}
			return "", fmt.Errorf("tool execution failed for step '%s': %w", step.Name, err)
		}

		if stderr != "" {
			p.debugf("Tool stderr: %s", stderr)
		}

		p.debugf("Tool output (%d bytes): %s", len(stdout), stdout[:min(200, len(stdout))])

		// Create a temporary file with the tool output
		tmpFile, err := os.CreateTemp("", "comanda-tool-*.txt")
		if err != nil {
			return "", fmt.Errorf("failed to create temp file for tool output: %w", err)
		}
		tmpPath := tmpFile.Name()
		defer os.Remove(tmpPath)

		if _, err := tmpFile.WriteString(stdout); err != nil {
			tmpFile.Close()
			return "", fmt.Errorf("failed to write tool output to temp file: %w", err)
		}
		tmpFile.Close()

		// Update inputs to use the temporary file
		inputs = []string{tmpPath}
	}

	// Check if chunking is enabled for this step
	var chunkResult *chunker.ChunkResult
	if step.Config.Chunk != nil && len(inputs) == 1 {
		// Only apply chunking to a single file input
		inputFile := inputs[0]

		// Skip chunking for special inputs like STDIN
		if inputFile != "STDIN" && inputFile != "NA" {
			p.debugf("Chunking enabled for step '%s', input file: %s", step.Name, inputFile)

			// Convert the ChunkConfig from the YAML to the chunker's ChunkConfig
			chunkConfig := chunker.ChunkConfig{
				By:        step.Config.Chunk.By,
				Size:      step.Config.Chunk.Size,
				Overlap:   step.Config.Chunk.Overlap,
				MaxChunks: step.Config.Chunk.MaxChunks,
			}

			// Split the file into chunks
			var err error
			chunkResult, err = chunker.SplitFile(inputFile, chunkConfig)
			if err != nil {
				errMsg := fmt.Sprintf("Failed to chunk file '%s' for step '%s': %v", inputFile, step.Name, err)
				p.debugf("%s", errMsg)
				return "", fmt.Errorf("%s", errMsg)
			}

			p.debugf("Successfully chunked file '%s' into %d chunks", inputFile, chunkResult.TotalChunks)

			// Replace the original input with the chunk paths
			inputs = chunkResult.ChunkPaths

			// Ensure cleanup of temporary files when the step is done
			defer func() {
				if err := chunker.CleanupChunks(chunkResult); err != nil {
					p.debugf("Error cleaning up chunks for step '%s': %v", step.Name, err)
					// Log the error but don't fail the step - cleanup errors are non-fatal
					log.Printf("Warning: Failed to clean up temporary chunk files: %v\n", err)
				}
			}()
		}
	}

	// Process inputs for this step
	if len(inputs) > 0 {
		p.debugf("Processing inputs for step %s...", step.Name)
		if err := p.processInputs(inputs); err != nil {
			err = fmt.Errorf("input processing error in step %s: %w", step.Name, err)
			log.Printf("Error: %v\n", err)
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

		// If we're processing chunks, add chunk-specific placeholders
		if chunkResult != nil && len(p.handler.GetInputs()) > 0 {
			// Get the current chunk index from the input path
			currentInput := p.handler.GetInputs()[0]
			chunkIndex := -1
			for i, chunkPath := range chunkResult.ChunkPaths {
				if chunkPath == currentInput.Path {
					chunkIndex = i
					break
				}
			}

			if chunkIndex >= 0 {
				// Replace placeholders with actual values
				substituted = strings.ReplaceAll(substituted, "{{ chunk_index }}", fmt.Sprintf("%d", chunkIndex+1))
				substituted = strings.ReplaceAll(substituted, "{{ total_chunks }}", fmt.Sprintf("%d", chunkResult.TotalChunks))
				substituted = strings.ReplaceAll(substituted, "{{ current_chunk }}", string(currentInput.Contents))
			}
		}

		// If memory is enabled for this step, inject memory content
		if step.Config.Memory {
			var memoryContent string

			// Combine file-based memory and external memory context
			if p.memory != nil && p.memory.HasMemory() {
				memoryContent = p.memory.GetMemory()
			}

			// Append external memory context if available
			if p.externalMemory != "" {
				if memoryContent != "" {
					memoryContent = memoryContent + "\n\n" + p.externalMemory
				} else {
					memoryContent = p.externalMemory
				}
			}

			if memoryContent != "" {
				// Prepend memory context to the action
				memoryPrefix := fmt.Sprintf("Context from project memory:\n---\n%s\n---\n\n", memoryContent)
				substituted = memoryPrefix + substituted
				p.debugf("Injected memory context into action (memory length: %d chars)", len(memoryContent))
			}
		}

		// If output is a file path, inject context so agents know to output content directly.
		// This helps agents (especially Claude Code in --print mode) understand that they
		// should output content directly rather than attempting to write files themselves.
		// Skip this when in agentic mode - the agent can write files directly.
		agenticConfig := p.getAgenticConfig()
		isAgenticMode := agenticConfig != nil && len(agenticConfig.AllowedPaths) > 0
		if !isAgenticMode {
			if outputPath := getFileOutputPath(step.Config.Output); outputPath != "" {
				outputContext := "[Output Handling]\nSimply output the content directly. Do not attempt to write files - your output will be captured automatically.\n\n"
				substituted = outputContext + substituted
				p.debugf("Injected output context into action (output path: %s)", outputPath)
			}
		} else {
			p.debugf("Skipping output context injection - agentic mode enabled")
		}

		substitutedActions[i] = substituted
		if original != substituted {
			p.debugf("Variable substitution: original='%s' substituted='%s'", original, substituted)
		}
	}

	p.debugf("Executing actions: models=%v actions=%v", modelNames, substitutedActions)
	actionResult, err := p.processActions(modelNames, substitutedActions)
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

	// Determine the final response to return (for downstream steps)
	var finalResponse string

	// Handle output based on type
	var handled bool
	switch v := step.Config.Output.(type) {
	case map[string]interface{}:
		if _, hasDB := v["database"]; hasDB {
			p.debugf("Processing database output for step '%s'", step.Name)
			// For database output, use combined result if available, otherwise join individual results
			dbOutput := actionResult.CombinedResult
			if actionResult.HasIndividualResults {
				dbOutput = strings.Join(actionResult.IndividualResults, "\n\n")
			}
			if err := p.handleDatabaseOutput(dbOutput, v); err != nil {
				errMsg := fmt.Sprintf("Database output processing failed for step '%s': %v (config=%v)",
					step.Name, err, v)
				p.debugf("Database output error: %s", errMsg)
				return "", fmt.Errorf("database output error: %w", err)
			}

			// For database outputs, we still want to show performance metrics in STDOUT
			if p.verbose {
				log.Printf("\nPerformance Metrics for step '%s':\n"+
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
			finalResponse = dbOutput
			handled = true
		}
	}

	// Handle regular output if not already handled
	if !handled {
		outputs := p.NormalizeStringSlice(step.Config.Output)

		// Handle individual results from either chunking or batch_mode: individual
		if actionResult.HasIndividualResults {
			p.debugf("Processing individual results (%d results)", len(actionResult.IndividualResults))

			// Determine the total count for template substitution
			totalCount := len(actionResult.IndividualResults)

			// For chunking, we may have explicit chunk info
			if chunkResult != nil {
				totalCount = chunkResult.TotalChunks
			}

			// Process each individual result with its own output file
			for idx, result := range actionResult.IndividualResults {
				// Safety check: ensure we have a corresponding input path
				if idx >= len(actionResult.InputPaths) {
					p.debugf("Warning: No input path for result at index %d, skipping", idx)
					continue
				}

				// Determine the index for this result
				var fileIndex int

				if chunkResult != nil && chunkResult.ChunkPaths != nil {
					// For chunking: Find chunk index for this input path
					chunkIndex := -1
					for i, chunkPath := range chunkResult.ChunkPaths {
						if chunkPath == actionResult.InputPaths[idx] {
							chunkIndex = i
							break
						}
					}

					if chunkIndex < 0 {
						p.debugf("Warning: Could not find chunk index for path %s", actionResult.InputPaths[idx])
						continue
					}
					fileIndex = chunkIndex
				} else {
					// For batch_mode: individual without chunking, use array index
					fileIndex = idx
				}

				// Substitute template variables in output filename
				substitutedOutputs := make([]string, len(outputs))
				for i, output := range outputs {
					substituted := output
					substituted = strings.ReplaceAll(substituted, "{{ chunk_index }}", fmt.Sprintf("%d", fileIndex))
					substituted = strings.ReplaceAll(substituted, "{{ total_chunks }}", fmt.Sprintf("%d", totalCount))
					// Also support {{ file_index }} as an alias for clarity in batch mode
					substituted = strings.ReplaceAll(substituted, "{{ file_index }}", fmt.Sprintf("%d", fileIndex))
					substituted = strings.ReplaceAll(substituted, "{{ total_files }}", fmt.Sprintf("%d", totalCount))
					substitutedOutputs[i] = substituted
					if output != substituted {
						p.debugf("File %d output filename substitution: original='%s' substituted='%s'", fileIndex, output, substituted)
					}
				}

				// Write this result to its corresponding output file
				p.debugf("Writing file %d/%d result to output", fileIndex, totalCount)
				if err := p.handleOutputWithToolConfig(modelNames[0], result, substitutedOutputs, metrics, step.Config.ToolConfig); err != nil {
					errMsg := fmt.Sprintf("Output processing failed for file %d of step '%s': %v",
						fileIndex, step.Name, err)
					p.debugf("Output processing error: %s", errMsg)
					return "", fmt.Errorf("output handling error: %w", err)
				}
			}

			// Combine all results for the return value (for downstream steps)
			finalResponse = strings.Join(actionResult.IndividualResults, "\n\n")
			p.debugf("Successfully processed all %d individual outputs for step: %s", len(actionResult.IndividualResults), step.Name)

		} else {
			// Standard output handling (single result or combined mode)
			response := actionResult.CombinedResult

			p.debugf("Processing regular output for step '%s': model=%s outputs=%v",
				step.Name, modelNames[0], outputs)
			if err := p.handleOutputWithToolConfig(modelNames[0], response, outputs, metrics, step.Config.ToolConfig); err != nil {
				errMsg := fmt.Sprintf("Output processing failed for step '%s': %v (model=%s outputs=%v)",
					step.Name, err, modelNames[0], outputs)
				p.debugf("Output processing error: %s", errMsg)
				return "", fmt.Errorf("output handling error: %w", err)
			}
			finalResponse = response
			p.debugf("Successfully processed output for step: %s", step.Name)
		}
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

	return finalResponse, nil
}

// processGenerateStep handles the logic for a 'generate' step
func (p *Processor) processGenerateStep(step Step, isParallel bool, parallelID string, metrics *PerformanceMetrics, startTime time.Time) (string, error) {
	stepInfo := &StepInfo{
		Name:   step.Name,
		Model:  fmt.Sprintf("%v", step.Config.Generate.Model),
		Action: fmt.Sprintf("%v", step.Config.Generate.Action),
	}
	p.debugf("Processing generate step: %s", step.Name)
	// Emit progress
	if isParallel {
		p.emitParallelProgress(fmt.Sprintf("Generating workflow: %s", step.Name), stepInfo, parallelID)
	} else {
		p.emitProgress(fmt.Sprintf("Generating workflow: %s", step.Name), stepInfo)
	}

	// 1. Determine model for generation
	var genModelName string
	if step.Config.Generate.Model != nil {
		modelNames := p.NormalizeStringSlice(step.Config.Generate.Model)
		if len(modelNames) > 0 {
			genModelName = modelNames[0] // Use the first model specified
		}
	}
	if genModelName == "" {
		genModelName = p.envConfig.DefaultGenerationModel // Use default from env config
		if genModelName == "" {
			return "", fmt.Errorf("no model specified for generate step '%s' and no default_generation_model configured", step.Name)
		}
	}
	p.debugf("Using model '%s' for workflow generation in step '%s'", genModelName, step.Name)

	// 2. Prepare the prompt for the LLM
	//    This includes the Comanda DSL guide and the user's action.
	// Use the embedded guide with injected model names instead of reading from file
	dslGuide := []byte(GetEmbeddedLLMGuide())

	userAction := ""
	if actions := p.NormalizeStringSlice(step.Config.Generate.Action); len(actions) > 0 {
		userAction = actions[0] // Assuming single action for generation prompt
	}
	if userAction == "" {
		return "", fmt.Errorf("action for generate step '%s' is empty", step.Name)
	}

	// Handle input for generate step (e.g., from STDIN or context_files)
	var contextInput string
	if step.Config.Input != nil {
		inputValStr := fmt.Sprintf("%v", step.Config.Input)
		if inputValStr == "STDIN" {
			contextInput = p.lastOutput
			p.debugf("Generate step '%s' using STDIN content as part of prompt context.", step.Name)
		} else if inputValStr != "NA" && inputValStr != "" {
			// If Input is a file path or direct string
			inputs := p.NormalizeStringSlice(step.Config.Input)
			if len(inputs) > 0 {
				// For simplicity, concatenate all inputs if multiple are provided.
				// A more sophisticated approach might handle them differently.
				var sb strings.Builder
				for _, inputPath := range inputs {
					content, err := os.ReadFile(inputPath)
					if err != nil {
						p.debugf("Warning: could not read input file %s for generate step %s: %v", inputPath, step.Name, err)
						continue // Or handle error more strictly
					}
					sb.Write(content)
					sb.WriteString("\n\n")
				}
				contextInput = sb.String()
				p.debugf("Generate step '%s' using content from specified input files as part of prompt context.", step.Name)
			}
		}
	}

	// Add content from context_files
	var contextFilesContent strings.Builder
	for _, filePath := range step.Config.Generate.ContextFiles {
		content, err := os.ReadFile(filePath)
		if err != nil {
			p.debugf("Warning: could not read context_file %s for generate step %s: %v", filePath, step.Name, err)
			continue
		}
		contextFilesContent.Write(content)
		contextFilesContent.WriteString("\n\n")
	}

	// Get list of configured models
	configuredModels := p.envConfig.GetAllConfiguredModels()
	var modelsList string
	if len(configuredModels) > 0 {
		modelsList = fmt.Sprintf("\n--- CONFIGURED MODELS ---\nThe following models are configured and available for use:\n%s\n--- END CONFIGURED MODELS ---\n", strings.Join(configuredModels, "\n"))
	} else {
		modelsList = "\n--- CONFIGURED MODELS ---\nNo models are currently configured. Use 'NA' for model fields.\n--- END CONFIGURED MODELS ---\n"
	}

	// Create a more forceful prompt that emphasizes YAML-only output
	fullPrompt := fmt.Sprintf(`SYSTEM: You are a YAML generator. You MUST output ONLY valid YAML content. No explanations, no markdown, no code blocks, no commentary - just raw YAML.

--- BEGIN COMANDA DSL SPECIFICATION ---
%s
--- END COMANDA DSL SPECIFICATION ---
%s
User's request: %s

Additional Context (if any):
%s
%s

CRITICAL INSTRUCTION: Your entire response must be valid YAML syntax that can be directly saved to a .yaml file. Do not include ANY text before or after the YAML content. Start your response with the first line of YAML and end with the last line of YAML.

IMPORTANT: When specifying models in the generated YAML, you MUST use one of the configured models listed above, or use 'NA' if no model is needed for a step.`,
		string(dslGuide), modelsList, userAction, contextInput, contextFilesContent.String())

	// 3. Call the LLM
	provider, err := p.getProviderForModel(genModelName)
	if err != nil {
		return "", fmt.Errorf("failed to get provider for model '%s' in generate step '%s': %w", genModelName, step.Name, err)
	}

	// Create a temporary input.Input for the LLM call
	// tempLLMInput := &input.Input{ // Not needed if SendPrompt takes a string
	// 	Contents: []byte(fullPrompt),
	// 	Path:     "generate-prompt", // Placeholder path
	// 	Type:     input.StdinInput,  // Assign a type, StdinInput seems appropriate for a string prompt
	// 	MimeType: "text/plain",
	// }

	// Assuming provider is already configured via configureProviders() or similar mechanism
	generatedResponse, err := provider.SendPrompt(genModelName, fullPrompt)
	if err != nil {
		return "", fmt.Errorf("LLM execution failed for generate step '%s' with model '%s': %w", step.Name, genModelName, err)
	}

	// Extract YAML content from the response
	yamlContent := generatedResponse

	// Check if the response contains code blocks
	if strings.Contains(generatedResponse, "```yaml") {
		// Extract content between ```yaml and ```
		startMarker := "```yaml"
		endMarker := "```"

		startIdx := strings.Index(generatedResponse, startMarker)
		if startIdx != -1 {
			startIdx += len(startMarker)
			// Find the next ``` after the start marker
			remaining := generatedResponse[startIdx:]
			endIdx := strings.Index(remaining, endMarker)
			if endIdx != -1 {
				yamlContent = strings.TrimSpace(remaining[:endIdx])
			}
		}
	} else if strings.Contains(generatedResponse, "```") {
		// Try generic code block
		parts := strings.Split(generatedResponse, "```")
		if len(parts) >= 3 {
			// Take the content of the first code block
			yamlContent = strings.TrimSpace(parts[1])
			// Remove language identifier if present (e.g., "yaml" at the start)
			lines := strings.Split(yamlContent, "\n")
			if len(lines) > 0 && !strings.Contains(lines[0], ":") {
				yamlContent = strings.Join(lines[1:], "\n")
			}
		}
	}

	// 4. Validate the generated YAML before saving
	if err := p.validateGeneratedWorkflow(yamlContent); err != nil {
		return "", fmt.Errorf("generated workflow validation failed for step '%s': %w", step.Name, err)
	}

	// 5. Save the generated YAML to the output file
	outputFilePath := step.Config.Generate.Output
	if err := os.WriteFile(outputFilePath, []byte(yamlContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write generated workflow to '%s' in generate step '%s': %w", outputFilePath, step.Name, err)
	}

	p.debugf("Generated workflow saved to %s", outputFilePath)

	// Record metrics
	metrics.TotalProcessingTime = time.Since(startTime).Milliseconds()
	if isParallel {
		p.emitParallelProgressWithMetrics(fmt.Sprintf("Completed generate step: %s", step.Name), stepInfo, parallelID, metrics)
	} else {
		p.emitProgressWithMetrics(fmt.Sprintf("Completed generate step: %s", step.Name), stepInfo, metrics)
	}

	return fmt.Sprintf("Generated workflow saved to %s", outputFilePath), nil
}

// processProcessStep handles the logic for a 'process' step
func (p *Processor) processProcessStep(step Step, isParallel bool, parallelID string, metrics *PerformanceMetrics, startTime time.Time) (string, error) {
	stepInfo := &StepInfo{
		Name:   step.Name,
		Action: fmt.Sprintf("Process workflow: %s", step.Config.Process.WorkflowFile),
		Model:  "N/A",
	}
	p.debugf("Processing process step: %s, workflow_file: %s", step.Name, step.Config.Process.WorkflowFile)
	// Emit progress
	if isParallel {
		p.emitParallelProgress(fmt.Sprintf("Processing sub-workflow: %s (%s)", step.Name, step.Config.Process.WorkflowFile), stepInfo, parallelID)
	} else {
		p.emitProgress(fmt.Sprintf("Processing sub-workflow: %s (%s)", step.Name, step.Config.Process.WorkflowFile), stepInfo)
	}

	// 1. Read the sub-workflow YAML file
	subWorkflowPath := step.Config.Process.WorkflowFile
	yamlFile, err := os.ReadFile(subWorkflowPath)
	if err != nil {
		return "", fmt.Errorf("failed to read sub-workflow file '%s' for process step '%s': %w", subWorkflowPath, step.Name, err)
	}

	var subDSLConfig DSLConfig
	if err := yaml.Unmarshal(yamlFile, &subDSLConfig); err != nil {
		return "", fmt.Errorf("failed to unmarshal sub-workflow YAML '%s' for process step '%s': %w", subWorkflowPath, step.Name, err)
	}

	// 2. Create a new Processor for the sub-workflow
	//    It inherits verbose settings and envConfig, but has its own DSLConfig and variables.
	//    The runtimeDir for the sub-processor could be the directory of the sub-workflow file or inherited.
	//    For now, let's assume it inherits the parent's runtimeDir.
	subProcessor := NewProcessor(&subDSLConfig, p.envConfig, p.serverConfig, p.verbose, p.runtimeDir)
	if p.progress != nil { // Propagate progress writer if available
		subProcessor.SetProgressWriter(p.progress)
	}

	// 3. Handle inputs for the sub-workflow (optional)
	if step.Config.Process.Inputs != nil {
		for key, value := range step.Config.Process.Inputs {
			// How these inputs are made available to the sub-workflow needs careful design.
			// Option 1: Set them as initial variables in the sub-processor.
			subProcessor.variables[key] = fmt.Sprintf("%v", value) // Convert value to string
			p.debugf("Passing input '%s' (value: '%v') to sub-workflow '%s'", key, value, subWorkflowPath)

			// Option 2: If the first step of sub-workflow expects STDIN and is named,
			// we could set p.lastOutput. This is more complex.
			// For now, using variables is simpler.
		}
	}

	// If the parent 'process' step received STDIN, pass it to the sub-processor's lastOutput
	if inputValStr := fmt.Sprintf("%v", step.Config.Input); inputValStr == "STDIN" {
		subProcessor.SetLastOutput(p.lastOutput)
		p.debugf("Passing STDIN from parent step '%s' to sub-workflow '%s'", step.Name, subWorkflowPath)
	}

	// 4. Execute the sub-workflow
	if err := subProcessor.Process(); err != nil {
		return "", fmt.Errorf("error processing sub-workflow '%s' in step '%s': %w", subWorkflowPath, step.Name, err)
	}

	// 5. Handle output capture (optional)
	//    This is a placeholder for now. Capturing specific outputs from a sub-workflow
	//    and making them available to the parent requires more design (e.g., sub-workflow
	//    explicitly "exports" variables or files).
	//    For now, the main output of the sub-processor (lastOutput) could be returned.

	resultMessage := fmt.Sprintf("Successfully processed sub-workflow %s", subWorkflowPath)
	p.debugf(resultMessage)

	// Record metrics
	metrics.TotalProcessingTime = time.Since(startTime).Milliseconds()
	if isParallel {
		p.emitParallelProgressWithMetrics(fmt.Sprintf("Completed process step: %s", step.Name), stepInfo, parallelID, metrics)
	} else {
		p.emitProgressWithMetrics(fmt.Sprintf("Completed process step: %s", step.Name), stepInfo, metrics)
	}

	return subProcessor.LastOutput(), nil // Return the last output of the sub-workflow
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

// validateGeneratedWorkflow validates that the generated YAML contains valid model references
func (p *Processor) validateGeneratedWorkflow(yamlContent string) error {
	p.debugf("Validating generated workflow YAML")

	// Parse the YAML to check for model validity
	var generatedConfig DSLConfig
	if err := yaml.Unmarshal([]byte(yamlContent), &generatedConfig); err != nil {
		return fmt.Errorf("generated YAML is invalid: %w", err)
	}

	// Collect all models referenced in the generated workflow
	var referencedModels []string

	// Check models in sequential steps
	for _, step := range generatedConfig.Steps {
		// Skip generate and process steps as they don't use models directly
		if step.Config.Generate != nil || step.Config.Process != nil {
			continue
		}

		// Check models in standard steps
		modelNames := p.NormalizeStringSlice(step.Config.Model)
		for _, modelName := range modelNames {
			if modelName != "NA" && modelName != "" {
				referencedModels = append(referencedModels, modelName)
			}
		}

		// Check models in generate steps
		if step.Config.Generate != nil && step.Config.Generate.Model != nil {
			genModelNames := p.NormalizeStringSlice(step.Config.Generate.Model)
			for _, modelName := range genModelNames {
				if modelName != "" {
					referencedModels = append(referencedModels, modelName)
				}
			}
		}
	}

	// Check models in parallel steps
	for _, steps := range generatedConfig.ParallelSteps {
		for _, step := range steps {
			// Skip generate and process steps
			if step.Config.Generate != nil || step.Config.Process != nil {
				continue
			}

			modelNames := p.NormalizeStringSlice(step.Config.Model)
			for _, modelName := range modelNames {
				if modelName != "NA" && modelName != "" {
					referencedModels = append(referencedModels, modelName)
				}
			}

			// Check models in generate steps
			if step.Config.Generate != nil && step.Config.Generate.Model != nil {
				genModelNames := p.NormalizeStringSlice(step.Config.Generate.Model)
				for _, modelName := range genModelNames {
					if modelName != "" {
						referencedModels = append(referencedModels, modelName)
					}
				}
			}
		}
	}

	// Validate each referenced model
	invalidModels := []string{}
	for _, modelName := range referencedModels {
		p.debugf("Checking if model '%s' in generated workflow is valid", modelName)

		// Check if provider exists for this model
		provider := models.DetectProvider(modelName)
		if provider == nil {
			invalidModels = append(invalidModels, fmt.Sprintf("%s (no provider found)", modelName))
			continue
		}

		// Check if provider supports this model
		if !provider.SupportsModel(modelName) {
			invalidModels = append(invalidModels, fmt.Sprintf("%s (not supported by %s)", modelName, provider.Name()))
			continue
		}

		// Check if model is configured
		_, err := p.envConfig.GetModelConfig(provider.Name(), modelName)
		if err != nil {
			if strings.Contains(err.Error(), "not found for provider") {
				invalidModels = append(invalidModels, fmt.Sprintf("%s (not configured - run 'comanda configure' to add it)", modelName))
			} else {
				invalidModels = append(invalidModels, fmt.Sprintf("%s (configuration error: %v)", modelName, err))
			}
		}
	}

	if len(invalidModels) > 0 {
		return fmt.Errorf("generated workflow contains invalid or unconfigured models:\n  - %s", strings.Join(invalidModels, "\n  - "))
	}

	p.debugf("Generated workflow validation successful - all %d model references are valid", len(referencedModels))
	return nil
}

// handleDeferredStep checks the last output for a deferred step call and executes it.
func (p *Processor) handleDeferredStep() error {
	var deferredCall struct {
		StepName string `json:"step"`
		Input    string `json:"input"`
	}

	// Trim whitespace and check if the output is a JSON object
	trimmedOutput := strings.TrimSpace(p.lastOutput)
	if !strings.HasPrefix(trimmedOutput, "{") || !strings.HasSuffix(trimmedOutput, "}") {
		return nil // Not a JSON object, so no deferred step to process
	}

	// Try to parse as JSON using encoding/json
	if err := json.Unmarshal([]byte(p.lastOutput), &deferredCall); err != nil {
		// Not a valid deferred call, so just continue
		p.debugf("Output is not a valid deferred step call: %v", err)
		return nil
	}

	p.debugf("Parsed deferred call: step=%s, input=%s", deferredCall.StepName, deferredCall.Input)

	if deferredCall.StepName == "" {
		return nil // No step name provided
	}

	deferredStepConfig, ok := p.config.Defer[deferredCall.StepName]
	if !ok {
		p.debugf("Deferred step '%s' not found in configuration", deferredCall.StepName)
		return nil // Step not found in defer block
	}

	p.debugf("Executing deferred step: %s", deferredCall.StepName)

	// Set the input for the deferred step
	p.lastOutput = deferredCall.Input

	// Create a Step object to process
	deferredStep := Step{
		Name:   deferredCall.StepName,
		Config: deferredStepConfig,
	}

	// Process the deferred step
	response, err := p.processStep(deferredStep, false, "")
	if err != nil {
		return fmt.Errorf("error processing deferred step '%s': %w", deferredCall.StepName, err)
	}

	// The output of the deferred step becomes the new lastOutput
	p.lastOutput = response

	p.debugf("Successfully processed deferred step: %s", deferredCall.StepName)
	return nil
}

// getProviderForModel retrieves a model provider based on the model name
func (p *Processor) getProviderForModel(modelName string) (models.Provider, error) {
	// First, check if the provider is already initialized
	for _, provider := range p.providers {
		if provider.SupportsModel(modelName) {
			return provider, nil
		}
	}

	// If not initialized, find the provider in the environment configuration
	for providerName, providerConfig := range p.envConfig.Providers {
		for _, model := range providerConfig.Models {
			if model.Name == modelName {
				// Initialize the provider if it's not already in the map
				if _, ok := p.providers[providerName]; !ok {
					var newProvider models.Provider
					switch providerName {
					case "openai":
						newProvider = models.NewOpenAIProvider()
					case "anthropic":
						newProvider = models.NewAnthropicProvider()
					case "google":
						newProvider = models.NewGoogleProvider()
					case "xai":
						newProvider = models.NewXAIProvider()
					case "deepseek":
						newProvider = models.NewDeepseekProvider()
					case "moonshot":
						newProvider = models.NewMoonshotProvider()
					case "ollama":
						newProvider = models.NewOllamaProvider()
					case "vllm":
						newProvider = models.NewVLLMProvider()
					default:
						return nil, fmt.Errorf("unknown provider: %s", providerName)
					}
					if err := newProvider.Configure(providerConfig.APIKey); err != nil {
						return nil, fmt.Errorf("failed to configure provider %s: %w", providerName, err)
					}
					newProvider.SetVerbose(p.verbose)
					p.providers[providerName] = newProvider
				}
				return p.providers[providerName], nil
			}
		}
	}

	return nil, fmt.Errorf("no provider configured or found for model %s", modelName)
}

// getFileOutputPath returns the file path if output is a file, empty string otherwise.
// This is used to inject output context into prompts so agents know where their output will be saved.
func getFileOutputPath(output interface{}) string {
	outputStr, ok := output.(string)
	if !ok {
		return ""
	}
	outputStr = strings.TrimSpace(outputStr)

	// Skip non-file outputs
	if outputStr == "" ||
		outputStr == "STDOUT" ||
		strings.HasPrefix(outputStr, "MEMORY") ||
		strings.HasPrefix(outputStr, "tool:") ||
		strings.Contains(outputStr, "|") ||
		strings.HasPrefix(outputStr, "$") {
		return ""
	}

	return outputStr
}
