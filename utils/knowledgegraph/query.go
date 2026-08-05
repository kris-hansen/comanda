package knowledgegraph

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/kris-hansen/comanda/utils/semanticmemory"
)

// Querier runs read-only graph questions against the semantic memory store.
type Querier struct {
	store     *semanticmemory.Store
	namespace string
}

// NewQuerier binds a querier to a store and namespace.
func NewQuerier(store *semanticmemory.Store, namespace string) *Querier {
	return &Querier{store: store, namespace: namespace}
}

// resolve finds the single best node for a user-supplied name. Exact
// case-sensitive matches win, then case-insensitive, then more specific kinds
// (a type named "Store" beats a package named "store"), then FTS ranking.
func (q *Querier) resolve(ctx context.Context, name string) (*semanticmemory.GraphNode, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("empty node name")
	}
	nodes, err := q.store.GraphFindNodes(ctx, q.namespace, name, 10)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no graph node matches %q", name)
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		return resolvePriority(nodes[i].Kind) < resolvePriority(nodes[j].Kind)
	})
	for i := range nodes {
		if nodes[i].Name == name {
			return &nodes[i], nil
		}
	}
	for i := range nodes {
		if strings.EqualFold(nodes[i].Name, name) {
			return &nodes[i], nil
		}
	}
	return &nodes[0], nil
}

// Explain renders a node and its connections, grouped by edge kind, with
// confidence tags. Mirrors `graphify explain`.
func (q *Querier) Explain(ctx context.Context, name string) (string, error) {
	node, err := q.resolve(ctx, name)
	if err != nil {
		return "", err
	}
	edges, neighbors, err := q.store.GraphNeighbors(ctx, q.namespace, node.ID)
	if err != nil {
		return "", err
	}
	byID := make(map[string]semanticmemory.GraphNode, len(neighbors))
	for _, n := range neighbors {
		byID[n.ID] = n
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Node: %s\n", NodeLabel(node))
	if node.Package != "" {
		fmt.Fprintf(&b, "  Package: %s\n", node.Package)
	}
	if node.Summary != "" {
		fmt.Fprintf(&b, "  Summary: %s\n", node.Summary)
	}
	fmt.Fprintf(&b, "  Degree:  %d\n", node.Degree)
	if len(edges) == 0 {
		b.WriteString("\nNo connections.\n")
		return b.String(), nil
	}

	fmt.Fprintf(&b, "\nConnections (%d):\n", len(edges))
	for _, edge := range edges {
		other, ok := byID[edge.TargetID]
		arrow := "-->"
		if edge.SourceID == node.ID {
			arrow = "-->"
		} else {
			other, ok = byID[edge.SourceID]
			arrow = "<--"
		}
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "  %s %s [%s] [%s]\n", arrow, NodeLabel(&other), edge.Kind, strings.ToUpper(edge.Confidence))
	}
	return b.String(), nil
}

