package semanticmemory

import (
	"context"
	"path/filepath"
	"testing"
)

func TestStoreSearchHonorsNamespaceAndTypes(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	if _, err := store.Upsert(ctx, Record{Namespace: "repo", Type: "decision", Priority: 90, Content: "Use SQLite FTS5 as the local memory retrieval engine."}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Upsert(ctx, Record{Namespace: "repo", Type: "finding", Content: "The existing memory file is injected in full."}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Upsert(ctx, Record{Namespace: "other", Type: "decision", Content: "Use SQLite for unrelated work."}); err != nil {
		t.Fatal(err)
	}

	results, err := store.Search(ctx, "SQLite local retrieval", SearchOptions{Namespace: "repo", Types: []string{"decision"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d records, want 1", len(results))
	}
	if results[0].Type != "decision" || results[0].Namespace != "repo" {
		t.Fatalf("unexpected record: %#v", results[0])
	}
}

func TestStoreUpsertRefreshesSearchIndex(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	record, err := store.Upsert(ctx, Record{ID: "stable-id", Content: "Old storage implementation"})
	if err != nil {
		t.Fatal(err)
	}
	record.Content = "New retrieval implementation"
	if _, err := store.Upsert(ctx, record); err != nil {
		t.Fatal(err)
	}
	oldResults, err := store.Search(ctx, "old storage", SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(oldResults) != 0 {
		t.Fatalf("old FTS entry remained: %#v", oldResults)
	}
	newResults, err := store.Search(ctx, "new retrieval", SearchOptions{})
	if err != nil || len(newResults) != 1 {
		t.Fatalf("got records=%#v err=%v", newResults, err)
	}
}
