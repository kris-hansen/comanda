package skills

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Skill represents a parsed skill file with frontmatter and body content.
type Skill struct {
	// Frontmatter fields
	Name          string   `yaml:"name"`
	Description   string   `yaml:"description"`
	WhenToUse     string   `yaml:"when_to_use"`
	Model         string   `yaml:"model"`
	AllowedTools  []string `yaml:"allowed-tools"`
	Arguments     []string `yaml:"arguments"`
	ArgumentHint  string   `yaml:"argument-hint"`
	Version       string   `yaml:"version"`
	Effort        string   `yaml:"effort"`
	Context       string   `yaml:"context"`
	Paths         []string `yaml:"paths"`
	UserInvocable *bool    `yaml:"user-invocable"`

	// Computed fields (not from YAML)
	Body     string `yaml:"-"` // Markdown body after frontmatter
	FilePath string `yaml:"-"` // Absolute path to the skill file
	Source   string `yaml:"-"` // "user", "project", or "bundled"
	Dir      string `yaml:"-"` // Directory containing the skill file
}

// IsUserInvocable returns whether this skill can be invoked directly by users.
// Defaults to true if not specified.
func (s *Skill) IsUserInvocable() bool {
	if s.UserInvocable == nil {
		return true
	}
	return *s.UserInvocable
}

// DisplayName returns the skill's display name, falling back to filename.
func (s *Skill) DisplayName() string {
	if s.Name != "" {
		return s.Name
	}
	return strings.TrimSuffix(filepath.Base(s.FilePath), ".md")
}

// LoadSkill reads and parses a skill file from disk.
func LoadSkill(path string) (*Skill, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading skill file: %w", err)
	}

	skill, err := ParseSkill(string(data))
	if err != nil {
		return nil, fmt.Errorf("parsing skill %s: %w", path, err)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		absPath = path
	}
	skill.FilePath = absPath
	skill.Dir = filepath.Dir(absPath)

	// Default name from filename if not set
	if skill.Name == "" {
		skill.Name = strings.TrimSuffix(filepath.Base(path), ".md")
	}

	return skill, nil
}

// ParseSkill parses a skill from its raw content (frontmatter + body).
func ParseSkill(content string) (*Skill, error) {
	frontmatter, body, err := splitFrontmatter(content)
	if err != nil {
		return nil, err
	}

	skill := &Skill{}
	if frontmatter != "" {
		if err := yaml.Unmarshal([]byte(frontmatter), skill); err != nil {
			return nil, fmt.Errorf("parsing frontmatter YAML: %w", err)
		}
	}

	skill.Body = body
	return skill, nil
}

// splitFrontmatter splits a markdown file into YAML frontmatter and body.
// Frontmatter is delimited by --- on its own line at the start.
func splitFrontmatter(content string) (frontmatter string, body string, err error) {
	scanner := bufio.NewScanner(strings.NewReader(content))

	// First line must be ---
	if !scanner.Scan() {
		return "", content, nil
	}
	firstLine := strings.TrimSpace(scanner.Text())
	if firstLine != "---" {
		// No frontmatter, entire content is body
		return "", content, nil
	}

	// Read until closing ---
	var fmLines []string
	foundClose := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			foundClose = true
			break
		}
		fmLines = append(fmLines, line)
	}

	if !foundClose {
		return "", "", fmt.Errorf("unclosed frontmatter: missing closing ---")
	}

	frontmatter = strings.Join(fmLines, "\n")

	// Rest is body
	var bodyLines []string
	for scanner.Scan() {
		bodyLines = append(bodyLines, scanner.Text())
	}
	body = strings.Join(bodyLines, "\n")
	// Trim leading blank line that typically follows frontmatter
	body = strings.TrimLeft(body, "\n")

	return frontmatter, body, nil
}

// SubstituteVariables replaces ${...} placeholders in the skill body.
// Handles both ${KEY} and ${KEY:-default} forms.
func SubstituteVariables(body string, vars map[string]string) string {
	result := body
	for key, value := range vars {
		// Replace exact match: ${KEY}
		result = strings.ReplaceAll(result, "${"+key+"}", value)
		// Replace with-default match: ${KEY:-anything}
		prefix := "${" + key + ":-"
		for {
			idx := strings.Index(result, prefix)
			if idx == -1 {
				break
			}
			end := strings.Index(result[idx:], "}")
			if end == -1 {
				break
			}
			end += idx
			result = result[:idx] + value + result[end+1:]
		}
	}
	return result
}

// BuildVariables constructs the variable map for substitution.
func BuildVariables(skill *Skill, userInput string, args map[string]string, stdin string) map[string]string {
	vars := make(map[string]string)

	// Built-in variables
	vars["USER_INPUT"] = userInput
	vars["TIMESTAMP"] = time.Now().UTC().Format(time.RFC3339)
	vars["STDIN"] = stdin
	if skill.Dir != "" {
		vars["COMANDA_SKILL_DIR"] = skill.Dir
	}

	// Named arguments from args map
	for key, value := range args {
		vars[key] = value
	}

	// Handle default values for arguments: ${argname:-default}
	// We do this as a post-processing step after standard substitution
	return vars
}

// RenderBody applies variable substitution to the skill body and handles defaults.
func RenderBody(skill *Skill, userInput string, args map[string]string, stdin string) string {
	vars := BuildVariables(skill, userInput, args, stdin)
	result := SubstituteVariables(skill.Body, vars)

	// Handle ${var:-default} syntax for remaining unsubstituted variables
	result = resolveDefaults(result)

	return result
}

// resolveDefaults handles ${var:-default} syntax.
// If the variable was already substituted, this is a no-op.
// If it remains as ${var:-default}, it gets replaced with "default".
func resolveDefaults(text string) string {
	var result strings.Builder
	i := 0
	for i < len(text) {
		if i+1 < len(text) && text[i] == '$' && text[i+1] == '{' {
			// Find the closing }
			end := strings.Index(text[i:], "}")
			if end == -1 {
				result.WriteByte(text[i])
				i++
				continue
			}
			end += i // absolute index

			inner := text[i+2 : end] // content between ${ and }
			if idx := strings.Index(inner, ":-"); idx >= 0 {
				// Has default value
				defaultVal := inner[idx+2:]
				result.WriteString(defaultVal)
			} else {
				// No default, leave as-is (already substituted or unknown)
				result.WriteString(text[i : end+1])
			}
			i = end + 1
		} else {
			result.WriteByte(text[i])
			i++
		}
	}
	return result.String()
}
