package knowledgegraph

import (
	"context"
	"crypto/sha256"
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

// Namespace returns the graph namespace this querier is bound to.
func (q *Querier) Namespace() string { return q.namespace }

// Annotations returns durable human guidance for a node.
func (q *Querier) Annotations(ctx context.Context, node semanticmemory.GraphNode) ([]semanticmemory.GraphAnnotation, error) {
	return q.store.GraphAnnotations(ctx, q.namespace, node.ID)
}

// Annotate adds durable human guidance to an existing node.
func (q *Querier) Annotate(ctx context.Context, node semanticmemory.GraphNode, content string) (semanticmemory.GraphAnnotation, error) {
	return q.store.UpsertGraphAnnotation(ctx, semanticmemory.GraphAnnotation{ID: annotationID(node.ID, content), Namespace: q.namespace, NodeID: node.ID, Content: content})
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
	annotations, err := q.Annotations(ctx, *node)
	if err != nil {
		return "", err
	}
	for _, annotation := range annotations {
		fmt.Fprintf(&b, "  Human guidance: %s\n", annotation.Content)
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

func annotationID(nodeID, content string) string {
	return "annotation:" + nodeID + ":" + fmt.Sprintf("%x", sha256.Sum256([]byte(strings.TrimSpace(content))))[:16]
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

// Overview returns component nodes (or packages for an index without component
// classification), plus complete kind counts. It is the cheap first request
// for visualizing a large graph.
func (q *Querier) Overview(ctx context.Context) (*Overview, error) {
	counts, err := q.store.GraphNodeKindCounts(ctx, q.namespace)
	if err != nil {
		return nil, err
	}
	nodes, err := q.store.GraphNodesByKind(ctx, q.namespace, NodeComponent, 100)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		nodes, err = q.store.GraphNodesByKind(ctx, q.namespace, NodePackage, 100)
		if err != nil {
			return nil, err
		}
	}
	out := &Overview{Namespace: q.namespace, Nodes: make([]ExportNode, 0, len(nodes)), NodeKindCounts: counts}
	for _, node := range nodes {
		out.Nodes = append(out.Nodes, exportGraphNode(node))
	}
	return out, nil
}

// DatabaseNodeKinds identifies the kinds that make up a database's own
// self-contained graph, extracted from SQL DDL (see codebaseindex's Postgres
// DDL parser). They never connect to source-code nodes directly, so a
// scope-aware "database only" view needs its own fetch path rather than
// hiding code-kind checkboxes over whatever code-oriented graph happens to
// already be loaded client-side.
var DatabaseNodeKinds = map[string]bool{"schema": true, "table": true, "column": true}

// Bounds for DatabaseOverview's architecture-level database map. Table nodes
// already carry a compact "N columns" summary from extraction, so the
// architecture level never has to load column nodes — there can be tens of
// thousands of them — to show the database's footprint.
const (
	maxDatabaseOverviewSchemas = 200
	maxDatabaseOverviewTables  = 1000
)

// DatabaseOverview returns the architecture-level database map: every schema
// and table node (bounded, highest-degree first), their containment
// hierarchy and table-to-table foreign keys, and complete kind counts
// (including columns) so the caller can report the full database footprint
// without rendering a node per column. It degrades gracefully to an empty
// map for a namespace with no database nodes. It is the database-scoped
// counterpart to Overview, the cheap first request for visualizing a large
// database schema.
func (q *Querier) DatabaseOverview(ctx context.Context) (*Overview, error) {
	counts, err := q.store.GraphNodeKindCounts(ctx, q.namespace)
	if err != nil {
		return nil, err
	}
	schemas, err := q.store.GraphNodesByKind(ctx, q.namespace, "schema", maxDatabaseOverviewSchemas)
	if err != nil {
		return nil, err
	}
	tables, err := q.store.GraphNodesByKind(ctx, q.namespace, "table", maxDatabaseOverviewTables)
	if err != nil {
		return nil, err
	}

	out := &Overview{
		Namespace:      q.namespace,
		Nodes:          make([]ExportNode, 0, len(schemas)+len(tables)),
		NodeKindCounts: counts,
		Truncated:      counts["schema"] > len(schemas) || counts["table"] > len(tables),
	}
	inScope := make(map[string]bool, len(schemas)+len(tables))
	ids := make([]string, 0, len(schemas)+len(tables))
	for _, n := range schemas {
		out.Nodes = append(out.Nodes, exportGraphNode(n))
		inScope[n.ID] = true
		ids = append(ids, n.ID)
	}
	for _, n := range tables {
		out.Nodes = append(out.Nodes, exportGraphNode(n))
		inScope[n.ID] = true
		ids = append(ids, n.ID)
	}
	if len(ids) == 0 {
		return out, nil
	}

	// GraphEdgesBetweenNodes (not GraphEdgesForNodes) is required here: a
	// union query would pull in every table's edges to its columns before
	// ever reaching the schema/table hierarchy this view wants, so a
	// database with tens of thousands of columns would exhaust the edge
	// budget on nodes the architecture level never shows.
	edges, hasMore, err := q.store.GraphEdgesBetweenNodes(ctx, q.namespace, ids, maxScopedEdgesPerHop)
	if err != nil {
		return nil, err
	}
	if hasMore {
		out.Truncated = true
	}
	for _, e := range edges {
		out.Edges = append(out.Edges, ExportEdge{
			Source: LocalID(e.SourceID), Target: LocalID(e.TargetID),
			Kind: e.Kind, Confidence: e.Confidence, Evidence: e.Evidence,
		})
	}
	return out, nil
}

// NeighborPage returns one bounded page of direct neighbors without scanning
// or serializing the complete graph.
func (q *Querier) NeighborPage(ctx context.Context, focus semanticmemory.GraphNode, limit, offset int) (*NeighborPage, error) {
	return q.neighborPage(ctx, focus, limit, offset, nil)
}

// NeighborPageKinds returns a bounded direct-neighbor page whose kind filter
// is applied in the store before pagination. Filtering after NeighborPage is
// too late for mixed, high-degree monorepo nodes because the requested domain
// may not occur in the first generic page.
func (q *Querier) NeighborPageKinds(ctx context.Context, focus semanticmemory.GraphNode, limit, offset int, allowKinds map[string]bool) (*NeighborPage, error) {
	kinds := make([]string, 0, len(allowKinds))
	for kind, allowed := range allowKinds {
		if allowed {
			kinds = append(kinds, kind)
		}
	}
	sort.Strings(kinds)
	return q.neighborPage(ctx, focus, limit, offset, kinds)
}

func (q *Querier) neighborPage(ctx context.Context, focus semanticmemory.GraphNode, limit, offset int, kinds []string) (*NeighborPage, error) {
	var edges []semanticmemory.GraphEdge
	var nodes []semanticmemory.GraphNode
	var hasMore bool
	var err error
	if len(kinds) > 0 {
		edges, nodes, hasMore, err = q.store.GraphNeighborPageKinds(ctx, q.namespace, focus.ID, limit, offset, kinds)
	} else {
		edges, nodes, hasMore, err = q.store.GraphNeighborPage(ctx, q.namespace, focus.ID, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 160
	}
	out := &NeighborPage{
		Focus:   exportGraphNode(focus),
		Offset:  offset,
		HasMore: hasMore,
		Graph: ExportGraph{
			Namespace: q.namespace,
			Nodes:     make([]ExportNode, 0, len(nodes)+1),
			Edges:     make([]ExportEdge, 0, len(edges)),
		},
	}
	out.Graph.Nodes = append(out.Graph.Nodes, out.Focus)
	for _, node := range nodes {
		out.Graph.Nodes = append(out.Graph.Nodes, exportGraphNode(node))
	}
	for _, edge := range edges {
		out.Graph.Edges = append(out.Graph.Edges, ExportEdge{Source: LocalID(edge.SourceID), Target: LocalID(edge.TargetID), Kind: edge.Kind, Confidence: edge.Confidence, Evidence: edge.Evidence})
	}
	if hasMore {
		out.NextOffset = offset + len(edges)
	}
	return out, nil
}

// NodeByID returns a graph node by either its stable stored ID or its local
// export ID. It rejects nodes from another namespace.
func (q *Querier) NodeByID(ctx context.Context, id string) (*semanticmemory.GraphNode, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("empty node ID")
	}
	if !strings.Contains(id, "|") {
		id = NodeID(q.namespace, id)
	}
	node, err := q.store.GetGraphNode(ctx, id)
	if err != nil {
		return nil, err
	}
	if node.Namespace != q.namespace {
		return nil, fmt.Errorf("graph node not found")
	}
	return &node, nil
}

// PathHops returns the hop chain between two nodes for structured output.
// An empty slice means no path; a nil slice means both names are the same node.
func (q *Querier) PathHops(ctx context.Context, fromName, toName string) ([]string, error) {
	_, _, hops, err := q.pathHops(ctx, fromName, toName)
	return hops, err
}

// Bounds for ScopedExport's per-hop, per-total traversal. A focused subgraph
// must stay cheap even when a seed sits a few hops from a high-degree hub
// that touches a large fraction of a namespace's edges: without these caps a
// single hop can pull in thousands of edges from one hub node, and depth 3
// can cascade that across several hubs. When a cap is hit the traversal stops
// growing rather than hanging, and ExportGraph.Truncated reports it so
// callers can tell the user the view is partial.
const (
	maxScopedEdgesPerHop = 2000
	maxScopedTotalNodes  = 1500
	maxScopedTotalEdges  = 5000
)

// ScopedExport dumps the subgraph reachable within `hops` of the given seed
// nodes, in the same shape as Export. Used for --json query/explain output
// and the graph visualizer's focused/query subgraph views.
//
// Traversal is bounded and index-backed per hop (Store.GraphEdgesForNodes
// hits idx_graph_edges_namespace_source/target), not a scan of the
// namespace's full edge set: cost tracks the size of the requested
// neighborhood, not the graph. See the maxScoped* bounds above for what
// happens when a neighborhood is unusually large.
func (q *Querier) ScopedExport(ctx context.Context, seeds []semanticmemory.GraphNode, hops int) (*ExportGraph, error) {
	return q.scopedExport(ctx, seeds, hops, nil)
}

// ScopedExportKinds is ScopedExport restricted to a scope-aware node-kind
// allow-list during traversal (e.g. DatabaseNodeKinds). Seed nodes are
// always kept regardless of kind — the caller already chose them as the
// focus — but every node discovered while expanding the frontier is only
// admitted, and its edges only kept, when its kind is in allowKinds. This is
// what lets "database only" drill into a table and see its columns and
// foreign keys without crossing into unrelated node kinds, without assuming
// the domains are actually disjoint (they are, today, but nothing here
// depends on that).
func (q *Querier) ScopedExportKinds(ctx context.Context, seeds []semanticmemory.GraphNode, hops int, allowKinds map[string]bool) (*ExportGraph, error) {
	return q.scopedExport(ctx, seeds, hops, allowKinds)
}

func (q *Querier) scopedExport(ctx context.Context, seeds []semanticmemory.GraphNode, hops int, allowKinds map[string]bool) (*ExportGraph, error) {
	if hops < 1 {
		hops = 1
	}
	inScope := make(map[string]semanticmemory.GraphNode, len(seeds))
	frontier := make([]string, 0, len(seeds))
	for _, n := range seeds {
		if _, seen := inScope[n.ID]; seen {
			continue
		}
		inScope[n.ID] = n
		frontier = append(frontier, n.ID)
	}

	scopedEdges := make(map[string]semanticmemory.GraphEdge)
	// filteredIDs holds node IDs excluded by allowKinds — a scope choice, not
	// a size limit. An edge dropped only because its endpoint landed here
	// must not mark the result Truncated: that word means "there was more
	// and we stopped", not "the caller asked to see a narrower scope".
	filteredIDs := make(map[string]bool)
	truncated := false

	for hop := 0; hop < hops && len(frontier) > 0; hop++ {
		if len(inScope) >= maxScopedTotalNodes || len(scopedEdges) >= maxScopedTotalEdges {
			truncated = true
			break
		}

		edges, hasMore, err := q.store.GraphEdgesForNodes(ctx, q.namespace, frontier, maxScopedEdgesPerHop)
		if err != nil {
			return nil, err
		}
		if hasMore {
			truncated = true
		}

		// Cap the edges considered this hop to the remaining total-edge
		// budget before discovering new nodes from them, so a node is never
		// admitted on the strength of an edge that won't fit in the output.
		if budget := maxScopedTotalEdges - len(scopedEdges); len(edges) > budget {
			truncated = true
			edges = edges[:budget]
		}

		var newIDs []string
		seenNew := make(map[string]bool)
		for _, e := range edges {
			for _, id := range []string{e.SourceID, e.TargetID} {
				if _, seen := inScope[id]; seen {
					continue
				}
				if filteredIDs[id] {
					continue
				}
				if !seenNew[id] {
					seenNew[id] = true
					newIDs = append(newIDs, id)
				}
			}
		}

		if room := maxScopedTotalNodes - len(inScope); len(newIDs) > room {
			truncated = true
			if room < 0 {
				room = 0
			}
			newIDs = newIDs[:room]
		}

		if len(newIDs) > 0 {
			nodes, err := q.store.GetGraphNodesByIDs(ctx, newIDs)
			if err != nil {
				return nil, err
			}
			for _, node := range nodes {
				if allowKinds != nil && !allowKinds[node.Kind] {
					filteredIDs[node.ID] = true
					continue
				}
				inScope[node.ID] = node
			}
		}

		// Only now that node admission is final, add edges whose endpoints
		// both survived the caps. An edge whose endpoint was dropped for
		// exceeding maxScopedTotalNodes must never appear in the output —
		// it would dangle, pointing at a node the caller was never sent. An
		// edge whose endpoint was dropped by allowKinds instead is simply
		// out of scope, not truncated.
		frontier = frontier[:0]
		for _, e := range edges {
			_, srcIn := inScope[e.SourceID]
			_, tgtIn := inScope[e.TargetID]
			if !srcIn || !tgtIn {
				if !filteredIDs[e.SourceID] && !filteredIDs[e.TargetID] {
					truncated = true
				}
				continue
			}
			scopedEdges[e.ID] = e
		}
		for _, id := range newIDs {
			if _, ok := inScope[id]; ok {
				frontier = append(frontier, id)
			}
		}
	}

	out := &ExportGraph{
		Namespace: q.namespace,
		Nodes:     make([]ExportNode, 0, len(inScope)),
		Edges:     make([]ExportEdge, 0, len(scopedEdges)),
		Truncated: truncated,
	}
	for _, n := range inScope {
		out.Nodes = append(out.Nodes, exportGraphNode(n))
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

func exportGraphNode(node semanticmemory.GraphNode) ExportNode {
	return ExportNode{
		ID: LocalID(node.ID), Kind: node.Kind, Name: node.Name, Path: node.Path,
		Package: node.Package, Summary: node.Summary, Degree: node.Degree,
	}
}
