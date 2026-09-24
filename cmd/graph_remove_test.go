package cmd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/semanticmemory"
)

func resetGraphRemoveGlobals(t *testing.T) {
	t.Helper()
	previousConfig, previousNamespace, previousDB := envConfig, graphNamespace, graphDBPath
	previousEnhance, previousPlugins := graphEnhance, indexParserPlugins
	t.Cleanup(func() {
		envConfig, graphNamespace, graphDBPath = previousConfig, previousNamespace, previousDB
		graphEnhance, indexParserPlugins = previousEnhance, previousPlugins
	})
	graphNamespace, graphDBPath, graphEnhance, indexParserPlugins = "", "", false, nil
}

// TestGraphRemoveResolutionMatchesBuildPrecedence exercises the same
// positional-name / --namespace / nearest-project precedence as
// graph build and graph update, and confirms the index stays registered and
// the graph can be rebuilt afterward.
func TestGraphRemoveResolutionMatchesBuildPrecedence(t *testing.T) {
	resetGraphRemoveGlobals(t)
	root, other := t.TempDir(), t.TempDir()
	for _, dir := range []string{root, other} {
		if err := os.MkdirAll(filepath.Join(dir, ".comanda"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\ntype Store struct {}\nfunc main() {}\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	envConfig = &config.EnvConfig{Indexes: map[string]*config.IndexEntry{
		"project": {Path: root}, "other": {Path: other},
	}}
	t.Chdir(filepath.Join(root, ".comanda"))

	for _, tc := range []struct {
		name, namespace, want string
		args                  []string
	}{
		{name: "nearest project from subdirectory", want: "project"},
		{name: "namespace override", namespace: "other", want: "other"},
		{name: "explicit name overrides namespace", namespace: "other", args: []string{"project"}, want: "project"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Rebuild before every case so removal always has a graph to delete.
			graphNamespace = ""
			if err := graphBuildCmd.RunE(graphBuildCmd, []string{tc.want}); err != nil {
				t.Fatal(err)
			}

			graphNamespace = tc.namespace
			if err := graphRemoveCmd.ValidateArgs(tc.args); err != nil {
				t.Fatal(err)
			}
			if err := graphRemoveCmd.RunE(graphRemoveCmd, tc.args); err != nil {
				t.Fatal(err)
			}

			if _, ok := envConfig.Indexes[tc.want]; !ok {
				t.Fatalf("index %q was unregistered by graph remove", tc.want)
			}

			q, closeStore, err := openGraphQuerierFor(tc.want, "")
			if err != nil {
				t.Fatalf("graph database for %q was deleted by remove (expected only the namespace to be cleared): %v", tc.want, err)
			}
			defer closeStore()
			if _, err := q.Resolve(context.Background(), "Store"); err == nil {
				t.Fatalf("namespace %q still resolves nodes after remove", tc.want)
			}

			// Removal via graph remove must not re-invoke build itself: a
			// second remove of the same, now-empty, namespace must report
			// the same clear "no graph" error as an index that never had one.
			if err := graphRemoveCmd.RunE(graphRemoveCmd, tc.args); err == nil {
				t.Fatalf("second remove of %q succeeded; namespace should already be gone", tc.want)
			} else if !strings.Contains(err.Error(), "comanda graph build "+tc.want) {
				t.Fatalf("unexpected error on repeat remove: %v", err)
			}
		})
	}

	if err := graphRemoveCmd.ValidateArgs([]string{"one", "two"}); err == nil {
		t.Fatal("graph remove accepted extra positional arguments")
	}
}

// TestGraphRemoveCustomDatabasePreservesUnrelatedNamespaceAndMemory builds two
// namespaces plus an ordinary (non-graph) memory record in one custom
// database and confirms remove only touches the selected namespace's graph
// data.
func TestGraphRemoveCustomDatabasePreservesUnrelatedNamespaceAndMemory(t *testing.T) {
	resetGraphRemoveGlobals(t)
	root := t.TempDir()
	dbPath := filepath.Join(root, "custom.db")

	store, err := semanticmemory.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, ns := range []string{"target", "keep"} {
		if _, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{
			ID: ns + "|Store", Namespace: ns, Name: "Store", Kind: "type",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.UpsertGraphAnnotation(ctx, semanticmemory.GraphAnnotation{
		ID: "note-1", Namespace: "target", NodeID: "target|Store", Content: "handle with care",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Upsert(ctx, semanticmemory.Record{
		ID: "durable-1", Namespace: "target", Type: "note", Content: "unrelated durable memory",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	envConfig = &config.EnvConfig{Indexes: map[string]*config.IndexEntry{
		"target": {Path: t.TempDir()},
	}}
	graphNamespace = ""
	graphDBPath = dbPath

	if err := graphRemoveCmd.RunE(graphRemoveCmd, []string{"target"}); err != nil {
		t.Fatal(err)
	}

	store, err = semanticmemory.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	targetNodes, err := store.GraphNodes(ctx, "target")
	if err != nil {
		t.Fatal(err)
	}
	if len(targetNodes) != 0 {
		t.Fatalf("target graph nodes after remove = %d, want 0", len(targetNodes))
	}
	targetAnnotations, err := store.GraphAnnotations(ctx, "target", "target|Store")
	if err != nil {
		t.Fatal(err)
	}
	if len(targetAnnotations) != 0 {
		t.Fatalf("target annotations after remove = %d, want 0", len(targetAnnotations))
	}

	keepNodes, err := store.GraphNodes(ctx, "keep")
	if err != nil {
		t.Fatal(err)
	}
	if len(keepNodes) != 1 {
		t.Fatalf("unrelated namespace %q graph nodes = %d, want 1 (must survive remove)", "keep", len(keepNodes))
	}

	durable, err := store.Search(ctx, "unrelated durable memory", semanticmemory.SearchOptions{Namespace: "target", Types: []string{"note"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(durable) != 1 {
		t.Fatalf("ordinary durable memory record was deleted by graph remove: got %d records, want 1", len(durable))
	}
}

// TestGraphRemoveMissingGraphReturnsClearError covers a registered index that
// has no graph built yet: no database exists, and remove must not create one.
func TestGraphRemoveMissingGraphReturnsClearError(t *testing.T) {
	resetGraphRemoveGlobals(t)
	root := t.TempDir()
	envConfig = &config.EnvConfig{Indexes: map[string]*config.IndexEntry{
		"vault": {Path: root},
	}}
	graphNamespace = "vault"
	graphDBPath = ""

	err := graphRemoveCmd.RunE(graphRemoveCmd, nil)
	if err == nil {
		t.Fatal("graph remove succeeded for a namespace with no graph")
	}
	want := `no graph exists for namespace "vault". Build it with: comanda graph build vault`
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".comanda", "memory")); !os.IsNotExist(statErr) {
		t.Fatalf("graph remove created a memory directory for a database that never existed: stat error = %v", statErr)
	}
}

// TestGraphRemoveMissingNamespaceInExistingDatabase covers a database that
// exists (another namespace was built in it) but not the requested one.
func TestGraphRemoveMissingNamespaceInExistingDatabase(t *testing.T) {
	resetGraphRemoveGlobals(t)
	root := t.TempDir()
	dbPath := semanticmemory.DefaultPath(root, "vault")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := semanticmemory.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	envConfig = &config.EnvConfig{Indexes: map[string]*config.IndexEntry{
		"vault": {Path: root},
	}}
	graphNamespace = "vault"
	graphDBPath = ""

	if err := graphRemoveCmd.RunE(graphRemoveCmd, nil); err == nil {
		t.Fatal("graph remove succeeded for an existing database without a built graph")
	}
}

// TestGraphRemoveUnregisteredIndexReturnsClearError confirms remove uses the
// same registered-index lookup as build/update instead of silently no-op'ing.
func TestGraphRemoveUnregisteredIndexReturnsClearError(t *testing.T) {
	resetGraphRemoveGlobals(t)
	envConfig = &config.EnvConfig{Indexes: map[string]*config.IndexEntry{}}

	if err := graphRemoveCmd.RunE(graphRemoveCmd, []string{"missing"}); err == nil {
		t.Fatal("graph remove succeeded for an unregistered index")
	}
}

// TestGraphRemoveCommandRegisteredWithHelp confirms the command is wired into
// the graph command tree with usable help text and example content.
func TestGraphRemoveCommandRegisteredWithHelp(t *testing.T) {
	found := false
	for _, sub := range graphCmd.Commands() {
		if sub == graphRemoveCmd {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("graph remove is not registered under the graph command")
	}
	if graphRemoveCmd.Use != "remove [index-name]" {
		t.Fatalf("Use = %q", graphRemoveCmd.Use)
	}
	if graphRemoveCmd.Short == "" || graphRemoveCmd.Long == "" || graphRemoveCmd.Example == "" {
		t.Fatal("graph remove is missing help text")
	}
	if !strings.Contains(graphRemoveCmd.Example, "comanda graph remove") {
		t.Fatalf("Example does not reference the command: %q", graphRemoveCmd.Example)
	}
	if !strings.Contains(strings.ToLower(graphCmd.Long), "graph remove") {
		t.Fatal("top-level graph help does not mention graph remove")
	}
}
