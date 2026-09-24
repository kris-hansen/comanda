package codebaseindex

import (
	"strings"
	"testing"
)

func TestManagerScanAutoDetectsPostgresSchemaWithoutManifest(t *testing.T) {
	root := t.TempDir()
	writeStructureFixture(t, root, "db/schema.sql", `
CREATE SCHEMA IF NOT EXISTS app;

CREATE TABLE app.users (
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE
);

CREATE TABLE app.orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES app.users(id),
    CONSTRAINT fk_explicit FOREIGN KEY (user_id) REFERENCES app.users(id)
);
`)
	writeStructureFixture(t, root, "db/seed.sql", "INSERT INTO app.users (email) VALUES ('a@example.com');\n")

	cfg := DefaultConfig()
	cfg.Root = root
	m, err := NewManager(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	scan, languages, err := m.Scan()
	if err != nil {
		t.Fatal(err)
	}
	foundLang := false
	for _, l := range languages {
		if l == "postgresql" {
			foundLang = true
		}
	}
	if !foundLang {
		t.Fatalf("expected postgresql adapter to be auto-detected, got %v", languages)
	}

	var schemaFile *FileEntry
	for _, f := range scan.GraphFiles {
		if f.Path == "db/schema.sql" {
			schemaFile = f
		}
	}
	if schemaFile == nil {
		t.Fatal("schema.sql missing from scan")
	}
	if schemaFile.Symbols == nil || schemaFile.Symbols.SemanticGraph == nil {
		t.Fatal("expected a semantic graph extracted from schema.sql without any parser plugin manifest")
	}
	if err := ValidateSemanticGraph(schemaFile.Symbols); err != nil {
		t.Fatalf("invalid semantic graph: %v", err)
	}
}

func findEntity(t *testing.T, g *SemanticGraph, id string) SemanticEntity {
	t.Helper()
	for _, e := range g.Entities {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("entity %q not found among %d entities", id, len(g.Entities))
	return SemanticEntity{}
}

func findRelation(g *SemanticGraph, source, target, kind string) *SemanticRelation {
	for i := range g.Relations {
		r := &g.Relations[i]
		if r.Source == source && r.Target == target && r.Kind == kind {
			return r
		}
	}
	return nil
}

func mustExtract(t *testing.T, path, sql string) *SymbolInfo {
	t.Helper()
	info, err := extractPostgresSymbols(path, []byte(sql))
	if err != nil {
		t.Fatalf("extractPostgresSymbols returned an error: %v", err)
	}
	if info.SemanticGraph != nil {
		if err := ValidateSemanticGraph(info); err != nil {
			t.Fatalf("invalid semantic graph: %v\n%+v", err, info.SemanticGraph)
		}
	}
	return info
}

func TestPostgresBasicTableColumnsAndPrimaryKey(t *testing.T) {
	sql := `
CREATE TABLE public.users (
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT now(),
    nickname TEXT
);
`
	info := mustExtract(t, "schema.sql", sql)
	g := info.SemanticGraph
	if g == nil {
		t.Fatal("expected a semantic graph")
	}
	table := findEntity(t, g, "table:public.users")
	if table.Name != "public.users" || table.Kind != "table" {
		t.Fatalf("unexpected table entity: %+v", table)
	}
	id := findEntity(t, g, "column:public.users.id")
	if id.Kind != "column" {
		t.Fatalf("unexpected column entity: %+v", id)
	}
	email := findEntity(t, g, "column:public.users.email")
	if !strings.Contains(email.Summary, "varchar(255)") || !strings.Contains(email.Summary, "not null") {
		t.Fatalf("expected type/nullability in summary: %+v", email)
	}
	createdAt := findEntity(t, g, "column:public.users.created_at")
	if !strings.Contains(createdAt.Summary, "default now()") {
		t.Fatalf("expected default in summary: %+v", createdAt)
	}
	if findRelation(g, "table:public.users", "column:public.users.id", "primary_key") == nil {
		t.Fatal("missing inline primary key relation")
	}
	if findRelation(g, "table:public.users", "column:public.users.email", "unique") == nil {
		t.Fatal("missing inline unique relation")
	}
	for _, col := range []string{"id", "email", "created_at", "nickname"} {
		if findRelation(g, "table:public.users", "column:public.users."+col, "contains") == nil {
			t.Fatalf("missing contains relation for column %s", col)
		}
	}
}

func TestPostgresExplicitSchemaContainsTable(t *testing.T) {
	sql := `
CREATE SCHEMA IF NOT EXISTS app;
CREATE TABLE app.accounts (id INT PRIMARY KEY);
`
	info := mustExtract(t, "schema.sql", sql)
	g := info.SemanticGraph
	if g == nil {
		t.Fatal("expected a semantic graph")
	}
	findEntity(t, g, "schema:app")
	if findRelation(g, "schema:app", "table:app.accounts", "contains") == nil {
		t.Fatal("missing schema contains table relation")
	}
}

func TestPostgresNoSchemaEntityWithoutExplicitQualification(t *testing.T) {
	sql := `CREATE TABLE widgets (id INT PRIMARY KEY);`
	info := mustExtract(t, "schema.sql", sql)
	g := info.SemanticGraph
	if g == nil {
		t.Fatal("expected a semantic graph")
	}
	for _, e := range g.Entities {
		if e.Kind == "schema" {
			t.Fatalf("did not expect a schema entity for an unqualified table: %+v", e)
		}
	}
	findEntity(t, g, "table:public.widgets")
}

func TestPostgresInlineForeignKey(t *testing.T) {
	sql := `
CREATE TABLE public.orders (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES public.users(id) ON DELETE CASCADE
);
`
	info := mustExtract(t, "orders.sql", sql)
	g := info.SemanticGraph
	if g == nil {
		t.Fatal("expected a semantic graph")
	}
	if findRelation(g, "table:public.orders", "table:public.users", "foreign_key") == nil {
		t.Fatal("missing table-level foreign key relation")
	}
	if findRelation(g, "column:public.orders.user_id", "column:public.users.id", "references") == nil {
		t.Fatal("missing column-level references relation")
	}
	// The referenced table is not defined in this file; it must still resolve
	// safely as a lightweight placeholder rather than being dropped or erroring.
	target := findEntity(t, g, "table:public.users")
	if target.Name != "public.users" {
		t.Fatalf("unexpected placeholder table entity: %+v", target)
	}
}

func TestPostgresTableLevelConstraintsCompositeKeys(t *testing.T) {
	sql := `
CREATE TABLE public.order_items (
    order_id BIGINT NOT NULL,
    product_id BIGINT NOT NULL,
    quantity INT NOT NULL DEFAULT 1,
    CONSTRAINT pk_order_items PRIMARY KEY (order_id, product_id),
    CONSTRAINT fk_order FOREIGN KEY (order_id) REFERENCES public.orders (id),
    CONSTRAINT uq_product UNIQUE (product_id)
);
`
	info := mustExtract(t, "order_items.sql", sql)
	g := info.SemanticGraph
	if g == nil {
		t.Fatal("expected a semantic graph")
	}
	if findRelation(g, "table:public.order_items", "column:public.order_items.order_id", "primary_key") == nil {
		t.Fatal("missing composite primary key column order_id")
	}
	if findRelation(g, "table:public.order_items", "column:public.order_items.product_id", "primary_key") == nil {
		t.Fatal("missing composite primary key column product_id")
	}
	if findRelation(g, "table:public.order_items", "table:public.orders", "foreign_key") == nil {
		t.Fatal("missing table-level constraint foreign key relation")
	}
	if findRelation(g, "table:public.order_items", "column:public.order_items.product_id", "unique") == nil {
		t.Fatal("missing unique constraint relation")
	}
}

func TestPostgresAlterTableAddConstraintForeignKey(t *testing.T) {
	sql := `ALTER TABLE ONLY public.orders ADD CONSTRAINT fk_orders_user FOREIGN KEY (user_id) REFERENCES public.users (id);`
	info := mustExtract(t, "0002_add_fk.sql", sql)
	g := info.SemanticGraph
	if g == nil {
		t.Fatal("expected a semantic graph")
	}
	if findRelation(g, "table:public.orders", "table:public.users", "foreign_key") == nil {
		t.Fatal("missing ALTER TABLE foreign key relation")
	}
	if findRelation(g, "column:public.orders.user_id", "column:public.users.id", "references") == nil {
		t.Fatal("missing ALTER TABLE column-level references relation")
	}
}

func TestPostgresAlterTableAddPrimaryKeyAndUnique(t *testing.T) {
	sql := `
ALTER TABLE public.widgets ADD CONSTRAINT widgets_pkey PRIMARY KEY (id);
ALTER TABLE public.widgets ADD CONSTRAINT widgets_sku_key UNIQUE (sku);
`
	info := mustExtract(t, "0003_keys.sql", sql)
	g := info.SemanticGraph
	if g == nil {
		t.Fatal("expected a semantic graph")
	}
	if findRelation(g, "table:public.widgets", "column:public.widgets.id", "primary_key") == nil {
		t.Fatal("missing ALTER TABLE primary key relation")
	}
	if findRelation(g, "table:public.widgets", "column:public.widgets.sku", "unique") == nil {
		t.Fatal("missing ALTER TABLE unique relation")
	}
}

func TestPostgresQuotedIdentifiersCaseSensitive(t *testing.T) {
	sql := `CREATE TABLE "Public"."Users" ("Id" INT PRIMARY KEY, "Email" TEXT);`
	info := mustExtract(t, "quoted.sql", sql)
	g := info.SemanticGraph
	if g == nil {
		t.Fatal("expected a semantic graph")
	}
	table := findEntity(t, g, "table:Public.Users")
	if table.Name != "Public.Users" {
		t.Fatalf("expected case-preserved qualified name, got %+v", table)
	}
	findEntity(t, g, "column:Public.Users.Id")
	findEntity(t, g, "column:Public.Users.Email")
}

func TestPostgresCommentsAndDefaultsIgnoredSafely(t *testing.T) {
	sql := `
-- users table
/* block comment
   spanning lines */
CREATE TABLE public.users (
    id INT PRIMARY KEY, -- trailing comment
    status TEXT DEFAULT 'active', /* inline */
    metadata JSONB DEFAULT '{}'::jsonb
);
`
	info := mustExtract(t, "commented.sql", sql)
	g := info.SemanticGraph
	if g == nil {
		t.Fatal("expected a semantic graph")
	}
	status := findEntity(t, g, "column:public.users.status")
	if !strings.Contains(status.Summary, "default 'active'") {
		t.Fatalf("expected default value preserved: %+v", status)
	}
	metadata := findEntity(t, g, "column:public.users.metadata")
	if !strings.Contains(metadata.Summary, "jsonb") {
		t.Fatalf("expected jsonb default expression preserved: %+v", metadata)
	}
}

func TestPostgresIrrelevantAndMalformedSQLNeverErrors(t *testing.T) {
	cases := []string{
		`SELECT * FROM users WHERE id = 1;`,
		`INSERT INTO users (id, email) VALUES (1, 'a@example.com');`,
		`CREATE INDEX idx_users_email ON users (email);`,
		`CREATE VIEW active_users AS SELECT * FROM users WHERE active;`,
		`CREATE OR REPLACE FUNCTION add(a int, b int) RETURNS int AS $$ BEGIN RETURN a + b; END; $$ LANGUAGE plpgsql;`,
		`CREATE TABLE`,             // truncated
		`CREATE TABLE foo (`,       // unterminated paren
		`CREATE TABLE foo (id INT`, // unterminated, no closing paren or semicolon
		`ALTER TABLE foo ADD COLUMN bar INT;`,
		`-- just a comment, no statements`,
		`;;;`,
		`GRANT SELECT ON ALL TABLES IN SCHEMA public TO readonly;`,
	}
	for _, sql := range cases {
		if _, err := extractPostgresSymbols("mixed.sql", []byte(sql)); err != nil {
			t.Fatalf("extractPostgresSymbols must never error, got %v for input %q", err, sql)
		}
	}
}

func TestPostgresAdapterDetectionAndScoring(t *testing.T) {
	a := &PostgresAdapter{}
	if a.Name() != "postgresql" {
		t.Fatalf("unexpected name %q", a.Name())
	}
	found := false
	for _, ext := range a.FileExtensions() {
		if ext == ".sql" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected .sql extension support")
	}
}
