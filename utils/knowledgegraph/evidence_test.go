package knowledgegraph

import (
	"context"
	"testing"

	"github.com/kris-hansen/comanda/utils/semanticmemory"
)

func TestDeduplicatedEdgeRetainsMultipleEvidencePassages(t *testing.T) {
	graph := NewGraph("proj")
	left := graph.AddNode("file:left.md", NodeDocument, "left.md", "left.md", "", "")
	right := graph.AddNode("file:right.md", NodeDocument, "right.md", "right.md", "", "")
	graph.AddEdge(left.ID, right.ID, EdgeLinksTo, ConfidenceExtracted, "document link")
	edgeID := EdgeID("proj", left.ID, right.ID, EdgeLinksTo)
	for _, line := range []int{4, 9} {
		graph.AddEvidence(edgeID, semanticmemory.GraphEvidence{
			SourcePath: "left.md", StartLine: line, EndLine: line,
			Quote: "right.md", ParserVersion: "test-v1",
		})
	}

	store := openTestStore(t)
	if err := Rebuild(context.Background(), store, graph); err != nil {
		t.Fatal(err)
	}
	evidence, err := store.GraphEvidence(context.Background(), "proj")
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 2 {
		t.Fatalf("evidence passages = %d, want 2", len(evidence))
	}
}
