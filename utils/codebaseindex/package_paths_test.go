package codebaseindex

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestPackagePathsRejectEscapingModuleSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require extra privileges")
	}
	for _, nested := range []bool{false, true} {
		t.Run(map[bool]string{false: "root", true: "nested"}[nested], func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			writeStructureFixture(t, outside, "go.mod", "module private.example/secret\ngo 1.25\n")
			dir := root
			if nested {
				writeStructureFixture(t, root, "go.mod", "module example.org/project\ngo 1.25\n")
				dir = filepath.Join(root, "nested")
			}
			writeStructureFixture(t, dir, "main.go", "package main\nfunc main() {}\n")
			if err := os.Symlink(filepath.Join(outside, "go.mod"), filepath.Join(dir, "go.mod")); err != nil {
				t.Fatal(err)
			}
			cfg := DefaultConfig()
			cfg.Root = root
			m, err := NewManager(cfg, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := m.Scan(); err == nil {
				t.Fatal("accepted go.mod symlink outside index root")
			}
		})
	}
}

func TestPackagePathsAllowInternalModuleSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require extra privileges")
	}
	root := t.TempDir()
	writeStructureFixture(t, root, "module.txt", "module example.org/project\ngo 1.25\n")
	writeStructureFixture(t, root, "pkg/main.go", "package main\nfunc main() {}\n")
	if err := os.Symlink("module.txt", filepath.Join(root, "go.mod")); err != nil {
		t.Fatal(err)
	}
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
		if f.Path == "pkg/main.go" && f.PackagePath == "example.org/project/pkg" {
			return
		}
	}
	t.Fatal("internal module symlink did not resolve")
}

func TestPackagePathsAllowAbsoluteInternalModuleSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks require extra privileges")
	}
	root := t.TempDir()
	writeStructureFixture(t, root, "module.txt", "module example.org/project\ngo 1.25\n")
	writeStructureFixture(t, root, "main.go", "package main\nfunc main() {}\n")
	if err := os.Symlink(filepath.Join(root, "module.txt"), filepath.Join(root, "go.mod")); err != nil {
		t.Fatal(err)
	}
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
		if f.Path == "main.go" && f.PackagePath == "example.org/project" {
			return
		}
	}
	t.Fatal("absolute internal module symlink did not resolve")
}

func TestPackagePathsRejectNonlocalEntries(t *testing.T) {
	root := t.TempDir()
	writeStructureFixture(t, root, "go.mod", "module example.org/project\ngo 1.25\n")
	m, err := NewManager(&Config{Root: root}, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../outside/main.go", filepath.Join(root, "main.go"), ""} {
		if err := m.resolvePackagePaths([]*FileEntry{{Path: name, Language: "go", Symbols: &SymbolInfo{Package: "main"}}}); err == nil {
			t.Fatalf("accepted nonlocal entry %q", name)
		}
	}
}
