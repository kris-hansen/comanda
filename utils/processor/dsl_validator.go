package processor

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Common I/O constants
const (
	OutputSTDOUT = "STDOUT"
	InputSTDIN   = "STDIN"
	InputNA      = "NA"
)

// ValidationError represents a single validation error with actionable feedback
type ValidationError struct {
	Line    int    // Line number (0 if unknown)
	Field   string // Field or step name involved
	Message string // Human-readable error message
	Fix     string // Suggested fix
}

func (e ValidationError) String() string {
	if e.Line > 0 {
		return fmt.Sprintf("Line %d: %s. %s", e.Line, e.Message, e.Fix)
	}
	if e.Field != "" {
		return fmt.Sprintf("Step '%s': %s. %s", e.Field, e.Message, e.Fix)
	}
	return fmt.Sprintf("%s. %s", e.Message, e.Fix)
}

// ValidationResult contains all validation errors
type ValidationResult struct {
	Valid  bool
	Errors []ValidationError
}

// ErrorSummary returns a formatted string of all errors for LLM feedback
func (r ValidationResult) ErrorSummary() string {
	if r.Valid || len(r.Errors) == 0 {
		return ""
	}

	var lines []string
	lines = append(lines, "Validation errors found:")
	for i, err := range r.Errors {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, err.String()))
	}
	return strings.Join(lines, "\n")
}

// ValidateWorkflowStructure performs comprehensive DSL validation on workflow YAML
// Returns actionable errors that can be fed back to an LLM for correction
func ValidateWorkflowStructure(yamlContent string) ValidationResult {
	result := ValidationResult{Valid: true}

	// First, check if it's valid YAML at all
	var rawNode yaml.Node
	if err := yaml.Unmarshal([]byte(yamlContent), &rawNode); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Message: fmt.Sprintf("Invalid YAML syntax: %v", err),
			Fix:     "Ensure proper YAML formatting with correct indentation and no syntax errors.",
		})
		return result
	}

	// Check for common YAML mistakes in raw content
	result.Errors = append(result.Errors, checkRawYAMLMistakes(yamlContent)...)

	// Parse into map for structural validation
	var workflow map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &workflow); err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, ValidationError{
			Message: fmt.Sprintf("Failed to parse workflow structure: %v", err),
			Fix:     "Check that the YAML represents a valid map of step names to step definitions.",
		})
		return result
	}

	// Validate each top-level entry
	for stepName, stepValue := range workflow {
		// Skip special top-level keys
		if stepName == "agentic-loop" || stepName == "loops" || stepName == "execute_loops" || stepName == "workflow" {
			continue
		}

		if stepName == "defer" || stepName == "steps" {
			result.Errors = append(result.Errors, validateStepCollection(stepName, stepValue)...)
			continue
		}

		if isStepCollection(stepValue) {
			result.Errors = append(result.Errors, validateStepCollection(stepName, stepValue)...)
			continue
		}

		stepErrors := validateStep(stepName, stepValue)
		result.Errors = append(result.Errors, stepErrors...)
	}

	// Check for agentic-loop block structure
	if agenticLoop, ok := workflow["agentic-loop"]; ok {
		loopErrors := validateAgenticLoopBlock(agenticLoop)
		result.Errors = append(result.Errors, loopErrors...)
	}

	// Check for loops block structure (multi-loop orchestration)
	if loops, ok := workflow["loops"]; ok {
		loopsErrors := validateLoopsBlock(loops)
		result.Errors = append(result.Errors, loopsErrors...)
	}

	// Check for ignored top-level steps when execute_loops is present
	if _, hasExecuteLoops := workflow["execute_loops"]; hasExecuteLoops {
		ignoredStepsErrors := validateExecuteLoopsIgnoredSteps(workflow)
		result.Errors = append(result.Errors, ignoredStepsErrors...)
	}

	// Validate step chaining (variables and file dependencies)
	chainingErrors := validateStepChaining(workflow)
	result.Errors = append(result.Errors, chainingErrors...)

	if len(result.Errors) > 0 {
		result.Valid = false
	}

	return result
}

