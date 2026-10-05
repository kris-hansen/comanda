package knowledgegraph

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kris-hansen/comanda/utils/codebaseindex"
	"github.com/kris-hansen/comanda/utils/semanticmemory"
)

// postgresFixtureScan mirrors a small multi-migration-file repository: table
// definitions in one file, and a foreign key added against those tables from
// a later migration file, which is the common real-world pattern this
// integration needs to support.
func postgresFixtureScan(t *testing.T) *codebaseindex.ScanResult {
	t.Helper()
	adapter := &codebaseindex.PostgresAdapter{}

	schemaSQL := `
CREATE SCHEMA IF NOT EXISTS app;

CREATE TABLE app.users (
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE
);

CREATE TABLE app.orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES app.users(id),
    total NUMERIC(10,2) NOT NULL DEFAULT 0
);
`
	alterSQL := `ALTER TABLE ONLY app.orders ADD CONSTRAINT fk_orders_user FOREIGN KEY (user_id) REFERENCES app.users (id) ON DELETE CASCADE;`

	makeEntry := func(path, sql string) *codebaseindex.FileEntry {
		symbols, err := adapter.ExtractSymbols(path, []byte(sql))
		if err != nil {
			t.Fatalf("extract %s: %v", path, err)
		}
		return &codebaseindex.FileEntry{Path: path, Language: "postgresql", Symbols: symbols}
	}

	return &codebaseindex.ScanResult{
		Candidates: []*codebaseindex.FileEntry{
			makeEntry("db/0001_init.sql", schemaSQL),
			makeEntry("db/0002_add_fk.sql", alterSQL),
			makeEntry("db/seed.sql", "INSERT INTO app.users (email) VALUES ('a@example.com');"),
		},
	}
}

func TestPostgresSchemaGraphBuild(t *testing.T) {
	scan := postgresFixtureScan(t)
	g := Build(scan, "pgdemo")

	schemaID := NodeID("pgdemo", "semantic:10:postgresql:project:schema:app")
	usersID := NodeID("pgdemo", "semantic:10:postgresql:project:table:app.users")
	ordersID := NodeID("pgdemo", "semantic:10:postgresql:project:table:app.orders")
	userIDCol := NodeID("pgdemo", "semantic:10:postgresql:project:column:app.users.id")
	orderUserIDCol := NodeID("pgdemo", "semantic:10:postgresql:project:column:app.orders.user_id")

	if g.Nodes[schemaID] == nil {
		t.Fatal("missing schema node")
	}
	if g.Nodes[usersID] == nil || g.Nodes[usersID].Kind != "table" {
		t.Fatalf("missing/incorrect users table node: %+v", g.Nodes[usersID])
	}
	if g.Nodes[ordersID] == nil {
		t.Fatal("missing orders table node")
	}

	if g.Edges[EdgeID("pgdemo", schemaID, usersID, "contains")] == nil {
		t.Fatal("missing schema->table contains edge")
	}
	if g.Edges[EdgeID("pgdemo", usersID, userIDCol, "primary_key")] == nil {
		t.Fatal("missing users.id primary_key edge")
	}
	if g.Edges[EdgeID("pgdemo", ordersID, usersID, "foreign_key")] == nil {
		t.Fatal("missing orders->users table-level foreign_key edge (inline)")
	}
	if g.Edges[EdgeID("pgdemo", orderUserIDCol, userIDCol, "references")] == nil {
		t.Fatal("missing orders.user_id->users.id column-level references edge (inline)")
	}

	// The second migration file adds the same relationship again via ALTER
	// TABLE; duplicate declarations across files must collapse rather than
	// producing parallel disconnected nodes.
	tableNodes := 0
	for _, n := range g.Nodes {
		if n.Kind == "table" && n.Name == "app.orders" {
			tableNodes++
		}
	}
	if tableNodes != 1 {
		t.Fatalf("expected exactly one orders table node across files, got %d", tableNodes)
	}

	store, err := semanticmemory.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := RebuildWithProgress(context.Background(), store, g, nil); err != nil {
		t.Fatal(err)
	}
	edges, err := store.GraphEdges(context.Background(), "pgdemo")
	if err != nil {
		t.Fatal(err)
	}
	foundFK := false
	for _, e := range edges {
		if e.Kind == "foreign_key" && e.SourceID == ordersID && e.TargetID == usersID {
			foundFK = true
		}
	}
	if !foundFK {
		t.Fatal("foreign_key relationship lost when persisted to SQLite graph storage")
	}
}

