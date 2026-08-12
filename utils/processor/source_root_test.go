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

func TestEffectiveWorkDirUsesSelectedProjectOverCanvasRuntime(t *testing.T) {
	projectRoot := t.TempDir()
	dataDir := t.TempDir()
	processor := NewProcessor(
		&DSLConfig{},
		&config.EnvConfig{},
		&config.ServerConfig{Enabled: true, DataDir: dataDir},
		false,
		"canvas-run",
	)
	processor.SetSourceRoot(projectRoot)

	if got := processor.getEffectiveWorkDir(); got != projectRoot {
		t.Fatalf("effective work dir = %q, want selected project %q", got, projectRoot)
	}
}

func TestEffectiveWorkDirUsesDataDirectoryForUnscopedServerRuntime(t *testing.T) {
	dataDir := t.TempDir()
	processor := NewProcessor(
		&DSLConfig{},
		&config.EnvConfig{},
		&config.ServerConfig{Enabled: true, DataDir: dataDir},
		false,
		"canvas-run",
	)

	want := filepath.Join(dataDir, "canvas-run")
	if got := processor.getEffectiveWorkDir(); got != want {
		t.Fatalf("effective work dir = %q, want runtime directory %q", got, want)
	}
}

func TestPreStepQualityGatesUseSelectedProjectRoot(t *testing.T) {
	projectRoot := t.TempDir()
	dataDir := t.TempDir()
	processor := NewProcessor(
		&DSLConfig{},
		&config.EnvConfig{},
		&config.ServerConfig{Enabled: true, DataDir: dataDir},
		false,
		"canvas-run",
	)
	processor.SetSourceRoot(projectRoot)

	loopConfig := &AgenticLoopConfig{
		QualityGates: []QualityGateConfig{{
			Name:    "prepare-project-state",
			Command: "mkdir -p .comanda/qmd/state && printf ready > .comanda/qmd/state/coverage.txt",
			OnFail:  "abort",
		}},
	}
	if err := processor.runPreStepQualityGates(&LoopContext{Iteration: 1}, loopConfig, "workflow.yaml"); err != nil {
		t.Fatalf("runPreStepQualityGates() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(projectRoot, ".comanda/qmd/state/coverage.txt")); err != nil {
		t.Fatalf("project state was not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "canvas-run/.comanda/qmd/state/coverage.txt")); !os.IsNotExist(err) {
		t.Fatalf("runtime directory unexpectedly received project state, stat error = %v", err)
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
