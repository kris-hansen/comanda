package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/codebaseindex"
	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/semanticmemory"
)

func TestIndexUpdateRefreshesExistingGraphWithoutSourceChanges(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{"go.mod": "module example.org/project\ngo 1.25\n", "main.go": "package main\ntype Store struct {}\nfunc main() {}\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := codebaseindex.DefaultConfig()
	cfg.Root = root
	m, err := codebaseindex.NewManager(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.Generate()
	if err != nil {
		t.Fatal(err)
	}
	previousConfig, previousGraph, previousFull := envConfig, indexGraph, updateFull
	previousEnhance, previousPlugins, previousDB := indexEnhance, indexParserPlugins, graphDBPath
	t.Cleanup(func() {
		envConfig, indexGraph, updateFull = previousConfig, previousGraph, previousFull
		indexEnhance, indexParserPlugins, graphDBPath = previousEnhance, previousPlugins, previousDB
	})
	indexGraph, updateFull, indexEnhance = false, false, false
	indexParserPlugins, graphDBPath = nil, ""
	envConfig = &config.EnvConfig{Indexes: map[string]*config.IndexEntry{"project": {Path: root, IndexPath: result.OutputPath, Format: "structured"}}}
	if refresh, err := shouldRefreshIndexGraph("project", root, false); err != nil || refresh {
		t.Fatal("must not create a graph implicitly")
	}
	dbPath := semanticmemory.DefaultPath(root, "project")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		t.Fatal(err)
	}
	store, err := semanticmemory.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if refresh, err := shouldRefreshIndexGraph("project", root, false); err != nil || refresh {
		t.Fatal("ordinary memory database should not opt into graphs")
	}
	if _, err := store.UpsertGraphNode(context.Background(), semanticmemory.GraphNode{ID: "project|file:deleted.go", Namespace: "project", Name: "deleted.go", Kind: "file"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if refresh, err := shouldRefreshIndexGraph("project", root, true); err != nil || refresh {
		t.Fatal("encrypted index would create a plaintext graph")
	}
	t.Chdir(filepath.Join(root, ".comanda"))
	if err := runUpdate(updateCmd, nil); err != nil {
		t.Fatal(err)
	}
	querier, closeStore, err := openGraphQuerierFor("project", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer closeStore()
	if _, err := querier.Resolve(context.Background(), "Store"); err != nil {
		t.Fatalf("unchanged update did not refresh graph: %v", err)
	}
	exported, err := querier.Export(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range exported.Nodes {
		if node.ID == "project|file:deleted.go" {
			t.Fatal("graph retained a stale file")
		}
	}
}

func TestOpenGraphQuerierMissingGraphGuidesBuildWithoutCreatingMemoryDirectory(t *testing.T) {
	root := t.TempDir()
	originalConfig, originalNamespace, originalDBPath := envConfig, graphNamespace, graphDBPath
	t.Cleanup(func() {
		envConfig, graphNamespace, graphDBPath = originalConfig, originalNamespace, originalDBPath
	})
	envConfig = &config.EnvConfig{Indexes: map[string]*config.IndexEntry{
		"vault": {Path: root},
	}}
	graphNamespace = "vault"
	graphDBPath = ""

	_, _, err := openGraphQuerier()
	if err == nil {
		t.Fatal("openGraphQuerier() succeeded for a graph that has not been built")
	}
	want := `no graph exists for namespace "vault". Build it with: comanda graph build vault`
	if err.Error() != want {
		t.Fatalf("openGraphQuerier() error = %q, want %q", err, want)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".comanda", "memory")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("graph read created a memory directory: stat error = %v", statErr)
	}
}

func TestOpenGraphQuerierOpensExistingGraphDatabase(t *testing.T) {
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

	originalConfig, originalNamespace, originalDBPath := envConfig, graphNamespace, graphDBPath
	t.Cleanup(func() {
		envConfig, graphNamespace, graphDBPath = originalConfig, originalNamespace, originalDBPath
	})
	envConfig = &config.EnvConfig{Indexes: map[string]*config.IndexEntry{
		"vault": {Path: root},
	}}
	graphNamespace = "vault"
	graphDBPath = ""

	_, closeStore, err := openGraphQuerier()
	if err != nil {
		t.Fatal(err)
	}
	if err := closeStore(); err != nil {
		t.Fatal(err)
	}
}

func TestGraphNotBuiltErrorUsesNamespaceBuildCommand(t *testing.T) {
	if got := graphNotBuiltError("vault").Error(); !strings.Contains(got, "comanda graph build vault") {
		t.Fatalf("graphNotBuiltError() = %q", got)
	}
}
