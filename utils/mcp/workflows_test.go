package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeToolName(t *testing.T) {
	tests := []struct {
		base string
		want string
	}{
		{"code-review", "code_review"},
		{"summarize", "summarize"},
		{"my workflow.v2", "my_workflow_v2"},
		{"__leading_and_trailing__", "leading_and_trailing"},
		{"!!!", ""},
		{strings.Repeat("a", 100), strings.Repeat("a", 64)},
	}
	for _, tt := range tests {
		if got := sanitizeToolName(tt.base); got != tt.want {
			t.Errorf("sanitizeToolName(%q) = %q, want %q", tt.base, got, tt.want)
		}
	}
}

func TestExtractVars(t *testing.T) {
	content := `
step1:
  input: "{{ filename }}"
  model: gpt-4o
  action:
    - "Summarize {{ topic }} for {{audience}}"
    - "Chunk {{ chunk_index }} of {{ total_chunks }} ({{ current_chunk }})"
    - "File {{ file_index }} of {{ total_files }}"
    - "Loop counter {{ loop.counter }} and {{loop.max_iterations}}"
  output: STDOUT
`
	got := extractVars(content)
	want := []string{"audience", "filename", "topic"}
	if len(got) != len(want) {
		t.Fatalf("extractVars() = %v, want %v", got, want)
	}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("extractVars()[%d] = %q, want %q (full result: %v)", i, got[i], v, got)
		}
	}
}

func TestExtractDescription(t *testing.T) {
	withComment := "# Summarize a document\n# second line\nstep:\n  input: NA\n"
	if got := extractDescription(withComment, "summarize"); got != "Summarize a document" {
		t.Errorf("extractDescription() = %q, want %q", got, "Summarize a document")
	}

	noComment := "step:\n  input: NA\n"
	want := "Run comanda workflow 'summarize'"
	if got := extractDescription(noComment, "summarize"); got != want {
		t.Errorf("extractDescription() = %q, want %q", got, want)
	}

	emptyComment := "#\nstep:\n  input: NA\n"
	if got := extractDescription(emptyComment, "summarize"); got != want {
		t.Errorf("extractDescription() = %q, want fallback %q", got, want)
	}
}

func writeWorkflow(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writing workflow %s: %v", path, err)
	}
	return path
}

func TestDiscoverWorkflows(t *testing.T) {
	dir := t.TempDir()
	writeWorkflow(t, dir, "summarize.yaml", "# Summarize input\nstep:\n  input: STDIN\n  model: NA\n  action:\n    - echo\n  output: STDOUT\n")
	writeWorkflow(t, dir, "code-review.yml", "step:\n  input: \"{{ filename }}\"\n  model: NA\n  action:\n    - echo\n  output: STDOUT\n")
	writeWorkflow(t, dir, "notes.txt", "not a workflow")

	defs, err := DiscoverWorkflows([]string{dir}, nil)
	if err != nil {
		t.Fatalf("DiscoverWorkflows() error: %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("DiscoverWorkflows() found %d defs, want 2: %v", len(defs), defs)
	}

	// Sorted by path: code-review.yml before summarize.yaml
	if defs[0].Name != "code_review" {
		t.Errorf("defs[0].Name = %q, want %q", defs[0].Name, "code_review")
	}
	if defs[0].Description != "Run comanda workflow 'code-review'" {
		t.Errorf("defs[0].Description = %q", defs[0].Description)
	}
	if len(defs[0].Vars) != 1 || defs[0].Vars[0] != "filename" {
		t.Errorf("defs[0].Vars = %v, want [filename]", defs[0].Vars)
	}

	if defs[1].Name != "summarize" {
		t.Errorf("defs[1].Name = %q, want %q", defs[1].Name, "summarize")
	}
	if defs[1].Description != "Summarize input" {
		t.Errorf("defs[1].Description = %q, want %q", defs[1].Description, "Summarize input")
	}
}

func TestDiscoverWorkflowsExplicitFiles(t *testing.T) {
	dir := t.TempDir()
	path := writeWorkflow(t, dir, "echo.yaml", "step:\n  input: STDIN\n  model: NA\n  action:\n    - echo\n  output: STDOUT\n")

	defs, err := DiscoverWorkflows(nil, []string{path})
	if err != nil {
		t.Fatalf("DiscoverWorkflows() error: %v", err)
	}
	if len(defs) != 1 || defs[0].Path != path {
		t.Fatalf("DiscoverWorkflows() = %v, want one def for %s", defs, path)
	}

	if _, err := DiscoverWorkflows(nil, []string{filepath.Join(dir, "missing.yaml")}); err == nil {
		t.Error("DiscoverWorkflows() with missing file: expected error, got nil")
	}

	txtPath := writeWorkflow(t, dir, "notaworkflow.txt", "hello")
	if _, err := DiscoverWorkflows(nil, []string{txtPath}); err == nil {
		t.Error("DiscoverWorkflows() with .txt file: expected error, got nil")
	}
}

func TestDiscoverWorkflowsDuplicateNames(t *testing.T) {
	dirA := t.TempDir()
	dirB := t.TempDir()
	writeWorkflow(t, dirA, "code-review.yaml", "step:\n  input: STDIN\n  model: NA\n  action:\n    - a\n  output: STDOUT\n")
	writeWorkflow(t, dirB, "code_review.yml", "step:\n  input: STDIN\n  model: NA\n  action:\n    - b\n  output: STDOUT\n")

	_, err := DiscoverWorkflows([]string{dirA, dirB}, nil)
	if err == nil {
		t.Fatal("DiscoverWorkflows() with duplicate tool names: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "code_review") {
		t.Errorf("duplicate name error should mention the tool name, got: %v", err)
	}
	// Both conflicting paths should be listed for a clear startup error.
	if !strings.Contains(err.Error(), dirA) || !strings.Contains(err.Error(), dirB) {
		t.Errorf("duplicate name error should list both paths, got: %v", err)
	}
}