// checkRawYAMLMistakes looks for common syntax mistakes in the raw YAML string
func checkRawYAMLMistakes(content string) []ValidationError {
	var errors []ValidationError

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Check for hyphen misuse in step fields (common mistake)
		// Pattern: "  - input:" or "  - model:" etc. at wrong indentation
		if regexp.MustCompile(`^\s{2,4}-\s+(input|model|action|output):`).MatchString(line) {
			// Check if this looks like it should be a key-value, not a list item
			errors = append(errors, ValidationError{
				Line:    lineNum,
				Message: "Appears to use list syntax (hyphen) for step fields",
				Fix:     "Remove the hyphen. Use 'input: value' not '- input: value'. Hyphens are only for actual lists.",
			})
		}

		// Check for tabs used as indentation (YAML should use spaces for indentation)
		// Only flag tabs at the start of a line (indentation), not tabs in string values
		if strings.HasPrefix(line, "\t") || strings.Contains(line, " \t") {
			errors = append(errors, ValidationError{
				Line:    lineNum,
				Message: "Contains tab character in indentation",
				Fix:     "Replace tabs with spaces. YAML requires consistent space indentation.",
			})
		}

		// Check for trailing colons without values on fields that need them
		if regexp.MustCompile(`^\s+(input|model|output):\s*$`).MatchString(line) {
			// This might be intentional (multi-line) but flag it
			if i+1 < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i+1]), "-") &&
				!strings.HasPrefix(lines[i+1], "  ") {
				errors = append(errors, ValidationError{
					Line:    lineNum,
					Message: fmt.Sprintf("Field appears to have no value: '%s'", trimmed),
					Fix:     "Provide a value after the colon, or use 'NA' if not applicable.",
				})
			}
		}
	}

	return errors
}

// validateStep validates a single step definition
func validateStep(stepName string, stepValue interface{}) []ValidationError {
	var errors []ValidationError

	stepMap, ok := stepValue.(map[string]interface{})
	if !ok {
		errors = append(errors, ValidationError{
			Field:   stepName,
			Message: "Step definition must be a map/object",
			Fix:     "Define the step as a YAML map with keys like 'input', 'model', 'action', 'output'.",
		})
		return errors
	}

	// Determine step type
	hasGenerate := stepMap["generate"] != nil
	hasProcess := stepMap["process"] != nil
	hasAgenticLoop := stepMap["agentic_loop"] != nil
	hasCodebaseIndex := stepMap["codebase_index"] != nil || stepMap["step_type"] == "codebase-index"
	hasQmdSearch := stepMap["qmd_search"] != nil || stepMap["type"] == "qmd-search"
	hasSkill := stepMap["skill"] != nil

	// Check for mixed step types
	typeCount := 0
	if hasGenerate {
		typeCount++
	}
	if hasProcess {
		typeCount++
	}
	if hasCodebaseIndex {
		typeCount++
	}
	if hasQmdSearch {
		typeCount++
	}
	if hasSkill {
		typeCount++
	}
	if typeCount > 1 {
		errors = append(errors, ValidationError{
			Field:   stepName,
			Message: "Step mixes multiple types (generate/process/codebase_index/qmd_search/skill)",
			Fix:     "A step can only be one type. Split into separate steps if needed.",
		})
	}

	// Validate based on step type
	if hasGenerate {
		errors = append(errors, validateGenerateStep(stepName, stepMap)...)
	} else if hasProcess {
		errors = append(errors, validateProcessStep(stepName, stepMap)...)
	} else if hasCodebaseIndex {
		// Codebase index steps don't need input/model/action/output
		errors = append(errors, validateCodebaseIndexStep(stepName, stepMap)...)
	} else if hasQmdSearch {
		errors = append(errors, validateQmdSearchStep(stepName, stepMap)...)
	} else if hasSkill {
		errors = append(errors, validateSkillStep(stepName, stepMap)...)
	} else {
		// Standard step (with or without agentic_loop)
		errors = append(errors, validateStandardStep(stepName, stepMap, hasAgenticLoop)...)
	}

	return errors
}

func isStepCollection(stepValue interface{}) bool {
	stepMap, ok := stepValue.(map[string]interface{})
	if !ok || len(stepMap) == 0 {
		return false
	}

	for _, child := range stepMap {
		childMap, ok := child.(map[string]interface{})
		if !ok {
			return false
		}
		if !looksLikeStepDefinition(childMap) {
			return false
		}
	}

	return true
}

