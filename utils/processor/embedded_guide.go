package processor

import (
	"fmt"
	"strings"

	"github.com/kris-hansen/comanda/utils/models"
)

// GetEmbeddedLLMGuide returns the Comanda YAML DSL Guide for LLM consumption
// with the current supported models injected
func GetEmbeddedLLMGuide() string {
	// Get all models from the registry
	registry := models.GetRegistry()
	allModels := registry.GetAllModelsList()

	// Format the models as a comma-separated list with code formatting
	modelsList := formatModelsList(allModels)

	// Replace the models section in the guide
	guide := strings.Replace(
		embeddedLLMGuideTemplate,
		"{{SUPPORTED_MODELS}}",
		modelsList,
		1,
	)

	return guide
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

Comanda workflows consist of one or more named steps. Each step performs an operation. There are three main types of steps:
1.  **Standard Processing Step:** Involves LLMs, file processing, data operations.
2.  **Generate Step:** Uses an LLM to dynamically create a new Comanda workflow YAML file.
3.  **Process Step:** Executes another Comanda workflow file (static or dynamically generated).

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
- File: ` + "`output: results.txt`" + `
- Database: ` + "`output: { database: { type: \"postgres\", table: \"results_table\" } }`" + `
- Output with alias (if supported for variable creation from output): ` + "`output: STDOUT as $step_output_var`" + `

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

This guide covers the core concepts and syntax of Comanda's YAML DSL, including meta-processing capabilities. LLMs should use this structure to generate valid workflow files.`

// embeddedLLMGuideTemplate is the template for the Comanda YAML DSL Guide
// It includes a placeholder for the supported models
const embeddedLLMGuideTemplate = `# Comanda YAML DSL Guide (for LLM Consumption)

This guide specifies the YAML-based Domain Specific Language (DSL) for Comanda workflows, enabling LLMs to generate valid workflow files.

## Overview

Comanda workflows consist of one or more named steps. Each step performs an operation. There are three main types of steps:
1.  **Standard Processing Step:** Involves LLMs, file processing, data operations.
2.  **Generate Step:** Uses an LLM to dynamically create a new Comanda workflow YAML file.
3.  **Process Step:** Executes another Comanda workflow file (static or dynamically generated).

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
- Single model: ` + "`model: gpt-4o-mini`" + `
- No model (for non-LLM operations): ` + "`model: NA`" + `
- Multiple models (for comparison): ` + "`model: [gpt-4o-mini, claude-3-opus-20240229]`" + `

### Supported Models
{{SUPPORTED_MODELS}}

### Actions
- Single instruction: ` + "`action: \"Summarize this text.\"`" + `
- Multiple sequential instructions: ` + "`action: [\"Action 1\", \"Action 2\"]`" + `
- Reference variable: ` + "`action: \"Compare with $previous_data.\"`" + `
- Reference markdown file: ` + "`action: path/to/prompt.md`" + `

### Outputs
- Console: ` + "`output: STDOUT`" + `
- File: ` + "`output: results.txt`" + `
- Database: ` + "`output: { database: { type: \"postgres\", table: \"results_table\" } }`" + `
- Output with alias (if supported for variable creation from output): ` + "`output: STDOUT as $step_output_var`" + `

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

This guide covers the core concepts and syntax of Comanda's YAML DSL, including meta-processing capabilities. LLMs should use this structure to generate valid workflow files.`
