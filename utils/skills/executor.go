package skills

import (
	"fmt"
	"io"
	"os"
)

// ExecuteOptions holds options for executing a skill.
type ExecuteOptions struct {
	UserInput string            // User-provided input text
	Args      map[string]string // Named argument substitutions
	Stdin     string            // Content from STDIN
	Model     string            // Model override (overrides skill's model)
}

// ExecuteResult holds the result of executing a skill.
type ExecuteResult struct {
	RenderedBody string // The skill body after variable substitution
	Model        string // The resolved model to use
}

// Prepare resolves a skill into its rendered body and model, ready for execution.
// It does not actually call an LLM - that is handled by the processor.
func Prepare(skill *Skill, opts ExecuteOptions) (*ExecuteResult, error) {
	if skill == nil {
		return nil, fmt.Errorf("skill is nil")
	}

	// Render body with variable substitution
	body := RenderBody(skill, opts.UserInput, opts.Args, opts.Stdin)

	// Resolve model: option override > skill default
	model := skill.Model
	if opts.Model != "" {
		model = opts.Model
	}

	return &ExecuteResult{
		RenderedBody: body,
		Model:        model,
	}, nil
}

// ReadStdin reads all available stdin if it's being piped.
func ReadStdin() string {
	info, err := os.Stdin.Stat()
	if err != nil {
		return ""
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		// stdin is a terminal, not a pipe
		return ""
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return ""
	}
	return string(data)
}
