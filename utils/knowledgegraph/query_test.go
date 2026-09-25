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

// buildDatabaseGraph builds a small namespace mixing a database subgraph
// (schema -> two tables -> columns, one table-to-table foreign key) with an
// unrelated code subgraph (a function using a type), mirroring the shape
// codebaseindex's Postgres DDL extractor and Go extractor each produce in the
// same namespace.
func buildDatabaseGraph(t *testing.T, store *semanticmemory.Store, namespace string) (schema, users, orders, usersID, ordersUserID semanticmemory.GraphNode) {
	t.Helper()
	ctx := context.Background()
	upsertNode := func(id, kind, name string) semanticmemory.GraphNode {
		node, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{
			ID: namespace + "|" + id, Namespace: namespace, Kind: kind, Name: name,
		})
		if err != nil {
			t.Fatalf("upsert node %s: %v", id, err)
		}
		return node
	}
	upsertEdge := func(id string, source, target semanticmemory.GraphNode, kind string) {
		if _, err := store.UpsertGraphEdge(ctx, semanticmemory.GraphEdge{
			ID: namespace + "|" + id, Namespace: namespace, SourceID: source.ID, TargetID: target.ID,
			Kind: kind, Confidence: semanticmemory.GraphConfidenceExtracted,
		}); err != nil {
			t.Fatalf("upsert edge %s: %v", id, err)
		}
	}

	schema = upsertNode("schema:public", "schema", "public")
	users = upsertNode("table:public.users", "table", "public.users")
	orders = upsertNode("table:public.orders", "table", "public.orders")
	usersID = upsertNode("column:public.users.id", "column", "public.users.id")
	ordersUserID = upsertNode("column:public.orders.user_id", "column", "public.orders.user_id")
	upsertEdge("e-schema-users", schema, users, semanticmemory.GraphEdgeContains)
	upsertEdge("e-schema-orders", schema, orders, semanticmemory.GraphEdgeContains)
	upsertEdge("e-users-id", users, usersID, semanticmemory.GraphEdgeContains)
	upsertEdge("e-users-pk", users, usersID, "primary_key")
	upsertEdge("e-orders-userid", orders, ordersUserID, semanticmemory.GraphEdgeContains)
	upsertEdge("e-orders-fk", orders, users, "foreign_key")
	upsertEdge("e-orders-references", ordersUserID, usersID, "references")

	fn := upsertNode("func:main", semanticmemory.GraphNodeFunction, "main")
	typ := upsertNode("type:Handler", semanticmemory.GraphNodeType, "Handler")
	upsertEdge("e-code", fn, typ, semanticmemory.GraphEdgeUses)

	if err := store.RefreshGraphDegrees(ctx, namespace); err != nil {
		t.Fatalf("refresh degrees: %v", err)
	}
	return schema, users, orders, usersID, ordersUserID
}

// TestDatabaseOverviewReturnsSchemaTableHierarchyExcludingCode proves the
// core architecture-level bug fix: DatabaseOverview surfaces the schema/table
// hierarchy (non-empty, connected) while excluding both column and code
// nodes, unlike the plain Overview which returns only component/package
// nodes and leaves a "Database only" toggle with nothing loaded to show.
func TestDatabaseOverviewReturnsSchemaTableHierarchyExcludingCode(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	schema, users, orders, _, _ := buildDatabaseGraph(t, store, "dbdemo")
	q := NewQuerier(store, "dbdemo")

	out, err := q.DatabaseOverview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Nodes) == 0 {
		t.Fatal("DatabaseOverview returned no nodes for a graph with schema/table/column nodes")
	}
	if out.Truncated {
		t.Fatal("unexpectedly truncated for a small graph")
	}
	byID := make(map[string]ExportNode, len(out.Nodes))
	for _, n := range out.Nodes {
		byID[n.ID] = n
		switch n.Kind {
		case "column", semanticmemory.GraphNodeFunction, semanticmemory.GraphNodeType:
			t.Fatalf("DatabaseOverview leaked a non schema/table node: %+v", n)
		}
	}
	for _, want := range []semanticmemory.GraphNode{schema, users, orders} {
		if _, ok := byID[LocalID(want.ID)]; !ok {
			t.Fatalf("DatabaseOverview missing expected node %s", want.Name)
		}
	}

	// Hierarchy: schema contains both tables, and the table-to-table foreign
	// key survives even though it was extracted alongside column-level
	// primary_key/references edges that must not appear here.
	var sawSchemaContains, sawForeignKey int
	for _, e := range out.Edges {
		if e.Source == LocalID(schema.ID) && e.Kind == semanticmemory.GraphEdgeContains {
			sawSchemaContains++
		}
		if e.Kind == "primary_key" || e.Kind == "references" {
			t.Fatalf("DatabaseOverview leaked a column-level edge: %+v", e)
		}
		if e.Kind == "foreign_key" {
			sawForeignKey++
		}
	}
	if sawSchemaContains != 2 {
		t.Fatalf("schema -> table contains edges = %d, want 2", sawSchemaContains)
	}
	if sawForeignKey != 1 {
		t.Fatalf("table -> table foreign_key edges = %d, want 1", sawForeignKey)
	}

	// Compact metadata: counts (including columns and code kinds) are still
	// reported so the UI can show the full footprint without node-per-column
	// rendering.
	for _, kind := range []string{"schema", "table", "column", semanticmemory.GraphNodeFunction} {
		if out.NodeKindCounts[kind] == 0 {
			t.Fatalf("node_kind_counts missing kind %q: %#v", kind, out.NodeKindCounts)
		}
	}
}

