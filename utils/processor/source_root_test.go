package processor

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
)

func TestCodebaseIndexRelativeRootUsesSelectedProject(t *testing.T) {
	projectRoot := t.TempDir()
	processor := NewProcessor(&DSLConfig{}, &config.EnvConfig{}, &config.ServerConfig{Enabled: true}, false, "run")
	processor.SetSourceRoot(projectRoot)
	result, err := processor.buildCodebaseIndexConfigWithError(StepConfig{
		CodebaseIndex: &CodebaseIndexConfig{Root: "./src"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Root != filepath.Join(projectRoot, "src") {
		t.Fatalf("index root = %q, want project-relative %q", result.Root, filepath.Join(projectRoot, "src"))
	}
}

func TestProjectInputPathCannotEscapeSelectedRoot(t *testing.T) {
	root := t.TempDir()
	if _, err := resolveProjectInputPath(root, "../outside.txt"); err == nil {
		t.Fatal("expected project traversal to be rejected")
	}
	resolved, err := resolveProjectInputPath(root, "src/main.go")
	if err != nil {
		t.Fatalf("resolve project input: %v", err)
	}
	if want := filepath.Join(root, "src", "main.go"); resolved != want {
		t.Fatalf("resolved = %q, want %q", resolved, want)
	}
}

func TestResolveProjectPathAllowsAbsolutePathsWithinSelectedRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	resolvedRoot, err := ResolveProjectPath(root, root)
	if err != nil {
		t.Fatalf("resolve absolute project root: %v", err)
	}
	if resolvedRoot != root {
		t.Fatalf("resolved root = %q, want %q", resolvedRoot, root)
	}
	resolvedChild, err := ResolveProjectPath(root, filepath.Join(root, "src"))
	if err != nil {
		t.Fatalf("resolve absolute project child: %v", err)
	}
	if resolvedChild != filepath.Join(root, "src") {
		t.Fatalf("resolved child = %q, want %q", resolvedChild, filepath.Join(root, "src"))
	}
	if _, err := ResolveProjectPath(root, outside); err == nil {
		t.Fatal("expected outside absolute project path to be rejected")
	}
	if _, err := ResolveProjectPath(root, "../outside"); err == nil {
		t.Fatal("expected project traversal to be rejected")
	}
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks requires extra Windows test privileges")
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveProjectPath(root, "escape"); err == nil {
		t.Fatal("expected symlink escape to be rejected")
	}
	if _, err := ResolveProjectPath(root, filepath.Join(root, "escape")); err == nil {
		t.Fatal("expected absolute symlink escape to be rejected")
	}
}

func TestCodebaseIndexRootCannotEscapeSelectedProject(t *testing.T) {
	projectRoot := t.TempDir()
	processor := NewProcessor(&DSLConfig{}, &config.EnvConfig{}, &config.ServerConfig{Enabled: true}, false, "run")
	processor.SetSourceRoot(projectRoot)
	if _, err := processor.buildCodebaseIndexConfigWithError(StepConfig{
		CodebaseIndex: &CodebaseIndexConfig{Root: "../outside"},
	}); err == nil {
		t.Fatal("expected codebase index root traversal to be rejected")
	}
}