func validateStepCollection(groupName string, groupValue interface{}) []ValidationError {
	groupMap, ok := groupValue.(map[string]interface{})
	if !ok {
		return []ValidationError{{
			Field:   groupName,
			Message: "Step collection must be a map of named steps",
			Fix:     "Define each nested step as 'name: { input: ..., model: ..., action: ..., output: ... }'.",
		}}
	}

	var errors []ValidationError
	for stepName, stepValue := range groupMap {
		errors = append(errors, validateStep(stepName, stepValue)...)
	}
	return errors
}

func looksLikeStepDefinition(stepMap map[string]interface{}) bool {
	stepKeys := []string{
		"input", "model", "action", "output", "generate", "process",
		"type", "agentic_loop", "codebase_index", "step_type", "qmd_search", "skill",
	}

	for _, key := range stepKeys {
		if _, ok := stepMap[key]; ok {
			return true
		}
	}

	return false
}

// validateStandardStep validates a standard processing step
func validateStandardStep(stepName string, stepMap map[string]interface{}, hasAgenticLoop bool) []ValidationError {
	var errors []ValidationError
	isResponsesStep := stepMap["type"] == "openai-responses"

	// Required fields for standard steps
	requiredFields := []string{"input", "model", "output"}
	if !isResponsesStep {
		requiredFields = append(requiredFields, "action")
	}

	for _, field := range requiredFields {
		if _, ok := stepMap[field]; !ok {
			errors = append(errors, ValidationError{
				Field:   stepName,
				Message: fmt.Sprintf("Missing required field '%s'", field),
				Fix:     fmt.Sprintf("Add '%s:' to the step. Use 'NA' if not applicable.", field),
			})
		}
	}

	if isResponsesStep {
		if _, hasAction := stepMap["action"]; !hasAction {
			if _, hasInstructions := stepMap["instructions"]; !hasInstructions {
				errors = append(errors, ValidationError{
					Field:   stepName,
					Message: "Missing required field 'action' or 'instructions'",
					Fix:     "Add either 'action:' or 'instructions:' to the OpenAI responses step.",
				})
			}
		}
	}

	// Check for misplaced fields that belong in sub-blocks
	if hasAgenticLoop {
		agenticLoop := stepMap["agentic_loop"]
		if loopMap, ok := agenticLoop.(map[string]interface{}); ok {
			// Validate agentic_loop config
			errors = append(errors, validateAgenticLoopConfig(stepName, loopMap)...)
		}
	}

	// Check input value
	if input, ok := stepMap["input"]; ok {
		errors = append(errors, validateInputField(stepName, input)...)
	}

	// Check output value
	if output, ok := stepMap["output"]; ok {
		errors = append(errors, validateOutputField(stepName, output)...)
	}

	return errors
}

// validateGenerateStep validates a generate step
func validateGenerateStep(stepName string, stepMap map[string]interface{}) []ValidationError {
	var errors []ValidationError

	generate, ok := stepMap["generate"].(map[string]interface{})
	if !ok {
		errors = append(errors, ValidationError{
			Field:   stepName,
			Message: "'generate' must be a map/object",
			Fix:     "Define generate as: generate:\\n    action: \"...\"\\n    output: filename.yaml",
		})
		return errors
	}

	// Check generate block has required fields
	if _, ok := generate["action"]; !ok {
		errors = append(errors, ValidationError{
			Field:   stepName,
			Message: "generate block missing required 'action' field",
			Fix:     "Add 'action:' with the prompt for workflow generation inside the generate block.",
		})
	}

	if _, ok := generate["output"]; !ok {
		errors = append(errors, ValidationError{
			Field:   stepName,
			Message: "generate block missing required 'output' field",
			Fix:     "Add 'output:' with the filename for the generated workflow (e.g., 'generated.yaml').",
		})
	}

	// Check for misplaced fields - these should NOT be at top level for generate steps
	topLevelFields := []string{"model", "action", "output"}
	for _, field := range topLevelFields {
		if _, ok := stepMap[field]; ok {
			errors = append(errors, ValidationError{
				Field:   stepName,
				Message: fmt.Sprintf("Field '%s' should be inside 'generate' block, not at step level", field),
				Fix:     fmt.Sprintf("Move '%s' inside the 'generate:' block.", field),
			})
		}
	}

	return errors
}

