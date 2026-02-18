package processor

import (
	"strings"
	"testing"
)

func TestValidateWorkflowStructure_ValidSimple(t *testing.T) {
	yaml := `
analyze_document:
  input: document.txt
  model: gpt-4o-mini
  action: "Summarize this document"
  output: STDOUT
`
	result := ValidateWorkflowStructure(yaml)
	if !result.Valid {
		t.Errorf("Expected valid workflow, got errors: %s", result.ErrorSummary())
	}
}

func TestValidateWorkflowStructure_ValidMultiStep(t *testing.T) {
	yaml := `
extract_data:
  input: data.csv
  model: gpt-4o-mini
  action: "Extract key metrics"
  output: metrics.txt

summarize_metrics:
  input: STDIN
  model: gpt-4o-mini
  action: "Summarize the metrics"
  output: STDOUT
`
	result := ValidateWorkflowStructure(yaml)
	if !result.Valid {
		t.Errorf("Expected valid workflow, got errors: %s", result.ErrorSummary())
	}
}

func TestValidateWorkflowStructure_MissingRequiredFields(t *testing.T) {
	yaml := `
incomplete_step:
  input: file.txt
  model: gpt-4o-mini
`
	result := ValidateWorkflowStructure(yaml)
	if result.Valid {
		t.Error("Expected invalid workflow for missing fields")
	}

	// Should have errors for missing action and output
	summary := result.ErrorSummary()
	if !strings.Contains(summary, "action") {
		t.Error("Expected error about missing 'action' field")
	}
	if !strings.Contains(summary, "output") {
		t.Error("Expected error about missing 'output' field")
	}
}

func TestValidateWorkflowStructure_HyphenMisuse(t *testing.T) {
	yaml := `
bad_step:
  - input: file.txt
  - model: gpt-4o-mini
  - action: "Do something"
  - output: STDOUT
`
	result := ValidateWorkflowStructure(yaml)
	if result.Valid {
		t.Error("Expected invalid workflow for hyphen misuse")
	}

	summary := result.ErrorSummary()
	if !strings.Contains(summary, "hyphen") || !strings.Contains(summary, "list") {
		t.Errorf("Expected error about hyphen/list misuse, got: %s", summary)
	}
}

func TestValidateWorkflowStructure_ValidGenerateStep(t *testing.T) {
	yaml := `
create_workflow:
  input: requirements.txt
  generate:
    model: gpt-4o-mini
    action: "Generate a workflow based on requirements"
    output: generated.yaml
`
	result := ValidateWorkflowStructure(yaml)
	if !result.Valid {
		t.Errorf("Expected valid generate step, got errors: %s", result.ErrorSummary())
	}
}

func TestValidateWorkflowStructure_GenerateMissingFields(t *testing.T) {
	yaml := `
bad_generate:
  input: NA
  generate:
    model: gpt-4o-mini
`
	result := ValidateWorkflowStructure(yaml)
	if result.Valid {
		t.Error("Expected invalid for generate missing action and output")
	}

	summary := result.ErrorSummary()
	if !strings.Contains(summary, "action") {
		t.Error("Expected error about missing action in generate block")
	}
	if !strings.Contains(summary, "output") {
		t.Error("Expected error about missing output in generate block")
	}
}

func TestValidateWorkflowStructure_GenerateMisplacedFields(t *testing.T) {
	yaml := `
misplaced_generate:
  input: NA
  model: gpt-4o-mini
  action: "This should be inside generate"
  output: STDOUT
  generate:
    output: generated.yaml
`
	result := ValidateWorkflowStructure(yaml)
	if result.Valid {
		t.Error("Expected invalid for misplaced fields in generate step")
	}

	summary := result.ErrorSummary()
	if !strings.Contains(summary, "inside") || !strings.Contains(summary, "generate") {
		t.Errorf("Expected error about fields belonging inside generate block, got: %s", summary)
	}
}

func TestValidateWorkflowStructure_ValidProcessStep(t *testing.T) {
	yaml := `
run_workflow:
  input: STDIN
  process:
    workflow_file: other_workflow.yaml
    inputs:
      key: value
`
	result := ValidateWorkflowStructure(yaml)
	if !result.Valid {
		t.Errorf("Expected valid process step, got errors: %s", result.ErrorSummary())
	}
}

func TestValidateWorkflowStructure_ProcessMissingWorkflowFile(t *testing.T) {
	yaml := `
bad_process:
  input: NA
  process:
    inputs:
      key: value
`
	result := ValidateWorkflowStructure(yaml)
	if result.Valid {
		t.Error("Expected invalid for process missing workflow_file")
	}

	summary := result.ErrorSummary()
	if !strings.Contains(summary, "workflow_file") {
		t.Error("Expected error about missing workflow_file")
	}
}

func TestValidateWorkflowStructure_ValidAgenticLoop(t *testing.T) {
	yaml := `
iterative_improvement:
  agentic_loop:
    max_iterations: 5
    exit_condition: llm_decides
    allowed_paths: [.]
  input: STDIN
  model: claude-code
  action: "Improve the code. Say DONE when finished."
  output: STDOUT
`
	result := ValidateWorkflowStructure(yaml)
	if !result.Valid {
		t.Errorf("Expected valid agentic loop, got errors: %s", result.ErrorSummary())
	}
}

