package processor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Default values for agentic loop safety
const (
	DefaultMaxIterations      = 10
	DefaultTimeoutSeconds     = 0 // 0 = no timeout (rely on max_iterations)
	DefaultContextWindow      = 5
	DefaultCheckpointInterval = 5 // Save state every N iterations
)

// processAgenticLoop handles the execution of an agentic loop
func (p *Processor) processAgenticLoop(loopName string, config *AgenticLoopConfig, initialInput string) (string, error) {
	return p.processAgenticLoopWithFile(loopName, config, initialInput, "")
}

// processAgenticLoopWithFile handles the execution of an agentic loop with optional workflow file tracking
func (p *Processor) processAgenticLoopWithFile(loopName string, config *AgenticLoopConfig, initialInput string, workflowFile string) (string, error) {
	p.debugf("Starting agentic loop: %s", loopName)

	// Stream log header
	if p.streamLog != nil && p.streamLog.IsEnabled() {
		p.streamLog.LogSection(fmt.Sprintf("AGENTIC LOOP: %s", loopName))
		p.streamLog.Log("Max iterations: %d", config.MaxIterations)
		p.streamLog.Log("Exit condition: %s", config.ExitCondition)
		if len(config.AllowedPaths) > 0 {
			p.streamLog.Log("Allowed paths: %v", config.AllowedPaths)
		}
	}

	// Check if agentic tools are enabled and we have allowed paths
	if len(config.AllowedPaths) > 0 {
		if p.envConfig != nil && !p.envConfig.IsAgenticToolsAllowed() {
			return "", fmt.Errorf("agentic tool use is disabled in global config (security.allow_agentic_tools: false)")
		}
		p.debugf("Agentic tools enabled with allowed paths: %v, tools: %v", config.AllowedPaths, config.Tools)
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
		p.setLoopVariables(loopCtx, maxIterations)

		// Build iteration input with context
		iterationInput := p.buildIterationContext(loopCtx, contextWindow)

		// Execute the loop steps
		output, err := p.executeLoopSteps(config.Steps, iterationInput)
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

// setLoopVariables sets template variables for the current loop iteration
func (p *Processor) setLoopVariables(loopCtx *LoopContext, maxIterations int) {
	p.variables["loop.iteration"] = fmt.Sprintf("%d", loopCtx.Iteration)
	p.variables["loop.previous_output"] = loopCtx.PreviousOutput
	p.variables["loop.total_iterations"] = fmt.Sprintf("%d", maxIterations)
	p.variables["loop.elapsed_seconds"] = fmt.Sprintf("%.0f", time.Since(loopCtx.StartTime).Seconds())
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

// executeLoopSteps runs the sub-steps within an agentic loop iteration
func (p *Processor) executeLoopSteps(steps []Step, input string) (string, error) {
	var output string

	// If no sub-steps, return error
	if len(steps) == 0 {
		return "", fmt.Errorf("agentic loop has no steps defined")
	}

	// Set the input for the first step
	p.lastOutput = input

	for _, step := range steps {
		result, err := p.processStep(step, false, "")
		if err != nil {
			return "", err
		}
		output = result
		p.lastOutput = result
	}

	return output, nil
}

// checkExitCondition determines if the loop should exit
func (p *Processor) checkExitCondition(config *AgenticLoopConfig, output string) (bool, string) {
	switch config.ExitCondition {
	case "llm_decides", "":
		// Look for common completion indicators
		completionPatterns := []string{
			`(?i)^\s*DONE\s*$`,
			`(?i)^\s*COMPLETE\s*$`,
			`(?i)^\s*FINISHED\s*$`,
			`(?i)TASK[_\s-]?COMPLETE`,
		}
		trimmedOutput := strings.TrimSpace(output)
		for _, pattern := range completionPatterns {
			if matched, _ := regexp.MatchString(pattern, trimmedOutput); matched {
				return true, "LLM indicated completion"
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
	}

	return p.processAgenticLoop(step.Name, config, p.lastOutput)
}