// validateProcessStep validates a process step
func validateProcessStep(stepName string, stepMap map[string]interface{}) []ValidationError {
	var errors []ValidationError

	process, ok := stepMap["process"].(map[string]interface{})
	if !ok {
		errors = append(errors, ValidationError{
			Field:   stepName,
			Message: "'process' must be a map/object",
			Fix:     "Define process as: process:\\n    workflow_file: path/to/workflow.yaml",
		})
		return errors
	}

	// Check process block has required fields
	if _, ok := process["workflow_file"]; !ok {
		errors = append(errors, ValidationError{
			Field:   stepName,
			Message: "process block missing required 'workflow_file' field",
			Fix:     "Add 'workflow_file:' with the path to the workflow to execute.",
		})
	}

	return errors
}

// validateCodebaseIndexStep validates a codebase-index step
func validateCodebaseIndexStep(stepName string, stepMap map[string]interface{}) []ValidationError {
	var errors []ValidationError

	// codebase_index block is optional if step_type is set
	if cbIndex, ok := stepMap["codebase_index"].(map[string]interface{}); ok {
		// Validate root if present
		if root, ok := cbIndex["root"]; ok {
			if _, ok := root.(string); !ok {
				errors = append(errors, ValidationError{
					Field:   stepName,
					Message: "codebase_index.root must be a string",
					Fix:     "Set root to a directory path string, e.g., 'root: ./src'",
				})
			}
		}
	}

	return errors
}

// validateQmdSearchStep validates a qmd-search step
func validateQmdSearchStep(stepName string, stepMap map[string]interface{}) []ValidationError {
	var errors []ValidationError

	qmdSearch, ok := stepMap["qmd_search"].(map[string]interface{})
	if !ok {
		errors = append(errors, ValidationError{
			Field:   stepName,
			Message: "'qmd_search' must be a map/object",
			Fix:     "Define qmd_search as: qmd_search:\n    query: \"...\"",
		})
		return errors
	}

	if _, ok := qmdSearch["query"]; !ok {
		errors = append(errors, ValidationError{
			Field:   stepName,
			Message: "qmd_search block missing required 'query' field",
			Fix:     "Add 'query:' with the search text or variable to look up.",
		})
	}

	if output, ok := stepMap["output"]; ok {
		errors = append(errors, validateOutputField(stepName, output)...)
	} else {
		errors = append(errors, ValidationError{
			Field:   stepName,
			Message: "Missing required field 'output'",
			Fix:     "Add 'output:' to capture the search results for later steps or STDOUT.",
		})
	}

	return errors
}

// validateSkillStep validates a skill step
func validateSkillStep(stepName string, stepMap map[string]interface{}) []ValidationError {
	var errors []ValidationError

	skillName, ok := stepMap["skill"].(string)
	if !ok || strings.TrimSpace(skillName) == "" {
		errors = append(errors, ValidationError{
			Field:   stepName,
			Message: "skill must be a non-empty string",
			Fix:     "Set 'skill:' to the name of an installed skill, e.g. 'skill: summarize'.",
		})
	}

	if input, ok := stepMap["input"]; ok {
		errors = append(errors, validateInputField(stepName, input)...)
	}

	if output, ok := stepMap["output"]; ok {
		errors = append(errors, validateOutputField(stepName, output)...)
	} else {
		errors = append(errors, ValidationError{
			Field:   stepName,
			Message: "Missing required field 'output'",
			Fix:     "Add 'output:' to capture the rendered skill result or use STDOUT.",
		})
	}

	if args, ok := stepMap["args"]; ok {
		if _, ok := args.(map[string]interface{}); !ok {
			errors = append(errors, ValidationError{
				Field:   stepName,
				Message: "args must be a map/object",
				Fix:     "Define args as key/value pairs under 'args:'.",
			})
		}
	}

	return errors
}

