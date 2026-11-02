package knowledgegraph

import (
	"context"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/codebaseindex"
)

func TestDocumentGraphRetrievalResolutionAndEvidence(t *testing.T) {
	root := t.TempDir()
	writeDoc(t, root, "main.go", "package main\n\nfunc main() {}\n")
	writeDoc(t, root, "AGENTS.md", "# Agent Rules\n\nRun the document graph tests.\n")
	writeDoc(t, root, "README.md", "# Readme\n\nGeneral project documentation.\n")
	writeDoc(t, root, "docs/guide.md", `---
id: guide
related_to: other
---
# Guide

A decision related to the other document.
`)
	writeDoc(t, root, "docs/source.md", `# Source

See [install](other.md#install), [[duplicate]], [missing](missing.md), and [code](../main.go).
`)
	writeDoc(t, root, "docs/other.md", `---
id: other
---
# Other

## Install

This paragraph contains unique searchable prose for section retrieval.
`)
	writeDoc(t, root, "docs/a/duplicate.md", "# A\n")
	writeDoc(t, root, "docs/b/duplicate.md", "# B\n")

	manager, err := codebaseindex.NewManager(&codebaseindex.Config{Root: root}, false)
	if err != nil {
		t.Fatal(err)
	}
	scan, _, err := manager.Scan()
	if err != nil {
		t.Fatal(err)
	}
	graph := Build(scan, "proj")

	if !hasEdge(graph, "proj", "section:docs/source.md#source", "section:docs/other.md#install", EdgeLinksTo) {
		t.Fatal("section anchor did not resolve to a section node")
	}
	if !hasEdge(graph, "proj", "section:docs/source.md#source", "file:main.go", EdgeLinksTo) {
		t.Fatal("documentation did not link to the source file")
	}
	if !hasEdge(graph, "proj", "file:docs/guide.md", "file:docs/other.md", EdgeRelatedTo) {
		t.Fatal("frontmatter relation was not extracted")
	}
	if hasEdge(graph, "proj", "file:README.md", "pkg:main", EdgeGuides) {
		t.Fatal("README should not become an agent-instruction guide")
	}
	if !hasEdge(graph, "proj", "file:AGENTS.md", "pkg:main", EdgeGuides) {
		t.Fatal("agent instructions should guide their directory scope")
	}

	diagnosticKinds := map[string]bool{}
	for _, diagnostic := range graph.Diagnostics {
		diagnosticKinds[diagnostic.Kind] = true
	}
	if !diagnosticKinds["ambiguous_link"] || !diagnosticKinds["unresolved_link"] {
		t.Fatalf("missing link diagnostics: %#v", graph.Diagnostics)
	}
	for _, edge := range graph.Edges {
		if edge.Kind == EdgeLinksTo && edge.SourceID == NodeID("proj", "section:docs/source.md#source") &&
			(edge.TargetID == NodeID("proj", "file:docs/a/duplicate.md") || edge.TargetID == NodeID("proj", "file:docs/b/duplicate.md")) {
			t.Fatal("ambiguous wiki link resolved to an arbitrary document")
		}
	}

	store := openTestStore(t)
	if err := Rebuild(context.Background(), store, graph); err != nil {
		t.Fatal(err)
	}
	matches, err := store.GraphFindNodes(context.Background(), "proj", "unique searchable prose", 10)
	if err != nil {
		t.Fatal(err)
	}
	var found *string
	for _, node := range matches {
		if node.Kind == NodeSection && strings.Contains(node.Content, "unique searchable prose") {
			content := node.Content
			found = &content
			if node.StartLine == 0 || !strings.Contains(NodeLabel(&node), "docs/other.md:") {
				t.Errorf("retrieved section lacks citation: %#v", node)
			}
		}
	}
	if found == nil {
		t.Fatalf("section body was not searchable: %#v", matches)
	}
	answer, err := NewQuerier(store, "proj").Query(context.Background(), "unique searchable prose")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(answer, "unique searchable prose") || !strings.Contains(answer, "docs/other.md:") {
		t.Fatalf("query did not retain the exact body-bearing section:\n%s", answer)
	}
	exact, err := NewQuerier(store, "proj").Explain(context.Background(), "section:docs/other.md#install")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(exact, "unique searchable prose") {
		t.Fatalf("exact section ID did not resolve:\n%s", exact)
	}

	evidence, err := store.GraphEvidence(context.Background(), "proj")
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) == 0 {
		t.Fatal("structured evidence was not persisted")
	}
	exported, err := NewQuerier(store, "proj").Export(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(exported.Evidence) != len(evidence) {
		t.Fatalf("export evidence = %d, want %d", len(exported.Evidence), len(evidence))
	}
}

func TestDocumentSectionSplitsLargeContentIntoBlocks(t *testing.T) {
	scan := &codebaseindex.ScanResult{Candidates: []*codebaseindex.FileEntry{{
		Path: "large.md", Language: "markdown", Symbols: &codebaseindex.SymbolInfo{
			Package: "Large",
			Document: &codebaseindex.DocumentInfo{Title: "Large", Sections: []codebaseindex.DocumentSection{{
				ID: "large", Anchor: "large", Title: "Large", HeadingPath: "Large", Level: 1,
				StartLine: 1, BodyEndLine: 20, Content: strings.Repeat("x", maxDocumentBlockRunes+100),
			}}},
		},
	}}}
	graph := Build(scan, "proj")
	blocks := 0
	for _, node := range graph.Nodes {
		if node.Kind == NodeBlock {
			blocks++
			if node.Content == "" {
				t.Fatal("block has no retrievable content")
			}
		}
	}
	if blocks != 2 {
		t.Fatalf("blocks = %d, want 2", blocks)
	}
	section := graph.Nodes[NodeID("proj", "section:large.md#large")]
	if section.Content != "" {
		t.Fatal("large section should retrieve through its blocks")
	}
}
