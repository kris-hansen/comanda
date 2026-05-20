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
	conventions = append(conventions, m.detectEvidenceBackedConventions(scan)...)

	languages := make(map[string]bool)
	for _, adapter := range m.adapters {
		languages[strings.ToLower(adapter.Name())] = true
	}

	if languages["go"] {
		conventions = append(conventions, m.detectGoConventions(scan)...)
	}

	conventions = dedupeConventions(conventions)
	sort.SliceStable(conventions, func(i, j int) bool {
		if conventions[i].Confidence == conventions[j].Confidence {
			return conventions[i].Name < conventions[j].Name
		}
		return conventions[i].Confidence > conventions[j].Confidence
	})

	return conventions
}

// detectEvidenceBackedConventions mines repeated local file-role patterns from
// the scan result before language-specific fallbacks run. These conventions are
// intentionally evidence-first: they only appear when multiple files demonstrate
// the same editing pattern in this repository.
func (m *Manager) detectEvidenceBackedConventions(scan *ScanResult) []CodeConvention {
	files := allScanFiles(scan)
	if len(files) == 0 {
		return nil
	}

	var conventions []CodeConvention
	roleGroups := groupFilesByLocalRole(files)
	conventions = append(conventions, conventionFromRoleGroup("cmd/main.go", roleGroups["cmd/main.go"], CodeConvention{
		Name:      "Command entrypoints are isolated under cmd",
		Rationale: "Executable surfaces repeat under cmd/, which keeps user-facing command wiring separate from reusable implementation packages.",
		Guidance:  "When adding an executable or CLI surface, create or extend the matching cmd/.../main.go entrypoint and move reusable behavior into a package outside cmd/.",
	})...)
	conventions = append(conventions, conventionFromRoleGroup("db/store.go", roleGroups["db/store.go"], CodeConvention{
		Name:      "Storage logic is isolated behind db/store files",
		Rationale: "Multiple domains keep persistence behavior in local db/store.go files, which makes storage boundaries easy to find and keeps callers out of database details.",
		Guidance:  "For persistence changes, edit the domain's db/store.go boundary and keep SQL/storage details there rather than spreading them through service or handler code.",
	})...)
	conventions = append(conventions, conventionFromRoleGroup("manager.go", roleGroups["manager.go"], CodeConvention{
		Name:      "Managers own orchestration boundaries",
		Rationale: "Repeated manager.go files mark packages where coordination logic is separated from lower-level helpers.",
		Guidance:  "Put sequencing, lifecycle, and cross-step coordination in the package manager; keep parsing, extraction, persistence, and rendering work in narrower files.",
	})...)
	conventions = append(conventions, conventionFromRoleGroup("handler.go", roleGroups["handler.go"], CodeConvention{
		Name:      "Handlers form integration boundaries",
		Rationale: "Repeated handler.go files indicate that input/output or transport integration is kept at the edge of each feature area.",
		Guidance:  "When changing external behavior, adjust the relevant handler boundary, then delegate substantial validation or business logic to package-local helpers.",
	})...)
	conventions = append(conventions, conventionFromRoleGroup("validator.go", roleGroups["validator.go"], CodeConvention{
		Name:      "Validation is kept explicit and package-local",
		Rationale: "Repeated validator files show that invalid states are rejected close to the feature schema rather than being handled ad hoc at call sites.",
		Guidance:  "When adding fields or accepted values, update the validator beside the feature and add focused tests for both valid and invalid cases.",
	})...)
	conventions = append(conventions, conventionFromRoleGroup("*_test.go", roleGroups["*_test.go"], CodeConvention{
		Name:      "Behavior is protected by package-local tests",
		Rationale: "Test files recur beside implementation packages, so local behavior usually has a nearby safety net.",
		Guidance:  "Before editing a package, inspect its adjacent *_test.go files; update or add focused tests in the same package for changed parser, handler, synthesis, or storage behavior.",
	})...)

	if evidence := pipelineEvidence(roleGroups); len(evidence) >= 3 {
		conventions = append(conventions, CodeConvention{
			Language:   "Repository",
			Name:       "Pipeline phases are split into role-specific files",
			Rationale:  "The repository repeatedly names pipeline phases as separate files, which keeps orchestration, input collection, extraction, synthesis, and storage work independently editable.",
			Guidance:   "Identify the phase you are changing before patching: manager/orchestration, scan/input, extract/analysis, synthesize/output, handler/integration, validate/schema, or store/persistence. Avoid mixing phase changes unless the feature contract requires it.",
			Evidence:   evidence,
			Confidence: confidenceFromEvidence(len(evidence), 4),
		})
	}

	if evidence := schemaChangeEvidence(files); len(evidence) >= 4 {
		conventions = append(conventions, CodeConvention{
			Language:   "Repository",
			Name:       "Schema changes require coordinated parser, validator, executor, and docs updates",
			Rationale:  "Typed config, validation, execution handlers, docs, and examples all exist in the indexed evidence, so user-facing workflow fields usually cross several layers.",
			Guidance:   "When adding or changing a workflow/index field, update the type definition, parsing/builder path, validator, execution handler, docs or embedded guide, examples, and package-local tests together.",
			Evidence:   evidence,
			Confidence: confidenceFromEvidence(len(evidence), 5),
		})
	}

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

func groupFilesByLocalRole(files []*FileEntry) map[string][]string {
	groups := make(map[string][]string)
	for _, f := range files {
		if f == nil || f.Path == "" {
			continue
		}
		path := filepath.ToSlash(f.Path)
		base := filepath.Base(path)
		parts := strings.Split(path, "/")
		if strings.HasSuffix(base, "_test.go") {
			groups["*_test.go"] = append(groups["*_test.go"], path)
		}
		if base == "main.go" && containsPathPart(parts, "cmd") {
			groups["cmd/main.go"] = append(groups["cmd/main.go"], path)
		}
		if len(parts) >= 2 && parts[len(parts)-2] == "db" && base == "store.go" {
			groups["db/store.go"] = append(groups["db/store.go"], path)
		}
		for _, role := range []string{
			"manager.go",
			"scan.go",
			"extract.go",
			"synthesize.go",
			"store.go",
			"handler.go",
			"validator.go",
			"types.go",
			"config.go",
		} {
			if base == role {
				groups[role] = append(groups[role], path)
			}
		}
	}

	for role := range groups {
		sort.Strings(groups[role])
	}
	return groups
}

func containsPathPart(parts []string, want string) bool {
	for _, part := range parts {
		if part == want {
			return true
		}
	}
	return false
}

func conventionFromRoleGroup(role string, evidence []string, convention CodeConvention) []CodeConvention {
	if len(evidence) < minimumEvidenceForRole(role) {
		return nil
	}
	if convention.Language == "" {
		convention.Language = "Repository"
	}
	convention.Evidence = limitEvidence(evidence, 6)
	convention.Confidence = confidenceFromEvidence(len(evidence), minimumEvidenceForRole(role))
	return []CodeConvention{convention}
}

func minimumEvidenceForRole(role string) int {
	switch role {
	case "cmd/main.go", "db/store.go":
		return 2
	case "*_test.go":
		return 3
	default:
		return 2
	}
}

func pipelineEvidence(roleGroups map[string][]string) []string {
	var evidence []string
	for _, role := range []string{"manager.go", "scan.go", "extract.go", "synthesize.go", "handler.go", "validator.go", "store.go"} {
		if files := roleGroups[role]; len(files) > 0 {
			evidence = append(evidence, files[0])
		}
	}
	return limitEvidence(evidence, 7)
}

func schemaChangeEvidence(files []*FileEntry) []string {
	required := []string{"types.go", "validator.go", "handler.go"}
	for _, needle := range required {
		if firstEvidenceForNeedle(files, needle) == "" {
			return nil
		}
	}
	needles := append(required, "embedded_guide.go", "README.md", "examples/", "_test.go")
	var evidence []string
	for _, needle := range needles {
		if path := firstEvidenceForNeedle(files, needle); path != "" {
			evidence = append(evidence, path)
		}
	}
	return limitEvidence(evidence, 7)
}

func firstEvidenceForNeedle(files []*FileEntry, needle string) string {
	for _, f := range files {
		if f == nil {
			continue
		}
		path := filepath.ToSlash(f.Path)
		if strings.Contains(path, needle) {
			return path
		}
	}
	return ""
}

func confidenceFromEvidence(count, threshold int) float64 {
	if threshold <= 0 {
		threshold = 1
	}
	confidence := 0.55 + (float64(count-threshold+1) * 0.08)
	if confidence > 0.92 {
		return 0.92
	}
	if confidence < 0.6 {
		return 0.6
	}
	return confidence
}

func limitEvidence(evidence []string, limit int) []string {
	if limit <= 0 || len(evidence) <= limit {
		return evidence
	}
	return append([]string(nil), evidence[:limit]...)
}

func dedupeConventions(conventions []CodeConvention) []CodeConvention {
	seen := make(map[string]bool)
	var result []CodeConvention
	for _, c := range conventions {
		key := strings.ToLower(c.Language + ":" + c.Name)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, c)
	}
	return result
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
