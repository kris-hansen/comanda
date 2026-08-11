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
