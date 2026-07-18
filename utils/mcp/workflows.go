// Package mcp implements comanda's Model Context Protocol (MCP) server mode,
// exposing workflows as MCP tools and skills as MCP prompts.
package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// WorkflowDef describes a discovered workflow file exposed as an MCP tool.
type WorkflowDef struct {
	Name        string   // MCP tool name (sanitized workflow file base)
	Path        string   // Path to the workflow YAML file
	Description string   // Tool description (first comment line or fallback)
	Vars        []string // {{ var }} placeholders discovered in the file
}

// toolNamePattern is the MCP-safe tool name pattern: ^[a-zA-Z0-9_-]{1,64}$.
var toolNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// invalidToolNameChars matches characters that are replaced with underscores
// when deriving tool names (e.g. "code-review" -> "code_review").
var invalidToolNameChars = regexp.MustCompile(`[^a-zA-Z0-9_]`)

// placeholderPattern matches {{ var }} placeholders, mirroring the CLI variable
// substitution pattern in utils/processor/dsl.go (SubstituteCLIVariables).
var placeholderPattern = regexp.MustCompile(`\{\{\s*([^}]+?)\s*\}\}`)

// reservedVars are placeholder names populated by the processor itself; they
// are never exposed as tool arguments.
var reservedVars = map[string]bool{
	"current_chunk": true,
	"chunk_index":   true,
	"total_chunks":  true,
	"file_index":    true,
	"total_files":   true,
}

// DefaultWorkflowDirs returns the default workflow directories that exist:
// ~/.comanda/workflows/ (user level) and .comanda/workflows/ (project level).
func DefaultWorkflowDirs() []string {
	var dirs []string
	if home, err := os.UserHomeDir(); err == nil {
		userDir := filepath.Join(home, ".comanda", "workflows")
		if info, err := os.Stat(userDir); err == nil && info.IsDir() {
			dirs = append(dirs, userDir)
		}
	}
	projectDir := filepath.Join(".comanda", "workflows")
	if info, err := os.Stat(projectDir); err == nil && info.IsDir() {
		dirs = append(dirs, projectDir)
	}
	return dirs
}

// DiscoverWorkflows scans dirs for *.yaml/*.yml workflow files and combines
// them with the explicit files, building one WorkflowDef per file. It returns
// an error if two files map to the same tool name, if a file cannot be read,
// or if a file base cannot be sanitized into a valid tool name.
func DiscoverWorkflows(dirs, files []string) ([]WorkflowDef, error) {
	var paths []string

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("reading workflow directory %s: %w", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !isWorkflowFile(entry.Name()) {
				continue
			}
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	// Sort directory scan results for deterministic ordering.
	sort.Strings(paths)

	for _, file := range files {
		if !isWorkflowFile(filepath.Base(file)) {
			return nil, fmt.Errorf("workflow file %s must have a .yaml or .yml extension", file)
		}
		if _, err := os.Stat(file); err != nil {
			return nil, fmt.Errorf("workflow file %s: %w", file, err)
		}
		paths = append(paths, file)
	}

	var defs []WorkflowDef
	seen := make(map[string]string) // tool name -> first path
	for _, path := range paths {
		def, err := workflowDefFromFile(path)
		if err != nil {
			return nil, err
		}
		if firstPath, dup := seen[def.Name]; dup {
			return nil, fmt.Errorf("duplicate MCP tool name %q: %s and %s; rename one of the files", def.Name, firstPath, path)
		}
		seen[def.Name] = path
		defs = append(defs, def)
	}
	return defs, nil
}

// isWorkflowFile reports whether name has a .yaml or .yml extension.
func isWorkflowFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".yaml" || ext == ".yml"
}

// workflowDefFromFile builds a WorkflowDef from a workflow YAML file.
func workflowDefFromFile(path string) (WorkflowDef, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return WorkflowDef{}, fmt.Errorf("reading workflow file %s: %w", path, err)
	}

	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	name := sanitizeToolName(base)
	if name == "" {
		return WorkflowDef{}, fmt.Errorf("workflow file %s: cannot derive an MCP-safe tool name from %q", path, base)
	}

	return WorkflowDef{
		Name:        name,
		Path:        path,
		Description: extractDescription(string(content), base),
		Vars:        extractVars(string(content)),
	}, nil
}

// sanitizeToolName converts a workflow file base into an MCP-safe tool name
// (e.g. "code-review" -> "code_review"), replacing unsupported characters with
// underscores and truncating to 64 characters.
func sanitizeToolName(base string) string {
	name := invalidToolNameChars.ReplaceAllString(base, "_")
	name = strings.Trim(name, "_")
	if len(name) > 64 {
		name = strings.Trim(name[:64], "_")
	}
	if !toolNamePattern.MatchString(name) {
		return ""
	}
	return name
}

// extractVars returns the sorted, deduplicated {{ var }} placeholder names in
// content, excluding processor-reserved names (loop.*, current_chunk, ...).
func extractVars(content string) []string {
	seen := make(map[string]bool)
	var vars []string
	for _, match := range placeholderPattern.FindAllStringSubmatch(content, -1) {
		name := strings.TrimSpace(match[1])
		if name == "" || seen[name] || isReservedVar(name) {
			continue
		}
		seen[name] = true
		vars = append(vars, name)
	}
	sort.Strings(vars)
	return vars
}

// isReservedVar reports whether a placeholder name is populated by the
// processor and therefore not exposed as a tool argument.
func isReservedVar(name string) bool {
	if reservedVars[name] {
		return true
	}
	return strings.HasPrefix(name, "loop.")
}

// extractDescription returns the first '#' comment line in the workflow file,
// or a fallback description naming the workflow.
func extractDescription(content, base string) string {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			desc := strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
			if desc != "" {
				return desc
			}
		}
	}
	return fmt.Sprintf("Run comanda workflow '%s'", base)
}
