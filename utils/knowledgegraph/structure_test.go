package knowledgegraph

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kris-hansen/comanda/utils/codebaseindex"
)

func TestBuildCompleteStructureFromScan(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		"go.mod":             "module example.org/project\ngo 1.25\n",
		"main.go":            "package main\nimport (\"example.org/project/a/store\"; \"example.org/third-party/store\")\nfunc main() {}\n",
		"a/store/store.go":   "package store\ntype Store struct {}\n",
		"a/store/methods.go": "package store\nfunc (s *Store) Open() {}\n",
		"b/store/store.go":   "package store\ntype Store struct {}\n",
	} {
		file := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := codebaseindex.DefaultConfig()
	cfg.Root = root
	cfg.MaxFiles = 1
	m, err := codebaseindex.NewManager(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	scan, _, err := m.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(scan.Candidates) != 1 {
		t.Fatal("markdown selection was not capped")
	}
	g := Build(scan, "project")
	for _, local := range []string{"file:b/store/store.go", "pkg:example.org/project/a/store", "pkg:example.org/project/b/store", "pkg:example.org/third-party/store"} {
		if g.Nodes[NodeID("project", local)] == nil {
			t.Fatalf("missing %s", local)
		}
	}
	for _, edge := range []struct{ source, target, kind string }{
		{"file:main.go", "pkg:example.org/project/a/store", EdgeImports},
		{"file:main.go", "pkg:example.org/third-party/store", EdgeImports},
		{"type:Store@a/store/store.go", "func:Store.Open@a/store/methods.go", EdgeDefines},
	} {
		id := EdgeID("project", NodeID("project", edge.source), NodeID("project", edge.target), edge.kind)
		if g.Edges[id] == nil {
			t.Fatalf("missing edge %+v", edge)
		}
	}
	contained := false
	for _, edge := range g.Edges {
		if edge.Kind == EdgeContains && edge.TargetID == NodeID("project", "file:b/store/store.go") {
			contained = true
		}
		if g.Nodes[edge.SourceID] == nil || g.Nodes[edge.TargetID] == nil {
			t.Fatalf("dangling edge %+v", edge)
		}
	}
	if !contained {
		t.Fatal("root component does not contain nested files")
	}
}

func TestBuildLegacyPackageAndComponentCollisions(t *testing.T) {
	scan := &codebaseindex.ScanResult{
		Candidates: []*codebaseindex.FileEntry{
			{Path: "a/store.go", Language: "go", Symbols: &codebaseindex.SymbolInfo{Package: "store"}},
			{Path: "b/store.go", Language: "go", Symbols: &codebaseindex.SymbolInfo{Package: "store"}},
		},
		Components: []*codebaseindex.CodebaseComponent{{Name: "app", Root: "a"}, {Name: "app", Root: "b"}},
	}
	g := Build(scan, "legacy")
	packages, components := 0, 0
	for _, node := range g.Nodes {
		if node.Kind == NodePackage {
			packages++
		}
		if node.Kind == NodeComponent {
			components++
		}
	}
	if packages != 2 || components != 2 {
		t.Fatalf("collisions: %d packages, %d components", packages, components)
	}
}