// TestDatabaseOverviewGracefulOnCodeOnlyGraph proves an ordinary code-only
// graph (no schema/table/column nodes at all) degrades gracefully: an empty,
// non-error database overview instead of a crash or a nonsensical result.
func TestDatabaseOverviewGracefulOnCodeOnlyGraph(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	nodes := buildChainGraph(t, store, "codeonly")
	if err := store.RefreshGraphDegrees(ctx, "codeonly"); err != nil {
		t.Fatal(err)
	}
	q := NewQuerier(store, "codeonly")

	out, err := q.DatabaseOverview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Nodes) != 0 {
		t.Fatalf("got %d nodes for a code-only graph, want 0", len(out.Nodes))
	}
	if len(out.Edges) != 0 {
		t.Fatalf("got %d edges for a code-only graph, want 0", len(out.Edges))
	}
	if out.Truncated {
		t.Fatal("empty database overview must not report truncation")
	}
	if out.NodeKindCounts[semanticmemory.GraphNodeFunction] != len(nodes) {
		t.Fatalf("node_kind_counts[function] = %d, want %d", out.NodeKindCounts[semanticmemory.GraphNodeFunction], len(nodes))
	}
}

// TestDatabaseOverviewBoundsHighCardinalityColumns proves that a table with a
// very large number of columns does not blow the edge budget before the
// architecture level ever reaches the schema/table hierarchy it actually
// shows: GraphEdgesBetweenNodes (not the union-matching GraphEdgesForNodes)
// keeps the query scoped to schema/table nodes regardless of how many
// columns hang off of them.
func TestDatabaseOverviewBoundsHighCardinalityColumns(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	const namespace = "widecolumns"

	schema, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{
		ID: namespace + "|schema:public", Namespace: namespace, Kind: "schema", Name: "public",
	})
	if err != nil {
		t.Fatal(err)
	}
	table, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{
		ID: namespace + "|table:public.wide", Namespace: namespace, Kind: "table", Name: "public.wide",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGraphEdge(ctx, semanticmemory.GraphEdge{
		ID: namespace + "|e-schema-table", Namespace: namespace, SourceID: schema.ID, TargetID: table.ID,
		Kind: semanticmemory.GraphEdgeContains, Confidence: semanticmemory.GraphConfidenceExtracted,
	}); err != nil {
		t.Fatal(err)
	}

	// Comfortably more columns than maxScopedEdgesPerHop, standing in for the
	// ~79k column scale called out by the bug report.
	const columnCount = maxScopedEdgesPerHop + 500
	for i := 0; i < columnCount; i++ {
		colID := fmt.Sprintf("%s|column:public.wide.c%d", namespace, i)
		if _, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{
			ID: colID, Namespace: namespace, Kind: "column", Name: fmt.Sprintf("public.wide.c%d", i),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := store.UpsertGraphEdge(ctx, semanticmemory.GraphEdge{
			ID: fmt.Sprintf("%s|e-col%d", namespace, i), Namespace: namespace, SourceID: table.ID, TargetID: colID,
			Kind: semanticmemory.GraphEdgeContains, Confidence: semanticmemory.GraphConfidenceExtracted,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.RefreshGraphDegrees(ctx, namespace); err != nil {
		t.Fatal(err)
	}

	q := NewQuerier(store, namespace)
	out, err := q.DatabaseOverview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Nodes) != 2 {
		t.Fatalf("got %d architecture-level nodes, want 2 (schema + table, no columns)", len(out.Nodes))
	}
	if len(out.Edges) != 1 {
		t.Fatalf("got %d edges, want 1 (schema -> table), got columns leaking in: %+v", len(out.Edges), out.Edges)
	}
	if out.Truncated {
		t.Fatal("the schema/table hierarchy must not be reported truncated just because a table has many columns")
	}
	if out.NodeKindCounts["column"] != columnCount {
		t.Fatalf("node_kind_counts[column] = %d, want %d (compact count, not materialized nodes)", out.NodeKindCounts["column"], columnCount)
	}
}

// TestDatabaseOverviewBoundsHighCardinalityTables proves a namespace with an
// unusually large number of tables stays bounded and reports truncation
// instead of loading everything.
func TestDatabaseOverviewBoundsHighCardinalityTables(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	const namespace = "manytables"

	const tableCount = maxDatabaseOverviewTables + 50
	for i := 0; i < tableCount; i++ {
		id := fmt.Sprintf("%s|table:public.t%d", namespace, i)
		if _, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{
			ID: id, Namespace: namespace, Kind: "table", Name: fmt.Sprintf("public.t%d", i),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.RefreshGraphDegrees(ctx, namespace); err != nil {
		t.Fatal(err)
	}

	q := NewQuerier(store, namespace)
	out, err := q.DatabaseOverview(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Truncated {
		t.Fatal("expected Truncated=true for a table count beyond the architecture-level bound")
	}
	if len(out.Nodes) > maxDatabaseOverviewTables {
		t.Fatalf("got %d nodes, want <= %d (maxDatabaseOverviewTables)", len(out.Nodes), maxDatabaseOverviewTables)
	}
}

// TestScopedExportKindsTableDrillDownIncludesColumns proves that drilling
// into a table (the click-to-explore / focus-subgraph flow scoped to
// DatabaseNodeKinds) surfaces its columns and its table-to-table database
// relationships, not just the table node itself.
func TestScopedExportKindsTableDrillDownIncludesColumns(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	_, users, orders, usersID, ordersUserID := buildDatabaseGraph(t, store, "drilldemo")
	q := NewQuerier(store, "drilldemo")

	out, err := q.ScopedExportKinds(ctx, []semanticmemory.GraphNode{orders}, 1, DatabaseNodeKinds)
	if err != nil {
		t.Fatal(err)
	}
	present := make(map[string]bool, len(out.Nodes))
	for _, n := range out.Nodes {
		present[n.ID] = true
	}
	for _, want := range []semanticmemory.GraphNode{orders, ordersUserID, users} {
		if !present[LocalID(want.ID)] {
			t.Fatalf("table drill-down missing %s: nodes=%v", want.Name, out.Nodes)
		}
	}
	if present[LocalID(usersID.ID)] {
		t.Fatal("depth-1 drill-down from orders should not reach users.id without visiting users first")
	}
	if out.Truncated {
		t.Fatal("unexpectedly truncated for a small graph")
	}
}

// TestScopedExportKindsExcludesUnrelatedKindsWithoutFalseTruncation proves
// that a node dropped only because its kind is outside the allow-list is
// treated as an intentional scope restriction, not budget truncation — so a
// database-only drill-down whose focus happens to sit next to a code node
// reports Truncated=false rather than telling the user the view is partial.
func TestScopedExportKindsExcludesUnrelatedKindsWithoutFalseTruncation(t *testing.T) {
	ctx := context.Background()
	store := openTestStore(t)
	_, users, _, usersID, _ := buildDatabaseGraph(t, store, "mixeddemo")

	// A hypothetical cross-domain link from a table to a code node, which the
	// current extractors never produce but the filtering logic must still
	// handle correctly if one ever exists.
	crossLink, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{
		ID: "mixeddemo|type:UsersRepo", Namespace: "mixeddemo", Kind: semanticmemory.GraphNodeType, Name: "UsersRepo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGraphEdge(ctx, semanticmemory.GraphEdge{
		ID: "mixeddemo|e-cross", Namespace: "mixeddemo", SourceID: users.ID, TargetID: crossLink.ID,
		Kind: semanticmemory.GraphEdgeUses, Confidence: semanticmemory.GraphConfidenceExtracted,
	}); err != nil {
		t.Fatal(err)
	}

	q := NewQuerier(store, "mixeddemo")
	out, err := q.ScopedExportKinds(ctx, []semanticmemory.GraphNode{users}, 1, DatabaseNodeKinds)
	if err != nil {
		t.Fatal(err)
	}
	if out.Truncated {
		t.Fatal("a kind-filtered node must not be reported as truncation")
	}
	for _, n := range out.Nodes {
		if n.ID == LocalID(crossLink.ID) {
			t.Fatal("code-kind node leaked into a DatabaseNodeKinds-scoped export")
		}
	}
	present := make(map[string]bool, len(out.Nodes))
	for _, n := range out.Nodes {
		present[n.ID] = true
	}
	if !present[LocalID(usersID.ID)] {
		t.Fatal("expected users.id column to still be present alongside the filtered-out cross-domain node")
	}
}
