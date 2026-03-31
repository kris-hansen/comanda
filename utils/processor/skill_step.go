package processor

import (
	"fmt"
	"time"

	"github.com/kris-hansen/comanda/utils/skills"
)

// processSkillStep handles execution of a skill step in a workflow.
// It loads the skill, renders its body with variable substitution,
// and executes it as if it were an inline prompt step.
func (p *Processor) processSkillStep(step Step, isParallel bool, parallelID string, metrics *PerformanceMetrics, startTime time.Time) (string, error) {
	skillName := step.Config.Skill
	p.debugf("Processing skill step: %s, skill: %s", step.Name, skillName)

	stepInfo := &StepInfo{
		Name:   step.Name,
		Action: fmt.Sprintf("Skill: %s", skillName),
		Model:  "N/A",
	}

	if isParallel {
		p.emitParallelProgress(fmt.Sprintf("Loading skill: %s (%s)", step.Name, skillName), stepInfo, parallelID)
	} else {
		p.emitProgress(fmt.Sprintf("Loading skill: %s (%s)", step.Name, skillName), stepInfo)
	}

	// Load the skill from the index
	idx := skills.NewIndex()
	if err := idx.Load(); err != nil {
		return "", fmt.Errorf("loading skills index for step '%s': %w", step.Name, err)
	}

	skill := idx.Get(skillName)
	if skill == nil {
		return "", fmt.Errorf("skill %q not found for step '%s'", skillName, step.Name)
	}

	// Determine input for the skill
	userInput := ""
	if step.Config.Input != nil {
		inputStr := fmt.Sprintf("%v", step.Config.Input)
		if inputStr == InputSTDIN {
			userInput = p.lastOutput
		} else if inputStr != "NA" && inputStr != "" {
			// Check if it's a variable reference
			userInput = p.substituteVariables(inputStr)
		}
	} else if p.lastOutput != "" {
		// Use previous step output as input
		userInput = p.lastOutput
	}

	// Resolve model: step override > skill default > leave empty for processor default
	model := skill.Model
	if step.Config.Model != nil {
		modelStr := fmt.Sprintf("%v", step.Config.Model)
		if modelStr != "" {
			model = modelStr
		}
	}

	// Prepare the skill
	result, err := skills.Prepare(skill, skills.ExecuteOptions{
		UserInput: userInput,
		Args:      step.Config.SkillArgs,
		Stdin:     "",
		Model:     model,
	})
	if err != nil {
		return "", fmt.Errorf("preparing skill %q for step '%s': %w", skillName, step.Name, err)
	}

	// Create a synthetic step with the rendered skill body as the action
	// and execute it through the normal processor pipeline
	syntheticStep := Step{
		Name: step.Name,
		Config: StepConfig{
			Input:  "NA",
			Model:  result.Model,
			Action: result.RenderedBody,
			Output: step.Config.Output,
		},
	}

	// If the skill has no model set, use the input from the original step
	if result.Model == "" {
		// Fall back: use whatever model the step or config provides
		syntheticStep.Config.Model = step.Config.Model
	}

	p.debugf("Executing skill %q as synthetic step with model=%v", skillName, syntheticStep.Config.Model)

	// Execute the synthetic step through the normal pipeline
	output, err := p.processStep(syntheticStep, isParallel, parallelID)
	if err != nil {
		return "", fmt.Errorf("executing skill %q in step '%s': %w", skillName, step.Name, err)
	}

	// Record metrics
	metrics.TotalProcessingTime = time.Since(startTime).Milliseconds()
	if isParallel {
		p.emitParallelProgressWithMetrics(fmt.Sprintf("Completed skill step: %s", step.Name), stepInfo, parallelID, metrics)
	} else {
		p.emitProgressWithMetrics(fmt.Sprintf("Completed skill step: %s", step.Name), stepInfo, metrics)
	}

	return output, nil
}