func TestPostgresGraphIgnoresPlainSQLWithoutErrors(t *testing.T) {
	adapter := &codebaseindex.PostgresAdapter{}
	symbols, err := adapter.ExtractSymbols("query.sql", []byte("SELECT 1;"))
	if err != nil {
		t.Fatalf("plain SQL must not error: %v", err)
	}
	scan := &codebaseindex.ScanResult{Candidates: []*codebaseindex.FileEntry{
		{Path: "query.sql", Language: "postgresql", Symbols: symbols},
	}}
	g := Build(scan, "pgdemo2")
	for _, n := range g.Nodes {
		if n.Kind == "table" || n.Kind == "column" || n.Kind == "schema" {
			t.Fatalf("did not expect schema entities from a plain query file: %+v", n)
		}
	}
}

func TestPostgresProjectEntitiesStayWithinMonorepoComponent(t *testing.T) {
	adapter := &codebaseindex.PostgresAdapter{}
	ddl := `CREATE SCHEMA app; CREATE TABLE app.items (id BIGINT PRIMARY KEY);`
	entry := func(filePath string) *codebaseindex.FileEntry {
		symbols, err := adapter.ExtractSymbols(filePath, []byte(ddl))
		if err != nil {
			t.Fatalf("extract %s: %v", filePath, err)
		}
		return &codebaseindex.FileEntry{Path: filePath, Language: "postgresql", Symbols: symbols}
	}
	scan := &codebaseindex.ScanResult{
		IsMonorepo: true,
		Components: []*codebaseindex.CodebaseComponent{
			{Name: "service-a", Root: "services/a", Language: "go", Kind: "backend", FileCount: 2},
			{Name: "service-b", Root: "services/b", Language: "go", Kind: "backend", FileCount: 2},
		},
		Candidates: []*codebaseindex.FileEntry{
			entry("services/a/db/001.sql"),
			entry("services/b/db/001.sql"),
		},
	}
	g := Build(scan, "monorepo")

	componentA := NodeID("monorepo", "component:service-a")
	componentB := NodeID("monorepo", "component:service-b")
	schemaA := NodeID("monorepo", "semantic:10:postgresql:project:10:services/a:schema:app")
	schemaB := NodeID("monorepo", "semantic:10:postgresql:project:10:services/b:schema:app")
	if g.Nodes[schemaA] == nil || g.Nodes[schemaB] == nil || schemaA == schemaB {
		t.Fatalf("project-scoped schemas were not separated by component: a=%+v b=%+v", g.Nodes[schemaA], g.Nodes[schemaB])
	}
	if g.Edges[EdgeID("monorepo", componentA, schemaA, EdgeContains)] == nil {
		t.Fatal("service-a component does not contain its schema")
	}
	if g.Edges[EdgeID("monorepo", componentB, schemaB, EdgeContains)] == nil {
		t.Fatal("service-b component does not contain its schema")
	}
	if g.Edges[EdgeID("monorepo", componentA, schemaB, EdgeContains)] != nil {
		t.Fatal("service-a component leaked service-b schema")
	}
	for _, edge := range g.Edges {
		if edge.SourceID == componentA && g.Nodes[edge.TargetID] != nil && g.Nodes[edge.TargetID].Kind == "column" {
			t.Fatal("component linked directly to columns instead of preserving progressive schema/table drill-down")
		}
	}

	store, err := semanticmemory.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := RebuildWithProgress(context.Background(), store, g, nil); err != nil {
		t.Fatal(err)
	}
	focus, err := store.GetGraphNode(context.Background(), componentA)
	if err != nil {
		t.Fatal(err)
	}
	view, err := NewQuerier(store, "monorepo").ScopedExportKinds(context.Background(), []semanticmemory.GraphNode{focus}, 2, DatabaseNodeKinds)
	if err != nil {
		t.Fatal(err)
	}
	present := make(map[string]bool, len(view.Nodes))
	for _, node := range view.Nodes {
		present[node.ID] = true
	}
	if !present[LocalID(schemaA)] {
		t.Fatal("database-only component view did not include its schema")
	}
	if present[LocalID(schemaB)] {
		t.Fatal("database-only component view included another component's schema")
	}
	tableA := LocalID(NodeID("monorepo", "semantic:10:postgresql:project:10:services/a:table:app.items"))
	if !present[tableA] {
		t.Fatal("database-only component view did not include its table at depth 2")
	}
}

