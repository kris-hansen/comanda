package codebaseindex

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const (
	maxTreeDepth      = 3
	maxFilesPerDir    = 12
	maxImportantFiles = 25
	maxSymbolsPerFile = 8
)

// synthesize generates the markdown index from scan results
func (m *Manager) synthesize(scan *ScanResult) (string, error) {
	var sb strings.Builder

	// 1. Purpose (always include)
	m.writePurpose(&sb, scan)

	// 2. Repo layout (always include)
	m.writeRepoLayout(&sb, scan)

	// 3. Primary capabilities (if inferred)
	m.writeCapabilities(&sb, scan)

	// 4. Entry points (if detected)
	m.writeEntryPoints(&sb, scan)

	// 5. Key modules (if detected)
	m.writeKeyModules(&sb, scan)

	// 6. Important files (always include)
	m.writeImportantFiles(&sb, scan)

	// 6.5. Token budget warnings (for large files)
	m.writeTokenBudget(&sb, scan)

	// 7. Operational notes (if found)
	m.writeOperationalNotes(&sb, scan)

	// 8. Risk/caution areas (if detected)
	m.writeRiskAreas(&sb, scan)

	// 9. Navigation hints (if detected)
	m.writeNavigationHints(&sb, scan)

	// 10. Footer metadata (always include)
	m.writeFooter(&sb, scan)

	return sb.String(), nil
}

// writePurpose writes the purpose section
func (m *Manager) writePurpose(sb *strings.Builder, scan *ScanResult) {
	sb.WriteString("# ")
	sb.WriteString(m.config.RepoFileSlug)
	sb.WriteString(" - Codebase Index\n\n")

	// Detect languages
	languages := make([]string, 0, len(m.adapters))
	for _, a := range m.adapters {
		languages = append(languages, a.Name())
	}

	sb.WriteString("**Languages:** ")
	sb.WriteString(strings.Join(languages, ", "))
	sb.WriteString("\n")

	sb.WriteString("**Files:** ")
	sb.WriteString(fmt.Sprintf("%d total, %d indexed", scan.TotalFiles, len(scan.Candidates)))
	sb.WriteString("\n\n")

	// Try to infer purpose from common patterns
	purpose := m.inferPurpose(scan)
	if purpose != "" {
		sb.WriteString(purpose)
		sb.WriteString("\n\n")
	}
}

// inferPurpose attempts to infer the project purpose from structure
func (m *Manager) inferPurpose(scan *ScanResult) string {
	var hints []string

	// Check for common patterns
	frameworks := make(map[string]bool)
	for _, f := range scan.Candidates {
		if f.Symbols != nil {
			for _, fw := range f.Symbols.Frameworks {
				frameworks[fw] = true
			}
		}
	}

	if frameworks["gin"] || frameworks["echo"] || frameworks["express"] || frameworks["fastapi"] || frameworks["flask"] || frameworks["django"] {
		hints = append(hints, "web service/API")
	}
	if frameworks["react"] || frameworks["vue"] || frameworks["angular"] || frameworks["svelte"] {
		hints = append(hints, "frontend application")
	}
	if frameworks["flutter"] {
		hints = append(hints, "mobile application")
	}
	if frameworks["cobra"] || frameworks["cli"] {
		hints = append(hints, "CLI tool")
	}
	if frameworks["grpc"] {
		hints = append(hints, "gRPC service")
	}

	if len(hints) > 0 {
		return "This appears to be a " + strings.Join(hints, " / ") + "."
	}

	return ""
}

// writeRepoLayout writes the directory tree
func (m *Manager) writeRepoLayout(sb *strings.Builder, scan *ScanResult) {
	sb.WriteString("## Repository Layout\n\n")
	sb.WriteString("```\n")
	m.writeTreeNode(sb, scan.DirTree, "", 0)
	sb.WriteString("```\n\n")
}

// writeTreeNode recursively writes a directory node
func (m *Manager) writeTreeNode(sb *strings.Builder, node *DirNode, prefix string, depth int) {
	if depth > maxTreeDepth {
		return
	}

	sb.WriteString(prefix)
	sb.WriteString(node.Name)
	sb.WriteString("/\n")

	// Sort children for deterministic output
	sort.Slice(node.Children, func(i, j int) bool {
		return node.Children[i].Name < node.Children[j].Name
	})

	// Write files first (limited)
	sort.Strings(node.Files)
	fileCount := len(node.Files)
	if fileCount > maxFilesPerDir {
		for i := 0; i < maxFilesPerDir-1; i++ {
			sb.WriteString(prefix)
			sb.WriteString("  ")
			sb.WriteString(node.Files[i])
			sb.WriteString("\n")
		}
		sb.WriteString(prefix)
		sb.WriteString(fmt.Sprintf("  ... and %d more files\n", fileCount-maxFilesPerDir+1))
	} else {
		for _, f := range node.Files {
			sb.WriteString(prefix)
			sb.WriteString("  ")
			sb.WriteString(f)
			sb.WriteString("\n")
		}
	}

	// Write child directories
	for _, child := range node.Children {
		m.writeTreeNode(sb, child, prefix+"  ", depth+1)
	}
}

