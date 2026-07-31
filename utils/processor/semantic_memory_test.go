package processor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/input"
	"github.com/kris-hansen/comanda/utils/semanticmemory"
	"gopkg.in/yaml.v3"
)

func TestMemoryConfigAcceptsLegacyBooleanAndSemanticMapping(t *testing.T) {
	var config DSLConfig
	err := yaml.Unmarshal([]byte(`
legacy:
  input: NA
  model: NA
  action: passthrough
  output: STDOUT
  memory: true
semantic:
  input: NA
  model: NA
  action: passthrough
  output: STDOUT
  memory:
    namespace: repo
    recall:
      query: input
      limit: 3
      max_chars: 1200
      types: [decision, constraint]
`), &config)
	if err != nil {
		t.Fatal(err)
	}
	if !config.Steps[0].Config.Memory.Legacy || config.Steps[0].Config.Memory.SemanticEnabled() {
		t.Fatalf("legacy memory did not retain its behavior: %#v", config.Steps[0].Config.Memory)
	}
	semantic := config.Steps[1].Config.Memory
	if semantic.Legacy || !semantic.SemanticEnabled() || semantic.Namespace != "repo" || semantic.Recall.Limit != 3 {
		t.Fatalf("semantic memory did not decode: %#v", semantic)
	}
}

func TestSemanticMemoryContextUsesBoundedRelevantRecords(t *testing.T) {
	root := t.TempDir()
	path := semanticmemory.DefaultPath(root, "repo")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := semanticmemory.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Upsert(context.Background(), semanticmemory.Record{
		Namespace: "repo", Type: "decision", SourceRef: "run-4", Content: "Use FTS5 for local durable memory retrieval.",
	})
	store.Close()
	if err != nil {
		t.Fatal(err)
	}

	handler := input.NewHandler()
	if err := handler.ProcessPath(writeMemoryInput(t, root, "We need local durable memory retrieval.")); err != nil {
		t.Fatal(err)
	}
	processor := &Processor{handler: handler, runtimeDir: root, variables: map[string]string{}, cliVariables: map[string]string{}}
	context := processor.semanticMemoryContext(Step{Name: "review", Config: StepConfig{Memory: MemoryConfig{
		Namespace: "repo", Recall: &MemoryRecallConfig{Query: "input", Limit: 2, MaxChars: 500},
	}}})
	if !strings.Contains(context, "Use FTS5") || !strings.Contains(context, "run-4") {
		t.Fatalf("unexpected recalled context: %q", context)
	}
}

func writeMemoryInput(t *testing.T, root, content string) string {
	t.Helper()
	path := filepath.Join(root, "input.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
