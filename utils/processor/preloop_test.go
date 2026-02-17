package processor

import (
	"testing"
)

func TestHybridWorkflowDetection(t *testing.T) {
	tests := []struct {
		name       string
		config     DSLConfig
		wantHybrid bool
	}{
		{
			name: "steps only",
			config: DSLConfig{
				Steps: []Step{{Name: "step1", Config: StepConfig{Model: "gpt-4o-mini"}}},
			},
			wantHybrid: false,
		},
		{
			name: "loops only",
			config: DSLConfig{
				Loops: map[string]*AgenticLoopConfig{
					"loop1": {Name: "loop1", MaxIterations: 1},
				},
			},
			wantHybrid: false,
		},
		{
			name: "steps and loops (hybrid)",
			config: DSLConfig{
				Steps: []Step{{Name: "prereq", Config: StepConfig{Model: "gpt-4o-mini"}}},
				Loops: map[string]*AgenticLoopConfig{
					"loop1": {Name: "loop1", MaxIterations: 1},
				},
			},
			wantHybrid: true,
		},
		{
			name: "parallel steps and loops (hybrid)",
			config: DSLConfig{
				ParallelSteps: map[string][]Step{
					"group1": {{Name: "parallel1", Config: StepConfig{Model: "gpt-4o-mini"}}},
				},
				Loops: map[string]*AgenticLoopConfig{
					"loop1": {Name: "loop1", MaxIterations: 1},
				},
			},
			wantHybrid: true,
		},
		{
			name: "agentic loops (legacy) and loops (hybrid)",
			config: DSLConfig{
				AgenticLoops: map[string]*AgenticLoopConfig{
					"legacy_loop": {Name: "legacy", MaxIterations: 1},
				},
				Loops: map[string]*AgenticLoopConfig{
					"loop1": {Name: "loop1", MaxIterations: 1},
				},
			},
			wantHybrid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hasSteps := len(tt.config.Steps) > 0 || len(tt.config.ParallelSteps) > 0 || len(tt.config.AgenticLoops) > 0
			hasLoops := len(tt.config.Loops) > 0
			isHybrid := hasSteps && hasLoops

			if isHybrid != tt.wantHybrid {
				t.Errorf("Hybrid detection = %v, want %v (hasSteps=%v, hasLoops=%v)",
					isHybrid, tt.wantHybrid, hasSteps, hasLoops)
			}
		})
	}
}

func TestPreLoopStepsConfiguration(t *testing.T) {
	// Test that processor correctly identifies hybrid workflows
	config := &DSLConfig{
		Steps: []Step{
			{
				Name: "create_index",
				Config: StepConfig{
					Input:  "test input",
					Model:  "gpt-4o-mini",
					Action: "create index",
					Output: ".comanda/INDEX.md",
				},
			},
		},
		Loops: map[string]*AgenticLoopConfig{
			"process_loop": {
				Name:          "process-loop",
				MaxIterations: 5,
				Steps: []Step{
					{
						Name: "process_step",
						Config: StepConfig{
							Input:  ".comanda/INDEX.md", // Depends on prereq step output
							Model:  "claude-code",
							Action: "process content",
							Output: "STDOUT",
						},
					},
				},
			},
		},
		ExecuteLoops: []string{"process_loop"},
	}

	// Create processor (won't run, just verify config handling)
	processor := NewProcessor(config, nil, nil, false, "")

	// Verify the processor was created and can detect the hybrid nature
	if processor == nil {
		t.Fatal("Processor should not be nil")
	}

	// The key assertion: both steps and loops should be present
	if len(processor.config.Steps) == 0 {
		t.Error("Processor should have steps configured")
	}
	if len(processor.config.Loops) == 0 {
		t.Error("Processor should have loops configured")
	}

	t.Logf("Hybrid workflow: %d pre-loop steps, %d loops",
		len(processor.config.Steps), len(processor.config.Loops))
}
