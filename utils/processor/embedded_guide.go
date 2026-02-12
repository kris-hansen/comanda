package processor

import (
	"fmt"
	"strings"

	"github.com/kris-hansen/comanda/utils/models"
	"gopkg.in/yaml.v3"
)

// GetEmbeddedLLMGuide returns the Comanda YAML DSL Guide for LLM consumption
// with the current supported models injected from the registry
func GetEmbeddedLLMGuide() string {
	// Get all models from the registry
	registry := models.GetRegistry()
	allModels := registry.GetAllModelsList()

	return GetEmbeddedLLMGuideWithModels(allModels)
}

// GetEmbeddedLLMGuideWithModels returns the Comanda YAML DSL Guide for LLM consumption
// with a specific list of available models injected. Use this when you have
// a known list of configured/available models (e.g., from envConfig).
func GetEmbeddedLLMGuideWithModels(availableModels []string) string {
	if len(availableModels) == 0 {
		// Fall back to registry models if none provided
		registry := models.GetRegistry()
		availableModels = registry.GetAllModelsList()
	}

	// Format the models as a comma-separated list with code formatting
	modelsList := formatModelsList(availableModels)

	// Replace the models section in the guide
	guide := strings.Replace(
		embeddedLLMGuideTemplate,
		"{{SUPPORTED_MODELS}}",
		modelsList,
		1,
	)

	return guide
}

// ValidateWorkflowModels parses a workflow YAML and validates that all model
// references are in the list of available models. Returns a list of invalid
// model names found, or nil if all models are valid.
func ValidateWorkflowModels(yamlContent string, availableModels []string) []string {
	if len(availableModels) == 0 {
		// No validation possible without a list of available models
		return nil
	}

	// Create a set for fast lookup
	validModels := make(map[string]bool)
	for _, m := range availableModels {
		validModels[strings.ToLower(m)] = true
	}

	// Parse the YAML into a generic map structure
	var workflow map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &workflow); err != nil {
		// If we can't parse, we can't validate - let runtime handle it
		return nil
	}

	var invalidModels []string
	seen := make(map[string]bool) // Avoid duplicates

	// Walk through each step and extract model references
	for _, stepValue := range workflow {
		stepMap, ok := stepValue.(map[string]interface{})
		if !ok {
			continue
		}

		// Check direct model field
		invalidModels = append(invalidModels, extractInvalidModels(stepMap["model"], validModels, seen)...)

		// Check generate block
		if generate, ok := stepMap["generate"].(map[string]interface{}); ok {
			invalidModels = append(invalidModels, extractInvalidModels(generate["model"], validModels, seen)...)
		}

		// Check agentic_loop block (inline syntax)
		if agenticLoop, ok := stepMap["agentic_loop"].(map[string]interface{}); ok {
			// Check steps within the agentic loop
			if steps, ok := agenticLoop["steps"].([]interface{}); ok {
				for _, step := range steps {
					if stepMap, ok := step.(map[string]interface{}); ok {
						invalidModels = append(invalidModels, extractInvalidModels(stepMap["model"], validModels, seen)...)
					}
				}
			}
		}
	}

	// Check agentic-loop block (top-level syntax)
	if agenticLoop, ok := workflow["agentic-loop"].(map[string]interface{}); ok {
		if steps, ok := agenticLoop["steps"].(map[string]interface{}); ok {
			for _, stepValue := range steps {
				if stepMap, ok := stepValue.(map[string]interface{}); ok {
					invalidModels = append(invalidModels, extractInvalidModels(stepMap["model"], validModels, seen)...)
				}
			}
		}
	}

	return invalidModels
}

// extractInvalidModels extracts model names from a model field value and returns
// those not in the validModels set
func extractInvalidModels(modelField interface{}, validModels map[string]bool, seen map[string]bool) []string {
	var invalid []string

	switch v := modelField.(type) {
	case string:
		if v != "" && strings.ToUpper(v) != "NA" {
			lower := strings.ToLower(v)
			if !validModels[lower] && !seen[lower] {
				invalid = append(invalid, v)
				seen[lower] = true
			}
		}
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" && strings.ToUpper(s) != "NA" {
				lower := strings.ToLower(s)
				if !validModels[lower] && !seen[lower] {
					invalid = append(invalid, s)
					seen[lower] = true
				}
			}
		}
	}

	return invalid
}

// formatModelsList formats a list of models as a comma-separated string with code formatting
func formatModelsList(models []string) string {
	if len(models) == 0 {
		return "No models configured"
	}

	// Sort models alphabetically for consistent output
	// sort.Strings(models)

	// Format the models as a comma-separated list with code formatting
	var formattedModels []string
	for _, model := range models {
		formattedModels = append(formattedModels, fmt.Sprintf("`%s`", model))
	}

	return strings.Join(formattedModels, ", ")
}

