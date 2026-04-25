package processor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/kris-hansen/comanda/utils/models"
)

// Default values for agentic loop safety
const (
	DefaultMaxIterations      = 10
	DefaultTimeoutSeconds     = 0 // 0 = no timeout (rely on max_iterations)
	DefaultContextWindow      = 5
	DefaultCheckpointInterval = 5 // Save state every N iterations
)

// Pre-compiled regex patterns for exit condition detection (better performance)
var (
	completionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)^\s*DONE\.?\s*$`),      // DONE as entire output
		regexp.MustCompile(`(?i)^\s*COMPLETE\.?\s*$`),  // COMPLETE as entire output
		regexp.MustCompile(`(?i)^\s*FINISHED\.?\s*$`),  // FINISHED as entire output
		regexp.MustCompile(`(?i)\bDONE\.?\s*$`),        // DONE at end of output
		regexp.MustCompile(`(?i)\bCOMPLETE\.?\s*$`),    // COMPLETE at end of output
		regexp.MustCompile(`(?i)\bFINISHED\.?\s*$`),    // FINISHED at end of output
		regexp.MustCompile(`(?i)^.*\bDONE\.?\s*$`),     // DONE at end of any line (multiline)
		regexp.MustCompile(`(?i)^.*\bCOMPLETE\.?\s*$`), // COMPLETE at end of any line
		regexp.MustCompile(`(?i)^.*\bFINISHED\.?\s*$`), // FINISHED at end of any line
		regexp.MustCompile(`(?i)TASK[_\s-]?COMPLETE`),  // TASK_COMPLETE anywhere
	}
	contextExhaustionPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)completion[_\-\s]?plan`),
		regexp.MustCompile(`(?i)context[_\s]*(limit|exhaust|full|window)`),
		regexp.MustCompile(`(?i)remaining[_\s]*work`),
		regexp.MustCompile(`(?i)continue[_\s]*in[_\s]*(a\s+)?new[_\s]*session`),
		regexp.MustCompile(`(?i)documented.*remaining`),
		regexp.MustCompile(`(?i)out\s+of\s+(context|tokens?)`),
		regexp.MustCompile(`(?i)cannot\s+continue`),
		regexp.MustCompile(`(?i)unable\s+to\s+complete`),
	}
)

// processAgenticLoop handles the execution of an agentic loop
func (p *Processor) processAgenticLoop(loopName string, config *AgenticLoopConfig, initialInput string) (string, error) {
	return p.processAgenticLoopWithFile(loopName, config, initialInput, "")
}

