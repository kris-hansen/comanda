package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/semanticmemory"
)

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