// EmbeddedLLMGuide contains the Comanda YAML DSL Guide for LLM consumption
// This is embedded directly in the binary to avoid file path issues
// For backward compatibility, we keep this constant
const EmbeddedLLMGuide = `# Comanda YAML DSL Guide (for LLM Consumption)

This guide specifies the YAML-based Domain Specific Language (DSL) for Comanda workflows, enabling LLMs to generate valid workflow files.

## Overview

Comanda workflows consist of one or more named steps. Each step performs an operation. There are six main types of steps:
1.  **Standard Processing Step:** Involves LLMs, file processing, data operations.
2.  **Generate Step:** Uses an LLM to dynamically create a new Comanda workflow YAML file.
3.  **Process Step:** Executes another Comanda workflow file (static or dynamically generated).
4.  **Agentic Loop Step:** Iteratively processes until an exit condition is met (for refinement, planning, autonomous tasks).
5.  **Multi-Loop Orchestration:** Coordinates multiple interdependent agentic loops with variable passing and creator/checker patterns.
6.  **Codebase Index Step:** Scans a repository and generates a compact Markdown index for LLM consumption.

## Core Workflow Structure

A Comanda workflow is a YAML map where each key is a ` + "`step_name`" + ` (string, user-defined), mapping to a dictionary defining the step.

` + "```yaml" + `
# Example of a workflow structure
workflow_step_1:
  # ... step definition ...
another_step_name:
  # ... step definition ...
` + "```" + `

## 1. Standard Processing Step Definition

This is the most common step type.

**Basic Structure:**
` + "```yaml" + `
step_name:
  input: [input source]
  model: [model name]
  action: [action to perform / prompt provided]
  output: [output destination]
  type: [optional, e.g., "openai-responses"] # Specifies specialized handling
  batch_mode: [individual|combined] # Optional, for multi-file inputs
  skip_errors: [true|false] # Optional, for multi-file inputs
  # ... other type-specific fields for "openai-responses" like 'instructions', 'tools', etc.
` + "```" + `

**Key Elements:**
- ` + "`input`" + `: (Required for most, can be ` + "`NA`" + `) Source of data. See "Input Types".
- ` + "`model`" + `: (Required, can be ` + "`NA`" + `) LLM model to use. See "Models".
- ` + "`action`" + `: (Required for most) Instructions or operations. See "Actions".
- ` + "`output`" + `: (Required) Destination for results. See "Outputs".
- ` + "`type`" + `: (Optional) Specifies a specialized handler for the step, e.g., ` + "`openai-responses`" + `. If omitted, it's a general-purpose LLM or NA step.
- ` + "`batch_mode`" + `: (Optional, default: ` + "`combined`" + `) For steps with multiple file inputs, defines if files are processed ` + "`combined`" + ` into one LLM call or ` + "`individual`" + `ly.
- ` + "`skip_errors`" + `: (Optional, default: ` + "`false`" + `) If ` + "`batch_mode: individual`" + `, determines if processing continues if one file fails.

**OpenAI Responses API Specific Fields (used when ` + "`type: openai-responses`" + `):**
- ` + "`instructions`" + `: (string) System message for the LLM.
- ` + "`tools`" + `: (list of maps) Configuration for tools/functions the LLM can call.
- ` + "`previous_response_id`" + `: (string) ID of a previous response for maintaining conversation state.
- ` + "`max_output_tokens`" + `: (int) Token limit for the LLM response.
- ` + "`temperature`" + `: (float) Sampling temperature.
- ` + "`top_p`" + `: (float) Nucleus sampling (top-p).
- ` + "`stream`" + `: (bool) Whether to stream the response.
- ` + "`response_format`" + `: (map) Specifies response format, e.g., ` + "`{ type: \"json_object\" }`" + `.


## 2. Generate Step Definition (` + "`generate`" + `)

This step uses an LLM to dynamically create a new Comanda workflow YAML file.

**Structure:**
` + "```yaml" + `
step_name_for_generation:
  input: [optional_input_source_for_context, or NA] # e.g., STDIN, a file with requirements
  generate:
    model: [llm_model_for_generation, optional] # e.g., gpt-4o-mini. Uses default if omitted.
    action: [prompt_for_workflow_generation] # Natural language instruction for the LLM.
    output: [filename_for_generated_yaml] # e.g., new_workflow.yaml
    context_files: [list_of_files_for_additional_context, optional] # e.g., [schema.txt, examples.yaml]
` + "```" + `
**` + "`generate`" + ` Block Attributes:**
- ` + "`model`" + `: (string, optional) Specifies the LLM to use for generation. If omitted, uses the ` + "`default_generation_model`" + ` configured in Comanda. You can set or update this default model by running ` + "`comanda configure`" + ` and following the prompts for setting a default generation model.
- ` + "`action`" + `: (string, required) The natural language instruction given to the LLM to guide the workflow generation.
- ` + "`output`" + `: (string, required) The filename where the generated Comanda workflow YAML file will be saved.
- ` + "`context_files`" + `: (list of strings, optional) A list of file paths to provide as additional context to the LLM, beyond the standard Comanda DSL guide (which is implicitly included).
- **Note:** The ` + "`input`" + ` field for a ` + "`generate`" + ` step is optional. If provided (e.g., ` + "`STDIN`" + ` or a file path), its content will be added to the context for the LLM generating the workflow. If not needed, use ` + "`input: NA`" + `.

## 3. Process Step Definition (` + "`process`" + `)

This step executes another Comanda workflow file.

**Structure:**
` + "```yaml" + `
step_name_for_processing:
  input: [optional_input_source_for_sub_workflow, or NA] # e.g., STDIN to pass data to the sub-workflow
  process:
    workflow_file: [path_to_comanda_yaml_to_execute] # e.g., generated_workflow.yaml or existing_flow.yaml
    inputs: {key1: value1, key2: value2, optional} # Map of inputs to pass to the sub-workflow.
    # capture_outputs: [list_of_outputs_to_capture, optional] # Future: Define how to capture specific outputs.
` + "```" + `
**` + "`process`" + ` Block Attributes:**
- ` + "`workflow_file`" + `: (string, required) The path to the Comanda workflow YAML file to be executed. This can be a statically defined path or the output of a ` + "`generate`" + ` step.
- ` + "`inputs`" + `: (map, optional) A map of key-value pairs to pass as initial variables to the sub-workflow. These can be accessed within the sub-workflow (e.g., as ` + "`$parent.key1`" + `).
- **Note:** The ` + "`input`" + ` field for a ` + "`process`" + ` step is optional. If ` + "`input: STDIN`" + ` is used, the output of the previous step in the parent workflow will be available as the initial ` + "`STDIN`" + ` for the *first* step of the sub-workflow if that first step expects ` + "`STDIN`" + `.

## 4. Agentic Loop Step Definition (` + "`agentic_loop`" + ` / ` + "`agentic-loop`" + `)

Agentic loops enable iterative LLM processing until an exit condition is met. This is powerful for tasks that require refinement, multi-step reasoning, or autonomous decision-making.

**When to use agentic loops:**
- Iterative code improvement (analyze → fix → verify cycles)
- Multi-step planning and execution
- Tasks where the LLM decides when work is complete
- Refinement workflows (draft → improve → finalize)

### Inline Syntax (Single-Step Loop)

For simple iterative tasks with a single step:

` + "```yaml" + `
step_name:
  agentic_loop:
    max_iterations: 5           # Safety limit (default: 10)
    exit_condition: pattern_match  # or "llm_decides"
    exit_pattern: "COMPLETE"    # Regex pattern for pattern_match
  input: STDIN
  model: claude-code
  action: |
    Iteration {{ loop.iteration }}.
    Previous work: {{ loop.previous_output }}

    Continue improving. Say COMPLETE when done.
  output: STDOUT
` + "```" + `

### Block Syntax (Multi-Step Loop)

For complex loops with multiple sub-steps per iteration:

` + "```yaml" + `
agentic-loop:
  config:
    max_iterations: 5           # Safety limit (default: 10)
    timeout_seconds: 300        # Total timeout (default: 300)
    exit_condition: llm_decides # or "pattern_match"
    exit_pattern: "DONE"        # For pattern_match
    context_window: 3           # Past iterations to include (default: 5)

  steps:
    plan:
      input: STDIN
      model: claude-code
      action: |
        Iteration {{ loop.iteration }}.
        Previous: {{ loop.previous_output }}

        Plan next steps. Say DONE if complete.
      output: $PLAN

    execute:
      input: $PLAN
      model: claude-code
      action: "Execute the plan"
      output: STDOUT
` + "```" + `

**` + "`agentic_loop`" + ` Configuration:**

**Core Settings:**
- ` + "`max_iterations`" + `: (int, default: 10) Maximum iterations before stopping.
- ` + "`timeout_seconds`" + `: (int, default: 0) Total time limit in seconds. **0 = no timeout** (loop runs until max_iterations or exit condition).
- ` + "`exit_condition`" + `: (string) How to detect completion:
  - ` + "`llm_decides`" + `: Exits when output contains "DONE", "COMPLETE", or "FINISHED"
  - ` + "`pattern_match`" + `: Exits when output matches ` + "`exit_pattern`" + ` regex
- ` + "`exit_pattern`" + `: (string) Regex pattern for ` + "`pattern_match`" + ` condition.
- ` + "`context_window`" + `: (int, default: 5) Number of past iterations to include in context.

**File Access (REQUIRED for Claude Code):**
- ` + "`allowed_paths`" + `: (list, **REQUIRED for file operations**) Directories where Claude Code can use tools (Read, Write, Edit, Bash, etc.). **Without this, Claude Code runs in print-only mode and CANNOT read/write files.** When generating workflows, infer paths from: the ` + "`codebase_index.root`" + ` if present, file paths mentioned in the action, or use ` + "`[.]`" + ` (current directory) as fallback.
- ` + "`tools`" + `: (list, optional) Restrict available tools (e.g., ` + "`[Read, Glob, Grep]`" + ` for read-only access). If omitted, all tools are available.

**State Persistence (for Long-Running Loops):**
- ` + "`name`" + `: (string, **REQUIRED for stateful loops**) Unique identifier for the loop. Enables state persistence and resume capability.
- ` + "`stateful`" + `: (bool, default: false) Enable state persistence to ` + "`~/.comanda/loop-states/{name}.json`" + `. Allows resuming after interruption.
- ` + "`checkpoint_interval`" + `: (int, default: 5) Save state every N iterations. Lower values = more frequent saves = safer resume.

**Quality Gates (Automated Validation):**
- ` + "`quality_gates`" + `: (list, optional) Automated checks to run after each iteration. Each gate validates loop output and can trigger retry/abort/skip actions.

**Quality Gate Configuration:**
` + "```yaml" + `
quality_gates:
  - name: typecheck           # Gate name
    command: "npm run typecheck"  # Shell command to execute
    on_fail: retry            # Action: retry, abort, or skip
    timeout: 60               # Timeout in seconds
    retry:                    # Retry configuration (optional)
      max_attempts: 3         # Maximum retry attempts
      backoff_type: exponential  # linear or exponential
      initial_delay: 5        # Initial delay in seconds

  - name: security
    type: security            # Built-in gate type (syntax, security, test)
    on_fail: abort

  - name: tests
    command: "npm test"
    on_fail: skip             # Continue even if this fails
` + "```" + `

**Built-in Quality Gate Types:**
- ` + "`syntax`" + `: Checks for syntax errors (Python, JS, Go, etc.)
- ` + "`security`" + `: Scans for hardcoded secrets, security issues
- ` + "`test`" + `: Runs test commands with coverage reporting

**Quality Gate Actions (` + "`on_fail`" + `):**
- ` + "`retry`" + `: Retry the gate with backoff (exponential or linear)
- ` + "`abort`" + `: Stop loop immediately and save state as "failed"
- ` + "`skip`" + `: Log warning and continue to next iteration

**Template Variables in Actions:**
- ` + "`{{ loop.iteration }}`" + `: Current iteration number (1-based)
- ` + "`{{ loop.previous_output }}`" + `: Output from previous iteration
- ` + "`{{ loop.total_iterations }}`" + `: Maximum allowed iterations
- ` + "`{{ loop.elapsed_seconds }}`" + `: Seconds since loop started

**Example: Agentic Code Exploration**
` + "```yaml" + `
explore_codebase:
  agentic_loop:
    max_iterations: 3
    exit_condition: llm_decides
    allowed_paths: [./src, ./tests]
    tools: [Read, Glob, Grep]  # Read-only access
  input: STDIN
  model: claude-code
  action: |
    Explore the codebase and answer: {{ loop.previous_output }}
    Say DONE when you have the answer.
  output: STDOUT
` + "```" + `

**Example: Iterative Code Implementation**
` + "```yaml" + `
implement:
  agentic_loop:
    max_iterations: 3
    exit_condition: pattern_match
    exit_pattern: "SATISFIED"
    allowed_paths: [.]  # Required for file operations
  input: STDIN
  model: claude-code
  action: |
    Iteration {{ loop.iteration }}. Implement and improve the code.
    Previous: {{ loop.previous_output }}

    Add error handling, edge cases, tests.
    Say SATISFIED when production-ready.
  output: STDOUT
` + "```" + `

**Example: Plan and Build Loop**
` + "```yaml" + `
agentic-loop:
  config:
    max_iterations: 5
    exit_condition: llm_decides
    allowed_paths: [.]  # Required for file operations

  steps:
    plan:
      input: STDIN
      model: claude-code
      action: |
        Iteration {{ loop.iteration }}.
        Create/refine the implementation plan.
        Say DONE when ready to implement.
      output: $PLAN

    build:
      input: $PLAN
      model: claude-code
      action: "Generate code based on the plan"
      output: STDOUT
` + "```" + `

**Example: Long-Running Loop with State Persistence**
` + "```yaml" + `
agentic-loop:
  config:
    name: code-refactor-loop      # Required for stateful loops
    stateful: true                 # Enable state persistence
    max_iterations: 50
    timeout_seconds: 0             # No timeout - run until complete
    checkpoint_interval: 5         # Save every 5 iterations
    exit_condition: llm_decides
    allowed_paths: [./src]

    quality_gates:
    name: syntax-check
        type: syntax
        on_fail: retry
        retry:
          max_attempts: 3
          backoff_type: exponential
          initial_delay: 2

    name: tests
        command: "npm test"
        on_fail: abort              # Stop if tests fail
        timeout: 300

  steps:
    analyze:
      input: STDIN
      model: claude-code
      action: |
        Iteration {{ loop.iteration }} of {{ loop.total_iterations }}.
        Analyze code and refactor one module.
        Say DONE when all modules are refactored.
      output: STDOUT
` + "```" + `

**Example: Quality Gates with Retry**
` + "```yaml" + `
code_improvement:
  agentic_loop:
    name: improve-code
    stateful: true
    max_iterations: 10
    allowed_paths: [.]

    quality_gates:
    name: typecheck
        command: "npm run typecheck"
        on_fail: retry
        timeout: 60
        retry:
          max_attempts: 3
          backoff_type: exponential
          initial_delay: 5

    name: lint
        command: "npm run lint"
        on_fail: skip               # Non-critical, continue

  input: STDIN
  model: claude-code
  action: "Improve code quality. Say COMPLETE when done."
  output: STDOUT
` + "```" + `

**CRITICAL: When generating agentic_loop workflows with Claude Code:**
- **ALWAYS include ` + "`allowed_paths`" + `** - Claude Code CANNOT read/write files without it
- If the workflow uses ` + "`codebase_index`" + `, use the same ` + "`root`" + ` directory in ` + "`allowed_paths`" + `
- If the action mentions specific file paths or directories, include those directories
- When in doubt, use ` + "`allowed_paths: [.]`" + ` for current directory access
- Forgetting ` + "`allowed_paths`" + ` is a common error that causes "permission denied" failures
- For long-running tasks (hours/days), use ` + "`stateful: true`" + ` and ` + "`timeout_seconds: 0`" + `
- **Default to ` + "`claude-code`" + ` model for agentic workflows** - it provides the best tool use and autonomous capabilities

## 5. Multi-Loop Orchestration

Multi-loop orchestration enables complex autonomous workflows with multiple interdependent agentic loops. This is essential for creator/checker patterns, sequential processing pipelines, and complex task decomposition.

**When to use multi-loop orchestration:**
- Creator/checker validation workflows (loop A creates, loop B validates, rerun A if validation fails)
- Sequential data processing (collect → analyze → report)
- Complex task decomposition (break large task into specialized sub-loops)
- Workflows requiring variable passing between autonomous agents

### Named Loops Syntax

Define multiple named loops with dependencies and variable passing:

` + "```yaml" + `
loops:
  data-collector:
    name: data-collector
    stateful: true
    max_iterations: 10
    timeout_seconds: 0
    exit_condition: llm_decides
    allowed_paths: [.]
    output_state: $RAW_DATA         # Export result to variable

    steps:
    collect:
          input: STDIN
          model: claude-code
          action: "Collect data and say DONE when complete"
          output: STDOUT

  data-analyzer:
    name: data-analyzer
    depends_on: [data-collector]    # Wait for collector to complete
    input_state: $RAW_DATA           # Read collector's output
    stateful: true
    max_iterations: 5
    exit_condition: llm_decides
    allowed_paths: [.]
    output_state: $ANALYSIS          # Export analysis result

    steps:
    analyze:
          input: STDIN
          model: claude-code
          action: |
            Analyze this data: {{ loop.previous_output }}
            Say DONE when analysis is complete.
          output: STDOUT

  report-generator:
    name: report-generator
    depends_on: [data-analyzer]     # Wait for analyzer
    input_state: $ANALYSIS           # Read analyzer's output
    max_iterations: 1
    allowed_paths: [.]

    steps:
    generate:
          input: STDIN
          model: claude-code
          action: "Create final report based on analysis"
          output: STDOUT

# Execute loops in dependency order (topological sort)
execute_loops:
  - data-collector
  - data-analyzer
  - report-generator
` + "```" + `

**Key Configuration Fields:**
- ` + "`loops`" + `: (map) Named loop definitions
- ` + "`depends_on`" + `: (list) Loops that must complete before this one starts
- ` + "`input_state`" + `: (string) Variable to read as input (e.g., ` + "`$RAW_DATA`" + `)
- ` + "`output_state`" + `: (string) Variable to export result to (e.g., ` + "`$ANALYSIS`" + `)
- ` + "`execute_loops`" + `: (list) Simple execution order (dependencies override this)

### Creator/Checker Pattern

Advanced workflow pattern where a creator loop implements features and a checker loop validates them, with automatic rerun on failure:

` + "```yaml" + `
loops:
  # Creator loop: implements features
  feature-creator:
    name: feature-creator
    stateful: true
    max_iterations: 3
    timeout_seconds: 0
    exit_condition: llm_decides
    allowed_paths: [.]
    output_state: $CODE

    quality_gates:
    name: syntax
        type: syntax
        on_fail: abort

    steps:
    implement:
          input: STDIN
          model: claude-code
          action: |
            Iteration {{ loop.iteration }}.
            Implement the requested feature.
            Say DONE when implementation is complete.
          output: STDOUT

  # Checker loop: validates implementation
  code-checker:
    name: code-checker
    depends_on: [feature-creator]   # Wait for creator
    input_state: $CODE               # Read creator's output
    max_iterations: 1
    exit_condition: pattern_match
    exit_pattern: "^PASS"            # Exit when output starts with PASS

    steps:
    review:
          input: STDIN
          model: claude-code
          action: |
            Review this implementation: {{ loop.previous_output }}

            Check for:
            - Correctness
            - Edge cases
            - Code quality
            - Test coverage

            Output: PASS if acceptable, or FAIL with specific issues.
          output: STDOUT

# Advanced workflow with creator/checker relationship
workflow:
  creator:
    type: loop
    loop: feature-creator
    role: creator

  checker:
    type: loop
    loop: code-checker
    role: checker
    validates: creator               # This loop validates the creator
    on_fail: rerun_creator          # Auto-rerun creator if validation fails
` + "```" + `

**Workflow Node Configuration:**
- ` + "`type`" + `: Always ` + "`loop`" + ` for loop nodes
- ` + "`loop`" + `: Name of the loop to execute
- ` + "`role`" + `: Node role (` + "`creator`" + `, ` + "`checker`" + `, ` + "`finalizer`" + `)
- ` + "`validates`" + `: Name of the node this checker validates
- ` + "`on_fail`" + `: Action on validation failure:
  - ` + "`rerun_creator`" + `: Automatically rerun the creator loop (max 3 attempts)
  - ` + "`abort`" + `: Stop workflow immediately
  - ` + "`manual`" + `: Return for manual review

**Validation Logic:**
- Checker loop output is scanned for "PASS" or "FAIL"
- If "PASS" found → workflow continues
- If "FAIL" found and ` + "`on_fail: rerun_creator`" + ` → creator reruns with checker feedback
- Maximum 3 rerun attempts (prevents infinite loops)

### Dependency Graph Execution

Comanda automatically executes loops in correct order using topological sort:

1. **Build dependency graph** from ` + "`depends_on`" + ` relationships
2. **Detect cycles** - fail fast if circular dependencies exist
3. **Topological sort** using Kahn's algorithm
4. **Execute in order** - each loop waits for its dependencies

**Cycle Detection Example:**
` + "```yaml" + `
loops:
  loop-a:
    depends_on: [loop-b]

  loop-b:
    depends_on: [loop-a]

execute_loops:
  - loop-a
  - loop-b

# Error: dependency cycle detected: loop-a -> loop-b -> loop-a
` + "```" + `

### Variable Passing

Variables flow between loops via ` + "`input_state`" + ` and ` + "`output_state`" + `:

` + "```yaml" + `
loops:
  producer:
    output_state: $MY_DATA          # Write to variable

  consumer:
    depends_on: [producer]
    input_state: $MY_DATA            # Read from variable
` + "```" + `

**Variable Rules:**
- Variables are stored in memory during workflow execution
- ` + "`output_state`" + ` exports loop's final output to a variable
- ` + "`input_state`" + ` reads variable as loop input
- Variables persist across loop boundaries
- Missing variable causes error (check dependencies)

### Complete Example: Codebase Analysis Pipeline

` + "```yaml" + `
# Multi-loop workflow: analyze codebase → identify issues → generate report
loops:
  codebase-analyzer:
    name: codebase-analyzer
    stateful: true
    max_iterations: 20
    timeout_seconds: 0
    exit_condition: llm_decides
    allowed_paths: [/tmp/code]
    output_state: $ANALYSIS

    steps:
    analyze:
          input: NA
          model: claude-code
          action: |
            Analyze the codebase at /tmp/code.
            Find patterns, architecture issues, dependencies.
            Say DONE when analysis is complete.
          output: STDOUT

  tech-debt-finder:
    name: tech-debt-finder
    depends_on: [codebase-analyzer]
    input_state: $ANALYSIS
    stateful: true
    max_iterations: 10
    timeout_seconds: 0
    exit_condition: llm_decides
    allowed_paths: [/tmp/code]
    output_state: $TECH_DEBT

    steps:
    identify:
          input: STDIN
          model: claude-code
          action: |
            Based on this analysis: {{ loop.previous_output }}

            Identify tech debt and anti-patterns:
            - Code smells
            - Security issues
            - Performance bottlenecks
            - Maintainability problems

            Say DONE when complete.
          output: STDOUT

  report-writer:
    name: report-writer
    depends_on: [tech-debt-finder]
    input_state: $TECH_DEBT
    max_iterations: 1
    allowed_paths: [.]

    steps:
    write:
          input: STDIN
          model: claude-code
          action: |
            Create a comprehensive markdown report:
            {{ loop.previous_output }}

            Save to ./tech-debt-report.md
          output: STDOUT

execute_loops:
  - codebase-analyzer
  - tech-debt-finder
  - report-writer
` + "```" + `

**CRITICAL: When generating multi-loop workflows:**
- **Use ` + "`claude-code`" + ` as default model** for agentic capabilities
- **Always include ` + "`allowed_paths`" + `** for each loop that needs file access
- **Use ` + "`stateful: true`" + `** for long-running loops
- **Set ` + "`timeout_seconds: 0`" + `** for unlimited runtime
- **Use meaningful variable names** (` + "`$RAW_DATA`" + `, not ` + "`$OUTPUT`" + `)
- **Infer ` + "`allowed_paths`" + ` from user prompt** (e.g., "/tmp/code" → ` + "`allowed_paths: [/tmp/code]`" + `)
- **Default ` + "`checkpoint_interval: 5`" + `** for safety

## 6. Codebase Index Step Definition (` + "`codebase_index`" + `)

This step scans a repository and generates a compact Markdown index optimized for LLM consumption. It supports multiple programming languages and exposes workflow variables for downstream steps.

**When to use codebase-index:**
- When you need to give an LLM context about a codebase structure
- Before code analysis, refactoring, or documentation tasks
- When building workflows that operate on unfamiliar repositories

**Structure:**
` + "```yaml" + `
step_name:
  step_type: codebase-index  # Alternative: use codebase_index block
  codebase_index:
    root: .                   # Repository path (default: current directory)
    output:
      path: .comanda/INDEX.md # Custom output path (optional)
      store: repo             # Where to store: repo, config, or both
      encrypt: false          # Enable AES-256 encryption
    expose:
      workflow_variable: true # Export as workflow variables
      memory:
        enabled: true         # Register as memory source
        key: repo.index       # Memory key name
    adapters:                 # Per-language configuration (optional)
      go:
        ignore_dirs: [vendor, testdata]
        priority_files: ["cmd/**/*.go"]
    max_output_kb: 100        # Maximum output size in KB
` + "```" + `

**` + "`codebase_index`" + ` Block Attributes:**
- ` + "`root`" + `: (string, default: ` + "`.`" + `) Repository path to scan.
- ` + "`output.path`" + `: (string, optional) Custom output file path. Default: ` + "`.comanda/<repo>_INDEX.md`" + `
- ` + "`output.store`" + `: (string, default: ` + "`repo`" + `) Where to save: ` + "`repo`" + ` (in repository), ` + "`config`" + ` (~/.comanda/), or ` + "`both`" + `.
- ` + "`output.encrypt`" + `: (bool, default: false) Encrypt output with AES-256 GCM. Saves as ` + "`.enc`" + ` file. Requires ` + "`COMANDA_INDEX_KEY`" + ` environment variable.
- ` + "`expose.workflow_variable`" + `: (bool, default: true) Export index as workflow variables.
- ` + "`expose.memory.enabled`" + `: (bool, default: false) Register as a named memory source.
- ` + "`expose.memory.key`" + `: (string) Key name for memory access.
- ` + "`adapters`" + `: (map, optional) Per-language configuration overrides.
- ` + "`max_output_kb`" + `: (int, default: 100) Maximum size of generated index.

**Workflow Variables Exported:**

After the step runs, these variables are available (where ` + "`<REPO>`" + ` is the uppercase repository name, e.g., ` + "`src`" + ` becomes ` + "`SRC`" + `):
- ` + "`$<REPO>_INDEX`" + `: Full Markdown content of the index
- ` + "`$<REPO>_INDEX_PATH`" + `: Absolute path to the saved index file
- ` + "`$<REPO>_INDEX_SHA`" + `: Hash of the index content
- ` + "`$<REPO>_INDEX_UPDATED`" + `: ` + "`true`" + ` if index was regenerated

**⚠️ CRITICAL: Referencing the Index in Subsequent Steps**

When a later step needs the codebase index content, you MUST use the exported variable — NOT the file path.

✅ **CORRECT** - Use the exported variable:
` + "```yaml" + `
index_codebase:
  step_type: codebase-index
  codebase_index:
    root: ./src

analyze_codebase:
  input: $SRC_INDEX          # ✅ Use the variable!
  model: claude-code
  action: "Analyze this codebase structure"
  output: STDOUT
` + "```" + `

❌ **WRONG** - Do NOT use the file path directly:
` + "```yaml" + `
index_codebase:
  step_type: codebase-index
  codebase_index:
    root: ./src
    output:
      path: .comanda/INDEX.md

analyze_codebase:
  input: .comanda/INDEX.md    # ❌ WRONG! Path may not resolve correctly
  model: claude-code
  action: "Analyze this codebase structure"
  output: STDOUT
` + "```" + `

**Why?** The ` + "`output.path`" + ` is relative to the repository root (when ` + "`store: repo`" + `), not the current working directory. The exported variable always contains the correct content regardless of where you run the workflow.

**Supported Languages:**
- **Go**: Uses AST parsing. Detection: ` + "`go.mod`" + `, ` + "`go.sum`" + `
- **Python**: Uses regex. Detection: ` + "`pyproject.toml`" + `, ` + "`requirements.txt`" + `, ` + "`setup.py`" + `
- **TypeScript/JavaScript**: Uses regex. Detection: ` + "`tsconfig.json`" + `, ` + "`package.json`" + `
- **Flutter/Dart**: Uses regex. Detection: ` + "`pubspec.yaml`" + `

**Example: Index and Analyze a Codebase**
` + "```yaml" + `
# Step 1: Generate codebase index
index_repo:
  step_type: codebase-index
  codebase_index:
    root: ./my-project
    expose:
      workflow_variable: true

# Step 2: Use the index for analysis
analyze_architecture:
  input: STDIN
  model: claude-code
  action: |
    Here is the codebase index:
    $MY_PROJECT_INDEX

    Analyze the architecture and suggest improvements.
  output: STDOUT
` + "```" + `

**Example: Minimal Usage**
` + "```yaml" + `
index_repo:
  step_type: codebase-index
  codebase_index:
    root: .
` + "```" + `

## Common Elements (for Standard Steps)

### Input Types
- File path: ` + "`input: path/to/file.txt`" + `
- Previous step output: ` + "`input: STDIN`" + `
- Multiple file paths: ` + "`input: [file1.txt, file2.txt]`" + `
- Web scraping: ` + "`input: { url: \"https://example.com\" }`" + ` (Further scrape config under ` + "`scrape_config`" + ` map if needed)
- Database query: ` + "`input: { database: { type: \"postgres\", query: \"SELECT * FROM users\" } }`" + `
- No input: ` + "`input: NA`" + `
- Input with alias for variable: ` + "`input: path/to/file.txt as $my_var`" + `
- List with aliases: ` + "`input: [file1.txt as $file1_content, file2.txt as $file2_content]`" + `

### Chunking
For processing large files, you can use the ` + "`chunk`" + ` configuration to split the input into manageable pieces:

**Basic Structure:**
` + "```yaml" + `
step_name:
  input: "large_file.txt"
  chunk:
    by: lines  # or "tokens"
    size: 1000  # number of lines or tokens per chunk
    overlap: 50  # optional: number of lines or tokens to overlap between chunks
    max_chunks: 10  # optional: limit the total number of chunks processed
  batch_mode: individual  # required for chunking to process each chunk separately
  model: gpt-4o-mini
  action: "Process this chunk of text: {{ current_chunk }}"
  output: "chunk_{{ chunk_index }}_result.txt"  # can use chunk_index in output path
` + "```" + `

**Key Elements:**
- ` + "`chunk`" + `: (Optional) Configuration block for chunking a large input file.
  - ` + "`by`" + `: (Required) Chunking method - either ` + "`lines`" + ` or ` + "`tokens`" + `.
  - ` + "`size`" + `: (Required) Number of lines or tokens per chunk.
  - ` + "`overlap`" + `: (Optional) Number of lines or tokens to include from the previous chunk, providing context continuity.
  - ` + "`max_chunks`" + `: (Optional) Maximum number of chunks to process, useful for testing or limiting processing.
- ` + "`batch_mode: individual`" + `: Required when using chunking to process each chunk as a separate LLM call.
- ` + "`{{ current_chunk }}`" + `: Template variable that gets replaced with the current chunk content in the action.
- ` + "`{{ chunk_index }}`" + `: Template variable for the current chunk number (0-based), useful in output paths.

**Consolidation Pattern:**
A common pattern is to process chunks individually and then consolidate the results:

` + "```yaml" + `
# Step 1: Process chunks
process_chunks:
  input: "large_document.txt"
  chunk:
    by: lines
    size: 1000
  batch_mode: individual
  model: gpt-4o-mini
  action: "Extract key points from: {{ current_chunk }}"
  output: "chunk_{{ chunk_index }}_summary.txt"

# Step 2: Consolidate results
consolidate_results:
  input: "chunk_*.txt"  # Use wildcard to collect all chunk outputs
  model: gpt-4o-mini
  action: "Combine these summaries into one coherent document."
  output: "final_summary.txt"
` + "```" + `

### Models
- Single model: ` + "`model: gpt-4o-mini`" + `
- No model (for non-LLM operations): ` + "`model: NA`" + `
- Multiple models (for comparison): ` + "`model: [gpt-4o-mini, claude-3-opus-20240229]`" + `

### Actions
- Single instruction: ` + "`action: \"Summarize this text.\"`" + `
- Multiple sequential instructions: ` + "`action: [\"Action 1\", \"Action 2\"]`" + `
- Reference variable: ` + "`action: \"Compare with $previous_data.\"`" + `
- Reference markdown file: ` + "`action: path/to/prompt.md`" + `

### Outputs
- Console: ` + "`output: STDOUT`" + `
- File: ` + "`output: results.txt`" + ` or ` + "`output: ./path/to/output.md`" + `
- Database: ` + "`output: { database: { type: \"postgres\", table: \"results_table\" } }`" + `
- Output with alias (if supported for variable creation from output): ` + "`output: STDOUT as $step_output_var`" + `

**⚠️ IMPORTANT: Writing to files**
When the result should be saved to a file, use the ` + "`output:`" + ` field directly. Do NOT instruct the LLM to "write to a file" in the action.

✅ **CORRECT** - Use ` + "`output:`" + ` for file writing:
` + "```yaml" + `
summarize_document:
  input: report.txt
  model: claude-code
  action: "Summarize this document"
  output: ./summary.md
` + "```" + `

❌ **WRONG** - Do NOT tell the LLM to write the file:
` + "```yaml" + `
summarize_document:
  input: report.txt
  model: claude-code
  action: "Summarize this document and write it to ./summary.md"
  output: STDOUT
` + "```" + `

## Variables
- Definition: ` + "`input: data.txt as $initial_data`" + `
- Reference: ` + "`action: \"Compare this analysis with $initial_data\"`" + `
- Scope: Variables are typically scoped to the workflow. For ` + "`process`" + ` steps, parent variables are not directly accessible by default; use the ` + "`process.inputs`" + ` map to pass data.

## Validation Rules Summary (for LLM)

1.  A step definition must clearly be one of: Standard, Generate, or Process.
    *   A step cannot mix top-level keys from different types (e.g., a ` + "`generate`" + ` step should not have a top-level ` + "`model`" + ` or ` + "`output`" + ` key; these belong inside the ` + "`generate`" + ` block).
2.  **Standard Step:**
    *   Must contain ` + "`input`" + `, ` + "`model`" + `, ` + "`action`" + `, ` + "`output`" + ` (unless ` + "`type: openai-responses`" + `, where ` + "`action`" + ` might be replaced by ` + "`instructions`" + `).
    *   ` + "`input`" + ` can be ` + "`NA`" + `. ` + "`model`" + ` can be ` + "`NA`" + `.
3.  **Generate Step:**
    *   Must contain a ` + "`generate`" + ` block.
    *   ` + "`generate`" + ` block must contain ` + "`action`" + ` (string prompt) and ` + "`output`" + ` (string filename).
    *   ` + "`generate.model`" + ` is optional (uses default if omitted).
    *   Top-level ` + "`input`" + ` for the step is optional (can be ` + "`NA`" + ` or provide context).
4.  **Process Step:**
    *   Must contain a ` + "`process`" + ` block.
    *   ` + "`process`" + ` block must contain ` + "`workflow_file`" + ` (string path).
    *   ` + "`process.inputs`" + ` is optional.
    *   Top-level ` + "`input`" + ` for the step is optional (can be ` + "`NA`" + ` or ` + "`STDIN`" + ` to pipe to sub-workflow).
5.  **Agentic Loop Step (Inline):**
    *   Must contain an ` + "`agentic_loop`" + ` block with loop configuration.
    *   Must also contain ` + "`input`" + `, ` + "`model`" + `, ` + "`action`" + `, ` + "`output`" + ` at the step level.
    *   ` + "`agentic_loop.max_iterations`" + ` defaults to 10 if not specified.
    *   ` + "`agentic_loop.exit_condition`" + ` can be ` + "`llm_decides`" + ` or ` + "`pattern_match`" + `.
6.  **Agentic Loop Block (Top-level):**
    *   Uses ` + "`agentic-loop:`" + ` as a top-level key (like ` + "`parallel-process:`" + `).
    *   Must contain ` + "`config`" + ` block with loop settings.
    *   Must contain ` + "`steps`" + ` block with one or more sub-steps.
    *   Each sub-step follows standard step structure (` + "`input`" + `, ` + "`model`" + `, ` + "`action`" + `, ` + "`output`" + `).
7.  **Codebase Index Step:**
    *   Must have ` + "`step_type: codebase-index`" + ` OR contain a ` + "`codebase_index`" + ` block.
    *   ` + "`codebase_index.root`" + ` defaults to ` + "`.`" + ` (current directory).
    *   Exports workflow variables: ` + "`<REPO>_INDEX`" + `, ` + "`<REPO>_INDEX_PATH`" + `, ` + "`<REPO>_INDEX_SHA`" + `, ` + "`<REPO>_INDEX_UPDATED`" + `.
    *   Does not require ` + "`input`" + `, ` + "`model`" + `, ` + "`action`" + `, or ` + "`output`" + ` fields.

## Chaining and Examples

Steps can be "chained together" by either passing STDOUT from one step to STDIN of the next step or by writing to a file and then having subsequent steps take this file as input.

**Meta-Processing Example:**
` + "```yaml" + `
gather_requirements:
  input: requirements_document.txt
  model: claude-3-opus-20240229
  action: "Based on the input document, define the core tasks for a data processing workflow. Output as a concise list."
  output: STDOUT

generate_data_workflow:
  input: STDIN # Using output from previous step as context
  generate:
    model: gpt-4o-mini # LLM to generate the workflow
    action: "Generate a Comanda workflow YAML to perform the tasks described in the input. The workflow should read 'raw_data.csv', perform transformations, and save to 'processed_data.csv'."
    output: dynamic_data_processor.yaml # Filename for the generated workflow

execute_data_workflow:
  input: NA # Or potentially STDIN if dynamic_data_processor.yaml's first step expects it
  process:
    workflow_file: dynamic_data_processor.yaml # Execute the generated workflow
    # inputs: { source_file: "override_data.csv" } # Optional: override inputs for the sub-workflow
  output: STDOUT # Log output of the process step itself (e.g., success/failure)
` + "```" + `

### Advanced Chaining: Enabling Independent Analysis with Files

The standard ` + "`STDIN`" + `/` + "`STDOUT`" + ` chain is designed for sequential processing, where each step receives the output of the one immediately before it. However, many workflows require a downstream step to **independently analyze outputs from multiple, potentially non-sequential, upstream steps.**

To enable this, you must use files to store intermediate results. This pattern ensures that each output is preserved and can be accessed directly by any subsequent step, rather than being lost in a pipeline.

**The recommended pattern is:**
1.  Each upstream step saves its result to a distinct file (e.g., ` + "`step1_output.txt`" + `, ` + "`step2_output.txt`" + `).
2.  The downstream step that needs to perform the independent analysis lists these files as its ` + "`input`" + `.

**Example: A 3-Step Workflow with a Final Review**

In this scenario, the third step needs to review the outputs of both the first and second steps independently.

` + "```yaml" + `
# Step 1: Initial analysis
analyze_introductions:
  input: introductions.md
  model: gpt-4o-mini
  action: "Perform a detailed analysis of the introductions document. Focus on key themes, writing style, and effectiveness."
  output: step1_analysis.txt

# Step 2: Quality assessment of the original document
quality_assessment:
  input: introductions.md
  model: gpt-4o-mini
  action: "Perform a quality assessment on the original document. Identify strengths and potential gaps."
  output: step2_qa.txt

# Step 3: Final summary based on both outputs
final_summary:
  input: [step1_analysis.txt, step2_qa.txt]
  model: gpt-4o-mini
  action: "Review the results from the analysis (step1_analysis.txt) and the QA (step2_qa.txt). Provide a comprehensive summary that synthesizes the findings from both."
  output: final_summary.md
` + "```" + `

This file-based approach is the correct way to handle any workflow where a step's logic depends on having discrete access to multiple prior outputs.

## CRITICAL: Workflow Simplicity Guidelines

**ALWAYS prefer the simplest possible workflow.** Over-engineered workflows are harder to debug, maintain, and understand.

**Key principles:**
1. **Minimize steps**: If a task can be done in 1 step, don't use 3. Most tasks need 1-2 steps.
2. **Avoid unnecessary chaining**: Don't chain steps unless the output of one is genuinely needed by the next.
3. **Use direct file I/O**: If you need to read a file and process it, that's ONE step, not three.
4. **Prefer STDIN/STDOUT**: Use simple STDIN/STDOUT chaining over complex file intermediates when sequential processing suffices.
5. **One model per workflow when possible**: Don't use multiple models unless comparing outputs or the task genuinely requires different capabilities.

**Examples of OVER-ENGINEERED workflows (AVOID):**
` + "```yaml" + `
# BAD: Too many steps for a simple task
read_file:
  input: document.txt
  model: NA
  action: NA
  output: temp_content.txt

analyze_content:
  input: temp_content.txt
  model: gpt-4o-mini
  action: "Analyze this"
  output: temp_analysis.txt

format_output:
  input: temp_analysis.txt
  model: gpt-4o-mini
  action: "Format nicely"
  output: STDOUT
` + "```" + `

**GOOD: Simple and direct:**
` + "```yaml" + `
# GOOD: One step does the job
analyze_document:
  input: document.txt
  model: gpt-4o-mini
  action: "Analyze this document and format the output nicely"
  output: STDOUT
` + "```" + `

**When multiple steps ARE appropriate:**
- Processing different source files independently, then combining results
- Using tool commands to pre-process data before LLM analysis
- Generating a workflow dynamically, then executing it
- Tasks that genuinely require different models for different capabilities
- **Agentic loops** for iterative refinement, planning, or autonomous decision-making

**When to use Agentic Loops:**
- Code improvement cycles (analyze → fix → verify)
- Planning and execution workflows
- Tasks where quality depends on iteration
- When the LLM should decide when work is complete

This guide covers the core concepts and syntax of Comanda's YAML DSL, including meta-processing capabilities. LLMs should use this structure to generate valid workflow files.`