// processAgenticLoopWithFile handles the execution of an agentic loop with optional workflow file tracking
func (p *Processor) processAgenticLoopWithFile(loopName string, config *AgenticLoopConfig, initialInput string, workflowFile string) (string, error) {
	p.debugf("Starting agentic loop: %s", loopName)

	// Apply intelligent path defaults if allowed_paths is empty
	if len(config.AllowedPaths) == 0 {
		config.AllowedPaths = p.inferDefaultAllowedPaths(workflowFile)
		if len(config.AllowedPaths) > 0 {
			p.debugf("Auto-inferred allowed_paths: %v", config.AllowedPaths)
			if p.streamLog != nil && p.streamLog.IsEnabled() {
				p.streamLog.Log("ℹ️  No allowed_paths specified, defaulting to: %v", config.AllowedPaths)
			}
		}
	}

	// Stream log header
	if p.streamLog != nil && p.streamLog.IsEnabled() {
		p.streamLog.LogSection(fmt.Sprintf("AGENTIC LOOP: %s", loopName))
		p.streamLog.Log("Max iterations: %d", config.MaxIterations)
		p.streamLog.Log("Exit condition: %s", config.ExitCondition)
		if len(config.AllowedPaths) > 0 {
			p.streamLog.Log("Allowed paths: %v", config.AllowedPaths)
		}
	}

	// Auto-add output directories to allowed_paths so agents can write output files
	// This prevents "permission denied" errors when output paths aren't explicitly in allowed_paths
	config.AllowedPaths = p.expandAllowedPathsWithOutputDirs(config.AllowedPaths, config.Steps)

	// Auto-expand to include common project directories that exist
	config.AllowedPaths = p.expandWithCommonProjectDirs(config.AllowedPaths)

	// Check if agentic tools are enabled and we have allowed paths
	if len(config.AllowedPaths) > 0 {
		if p.envConfig != nil && !p.envConfig.IsAgenticToolsAllowed() {
			return "", fmt.Errorf("agentic tool use is disabled in global config (security.allow_agentic_tools: false)")
		}
		p.debugf("Agentic tools enabled with allowed paths: %v, tools: %v", config.AllowedPaths, config.Tools)

		// Generate file manifest for token awareness
		manifest, err := GenerateFileManifest(config.AllowedPaths)
		if err != nil {
			p.debugf("Warning: failed to generate file manifest: %v", err)
		} else if manifest.HasLargeFiles() {
			// Prepend manifest to initial input so agent is aware of large files
			manifestStr := manifest.Manifest()
			initialInput = manifestStr + "\n---\n\n" + initialInput
			p.debugf("Injected file manifest: %d oversized, %d large files", manifest.OversizedCount, manifest.LargeCount)

			// Log to stream log
			if p.streamLog != nil {
				p.streamLog.Log("📁 File manifest: %d total files, %d oversized (>25k tokens), %d large (10-25k tokens)",
					manifest.TotalFiles, manifest.OversizedCount, manifest.LargeCount)
				for _, f := range manifest.FilterByCategory("oversized") {
					p.streamLog.Log("   ❌ %s (~%dk tokens)", f.RelPath, f.EstimatedTokens/1000)
				}
			}
		}
	}

	// Set the current agentic config (used by action handler)
	p.setAgenticConfig(config)
	defer func() {
		p.setAgenticConfig(nil)
	}()

	// Apply defaults
	maxIterations := config.MaxIterations
	if maxIterations <= 0 {
		maxIterations = DefaultMaxIterations
	}

	timeoutSeconds := config.TimeoutSeconds
	if timeoutSeconds < 0 {
		timeoutSeconds = DefaultTimeoutSeconds
	}

	contextWindow := config.ContextWindow
	if contextWindow <= 0 {
		contextWindow = DefaultContextWindow
	}

	checkpointInterval := config.CheckpointInterval
	if checkpointInterval <= 0 && config.Stateful {
		checkpointInterval = DefaultCheckpointInterval
	}

	// Initialize state manager if stateful
	var stateManager *LoopStateManager
	if config.Stateful {
		if config.Name == "" {
			return "", fmt.Errorf("stateful loops require a name")
		}
		stateDir := p.getLoopStateDir()
		stateManager = NewLoopStateManager(stateDir)
	}

	// Check for existing state (resume capability)
	var loopCtx *LoopContext
	if config.Stateful && config.Name != "" {
		if existingState, err := stateManager.LoadState(config.Name); err == nil {
			// Validate workflow hasn't changed
			if workflowFile != "" && existingState.WorkflowFile != "" {
				if err := ValidateWorkflowChecksum(workflowFile, existingState.WorkflowChecksum); err != nil {
					p.debugf("Warning: %v - starting fresh", err)
					loopCtx = p.createNewLoopContext(initialInput)
				} else {
					loopCtx = stateToLoopContext(existingState)
					p.debugf("Resuming loop '%s' from iteration %d", config.Name, loopCtx.Iteration)
				}
			} else {
				loopCtx = stateToLoopContext(existingState)
				p.debugf("Resuming loop '%s' from iteration %d", config.Name, loopCtx.Iteration)
			}
		} else {
			loopCtx = p.createNewLoopContext(initialInput)
		}
	} else {
		loopCtx = p.createNewLoopContext(initialInput)
	}

	// Create timeout context (0 = no timeout)
	var ctx context.Context
	var cancel context.CancelFunc
	if timeoutSeconds > 0 {
		ctx, cancel = context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
		defer cancel()
	} else {
		ctx = context.Background()
	}

	var finalOutput string
	var finalErr error

	// Main loop
	for loopCtx.Iteration < maxIterations {
		// Check timeout if set
		if timeoutSeconds > 0 {
			select {
			case <-ctx.Done():
				p.debugf("Agentic loop '%s' timed out after %d seconds", loopName, timeoutSeconds)
				// Save state before timeout exit
				if config.Stateful {
					state := loopStateFromContext(loopCtx, config.Name, config, workflowFile, p.variables)
					state.Status = "paused"
					if err := stateManager.SaveState(state); err != nil {
						p.debugf("Warning: Failed to save state on timeout: %v", err)
					}
				}
				return finalOutput, fmt.Errorf("agentic loop '%s' timed out, state saved for resume", loopName)
			default:
			}
		}

		loopCtx.Iteration++
		p.debugf("Agentic loop '%s' iteration %d/%d", loopName, loopCtx.Iteration, maxIterations)

		// Stream log iteration start
		if p.streamLog != nil {
			p.streamLog.LogIteration(loopCtx.Iteration, maxIterations, loopName)
		}

		// Set loop template variables
		p.ensureLoopPromptState(loopCtx, config.Steps)
		p.setLoopVariables(loopCtx, config.Steps, maxIterations)

		// Build iteration input with context
		iterationInput := p.buildIterationContext(loopCtx, contextWindow)

		// Execute the loop steps
		output, err := p.executeLoopSteps(loopCtx, config, config.Steps, iterationInput)
		if err != nil {
			finalErr = fmt.Errorf("error in agentic loop '%s' iteration %d: %w", loopName, loopCtx.Iteration, err)
			// Stream log error
			if p.streamLog != nil {
				p.streamLog.LogError(finalErr)
			}
			// Save failed state
			if config.Stateful {
				state := loopStateFromContext(loopCtx, config.Name, config, workflowFile, p.variables)
				state.Status = "failed"
				if saveErr := stateManager.SaveState(state); saveErr != nil {
					p.debugf("Warning: Failed to save failed state: %v", saveErr)
				}
			}
			return "", finalErr
		}

		// Stream log iteration output
		if p.streamLog != nil {
			p.streamLog.LogOutput(output, 50) // Show first 50 lines
		}

		// Record this iteration
		iteration := LoopIteration{
			Index:     loopCtx.Iteration,
			Output:    output,
			Timestamp: time.Now(),
		}
		loopCtx.History = append(loopCtx.History, iteration)
		loopCtx.PreviousOutput = output
		finalOutput = output

		// Run quality gates
		if len(config.QualityGates) > 0 {
			p.debugf("Running %d quality gates for iteration %d", len(config.QualityGates), loopCtx.Iteration)
			gateResults, err := RunQualityGates(config.QualityGates, p.runtimeDir)

			if err != nil {
				p.debugf("Quality gate failure: %v", err)
				// Save failed state
				if config.Stateful {
					state := loopStateFromContext(loopCtx, config.Name, config, workflowFile, p.variables)
					state.Status = "failed"
					state.QualityGateResults = gateResults
					if saveErr := stateManager.SaveState(state); saveErr != nil {
						p.debugf("Warning: Failed to save failed state: %v", saveErr)
					}
				}
				return finalOutput, fmt.Errorf("quality gate failed in iteration %d: %w", loopCtx.Iteration, err)
			}

			// Log gate results
			for _, result := range gateResults {
				if result.Passed {
					p.debugf("Quality gate '%s' passed (attempts: %d, duration: %v)", result.GateName, result.Attempts, result.Duration)
				} else {
					p.debugf("Quality gate '%s' failed after %d attempts: %s", result.GateName, result.Attempts, result.Message)
				}
			}
		}

		// Checkpoint save
		if config.Stateful && checkpointInterval > 0 && loopCtx.Iteration%checkpointInterval == 0 {
			state := loopStateFromContext(loopCtx, config.Name, config, workflowFile, p.variables)
			state.Status = "running"
			if err := stateManager.SaveState(state); err != nil {
				p.debugf("Warning: Failed to save checkpoint: %v", err)
			} else {
				p.debugf("Checkpoint saved at iteration %d", loopCtx.Iteration)
			}
		}

		// Check exit condition
		shouldExit, reason := p.checkExitCondition(config, output)
		if shouldExit {
			p.debugf("Agentic loop '%s' exiting: %s", loopName, reason)
			// Stream log exit
			if p.streamLog != nil {
				p.streamLog.LogExit(reason)
			}
			break
		}
	}

	if loopCtx.Iteration >= maxIterations {
		p.debugf("Agentic loop '%s' reached max iterations (%d)", loopName, maxIterations)
	}

	// Final state save
	if config.Stateful {
		finalState := loopStateFromContext(loopCtx, config.Name, config, workflowFile, p.variables)
		finalState.Status = "completed"
		if err := stateManager.SaveState(finalState); err != nil {
			p.debugf("Warning: Failed to save final state: %v", err)
		} else {
			p.debugf("Loop completed, final state saved")
		}
	}

	return finalOutput, nil
}

