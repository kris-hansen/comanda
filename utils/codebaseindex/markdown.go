package codebaseindex

import (
	"path/filepath"
	"strings"
)

// markdownContextFileNames are well-known documents that should remain visible
// in bounded indexes. Only the stricter instruction set below creates guides
// relationships in the knowledge graph.
var markdownContextFileNames = map[string]bool{
	"agents.md":                true,
	"claude.md":                true,
	"readme.md":                true,
	"contributing.md":          true,
	"changelog.md":             true,
	"copilot-instructions.md":  true,
	".cursorrules":             true,
	".windsurfrules":           true,
	".clinerules":              true,
	"gemini.md":                true,
	"agents.local.md":          true,
	"llms.txt":                 true,
	"security.md":              true,
	"code_of_conduct.md":       true,
	"pull_request_template.md": true,
}

var markdownInstructionFileNames = map[string]bool{
	"agents.md":               true,
	"claude.md":               true,
	"copilot-instructions.md": true,
	".cursorrules":            true,
	".windsurfrules":          true,
	".clinerules":             true,
	"gemini.md":               true,
	"agents.local.md":         true,
}

// IsContextFile reports whether path is a recognized high-value context or
// guidance document for index candidate selection.
func IsContextFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if markdownContextFileNames[base] {
		return true
	}
	return isCursorRule(path)
}

// IsAgentInstructionFile reports whether a document contains instructions that
// should guide code in its directory scope. General documentation, changelogs,
// and policy pages intentionally do not receive this role implicitly.
func IsAgentInstructionFile(path string) bool {
	return markdownInstructionFileNames[strings.ToLower(filepath.Base(path))] || isCursorRule(path)
}

func isCursorRule(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	dir := strings.ToLower(filepath.ToSlash(filepath.Dir(path)))
	if !strings.HasSuffix(dir, ".cursor/rules") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(base))
	return ext == ".md" || ext == ".markdown" || ext == ".mdc"
}

// MarkdownAdapter indexes Markdown structure, frontmatter, links, and content.
type MarkdownAdapter struct{}

func (a *MarkdownAdapter) Name() string { return "markdown" }
func (a *MarkdownAdapter) ExtractorVersion() string {
	return MarkdownParserVersion
}
func (a *MarkdownAdapter) DetectionFiles() []string {
	return []string{"AGENTS.md", "CLAUDE.md", "README.md", ".cursorrules", ".github/copilot-instructions.md"}
}
func (a *MarkdownAdapter) FileExtensions() []string { return []string{".md", ".markdown"} }
func (a *MarkdownAdapter) IgnoreDirs() []string {
	return []string{"node_modules", "vendor", ".comanda"}
}
func (a *MarkdownAdapter) IgnoreGlobs() []string { return nil }
func (a *MarkdownAdapter) EntrypointPatterns() []string {
	return []string{"README.md", "AGENTS.md"}
}

func (a *MarkdownAdapter) ConfigPatterns() []string {
	return []string{
		"AGENTS.md", "CLAUDE.md", "README.md", "CONTRIBUTING.md", "CHANGELOG.md",
		"copilot-instructions.md", ".cursorrules", ".windsurfrules", ".clinerules",
	}
}

func (a *MarkdownAdapter) ScoreFile(path string, depth int, isEntrypoint, isConfig bool) int {
	if IsContextFile(path) {
		score := 100
		if depth <= 1 {
			score += 40
		}
		return score
	}
	if depth <= 2 {
		return 10
	}
	return 0
}

func (a *MarkdownAdapter) ExtractSymbols(path string, content []byte) (*SymbolInfo, error) {
	return extractMarkdownDocument(path, content)
}

func extractMarkdownSymbols(path string, content []byte) (*SymbolInfo, error) {
	return extractMarkdownDocument(path, content)
}
