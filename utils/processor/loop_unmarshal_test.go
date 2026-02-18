package processor

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestAgenticLoopConfigUnmarshal verifies that all fields are properly
// decoded when using custom unmarshaling with map-style steps
func TestAgenticLoopConfigUnmarshal(t *testing.T) {
	yamlContent := `
loops:
  backend_loop:
    name: backend-processing
    max_iterations: 10
    timeout_seconds: 300
    stateful: true
    checkpoint_interval: 3
    exit_condition: llm_decides
    allowed_paths: ["/tmp", "."]
    tools: [Read, Write, Edit]
    output_state: $BACKEND_RESULT
    steps:
      analyze:
        input: NA
        model: mock
        action: analyze
        output: STDOUT

  frontend_loop:
    name: frontend-processing
    depends_on: [backend_loop]
    input_state: $BACKEND_RESULT
    max_iterations: 5
    steps:
      process:
        input: $BACKEND_RESULT
        model: mock
        action: process
        output: STDOUT

execute_loops:
  - backend_loop
  - frontend_loop
`

	var config DSLConfig
	err := yaml.Unmarshal([]byte(yamlContent), &config)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify loop count
	if len(config.Loops) != 2 {
		t.Errorf("Expected 2 loops, got %d", len(config.Loops))
	}

	// Test backend_loop fields
	backend := config.Loops["backend_loop"]
	if backend == nil {
		t.Fatal("backend_loop not found")
	}

	if backend.Name != "backend-processing" {
		t.Errorf("backend.Name = %q, want 'backend-processing'", backend.Name)
	}
	if backend.MaxIterations != 10 {
		t.Errorf("backend.MaxIterations = %d, want 10", backend.MaxIterations)
	}
	if backend.TimeoutSeconds != 300 {
		t.Errorf("backend.TimeoutSeconds = %d, want 300", backend.TimeoutSeconds)
	}
	if !backend.Stateful {
		t.Error("backend.Stateful should be true")
	}
	if backend.CheckpointInterval != 3 {
		t.Errorf("backend.CheckpointInterval = %d, want 3", backend.CheckpointInterval)
	}
	if backend.ExitCondition != "llm_decides" {
		t.Errorf("backend.ExitCondition = %q, want 'llm_decides'", backend.ExitCondition)
	}
	if len(backend.AllowedPaths) != 2 {
		t.Errorf("backend.AllowedPaths length = %d, want 2", len(backend.AllowedPaths))
	}
	if len(backend.Tools) != 3 {
		t.Errorf("backend.Tools length = %d, want 3", len(backend.Tools))
	}
	if backend.OutputState != "$BACKEND_RESULT" {
		t.Errorf("backend.OutputState = %q, want '$BACKEND_RESULT'", backend.OutputState)
	}
	if len(backend.Steps) != 1 {
		t.Errorf("backend.Steps length = %d, want 1", len(backend.Steps))
	}

	// Test frontend_loop fields - especially depends_on
	frontend := config.Loops["frontend_loop"]
	if frontend == nil {
		t.Fatal("frontend_loop not found")
	}

	if frontend.Name != "frontend-processing" {
		t.Errorf("frontend.Name = %q, want 'frontend-processing'", frontend.Name)
	}
	if len(frontend.DependsOn) != 1 || frontend.DependsOn[0] != "backend_loop" {
		t.Errorf("frontend.DependsOn = %v, want [backend_loop]", frontend.DependsOn)
	}
	if frontend.InputState != "$BACKEND_RESULT" {
		t.Errorf("frontend.InputState = %q, want '$BACKEND_RESULT'", frontend.InputState)
	}
	if frontend.MaxIterations != 5 {
		t.Errorf("frontend.MaxIterations = %d, want 5", frontend.MaxIterations)
	}
	if len(frontend.Steps) != 1 {
		t.Errorf("frontend.Steps length = %d, want 1", len(frontend.Steps))
	}

	// Verify execute_loops
	if len(config.ExecuteLoops) != 2 {
		t.Errorf("ExecuteLoops length = %d, want 2", len(config.ExecuteLoops))
	}
}

// TestAgenticLoopConfigWithListSteps verifies that list-style steps also work
func TestAgenticLoopConfigWithListSteps(t *testing.T) {
	yamlContent := `
loops:
  test_loop:
    name: test
    max_iterations: 3
    depends_on: [other_loop]
    steps:
      - step1:
          input: NA
          model: mock
          action: test1
          output: $VAR1
      - step2:
          input: $VAR1
          model: mock
          action: test2
          output: STDOUT
`

	var config DSLConfig
	err := yaml.Unmarshal([]byte(yamlContent), &config)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	loop := config.Loops["test_loop"]
	if loop == nil {
		t.Fatal("test_loop not found")
	}

	if loop.MaxIterations != 3 {
		t.Errorf("MaxIterations = %d, want 3", loop.MaxIterations)
	}
	if len(loop.DependsOn) != 1 || loop.DependsOn[0] != "other_loop" {
		t.Errorf("DependsOn = %v, want [other_loop]", loop.DependsOn)
	}
	if len(loop.Steps) != 2 {
		t.Errorf("Steps length = %d, want 2", len(loop.Steps))
	}
}

// TestDependsOnChainParsing verifies complex dependency chains are parsed correctly
func TestDependsOnChainParsing(t *testing.T) {
	yamlContent := `
loops:
  loop1:
    name: first
    max_iterations: 1
    steps:
      s1:
        input: NA
        model: mock
        action: a
        output: STDOUT

  loop2:
    name: second
    depends_on: [loop1]
    max_iterations: 2
    steps:
      s1:
        input: NA
        model: mock
        action: a
        output: STDOUT

  loop3:
    name: third
    depends_on: [loop2]
    max_iterations: 3
    steps:
      s1:
        input: NA
        model: mock
        action: a
        output: STDOUT

  loop4:
    name: fourth
    depends_on: [loop3]
    max_iterations: 4
    steps:
      s1:
        input: NA
        model: mock
        action: a
        output: STDOUT

execute_loops:
  - loop1
  - loop2
  - loop3
  - loop4
`

	var config DSLConfig
	err := yaml.Unmarshal([]byte(yamlContent), &config)
	if err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Verify all loops have correct dependencies
	expectations := map[string]struct {
		maxIter   int
		dependsOn []string
	}{
		"loop1": {1, nil},
		"loop2": {2, []string{"loop1"}},
		"loop3": {3, []string{"loop2"}},
		"loop4": {4, []string{"loop3"}},
	}

	for name, expected := range expectations {
		loop := config.Loops[name]
		if loop == nil {
			t.Errorf("Loop %s not found", name)
			continue
		}

		if loop.MaxIterations != expected.maxIter {
			t.Errorf("%s.MaxIterations = %d, want %d", name, loop.MaxIterations, expected.maxIter)
		}

		if len(loop.DependsOn) != len(expected.dependsOn) {
			t.Errorf("%s.DependsOn = %v, want %v", name, loop.DependsOn, expected.dependsOn)
		} else {
			for i, dep := range expected.dependsOn {
				if loop.DependsOn[i] != dep {
					t.Errorf("%s.DependsOn[%d] = %q, want %q", name, i, loop.DependsOn[i], dep)
				}
			}
		}
	}
}