// validateAgenticLoopConfig validates the agentic_loop configuration block
func validateAgenticLoopConfig(stepName string, loopMap map[string]interface{}) []ValidationError {
	var errors []ValidationError

	// Check exit_condition validity
	if exitCond, ok := loopMap["exit_condition"]; ok {
		condStr, ok := exitCond.(string)
		if !ok {
			errors = append(errors, ValidationError{
				Field:   stepName,
				Message: "agentic_loop.exit_condition must be a string",
				Fix:     "Use 'exit_condition: llm_decides' or 'exit_condition: pattern_match'.",
			})
		} else if condStr != "llm_decides" && condStr != "pattern_match" {
			errors = append(errors, ValidationError{
				Field:   stepName,
				Message: fmt.Sprintf("Invalid exit_condition '%s'", condStr),
				Fix:     "Use 'exit_condition: llm_decides' or 'exit_condition: pattern_match'.",
			})
		}

		// If pattern_match, need exit_pattern
		if condStr == "pattern_match" {
			if _, ok := loopMap["exit_pattern"]; !ok {
				errors = append(errors, ValidationError{
					Field:   stepName,
					Message: "exit_condition 'pattern_match' requires 'exit_pattern'",
					Fix:     "Add 'exit_pattern:' with a regex pattern to match for exit.",
				})
			}
		}
	}

	// Validate max_iterations is positive if present
	if maxIter, ok := loopMap["max_iterations"]; ok {
		if num, ok := maxIter.(int); ok && num <= 0 {
			errors = append(errors, ValidationError{
				Field:   stepName,
				Message: "max_iterations must be positive",
				Fix:     "Set max_iterations to a positive number (default is 10).",
			})
		}
	}

	return errors
}

// validateAgenticLoopBlock validates a top-level agentic-loop block
func validateAgenticLoopBlock(loopValue interface{}) []ValidationError {
	var errors []ValidationError

	loopMap, ok := loopValue.(map[string]interface{})
	if !ok {
		errors = append(errors, ValidationError{
			Field:   "agentic-loop",
			Message: "agentic-loop must be a map with 'config' and 'steps'",
			Fix:     "Structure as: agentic-loop:\\n  config:\\n    ...\\n  steps:\\n    ...",
		})
		return errors
	}

	// Must have config
	if _, ok := loopMap["config"]; !ok {
		errors = append(errors, ValidationError{
			Field:   "agentic-loop",
			Message: "Missing required 'config' block",
			Fix:     "Add 'config:' with loop settings like max_iterations, exit_condition.",
		})
	} else {
		if configMap, ok := loopMap["config"].(map[string]interface{}); ok {
			errors = append(errors, validateAgenticLoopConfig("agentic-loop", configMap)...)
		}
	}

	// Must have steps
	if _, ok := loopMap["steps"]; !ok {
		errors = append(errors, ValidationError{
			Field:   "agentic-loop",
			Message: "Missing required 'steps' block",
			Fix:     "Add 'steps:' with one or more step definitions.",
		})
	} else {
		// Validate each step in the loop
		if stepsMap, ok := loopMap["steps"].(map[string]interface{}); ok {
			for stepName, stepValue := range stepsMap {
				stepErrors := validateStep(stepName, stepValue)
				errors = append(errors, stepErrors...)
			}
		}
	}

	return errors
}

// validateLoopsBlock validates a multi-loop orchestration block
func validateLoopsBlock(loopsValue interface{}) []ValidationError {
	var errors []ValidationError

	loopsMap, ok := loopsValue.(map[string]interface{})
	if !ok {
		errors = append(errors, ValidationError{
			Field:   "loops",
			Message: "loops must be a map of named loop definitions",
			Fix:     "Structure as: loops:\\n  loop-name:\\n    name: loop-name\\n    ...",
		})
		return errors
	}

	for loopName, loopValue := range loopsMap {
		loopMap, ok := loopValue.(map[string]interface{})
		if !ok {
			errors = append(errors, ValidationError{
				Field:   loopName,
				Message: "Loop definition must be a map",
				Fix:     "Define the loop with settings like 'name', 'max_iterations', 'steps'.",
			})
			continue
		}

		// Check for required 'steps' in each loop
		if _, ok := loopMap["steps"]; !ok {
			errors = append(errors, ValidationError{
				Field:   loopName,
				Message: "Loop missing required 'steps' block",
				Fix:     "Add 'steps:' with one or more step definitions.",
			})
		} else {
			errors = append(errors, validateLoopSteps(loopName, loopMap["steps"])...)
		}

		// Validate depends_on references exist (if we can)
		if deps, ok := loopMap["depends_on"].([]interface{}); ok {
			for _, dep := range deps {
				if depName, ok := dep.(string); ok {
					if _, exists := loopsMap[depName]; !exists {
						errors = append(errors, ValidationError{
							Field:   loopName,
							Message: fmt.Sprintf("depends_on references unknown loop '%s'", depName),
							Fix:     fmt.Sprintf("Ensure loop '%s' is defined in the loops block, or remove from depends_on.", depName),
						})
					}
				}
			}
		}
	}

	return errors
}

