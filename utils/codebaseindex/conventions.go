package codebaseindex

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// CodeConvention captures the deeper "why" layer of a codebase index: the
// conventions, architectural patterns, and change guidance agents should follow.
type CodeConvention struct {
	Language   string
	Name       string
	Rationale  string
	Guidance   string
	Evidence   []string
	Confidence float64
}

// writeCodeConventions writes language-specific conventions and agent guidance.
// This section intentionally goes one level deeper than layout/symbol summaries:
// it explains why code is shaped the way it is and how an agent should extend it.
func (m *Manager) writeCodeConventions(sb *strings.Builder, scan *ScanResult, maxItems int) {
	conventions := m.detectCodeConventions(scan)
	if len(conventions) == 0 {
		return
	}

	if maxItems > 0 && len(conventions) > maxItems {
		conventions = conventions[:maxItems]
	}

	sb.WriteString("## Code Conventions & Patterns\n\n")
	sb.WriteString("The current index answers *what exists*; this section captures *why the code is organized this way* and how agents should make changes safely.\n\n")

	for _, c := range conventions {
		sb.WriteString("### ")
		if c.Language != "" {
			sb.WriteString(c.Language)
			sb.WriteString(" — ")
		}
		sb.WriteString(c.Name)
		sb.WriteString("\n\n")

		if c.Rationale != "" {
			sb.WriteString("**Why:** ")
			sb.WriteString(c.Rationale)
			sb.WriteString("\n\n")
		}
		if c.Guidance != "" {
			sb.WriteString("**Agent guidance:** ")
			sb.WriteString(c.Guidance)
			sb.WriteString("\n\n")
		}
		if len(c.Evidence) > 0 {
			sb.WriteString("**Evidence:** ")
			items := make([]string, 0, len(c.Evidence))
			for _, e := range c.Evidence {
				items = append(items, "`"+e+"`")
			}
			sb.WriteString(strings.Join(items, ", "))
			sb.WriteString(fmt.Sprintf("  \n**Confidence:** %.0f%%\n\n", c.Confidence*100))
		}
	}
}

func (m *Manager) detectCodeConventions(scan *ScanResult) []CodeConvention {
	var conventions []CodeConvention

	languages := make(map[string]bool)
	for _, adapter := range m.adapters {
		languages[strings.ToLower(adapter.Name())] = true
	}

	if languages["go"] {
		conventions = append(conventions, m.detectGoConventions(scan)...)
	}

	sort.SliceStable(conventions, func(i, j int) bool {
		if conventions[i].Confidence == conventions[j].Confidence {
			return conventions[i].Name < conventions[j].Name
		}
		return conventions[i].Confidence > conventions[j].Confidence
	})

	return conventions
}

func (m *Manager) detectGoConventions(scan *ScanResult) []CodeConvention {
	var conventions []CodeConvention

	hasCmd := hasTopLevelDir(scan, "cmd")
	hasInternal := hasTopLevelDir(scan, "internal")
	hasUtils := hasTopLevelDir(scan, "utils")
	hasGoMod := hasFile(scan, "go.mod")

	if hasCmd && (hasInternal || hasUtils) {
		evidence := existingFiles(scan, "cmd", "internal", "utils", "go.mod")
		conventions = append(conventions, CodeConvention{
			Language:   "Go",
			Name:       "Separated entrypoints and reusable packages",
			Rationale:  "Command entrypoints are kept apart from implementation packages so CLI wiring stays thin and behavior can be tested or reused independently.",
			Guidance:   "Add user-facing commands under `cmd/`; put reusable behavior under `internal/`, `pkg/`, or `utils/` rather than embedding business logic directly in command files.",
			Evidence:   evidence,
			Confidence: 0.9,
		})
	}

	if hasFramework(scan, "cobra") || hasPathPrefix(scan, "cmd/") {
		conventions = append(conventions, CodeConvention{
			Language:   "Go",
			Name:       "Cobra-style CLI command wiring",
			Rationale:  "CLI behavior is modeled as commands with flags and Run/RunE handlers, which keeps the public interface discoverable and consistent.",
			Guidance:   "When adding a CLI feature, define flags near the Cobra command, keep validation close to RunE, and delegate substantial work to a package outside `cmd/`.",
			Evidence:   sampleEvidence(scan, []string{"cmd/", "github.com/spf13/cobra"}, 4),
			Confidence: confidence(hasFramework(scan, "cobra"), 0.9, 0.65),
		})
	}

	if hasPathSuffix(scan, "manager.go") || hasPathSuffix(scan, "scan.go") || hasPathSuffix(scan, "extract.go") || hasPathSuffix(scan, "synthesize.go") {
		conventions = append(conventions, CodeConvention{
			Language:   "Go",
			Name:       "Pipeline-oriented package design",
			Rationale:  "Complex features are split into orchestration, scanning/loading, extraction, synthesis/output, and storage concerns. This makes agent changes safer because each file has a narrow role.",
			Guidance:   "Before editing a pipeline feature, identify the phase you are changing: manager/orchestration, scan/input collection, extract/analysis, synthesize/output, or store/metadata. Avoid mixing phases in one patch.",
			Evidence:   sampleEvidence(scan, []string{"manager.go", "scan.go", "extract.go", "synthesize.go", "store.go"}, 5),
			Confidence: 0.85,
		})
	}

	if hasFile(scan, "utils/processor/types.go") || hasPathPrefix(scan, "utils/processor/") {
		conventions = append(conventions, CodeConvention{
			Language:   "Go",
			Name:       "Typed workflow configuration structs",
			Rationale:  "Workflow YAML is normalized into typed Go structs, so behavior changes usually require schema/types, validation, and execution logic to evolve together.",
			Guidance:   "For new workflow fields, update the config struct, builder/parser path, validation, execution handler, and docs/examples in the same change.",
			Evidence:   sampleEvidence(scan, []string{"utils/processor/types.go", "utils/processor/dsl_validator.go", "utils/processor/codebase_index_handler.go"}, 4),
			Confidence: 0.8,
		})
	}

	if hasGoTests(scan) {
		conventions = append(conventions, CodeConvention{
			Language:   "Go",
			Name:       "Package-local tests",
			Rationale:  "Tests live close to the implementation they protect, which helps agents find the right safety net before changing behavior.",
			Guidance:   "Add or update `*_test.go` files beside the package being changed; prefer focused table-driven tests for parser, synthesis, and handler behavior.",
			Evidence:   sampleEvidence(scan, []string{"_test.go"}, 5),
			Confidence: 0.75,
		})
	}

	if hasGoMod {
		conventions = append(conventions, CodeConvention{
			Language:   "Go",
			Name:       "Standard Go quality gate",
			Rationale:  "Go projects rely on formatter and static checks for consistent low-friction contributions.",
			Guidance:   "Before handing off a code change, run `gofmt -w .`, `go vet ./...`, and the relevant `go test ./...` package set.",
			Evidence:   existingFiles(scan, "go.mod"),
			Confidence: 0.7,
		})
	}

	return conventions
}