// createNewLoopContext initializes a fresh loop context
func (p *Processor) createNewLoopContext(initialInput string) *LoopContext {
	return &LoopContext{
		Iteration:      0,
		PreviousOutput: initialInput,
		History:        make([]LoopIteration, 0),
		StartTime:      time.Now(),
		CurrentActions: make(map[string]string),
	}
}

// getLoopStateDir returns the directory for storing loop states
func (p *Processor) getLoopStateDir() string {
	// Use the config helper to get the loop state directory
	// Import cycle prevention: we use a local implementation
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ".comanda/loop-states"
	}
	return filepath.Join(homeDir, ".comanda", "loop-states")
}

// setLoopVariables sets template variables for the current loop iteration.
func (p *Processor) setLoopVariables(loopCtx *LoopContext, steps []Step, maxIterations int) {
	loopVars := map[string]string{
		"loop.iteration":        fmt.Sprintf("%d", loopCtx.Iteration),
		"loop.previous_output":  loopCtx.PreviousOutput,
		"loop.total_iterations": fmt.Sprintf("%d", maxIterations),
		"loop.elapsed_seconds":  fmt.Sprintf("%.0f", time.Since(loopCtx.StartTime).Seconds()),
	}

	if currentPrompt := p.primaryLoopPrompt(loopCtx, steps); currentPrompt != "" {
		loopVars["loop.current_prompt"] = currentPrompt
	}

	for name, value := range loopVars {
		p.variables[name] = value
		if p.cliVariables != nil {
			p.cliVariables[name] = value
		}
	}
}