func TestValidateWorkflowStructure_AgenticLoopInvalidExitCondition(t *testing.T) {
	yaml := `
bad_loop:
  agentic_loop:
    max_iterations: 5
    exit_condition: invalid_condition
  input: STDIN
  model: claude-code
  action: "Do something"
  output: STDOUT
`
	result := ValidateWorkflowStructure(yaml)
	if result.Valid {
		t.Error("Expected invalid for bad exit_condition")
	}

	summary := result.ErrorSummary()
	if !strings.Contains(summary, "exit_condition") {
		t.Error("Expected error about invalid exit_condition")
	}
}

func TestValidateWorkflowStructure_PatternMatchMissingPattern(t *testing.T) {
	yaml := `
pattern_loop:
  agentic_loop:
    max_iterations: 5
    exit_condition: pattern_match
  input: STDIN
  model: claude-code
  action: "Do something"
  output: STDOUT
`
	result := ValidateWorkflowStructure(yaml)
	if result.Valid {
		t.Error("Expected invalid for pattern_match without exit_pattern")
	}

	summary := result.ErrorSummary()
	if !strings.Contains(summary, "exit_pattern") {
		t.Error("Expected error about missing exit_pattern")
	}
}

func TestValidateWorkflowStructure_ValidTopLevelAgenticLoop(t *testing.T) {
	yaml := `
agentic-loop:
  config:
    max_iterations: 5
    exit_condition: llm_decides
  steps:
    plan:
      input: STDIN
      model: claude-code
      action: "Plan the work"
      output: $PLAN
    execute:
      input: $PLAN
      model: claude-code
      action: "Execute the plan"
      output: STDOUT
`
	result := ValidateWorkflowStructure(yaml)
	if !result.Valid {
		t.Errorf("Expected valid top-level agentic-loop, got errors: %s", result.ErrorSummary())
	}
}

func TestValidateWorkflowStructure_TopLevelAgenticLoopMissingConfig(t *testing.T) {
	yaml := `
agentic-loop:
  steps:
    do_work:
      input: STDIN
      model: claude-code
      action: "Do something"
      output: STDOUT
`
	result := ValidateWorkflowStructure(yaml)
	if result.Valid {
		t.Error("Expected invalid for missing config in agentic-loop")
	}

	summary := result.ErrorSummary()
	if !strings.Contains(summary, "config") {
		t.Error("Expected error about missing config block")
	}
}

func TestValidateWorkflowStructure_ValidCodebaseIndex(t *testing.T) {
	yaml := `
index_repo:
  step_type: codebase-index
  codebase_index:
    root: ./src
`
	result := ValidateWorkflowStructure(yaml)
	if !result.Valid {
		t.Errorf("Expected valid codebase-index step, got errors: %s", result.ErrorSummary())
	}
}

func TestValidateWorkflowStructure_ValidLoopsBlock(t *testing.T) {
	yaml := `
loops:
  data-collector:
    name: data-collector
    max_iterations: 10
    steps:
      collect:
        input: NA
        model: claude-code
        action: "Collect data"
        output: STDOUT

  data-analyzer:
    name: data-analyzer
    depends_on: [data-collector]
    max_iterations: 5
    steps:
      analyze:
        input: STDIN
        model: claude-code
        action: "Analyze data"
        output: STDOUT

execute_loops:
  - data-collector
  - data-analyzer
`
	result := ValidateWorkflowStructure(yaml)
	if !result.Valid {
		t.Errorf("Expected valid loops block, got errors: %s", result.ErrorSummary())
	}
}

func TestValidateWorkflowStructure_LoopsInvalidDependency(t *testing.T) {
	yaml := `
loops:
  my-loop:
    name: my-loop
    depends_on: [nonexistent-loop]
    steps:
      work:
        input: NA
        model: claude-code
        action: "Do work"
        output: STDOUT
`
	result := ValidateWorkflowStructure(yaml)
	if result.Valid {
		t.Error("Expected invalid for nonexistent dependency")
	}

	summary := result.ErrorSummary()
	if !strings.Contains(summary, "nonexistent-loop") {
		t.Error("Expected error about unknown dependency")
	}
}

func TestValidateWorkflowStructure_InvalidYAML(t *testing.T) {
	yaml := `
this is not: valid: yaml:
  - broken
    indentation
`
	result := ValidateWorkflowStructure(yaml)
	if result.Valid {
		t.Error("Expected invalid for broken YAML")
	}

	summary := result.ErrorSummary()
	if !strings.Contains(strings.ToLower(summary), "yaml") {
		t.Error("Expected error mentioning YAML syntax")
	}
}

