package processor

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Default values for agentic loop safety
const (
	DefaultMaxIterations  = 10
	DefaultTimeoutSeconds = 300
	DefaultContextWindow  = 5
)

// processAgenticLoop handles the execution of an agentic loop
func (p *Processor) processAgenticLoop(loopName string, config *AgenticLoopConfig, initialInput string) (string, error) {
	p.debugf("Starting agentic loop: %s", loopName)

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
	if timeoutSeconds <= 0 {
		timeoutSeconds = DefaultTimeoutSeconds
	}

	contextWindow := config.ContextWindow
	if contextWindow <= 0 {
		contextWindow = DefaultContextWindow
	}

	// Initialize loop context
	loopCtx := &LoopContext{
		Iteration:      0,
		PreviousOutput: initialInput,
		History:        make([]LoopIteration, 0),
		StartTime:      time.Now(),
	}

	// Create timeout context
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	var finalOutput string

	// Main loop
	for loopCtx.Iteration < maxIterations {
		select {
		case <-ctx.Done():
			p.debugf("Agentic loop '%s' timed out after %d seconds", loopName, timeoutSeconds)
			return finalOutput, fmt.Errorf("agentic loop '%s' timed out after %d seconds", loopName, timeoutSeconds)
		default:
		}

		loopCtx.Iteration++
		p.debugf("Agentic loop '%s' iteration %d/%d", loopName, loopCtx.Iteration, maxIterations)

		// Set loop template variables
		p.setLoopVariables(loopCtx, maxIterations)

		// Build iteration input with context
		iterationInput := p.buildIterationContext(loopCtx, contextWindow)

		// Execute the loop steps
		output, err := p.executeLoopSteps(config.Steps, iterationInput)
		if err != nil {
			return "", fmt.Errorf("error in agentic loop '%s' iteration %d: %w", loopName, loopCtx.Iteration, err)
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

		// Check exit condition
		shouldExit, reason := p.checkExitCondition(config, output)
		if shouldExit {
			p.debugf("Agentic loop '%s' exiting: %s", loopName, reason)
			break
		}
	}

	if loopCtx.Iteration >= maxIterations {
		p.debugf("Agentic loop '%s' reached max iterations (%d)", loopName, maxIterations)
	}

	return finalOutput, nil
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
