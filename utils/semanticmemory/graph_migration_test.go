package semanticmemory

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestGraphContentFTSMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.UpsertGraphNode(context.Background(), GraphNode{
		ID: "proj|section:guide.md#install", Namespace: "proj", Kind: "section",
		Name: "Install", Path: "guide.md", Content: "unique migration retrieval passage",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`DROP TABLE graph_nodes_fts`)
	if err == nil {
		_, err = db.Exec(`CREATE VIRTUAL TABLE graph_nodes_fts USING fts5(
            id UNINDEXED, namespace UNINDEXED, name, summary, tokenize='unicode61')`)
	}
	if err == nil {
		_, err = db.Exec(`INSERT INTO graph_nodes_fts(id, namespace, name, summary)
            SELECT id, namespace, name, summary FROM graph_nodes`)
	}
	if closeErr := db.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	matches, err := store.GraphFindNodes(context.Background(), "proj", "migration retrieval", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || matches[0].Content != "unique migration retrieval passage" {
		t.Fatalf("content was not backfilled into migrated FTS: %#v", matches)
	}
}
