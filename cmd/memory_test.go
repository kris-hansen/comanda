package cmd

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestSplitMemoryTypes(t *testing.T) {
	got := splitMemoryTypes(" decision, failure ,, finding ")
	want := []string{"decision", "failure", "finding"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}

func TestOpenMemoryStoreUsesExplicitPath(t *testing.T) {
	originalPath, originalNamespace := memoryDBPath, memoryNamespace
	t.Cleanup(func() {
		memoryDBPath, memoryNamespace = originalPath, originalNamespace
	})
	memoryDBPath = filepath.Join(t.TempDir(), "memory.db")
	memoryNamespace = "repo"
	store, err := openMemoryStore()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
}
