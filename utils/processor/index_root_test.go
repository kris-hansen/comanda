package processor

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
	"gopkg.in/yaml.v3"
)

func TestCodebaseIndexRootConfinement(t *testing.T) {
	dataDir, project, outside := t.TempDir(), t.TempDir(), t.TempDir()
	for _, base := range []string{dataDir, project} {
		if err := os.Mkdir(filepath.Join(base, "src"), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, enabled := range []bool{false, true} {
		for _, selected := range []bool{false, true} {
			base := dataDir
			p := NewProcessor(&DSLConfig{}, &config.EnvConfig{}, &config.ServerConfig{Enabled: enabled, DataDir: dataDir}, false, "../../untrusted-runtime")
			if selected {
				base = project
				p.SetSourceRoot(project)
			}
			canonical, err := filepath.EvalSymlinks(base)
			if err != nil {
				t.Fatal(err)
			}
			for _, requested := range []string{"", ".", "src", filepath.Join(base, "src"), "../outside", outside} {
				got, err := p.buildCodebaseIndexConfigWithError(StepConfig{CodebaseIndex: &CodebaseIndexConfig{Root: requested}})
				preflightErr := p.preflightCodebaseIndex(Step{Name: "index", Config: StepConfig{CodebaseIndex: &CodebaseIndexConfig{Root: requested}}})
				if requested == "../outside" || requested == outside {
					if err == nil || preflightErr == nil {
						t.Fatalf("accepted outside root %q (auth=%v, selected=%v)", requested, enabled, selected)
					}
					continue
				}
				if err != nil || preflightErr != nil {
					t.Fatalf("legitimate root %q: runtime %v, preflight %v", requested, err, preflightErr)
				}
				want := canonical
				if requested != "" && requested != "." {
					want = filepath.Join(canonical, "src")
				}
				if got.Root != want {
					t.Fatalf("root %q resolved to %q, want %q", requested, got.Root, want)
				}
			}
			got, err := p.buildCodebaseIndexConfigWithError(StepConfig{})
			if err != nil || got.Root != canonical {
				t.Fatalf("omitted config escaped approved root: %+v, %v", got, err)
			}
		}
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(outside, filepath.Join(dataDir, "escape")); err != nil {
			t.Fatal(err)
		}
		p := NewProcessor(&DSLConfig{}, &config.EnvConfig{}, &config.ServerConfig{Enabled: true, DataDir: dataDir}, false, "")
		if _, err := p.buildCodebaseIndexConfigWithError(StepConfig{CodebaseIndex: &CodebaseIndexConfig{Root: "escape"}}); err == nil {
			t.Fatal("accepted root symlink outside data directory")
		}
	}
}

func TestNestedWorkflowPreservesIndexRoot(t *testing.T) {
	project, dataDir := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "go.mod"), []byte("module example.test/nested\ngo 1.25\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "main.go"), []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(project, "child.yaml")
	if err := os.WriteFile(child, []byte("index:\n  step_type: codebase-index\n  codebase_index: {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var dsl DSLConfig
	workflow, err := yaml.Marshal(map[string]any{"child": map[string]any{"process": map[string]any{"workflow_file": child}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(workflow, &dsl); err != nil {
		t.Fatal(err)
	}
	p := NewProcessor(&dsl, &config.EnvConfig{}, &config.ServerConfig{Enabled: true, DataDir: dataDir}, false, "")
	p.SetSourceRoot(project)
	if err := p.Preflight(); err != nil {
		t.Fatal(err)
	}
	if err := p.Process(); err != nil {
		t.Fatal(err)
	}
	files, err := filepath.Glob(filepath.Join(project, ".comanda", "*.meta.json"))
	if err != nil || len(files) != 1 {
		t.Fatalf("child index left selected project: %v, %v", files, err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, ".comanda")); !os.IsNotExist(err) {
		t.Fatalf("child index was written under data directory: %v", err)
	}
}

func TestCodebaseIndexRejectsUnconfiguredServerRoot(t *testing.T) {
	p := NewProcessor(&DSLConfig{}, &config.EnvConfig{}, &config.ServerConfig{Enabled: true}, false, "")
	if _, err := p.buildCodebaseIndexConfigWithError(StepConfig{}); err == nil {
		t.Fatal("unconfigured server indexed working directory")
	}
}

func TestCodebaseIndexCLIRootRemainsUnrestricted(t *testing.T) {
	root := t.TempDir()
	p := NewProcessor(&DSLConfig{}, &config.EnvConfig{}, &config.ServerConfig{Enabled: false}, false, "")
	got, err := p.buildCodebaseIndexConfigWithError(StepConfig{CodebaseIndex: &CodebaseIndexConfig{Root: root}})
	if err != nil || got.Root != root {
		t.Fatalf("CLI root changed: %+v, %v", got, err)
	}
}