// buildIterationContext constructs the input for an iteration with historical context
func (p *Processor) buildIterationContext(loopCtx *LoopContext, contextWindow int) string {
	var sb strings.Builder

	// Include relevant history
	historyStart := len(loopCtx.History) - contextWindow
	if historyStart < 0 {
		historyStart = 0
	}

	if len(loopCtx.History) > 0 && historyStart < len(loopCtx.History) {
		sb.WriteString("=== Previous Iterations ===\n")
		for _, iter := range loopCtx.History[historyStart:] {
			sb.WriteString(fmt.Sprintf("--- Iteration %d ---\n", iter.Index))
			sb.WriteString(fmt.Sprintf("Output: %s\n\n", iter.Output))
		}
		sb.WriteString("=== Current Iteration ===\n")
	}

	// Add the previous output as current input
	sb.WriteString(loopCtx.PreviousOutput)

	return sb.String()
}

// executeLoopSteps runs the sub-steps within an agentic loop iteration.
func (p *Processor) executeLoopSteps(loopCtx *LoopContext, loopConfig *AgenticLoopConfig, steps []Step, input string) (string, error) {
	var output string

	// If no sub-steps, return error
	if len(steps) == 0 {
		return "", fmt.Errorf("agentic loop has no steps defined")
	}

	// Set the input for the first step
	p.lastOutput = input

	for i, step := range steps {
		stepKey := loopStepKey(step, i)
		stepToExecute := step
		if currentAction, ok := loopCtx.CurrentActions[stepKey]; ok && currentAction != "" {
			stepToExecute.Config.Action = currentAction
		}

		// Log step start
		if p.streamLog != nil {
			stepName := stepToExecute.Name
			if stepName == "" {
				stepName = fmt.Sprintf("step-%d", i+1)
			}
			model := fmt.Sprintf("%v", stepToExecute.Config.Model)
			if model == "" || model == "<nil>" {
				model = "(default)"
			}
			p.streamLog.Log("→ Starting %s [model: %s]", stepName, model)
			if stepToExecute.Config.Action != nil {
				p.streamLog.Log("  Action: %v", stepToExecute.Config.Action)
			}
			inputPreview := p.lastOutput
			if len(inputPreview) > 200 {
				inputPreview = inputPreview[:200] + "..."
			}
			p.streamLog.Log("  Input: %s", inputPreview)
		}

		stepStart := time.Now()
		result, err := p.processStep(stepToExecute, false, "")

		// Log step completion
		if p.streamLog != nil {
			elapsed := time.Since(stepStart)
			if err != nil {
				p.streamLog.Log("✖ Step failed after %v: %v", elapsed.Round(time.Second), err)
			} else {
				outputPreview := result
				if len(outputPreview) > 300 {
					outputPreview = outputPreview[:300] + "..."
				}
				p.streamLog.Log("✓ Step completed in %v", elapsed.Round(time.Second))
				p.streamLog.Log("  Output: %s", outputPreview)
			}
		}

		if err != nil {
			return "", err
		}
		output = result
		p.lastOutput = result

		if err := p.refineLoopStepPrompt(loopCtx, loopConfig, stepToExecute, stepKey, result); err != nil {
			return "", err
		}
	}

	return output, nil
}

