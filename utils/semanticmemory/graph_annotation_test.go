package semanticmemory

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestGraphAnnotationLayerMigration(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "memory.db")

	// Simulate a pre-layering database: graph_annotations without the
	// source/source_path columns and one legacy human note.
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`CREATE TABLE graph_annotations (
        id TEXT PRIMARY KEY,
        namespace TEXT NOT NULL,
        node_id TEXT NOT NULL,
        content TEXT NOT NULL,
        created_at TEXT NOT NULL,
        updated_at TEXT NOT NULL
    )`); err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	if _, err := raw.Exec(`INSERT INTO graph_annotations (id, namespace, node_id, content, created_at, updated_at)
        VALUES ('legacy-1', 'repo', 'repo|worker', 'Legacy note', ?, ?)`, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// The migrated schema exposes both layer columns.
	rows, err := store.db.QueryContext(ctx, `PRAGMA table_info(graph_annotations)`)
	if err != nil {
		t.Fatal(err)
	}
	columns := make(map[string]bool)
	for rows.Next() {
		var cid, notNull, pk int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		columns[name] = true
	}
	rows.Close()
	for _, column := range []string{"source", "source_path"} {
		if !columns[column] {
			t.Fatalf("graph_annotations missing migrated column %s", column)
		}
	}

	// Legacy rows default to the human layer.
	annotations, err := store.GraphAnnotations(ctx, "repo", "repo|worker")
	if err != nil {
		t.Fatal(err)
	}
	if len(annotations) != 1 || annotations[0].Source != GraphAnnotationHuman || annotations[0].SourcePath != "" {
		t.Fatalf("legacy annotation = %#v, want human layer defaults", annotations)
	}

	// File-layer annotations round-trip with their source path.
	if _, err := store.UpsertGraphAnnotation(ctx, GraphAnnotation{
		ID: "file-1", Namespace: "repo", NodeID: "repo|worker",
		Content: "Run tests before shipping.", Source: GraphAnnotationFile, SourcePath: "AGENTS.md",
	}); err != nil {
		t.Fatal(err)
	}
	annotations, err = store.GraphAnnotations(ctx, "repo", "repo|worker")
	if err != nil {
		t.Fatal(err)
	}
	var fileLayer *GraphAnnotation
	for i := range annotations {
		if annotations[i].ID == "file-1" {
			fileLayer = &annotations[i]
		}
	}
	if fileLayer == nil || fileLayer.Source != GraphAnnotationFile || fileLayer.SourcePath != "AGENTS.md" {
		t.Fatalf("file annotation = %#v, want file/AGENTS.md", fileLayer)
	}

	// Reopening runs the column migration again without error.
	if err := store.migrateGraphAnnotationColumns(ctx); err != nil {
		t.Fatalf("repeated column migration: %v", err)
	}
}

func TestDeleteFileAnnotationsExceptKeepsHumanAndKeepSet(t *testing.T) {
	ctx := context.Background()
	store, err := Open(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	for _, annotation := range []GraphAnnotation{
		{ID: "human-1", Namespace: "repo", NodeID: "repo|worker", Content: "Human note"},
		{ID: "file-keep", Namespace: "repo", NodeID: "repo|worker", Content: "Current doc section", Source: GraphAnnotationFile, SourcePath: "AGENTS.md"},
		{ID: "file-stale", Namespace: "repo", NodeID: "repo|worker", Content: "Removed doc section", Source: GraphAnnotationFile, SourcePath: "AGENTS.md"},
	} {
		if _, err := store.UpsertGraphAnnotation(ctx, annotation); err != nil {
			t.Fatal(err)
		}
	}

	if err := store.DeleteFileAnnotationsExcept(ctx, "repo", []string{"file-keep"}); err != nil {
		t.Fatal(err)
	}

	annotations, err := store.GraphAnnotations(ctx, "repo", "repo|worker")
	if err != nil {
		t.Fatal(err)
	}
	remaining := make(map[string]string)
	for _, a := range annotations {
		remaining[a.ID] = a.Source
	}
	if len(remaining) != 2 || remaining["human-1"] != GraphAnnotationHuman || remaining["file-keep"] != GraphAnnotationFile {
		t.Fatalf("remaining annotations = %#v, want human-1 and file-keep", remaining)
	}

	// The stale file annotation's memory mirror is cleaned up too.
	if _, err := store.Get(ctx, graphAnnotationRecordPrefix+"file-stale"); err == nil {
		t.Fatal("stale file annotation mirror survived cleanup")
	}

	// An empty keep set prunes every file annotation but never human ones.
	if err := store.DeleteFileAnnotationsExcept(ctx, "repo", nil); err != nil {
		t.Fatal(err)
	}
	annotations, err = store.GraphAnnotations(ctx, "repo", "repo|worker")
	if err != nil {
		t.Fatal(err)
	}
	if len(annotations) != 1 || annotations[0].ID != "human-1" {
		t.Fatalf("annotations after full file prune = %#v, want only human-1", annotations)
	}
}