// Path finds the shortest connection between two nodes (edges traversed in
// either direction) and renders the hop chain.
func (q *Querier) Path(ctx context.Context, fromName, toName string) (string, error) {
	from, to, hops, err := q.pathHops(ctx, fromName, toName)
	if err != nil {
		return "", err
	}
	if hops == nil {
		return fmt.Sprintf("%s and %s resolve to the same node.\n", fromName, toName), nil
	}
	if len(hops) == 0 {
		return fmt.Sprintf("No path between %s and %s.\n", NodeLabel(from), NodeLabel(to)), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Shortest path (%d hops):\n", len(hops))
	for _, hop := range hops {
		fmt.Fprintf(&b, "  %s\n", hop)
	}
	return b.String(), nil
}

// pathHops resolves both names and returns the hop chain between them.
// hops == nil means both names resolved to the same node; len(hops) == 0
// means no path exists.
func (q *Querier) pathHops(ctx context.Context, fromName, toName string) (*semanticmemory.GraphNode, *semanticmemory.GraphNode, []string, error) {
	from, err := q.resolve(ctx, fromName)
	if err != nil {
		return nil, nil, nil, err
	}
	to, err := q.resolve(ctx, toName)
	if err != nil {
		return nil, nil, nil, err
	}
	if from.ID == to.ID {
		return from, to, nil, nil
	}

	edges, err := q.store.GraphEdges(ctx, q.namespace)
	if err != nil {
		return nil, nil, nil, err
	}
	nodes, err := q.store.GraphNodes(ctx, q.namespace)
	if err != nil {
		return nil, nil, nil, err
	}
	byID := make(map[string]semanticmemory.GraphNode, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}

	// BFS over the undirected adjacency.
	type step struct {
		neighbor string
		edge     semanticmemory.GraphEdge
	}
	adj := make(map[string][]step)
	for _, e := range edges {
		adj[e.SourceID] = append(adj[e.SourceID], step{neighbor: e.TargetID, edge: e})
		adj[e.TargetID] = append(adj[e.TargetID], step{neighbor: e.SourceID, edge: e})
	}
	type parent struct {
		node string
		edge semanticmemory.GraphEdge
	}
	parents := map[string]parent{from.ID: {}}
	queue := []string{from.ID}
	found := false
	for len(queue) > 0 && !found {
		var next []string
		for _, cur := range queue {
			for _, s := range adj[cur] {
				if _, seen := parents[s.neighbor]; seen {
					continue
				}
				parents[s.neighbor] = parent{node: cur, edge: s.edge}
				if s.neighbor == to.ID {
					found = true
					break
				}
				next = append(next, s.neighbor)
			}
			if found {
				break
			}
		}
		queue = next
	}
	if !found {
		return from, to, []string{}, nil
	}

	// Walk back from target to source.
	var chain []string
	cur := to.ID
	for cur != from.ID {
		p := parents[cur]
		fmtEdge := fmt.Sprintf("%s (%s)", p.edge.Kind, p.edge.Confidence)
		chain = append([]string{fmt.Sprintf("%s --%s--> %s", nodeShortName(byID[p.node]), fmtEdge, nodeShortName(byID[cur]))}, chain...)
		cur = p.node
	}
	return from, to, chain, nil
}

// maxQuerySeeds and maxQueryChars bound the scoped subgraph returned by Query.
const (
	maxQuerySeeds = 5
	maxQueryChars = 6000
)

// Query answers a plain-language question with a scoped subgraph: the best
// matching nodes plus their immediate connections, as compact markdown.
func (q *Querier) Query(ctx context.Context, question string) (string, error) {
	seeds, err := q.store.GraphFindNodes(ctx, q.namespace, question, maxQuerySeeds)
	if err != nil {
		return "", err
	}
	if len(seeds) == 0 {
		return fmt.Sprintf("No graph nodes match %q.\n", question), nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Subgraph for %q (%d seed nodes):\n", question, len(seeds))
	used := 0
	for i := range seeds {
		section, err := q.Explain(ctx, seeds[i].Name)
		if err != nil {
			continue
		}
		if used+len(section) > maxQueryChars {
			break
		}
		b.WriteString("\n---\n")
		b.WriteString(section)
		used += len(section)
	}
	return b.String(), nil
}

// Stats summarizes the graph: counts by kind and the highest-degree hub nodes
// (graphify's "god nodes").
func (q *Querier) Stats(ctx context.Context) (string, error) {
	nodes, err := q.store.GraphNodes(ctx, q.namespace)
	if err != nil {
		return "", err
	}
	edges, err := q.store.GraphEdges(ctx, q.namespace)
	if err != nil {
		return "", err
	}

	nodeKinds := make(map[string]int)
	for _, n := range nodes {
		nodeKinds[n.Kind]++
	}
	edgeKinds := make(map[string]int)
	inferred := 0
	for _, e := range edges {
		edgeKinds[e.Kind]++
		if e.Confidence == ConfidenceInferred {
			inferred++
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Graph: %s\n", q.namespace)
	fmt.Fprintf(&b, "Nodes: %d (%s)\n", len(nodes), kindBreakdown(nodeKinds))
	fmt.Fprintf(&b, "Edges: %d (%s; %d inferred)\n", len(edges), kindBreakdown(edgeKinds), inferred)

	const hubCount = 10
	sorted := make([]semanticmemory.GraphNode, len(nodes))
	copy(sorted, nodes)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Degree > sorted[j].Degree })
	b.WriteString("\nHub nodes (most connected):\n")
	for i, n := range sorted {
		if i >= hubCount || n.Degree == 0 {
			break
		}
		fmt.Fprintf(&b, "  %2d. %s — degree %d\n", i+1, NodeLabel(&n), n.Degree)
	}
	return b.String(), nil
}

// Export dumps the namespace graph in the graphify-style graph.json shape.
func (q *Querier) Export(ctx context.Context) (*ExportGraph, error) {
	nodes, err := q.store.GraphNodes(ctx, q.namespace)
	if err != nil {
		return nil, err
	}
	edges, err := q.store.GraphEdges(ctx, q.namespace)
	if err != nil {
		return nil, err
	}
	out := &ExportGraph{
		Namespace: q.namespace,
		Nodes:     make([]ExportNode, 0, len(nodes)),
		Edges:     make([]ExportEdge, 0, len(edges)),
	}
	for _, n := range nodes {
		out.Nodes = append(out.Nodes, ExportNode{
			ID: LocalID(n.ID), Kind: n.Kind, Name: n.Name, Path: n.Path,
			Package: n.Package, Summary: n.Summary, Degree: n.Degree,
		})
	}
	for _, e := range edges {
		out.Edges = append(out.Edges, ExportEdge{
			Source: LocalID(e.SourceID), Target: LocalID(e.TargetID),
			Kind: e.Kind, Confidence: e.Confidence, Evidence: e.Evidence,
		})
	}
	return out, nil
}

// Resolve exposes node resolution for callers that need the matched node
// itself (e.g. JSON output paths).
func (q *Querier) Resolve(ctx context.Context, name string) (*semanticmemory.GraphNode, error) {
	return q.resolve(ctx, name)
}

// FindNodes returns the best matching nodes for a plain-text query.
func (q *Querier) FindNodes(ctx context.Context, query string, limit int) ([]semanticmemory.GraphNode, error) {
	return q.store.GraphFindNodes(ctx, q.namespace, query, limit)
}

// PathHops returns the hop chain between two nodes for structured output.
// An empty slice means no path; a nil slice means both names are the same node.
func (q *Querier) PathHops(ctx context.Context, fromName, toName string) ([]string, error) {
	_, _, hops, err := q.pathHops(ctx, fromName, toName)
	return hops, err
}

// ScopedExport dumps the subgraph reachable within `hops` of the given seed
// nodes, in the same shape as Export. Used for --json query/explain output.
func (q *Querier) ScopedExport(ctx context.Context, seeds []semanticmemory.GraphNode, hops int) (*ExportGraph, error) {
	if hops < 1 {
		hops = 1
	}
	allEdges, err := q.store.GraphEdges(ctx, q.namespace)
	if err != nil {
		return nil, err
	}
	inScope := make(map[string]semanticmemory.GraphNode, len(seeds))
	frontier := make(map[string]bool, len(seeds))
	for _, n := range seeds {
		inScope[n.ID] = n
		frontier[n.ID] = true
	}
	var scopedEdges []semanticmemory.GraphEdge
	for hop := 0; hop < hops; hop++ {
		next := make(map[string]bool)
		for _, e := range allEdges {
			srcIn, tgtIn := frontier[e.SourceID], frontier[e.TargetID]
			if !srcIn && !tgtIn {
				continue
			}
			scopedEdges = append(scopedEdges, e)
			for _, id := range []string{e.SourceID, e.TargetID} {
				if _, seen := inScope[id]; !seen {
					node, err := q.store.GetGraphNode(ctx, id)
					if err != nil {
						continue
					}
					inScope[id] = node
				}
				if !frontier[id] {
					next[id] = true
				}
			}
		}
		frontier = next
	}

	out := &ExportGraph{Namespace: q.namespace, Nodes: []ExportNode{}, Edges: []ExportEdge{}}
	for _, n := range inScope {
		out.Nodes = append(out.Nodes, ExportNode{
			ID: LocalID(n.ID), Kind: n.Kind, Name: n.Name, Path: n.Path,
			Package: n.Package, Summary: n.Summary, Degree: n.Degree,
		})
	}
	for _, e := range scopedEdges {
		out.Edges = append(out.Edges, ExportEdge{
			Source: LocalID(e.SourceID), Target: LocalID(e.TargetID),
			Kind: e.Kind, Confidence: e.Confidence, Evidence: e.Evidence,
		})
	}
	return out, nil
}

func kindBreakdown(counts map[string]int) string {
	kinds := make([]string, 0, len(counts))
	for k := range counts {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	parts := make([]string, 0, len(kinds))
	for _, k := range kinds {
		parts = append(parts, fmt.Sprintf("%s: %d", k, counts[k]))
	}
	return strings.Join(parts, ", ")
}

func nodeShortName(n semanticmemory.GraphNode) string {
	if n.Name != "" {
		return n.Name
	}
	return LocalID(n.ID)
}
