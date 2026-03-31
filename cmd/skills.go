package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/kris-hansen/comanda/utils/skills"
	"github.com/spf13/cobra"
)

var skillsFormat string

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Manage and invoke reusable skills",
	Long: `Skills are reusable workflow fragments defined in markdown files with YAML frontmatter.

Skill directories (in priority order):
  1. ~/.comanda/skills/       User-level skills (highest priority)
  2. .comanda/skills/         Project-level skills
  3. <install-dir>/skills/    Bundled skills (lowest priority)`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var skillsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all available skills",
	RunE: func(cmd *cobra.Command, args []string) error {
		idx := skills.NewIndex()
		if err := idx.Load(); err != nil {
			return fmt.Errorf("loading skills: %w", err)
		}

		allSkills := idx.All()
		if len(allSkills) == 0 {
			fmt.Println("No skills found.")
			fmt.Println("\nSkill directories searched:")
			for _, dir := range skills.SkillDirectoryPaths() {
				fmt.Printf("  %s\n", dir)
			}
			return nil
		}

		if skillsFormat == "json" {
			return printSkillsJSON(allSkills)
		}
		return printSkillsTable(allSkills)
	},
}

var skillsShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show details of a specific skill",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		idx := skills.NewIndex()
		if err := idx.Load(); err != nil {
			return fmt.Errorf("loading skills: %w", err)
		}

		skill := idx.Get(args[0])
		if skill == nil {
			return fmt.Errorf("skill %q not found", args[0])
		}

		fmt.Printf("Name:        %s\n", skill.DisplayName())
		fmt.Printf("Description: %s\n", skill.Description)
		fmt.Printf("Source:      %s\n", skill.Source)
		fmt.Printf("File:        %s\n", skill.FilePath)
		if skill.Model != "" {
			fmt.Printf("Model:       %s\n", skill.Model)
		}
		if skill.WhenToUse != "" {
			fmt.Printf("When to use: %s\n", skill.WhenToUse)
		}
		if len(skill.Arguments) > 0 {
			fmt.Printf("Arguments:   %s\n", strings.Join(skill.Arguments, ", "))
		}
		if skill.ArgumentHint != "" {
			fmt.Printf("Usage:       %s\n", skill.ArgumentHint)
		}
		if skill.Effort != "" {
			fmt.Printf("Effort:      %s\n", skill.Effort)
		}
		if len(skill.Paths) > 0 {
			fmt.Printf("Paths:       %s\n", strings.Join(skill.Paths, ", "))
		}
		fmt.Printf("\n--- Body ---\n%s\n", skill.Body)

		return nil
	},
}

var skillRunArgs []string

var skillsRunCmd = &cobra.Command{
	Use:   "run <name> [input...]",
	Short: "Invoke a skill directly",
	Long: `Run a skill by name, passing optional input text and named arguments.

Examples:
  comanda skills run summarize "Please summarize this document"
  comanda skills run summarize --arg length=short "Summarize this"
  echo "some text" | comanda skills run summarize`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		skillName := args[0]

		idx := skills.NewIndex()
		if err := idx.Load(); err != nil {
			return fmt.Errorf("loading skills: %w", err)
		}

		skill := idx.Get(skillName)
		if skill == nil {
			return fmt.Errorf("skill %q not found", skillName)
		}

		if !skill.IsUserInvocable() {
			return fmt.Errorf("skill %q is not user-invocable", skillName)
		}

		// Collect user input from remaining args
		userInput := strings.Join(args[1:], " ")

		// Parse --arg key=value flags
		parsedArgs := make(map[string]string)
		for _, a := range skillRunArgs {
			parts := strings.SplitN(a, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid argument format %q, expected key=value", a)
			}
			parsedArgs[parts[0]] = parts[1]
		}

		// Read stdin if piped
		stdinContent := skills.ReadStdin()

		result, err := skills.Prepare(skill, skills.ExecuteOptions{
			UserInput: userInput,
			Args:      parsedArgs,
			Stdin:     stdinContent,
		})
		if err != nil {
			return fmt.Errorf("preparing skill: %w", err)
		}

		// For Phase 3, this would execute via the processor/LLM.
		// For now, output the rendered prompt.
		fmt.Println(result.RenderedBody)
		if result.Model != "" {
			fmt.Fprintf(os.Stderr, "\n[Model: %s]\n", result.Model)
		}

		return nil
	},
}

func printSkillsTable(allSkills []*skills.Skill) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSOURCE\tMODEL\tDESCRIPTION")
	for _, s := range allSkills {
		model := s.Model
		if model == "" {
			model = "(default)"
		}
		desc := s.Description
		if len(desc) > 60 {
			desc = desc[:57] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", s.DisplayName(), s.Source, model, desc)
	}
	return w.Flush()
}

func printSkillsJSON(allSkills []*skills.Skill) error {
	type jsonSkill struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Source      string   `json:"source"`
		Model       string   `json:"model,omitempty"`
		FilePath    string   `json:"file_path"`
		Arguments   []string `json:"arguments,omitempty"`
	}

	out := make([]jsonSkill, len(allSkills))
	for i, s := range allSkills {
		out[i] = jsonSkill{
			Name:        s.DisplayName(),
			Description: s.Description,
			Source:      s.Source,
			Model:       s.Model,
			FilePath:    s.FilePath,
			Arguments:   s.Arguments,
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func init() {
	skillsListCmd.Flags().StringVar(&skillsFormat, "format", "table", "Output format: table or json")
	skillsRunCmd.Flags().StringArrayVar(&skillRunArgs, "arg", nil, "Named argument in key=value format")

	skillsCmd.AddCommand(skillsListCmd)
	skillsCmd.AddCommand(skillsShowCmd)
	skillsCmd.AddCommand(skillsRunCmd)

	rootCmd.AddCommand(skillsCmd)
}