// TestPostgresSchemalessTablesLinkToComponent guards the regression behind
// "enter a package in a monorepo, filter by db, see only a table with no
// relationships": DDL without schema qualification produces project-scope
// table entities with no schema containment, and before they were linked to
// their component directly they were unreachable from component-scoped
// database views at any depth.
func TestPostgresSchemalessTablesLinkToComponent(t *testing.T) {
	adapter := &codebaseindex.PostgresAdapter{}
	ddl := `
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL
);

CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id),
    total NUMERIC(10,2) NOT NULL
);
`
	symbols, err := adapter.ExtractSymbols("db/schema.sql", []byte(ddl))
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	scan := &codebaseindex.ScanResult{
		Components: []*codebaseindex.CodebaseComponent{
			{Name: "app", Root: ".", Language: "go", Kind: "backend", FileCount: 3},
		},
		Candidates: []*codebaseindex.FileEntry{
			{Path: "db/schema.sql", Language: "postgresql", Symbols: symbols},
		},
	}
	g := Build(scan, "schemaless")

	componentID := NodeID("schemaless", "component:app")
	usersID := NodeID("schemaless", "semantic:10:postgresql:project:table:public.users")
	ordersID := NodeID("schemaless", "semantic:10:postgresql:project:table:public.orders")
	userIDCol := NodeID("schemaless", "semantic:10:postgresql:project:column:public.users.id")
	orderUserIDCol := NodeID("schemaless", "semantic:10:postgresql:project:column:public.orders.user_id")

	if g.Edges[EdgeID("schemaless", componentID, usersID, EdgeContains)] == nil {
		t.Fatal("component does not contain schema-less users table")
	}
	if g.Edges[EdgeID("schemaless", componentID, ordersID, EdgeContains)] == nil {
		t.Fatal("component does not contain schema-less orders table")
	}
	for _, edge := range g.Edges {
		if edge.SourceID == componentID && g.Nodes[edge.TargetID] != nil && g.Nodes[edge.TargetID].Kind == "column" {
			t.Fatal("component linked directly to columns instead of progressive table drill-down")
		}
	}

	// The viewer's database-only path: component-scoped subgraph must surface
	// both tables, the table-level foreign key, and the column-level
	// references edge (component→table→column needs depth 3).
	store, err := semanticmemory.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := RebuildWithProgress(context.Background(), store, g, nil); err != nil {
		t.Fatal(err)
	}
	focus, err := store.GetGraphNode(context.Background(), componentID)
	if err != nil {
		t.Fatal(err)
	}
	view, err := NewQuerier(store, "schemaless").ScopedExportKinds(context.Background(), []semanticmemory.GraphNode{focus}, 3, DatabaseNodeKinds)
	if err != nil {
		t.Fatal(err)
	}
	present := make(map[string]bool, len(view.Nodes))
	for _, node := range view.Nodes {
		present[node.ID] = true
	}
	if !present[LocalID(usersID)] || !present[LocalID(ordersID)] {
		t.Fatalf("database-only component view missing tables: %v", present)
	}
	foundFK, foundRef := false, false
	for _, e := range view.Edges {
		if e.Kind == "foreign_key" && e.Source == LocalID(ordersID) && e.Target == LocalID(usersID) {
			foundFK = true
		}
		if e.Kind == "references" && e.Source == LocalID(orderUserIDCol) && e.Target == LocalID(userIDCol) {
			foundRef = true
		}
	}
	if !foundFK {
		t.Fatal("database-only component view missing foreign_key edge")
	}
	if !foundRef {
		t.Fatal("database-only component view missing column-level references edge at depth 3")
	}
}

// TestPostgresSchemaQualifiedTablesStayBehindSchema ensures the schema-less
// component linking does not short-circuit the schema drill-down when the DDL
// is schema-qualified: tables stay behind their schema node.
func TestPostgresSchemaQualifiedTablesStayBehindSchema(t *testing.T) {
	scan := postgresFixtureScan(t)
	scan.Components = []*codebaseindex.CodebaseComponent{
		{Name: "app", Root: ".", Language: "go", Kind: "backend", FileCount: 3},
	}
	g := Build(scan, "qualified")

	componentID := NodeID("qualified", "component:app")
	schemaID := NodeID("qualified", "semantic:10:postgresql:project:schema:app")
	usersID := NodeID("qualified", "semantic:10:postgresql:project:table:app.users")
	if g.Edges[EdgeID("qualified", componentID, schemaID, EdgeContains)] == nil {
		t.Fatal("component does not contain its schema")
	}
	if g.Edges[EdgeID("qualified", componentID, usersID, EdgeContains)] != nil {
		t.Fatal("schema-qualified table linked directly to component, bypassing the schema layer")
	}
}
