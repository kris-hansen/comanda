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

func fixtureScan() *codebaseindex.ScanResult {
	return &codebaseindex.ScanResult{
		Candidates: []*codebaseindex.FileEntry{
			{
				Path:     "cmd/server.go",
				Language: "go",
				Symbols: &codebaseindex.SymbolInfo{
					Package: "cmd",
					Imports: []string{"github.com/acme/proj/utils/store", "fmt"},
					Functions: []codebaseindex.FunctionInfo{
						{Name: "main", Signature: "func main()"},
						{Name: "Run", Signature: "func Run(s *store.Store) error", IsExported: true},
					},
				},
			},
			{
				Path:     "utils/store/store.go",
				Language: "go",
				Symbols: &codebaseindex.SymbolInfo{
					Package: "store",
					Types: []codebaseindex.TypeInfo{
						{Name: "Store", Kind: "struct", IsExported: true, Fields: []string{"db *sql.DB"}},
					},
					Functions: []codebaseindex.FunctionInfo{
						{Name: "Open", Signature: "func Open(path string) (*Store, error)", IsExported: true},
					},
				},
			},
		},
		Components: []*codebaseindex.CodebaseComponent{
			{Name: "cli", Root: "cmd", Kind: "cli", FileCount: 1},
		},
	}
}

func TestBuildDeterministicEdges(t *testing.T) {
	g := Build(fixtureScan(), "proj")

	// File, package, type, function, component, and external package nodes.
	for _, local := range []string{
		"file:cmd/server.go", "file:utils/store/store.go",
		"pkg:cmd", "pkg:store", "pkg:fmt",
		"type:Store@utils/store/store.go",
		"func:Open@utils/store/store.go",
		"component:cli",
	} {
		if g.Nodes[NodeID("proj", local)] == nil {
			t.Errorf("missing node %s", local)
		}
	}

	// belongs_to, defines, contains, imports edges are extracted.
	expect := []struct {
		src, tgt, kind string
	}{
		{"file:cmd/server.go", "pkg:cmd", EdgeBelongsTo},
		{"file:utils/store/store.go", "type:Store@utils/store/store.go", EdgeDefines},
		{"component:cli", "file:cmd/server.go", EdgeContains},
		{"file:cmd/server.go", "pkg:store", EdgeImports}, // resolves to local package
		{"file:cmd/server.go", "pkg:fmt", EdgeImports},   // external package node
	}
	for _, e := range expect {
		id := EdgeID("proj", NodeID("proj", e.src), NodeID("proj", e.tgt), e.kind)
		edge := g.Edges[id]
		if edge == nil {
			t.Errorf("missing edge %s -%s-> %s", e.src, e.kind, e.tgt)
			continue
		}
		if edge.Confidence != ConfidenceExtracted {
			t.Errorf("edge %s -%s-> %s confidence = %s, want extracted", e.src, e.kind, e.tgt, edge.Confidence)
		}
	}

	// cmd/server.go references store.Store in a signature -> inferred uses.
	usesID := EdgeID("proj", NodeID("proj", "file:cmd/server.go"), NodeID("proj", "type:Store@utils/store/store.go"), EdgeUses)
	uses := g.Edges[usesID]
	if uses == nil {
		t.Fatal("missing inferred uses edge from server.go to Store")
	}
	if uses.Confidence != ConfidenceInferred {
		t.Fatalf("uses edge confidence = %s, want inferred", uses.Confidence)
	}
}

func TestEnhanceAddsConceptNodes(t *testing.T) {
	g := Build(fixtureScan(), "proj")
	fakeLLM := func(prompt string) (string, error) {
		return "```json\n" + `{
          "nodes": [{"name": "Persistence", "summary": "Storage layer concern"}],
          "edges": [
            {"source": "Persistence", "target": "Store", "kind": "references", "evidence": "Store persists data"},
            {"source": "main", "target": "Store", "kind": "deletes", "evidence": "bad kind"},
            {"source": "Nobody", "target": "Store", "kind": "uses", "evidence": "unresolvable"}
          ]
        }` + "\n```", nil
	}
	if err := Enhance(g, fakeLLM); err != nil {
		t.Fatal(err)
	}

	concept := g.Nodes[NodeID("proj", "concept:persistence")]
	if concept == nil {
		t.Fatal("concept node not added")
	}
	edgeID := EdgeID("proj", concept.ID, NodeID("proj", "type:Store@utils/store/store.go"), EdgeReference)
	if g.Edges[edgeID] == nil {
		t.Fatal("concept references edge not added")
	}
	// Bad kind and unresolvable endpoints are skipped.
	for _, e := range g.Edges {
		if e.Kind == "deletes" {
			t.Fatal("edge with unsupported kind was added")
		}
		if strings.Contains(e.SourceID, "Nobody") {
			t.Fatal("edge with unresolvable endpoint was added")
		}
	}
}

func TestEnhanceRejectsBadJSON(t *testing.T) {
	g := Build(fixtureScan(), "proj")
	if err := Enhance(g, func(string) (string, error) { return "not json", nil }); err == nil {
		t.Fatal("expected error for malformed enhancement response")
	}
}

func openTestStore(t *testing.T) *semanticmemory.Store {
	t.Helper()
	store, err := semanticmemory.Open(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestSaveRebuildAndQuery(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	g := Build(fixtureScan(), "proj")
	if err := Rebuild(ctx, store, g); err != nil {
		t.Fatal(err)
	}

	q := NewQuerier(store, "proj")

	explain, err := q.Explain(ctx, "Store")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(explain, "Node: Store") || !strings.Contains(explain, "[EXTRACTED]") {
		t.Fatalf("unexpected explain output:\n%s", explain)
	}

	pathOut, err := q.Path(ctx, "cli", "Store")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pathOut, "Shortest path") {
		t.Fatalf("unexpected path output:\n%s", pathOut)
	}

	stats, err := q.Stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stats, "Hub nodes") {
		t.Fatalf("unexpected stats output:\n%s", stats)
	}

	exported, err := q.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.Nodes) == 0 || len(exported.Edges) == 0 {
		t.Fatal("export is empty")
	}
	if _, err := json.Marshal(exported); err != nil {
		t.Fatal(err)
	}

	// Rebuild replaces rather than duplicates.
	before := len(exported.Nodes)
	if err := Rebuild(ctx, store, Build(fixtureScan(), "proj")); err != nil {
		t.Fatal(err)
	}
	after, err := q.Export(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Nodes) != before {
		t.Fatalf("rebuild duplicated nodes: before %d, after %d", before, len(after.Nodes))
	}
}

func TestWordMatch(t *testing.T) {
	cases := []struct {
		text, name string
		want       bool
	}{
		{"func Run(s *store.Store) error", "Store", true},
		{"Storage layer", "Store", false},
		{"func Open(path string)", "Store", false},
		{"re *StoreManager", "Store", false}, // prefix of a longer identifier is not a word match
	}
	for _, c := range cases {
		if got := wordMatch(c.text, c.name); got != c.want {
			t.Errorf("wordMatch(%q, %q) = %v, want %v", c.text, c.name, got, c.want)
		}
	}
}