// writeCapabilities writes the capabilities section
func (m *Manager) writeCapabilities(sb *strings.Builder, scan *ScanResult) {
	capabilities := m.inferCapabilities(scan)
	if len(capabilities) == 0 {
		return
	}

	sb.WriteString("## Primary Capabilities\n\n")
	for dir, caps := range capabilities {
		sb.WriteString("- **")
		sb.WriteString(dir)
		sb.WriteString("**: ")
		sb.WriteString(strings.Join(caps, ", "))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
}

// inferCapabilities infers capabilities from directory structure
func (m *Manager) inferCapabilities(scan *ScanResult) map[string][]string {
	capabilities := make(map[string][]string)

	// Common directory patterns
	dirPatterns := map[string][]string{
		"cmd":         {"CLI commands", "entrypoints"},
		"api":         {"API endpoints", "handlers"},
		"handlers":    {"request handlers"},
		"routes":      {"routing", "endpoints"},
		"controllers": {"request handling"},
		"models":      {"data models"},
		"entities":    {"domain entities"},
		"services":    {"business logic"},
		"utils":       {"utilities", "helpers"},
		"lib":         {"shared libraries"},
		"pkg":         {"packages"},
		"internal":    {"internal packages"},
		"config":      {"configuration"},
		"db":          {"database"},
		"migrations":  {"database migrations"},
		"tests":       {"testing"},
		"test":        {"testing"},
		"docs":        {"documentation"},
		"scripts":     {"automation scripts"},
		"components":  {"UI components"},
		"views":       {"UI views"},
		"pages":       {"page components"},
		"store":       {"state management"},
		"hooks":       {"React hooks"},
		"middleware":  {"middleware"},
	}

	// Check each top-level directory
	for _, child := range scan.DirTree.Children {
		name := strings.ToLower(child.Name)
		if caps, ok := dirPatterns[name]; ok {
			capabilities[child.Name] = caps
		}
	}

	return capabilities
}

// writeEntryPoints writes the entry points section
func (m *Manager) writeEntryPoints(sb *strings.Builder, scan *ScanResult) {
	var entrypoints []*FileEntry
	for _, f := range scan.Candidates {
		if f.IsEntrypoint {
			entrypoints = append(entrypoints, f)
		}
	}

	if len(entrypoints) == 0 {
		return
	}

	sb.WriteString("## Entry Points\n\n")
	for _, f := range entrypoints {
		sb.WriteString("- `")
		sb.WriteString(f.Path)
		sb.WriteString("`")

		// Add description if we have symbols
		if f.Symbols != nil && f.Symbols.Package != "" {
			sb.WriteString(" (package: ")
			sb.WriteString(f.Symbols.Package)
			sb.WriteString(")")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
}

// writeKeyModules writes the key modules section
func (m *Manager) writeKeyModules(sb *strings.Builder, scan *ScanResult) {
	modules := m.detectModules(scan)
	if len(modules) == 0 {
		return
	}

	sb.WriteString("## Key Modules\n\n")
	for _, mod := range modules {
		sb.WriteString("### ")
		sb.WriteString(mod.Name)
		sb.WriteString("\n\n")

		if mod.Description != "" {
			sb.WriteString(mod.Description)
			sb.WriteString("\n\n")
		}

		if len(mod.Files) > 0 {
			sb.WriteString("Files:\n")
			for _, f := range mod.Files {
				sb.WriteString("- `")
				sb.WriteString(f)
				sb.WriteString("`\n")
			}
			sb.WriteString("\n")
		}
	}
}

type moduleInfo struct {
	Name        string
	Description string
	Files       []string
}

// detectModules detects logical modules from the codebase
func (m *Manager) detectModules(scan *ScanResult) []moduleInfo {
	var modules []moduleInfo

	// Group files by top-level directory
	dirFiles := make(map[string][]string)
	for _, f := range scan.Candidates {
		parts := strings.Split(f.Path, string(filepath.Separator))
		if len(parts) > 1 {
			dir := parts[0]
			dirFiles[dir] = append(dirFiles[dir], f.Path)
		}
	}

	// Create module info for directories with multiple files
	for dir, files := range dirFiles {
		if len(files) >= 2 {
			modules = append(modules, moduleInfo{
				Name:  dir,
				Files: files,
			})
		}
	}

	// Sort by name
	sort.Slice(modules, func(i, j int) bool {
		return modules[i].Name < modules[j].Name
	})

	// Limit to top modules
	if len(modules) > 10 {
		modules = modules[:10]
	}

	return modules
}

// writeImportantFiles writes the important files section
func (m *Manager) writeImportantFiles(sb *strings.Builder, scan *ScanResult) {
	sb.WriteString("## Important Files\n\n")

	// Candidates are already sorted by score
	count := len(scan.Candidates)
	if count > maxImportantFiles {
		count = maxImportantFiles
	}

	for i := 0; i < count; i++ {
		f := scan.Candidates[i]
		sb.WriteString("### `")
		sb.WriteString(f.Path)
		sb.WriteString("`\n\n")

		if f.Symbols != nil {
			// Package info
			if f.Symbols.Package != "" {
				sb.WriteString("**Package:** ")
				sb.WriteString(f.Symbols.Package)
				sb.WriteString("\n\n")
			}

			// Types (limited)
			if len(f.Symbols.Types) > 0 {
				sb.WriteString("**Types:** ")
				typeNames := make([]string, 0, maxSymbolsPerFile)
				for j, t := range f.Symbols.Types {
					if j >= maxSymbolsPerFile {
						typeNames = append(typeNames, fmt.Sprintf("... +%d more", len(f.Symbols.Types)-maxSymbolsPerFile))
						break
					}
					typeNames = append(typeNames, fmt.Sprintf("`%s` (%s)", t.Name, t.Kind))
				}
				sb.WriteString(strings.Join(typeNames, ", "))
				sb.WriteString("\n\n")
			}

			// Functions (limited)
			if len(f.Symbols.Functions) > 0 {
				sb.WriteString("**Functions:** ")
				funcNames := make([]string, 0, maxSymbolsPerFile)
				for j, fn := range f.Symbols.Functions {
					if j >= maxSymbolsPerFile {
						funcNames = append(funcNames, fmt.Sprintf("... +%d more", len(f.Symbols.Functions)-maxSymbolsPerFile))
						break
					}
					funcNames = append(funcNames, "`"+fn.Name+"`")
				}
				sb.WriteString(strings.Join(funcNames, ", "))
				sb.WriteString("\n\n")
			}

			// Frameworks
			if len(f.Symbols.Frameworks) > 0 {
				sb.WriteString("**Frameworks:** ")
				sb.WriteString(strings.Join(f.Symbols.Frameworks, ", "))
				sb.WriteString("\n\n")
			}
		}
	}
}

// writeOperationalNotes writes operational notes section
func (m *Manager) writeOperationalNotes(sb *strings.Builder, scan *ScanResult) {
	notes := m.detectOperationalFiles(scan)
	if len(notes) == 0 {
		return
	}

	sb.WriteString("## Operational Notes\n\n")
	for category, files := range notes {
		sb.WriteString("**")
		sb.WriteString(category)
		sb.WriteString(":** ")
		fileLinks := make([]string, len(files))
		for i, f := range files {
			fileLinks[i] = "`" + f + "`"
		}
		sb.WriteString(strings.Join(fileLinks, ", "))
		sb.WriteString("\n\n")
	}
}

// detectOperationalFiles finds build, test, and run configuration files
func (m *Manager) detectOperationalFiles(scan *ScanResult) map[string][]string {
	notes := make(map[string][]string)

	operationalPatterns := map[string][]string{
		"Build":   {"Makefile", "build.sh", "build.gradle", "pom.xml", "Cargo.toml"},
		"Test":    {"test.sh", "pytest.ini", "jest.config.js", "jest.config.ts"},
		"Docker":  {"Dockerfile", "docker-compose.yml", "docker-compose.yaml"},
		"CI/CD":   {".github/workflows", ".gitlab-ci.yml", "Jenkinsfile", ".circleci"},
		"Config":  {"config.yaml", "config.json", ".env.example"},
		"Package": {"go.mod", "package.json", "requirements.txt", "Gemfile", "pubspec.yaml"},
	}

	for _, f := range scan.Files {
		name := filepath.Base(f.Path)
		for category, patterns := range operationalPatterns {
			for _, pattern := range patterns {
				if matched, _ := filepath.Match(pattern, name); matched {
					notes[category] = appendUnique(notes[category], f.Path)
				}
				if strings.Contains(f.Path, pattern) {
					notes[category] = appendUnique(notes[category], f.Path)
				}
			}
		}
	}

	return notes
}

// writeRiskAreas writes risk areas section
func (m *Manager) writeRiskAreas(sb *strings.Builder, scan *ScanResult) {
	risks := m.collectRiskAreas(scan)
	if len(risks) == 0 {
		return
	}

	sb.WriteString("## Risk / Caution Areas\n\n")
	titleCaser := cases.Title(language.English)
	for tag, files := range risks {
		sb.WriteString("**")
		sb.WriteString(titleCaser.String(tag))
		sb.WriteString(":**\n")
		for _, f := range files {
			sb.WriteString("- `")
			sb.WriteString(f)
			sb.WriteString("`\n")
		}
		sb.WriteString("\n")
	}
}

// collectRiskAreas collects files with risk tags
func (m *Manager) collectRiskAreas(scan *ScanResult) map[string][]string {
	risks := make(map[string][]string)

	for _, f := range scan.Candidates {
		if f.Symbols != nil {
			for _, tag := range f.Symbols.RiskTags {
				risks[tag] = appendUnique(risks[tag], f.Path)
			}
		}
	}

	return risks
}

// writeNavigationHints writes navigation hints section
func (m *Manager) writeNavigationHints(sb *strings.Builder, scan *ScanResult) {
	hints := m.detectConventions(scan)
	if len(hints) == 0 {
		return
	}

	sb.WriteString("## Navigation Hints\n\n")
	for _, hint := range hints {
		sb.WriteString("- ")
		sb.WriteString(hint)
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
}

// detectConventions detects naming and structure conventions
func (m *Manager) detectConventions(scan *ScanResult) []string {
	var hints []string

	// Check for common Go conventions
	hasCmd := false
	hasInternal := false
	hasPkg := false
	for _, child := range scan.DirTree.Children {
		switch child.Name {
		case "cmd":
			hasCmd = true
		case "internal":
			hasInternal = true
		case "pkg":
			hasPkg = true
		}
	}

	if hasCmd && hasInternal {
		hints = append(hints, "Follows Go standard layout (cmd/, internal/)")
	}
	if hasPkg {
		hints = append(hints, "Has public packages in pkg/")
	}

	// Check for test patterns
	testCount := 0
	for _, f := range scan.Files {
		if strings.HasSuffix(f.Path, "_test.go") || strings.HasSuffix(f.Path, ".test.ts") || strings.HasSuffix(f.Path, ".test.js") || strings.HasSuffix(f.Path, "_test.py") {
			testCount++
		}
	}
	if testCount > 0 {
		hints = append(hints, fmt.Sprintf("Has %d test files", testCount))
	}

	return hints
}

// writeTokenBudget writes the token budget section for large files
func (m *Manager) writeTokenBudget(sb *strings.Builder, scan *ScanResult) {
	var largeFiles, oversizedFiles []*FileEntry

	// Categorize files by token budget
	for _, f := range scan.Candidates {
		switch f.TokenBudgetCategory() {
		case "large":
			largeFiles = append(largeFiles, f)
		case "oversized":
			oversizedFiles = append(oversizedFiles, f)
		}
	}

	// Only write section if there are files to warn about
	if len(largeFiles) == 0 && len(oversizedFiles) == 0 {
		return
	}

	sb.WriteString("## ⚠️ Token Budget\n\n")
	sb.WriteString("Some files exceed recommended token limits for single reads.\n\n")

	if len(oversizedFiles) > 0 {
		sb.WriteString("### ❌ Oversized (>25k tokens) — Use grep or chunked reads\n\n")
		for _, f := range oversizedFiles {
			sb.WriteString(fmt.Sprintf("- `%s` (~%dk tokens)\n", f.Path, f.EstimatedTokens/1000))
		}
		sb.WriteString("\n")
	}

	if len(largeFiles) > 0 {
		sb.WriteString("### ⚡ Large (10k-25k tokens) — Read with care\n\n")
		for _, f := range largeFiles {
			sb.WriteString(fmt.Sprintf("- `%s` (~%dk tokens)\n", f.Path, f.EstimatedTokens/1000))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("**Tip:** For oversized files, use `grep` to find relevant sections, or read with `offset` and `limit` parameters.\n\n")
}

// writeFooter writes the footer metadata section
func (m *Manager) writeFooter(sb *strings.Builder, scan *ScanResult) {
	sb.WriteString("---\n\n")
	sb.WriteString("*Index generated at ")
	sb.WriteString(time.Now().UTC().Format(time.RFC3339))
	sb.WriteString("*\n\n")
	sb.WriteString("*Scan time: ")
	sb.WriteString(scan.ProcessedTime.String())
	sb.WriteString("*\n")
}