func hasTopLevelDir(scan *ScanResult, name string) bool {
	if scan == nil || scan.DirTree == nil {
		return false
	}
	for _, child := range scan.DirTree.Children {
		if child.Name == name {
			return true
		}
	}
	return false
}

func hasFile(scan *ScanResult, path string) bool {
	for _, f := range allScanFiles(scan) {
		if f.Path == path {
			return true
		}
	}
	return false
}

func hasPathPrefix(scan *ScanResult, prefix string) bool {
	for _, f := range allScanFiles(scan) {
		if strings.HasPrefix(f.Path, prefix) {
			return true
		}
	}
	return false
}

func hasPathSuffix(scan *ScanResult, suffix string) bool {
	for _, f := range allScanFiles(scan) {
		if strings.HasSuffix(f.Path, suffix) {
			return true
		}
	}
	return false
}

func hasGoTests(scan *ScanResult) bool {
	return hasPathSuffix(scan, "_test.go")
}

func hasFramework(scan *ScanResult, framework string) bool {
	for _, f := range allScanFiles(scan) {
		if f.Symbols == nil {
			continue
		}
		for _, fw := range f.Symbols.Frameworks {
			if strings.EqualFold(fw, framework) {
				return true
			}
		}
		for _, imp := range f.Symbols.Imports {
			if strings.Contains(strings.ToLower(imp), strings.ToLower(framework)) {
				return true
			}
		}
	}
	return false
}

func existingFiles(scan *ScanResult, paths ...string) []string {
	var evidence []string
	for _, path := range paths {
		if hasTopLevelDir(scan, path) {
			evidence = append(evidence, path+"/")
			continue
		}
		if hasFile(scan, path) {
			evidence = append(evidence, path)
		}
	}
	return evidence
}

func sampleEvidence(scan *ScanResult, needles []string, limit int) []string {
	seen := make(map[string]bool)
	var evidence []string
	for _, f := range allScanFiles(scan) {
		for _, needle := range needles {
			matched := strings.Contains(f.Path, needle)
			if !matched && f.Symbols != nil {
				for _, imp := range f.Symbols.Imports {
					if strings.Contains(imp, needle) {
						matched = true
						break
					}
				}
			}
			if matched && !seen[f.Path] {
				seen[f.Path] = true
				evidence = append(evidence, filepath.ToSlash(f.Path))
				if len(evidence) >= limit {
					return evidence
				}
			}
		}
	}
	return evidence
}

func allScanFiles(scan *ScanResult) []*FileEntry {
	if scan == nil {
		return nil
	}
	if len(scan.Files) > 0 {
		return scan.Files
	}
	return scan.Candidates
}

func confidence(condition bool, whenTrue, whenFalse float64) float64 {
	if condition {
		return whenTrue
	}
	return whenFalse
}
