package knowledgegraph

import (
	"context"
	"fmt"
	"testing"

	"github.com/kris-hansen/comanda/utils/semanticmemory"
)

// chainNode/chainEdge build a simple linear namespace: seed -> n1 -> n2 -> n3 -> n4.
func buildChainGraph(t *testing.T, store *semanticmemory.Store, namespace string) []semanticmemory.GraphNode {
	t.Helper()
	ctx := context.Background()
	names := []string{"seed", "n1", "n2", "n3", "n4"}
	nodes := make([]semanticmemory.GraphNode, len(names))
	for i, name := range names {
		node, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{
			ID: namespace + "|" + name, Namespace: namespace,
			Kind: semanticmemory.GraphNodeFunction, Name: name,
		})
		if err != nil {
			t.Fatalf("upsert node %s: %v", name, err)
		}
		nodes[i] = node
	}
	for i := 0; i < len(nodes)-1; i++ {
		_, err := store.UpsertGraphEdge(ctx, semanticmemory.GraphEdge{
			ID: fmt.Sprintf("%s|edge%d", namespace, i), Namespace: namespace,
			SourceID: nodes[i].ID, TargetID: nodes[i+1].ID,
			Kind: semanticmemory.GraphEdgeUses, Confidence: semanticmemory.GraphConfidenceExtracted,
		})
		if err != nil {
			t.Fatalf("upsert edge %d: %v", i, err)
		}
	}
	return nodes
}

func TestScopedExportRespectsDepthBounds(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	nodes := buildChainGraph(t, store, "chain")
	q := NewQuerier(store, "chain")

	for hops, wantNodes := range map[int]int{1: 2, 2: 3, 3: 4} {
		out, err := q.ScopedExport(ctx, []semanticmemory.GraphNode{nodes[0]}, hops)
		if err != nil {
			t.Fatalf("hops=%d: %v", hops, err)
		}
		if len(out.Nodes) != wantNodes {
			t.Fatalf("hops=%d: got %d nodes, want %d", hops, len(out.Nodes), wantNodes)
		}
		if len(out.Edges) != wantNodes-1 {
			t.Fatalf("hops=%d: got %d edges, want %d", hops, len(out.Edges), wantNodes-1)
		}
		if out.Truncated {
			t.Fatalf("hops=%d: unexpectedly truncated", hops)
		}
	}

	// hops beyond the graph's actual depth should just return everything
	// reachable and stop (the frontier empties out), not error or overrun.
	out, err := q.ScopedExport(ctx, []semanticmemory.GraphNode{nodes[0]}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Nodes) != len(nodes) {
		t.Fatalf("hops=10 got %d nodes, want %d (the whole chain)", len(out.Nodes), len(nodes))
	}
	if len(out.Edges) != len(nodes)-1 {
		t.Fatalf("hops=10 got %d edges, want %d", len(out.Edges), len(nodes)-1)
	}
}

func TestScopedExportIgnoresUnrelatedNamespaceData(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	nodes := buildChainGraph(t, store, "chain")

	// A large, disjoint cluster in the same namespace that a full-namespace
	// scan would pull into memory but a bounded, frontier-driven traversal
	// never has a reason to touch.
	const disjointSize = 500
	prev := ""
	for i := 0; i < disjointSize; i++ {
		id := fmt.Sprintf("chain|far%d", i)
		if _, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{
			ID: id, Namespace: "chain", Kind: semanticmemory.GraphNodeFunction, Name: fmt.Sprintf("far%d", i),
		}); err != nil {
			t.Fatal(err)
		}
		if prev != "" {
			if _, err := store.UpsertGraphEdge(ctx, semanticmemory.GraphEdge{
				ID: fmt.Sprintf("chain|far-edge%d", i), Namespace: "chain",
				SourceID: prev, TargetID: id, Kind: semanticmemory.GraphEdgeUses, Confidence: semanticmemory.GraphConfidenceExtracted,
			}); err != nil {
				t.Fatal(err)
			}
		}
		prev = id
	}

	q := NewQuerier(store, "chain")
	out, err := q.ScopedExport(ctx, []semanticmemory.GraphNode{nodes[0]}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Nodes) != 4 {
		t.Fatalf("got %d nodes, want 4 (disjoint cluster must not be pulled in)", len(out.Nodes))
	}
	for _, n := range out.Nodes {
		if n.ID == "far0" || n.ID == fmt.Sprintf("far%d", disjointSize-1) {
			t.Fatalf("disjoint node %s leaked into scoped export", n.ID)
		}
	}
}

