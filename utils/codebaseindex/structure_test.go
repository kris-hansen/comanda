package codebaseindex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeStructureFixture(t *testing.T, root, name, content string) {
	t.Helper()
	file := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestStructureMigrationAndIncrementalRetention(t *testing.T) {
	root := t.TempDir()
	writeStructureFixture(t, root, "go.mod", "module example.org/project\ngo 1.25\n")
	writeStructureFixture(t, root, "main.go", "package main\nfunc main() {}\n")
	writeStructureFixture(t, root, "store/store.go", "package store\n// Store keeps records.\ntype Store struct { Value string }\n")
	cfg := DefaultConfig()
	cfg.Root = root
	cfg.MaxFiles = 1
	m, err := NewManager(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := m.Generate()
	if err != nil {
		t.Fatal(err)
	}
	meta, err := loadMetadata(initial.OutputPath + ".meta.json")
	if err != nil {
		t.Fatal(err)
	}
	if initial.FileCount != 1 || len(meta.Files) != 3 {
		t.Fatalf("markdown count %d, structural count %d", initial.FileCount, len(meta.Files))
	}
	// Simulate metadata written by an older binary. No source file changes.
	meta.Version = 0
	for i := range meta.Files {
		meta.Files[i].Symbols = nil
	}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(initial.OutputPath+".meta.json", data, 0644); err != nil {
		t.Fatal(err)
	}
	upgraded, incremental, err := m.GenerateIncremental(initial.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !upgraded.Updated || incremental {
		t.Fatal("legacy metadata must trigger a full upgrade")
	}
	cache := LoadIndexSymbolCache(initial.OutputPath, root)
	if cache["store/store.go"].Symbols == nil {
		t.Fatal("upgrade omitted symbols outside markdown limit")
	}
	writeStructureFixture(t, root, "main.go", "package main\nfunc main() {}\nfunc Run() {}\n")
	updated, incremental, err := m.GenerateIncremental(initial.OutputPath)
	if err != nil || !updated.Updated || !incremental {
		t.Fatalf("update: %v, %v", updated, err)
	}
	cache = LoadIndexSymbolCache(initial.OutputPath, root)
	if len(cache["store/store.go"].Symbols.Types) != 1 {
		t.Fatal("unchanged type lost")
	}
	if len(cache["main.go"].Symbols.Functions) != 2 {
		t.Fatal("changed functions not refreshed")
	}
	if err := os.Remove(filepath.Join(root, "store/store.go")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.GenerateIncremental(initial.OutputPath); err != nil {
		t.Fatal(err)
	}
	if _, exists := LoadIndexSymbolCache(initial.OutputPath, root)["store/store.go"]; exists {
		t.Fatal("deleted file retained")
	}
	cfg.MaxFiles = 0
	unlimited, _, err := m.GenerateIncremental(initial.OutputPath)
	if err != nil || !unlimited.Updated || unlimited.FileCount != 2 {
		t.Fatalf("markdown limit change was not applied: %+v, %v", unlimited, err)
	}
}

func TestCompleteGoStructureAndNestedModules(t *testing.T) {
	root := t.TempDir()
	writeStructureFixture(t, root, "go.mod", "module example.org/project\ngo 1.25\n")
	writeStructureFixture(t, root, "pkg/store.go", "package store\nimport \"io\"\n// Store keeps data.\ntype Store[T any] struct { Reader io.Reader; Value T }\n"+strings.Repeat("// padding\n", 4000)+"\n// Read fetches data.\nfunc (s *Store[T]) Read(p []byte) (int, error) { return 0, nil }\n")
	writeStructureFixture(t, root, "nested/go.mod", "module example.org/nested\ngo 1.25\n")
	writeStructureFixture(t, root, "nested/store.go", "package store\ntype Store struct {}\n")
	cfg := DefaultConfig()
	cfg.Root = root
	m, err := NewManager(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	scan, _, err := m.Scan()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range scan.GraphFiles {
		switch f.Path {
		case "pkg/store.go":
			if f.PackagePath != "example.org/project/pkg" {
				t.Fatal(f.PackagePath)
			}
			if len(f.Symbols.Functions) != 1 {
				t.Fatal("function after 32KB missing")
			}
			fn := f.Symbols.Functions[0]
			if fn.Receiver != "Store" || !strings.Contains(fn.Signature, "(p []byte) (int, error)") || fn.Comments != "Read fetches data." {
				t.Fatalf("incomplete method: %+v", fn)
			}
			if f.Symbols.Types[0].Fields[0] != "Reader io.Reader" {
				t.Fatal(f.Symbols.Types)
			}
		case "nested/store.go":
			if f.PackagePath != "example.org/nested" {
				t.Fatal(f.PackagePath)
			}
		}
	}
}

func TestEncryptedStructureUpgrade(t *testing.T) {
	root := t.TempDir()
	writeStructureFixture(t, root, "go.mod", "module example.org/project\ngo 1.25\n")
	writeStructureFixture(t, root, "main.go", "package main\ntype PrivateSourceType struct {}\n")
	cfg := DefaultConfig()
	cfg.Root = root
	cfg.Encrypt = true
	cfg.EncryptionKey = "test-key"
	m, err := NewManager(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	result, err := m.Generate()
	if err != nil {
		t.Fatal(err)
	}
	cfg.OutputPath = result.OutputPath
	if err := os.Remove(result.OutputPath + ".meta.json"); err != nil {
		t.Fatal(err)
	}
	updated, _, err := m.GenerateIncremental(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if updated.OutputPath != result.OutputPath {
		t.Fatalf("encrypted path changed: %s", updated.OutputPath)
	}
	data, err := os.ReadFile(updated.OutputPath + ".meta.json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "PrivateSourceType") {
		t.Fatal("plaintext metadata leaked symbols")
	}
	if _, err := DecryptFromFile(updated.OutputPath, cfg.EncryptionKey); err != nil {
		t.Fatal(err)
	}
}

func TestFileHashIncludesTail(t *testing.T) {
	root := t.TempDir()
	content := strings.Repeat("x", 1024*1024)
	writeStructureFixture(t, root, "large.go", content+"a")
	m := &Manager{config: DefaultConfig()}
	first := m.computeFileHash(filepath.Join(root, "large.go"))
	writeStructureFixture(t, root, "large.go", content+"b")
	if first == m.computeFileHash(filepath.Join(root, "large.go")) {
		t.Fatal("tail change was missed")
	}
}
