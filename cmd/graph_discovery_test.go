package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/semanticmemory"
	"github.com/spf13/cobra"
)

func TestGraphBuildAndUpdateInferProject(t *testing.T) {
	previousConfig, previousNamespace, previousDB := envConfig, graphNamespace, graphDBPath
	previousEnhance, previousPlugins := graphEnhance, indexParserPlugins
	t.Cleanup(func() {
		envConfig, graphNamespace, graphDBPath = previousConfig, previousNamespace, previousDB
		graphEnhance, indexParserPlugins = previousEnhance, previousPlugins
	})
	graphNamespace, graphDBPath, graphEnhance, indexParserPlugins = "", "", false, nil
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
		cmd                   *cobra.Command
		args                  []string
	}{
		{name: "build from subdirectory", cmd: graphBuildCmd, want: "project"},
		{name: "update from subdirectory", cmd: graphUpdateCmd, want: "project"},
		{name: "namespace override", cmd: graphBuildCmd, namespace: "other", want: "other"},
		{name: "explicit name overrides namespace", cmd: graphBuildCmd, namespace: "other", args: []string{"project"}, want: "project"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			graphNamespace = tc.namespace
			if err := tc.cmd.ValidateArgs(tc.args); err != nil {
				t.Fatal(err)
			}
			if err := tc.cmd.RunE(tc.cmd, tc.args); err != nil {
				t.Fatal(err)
			}
			q, closeStore, err := openGraphQuerierFor(tc.want, "")
			if err != nil {
				t.Fatal(err)
			}
			defer closeStore()
			if _, err := q.Resolve(context.Background(), "Store"); err != nil {
				t.Fatalf("graph missing built source: %v", err)
			}
		})
	}
	graphNamespace = ""
	q, closeStore, err := openGraphQuerier()
	if err != nil {
		t.Fatalf("query from .comanda: %v", err)
	}
	defer closeStore()
	if _, err := q.Resolve(context.Background(), "Store"); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []*cobra.Command{graphBuildCmd, graphUpdateCmd} {
		if err := cmd.ValidateArgs([]string{"one", "two"}); err == nil {
			t.Fatalf("%s accepted extra arguments", cmd.Name())
		}
	}
}

func TestGraphList(t *testing.T) {
	previousConfig, previousNamespace, previousDB := envConfig, graphNamespace, graphDBPath
	t.Cleanup(func() { envConfig, graphNamespace, graphDBPath = previousConfig, previousNamespace, previousDB })
	graphNamespace, graphDBPath = "", ""
	root := t.TempDir()
	envConfig = &config.EnvConfig{Indexes: map[string]*config.IndexEntry{
		"zeta": {Path: root}, "alpha": {Path: root}, "unbuilt": {Path: root},
		"memory_only": {Path: root}, "invalid": nil,
	}}
	ctx := context.Background()
	for _, name := range []string{"zeta", "alpha", "memory_only"} {
		path := semanticmemory.DefaultPath(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		store, err := semanticmemory.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		if name != "memory_only" {
			if _, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{ID: name, Namespace: name, Name: "Store", Kind: "type"}); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := listGraphs(ctx, "", "")
	if err != nil || len(entries) != 2 {
		t.Fatalf("list = %+v, %v", entries, err)
	}
	if entries[0].Namespace != "alpha" || entries[1].Namespace != "zeta" || entries[0].Nodes != 1 || entries[0].Edges != 0 {
		t.Fatalf("unexpected graph summaries: %+v", entries)
	}
	if _, err := os.Stat(semanticmemory.DefaultPath(root, "unbuilt")); !os.IsNotExist(err) {
		t.Fatalf("listing created an unbuilt graph: %v", err)
	}
	filtered, err := listGraphs(ctx, "zeta", "")
	if err != nil || len(filtered) != 1 || filtered[0].Namespace != "zeta" {
		t.Fatalf("filtered list = %+v, %v", filtered, err)
	}
	custom, err := listGraphs(ctx, "", entries[0].Database)
	if err != nil || len(custom) != 1 || custom[0] != entries[0] {
		t.Fatalf("custom database list = %+v, %v", custom, err)
	}
	if _, err := listGraphs(ctx, "", filepath.Join(root, "missing.db")); err == nil {
		t.Fatal("explicit missing database should report an error")
	}
	badDB := filepath.Join(root, "corrupt.db")
	if err := os.WriteFile(badDB, []byte("not a database"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := listGraphs(ctx, "", badDB); err == nil {
		t.Fatal("corrupt database should report an error")
	}
	for _, asJSON := range []bool{false, true} {
		cmd := &cobra.Command{}
		cmd.SetContext(ctx)
		cmd.Flags().Bool("json", asJSON, "")
		var out bytes.Buffer
		cmd.SetOut(&out)
		if err := runGraphList(cmd, nil); err != nil {
			t.Fatal(err)
		}
		if asJSON {
			var decoded []graphListEntry
			if err := json.Unmarshal(out.Bytes(), &decoded); err != nil || len(decoded) != 2 {
				t.Fatalf("invalid list JSON: %q, %v", out.String(), err)
			}
		} else if !strings.Contains(out.String(), "NAME") || !strings.Contains(out.String(), "alpha") || strings.Contains(out.String(), "unbuilt") {
			t.Fatalf("unexpected table: %s", out.String())
		}
	}
	envConfig = nil
	empty, err := listGraphs(ctx, "", "")
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty list = %+v, %v", empty, err)
	}
	cmd := &cobra.Command{}
	cmd.SetContext(ctx)
	cmd.Flags().Bool("json", false, "")
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := runGraphList(cmd, nil); err != nil || !strings.Contains(out.String(), "No graphs found") {
		t.Fatalf("empty table = %q, %v", out.String(), err)
	}
}
