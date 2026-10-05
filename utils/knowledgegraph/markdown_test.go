package knowledgegraph

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/codebaseindex"
	"github.com/kris-hansen/comanda/utils/semanticmemory"
)

func markdownFixtureScan() *codebaseindex.ScanResult {
	return &codebaseindex.ScanResult{
		Candidates: []*codebaseindex.FileEntry{
			{
				Path:     "pkg/foo/foo.go",
				Language: "go",
				Symbols:  &codebaseindex.SymbolInfo{Package: "foo"},
			},
			{
				Path:     "cmd/tool.go",
				Language: "go",
				Symbols:  &codebaseindex.SymbolInfo{Package: "cmd"},
			},
			{
				Path:     "AGENTS.md",
				Language: "markdown",
				Symbols: &codebaseindex.SymbolInfo{
					Package: "Repo Guide",
					Types:   []codebaseindex.TypeInfo{{Name: "Repo Guide", Kind: "h1", IsExported: true}},
					Imports: []string{"pkg/foo/notes.md"},
				},
			},
			{
				Path:     "pkg/foo/AGENTS.md",
				Language: "markdown",
				Symbols: &codebaseindex.SymbolInfo{
					Package: "Foo Guide",
					Imports: []string{"./notes.md"},
				},
			},
			{
				Path:     "pkg/foo/notes.md",
				Language: "markdown",
				Symbols: &codebaseindex.SymbolInfo{
					Imports: []string{"./more.md", "More", "missing.md"},
				},
			},
			{Path: "pkg/foo/more.md", Language: "markdown"},
			{Path: "docs/plain.md", Language: "markdown", Symbols: &codebaseindex.SymbolInfo{}},
		},
		Components: []*codebaseindex.CodebaseComponent{
			{Name: "foo", Root: "pkg/foo", Kind: "backend", FileCount: 1},
			{Name: "cli", Root: "cmd", Kind: "cli", FileCount: 1},
		},
	}
}

func hasEdge(g *Graph, namespace, src, tgt, kind string) bool {
	return g.Edges[EdgeID(namespace, NodeID(namespace, src), NodeID(namespace, tgt), kind)] != nil
}

func TestBuildMarkdownDocuments(t *testing.T) {
	g := Build(markdownFixtureScan(), "proj")

	// Every markdown file is a document node on the stable file:<path> ID,
	// summarized by its first H1.
	for _, p := range []string{"AGENTS.md", "pkg/foo/AGENTS.md", "pkg/foo/notes.md", "pkg/foo/more.md", "docs/plain.md"} {
		node := g.Nodes[NodeID("proj", "file:"+p)]
		if node == nil {
			t.Fatalf("missing document node for %s", p)
		}
		if node.Kind != NodeDocument {
			t.Errorf("node %s kind = %s, want document", p, node.Kind)
		}
	}
	if got := g.Nodes[NodeID("proj", "file:AGENTS.md")].Summary; got != "Repo Guide" {
		t.Errorf("document summary = %q, want first H1", got)
	}

	// A document's H1 is a title, not a package: no package node and no
	// belongs_to edge for it.
	if g.Nodes[NodeID("proj", "pkg:Repo Guide")] != nil {
		t.Error("document title leaked into package nodes")
	}
	for _, e := range g.Edges {
		if e.Kind == EdgeBelongsTo && g.Nodes[e.SourceID] != nil && g.Nodes[e.SourceID].Kind == NodeDocument {
			t.Errorf("document %s has a belongs_to edge", e.SourceID)
		}
	}

	// guides edges follow the directory prefix rule: the root document guides
	// every package/component, the pkg/foo document only its own subtree.
	for _, tgt := range []string{"pkg:foo", "component:foo", "pkg:cmd", "component:cli"} {
		if !hasEdge(g, "proj", "file:AGENTS.md", tgt, EdgeGuides) {
			t.Errorf("root AGENTS.md missing guides edge to %s", tgt)
		}
	}
	for _, tgt := range []string{"pkg:foo", "component:foo"} {
		if !hasEdge(g, "proj", "file:pkg/foo/AGENTS.md", tgt, EdgeGuides) {
			t.Errorf("pkg/foo/AGENTS.md missing guides edge to %s", tgt)
		}
	}
	for _, tgt := range []string{"pkg:cmd", "component:cli"} {
		if hasEdge(g, "proj", "file:pkg/foo/AGENTS.md", tgt, EdgeGuides) {
			t.Errorf("pkg/foo/AGENTS.md guides %s outside its subtree", tgt)
		}
	}
	// Ordinary markdown documents never guide anything.
	for _, e := range g.Edges {
		if e.Kind == EdgeGuides && (e.SourceID == NodeID("proj", "file:docs/plain.md") || e.SourceID == NodeID("proj", "file:pkg/foo/notes.md")) {
			t.Errorf("non-context document %s has a guides edge", e.SourceID)
		}
		if e.Kind == EdgeGuides && e.Confidence != ConfidenceExtracted {
			t.Errorf("guides edge confidence = %s, want extracted", e.Confidence)
		}
	}

	// Doc-to-doc links resolve relative to the importing file, by basename for
	// wiki links, and are skipped when the target was not indexed.
	for _, link := range [][2]string{
		{"file:AGENTS.md", "file:pkg/foo/notes.md"},
		{"file:pkg/foo/AGENTS.md", "file:pkg/foo/notes.md"},
		{"file:pkg/foo/notes.md", "file:pkg/foo/more.md"},
	} {
		if !hasEdge(g, "proj", link[0], link[1], EdgeImports) {
			t.Errorf("missing doc import edge %s -> %s", link[0], link[1])
		}
	}
	if g.Nodes[NodeID("proj", "file:pkg/foo/missing.md")] != nil {
		t.Error("unresolved doc link created a node")
	}
	if g.Nodes[NodeID("proj", "pkg:./notes.md")] != nil || g.Nodes[NodeID("proj", "pkg:pkg/foo/notes.md")] != nil {
		t.Error("doc link leaked into package nodes")
	}

	// Code files are untouched: foo.go still belongs to its package.
	if g.Nodes[NodeID("proj", "file:pkg/foo/foo.go")].Kind != NodeFile {
		t.Error("go file lost its file kind")
	}
	if !hasEdge(g, "proj", "file:pkg/foo/foo.go", "pkg:foo", EdgeBelongsTo) {
		t.Error("go file lost its belongs_to edge")
	}
}

