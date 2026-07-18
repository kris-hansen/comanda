package mcp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
)

// naEchoWorkflow echoes its STDIN input without needing any LLM provider.
const naEchoWorkflow = `# Echo input back
echo:
  input: STDIN
  model: NA
  action:
    - Echo the input
  output: STDOUT
`

// naFileWorkflow reads the file named by the {{ filepath }} variable.
const naFileWorkflow = `# Read a file
readfile:
  input: "{{ filepath }}"
  model: NA
  action:
    - Return the file contents
  output: STDOUT
`

func TestRunWorkflowEchoesInput(t *testing.T) {
	dir := t.TempDir()
	path := writeWorkflow(t, dir, "echo.yaml", naEchoWorkflow)

	defs, err := DiscoverWorkflows(nil, []string{path})
	if err != nil {
		t.Fatalf("DiscoverWorkflows() error: %v", err)
	}

	runner := NewRunner(&config.EnvConfig{}, false)
	output, err := runner.Run(context.Background(), defs[0], map[string]string{"input": "hello mcp"})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !strings.Contains(output, "hello mcp") {
		t.Errorf("Run() output = %q, want it to contain %q", output, "hello mcp")
	}
}

func TestRunWorkflowSubstitutesVars(t *testing.T) {
	dir := t.TempDir()
	path := writeWorkflow(t, dir, "readfile.yaml", naFileWorkflow)

	inputFile := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(inputFile, []byte("secret file contents"), 0644); err != nil {
		t.Fatalf("writing input file: %v", err)
	}

	defs, err := DiscoverWorkflows(nil, []string{path})
	if err != nil {
		t.Fatalf("DiscoverWorkflows() error: %v", err)
	}
	if len(defs[0].Vars) != 1 || defs[0].Vars[0] != "filepath" {
		t.Fatalf("def vars = %v, want [filepath]", defs[0].Vars)
	}

	runner := NewRunner(&config.EnvConfig{}, false)
	output, err := runner.Run(context.Background(), defs[0], map[string]string{"filepath": inputFile})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !strings.Contains(output, "secret file contents") {
		t.Errorf("Run() output = %q, want it to contain %q", output, "secret file contents")
	}
}

func TestRunWorkflowErrors(t *testing.T) {
	runner := NewRunner(&config.EnvConfig{}, false)

	// Missing workflow file.
	missing := WorkflowDef{Name: "missing", Path: filepath.Join(t.TempDir(), "missing.yaml")}
	if _, err := runner.Run(context.Background(), missing, nil); err == nil {
		t.Error("Run() with missing workflow file: expected error, got nil")
	}

	// Workflow whose input file does not exist.
	dir := t.TempDir()
	path := writeWorkflow(t, dir, "readfile.yaml", naFileWorkflow)
	defs, err := DiscoverWorkflows(nil, []string{path})
	if err != nil {
		t.Fatalf("DiscoverWorkflows() error: %v", err)
	}
	_, err = runner.Run(context.Background(), defs[0], map[string]string{"filepath": filepath.Join(dir, "nope.txt")})
	if err == nil {
		t.Error("Run() with missing input file: expected error, got nil")
	}

	// Cancelled context.
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runner.Run(cancelled, defs[0], map[string]string{"filepath": "x"}); err == nil {
		t.Error("Run() with cancelled context: expected error, got nil")
	}
}