func validateLoopSteps(loopName string, stepsValue interface{}) []ValidationError {
	var errors []ValidationError

	switch steps := stepsValue.(type) {
	case map[string]interface{}:
		for stepName, stepValue := range steps {
			errors = append(errors, validateStep(stepName, stepValue)...)
		}
	case []interface{}:
		for index, item := range steps {
			stepDef, ok := item.(map[string]interface{})
			if !ok || len(stepDef) != 1 {
				errors = append(errors, ValidationError{
					Field:   loopName,
					Message: fmt.Sprintf("Loop step %d must be a single-entry map", index+1),
					Fix:     "Use list-style loop steps like '- step_name: { ... }'.",
				})
				continue
			}
			for stepName, stepValue := range stepDef {
				errors = append(errors, validateStep(stepName, stepValue)...)
			}
		}
	default:
		errors = append(errors, ValidationError{
			Field:   loopName,
			Message: "Loop steps must be a map or list of named steps",
			Fix:     "Define steps as either 'steps: { step_name: ... }' or a list of single-entry maps.",
		})
	}

	return errors
}

// validateInputField validates the input field value
func validateInputField(stepName string, input interface{}) []ValidationError {
	var errors []ValidationError

	switch v := input.(type) {
	case string:
		// Valid: "file.txt", InputSTDIN, "NA", "tool: command", etc.
		// Just basic sanity checks
		if v == "" {
			errors = append(errors, ValidationError{
				Field:   stepName,
				Message: "input field is empty",
				Fix:     "Provide an input source, 'STDIN', or 'NA'.",
			})
		}
	case []interface{}:
		// Valid: list of files
		for i, item := range v {
			if _, ok := item.(string); !ok {
				errors = append(errors, ValidationError{
					Field:   stepName,
					Message: fmt.Sprintf("input list item %d is not a string", i),
					Fix:     "Input list items should be file paths or variable references.",
				})
			}
		}
	case map[string]interface{}:
		// Valid: complex input like {url: ...} or {database: ...}
		// No additional validation needed here
	default:
		errors = append(errors, ValidationError{
			Field:   stepName,
			Message: "input has invalid type",
			Fix:     "Input should be a string, list of strings, or map (for url/database inputs).",
		})
	}

	return errors
}

// validateOutputField validates the output field value
func validateOutputField(stepName string, output interface{}) []ValidationError {
	var errors []ValidationError

	switch v := output.(type) {
	case string:
		if v == "" {
			errors = append(errors, ValidationError{
				Field:   stepName,
				Message: "output field is empty",
				Fix:     "Provide an output destination like 'STDOUT' or a filename.",
			})
		}
	case []interface{}:
		if len(v) == 0 {
			errors = append(errors, ValidationError{
				Field:   stepName,
				Message: "output list is empty",
				Fix:     "Provide at least one output destination such as 'STDOUT' or a filename.",
			})
		}
		for i, item := range v {
			outputItem, ok := item.(string)
			if !ok || outputItem == "" {
				errors = append(errors, ValidationError{
					Field:   stepName,
					Message: fmt.Sprintf("output list item %d is not a non-empty string", i),
					Fix:     "Output list items should be strings such as 'STDOUT', variables, or file paths.",
				})
			}
		}
	case map[string]interface{}:
		// Valid: complex output like {database: ...}
	default:
		errors = append(errors, ValidationError{
			Field:   stepName,
			Message: "output has invalid type",
			Fix:     "Output should be a string (STDOUT, filename) or map (for database outputs).",
		})
	}

	return errors
}

func collectStepOutputs(stepMap map[string]interface{}, stepName string, outputFiles map[string]string) {
	switch output := stepMap["output"].(type) {
	case string:
		if strings.HasPrefix(output, "$") {
			return
		}
		if output != OutputSTDOUT && output != InputSTDIN {
			outputFiles[output] = stepName
		}
	case []interface{}:
		for _, item := range output {
			outputStr, ok := item.(string)
			if !ok {
				continue
			}
			if outputStr == OutputSTDOUT || outputStr == InputSTDIN || strings.HasPrefix(outputStr, "$") {
				continue
			}
			if outputStr != OutputSTDOUT && outputStr != InputSTDIN {
				outputFiles[outputStr] = stepName
			}
		}
	}
}

