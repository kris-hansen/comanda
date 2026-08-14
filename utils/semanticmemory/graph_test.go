package semanticmemory

import (
	"context"
	"path/filepath"
	"testing"
)

func TestGraphNodeUpsertMirrorsIntoMemorySearch(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	node, err := store.UpsertGraphNode(ctx, GraphNode{
		ID: "n1", Namespace: "repo", Kind: GraphNodeType,
		Name: "Manager", Path: "utils/codebaseindex/manager.go", Summary: "Orchestrates codebase indexing",
	})
	if err != nil {
		t.Fatal(err)
	}
	if node.Namespace != "repo" {
		t.Fatalf("unexpected namespace: %s", node.Namespace)
	}

	// Mirror record should be findable through regular memory search.
	records, err := store.Search(ctx, "Manager indexing", SearchOptions{Namespace: "repo", Types: []string{"graph_node"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d mirrored records, want 1", len(records))
	}
	if records[0].ID != graphMirrorRecordPrefix+"n1" {
		t.Fatalf("unexpected mirror ID: %s", records[0].ID)
	}

	// Graph FTS lookup should find the node itself.
	found, err := store.GraphFindNodes(ctx, "repo", "Manager", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 || found[0].ID != "n1" {
		t.Fatalf("unexpected find result: %#v", found)
	}

	// Stable ID upsert updates in place.
	if _, err := store.UpsertGraphNode(ctx, GraphNode{ID: "n1", Namespace: "repo", Kind: GraphNodeType, Name: "Manager", Summary: "Updated"}); err != nil {
		t.Fatal(err)
	}
	updated, err := store.GetGraphNode(ctx, "n1")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Summary != "Updated" {
		t.Fatalf("summary not updated: %s", updated.Summary)
	}
}

func TestGraphEdgesNeighborsDegreesAndDelete(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	for _, node := range []GraphNode{
		{ID: "file-a", Namespace: "repo", Kind: GraphNodeFile, Name: "a.go"},
		{ID: "file-b", Namespace: "repo", Kind: GraphNodeFile, Name: "b.go"},
		{ID: "fn-x", Namespace: "repo", Kind: GraphNodeFunction, Name: "X"},
	} {
		if _, err := store.UpsertGraphNode(ctx, node); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.UpsertGraphEdge(ctx, GraphEdge{ID: "e1", Namespace: "repo", SourceID: "file-a", TargetID: "fn-x", Kind: GraphEdgeDefines}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGraphEdge(ctx, GraphEdge{ID: "e2", Namespace: "repo", SourceID: "file-b", TargetID: "fn-x", Kind: GraphEdgeUses, Confidence: GraphConfidenceInferred}); err != nil {
		t.Fatal(err)
	}
	if err := store.RefreshGraphDegrees(ctx, "repo"); err != nil {
		t.Fatal(err)
	}

	hub, err := store.GetGraphNode(ctx, "fn-x")
	if err != nil {
		t.Fatal(err)
	}
	if hub.Degree != 2 {
		t.Fatalf("degree = %d, want 2", hub.Degree)
	}

	edges, nodes, err := store.GraphNeighbors(ctx, "repo", "fn-x")
	if err != nil {
		t.Fatal(err)
	}
	if len(edges) != 2 || len(nodes) != 2 {
		t.Fatalf("neighbors: %d edges, %d nodes; want 2/2", len(edges), len(nodes))
	}

	allEdges, err := store.GraphEdges(ctx, "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(allEdges) != 2 {
		t.Fatalf("got %d edges, want 2", len(allEdges))
	}

	if err := store.DeleteGraphNamespace(ctx, "repo"); err != nil {
		t.Fatal(err)
	}
	remainingNodes, err := store.GraphNodes(ctx, "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(remainingNodes) != 0 {
		t.Fatalf("got %d nodes after delete, want 0", len(remainingNodes))
	}
	remainingEdges, err := store.GraphEdges(ctx, "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(remainingEdges) != 0 {
		t.Fatalf("got %d edges after delete, want 0", len(remainingEdges))
	}
	mirrored, err := store.Search(ctx, "X", SearchOptions{Namespace: "repo", Types: []string{"graph_node"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(mirrored) != 0 {
		t.Fatalf("got %d mirrored records after delete, want 0", len(mirrored))
	}
}

func TestGraphAnnotationPersistsAndMirrorsIntoGraphRecall(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	annotation, err := store.UpsertGraphAnnotation(ctx, GraphAnnotation{
		ID: "note-1", Namespace: "repo", NodeID: "repo|worker",
		Content: "Keep this worker idempotent when changing retries.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if annotation.UpdatedAt.IsZero() {
		t.Fatal("annotation update time was not set")
	}

	annotations, err := store.GraphAnnotations(ctx, "repo", "repo|worker")
	if err != nil {
		t.Fatal(err)
	}
	if len(annotations) != 1 || annotations[0].Content != annotation.Content {
		t.Fatalf("annotations = %#v, want saved annotation", annotations)
	}

	records, err := store.Search(ctx, "idempotent retries", SearchOptions{Namespace: "repo", Types: []string{"graph_node"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].ID != graphAnnotationRecordPrefix+"note-1" {
		t.Fatalf("annotation mirror = %#v, want graph recall record", records)
	}

	if err := store.DeleteGraphNamespace(ctx, "repo"); err != nil {
		t.Fatal(err)
	}
	annotations, err = store.GraphAnnotations(ctx, "repo", "repo|worker")
	if err != nil {
		t.Fatal(err)
	}
	if len(annotations) != 0 {
		t.Fatalf("annotations after delete = %#v, want none", annotations)
	}
}
