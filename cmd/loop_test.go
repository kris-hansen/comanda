package cmd

import (
	"testing"

	"github.com/kris-hansen/comanda/utils/processor"
)

func TestLoopGateSummary(t *testing.T) {
	tests := []struct {
		name      string
		state     *processor.LoopState
		wantGate  string
		wantError string
	}{
		{"no gates", &processor.LoopState{}, "-", "-"},
		{"passing gate", &processor.LoopState{QualityGateResults: []processor.QualityGateResult{{GateName: "tests", Passed: true}}}, "PASS tests", "-"},
		{"failed gate", &processor.LoopState{QualityGateResults: []processor.QualityGateResult{{GateName: "tests", Message: "test suite failed"}}}, "FAIL tests", "test suite failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gate, lastError := loopGateSummary(tt.state)
			if gate != tt.wantGate || lastError != tt.wantError {
				t.Fatalf("loopGateSummary() = (%q, %q), want (%q, %q)", gate, lastError, tt.wantGate, tt.wantError)
			}
		})
	}
}