func collectStepVariables(stepMap map[string]interface{}, stepName string, exportedVars map[string]string) {
	switch output := stepMap["output"].(type) {
	case string:
		if strings.HasPrefix(output, "$") {
			exportedVars[strings.TrimPrefix(output, "$")] = stepName
		}
	case []interface{}:
		for _, item := range output {
			outputStr, ok := item.(string)
			if ok && strings.HasPrefix(outputStr, "$") {
				exportedVars[strings.TrimPrefix(outputStr, "$")] = stepName
			}
		}
	}
}

func validateStepVariableRefs(stepName string, stepMap map[string]interface{}, exportedVars map[string]string) []ValidationError {
	var errors []ValidationError
	varRefRegex := regexp.MustCompile(`\$([A-Z_][A-Z0-9_]*)`)

	checkRefs := func(fieldName string, value string) {
		matches := varRefRegex.FindAllStringSubmatch(value, -1)
		for _, match := range matches {
			varName := match[1]
			if fieldName == "action" && (strings.HasPrefix(varName, "LOOP") || varName == InputSTDIN || varName == OutputSTDOUT) {
				continue
			}
			if _, exists := exportedVars[varName]; !exists {
				messagePrefix := "References"
				if fieldName == "action" {
					messagePrefix = "Action references"
				}
				if fieldName == "action" {
					if strings.Contains(varName, "_") && !strings.HasSuffix(varName, "_INDEX") {
						continue
					}
				}
				errors = append(errors, ValidationError{
					Field:   stepName,
					Message: fmt.Sprintf("%s variable $%s but no prior step exports it", messagePrefix, varName),
					Fix:     fmt.Sprintf("Ensure a prior step or loop sets 'output_state: $%s', or use a codebase-index step that exports this variable.", varName),
				})
			}
		}
	}

	switch input := stepMap["input"].(type) {
	case string:
		checkRefs("input", input)
	case []interface{}:
		for _, item := range input {
			if inputStr, ok := item.(string); ok {
				checkRefs("input", inputStr)
			}
		}
	}

	switch action := stepMap["action"].(type) {
	case string:
		checkRefs("action", action)
	case []interface{}:
		for _, item := range action {
			if actionStr, ok := item.(string); ok {
				checkRefs("action", actionStr)
			}
		}
	}

	return errors
}

func forEachLoopStep(stepsValue interface{}, fn func(stepName string, stepMap map[string]interface{})) {
	switch steps := stepsValue.(type) {
	case map[string]interface{}:
		for stepName, stepDef := range steps {
			if stepMap, ok := stepDef.(map[string]interface{}); ok {
				fn(stepName, stepMap)
			}
		}
	case []interface{}:
		for _, item := range steps {
			stepDef, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			for stepName, stepValue := range stepDef {
				if stepMap, ok := stepValue.(map[string]interface{}); ok {
					fn(stepName, stepMap)
				}
			}
		}
	}
}

// validateExecuteLoopsIgnoredSteps checks for top-level steps that will be ignored when execute_loops is present
func validateExecuteLoopsIgnoredSteps(workflow map[string]interface{}) []ValidationError {
	var errors []ValidationError

	reservedKeys := map[string]bool{
		"loops":         true,
		"execute_loops": true,
		"workflow":      true,
		"agentic-loop":  true,
	}

	var ignoredSteps []string
	for stepName := range workflow {
		if !reservedKeys[stepName] {
			ignoredSteps = append(ignoredSteps, stepName)
		}
	}

	if len(ignoredSteps) > 0 {
		errors = append(errors, ValidationError{
			Field:   "execute_loops",
			Message: fmt.Sprintf("Top-level steps will be IGNORED when execute_loops is present: %v", ignoredSteps),
			Fix:     "Move these steps inside the 'loops:' block, or remove 'execute_loops:' if you want sequential step execution.",
		})
	}

	return errors
}

