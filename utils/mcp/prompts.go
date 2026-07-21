package mcp

import (
	"context"
	"fmt"
	"log"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kris-hansen/comanda/utils/skills"
)

// registerPrompt exposes a comanda skill as an MCP prompt. The prompt returns
// the skill's rendered body as a single user message; skills.Prepare performs
// the variable substitution without calling any LLM.
func registerPrompt(s *mcpsdk.Server, skill *skills.Skill, verbose bool) {
	name := skill.DisplayName()
	if name == "" {
		log.Printf("[WARN][MCP] Skipping skill with no name (%s)\n", skill.FilePath)
		return
	}

	description := skill.Description
	if description == "" {
		description = fmt.Sprintf("Comanda skill '%s'", name)
	}

	prompt := &mcpsdk.Prompt{
		Name:        name,
		Description: description,
		Arguments:   promptArguments(skill),
	}

	s.AddPrompt(prompt, func(ctx context.Context, req *mcpsdk.GetPromptRequest) (*mcpsdk.GetPromptResult, error) {
		args := make(map[string]string)
		if req.Params != nil {
			args = req.Params.Arguments
		}
		if verbose {
			log.Printf("[DEBUG][MCP] Rendering skill prompt %s with %d argument(s)\n", name, len(args))
		}
		result, err := skills.Prepare(skill, skills.ExecuteOptions{Args: args})
		if err != nil {
			return nil, fmt.Errorf("failed to render skill %q: %w", name, err)
		}
		return &mcpsdk.GetPromptResult{
			Description: description,
			Messages: []*mcpsdk.PromptMessage{
				{
					Role:    "user",
					Content: &mcpsdk.TextContent{Text: result.RenderedBody},
				},
			},
		}, nil
	})
}

// promptArguments maps a skill's declared arguments to MCP prompt arguments.
// The skill's argument-hint, when present, is used as the argument description.
func promptArguments(skill *skills.Skill) []*mcpsdk.PromptArgument {
	if len(skill.Arguments) == 0 {
		return nil
	}
	args := make([]*mcpsdk.PromptArgument, 0, len(skill.Arguments))
	for _, argName := range skill.Arguments {
		description := skill.ArgumentHint
		if description == "" {
			description = fmt.Sprintf("Value for the %s argument", argName)
		}
		args = append(args, &mcpsdk.PromptArgument{
			Name:        argName,
			Description: description,
		})
	}
	return args
}
