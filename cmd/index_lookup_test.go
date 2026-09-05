package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
)

func TestFindIndexFromSubdirectories(t *testing.T) {
	workspace := t.TempDir()
	root := filepath.Join(workspace, "pyrogy-popper")
	nested := filepath.Join(root, "packages", "nested")
	for _, dir := range []string{filepath.Join(root, ".comanda"), filepath.Join(nested, "src"), root + "-other"} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	previous := envConfig
	t.Cleanup(func() { envConfig = previous })
	envConfig = &config.EnvConfig{Indexes: map[string]*config.IndexEntry{
		"pyrogy_popper": {Path: root}, "nested": {Path: nested},
		"stale": {Path: filepath.Join(workspace, "missing")}, "nil": nil,
	}}
	for _, tc := range []struct{ name, dir, want string }{
		{"root", root, "pyrogy_popper"},
		{"metadata directory", filepath.Join(root, ".comanda"), "pyrogy_popper"},
		{"deep subdirectory", filepath.Join(root, "packages"), "pyrogy_popper"},
		{"nested project", nested, "nested"},
		{"nested subdirectory", filepath.Join(nested, "src"), "nested"},
		{"sibling prefix", root + "-other", ""},
		{"outside project", workspace, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(tc.dir)
			entry, name, err := findIndex("")
			if tc.want == "" {
				if err == nil || !strings.Contains(err.Error(), "no registered index contains current directory") {
					t.Fatalf("unexpected lookup: %q, %v", name, err)
				}
				return
			}
			if err != nil || name != tc.want || entry != envConfig.Indexes[tc.want] {
				t.Fatalf("lookup = %q, %v; want %q", name, err, tc.want)
			}
		})
	}
	t.Run("explicit name overrides nearest project", func(t *testing.T) {
		t.Chdir(nested)
		_, name, err := findIndex("pyrogy_popper")
		if err != nil || name != "pyrogy_popper" {
			t.Fatalf("explicit lookup = %q, %v", name, err)
		}
	})
	t.Run("ambiguous root", func(t *testing.T) {
		t.Chdir(nested)
		envConfig.Indexes["nested_copy"] = &config.IndexEntry{Path: nested}
		defer delete(envConfig.Indexes, "nested_copy")
		_, _, err := findIndex("")
		if err == nil || !strings.Contains(err.Error(), "nested, nested_copy") {
			t.Fatalf("ambiguous lookup = %v", err)
		}
	})
}

func TestFindIndexThroughRegisteredSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires extra privileges")
	}
	workspace := t.TempDir()
	root, alias := filepath.Join(workspace, "project"), filepath.Join(workspace, "alias")
	if err := os.MkdirAll(filepath.Join(root, ".comanda"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, alias); err != nil {
		t.Fatal(err)
	}
	previous := envConfig
	t.Cleanup(func() { envConfig = previous })
	envConfig = &config.EnvConfig{Indexes: map[string]*config.IndexEntry{"project": {Path: alias}}}
	t.Chdir(filepath.Join(root, ".comanda"))
	_, name, err := findIndex("")
	if err != nil || name != "project" {
		t.Fatalf("symlink lookup = %q, %v", name, err)
	}
}
