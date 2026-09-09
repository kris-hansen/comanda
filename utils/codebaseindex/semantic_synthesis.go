package codebaseindex

import (
	"fmt"
	"sort"
	"strings"
)

// writeSemanticOverview makes the parser's ontology discoverable from the index
// while the complete entities, relationships and evidence stay in graph metadata.
func (m *Manager) writeSemanticOverview(sb *strings.Builder, scan *ScanResult) {
	kinds := make(map[string]int)
	concepts := make(map[string]bool)
	var examples []string
	files := scan.GraphFiles
	if files == nil {
		files = scan.Candidates
	}
	for _, f := range files {
		if f.Symbols == nil || f.Symbols.SemanticGraph == nil {
			continue
		}
		for _, e := range f.Symbols.SemanticGraph.Entities {
			if e.Kind == "concept" {
				concepts[e.Name] = true
			}
		}
		for _, r := range f.Symbols.SemanticGraph.Relations {
			kinds[r.Kind]++
			if r.Kind == "calculates" || r.Kind == "has_placeholder" {
				examples = append(examples, fmt.Sprintf("- `%s` → **%s** → `%s` (%s): %s\n", r.Source, r.Kind, r.Target, r.Confidence, strings.ReplaceAll(r.Evidence, "\n", "; ")))
			}
		}
	}
	if len(kinds) == 0 {
		return
	}
	sb.WriteString("## Parser Semantic Model\n\n")
	sb.WriteString("Typed relationships and source evidence supplied by the parser. Concept labels and name binding may be inferred; inspect graph confidence and evidence before treating them as established behavior. Counts below precede graph deduplication.\n\n")
	var names []string
	for name := range kinds {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		sb.WriteString(fmt.Sprintf("- **%s:** %d\n", name, kinds[name]))
	}
	if len(concepts) > 0 {
		names = nil
		for name := range concepts {
			names = append(names, name)
		}
		sort.Strings(names)
		sb.WriteString("\n**Concepts:** " + strings.Join(names, ", ") + "\n")
	}
	sort.Strings(examples)
	if len(examples) > 0 {
		sb.WriteString("\n### Example evidence\n\n")
		for _, example := range examples[:minInt(len(examples), 8)] {
			sb.WriteString(example)
		}
	}
	sb.WriteString("\n")
}
