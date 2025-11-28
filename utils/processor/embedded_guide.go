package processor

import (
	"fmt"
	"strings"

	"github.com/kris-hansen/comanda/utils/models"
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
- **IMPORTANT**: When specifying a model, you **must** use one of the supported models listed below. Do not use model names that are not in this list.

### Supported Models
{{SUPPORTED_MODELS}}

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
- File: ` + "`output: results.txt`" + `
- Database: ` + "`output: { database: { type: \"postgres\", table: \"results_table\" } }`" + `
- Output with alias (if supported for variable creation from output): ` + "`output: STDOUT as $step_output_var`" + `

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

## Variables
- Definition: ` + "`input: data.txt as $initial_data`" + `
- Reference: ` + "`action: \"Compare this analysis with $initial_data\"`" + `
- Scope: Variables are typically scoped to the workflow. For ` + "`process`" + ` steps, parent variables are not directly accessible by default; use the ` + "`process.inputs`" + ` map to pass data.

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

This guide covers the core concepts and syntax of Comanda's YAML DSL, including meta-processing capabilities. LLMs should use this structure to generate valid workflow files.`
