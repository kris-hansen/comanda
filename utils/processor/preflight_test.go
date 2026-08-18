package processor

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
)

func TestPreflightChecksInputsAndDoesNotWriteWorkflowOutput(t *testing.T) {
	dir := t.TempDir()
	inputPath := filepath.Join(dir, "input.txt")
	outputPath := filepath.Join(dir, "output.txt")
	if err := os.WriteFile(inputPath, []byte("input"), 0o644); err != nil {
		t.Fatal(err)
	}

	processor := NewProcessor(&DSLConfig{Steps: []Step{{
		Name: "summarize",
		Config: StepConfig{
			Input:  inputPath,
			Model:  "NA",
			Action: "nothing is executed",
			Output: outputPath,
		},
	}}}, &config.EnvConfig{}, nil, false, "")

	if err := processor.Preflight(); err != nil {
		t.Fatalf("Preflight() error = %v", err)
	}
	if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
		t.Fatalf("Preflight() created workflow output: stat error = %v", err)
	}
}

func TestPreflightRejectsMissingInput(t *testing.T) {
	dir := t.TempDir()
	processor := NewProcessor(&DSLConfig{Steps: []Step{{
		Name: "summarize",
		Config: StepConfig{
			Input:  filepath.Join(dir, "missing.txt"),
			Model:  "NA",
			Action: "nothing is executed",
			Output: "STDOUT",
		},
	}}}, &config.EnvConfig{}, nil, false, "")

	err := processor.Preflight()
	if err == nil || !strings.Contains(err.Error(), "missing.txt") {
		t.Fatalf("Preflight() error = %v, want missing input error", err)
	}
}

func TestPreflightRejectsDeniedToolWithoutExecutingIt(t *testing.T) {
	processor := NewProcessor(&DSLConfig{Steps: []Step{{
		Name: "unsafe",
		Config: StepConfig{
			Input:  "tool: rm -rf /tmp/not-run",
			Model:  "NA",
			Action: "nothing is executed",
			Output: "STDOUT",
		},
	}}}, &config.EnvConfig{}, nil, false, "")

	err := processor.Preflight()
	if err == nil || !strings.Contains(err.Error(), "tool execution denied") {
		t.Fatalf("Preflight() error = %v, want denied tool error", err)
	}
}
