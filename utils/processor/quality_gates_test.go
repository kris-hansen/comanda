package processor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunQualityGates_RetryFailureIsReturnedForVisibility(t *testing.T) {
	results, err := RunQualityGates([]QualityGateConfig{{
		Name:    "always-fails",
		Command: "exit 1",
		OnFail:  "retry",
		Retry:   &RetryConfig{MaxAttempts: 2},
	}}, t.TempDir())
	if err != nil {
		t.Fatalf("RunQualityGates() error = %v", err)
	}
	if len(results) != 1 || results[0].Passed || results[0].Attempts != 2 {
		t.Fatalf("unexpected retry result: %#v", results)
	}
}

func TestRunQualityGates_RepairFixesDeterministicFailure(t *testing.T) {
	workDir := t.TempDir()
	marker := filepath.Join(workDir, "ready")
	results, err := RunQualityGates([]QualityGateConfig{{
		Name:          "marker",
		Command:       "test -f ready",
		OnFail:        "repair",
		RepairCommand: "touch ready",
	}}, workDir)
	if err != nil {
		t.Fatalf("RunQualityGates() error = %v", err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("repair did not create marker: %v", err)
	}
	if len(results) != 1 || !results[0].Passed || results[0].Attempts != 2 {
		t.Fatalf("unexpected repair result: %#v", results)
	}
}

func TestRunQualityGates_RepairRequiresCommand(t *testing.T) {
	_, err := RunQualityGates([]QualityGateConfig{{
		Name:    "missing-repair",
		Command: "exit 1",
		OnFail:  "repair",
	}}, t.TempDir())
	if err == nil {
		t.Fatal("RunQualityGates() error = nil, want missing repair_command error")
	}
}