// validateStepChaining validates that step inputs reference outputs from prior steps
func validateStepChaining(workflow map[string]interface{}) []ValidationError {
	var errors []ValidationError

	// Collect all exported variables and output files
	exportedVars := make(map[string]string) // variable name -> step that exports it
	outputFiles := make(map[string]string)  // file path -> step that writes it

	// Check loops block for output_state and codebase_index exports
	if loops, ok := workflow["loops"].(map[string]interface{}); ok {
		for loopName, loopDef := range loops {
			loopMap, ok := loopDef.(map[string]interface{})
			if !ok {
				continue
			}

			// Check output_state
			if outputState, ok := loopMap["output_state"].(string); ok {
				varName := strings.TrimPrefix(outputState, "$")
				exportedVars[varName] = loopName
			}

			// Check steps within loop for codebase_index and file outputs
			if steps, ok := loopMap["steps"].(map[string]interface{}); ok {
				for stepName, stepDef := range steps {
					stepMap, ok := stepDef.(map[string]interface{})
					if !ok {
						continue
					}

					// Check for codebase_index exports
					if ci, ok := stepMap["codebase_index"].(map[string]interface{}); ok {
						if output, ok := ci["output"].(map[string]interface{}); ok {
							if path, ok := output["path"].(string); ok {
								outputFiles[path] = stepName
							}
						}
						// Infer variable name from root path
						if root, ok := ci["root"].(string); ok {
							// Extract repo name from path and uppercase it
							parts := strings.Split(strings.TrimSuffix(root, "/"), "/")
							if len(parts) > 0 {
								repoName := strings.ToUpper(parts[len(parts)-1])
								repoName = strings.ReplaceAll(repoName, "-", "_")
								exportedVars[repoName+"_INDEX"] = stepName
							}
						}
					}

					collectStepOutputs(stepMap, stepName, outputFiles)
					collectStepVariables(stepMap, stepName, exportedVars)
				}
			}
		}
	}

	// Check top-level steps (when no execute_loops)
	if _, hasExecuteLoops := workflow["execute_loops"]; !hasExecuteLoops {
		for stepName, stepDef := range workflow {
			if stepName == "loops" || stepName == "execute_loops" || stepName == "workflow" || stepName == "agentic-loop" {
				continue
			}
			if isStepCollection(stepDef) {
				groupMap := stepDef.(map[string]interface{})
				for nestedName, nestedDef := range groupMap {
					nestedStepMap, ok := nestedDef.(map[string]interface{})
					if !ok {
						continue
					}
					collectStepOutputs(nestedStepMap, nestedName, outputFiles)
					collectStepVariables(nestedStepMap, nestedName, exportedVars)
				}
				continue
			}
			stepMap, ok := stepDef.(map[string]interface{})
			if !ok {
				continue
			}

			// Check for codebase_index exports
			if ci, ok := stepMap["codebase_index"].(map[string]interface{}); ok {
				if output, ok := ci["output"].(map[string]interface{}); ok {
					if path, ok := output["path"].(string); ok {
						outputFiles[path] = stepName
					}
				}
				if root, ok := ci["root"].(string); ok {
					parts := strings.Split(strings.TrimSuffix(root, "/"), "/")
					if len(parts) > 0 {
						repoName := strings.ToUpper(parts[len(parts)-1])
						repoName = strings.ReplaceAll(repoName, "-", "_")
						exportedVars[repoName+"_INDEX"] = stepName
					}
				}
			}

			collectStepOutputs(stepMap, stepName, outputFiles)
			collectStepVariables(stepMap, stepName, exportedVars)
		}
	}

	// Now validate that inputs reference valid exports
	// Validate loops
	if loops, ok := workflow["loops"].(map[string]interface{}); ok {
		for loopName, loopDef := range loops {
			loopMap, ok := loopDef.(map[string]interface{})
			if !ok {
				continue
			}
			forEachLoopStep(loopMap["steps"], func(stepName string, stepMap map[string]interface{}) {
				errors = append(errors, validateStepVariableRefs(fmt.Sprintf("%s.%s", loopName, stepName), stepMap, exportedVars)...)
			})
		}
	}

	// Validate top-level steps
	if _, hasExecuteLoops := workflow["execute_loops"]; !hasExecuteLoops {
		for stepName, stepDef := range workflow {
			if stepName == "loops" || stepName == "execute_loops" || stepName == "workflow" || stepName == "agentic-loop" {
				continue
			}
			if stepMap, ok := stepDef.(map[string]interface{}); ok && !isStepCollection(stepMap) {
				errors = append(errors, validateStepVariableRefs(stepName, stepMap, exportedVars)...)
			} else if isStepCollection(stepDef) {
				groupMap := stepDef.(map[string]interface{})
				for nestedName, nestedDef := range groupMap {
					nestedStepMap, ok := nestedDef.(map[string]interface{})
					if !ok {
						continue
					}
					errors = append(errors, validateStepVariableRefs(nestedName, nestedStepMap, exportedVars)...)
				}
			}
		}
	}

	return errors
}