// embeddedLLMGuideTemplate is the template for the Comanda YAML DSL Guide
// It includes a placeholder for the supported models
const embeddedLLMGuideTemplate = `# Comanda YAML DSL Guide (for LLM Consumption)

This guide specifies the YAML-based Domain Specific Language (DSL) for Comanda workflows, enabling LLMs to generate valid workflow files.

## ⚠️ CRITICAL RULES - READ FIRST ⚠️

Before generating any workflow, you MUST follow these rules:

1. **MODEL RESTRICTION**: You may ONLY use models from the "Supported Models" list in this guide. DO NOT invent model names.
2. **STEP NAMING**: Every step name must be descriptive (e.g., ` + "`extract_customer_emails`" + `, NOT ` + "`step_1`" + `).
3. **SIMPLICITY**: Use the minimum number of steps needed. Most tasks need 1-2 steps.

## Overview

Comanda workflows consist of one or more named steps. Each step performs an operation. There are six main types of steps:
1.  **Standard Processing Step:** Involves LLMs, file processing, data operations.
2.  **Generate Step:** Uses an LLM to dynamically create a new Comanda workflow YAML file.
3.  **Process Step:** Executes another Comanda workflow file (static or dynamically generated).
4.  **Agentic Loop Step:** Iteratively processes until an exit condition is met (for refinement, planning, autonomous tasks).
5.  **Multi-Loop Orchestration:** Coordinates multiple interdependent agentic loops with variable passing and creator/checker patterns.
6.  **Codebase Index Step:** Scans a repository and generates a compact Markdown index for LLM consumption.

## Core Workflow Structure

A Comanda workflow is a YAML map where each key is a ` + "`step_name`" + ` (string, user-defined), mapping to a dictionary defining the step.

` + "```yaml" + `
# Example of a workflow structure (note: descriptive step names!)
extract_customer_emails:
  # ... step definition ...
summarize_quarterly_report:
  # ... step definition ...
` + "```" + `

### Step Naming Requirements

**Step names MUST be descriptive and meaningful.** They should clearly describe what the step does.

**✅ GOOD step names** (use these patterns):
- ` + "`extract_customer_emails`" + ` - describes the extraction action and target
- ` + "`summarize_quarterly_report`" + ` - describes action and document type
- ` + "`validate_json_schema`" + ` - describes the validation being performed
- ` + "`translate_to_spanish`" + ` - describes transformation and target language
- ` + "`analyze_sentiment_scores`" + ` - describes analysis type
- ` + "`generate_markdown_index`" + ` - describes output format

**❌ BAD step names** (DO NOT use these):
- ` + "`step_1`" + `, ` + "`step_2`" + `, ` + "`step_3`" + ` - meaningless numbers
- ` + "`analyze`" + `, ` + "`process`" + `, ` + "`transform`" + ` - too generic
- ` + "`do_thing`" + `, ` + "`run_llm`" + `, ` + "`handle_data`" + ` - vague and unhelpful
- ` + "`first`" + `, ` + "`second`" + `, ` + "`final`" + ` - positional, not descriptive

**Naming pattern:** ` + "`<verb>_<object>_<qualifier>`" + ` where:
- **verb**: extract, summarize, validate, translate, analyze, generate, filter, merge, compare
- **object**: what is being acted on (emails, report, schema, data, logs)
- **qualifier**: optional specifics (quarterly, customer, json, spanish)

## 1. Standard Processing Step Definition

This is the most common step type.

**Basic Structure:**
` + "```yaml" + `
step_name:
  input: [input source]
  model: [model name]
  action: [action to perform / prompt provided]
  output: [output destination]
  type: [optional, e.g., "openai-responses"] # Specifies specialized handling
  batch_mode: [individual|combined] # Optional, for multi-file inputs
  skip_errors: [true|false] # Optional, for multi-file inputs
  # ... other type-specific fields for "openai-responses" like 'instructions', 'tools', etc.
` + "```" + `

**Key Elements:**
- ` + "`input`" + `: (Required for most, can be ` + "`NA`" + `) Source of data. See "Input Types".
- ` + "`model`" + `: (Required, can be ` + "`NA`" + `) LLM model to use. See "Models".
- ` + "`action`" + `: (Required for most) Instructions or operations. See "Actions".
- ` + "`output`" + `: (Required) Destination for results. See "Outputs".
- ` + "`type`" + `: (Optional) Specifies a specialized handler for the step, e.g., ` + "`openai-responses`" + `. If omitted, it's a general-purpose LLM or NA step.
- ` + "`batch_mode`" + `: (Optional, default: ` + "`combined`" + `) For steps with multiple file inputs, defines if files are processed ` + "`combined`" + ` into one LLM call or ` + "`individual`" + `ly.
- ` + "`skip_errors`" + `: (Optional, default: ` + "`false`" + `) If ` + "`batch_mode: individual`" + `, determines if processing continues if one file fails.

**OpenAI Responses API Specific Fields (used when ` + "`type: openai-responses`" + `):**
- ` + "`instructions`" + `: (string) System message for the LLM.
- ` + "`tools`" + `: (list of maps) Configuration for tools/functions the LLM can call.
- ` + "`previous_response_id`" + `: (string) ID of a previous response for maintaining conversation state.
- ` + "`max_output_tokens`" + `: (int) Token limit for the LLM response.
- ` + "`temperature`" + `: (float) Sampling temperature.
- ` + "`top_p`" + `: (float) Nucleus sampling (top-p).
- ` + "`stream`" + `: (bool) Whether to stream the response.
- ` + "`response_format`" + `: (map) Specifies response format, e.g., ` + "`{ type: \"json_object\" }`" + `.


## 2. Generate Step Definition (` + "`generate`" + `)

This step uses an LLM to dynamically create a new Comanda workflow YAML file.

**Structure:**
` + "```yaml" + `
step_name_for_generation:
  input: [optional_input_source_for_context, or NA] # e.g., STDIN, a file with requirements
  generate:
    model: [llm_model_for_generation, optional] # e.g., gpt-4o-mini. Uses default if omitted.
    action: [prompt_for_workflow_generation] # Natural language instruction for the LLM.
    output: [filename_for_generated_yaml] # e.g., new_workflow.yaml
    context_files: [list_of_files_for_additional_context, optional] # e.g., [schema.txt, examples.yaml]
` + "```" + `
**` + "`generate`" + ` Block Attributes:**
- ` + "`model`" + `: (string, optional) Specifies the LLM to use for generation. If omitted, uses the ` + "`default_generation_model`" + ` configured in Comanda. You can set or update this default model by running ` + "`comanda configure`" + ` and following the prompts for setting a default generation model.
- ` + "`action`" + `: (string, required) The natural language instruction given to the LLM to guide the workflow generation.
- ` + "`output`" + `: (string, required) The filename where the generated Comanda workflow YAML file will be saved.
- ` + "`context_files`" + `: (list of strings, optional) A list of file paths to provide as additional context to the LLM, beyond the standard Comanda DSL guide (which is implicitly included).
- **Note:** The ` + "`input`" + ` field for a ` + "`generate`" + ` step is optional. If provided (e.g., ` + "`STDIN`" + ` or a file path), its content will be added to the context for the LLM generating the workflow. If not needed, use ` + "`input: NA`" + `.

## 3. Process Step Definition (` + "`process`" + `)

This step executes another Comanda workflow file.

**Structure:**
` + "```yaml" + `
step_name_for_processing:
  input: [optional_input_source_for_sub_workflow, or NA] # e.g., STDIN to pass data to the sub-workflow
  process:
    workflow_file: [path_to_comanda_yaml_to_execute] # e.g., generated_workflow.yaml or existing_flow.yaml
    inputs: {key1: value1, key2: value2, optional} # Map of inputs to pass to the sub-workflow.
    # capture_outputs: [list_of_outputs_to_capture, optional] # Future: Define how to capture specific outputs.
` + "```" + `
**` + "`process`" + ` Block Attributes:**
- ` + "`workflow_file`" + `: (string, required) The path to the Comanda workflow YAML file to be executed. This can be a statically defined path or the output of a ` + "`generate`" + ` step.
- ` + "`inputs`" + `: (map, optional) A map of key-value pairs to pass as initial variables to the sub-workflow. These can be accessed within the sub-workflow (e.g., as ` + "`$parent.key1`" + `).
- **Note:** The ` + "`input`" + ` field for a ` + "`process`" + ` step is optional. If ` + "`input: STDIN`" + ` is used, the output of the previous step in the parent workflow will be available as the initial ` + "`STDIN`" + ` for the *first* step of the sub-workflow if that first step expects ` + "`STDIN`" + `.

## 4. Agentic Loop Step Definition (` + "`agentic_loop`" + ` / ` + "`agentic-loop`" + `)

Agentic loops enable iterative LLM processing until an exit condition is met. This is powerful for tasks that require refinement, multi-step reasoning, or autonomous decision-making.

**When to use agentic loops:**
- Iterative code improvement (analyze → fix → verify cycles)
- Multi-step planning and execution
- Tasks where the LLM decides when work is complete
- Refinement workflows (draft → improve → finalize)

### Inline Syntax (Single-Step Loop)

For simple iterative tasks with a single step:

` + "```yaml" + `
step_name:
  agentic_loop:
    max_iterations: 5           # Safety limit (default: 10)
    exit_condition: pattern_match  # or "llm_decides"
    exit_pattern: "COMPLETE"    # Regex pattern for pattern_match
  input: STDIN
  model: claude-code
  action: |
    Iteration {{ loop.iteration }}.
    Previous work: {{ loop.previous_output }}

    Continue improving. Say COMPLETE when done.
  output: STDOUT
` + "```" + `

### Block Syntax (Multi-Step Loop)

For complex loops with multiple sub-steps per iteration:

` + "```yaml" + `
agentic-loop:
  config:
    max_iterations: 5           # Safety limit (default: 10)
    timeout_seconds: 300        # Total timeout (default: 300)
    exit_condition: llm_decides # or "pattern_match"
    exit_pattern: "DONE"        # For pattern_match
    context_window: 3           # Past iterations to include (default: 5)

  steps:
    plan:
      input: STDIN
      model: claude-code
      action: |
        Iteration {{ loop.iteration }}.
        Previous: {{ loop.previous_output }}

        Plan next steps. Say DONE if complete.
      output: $PLAN

    execute:
      input: $PLAN
      model: claude-code
      action: "Execute the plan"
      output: STDOUT
` + "```" + `

**` + "`agentic_loop`" + ` Configuration:**

**Core Settings:**
- ` + "`max_iterations`" + `: (int, default: 10) Maximum iterations before stopping.
- ` + "`timeout_seconds`" + `: (int, default: 0) Total time limit in seconds. **0 = no timeout** (loop runs until max_iterations or exit condition).
- ` + "`exit_condition`" + `: (string) How to detect completion:
  - ` + "`llm_decides`" + `: Exits when output contains "DONE", "COMPLETE", or "FINISHED"
  - ` + "`pattern_match`" + `: Exits when output matches ` + "`exit_pattern`" + ` regex
- ` + "`exit_pattern`" + `: (string) Regex pattern for ` + "`pattern_match`" + ` condition.
- ` + "`context_window`" + `: (int, default: 5) Number of past iterations to include in context.

**File Access (REQUIRED for Claude Code):**
- ` + "`allowed_paths`" + `: (list, **REQUIRED for file operations**) Directories where Claude Code can use tools (Read, Write, Edit, Bash, etc.). **Without this, Claude Code runs in print-only mode and CANNOT read/write files.** When generating workflows, infer paths from: the ` + "`codebase_index.root`" + ` if present, file paths mentioned in the action, or use ` + "`[.]`" + ` (current directory) as fallback.
- ` + "`tools`" + `: (list, optional) Restrict available tools (e.g., ` + "`[Read, Glob, Grep]`" + ` for read-only access). If omitted, all tools are available.

**State Persistence (for Long-Running Loops):**
- ` + "`name`" + `: (string, **REQUIRED for stateful loops**) Unique identifier for the loop. Enables state persistence and resume capability.
- ` + "`stateful`" + `: (bool, default: false) Enable state persistence to ` + "`~/.comanda/loop-states/{name}.json`" + `. Allows resuming after interruption.
- ` + "`checkpoint_interval`" + `: (int, default: 5) Save state every N iterations. Lower values = more frequent saves = safer resume.

**Quality Gates (Automated Validation):**
- ` + "`quality_gates`" + `: (list, optional) Automated checks to run after each iteration. Each gate validates loop output and can trigger retry/abort/skip actions.

**Quality Gate Configuration:**
` + "```yaml" + `
quality_gates:
  - name: typecheck           # Gate name
    command: "npm run typecheck"  # Shell command to execute
    on_fail: retry            # Action: retry, abort, or skip
    timeout: 60               # Timeout in seconds
    retry:                    # Retry configuration (optional)
      max_attempts: 3         # Maximum retry attempts
      backoff_type: exponential  # linear or exponential
      initial_delay: 5        # Initial delay in seconds

  - name: security
    type: security            # Built-in gate type (syntax, security, test)
    on_fail: abort

  - name: tests
    command: "npm test"
    on_fail: skip             # Continue even if this fails
` + "```" + `

**Built-in Quality Gate Types:**
- ` + "`syntax`" + `: Checks for syntax errors (Python, JS, Go, etc.)
- ` + "`security`" + `: Scans for hardcoded secrets, security issues
- ` + "`test`" + `: Runs test commands with coverage reporting

**Quality Gate Actions (` + "`on_fail`" + `):**
- ` + "`retry`" + `: Retry the gate with backoff (exponential or linear)
- ` + "`abort`" + `: Stop loop immediately and save state as "failed"
- ` + "`skip`" + `: Log warning and continue to next iteration

**Template Variables in Actions:**
- ` + "`{{ loop.iteration }}`" + `: Current iteration number (1-based)
- ` + "`{{ loop.previous_output }}`" + `: Output from previous iteration
- ` + "`{{ loop.total_iterations }}`" + `: Maximum allowed iterations
- ` + "`{{ loop.elapsed_seconds }}`" + `: Seconds since loop started

**Example: Agentic Code Exploration**
` + "```yaml" + `
explore_codebase:
  agentic_loop:
    max_iterations: 3
    exit_condition: llm_decides
    allowed_paths: [./src, ./tests]
    tools: [Read, Glob, Grep]  # Read-only access
  input: STDIN
  model: claude-code
  action: |
    Explore the codebase and answer: {{ loop.previous_output }}
    Say DONE when you have the answer.
  output: STDOUT
` + "```" + `

**Example: Iterative Code Implementation**
` + "```yaml" + `
implement:
  agentic_loop:
    max_iterations: 3
    exit_condition: pattern_match
    exit_pattern: "SATISFIED"
    allowed_paths: [.]  # Required for file operations
  input: STDIN
  model: claude-code
  action: |
    Iteration {{ loop.iteration }}. Implement and improve the code.
    Previous: {{ loop.previous_output }}

    Add error handling, edge cases, tests.
    Say SATISFIED when production-ready.
  output: STDOUT
` + "```" + `

**Example: Plan and Build Loop**
` + "```yaml" + `
agentic-loop:
  config:
    max_iterations: 5
    exit_condition: llm_decides
    allowed_paths: [.]  # Required for file operations

  steps:
    plan:
      input: STDIN
      model: claude-code
      action: |
        Iteration {{ loop.iteration }}.
        Create/refine the implementation plan.
        Say DONE when ready to implement.
      output: $PLAN

    build:
      input: $PLAN
      model: claude-code
      action: "Generate code based on the plan"
      output: STDOUT
` + "```" + `

**Example: Long-Running Loop with State Persistence**
` + "```yaml" + `
agentic-loop:
  config:
    name: code-refactor-loop      # Required for stateful loops
    stateful: true                 # Enable state persistence
    max_iterations: 50
    timeout_seconds: 0             # No timeout - run until complete
    checkpoint_interval: 5         # Save every 5 iterations
    exit_condition: llm_decides
    allowed_paths: [./src]

    quality_gates:
    name: syntax-check
        type: syntax
        on_fail: retry
        retry:
          max_attempts: 3
          backoff_type: exponential
          initial_delay: 2

    name: tests
        command: "npm test"
        on_fail: abort              # Stop if tests fail
        timeout: 300

  steps:
    analyze:
      input: STDIN
      model: claude-code
      action: |
        Iteration {{ loop.iteration }} of {{ loop.total_iterations }}.
        Analyze code and refactor one module.
        Say DONE when all modules are refactored.
      output: STDOUT
` + "```" + `

**Example: Quality Gates with Retry**
` + "```yaml" + `
code_improvement:
  agentic_loop:
    name: improve-code
    stateful: true
    max_iterations: 10
    allowed_paths: [.]

    quality_gates:
    name: typecheck
        command: "npm run typecheck"
        on_fail: retry
        timeout: 60
        retry:
          max_attempts: 3
          backoff_type: exponential
          initial_delay: 5

    name: lint
        command: "npm run lint"
        on_fail: skip               # Non-critical, continue

  input: STDIN
  model: claude-code
  action: "Improve code quality. Say COMPLETE when done."
  output: STDOUT
` + "```" + `

**CRITICAL: When generating agentic_loop workflows with Claude Code:**
- **ALWAYS include ` + "`allowed_paths`" + `** - Claude Code CANNOT read/write files without it
- If the workflow uses ` + "`codebase_index`" + `, use the same ` + "`root`" + ` directory in ` + "`allowed_paths`" + `
- If the action mentions specific file paths or directories, include those directories
- When in doubt, use ` + "`allowed_paths: [.]`" + ` for current directory access
- Forgetting ` + "`allowed_paths`" + ` is a common error that causes "permission denied" failures
- For long-running tasks (hours/days), use ` + "`stateful: true`" + ` and ` + "`timeout_seconds: 0`" + `
- **Default to ` + "`claude-code`" + ` model for agentic workflows** - it provides the best tool use and autonomous capabilities

## 5. Multi-Loop Orchestration

Multi-loop orchestration enables complex autonomous workflows with multiple interdependent agentic loops. This is essential for creator/checker patterns, sequential processing pipelines, and complex task decomposition.

**When to use multi-loop orchestration:**
- Creator/checker validation workflows (loop A creates, loop B validates, rerun A if validation fails)
- Sequential data processing (collect → analyze → report)
- Complex task decomposition (break large task into specialized sub-loops)
- Workflows requiring variable passing between autonomous agents

### Named Loops Syntax

Define multiple named loops with dependencies and variable passing:

` + "```yaml" + `
loops:
  data-collector:
    name: data-collector
    stateful: true
    max_iterations: 10
    timeout_seconds: 0
    exit_condition: llm_decides
    allowed_paths: [.]
    output_state: $RAW_DATA         # Export result to variable

    steps:
    collect:
          input: STDIN
          model: claude-code
          action: "Collect data and say DONE when complete"
          output: STDOUT

  data-analyzer:
    name: data-analyzer
    depends_on: [data-collector]    # Wait for collector to complete
    input_state: $RAW_DATA           # Read collector's output
    stateful: true
    max_iterations: 5
    exit_condition: llm_decides
    allowed_paths: [.]
    output_state: $ANALYSIS          # Export analysis result

    steps:
    analyze:
          input: STDIN
          model: claude-code
          action: |
            Analyze this data: {{ loop.previous_output }}
            Say DONE when analysis is complete.
          output: STDOUT

  report-generator:
    name: report-generator
    depends_on: [data-analyzer]     # Wait for analyzer
    input_state: $ANALYSIS           # Read analyzer's output
    max_iterations: 1
    allowed_paths: [.]

    steps:
    generate:
          input: STDIN
          model: claude-code
          action: "Create final report based on analysis"
          output: STDOUT

# Execute loops in dependency order (topological sort)
execute_loops:
  - data-collector
  - data-analyzer
  - report-generator
` + "```" + `

**Key Configuration Fields:**
- ` + "`loops`" + `: (map) Named loop definitions
- ` + "`depends_on`" + `: (list) Loops that must complete before this one starts
- ` + "`input_state`" + `: (string) Variable to read as input (e.g., ` + "`$RAW_DATA`" + `)
- ` + "`output_state`" + `: (string) Variable to export result to (e.g., ` + "`$ANALYSIS`" + `)
- ` + "`execute_loops`" + `: (list) Simple execution order (dependencies override this)

### Creator/Checker Pattern

Advanced workflow pattern where a creator loop implements features and a checker loop validates them, with automatic rerun on failure:

` + "```yaml" + `
loops:
  # Creator loop: implements features
  feature-creator:
    name: feature-creator
    stateful: true
    max_iterations: 3
    timeout_seconds: 0
    exit_condition: llm_decides
    allowed_paths: [.]
    output_state: $CODE

    quality_gates:
    name: syntax
        type: syntax
        on_fail: abort

    steps:
    implement:
          input: STDIN
          model: claude-code
          action: |
            Iteration {{ loop.iteration }}.
            Implement the requested feature.
            Say DONE when implementation is complete.
          output: STDOUT

  # Checker loop: validates implementation
  code-checker:
    name: code-checker
    depends_on: [feature-creator]   # Wait for creator
    input_state: $CODE               # Read creator's output
    max_iterations: 1
    exit_condition: pattern_match
    exit_pattern: "^PASS"            # Exit when output starts with PASS

    steps:
    review:
          input: STDIN
          model: claude-code
          action: |
            Review this implementation: {{ loop.previous_output }}

            Check for:
            - Correctness
            - Edge cases
            - Code quality
            - Test coverage

            Output: PASS if acceptable, or FAIL with specific issues.
          output: STDOUT

# Advanced workflow with creator/checker relationship
workflow:
  creator:
    type: loop
    loop: feature-creator
    role: creator

  checker:
    type: loop
    loop: code-checker
    role: checker
    validates: creator               # This loop validates the creator
    on_fail: rerun_creator          # Auto-rerun creator if validation fails
` + "```" + `

**Workflow Node Configuration:**
- ` + "`type`" + `: Always ` + "`loop`" + ` for loop nodes
- ` + "`loop`" + `: Name of the loop to execute
- ` + "`role`" + `: Node role (` + "`creator`" + `, ` + "`checker`" + `, ` + "`finalizer`" + `)
- ` + "`validates`" + `: Name of the node this checker validates
- ` + "`on_fail`" + `: Action on validation failure:
  - ` + "`rerun_creator`" + `: Automatically rerun the creator loop (max 3 attempts)
  - ` + "`abort`" + `: Stop workflow immediately
  - ` + "`manual`" + `: Return for manual review

**Validation Logic:**
- Checker loop output is scanned for "PASS" or "FAIL"
- If "PASS" found → workflow continues
- If "FAIL" found and ` + "`on_fail: rerun_creator`" + ` → creator reruns with checker feedback
- Maximum 3 rerun attempts (prevents infinite loops)

### Dependency Graph Execution

Comanda automatically executes loops in correct order using topological sort:

1. **Build dependency graph** from ` + "`depends_on`" + ` relationships
2. **Detect cycles** - fail fast if circular dependencies exist
3. **Topological sort** using Kahn's algorithm
4. **Execute in order** - each loop waits for its dependencies

**Cycle Detection Example:**
` + "```yaml" + `
loops:
  loop-a:
    depends_on: [loop-b]

  loop-b:
    depends_on: [loop-a]

execute_loops:
  - loop-a
  - loop-b

# Error: dependency cycle detected: loop-a -> loop-b -> loop-a
` + "```" + `

### Variable Passing

Variables flow between loops via ` + "`input_state`" + ` and ` + "`output_state`" + `:

` + "```yaml" + `
loops:
  producer:
    output_state: $MY_DATA          # Write to variable

  consumer:
    depends_on: [producer]
    input_state: $MY_DATA            # Read from variable
` + "```" + `

**Variable Rules:**
- Variables are stored in memory during workflow execution
- ` + "`output_state`" + ` exports loop's final output to a variable
- ` + "`input_state`" + ` reads variable as loop input
- Variables persist across loop boundaries
- Missing variable causes error (check dependencies)

### Complete Example: Codebase Analysis Pipeline

` + "```yaml" + `
# Multi-loop workflow: analyze codebase → identify issues → generate report
loops:
  codebase-analyzer:
    name: codebase-analyzer
    stateful: true
    max_iterations: 20
    timeout_seconds: 0
    exit_condition: llm_decides
    allowed_paths: [/tmp/code]
    output_state: $ANALYSIS

    steps:
    analyze:
          input: NA
          model: claude-code
          action: |
            Analyze the codebase at /tmp/code.
            Find patterns, architecture issues, dependencies.
            Say DONE when analysis is complete.
          output: STDOUT

  tech-debt-finder:
    name: tech-debt-finder
    depends_on: [codebase-analyzer]
    input_state: $ANALYSIS
    stateful: true
    max_iterations: 10
    timeout_seconds: 0
    exit_condition: llm_decides
    allowed_paths: [/tmp/code]
    output_state: $TECH_DEBT

    steps:
    identify:
          input: STDIN
          model: claude-code
          action: |
            Based on this analysis: {{ loop.previous_output }}

            Identify tech debt and anti-patterns:
            - Code smells
            - Security issues
            - Performance bottlenecks
            - Maintainability problems

            Say DONE when complete.
          output: STDOUT

  report-writer:
    name: report-writer
    depends_on: [tech-debt-finder]
    input_state: $TECH_DEBT
    max_iterations: 1
    allowed_paths: [.]

    steps:
    write:
          input: STDIN
          model: claude-code
          action: |
            Create a comprehensive markdown report:
            {{ loop.previous_output }}

            Save to ./tech-debt-report.md
          output: STDOUT

execute_loops:
  - codebase-analyzer
  - tech-debt-finder
  - report-writer
` + "```" + `

**CRITICAL: When generating multi-loop workflows:**
- **Use ` + "`claude-code`" + ` as default model** for agentic capabilities
- **Always include ` + "`allowed_paths`" + `** for each loop that needs file access
- **Use ` + "`stateful: true`" + `** for long-running loops
- **Set ` + "`timeout_seconds: 0`" + `** for unlimited runtime
- **Use meaningful variable names** (` + "`$RAW_DATA`" + `, not ` + "`$OUTPUT`" + `)
- **Infer ` + "`allowed_paths`" + ` from user prompt** (e.g., "/tmp/code" → ` + "`allowed_paths: [/tmp/code]`" + `)
- **Default ` + "`checkpoint_interval: 5`" + `** for safety

## 6. Codebase Index Step Definition (` + "`codebase_index`" + `)

This step scans a repository and generates a compact Markdown index optimized for LLM consumption. It supports multiple programming languages and exposes workflow variables for downstream steps.

**When to use codebase-index:**
- When you need to give an LLM context about a codebase structure
- Before code analysis, refactoring, or documentation tasks
- When building workflows that operate on unfamiliar repositories

**Structure:**
` + "```yaml" + `
step_name:
  step_type: codebase-index  # Alternative: use codebase_index block
  codebase_index:
    root: .                   # Repository path (default: current directory)
    output:
      path: .comanda/INDEX.md # Custom output path (optional)
      store: repo             # Where to store: repo, config, or both
      encrypt: false          # Enable AES-256 encryption
    expose:
      workflow_variable: true # Export as workflow variables
      memory:
        enabled: true         # Register as memory source
        key: repo.index       # Memory key name
    adapters:                 # Per-language configuration (optional)
      go:
        ignore_dirs: [vendor, testdata]
        priority_files: ["cmd/**/*.go"]
    max_output_kb: 100        # Maximum output size in KB
` + "```" + `

**` + "`codebase_index`" + ` Block Attributes:**
- ` + "`root`" + `: (string, default: ` + "`.`" + `) Repository path to scan.
- ` + "`output.path`" + `: (string, optional) Custom output file path. Default: ` + "`.comanda/<repo>_INDEX.md`" + `
- ` + "`output.store`" + `: (string, default: ` + "`repo`" + `) Where to save: ` + "`repo`" + ` (in repository), ` + "`config`" + ` (~/.comanda/), or ` + "`both`" + `.
- ` + "`output.encrypt`" + `: (bool, default: false) Encrypt output with AES-256 GCM. Saves as ` + "`.enc`" + ` file. Requires ` + "`COMANDA_INDEX_KEY`" + ` environment variable.
- ` + "`expose.workflow_variable`" + `: (bool, default: true) Export index as workflow variables.
- ` + "`expose.memory.enabled`" + `: (bool, default: false) Register as a named memory source.
- ` + "`expose.memory.key`" + `: (string) Key name for memory access.
- ` + "`adapters`" + `: (map, optional) Per-language configuration overrides.
- ` + "`max_output_kb`" + `: (int, default: 100) Maximum size of generated index.

**Workflow Variables Exported:**

After the step runs, these variables are available (where ` + "`<REPO>`" + ` is the uppercase repository name, e.g., ` + "`src`" + ` becomes ` + "`SRC`" + `):
- ` + "`$<REPO>_INDEX`" + `: Full Markdown content of the index
- ` + "`$<REPO>_INDEX_PATH`" + `: Absolute path to the saved index file
- ` + "`$<REPO>_INDEX_SHA`" + `: Hash of the index content
- ` + "`$<REPO>_INDEX_UPDATED`" + `: ` + "`true`" + ` if index was regenerated

**⚠️ CRITICAL: Referencing the Index in Subsequent Steps**

When a later step needs the codebase index content, you MUST use the exported variable — NOT the file path.

✅ **CORRECT** - Use the exported variable:
` + "```yaml" + `
index_codebase:
  step_type: codebase-index
  codebase_index:
    root: ./src

analyze_codebase:
  input: $SRC_INDEX          # ✅ Use the variable!
  model: claude-code
  action: "Analyze this codebase structure"
  output: STDOUT
` + "```" + `

❌ **WRONG** - Do NOT use the file path directly:
` + "```yaml" + `
index_codebase:
  step_type: codebase-index
  codebase_index:
    root: ./src
    output:
      path: .comanda/INDEX.md

analyze_codebase:
  input: .comanda/INDEX.md    # ❌ WRONG! Path may not resolve correctly
  model: claude-code
  action: "Analyze this codebase structure"
  output: STDOUT
` + "```" + `

**Why?** The ` + "`output.path`" + ` is relative to the repository root (when ` + "`store: repo`" + `), not the current working directory. The exported variable always contains the correct content regardless of where you run the workflow.

**Supported Languages:**
- **Go**: Uses AST parsing. Detection: ` + "`go.mod`" + `, ` + "`go.sum`" + `
- **Python**: Uses regex. Detection: ` + "`pyproject.toml`" + `, ` + "`requirements.txt`" + `, ` + "`setup.py`" + `
- **TypeScript/JavaScript**: Uses regex. Detection: ` + "`tsconfig.json`" + `, ` + "`package.json`" + `
- **Flutter/Dart**: Uses regex. Detection: ` + "`pubspec.yaml`" + `

**Example: Index and Analyze a Codebase**
` + "```yaml" + `
# Step 1: Generate codebase index
index_repo:
  step_type: codebase-index
  codebase_index:
    root: ./my-project
    expose:
      workflow_variable: true

# Step 2: Use the index for analysis
analyze_architecture:
  input: STDIN
  model: claude-code
  action: |
    Here is the codebase index:
    $MY_PROJECT_INDEX

    Analyze the architecture and suggest improvements.
  output: STDOUT
` + "```" + `

**Example: Minimal Usage**
` + "```yaml" + `
index_repo:
  step_type: codebase-index
  codebase_index:
    root: .
` + "```" + `

## Common Elements (for Standard Steps)

### Input Types
- File path: ` + "`input: path/to/file.txt`" + `
- Previous step output: ` + "`input: STDIN`" + `
- Multiple file paths: ` + "`input: [file1.txt, file2.txt]`" + `
- Web scraping: ` + "`input: { url: \"https://example.com\" }`" + ` (Further scrape config under ` + "`scrape_config`" + ` map if needed)
- Database query: ` + "`input: { database: { type: \"postgres\", query: \"SELECT * FROM users\" } }`" + `
- No input: ` + "`input: NA`" + `
- Input with alias for variable: ` + "`input: path/to/file.txt as $my_var`" + `
- List with aliases: ` + "`input: [file1.txt as $file1_content, file2.txt as $file2_content]`" + `

### Models

**⚠️ CRITICAL MODEL RESTRICTION ⚠️**

You **MUST ONLY** use models from the "Supported Models" list below. **DO NOT invent, guess, or use model names not explicitly listed.** Using unlisted model names will cause the workflow to fail at runtime.

**Syntax:**
- Single model: ` + "`model: gpt-4o-mini`" + `
- No model (for non-LLM operations): ` + "`model: NA`" + `
- Multiple models (for comparison): ` + "`model: [gpt-4o-mini, claude-3-opus-20240229]`" + `

### Supported Models (USE ONLY THESE)
{{SUPPORTED_MODELS}}

**🚫 DO NOT USE** model names that are not in the list above. Common mistakes:
- ❌ ` + "`gpt-4`" + ` (use ` + "`gpt-4o`" + ` or ` + "`gpt-4o-mini`" + ` instead)
- ❌ ` + "`claude-3`" + ` (use the full model name like ` + "`claude-sonnet-4-5`" + `)
- ❌ ` + "`gemini-pro`" + ` (use the exact name from the list above)
- ❌ Any model name you are "pretty sure" exists - ONLY use names from the list

### Claude Code Models (Local Agentic AI)

**IMPORTANT: Recognizing Claude Code requests:**
If the user's prompt mentions any of the following, they want to use a ` + "`claude-code`" + ` model:
- "claude code" (case insensitive)
- "Claude Code"
- "use claude code"
- "with claude code"
- "using claude code"
- "via claude code"
- "claude-code"

**What is Claude Code?**
Claude Code (` + "`claude-code`" + `, ` + "`claude-code-opus`" + `, ` + "`claude-code-sonnet`" + `, ` + "`claude-code-haiku`" + `) is a special model family that uses the local Claude Code CLI (` + "`claude`" + ` binary) instead of API calls. It provides:
- **Agentic capabilities**: Can autonomously perform multi-step tasks
- **Local execution**: Runs via the Claude CLI installed on the user's machine
- **Tool use**: Can interact with files, run commands, and perform complex operations

**When to use Claude Code models:**
- When the user explicitly mentions "claude code" in their request
- When agentic/autonomous capabilities are needed
- When the workflow should leverage Claude Code's tool-use abilities
- When local CLI execution is preferred over API calls

**Claude Code model variants:**
- ` + "`claude-code`" + `: Base variant (uses default Claude Code model)
- ` + "`claude-code-opus`" + `: Uses Claude Opus 4.5 (most capable)
- ` + "`claude-code-sonnet`" + `: Uses Claude Sonnet 4.5 (balanced)
- ` + "`claude-code-haiku`" + `: Uses Claude Haiku 4.5 (fastest/cheapest)

**Example using Claude Code:**
` + "```yaml" + `
generate_haiku:
  input: NA
  model: claude-code
  action: "Generate a beautiful haiku about nature"
  output: STDOUT
` + "```" + `

### OpenAI Codex Models (Local Agentic AI)

**IMPORTANT: Recognizing OpenAI Codex requests:**
If the user's prompt mentions any of the following, they want to use an ` + "`openai-codex`" + ` model:
- "openai codex" (case insensitive)
- "OpenAI Codex"
- "use codex"
- "with codex"
- "using codex"
- "via codex"
- "openai-codex"
- just "codex" (when referring to the CLI tool)

**What is OpenAI Codex?**
OpenAI Codex (` + "`openai-codex`" + `, ` + "`openai-codex-o3`" + `, ` + "`openai-codex-o4-mini`" + `, ` + "`openai-codex-gpt-4.1`" + `, ` + "`openai-codex-gpt-4o`" + `) is a special model family that uses the local OpenAI Codex CLI (` + "`codex`" + ` binary) instead of API calls. It provides:
- **Agentic capabilities**: Can autonomously perform multi-step tasks
- **Local execution**: Runs via the Codex CLI installed on the user's machine
- **Tool use**: Can interact with files, run commands, and perform complex operations

**When to use OpenAI Codex models:**
- When the user explicitly mentions "codex" or "openai codex" in their request
- When agentic/autonomous capabilities are needed with OpenAI models
- When the workflow should leverage Codex's tool-use abilities
- When local CLI execution is preferred over API calls

**OpenAI Codex model variants:**
- ` + "`openai-codex`" + `: Base variant (uses default Codex model)
- ` + "`openai-codex-o3`" + `: Uses o3 model (most capable reasoning)
- ` + "`openai-codex-o4-mini`" + ` or ` + "`openai-codex-mini`" + `: Uses o4-mini (fast/affordable)
- ` + "`openai-codex-gpt-4.1`" + `: Uses GPT-4.1
- ` + "`openai-codex-gpt-4o`" + `: Uses GPT-4o

**Example using OpenAI Codex:**
` + "```yaml" + `
generate_haiku:
  input: NA
  model: openai-codex
  action: "Generate a beautiful haiku about nature"
  output: STDOUT
` + "```" + `

### Model Selection Guidelines

**CRITICAL: Choose models appropriate for task complexity:**

**Use inexpensive/fast models (nano, mini, lite, flash, haiku) for:**
- Simple text transformations and formatting
- Data extraction and parsing
- Straightforward summarization
- Repetitive processing tasks
- High-volume batch operations

**Use flagship models (opus, pro, o1, o3, gpt-5) for:**
- Complex reasoning and analysis
- Creative writing and nuanced content
- Multi-step problem solving
- Tasks requiring deep understanding
- Small token window tasks where quality matters most

**Model tiers (from cheapest to most expensive):**
- **Nano/Lite tier**: ` + "`gpt-5.1-nano`" + `, ` + "`gpt-5-nano`" + `, ` + "`gemini-2.5-flash-lite`" + `
- **Mini/Flash tier**: ` + "`gpt-5.1-mini`" + `, ` + "`gpt-5-mini`" + `, ` + "`o4-mini`" + `, ` + "`o3-mini`" + `, ` + "`gemini-2.5-flash`" + `, ` + "`claude-haiku-4-5`" + `
- **Standard tier**: ` + "`gpt-4o`" + `, ` + "`gpt-4.1`" + `, ` + "`gemini-2.5-pro`" + `, ` + "`claude-sonnet-4-5`" + `
- **Flagship tier**: ` + "`gpt-5`" + `, ` + "`gpt-5.1`" + `, ` + "`o1`" + `, ` + "`o3`" + `, ` + "`o1-pro`" + `, ` + "`o3-pro`" + `, ` + "`claude-opus-4-5`" + `, ` + "`gemini-3-pro-preview`" + `

### Actions
- Single instruction: ` + "`action: \"Summarize this text.\"`" + `
- Multiple sequential instructions: ` + "`action: [\"Action 1\", \"Action 2\"]`" + `
- Reference variable: ` + "`action: \"Compare with $previous_data.\"`" + `
- Reference markdown file: ` + "`action: path/to/prompt.md`" + `

### Outputs
- Console: ` + "`output: STDOUT`" + `
- File: ` + "`output: results.txt`" + ` or ` + "`output: ./path/to/output.md`" + `
- Database: ` + "`output: { database: { type: \"postgres\", table: \"results_table\" } }`" + `
- Output with alias (if supported for variable creation from output): ` + "`output: STDOUT as $step_output_var`" + `

**⚠️ IMPORTANT: Writing to files**
When the result should be saved to a file, use the ` + "`output:`" + ` field directly. Do NOT instruct the LLM to "write to a file" in the action.

✅ **CORRECT** - Use ` + "`output:`" + ` for file writing:
` + "```yaml" + `
summarize_document:
  input: report.txt
  model: claude-code
  action: "Summarize this document"
  output: ./summary.md
` + "```" + `

❌ **WRONG** - Do NOT tell the LLM to write the file:
` + "```yaml" + `
summarize_document:
  input: report.txt
  model: claude-code
  action: "Summarize this document and write it to ./summary.md"
  output: STDOUT
` + "```" + `

### Tool Use (Shell Command Execution)

Comanda supports executing shell commands as part of workflows using the ` + "`tool:`" + ` prefix.

**Tool Input Formats:**
- Simple command: ` + "`input: \"tool: ls -la\"`" + `
- Pipe previous output to command: ` + "`input: \"tool: STDIN|grep pattern\"`" + `

**Tool Output Formats:**
- Pipe LLM output through command: ` + "`output: \"tool: jq '.data'\"`" + `
- Pipe STDOUT through command: ` + "`output: \"STDOUT|grep pattern\"`" + `

**Security Controls:**
Tools execute with security controls - a default allowlist of safe read-only commands and a denylist of dangerous commands.

**Safe commands (allowlist):** ` + "`ls`" + `, ` + "`cat`" + `, ` + "`head`" + `, ` + "`tail`" + `, ` + "`grep`" + `, ` + "`awk`" + `, ` + "`sed`" + `, ` + "`jq`" + `, ` + "`yq`" + `, ` + "`sort`" + `, ` + "`uniq`" + `, ` + "`wc`" + `, ` + "`cut`" + `, ` + "`tr`" + `, ` + "`diff`" + `, ` + "`find`" + `, ` + "`date`" + `, ` + "`echo`" + `, ` + "`base64`" + `, etc.

**Blocked commands (denylist):** ` + "`rm`" + `, ` + "`sudo`" + `, ` + "`chmod`" + `, ` + "`curl`" + `, ` + "`wget`" + `, ` + "`ssh`" + `, ` + "`bash`" + `, ` + "`sh`" + `, etc.

**Step-level tool configuration:**
` + "```yaml" + `
step_name:
  input: "tool: ls -la"
  model: NA
  tool_config:
    allowlist: [ls, cat, grep, jq]  # Override default allowlist
    denylist: [rm]                  # Additional commands to block
    timeout: 60                      # Timeout in seconds (default: 30)
  action: NA
  output: STDOUT
` + "```" + `

## Variables and Data Flow Between Steps

Understanding when to use variables vs files is critical for building effective workflows.

### Workflow Variables (` + "`$VARNAME`" + `)
- Definition: ` + "`input: data.txt as $initial_data`" + `
- Reference: ` + "`action: \"Compare this analysis with $initial_data\"`" + `
- Scope: Variables are scoped to the workflow. For ` + "`process`" + ` steps, use the ` + "`process.inputs`" + ` map to pass data.

### When to Use Variables vs Files

**Use ` + "`$VARIABLES`" + ` for:**
- **Ephemeral values** that are only needed within the workflow
- **Small text** (under ~10KB) being passed between steps
- **Loop state** in agentic loops (` + "`$PLAN`" + `, ` + "`$ANALYSIS`" + `, ` + "`$RESULT`" + `)
- **Intermediate processing** that doesn't need to persist

**Use **files** (` + "`.md`" + `, ` + "`.txt`" + `, ` + "`.json`" + `) for:**
- **Persistent outputs** that users will want to access later
- **Large content** (over ~10KB) - variables can bloat memory
- **Multi-step independent access** - when step 3 needs output from both step 1 AND step 2
- **Final deliverables** (reports, summaries, generated code)
- **Audit trail** - when you need to see intermediate results

### Chaining Patterns

**Pattern 1: Sequential STDIN/STDOUT (simplest)**
Use when each step only needs the previous step's output:
` + "```yaml" + `
extract_key_points:
  input: document.txt
  model: gpt-4o-mini
  action: "Extract key points"
  output: STDOUT

translate_to_spanish:
  input: STDIN  # Gets output from extract_key_points
  model: gpt-4o-mini
  action: "Translate to Spanish"
  output: translated_points.md  # Final output saved to file
` + "```" + `

**Pattern 2: Variables for Agentic Loops**
Use ` + "`$VARIABLES`" + ` within agentic loops to pass ephemeral state:
` + "```yaml" + `
agentic-loop:
  config:
    max_iterations: 5
    exit_condition: llm_decides
  steps:
    plan_next_action:
      input: STDIN
      model: claude-code
      action: "Plan the next step"
      output: $PLAN  # Ephemeral - only needed for next step

    execute_plan:
      input: $PLAN  # Uses the variable
      model: claude-code
      action: "Execute the plan"
      output: STDOUT
` + "```" + `

**Pattern 3: Files for Multi-Access**
Use files when multiple steps need independent access:
` + "```yaml" + `
analyze_code_structure:
  input: source_code.py
  model: gpt-4o-mini
  action: "Analyze code structure"
  output: structure_analysis.md  # Save to file

identify_security_issues:
  input: source_code.py
  model: gpt-4o-mini
  action: "Identify security vulnerabilities"
  output: security_report.md  # Save to file

create_combined_report:
  input: [structure_analysis.md, security_report.md]  # Access both files
  model: gpt-4o-mini
  action: "Create comprehensive report from both analysis outputs"
  output: final_report.md
` + "```" + `

### Variable Naming Conventions

Use **descriptive names** that indicate the content:
- ✅ ` + "`$EXTRACTED_EMAILS`" + `, ` + "`$CODE_ANALYSIS`" + `, ` + "`$TRANSLATION_RESULT`" + `
- ❌ ` + "`$OUTPUT`" + `, ` + "`$RESULT`" + `, ` + "`$DATA`" + `, ` + "`$TEMP`" + `

## CLI Variables (Runtime Substitution)

CLI variables allow runtime value injection using ` + "`--vars key=value`" + ` flags when running workflows:

**Usage:**
` + "```bash" + `
# Single variable
comanda process workflow.yaml --vars filename=/path/to/file.txt

# Multiple variables
comanda process workflow.yaml --vars key1=value1 --vars key2=value2

# Map STDIN to a variable
cat data.txt | comanda process workflow.yaml --vars data=STDIN
` + "```" + `

**In workflows, reference CLI variables with ` + "`{{varname}}`" + ` syntax:**
` + "```yaml" + `
step_name:
  input: "tool: grep -E 'error' {{filename}}"
  model: gpt-4o-mini
  action: "Analyze {{project_name}} logs"
  output: "{{output_dir}}/results.txt"
` + "```" + `

**Key differences from workflow variables (` + "`$varname`" + `):**
- ` + "`{{varname}}`" + `: CLI-provided at runtime via ` + "`--vars`" + ` flag, substituted before processing
- ` + "`$varname`" + `: Defined in workflow with ` + "`as $varname`" + ` syntax, scoped to workflow execution

**When to use CLI variables:**
- When the same workflow should work with different input files or parameters
- When values need to be provided dynamically at runtime
- When building reusable workflow templates

## Validation Rules Summary (for LLM)

1.  When specifying a model name, you **must** use one of the supported models listed in the "Supported Models" section. Do not use model names that are not explicitly listed as supported.
2.  A step definition must clearly be one of: Standard, Generate, or Process.
    *   A step cannot mix top-level keys from different types (e.g., a ` + "`generate`" + ` step should not have a top-level ` + "`model`" + ` or ` + "`output`" + ` key; these belong inside the ` + "`generate`" + ` block).
2.  **Standard Step:**
    *   Must contain ` + "`input`" + `, ` + "`model`" + `, ` + "`action`" + `, ` + "`output`" + ` (unless ` + "`type: openai-responses`" + `, where ` + "`action`" + ` might be replaced by ` + "`instructions`" + `).
    *   ` + "`input`" + ` can be ` + "`NA`" + `. ` + "`model`" + ` can be ` + "`NA`" + `.
3.  **Generate Step:**
    *   Must contain a ` + "`generate`" + ` block.
    *   ` + "`generate`" + ` block must contain ` + "`action`" + ` (string prompt) and ` + "`output`" + ` (string filename).
    *   ` + "`generate.model`" + ` is optional (uses default if omitted).
    *   Top-level ` + "`input`" + ` for the step is optional (can be ` + "`NA`" + ` or provide context).
4.  **Process Step:**
    *   Must contain a ` + "`process`" + ` block.
    *   ` + "`process`" + ` block must contain ` + "`workflow_file`" + ` (string path).
    *   ` + "`process.inputs`" + ` is optional.
    *   Top-level ` + "`input`" + ` for the step is optional (can be ` + "`NA`" + ` or ` + "`STDIN`" + ` to pipe to sub-workflow).
5.  **Agentic Loop Step (Inline):**
    *   Must contain an ` + "`agentic_loop`" + ` block with loop configuration.
    *   Must also contain ` + "`input`" + `, ` + "`model`" + `, ` + "`action`" + `, ` + "`output`" + ` at the step level.
    *   ` + "`agentic_loop.max_iterations`" + ` defaults to 10 if not specified.
    *   ` + "`agentic_loop.exit_condition`" + ` can be ` + "`llm_decides`" + ` or ` + "`pattern_match`" + `.
6.  **Agentic Loop Block (Top-level):**
    *   Uses ` + "`agentic-loop:`" + ` as a top-level key (like ` + "`parallel-process:`" + `).
    *   Must contain ` + "`config`" + ` block with loop settings.
    *   Must contain ` + "`steps`" + ` block with one or more sub-steps.
    *   Each sub-step follows standard step structure (` + "`input`" + `, ` + "`model`" + `, ` + "`action`" + `, ` + "`output`" + `).
7.  **Codebase Index Step:**
    *   Must have ` + "`step_type: codebase-index`" + ` OR contain a ` + "`codebase_index`" + ` block.
    *   ` + "`codebase_index.root`" + ` defaults to ` + "`.`" + ` (current directory).
    *   Exports workflow variables: ` + "`<REPO>_INDEX`" + `, ` + "`<REPO>_INDEX_PATH`" + `, ` + "`<REPO>_INDEX_SHA`" + `, ` + "`<REPO>_INDEX_UPDATED`" + `.
    *   Does not require ` + "`input`" + `, ` + "`model`" + `, ` + "`action`" + `, or ` + "`output`" + ` fields.

## Chaining and Examples

Steps can be "chained together" by either passing STDOUT from one step to STDIN of the next step or by writing to a file and then having subsequent steps take this file as input.

**Meta-Processing Example:**
` + "```yaml" + `
gather_requirements:
  input: requirements_document.txt
  model: claude-3-opus-20240229
  action: "Based on the input document, define the core tasks for a data processing workflow. Output as a concise list."
  output: STDOUT

generate_data_workflow:
  input: STDIN # Using output from previous step as context
  generate:
    model: gpt-4o-mini # LLM to generate the workflow
    action: "Generate a Comanda workflow YAML to perform the tasks described in the input. The workflow should read 'raw_data.csv', perform transformations, and save to 'processed_data.csv'."
    output: dynamic_data_processor.yaml # Filename for the generated workflow

execute_data_workflow:
  input: NA # Or potentially STDIN if dynamic_data_processor.yaml's first step expects it
  process:
    workflow_file: dynamic_data_processor.yaml # Execute the generated workflow
    # inputs: { source_file: "override_data.csv" } # Optional: override inputs for the sub-workflow
  output: STDOUT # Log output of the process step itself (e.g., success/failure)
` + "```" + `

### Advanced Chaining: Enabling Independent Analysis with Files

The standard ` + "`STDIN`" + `/` + "`STDOUT`" + ` chain is designed for sequential processing, where each step receives the output of the one immediately before it. However, many workflows require a downstream step to **independently analyze outputs from multiple, potentially non-sequential, upstream steps.**

To enable this, you must use files to store intermediate results. This pattern ensures that each output is preserved and can be accessed directly by any subsequent step, rather than being lost in a pipeline.

**The recommended pattern is:**
1.  Each upstream step saves its result to a distinct file (e.g., ` + "`step1_output.txt`" + `, ` + "`step2_output.txt`" + `).
2.  The downstream step that needs to perform the independent analysis lists these files as its ` + "`input`" + `.

**Example: A 3-Step Workflow with a Final Review**

In this scenario, the third step needs to review the outputs of both the first and second steps independently.

` + "```yaml" + `
# Step 1: Initial analysis
analyze_introductions:
  input: introductions.md
  model: gpt-4o-mini
  action: "Perform a detailed analysis of the introductions document. Focus on key themes, writing style, and effectiveness."
  output: step1_analysis.txt

# Step 2: Quality assessment of the original document
quality_assessment:
  input: introductions.md
  model: gpt-4o-mini
  action: "Perform a quality assessment on the original document. Identify strengths and potential gaps."
  output: step2_qa.txt

# Step 3: Final summary based on both outputs
final_summary:
  input: [step1_analysis.txt, step2_qa.txt]
  model: gpt-4o-mini
  action: "Review the results from the analysis (step1_analysis.txt) and the QA (step2_qa.txt). Provide a comprehensive summary that synthesizes the findings from both."
  output: final_summary.md
` + "```" + `

This file-based approach is the correct way to handle any workflow where a step's logic depends on having discrete access to multiple prior outputs.

## CRITICAL: Workflow Simplicity Guidelines

**ALWAYS prefer the simplest possible workflow.** Over-engineered workflows are harder to debug, maintain, and understand.

**Key principles:**
1. **Minimize steps**: If a task can be done in 1 step, don't use 3. Most tasks need 1-2 steps.
2. **Avoid unnecessary chaining**: Don't chain steps unless the output of one is genuinely needed by the next.
3. **Use direct file I/O**: If you need to read a file and process it, that's ONE step, not three.
4. **Prefer STDIN/STDOUT**: Use simple STDIN/STDOUT chaining over complex file intermediates when sequential processing suffices.
5. **One model per workflow when possible**: Don't use multiple models unless comparing outputs or the task genuinely requires different capabilities.

**Examples of OVER-ENGINEERED workflows (AVOID):**
` + "```yaml" + `
# BAD: Too many steps for a simple task
read_file:
  input: document.txt
  model: NA
  action: NA
  output: temp_content.txt

analyze_content:
  input: temp_content.txt
  model: gpt-4o-mini
  action: "Analyze this"
  output: temp_analysis.txt

format_output:
  input: temp_analysis.txt
  model: gpt-4o-mini
  action: "Format nicely"
  output: STDOUT
` + "```" + `

**GOOD: Simple and direct:**
` + "```yaml" + `
# GOOD: One step does the job
analyze_document:
  input: document.txt
  model: gpt-4o-mini
  action: "Analyze this document and format the output nicely"
  output: STDOUT
` + "```" + `

**When multiple steps ARE appropriate:**
- Processing different source files independently, then combining results
- Using tool commands to pre-process data before LLM analysis
- Generating a workflow dynamically, then executing it
- Tasks that genuinely require different models for different capabilities
- **Agentic loops** for iterative refinement, planning, or autonomous decision-making

**When to use Agentic Loops:**
- Code improvement cycles (analyze → fix → verify)
- Planning and execution workflows
- Tasks where quality depends on iteration
- When the LLM should decide when work is complete

This guide covers the core concepts and syntax of Comanda's YAML DSL, including meta-processing capabilities. LLMs should use this structure to generate valid workflow files.`
