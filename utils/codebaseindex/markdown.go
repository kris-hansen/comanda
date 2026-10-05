package codebaseindex

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// markdownContextFileNames are well-known agent-context and guidance document
// basenames, matched case-insensitively. Repos accumulate these to steer coding
// agents, so the indexer treats them as high-value context.
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

// IsContextFile reports whether path is a recognized agent-context or guidance
// document: a well-known basename (AGENTS.md, CLAUDE.md, README.md,
// .cursorrules, copilot-instructions.md, ...) or a Cursor rules file under
// .cursor/rules/. The graph builder reuses this to classify documentation
// nodes without duplicating the recognized-name list.
func IsContextFile(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	if markdownContextFileNames[base] {
		return true
	}
	dir := strings.ToLower(filepath.ToSlash(filepath.Dir(path)))
	if strings.HasSuffix(dir, ".cursor/rules") {
		ext := strings.ToLower(filepath.Ext(base))
		return ext == ".md" || ext == ".markdown" || ext == ".mdc"
	}
	return false
}

// MarkdownAdapter indexes markdown documentation: recognized agent-context
// files (AGENTS.md, CLAUDE.md, .cursorrules, copilot instructions) and ordinary
// documentation trees. Headings become symbol types and links to other local
// markdown files become imports so downstream consumers can build doc-to-doc
// edges.
type MarkdownAdapter struct{}

func (a *MarkdownAdapter) Name() string { return "markdown" }
func (a *MarkdownAdapter) DetectionFiles() []string {
	return []string{"AGENTS.md", "CLAUDE.md", "README.md", ".cursorrules", ".github/copilot-instructions.md"}
}
func (a *MarkdownAdapter) FileExtensions() []string { return []string{".md", ".markdown"} }
func (a *MarkdownAdapter) IgnoreDirs() []string {
	// .comanda holds the tool's own generated index/memory output; re-ingesting
	// it would feed the index back into itself.
	return []string{"node_modules", "vendor", ".comanda"}
}
func (a *MarkdownAdapter) IgnoreGlobs() []string { return nil }
func (a *MarkdownAdapter) EntrypointPatterns() []string {
	return []string{"README.md", "AGENTS.md"}
}

// ConfigPatterns lists basenames (matching is basename-only, see scan.go) that
// mark a file as repository configuration/guidance. This also pulls extensionless
// files like .cursorrules into the scan, since they never match FileExtensions.
func (a *MarkdownAdapter) ConfigPatterns() []string {
	return []string{
		"AGENTS.md", "CLAUDE.md", "README.md", "CONTRIBUTING.md", "CHANGELOG.md",
		"copilot-instructions.md", ".cursorrules", ".windsurfrules", ".clinerules",
	}
}

// ScoreFile keeps recognized context files ahead of ordinary documentation so
// they survive candidate capping, while plain docs score only modestly.
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
	return extractMarkdownSymbols(path, content)
}

var (
	mdHeadingRe    = regexp.MustCompile(`^(#{1,6})[ \t]+(.+?)[ \t#]*$`)
	mdInlineLinkRe = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)(?:[ \t]+"[^"]*")?\)`)
	mdRefDefRe     = regexp.MustCompile(`(?m)^[ \t]{0,3}\[[^\]]+\]:[ \t]*(\S+)`)
	mdWikiLinkRe   = regexp.MustCompile(`\[\[([^\]|]+)(?:\|[^\]]*)?\]\]`)
)

// extractMarkdownSymbols never returns an error: heading and link extraction is
// best-effort, and files without recognizable structure produce an empty
// SymbolInfo.
func extractMarkdownSymbols(_ string, content []byte) (*SymbolInfo, error) {
	info := &SymbolInfo{}

	lines := strings.Split(string(content), "\n")
	prose := make([]string, 0, len(lines))
	inFence := false
	fenceMarker := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			marker := trimmed[:3]
			switch {
			case !inFence:
				inFence = true
				fenceMarker = marker
			case marker == fenceMarker:
				inFence = false
			}
			continue
		}
		if inFence {
			continue
		}
		if match := mdHeadingRe.FindStringSubmatch(line); match != nil {
			level := len(match[1])
			text := strings.TrimSpace(match[2])
			info.Types = append(info.Types, TypeInfo{
				Name:       text,
				Kind:       "h" + strconv.Itoa(level),
				IsExported: true,
			})
			if info.Package == "" && level == 1 {
				info.Package = text
			}
		}
		prose = append(prose, line)
	}

	// Heading-less documents still get a title: the first non-empty line, so
	// downstream summaries never need to re-read the file from disk.
	if info.Package == "" {
		for _, line := range prose {
			line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "#"))
			if line == "" {
				continue
			}
			const maxTitleLen = 120
			if len(line) > maxTitleLen {
				line = line[:maxTitleLen-3] + "..."
			}
			info.Package = line
			break
		}
	}

	body := strings.Join(prose, "\n")
	for _, match := range mdInlineLinkRe.FindAllStringSubmatch(body, -1) {
		if target := localMarkdownTarget(match[1]); target != "" {
			info.Imports = appendUnique(info.Imports, target)
		}
	}
	for _, match := range mdRefDefRe.FindAllStringSubmatch(body, -1) {
		if target := localMarkdownTarget(match[1]); target != "" {
			info.Imports = appendUnique(info.Imports, target)
		}
	}
	for _, match := range mdWikiLinkRe.FindAllStringSubmatch(body, -1) {
		if target := strings.TrimSpace(match[1]); target != "" {
			info.Imports = appendUnique(info.Imports, target)
		}
	}

	return info, nil
}

// localMarkdownTarget returns the link target when it points at another local
// markdown file, stripped of any #anchor fragment. External URLs, mailto links,
// anchors, and non-markdown targets return "". The raw (relative) path is kept;
// resolving it against the repository happens downstream.
func localMarkdownTarget(target string) string {
	target = strings.TrimSpace(target)
	if target == "" || strings.HasPrefix(target, "#") {
		return ""
	}
	lower := strings.ToLower(target)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "mailto:") || strings.HasPrefix(lower, "ftp://") {
		return ""
	}
	if i := strings.Index(target, "#"); i >= 0 {
		target = target[:i]
	}
	lower = strings.ToLower(target)
	if strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown") {
		return target
	}
	return ""
}
