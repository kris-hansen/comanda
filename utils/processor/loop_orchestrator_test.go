package processor

import "testing"

func TestDependencyGraphFindCycle_ClosedPath(t *testing.T) {
	graph := &DependencyGraph{
		nodes: map[string]*GraphNode{
			"loop-a": {name: "loop-a"},
			"loop-b": {name: "loop-b"},
			"loop-c": {name: "loop-c"},
		},
		edges: map[string][]string{
			"loop-a": {"loop-c"},
			"loop-b": {"loop-a"},
			"loop-c": {"loop-b"},
		},
		reverseEdges: map[string][]string{
			"loop-a": {"loop-b"},
			"loop-b": {"loop-c"},
			"loop-c": {"loop-a"},
		},
	}

	cycle := graph.findCycle()
	if len(cycle) != 4 {
		t.Fatalf("expected closed 3-node cycle, got %v", cycle)
	}

	if cycle[0] != cycle[len(cycle)-1] {
		t.Fatalf("expected cycle to start and end on same node, got %v", cycle)
	}

	for i := 1; i < len(cycle)-1; i++ {
		if cycle[i] == cycle[i+1] {
			t.Fatalf("expected no duplicate adjacent nodes in cycle, got %v", cycle)
		}
	}

	seen := map[string]bool{}
	for _, node := range cycle[:len(cycle)-1] {
		seen[node] = true
	}
	for _, expected := range []string{"loop-a", "loop-b", "loop-c"} {
		if !seen[expected] {
			t.Fatalf("expected cycle to include %s, got %v", expected, cycle)
		}
	}
}
