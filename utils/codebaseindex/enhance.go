package codebaseindex

import (
	"fmt"
	"sort"
	"strings"
)

const maxEnhancementPromptBytes = 120000

func (m *Manager) enhanceIndex(scan *ScanResult, baseIndex string) (string, error) {
	if m.config.EnhancementFunc == nil {
		return "", fmt.Errorf("enhancement requested but no enhancement function was configured")
	}
	prompt := m.buildEnhancementPrompt(scan, baseIndex)
	markdown, err := m.config.EnhancementFunc(prompt)
	if err != nil {
		return "", err
	}
	markdown = cleanEnhancementMarkdown(markdown)
	if strings.TrimSpace(markdown) == "" {
		return "", fmt.Errorf("enhancement model returned empty analysis")
	}

	var sb strings.Builder
	sb.WriteString(strings.TrimSpace(baseIndex))
	sb.WriteString("\n\n---\n\n")
	if !strings.HasPrefix(strings.TrimSpace(markdown), "## ") {
		sb.WriteString("## AI Macro Analysis\n\n")
	}
	sb.WriteString(markdown)
	if !strings.HasSuffix(sb.String(), "\n") {
		sb.WriteString("\n")
	}
	return sb.String(), nil
}

func (m *Manager) buildEnhancementPrompt(scan *ScanResult, baseIndex string) string {
	var ctx strings.Builder
	ctx.WriteString("Repository: ")
	ctx.WriteString(m.config.RepoFileSlug)
	ctx.WriteString("\n")
	ctx.WriteString(fmt.Sprintf("Files indexed: %d candidates / %d total\n", len(scan.Candidates), scan.TotalFiles))
	ctx.WriteString(fmt.Sprintf("Monorepo detected: %t\n\n", scan.IsMonorepo))

	if len(scan.Components) > 0 {
		ctx.WriteString("Components:\n")
		for _, c := range scan.Components {
			ctx.WriteString(fmt.Sprintf("- %s root=%s language=%s kind=%s files=%d\n", c.Name, c.Root, c.Language, c.Kind, c.FileCount))
			if len(c.Frameworks) > 0 {
				ctx.WriteString("  frameworks: ")
				ctx.WriteString(strings.Join(c.Frameworks, ", "))
				ctx.WriteString("\n")
			}
			if len(c.ConfigFiles) > 0 {
				ctx.WriteString("  configs: ")
				ctx.WriteString(strings.Join(c.ConfigFiles, ", "))
				ctx.WriteString("\n")
			}
			if len(c.EntryPoints) > 0 {
				ctx.WriteString("  entrypoints: ")
				ctx.WriteString(strings.Join(c.EntryPoints, ", "))
				ctx.WriteString("\n")
			}
			if len(c.KeyDirs) > 0 {
				ctx.WriteString("  key dirs: ")
				ctx.WriteString(strings.Join(c.KeyDirs, ", "))
				ctx.WriteString("\n")
			}
		}
		ctx.WriteString("\n")
	}

	ctx.WriteString("Candidate files with symbols/frameworks/risk tags:\n")
	files := append([]*FileEntry(nil), scan.Candidates...)
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].Score == files[j].Score {
			return files[i].Path < files[j].Path
		}
		return files[i].Score > files[j].Score
	})
	if len(files) > 160 {
		files = files[:160]
	}
	for _, f := range files {
		ctx.WriteString("- ")
		ctx.WriteString(f.Path)
		ctx.WriteString(fmt.Sprintf(" language=%s score=%d", f.Language, f.Score))
		if f.IsEntrypoint {
			ctx.WriteString(" entrypoint")
		}
		if f.IsConfig {
			ctx.WriteString(" config")
		}
		if f.Symbols != nil {
			if len(f.Symbols.Frameworks) > 0 {
				ctx.WriteString(" frameworks=")
				ctx.WriteString(strings.Join(f.Symbols.Frameworks, ","))
			}
			if len(f.Symbols.RiskTags) > 0 {
				ctx.WriteString(" risks=")
				ctx.WriteString(strings.Join(f.Symbols.RiskTags, ","))
			}
			if len(f.Symbols.Types) > 0 {
				ctx.WriteString(" types=")
				ctx.WriteString(joinTypeNames(f.Symbols.Types, 5))
			}
			if len(f.Symbols.Functions) > 0 {
				ctx.WriteString(" funcs=")
				ctx.WriteString(joinFunctionNames(f.Symbols.Functions, 5))
			}
		}
		ctx.WriteString("\n")
	}

	baseExcerpt := baseIndex
	if len(baseExcerpt) > 50000 {
		baseExcerpt = baseExcerpt[:50000] + "\n...[base index truncated for prompt budget]..."
	}

	prompt := fmt.Sprintf(`You are improving a Comanda codebase index for future coding agents.

Goal: produce deep, repo-specific macro analysis, not generic best practices. The fast first-pass index already captured files/symbols. Your job is the second pass: infer architectural patterns, component boundaries, frontend/backend separation, data/control flow, testing/build conventions, and safe change strategy.

Rules:
- Return markdown only.
- Start with "## AI Macro Analysis".
- Include these subsections when supported by evidence:
  - Monorepo Shape & Component Boundaries
  - Deep Code Conventions & Agent Editing Rules
  - Frontend Patterns
  - Backend Patterns
  - Shared/Data/Infra Patterns
  - Cross-Cutting Conventions
  - Agent Change Playbook
  - Unknowns / Follow-up Exploration
- In Deep Code Conventions & Agent Editing Rules, use compact entries with Why, Agent guidance, Evidence paths, and Confidence when repeated code patterns support them.
- Cite concrete evidence paths inline. Do not invent files, frameworks, services, or business domains.
- Avoid generic advice like "follow best practices". Every claim should explain what pattern appears in this repository and how an agent should change code safely.
- If a section is not supported by evidence, say what is unknown and which files/dirs to inspect next.

--- FIRST-PASS MACRO CONTEXT ---
%s
--- END FIRST-PASS MACRO CONTEXT ---

--- CURRENT INDEX EXCERPT ---
%s
--- END CURRENT INDEX EXCERPT ---
`, ctx.String(), baseExcerpt)

	if len(prompt) > maxEnhancementPromptBytes {
		prompt = prompt[:maxEnhancementPromptBytes] + "\n...[prompt truncated to fit enhancement budget]..."
	}
	return prompt
}

func cleanEnhancementMarkdown(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```markdown") {
		s = strings.TrimPrefix(s, "```markdown")
		s = strings.TrimSuffix(s, "```")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}

func joinTypeNames(types []TypeInfo, limit int) string {
	var names []string
	for i, t := range types {
		if i >= limit {
			break
		}
		names = append(names, t.Name)
	}
	return strings.Join(names, ",")
}

func joinFunctionNames(funcs []FunctionInfo, limit int) string {
	var names []string
	for i, f := range funcs {
		if i >= limit {
			break
		}
		names = append(names, f.Name)
	}
	return strings.Join(names, ",")
}