// TestScopedExportCapsHighDegreeHubWithoutDanglingEdges proves that when a
// single hop's fan-out exceeds the node cap, every edge in the result still
// references a node present in the result: an edge whose far endpoint got
// dropped by the cap must not survive into the output.
func TestScopedExportCapsHighDegreeHubWithoutDanglingEdges(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	const namespace = "hub"
	hub, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{
		ID: namespace + "|hub", Namespace: namespace, Kind: semanticmemory.GraphNodeFunction, Name: "hub",
	})
	if err != nil {
		t.Fatal(err)
	}

	// More leaves than both maxScopedEdgesPerHop and maxScopedTotalNodes so
	// the per-hop edge fetch, the total-edge budget, and the total-node cap
	// can all plausibly bind depending on implementation details.
	const leafCount = maxScopedEdgesPerHop + 500
	for i := 0; i < leafCount; i++ {
		leafID := fmt.Sprintf("%s|leaf%d", namespace, i)
		if _, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{
			ID: leafID, Namespace: namespace, Kind: semanticmemory.GraphNodeFunction, Name: fmt.Sprintf("leaf%d", i),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.UpsertGraphEdge(ctx, semanticmemory.GraphEdge{
			ID: fmt.Sprintf("%s|hub-edge%d", namespace, i), Namespace: namespace,
			SourceID: hub.ID, TargetID: leafID, Kind: semanticmemory.GraphEdgeUses, Confidence: semanticmemory.GraphConfidenceExtracted,
		}); err != nil {
			t.Fatal(err)
		}
	}

	q := NewQuerier(store, namespace)
	out, err := q.ScopedExport(ctx, []semanticmemory.GraphNode{hub}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Truncated {
		t.Fatal("expected Truncated=true for a hub whose fan-out exceeds the bounds")
	}
	if len(out.Nodes) > maxScopedTotalNodes {
		t.Fatalf("got %d nodes, want <= %d (maxScopedTotalNodes)", len(out.Nodes), maxScopedTotalNodes)
	}
	if len(out.Edges) > maxScopedEdgesPerHop {
		t.Fatalf("got %d edges, want <= %d (maxScopedEdgesPerHop)", len(out.Edges), maxScopedEdgesPerHop)
	}

	present := make(map[string]bool, len(out.Nodes))
	for _, n := range out.Nodes {
		present[n.ID] = true
	}
	for _, e := range out.Edges {
		if !present[e.Source] {
			t.Fatalf("dangling edge: source %q not present in returned nodes", e.Source)
		}
		if !present[e.Target] {
			t.Fatalf("dangling edge: target %q not present in returned nodes", e.Target)
		}
	}
}

// TestScopedExportEdgeCapWithoutDanglingEdges drives the total-edge budget
// (rather than the node cap) to zero across several hops of a wide-but-short
// graph, and checks the same no-dangling-edge invariant.
func TestScopedExportEdgeCapWithoutDanglingEdges(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)

	const namespace = "wide"
	seed, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{
		ID: namespace + "|seed", Namespace: namespace, Kind: semanticmemory.GraphNodeFunction, Name: "seed",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Three layers of ~1900 nodes each (under maxScopedEdgesPerHop per hop,
	// but the cumulative edge count across hops exceeds maxScopedTotalEdges),
	// each layer fully wired to the previous one's frontier via one edge per
	// node so no single hop trips the per-hop or node caps first.
	const layerSize = 1900
	prevLayer := []string{seed.ID}
	for layer := 0; layer < 3; layer++ {
		nextLayer := make([]string, 0, layerSize)
		for i := 0; i < layerSize/len(prevLayer); i++ {
			for _, parent := range prevLayer {
				id := fmt.Sprintf("%s|l%d-%s-%d", namespace, layer, parent, i)
				if _, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{
					ID: id, Namespace: namespace, Kind: semanticmemory.GraphNodeFunction, Name: id,
				}); err != nil {
					t.Fatal(err)
				}
				if _, err := store.UpsertGraphEdge(ctx, semanticmemory.GraphEdge{
					ID: id + "|e", Namespace: namespace, SourceID: parent, TargetID: id,
					Kind: semanticmemory.GraphEdgeUses, Confidence: semanticmemory.GraphConfidenceExtracted,
				}); err != nil {
					t.Fatal(err)
				}
				nextLayer = append(nextLayer, id)
			}
		}
		prevLayer = nextLayer
	}

	q := NewQuerier(store, namespace)
	out, err := q.ScopedExport(ctx, []semanticmemory.GraphNode{seed}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Truncated {
		t.Fatal("expected Truncated=true once the cumulative edge/node budget is exceeded")
	}
	if len(out.Nodes) > maxScopedTotalNodes {
		t.Fatalf("got %d nodes, want <= %d", len(out.Nodes), maxScopedTotalNodes)
	}
	if len(out.Edges) > maxScopedTotalEdges {
		t.Fatalf("got %d edges, want <= %d", len(out.Edges), maxScopedTotalEdges)
	}

	present := make(map[string]bool, len(out.Nodes))
	for _, n := range out.Nodes {
		present[n.ID] = true
	}
	for _, e := range out.Edges {
		if !present[e.Source] || !present[e.Target] {
			t.Fatalf("dangling edge %s -> %s in truncated export", e.Source, e.Target)
		}
	}
}
