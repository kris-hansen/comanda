package processor

import (
	"path/filepath"
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