func (p *Processor) ensureLoopPromptState(loopCtx *LoopContext, steps []Step) {
	if loopCtx.CurrentActions == nil {
		loopCtx.CurrentActions = make(map[string]string, len(steps))
	}
	for i, step := range steps {
		stepKey := loopStepKey(step, i)
		if _, exists := loopCtx.CurrentActions[stepKey]; exists {
			continue
		}
		if action := firstStepAction(step); action != "" {
			loopCtx.CurrentActions[stepKey] = action
		}
	}
}

func loopStepKey(step Step, idx int) string {
	if step.Name != "" {
		return step.Name
	}
	return fmt.Sprintf("step-%d", idx+1)
}

func firstStepAction(step Step) string {
	switch action := step.Config.Action.(type) {
	case string:
		return strings.TrimSpace(action)
	case []string:
		if len(action) > 0 {
			return strings.TrimSpace(action[0])
		}
	case []interface{}:
		if len(action) > 0 {
			if text, ok := action[0].(string); ok {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

func (p *Processor) primaryLoopPrompt(loopCtx *LoopContext, steps []Step) string {
	for i, step := range steps {
		if prompt := loopCtx.CurrentActions[loopStepKey(step, i)]; prompt != "" {
			return prompt
		}
	}

	keys := make([]string, 0, len(loopCtx.CurrentActions))
	for key := range loopCtx.CurrentActions {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		if loopCtx.CurrentActions[key] != "" {
			return loopCtx.CurrentActions[key]
		}
	}
	return ""
}

func (p *Processor) refineLoopStepPrompt(loopCtx *LoopContext, loopConfig *AgenticLoopConfig, step Step, stepKey, latestResult string) error {
	if !p.isPromptImprovementEnabled(loopConfig, step) {
		return nil
	}

	currentAction := loopCtx.CurrentActions[stepKey]
	if currentAction == "" {
		currentAction = firstStepAction(step)
	}
	if currentAction == "" {
		return nil
	}

	modelNames := p.NormalizeStringSlice(step.Config.Model)
	modelName := p.promptImprovementModel(loopConfig, modelNames)
	if modelName == "" || modelName == "NA" {
		return nil
	}

	provider := models.DetectProvider(modelName)
	if provider == nil {
		return fmt.Errorf("provider not found for prompt improvement model: %s", modelName)
	}
	configuredProvider := p.providers[provider.Name()]
	if configuredProvider == nil {
		return fmt.Errorf("provider %s not configured", provider.Name())
	}

	instructions := p.promptImprovementInstructions(loopConfig, step)
	refinementPrompt := fmt.Sprintf(`You are refining the prompt for the next iteration of an agentic loop.

Return ONLY the improved prompt text to use next. Do not include commentary, markdown fences, or explanations.

Current iteration: %d
Current prompt:
---
%s
---

Latest result:
---
%s
---

Additional guidance:
%s`, loopCtx.Iteration, currentAction, latestResult, instructions)

	improvedAction, err := configuredProvider.SendPrompt(modelName, refinementPrompt)
	if err != nil {
		return fmt.Errorf("failed to refine loop prompt for step '%s': %w", stepKey, err)
	}

	improvedAction = strings.TrimSpace(improvedAction)
	if improvedAction != "" {
		loopCtx.CurrentActions[stepKey] = improvedAction
		if p.streamLog != nil {
			p.streamLog.Log("↻ Refined prompt for %s", stepKey)
		}
	}

	return nil
}

func (p *Processor) isPromptImprovementEnabled(loopConfig *AgenticLoopConfig, step Step) bool {
	if loopConfig != nil && loopConfig.PromptImprovement != nil {
		if loopConfig.PromptImprovement.Enabled || loopConfig.PromptImprovement.Model != "" || loopConfig.PromptImprovement.Instructions != "" {
			return true
		}
		return true
	}

	return len(p.NormalizeStringSlice(step.Config.NextAction)) > 0
}

func (p *Processor) promptImprovementModel(loopConfig *AgenticLoopConfig, stepModels []string) string {
	if loopConfig != nil && loopConfig.PromptImprovement != nil && loopConfig.PromptImprovement.Model != "" {
		return loopConfig.PromptImprovement.Model
	}
	if len(stepModels) > 0 {
		return stepModels[0]
	}
	return ""
}

func (p *Processor) promptImprovementInstructions(loopConfig *AgenticLoopConfig, step Step) string {
	if nextActions := p.NormalizeStringSlice(step.Config.NextAction); len(nextActions) > 0 {
		return strings.Join(nextActions, "\n")
	}
	if loopConfig != nil && loopConfig.PromptImprovement != nil && strings.TrimSpace(loopConfig.PromptImprovement.Instructions) != "" {
		return loopConfig.PromptImprovement.Instructions
	}
	return "Make the next prompt more specific, preserve the user's goal, incorporate what worked, avoid what failed, and increase the chance of a stronger next iteration."
}

// checkExitCondition determines if the loop should exit
func (p *Processor) checkExitCondition(config *AgenticLoopConfig, output string) (bool, string) {
	// Early return for empty output to avoid unnecessary processing
	if strings.TrimSpace(output) == "" {
		return false, ""
	}

	switch config.ExitCondition {
	case "llm_decides", "":
		// Look for common completion indicators using pre-compiled patterns
		// Patterns match DONE/COMPLETE/FINISHED:
		// - As the entire output (with optional whitespace)
		// - At the end of output (e.g., "All tasks completed. DONE")
		// - On their own line
		trimmedOutput := strings.TrimSpace(output)
		for _, pattern := range completionPatterns {
			if pattern.MatchString(trimmedOutput) {
				return true, "LLM indicated completion"
			}
		}

		// Check for context exhaustion / completion plan signals
		// These indicate the agent realized it can't continue and documented remaining work
		for _, pattern := range contextExhaustionPatterns {
			if pattern.MatchString(output) {
				return true, "Agent signaled context exhaustion or documented remaining work"
			}
		}

		return false, ""

	case "pattern_match":
		if config.ExitPattern == "" {
			return false, ""
		}
		if matched, _ := regexp.MatchString(config.ExitPattern, output); matched {
			return true, fmt.Sprintf("Pattern '%s' matched", config.ExitPattern)
		}
		return false, ""

	default:
		return false, ""
	}
}

// processInlineAgenticLoop handles a step with inline agentic_loop configuration
func (p *Processor) processInlineAgenticLoop(step Step) (string, error) {
	config := step.Config.AgenticLoop
	if config == nil {
		return "", fmt.Errorf("step '%s' has no agentic loop config", step.Name)
	}

	// Create a single-step loop using the step's own config
	singleStep := Step{
		Name: step.Name,
		Config: StepConfig{
			Input:      step.Config.Input,
			Model:      step.Config.Model,
			Action:     step.Config.Action,
			Output:     step.Config.Output,
			Memory:     step.Config.Memory,
			ToolConfig: step.Config.ToolConfig,
		},
	}

	// If no steps defined in the loop config, use the step itself
	if len(config.Steps) == 0 {
		config.Steps = []Step{singleStep}
	} else {
		// Inherit model from parent step if sub-steps don't specify their own
		parentModel := step.Config.Model

		// Validate: if parent has no model, check if any sub-steps need one
		if parentModel == nil {
			for _, subStep := range config.Steps {
				if subStep.Config.Model == nil && subStep.Config.Generate != nil {
					return "", fmt.Errorf("agentic loop '%s': sub-step '%s' requires a model but none specified (and parent has no model to inherit)",
						step.Name, subStep.Name)
				}
			}
		}

		for i := range config.Steps {
			if config.Steps[i].Config.Model == nil {
				config.Steps[i].Config.Model = parentModel
				if parentModel != nil {
					p.debugf("Step '%s' inherited model from parent: %v", config.Steps[i].Name, parentModel)
				}
			}
		}
	}

	return p.processAgenticLoop(step.Name, config, p.lastOutput)
}

// expandAllowedPathsWithOutputDirs adds parent directories of any output file paths
// to the allowed_paths list. This ensures agents can write to output locations
// without requiring explicit configuration of every output directory.
// It also creates any directories in allowed_paths that don't exist yet.
func (p *Processor) expandAllowedPathsWithOutputDirs(allowedPaths []string, steps []Step) []string {
	// Use a map to deduplicate paths
	pathSet := make(map[string]bool)
	for _, path := range allowedPaths {
		pathSet[path] = true

		// Create allowed_path directories if they don't exist
		// This prevents permission errors when the agent tries to write to .comanda/ etc.
		absPath, err := filepath.Abs(path)
		if err != nil {
			p.debugf("Warning: failed to resolve allowed_path %s: %v", path, err)
			continue
		}

		// Check if this looks like a directory path (no extension or ends with /)
		// or if it already exists as a directory
		info, statErr := os.Stat(absPath)
		isDir := (statErr == nil && info.IsDir()) ||
			strings.HasSuffix(path, "/") ||
			filepath.Ext(path) == ""

		if isDir && os.IsNotExist(statErr) {
			if err := os.MkdirAll(absPath, 0755); err != nil {
				p.debugf("Warning: failed to create allowed_path directory %s: %v", absPath, err)
			} else {
				p.debugf("Auto-created allowed_path directory: %s", absPath)
				if p.streamLog != nil && p.streamLog.IsEnabled() {
					p.streamLog.Log("📁 Created directory: %s", path)
				}
			}
		}
	}

	// Extract output directories from steps
	for _, step := range steps {
		outputPath := p.extractOutputPath(step.Config.Output)
		if outputPath != "" {
			// Get the parent directory of the output file
			dir := filepath.Dir(outputPath)

			// Resolve to absolute path
			absDir, err := filepath.Abs(dir)
			if err != nil {
				p.debugf("Warning: failed to resolve output directory %s: %v", dir, err)
				continue
			}

			// Add to set if not already present
			if !pathSet[absDir] {
				pathSet[absDir] = true
				p.debugf("Auto-added output directory to allowed_paths: %s (from output: %s)", absDir, outputPath)
			}

			// Create the output directory if it doesn't exist
			// This prevents permission errors when the agent tries to write
			if _, err := os.Stat(absDir); os.IsNotExist(err) {
				if err := os.MkdirAll(absDir, 0755); err != nil {
					p.debugf("Warning: failed to create output directory %s: %v", absDir, err)
				} else {
					p.debugf("Auto-created output directory: %s", absDir)
				}
			}
		}
	}

	// Convert set back to slice
	result := make([]string, 0, len(pathSet))
	for path := range pathSet {
		result = append(result, path)
	}

	return result
}

// extractOutputPath extracts the file path from an output configuration
func (p *Processor) extractOutputPath(output interface{}) string {
	if output == nil {
		return ""
	}

	outputStr, ok := output.(string)
	if !ok {
		return ""
	}

	outputStr = strings.TrimSpace(outputStr)

	// Skip non-file outputs
	if outputStr == "" ||
		outputStr == OutputSTDOUT ||
		outputStr == "NA" ||
		strings.HasPrefix(outputStr, "$") { // Variable reference
		return ""
	}

	// Expand ~ and resolve path
	if strings.HasPrefix(outputStr, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			outputStr = filepath.Join(home, outputStr[2:])
		}
	} else if outputStr == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			outputStr = home
		}
	}

	return outputStr
}

// inferDefaultAllowedPaths returns sensible default paths when none are specified.
// In CLI mode, workflow-relative paths are resolved from the invocation cwd,
// so the default agentic access path should match that same cwd.
func (p *Processor) inferDefaultAllowedPaths(workflowFile string) []string {
	cwd, err := os.Getwd()
	if err != nil {
		return nil
	}

	if workflowFile != "" {
		p.debugf("Using current working directory as default allowed_path for workflow %s: %s", workflowFile, cwd)
	} else {
		p.debugf("Using current working directory as default allowed_path: %s", cwd)
	}
	return []string{cwd}
}

// expandWithCommonProjectDirs adds common project subdirectories to allowed_paths
// if they exist. This reduces friction for typical project structures.
func (p *Processor) expandWithCommonProjectDirs(allowedPaths []string) []string {
	if len(allowedPaths) == 0 {
		return allowedPaths
	}

	// Common project directories that agents often need to access
	commonDirs := []string{
		"src", "lib", "pkg", "cmd", // Source code
		"test", "tests", "spec", "specs", // Tests
		"docs", "doc", "documentation", // Documentation
		"build", "dist", "out", "output", // Build outputs
		"scripts", "bin", "tools", // Scripts and tools
		"config", "configs", "conf", // Configuration
		"assets", "static", "public", // Static assets
		"internal", "vendor", "third_party", // Dependencies
		".github", ".gitlab", // CI/CD
	}

	// Use a set to deduplicate
	pathSet := make(map[string]bool)
	for _, p := range allowedPaths {
		pathSet[p] = true
	}

	// For each base allowed path, check if common subdirs exist and add them
	for _, basePath := range allowedPaths {
		info, err := os.Stat(basePath)
		if err != nil || !info.IsDir() {
			continue
		}

		for _, subdir := range commonDirs {
			fullPath := filepath.Join(basePath, subdir)
			if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
				if !pathSet[fullPath] {
					pathSet[fullPath] = true
					p.debugf("Auto-added common project directory: %s", fullPath)
				}
			}
		}
	}

	// Convert back to slice
	result := make([]string, 0, len(pathSet))
	for path := range pathSet {
		result = append(result, path)
	}
	return result
}

// FormatPathAccessError creates a helpful error message when path access fails
// It shows what paths were allowed and suggests what to add
func FormatPathAccessError(failedPath string, allowedPaths []string) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Cannot access path: %s\n", failedPath))
	sb.WriteString("\nCurrently allowed paths:\n")
	for _, p := range allowedPaths {
		sb.WriteString(fmt.Sprintf("  • %s\n", p))
	}

	// Suggest what to add
	suggestedPath := failedPath
	if info, err := os.Stat(failedPath); err == nil && !info.IsDir() {
		// If it's a file, suggest the parent directory
		suggestedPath = filepath.Dir(failedPath)
	}
	// Clean up the path for suggestion
	if absPath, err := filepath.Abs(suggestedPath); err == nil {
		suggestedPath = absPath
	}

	sb.WriteString(fmt.Sprintf("\n💡 Suggestion: Add this to allowed_paths in your workflow:\n"))
	sb.WriteString(fmt.Sprintf("  allowed_paths:\n"))
	sb.WriteString(fmt.Sprintf("    - %s\n", suggestedPath))

	return sb.String()
}
