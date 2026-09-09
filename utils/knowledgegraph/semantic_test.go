package knowledgegraph

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/codebaseindex"
	"github.com/kris-hansen/comanda/utils/semanticmemory"
)

func semanticFixture() *codebaseindex.ScanResult {
	return &codebaseindex.ScanResult{Candidates: []*codebaseindex.FileEntry{
		{Path: "app/main.cbl", Language: "demo", Symbols: &codebaseindex.SymbolInfo{
			Package: "MAIN", Imports: []string{"COPY"}, Functions: []codebaseindex.FunctionInfo{{Name: "CALCULATE"}},
			SemanticGraph: &codebaseindex.SemanticGraph{
				Entities: []codebaseindex.SemanticEntity{{ID: "interest", Kind: "concept", Name: "Interest", Scope: "project"}},
				Relations: []codebaseindex.SemanticRelation{
					{Source: "$function:CALCULATE", Target: "$name:BALANCE", Kind: "reads", Confidence: "extracted", Evidence: "main.cbl:12: COMPUTE INTEREST = BALANCE * RATE"},
					{Source: "$function:CALCULATE", Target: "interest", Kind: "relates_to", Confidence: "inferred", Evidence: "main.cbl:12: configured alias INTEREST"},
					{Source: "$function:CALCULATE", Target: "$callable:WRITE-RESULT", Kind: "performs", Confidence: "extracted", Evidence: "main.cbl:13: PERFORM WRITE-RESULT"},
				},
			},
		}},
		{Path: "cpy/copy.cpy", Language: "demo", Symbols: &codebaseindex.SymbolInfo{Package: "COPY", Imports: []string{"FIELDS"}}},
		{Path: "cpy/fields.cpy", Language: "demo", Symbols: &codebaseindex.SymbolInfo{Package: "FIELDS", Imports: []string{"COPY"}, Functions: []codebaseindex.FunctionInfo{{Name: "WRITE-RESULT"}}, SemanticGraph: &codebaseindex.SemanticGraph{Entities: []codebaseindex.SemanticEntity{
			{ID: "balance", Kind: "data_item", Name: "BALANCE", Referenceable: true},
			{ID: "interest", Kind: "concept", Name: "Interest", Scope: "project"},
		}}}},
		{Path: "unrelated/other.cpy", Language: "demo", Symbols: &codebaseindex.SymbolInfo{Package: "OTHER", Types: []codebaseindex.TypeInfo{{Name: "BALANCE"}}}},
	}}
}

func TestSemanticGraphScopedResolutionAndStorage(t *testing.T) {
	scan := semanticFixture()
	g := Build(scan, "demo")
	caller := NodeID("demo", "func:CALCULATE@app/main.cbl")
	target := NodeID("demo", semanticLocalID(scan.Candidates[2], scan.Candidates[2].Symbols.SemanticGraph.Entities[0]))
	edge := g.Edges[EdgeID("demo", caller, target, "reads")]
	if edge == nil || edge.Confidence != ConfidenceInferred || !strings.Contains(edge.Evidence, "main.cbl:12") {
		t.Fatalf("missing scoped relation: %#v", edge)
	}
	callable := NodeID("demo", "func:WRITE-RESULT@cpy/fields.cpy")
	if g.Edges[EdgeID("demo", caller, callable, "performs")] == nil {
		t.Fatal("missing transitive callable resolution")
	}
	concepts := 0
	for _, node := range g.Nodes {
		if node.Kind == "concept" {
			concepts++
		}
	}
	if concepts != 1 {
		t.Fatalf("shared concept count %d", concepts)
	}
	for _, e := range g.Edges {
		if g.Nodes[e.SourceID] == nil || g.Nodes[e.TargetID] == nil {
			t.Fatal("dangling endpoint")
		}
	}
	store, err := semanticmemory.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := RebuildWithProgress(context.Background(), store, g, nil); err != nil {
		t.Fatal(err)
	}
	edges, err := store.GraphEdges(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range edges {
		if e.Kind == "performs" {
			found = true
		}
	}
	if !found {
		t.Fatal("custom relations lost in SQLite")
	}
}

func TestSemanticGraphAmbiguousAndLocalNames(t *testing.T) {
	scan := semanticFixture()
	scan.Candidates[0].Symbols.Imports = append(scan.Candidates[0].Symbols.Imports, "OTHER")
	g := Build(scan, "demo")
	ambiguous := false
	for _, n := range g.Nodes {
		if n.Kind == "unresolved_reference" && n.Name == "BALANCE" && strings.Contains(n.Summary, "Ambiguous") {
			ambiguous = true
		}
	}
	if !ambiguous {
		t.Fatal("ambiguous import should remain unresolved")
	}
	scan.Candidates[0].Symbols.Types = []codebaseindex.TypeInfo{{Name: "BALANCE"}}
	g = Build(scan, "demo")
	edge := g.Edges[EdgeID("demo", NodeID("demo", "func:CALCULATE@app/main.cbl"), NodeID("demo", "type:BALANCE@app/main.cbl"), "reads")]
	if edge == nil {
		t.Fatal("local declaration should shadow imports")
	}
	// An unrelated unique type is not enough for a semantic name reference.
	scan.Candidates[0].Symbols.Types = nil
	scan.Candidates[0].Symbols.Imports = nil
	g = Build(scan, "demo")
	unresolved := false
	for _, n := range g.Nodes {
		if n.Kind == "unresolved_reference" && n.Name == "BALANCE" {
			unresolved = true
		}
	}
	if !unresolved {
		t.Fatal("resolved a name outside import scope")
	}
}

func TestSemanticNameScopesExcludeCalledModules(t *testing.T) {
	scan := semanticFixture()
	scan.Candidates[0].Symbols.Imports = append(scan.Candidates[0].Symbols.Imports, "OTHER")
	scan.Candidates[0].Symbols.SemanticGraph.ScopeImports = []string{"COPY"}
	g := Build(scan, "demo")
	target := NodeID("demo", semanticLocalID(scan.Candidates[2], scan.Candidates[2].Symbols.SemanticGraph.Entities[0]))
	caller := NodeID("demo", "func:CALCULATE@app/main.cbl")
	if g.Edges[EdgeID("demo", caller, target, "reads")] == nil {
		t.Fatal("called module contaminated COPY name scope")
	}
	scan.Candidates[0].Symbols.SemanticGraph.ScopeImports = []string{}
	// Empty and nil scope lists must stay distinct through index metadata.
	data, err := json.Marshal(scan.Candidates[0].Symbols)
	if err != nil {
		t.Fatal(err)
	}
	var symbols codebaseindex.SymbolInfo
	if err := json.Unmarshal(data, &symbols); err != nil {
		t.Fatal(err)
	}
	if symbols.SemanticGraph.ScopeImports == nil {
		t.Fatal("empty scope restriction lost in metadata")
	}
	g = Build(scan, "demo")
	if g.Edges[EdgeID("demo", caller, target, "reads")] != nil {
		t.Fatal("empty scope list should prevent imported name binding")
	}
}