func writeDoc(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDocAnnotationsLayerAndSurviveRebuilds(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	writeDoc(t, root, "AGENTS.md", "# Repo Guide\n\nAlways run tests before shipping.\n\n## Conventions\n\nUse table-driven tests for parsers.\n")
	writeDoc(t, root, "pkg/foo/AGENTS.md", "# Foo Guide\n\nFoo keeps its storage behind an interface.\n")
	store := openTestStore(t)

	g := BuildWithRoot(markdownFixtureScan(), "proj", root)
	if g.Annotations == nil {
		t.Fatal("BuildWithRoot did not derive file annotations")
	}
	if err := Rebuild(ctx, store, g); err != nil {
		t.Fatal(err)
	}

	// Each section annotates the document node itself.
	docID := NodeID("proj", "file:AGENTS.md")
	docAnnotations, err := store.GraphAnnotations(ctx, "proj", docID)
	if err != nil {
		t.Fatal(err)
	}
	if len(docAnnotations) != 2 {
		t.Fatalf("document annotations = %d, want 2 sections", len(docAnnotations))
	}
	for _, a := range docAnnotations {
		if a.Source != semanticmemory.GraphAnnotationFile || a.SourcePath != "AGENTS.md" {
			t.Errorf("annotation layer = %s/%s, want file/AGENTS.md", a.Source, a.SourcePath)
		}
	}

	// The lead section also annotates every guided node.
	pkgAnnotations, err := store.GraphAnnotations(ctx, "proj", NodeID("proj", "pkg:foo"))
	if err != nil {
		t.Fatal(err)
	}
	var leadSections []string
	for _, a := range pkgAnnotations {
		leadSections = append(leadSections, a.Content)
	}
	joined := strings.Join(leadSections, "\n")
	if !strings.Contains(joined, "Always run tests before shipping.") {
		t.Errorf("pkg:foo missing root guide lead section:\n%s", joined)
	}
	if !strings.Contains(joined, "Foo keeps its storage behind an interface.") {
		t.Errorf("pkg:foo missing foo guide lead section:\n%s", joined)
	}

	// Human guidance rides the same store and must survive rebuilds.
	q := NewQuerier(store, "proj")
	component, err := q.Resolve(ctx, "foo")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.Annotate(ctx, *component, "Keep foo's API surface small."); err != nil {
		t.Fatal(err)
	}

	// Rebuild with edited doc content: stale file annotations are pruned, the
	// edited section is re-derived, and the human note survives.
	writeDoc(t, root, "AGENTS.md", "# Repo Guide\n\nRun integration tests before shipping.\n")
	if err := Rebuild(ctx, store, BuildWithRoot(markdownFixtureScan(), "proj", root)); err != nil {
		t.Fatal(err)
	}
	docAnnotations, err = store.GraphAnnotations(ctx, "proj", docID)
	if err != nil {
		t.Fatal(err)
	}
	if len(docAnnotations) != 1 || !strings.Contains(docAnnotations[0].Content, "integration tests") {
		t.Fatalf("document annotations after edit = %#v, want the single edited section", docAnnotations)
	}
	componentAnnotations, err := store.GraphAnnotations(ctx, "proj", component.ID)
	if err != nil {
		t.Fatal(err)
	}
	human := 0
	for _, a := range componentAnnotations {
		if a.Source == semanticmemory.GraphAnnotationHuman && a.Content == "Keep foo's API surface small." {
			human++
		}
	}
	if human != 1 {
		t.Fatalf("human annotations after rebuild = %d, want 1", human)
	}
}

func TestBuildWithoutRootSkipsAnnotationDerivation(t *testing.T) {
	g := Build(markdownFixtureScan(), "proj")
	if g.Annotations != nil {
		t.Fatal("Build without a scan root must not derive annotations")
	}
}