func TestValidateWorkflowStructure_TabCharacter(t *testing.T) {
	// YAML itself rejects tabs for indentation - this should fail at parse time
	yaml := "step:\n\tinput: file.txt\n\tmodel: gpt-4o\n\taction: test\n\toutput: STDOUT"
	result := ValidateWorkflowStructure(yaml)
	if result.Valid {
		t.Error("Expected invalid for tab characters")
	}

	// The YAML parser rejects tabs, so we get a syntax error
	summary := result.ErrorSummary()
	if !strings.Contains(strings.ToLower(summary), "yaml") && !strings.Contains(strings.ToLower(summary), "syntax") {
		t.Errorf("Expected YAML syntax error for tab indentation, got: %s", summary)
	}
}

func TestValidateWorkflowStructure_TabInValue(t *testing.T) {
	// Tabs within string values are fine, but tabs in indentation that somehow parse should be caught
	// This test ensures tabs in content that parses are flagged
	yaml := "step:\n  input: \"file\twith\ttabs.txt\"\n  model: gpt-4o\n  action: test\n  output: STDOUT"
	result := ValidateWorkflowStructure(yaml)
	// Tabs in values are technically valid YAML, so this should pass
	if !result.Valid {
		t.Errorf("Tabs in string values should be valid, got errors: %s", result.ErrorSummary())
	}
}

func TestValidationError_String(t *testing.T) {
	// With line number
	err := ValidationError{
		Line:    5,
		Message: "Missing field",
		Fix:     "Add the field",
	}
	s := err.String()
	if !strings.Contains(s, "Line 5") {
		t.Errorf("Expected line number in error string: %s", s)
	}

	// With field name
	err = ValidationError{
		Field:   "my_step",
		Message: "Invalid config",
		Fix:     "Fix the config",
	}
	s = err.String()
	if !strings.Contains(s, "my_step") {
		t.Errorf("Expected step name in error string: %s", s)
	}
}

func TestValidationResult_ErrorSummary(t *testing.T) {
	result := ValidationResult{
		Valid: false,
		Errors: []ValidationError{
			{Field: "step1", Message: "Error 1", Fix: "Fix 1"},
			{Field: "step2", Message: "Error 2", Fix: "Fix 2"},
		},
	}

	summary := result.ErrorSummary()
	if !strings.Contains(summary, "Error 1") || !strings.Contains(summary, "Error 2") {
		t.Error("Expected both errors in summary")
	}
	if !strings.Contains(summary, "1.") || !strings.Contains(summary, "2.") {
		t.Error("Expected numbered errors in summary")
	}

	// Valid result should return empty summary
	validResult := ValidationResult{Valid: true}
	if validResult.ErrorSummary() != "" {
		t.Error("Expected empty summary for valid result")
	}
}

func TestValidateWorkflowStructure_ExecuteLoopsIgnoresTopLevel(t *testing.T) {
	yaml := `
index_codebase:
  step_type: codebase-index
  codebase_index:
    root: ~/myproject

categorize:
  input: $MYPROJECT_INDEX
  model: claude-code
  action: "Categorize"
  output: STDOUT

loops:
  analyze:
    max_iterations: 5
    steps:
      step1:
        input: STDIN
        model: claude-code
        action: "Analyze"
        output: STDOUT

execute_loops:
  - analyze
`
	result := ValidateWorkflowStructure(yaml)
	if result.Valid {
		t.Error("Expected validation to fail for top-level steps with execute_loops")
	}

	foundIgnoredWarning := false
	for _, err := range result.Errors {
		if strings.Contains(err.Message, "IGNORED") && strings.Contains(err.Message, "execute_loops") {
			foundIgnoredWarning = true
			break
		}
	}
	if !foundIgnoredWarning {
		t.Error("Expected warning about ignored top-level steps")
	}
}

func TestValidateWorkflowStructure_ValidLoopsWithCodebaseIndex(t *testing.T) {
	yaml := `
loops:
  indexer:
    max_iterations: 1
    steps:
      index:
        step_type: codebase-index
        codebase_index:
          root: ~/myproject
          output:
            path: .comanda/INDEX.md

  analyzer:
    depends_on: [indexer]
    max_iterations: 5
    steps:
      analyze:
        input: .comanda/INDEX.md
        model: claude-code
        action: "Analyze the codebase"
        output: results.md

execute_loops:
  - indexer
  - analyzer
`
	result := ValidateWorkflowStructure(yaml)

	// Should not have the "ignored steps" error
	for _, err := range result.Errors {
		if strings.Contains(err.Message, "IGNORED") {
			t.Errorf("Should not warn about ignored steps when all steps are inside loops: %s", err.Message)
		}
	}
}

func TestValidateWorkflowStructure_MissingVariableReference(t *testing.T) {
	yaml := `
analyze:
  input: $NONEXISTENT_VAR
  model: claude-code
  action: "Analyze"
  output: STDOUT
`
	result := ValidateWorkflowStructure(yaml)
	if result.Valid {
		t.Error("Expected validation to fail for missing variable reference")
	}

	foundMissingVar := false
	for _, err := range result.Errors {
		if strings.Contains(err.Message, "NONEXISTENT_VAR") && strings.Contains(err.Message, "no prior step exports") {
			foundMissingVar = true
			break
		}
	}
	if !foundMissingVar {
		t.Error("Expected error about missing variable reference")
	}
}
