# Generate and Process Examples

This directory contains examples demonstrating Comanda's meta-processing capabilities using the `generate` and `process` tags.

## Overview

Comanda supports dynamic workflow generation where:
- The `generate` tag uses an LLM to create a new Comanda workflow YAML file
- The `process` tag executes another Comanda workflow file (either static or dynamically generated)

## Examples

### 1. Basic Example: `generate-process-example.yaml`

This simple example demonstrates:
- **Step 1**: Uses the `generate` tag to create a workflow that generates a haiku
- **Step 2**: Uses the `process` tag to execute the generated workflow

To run:
```bash
comanda process examples/generate-process-example.yaml
```

Expected behavior:
1. First, it generates a file called `generated_haiku_workflow.yaml` containing a simple workflow
2. Then it executes that workflow, which outputs a haiku about coding to the console

### 2. Advanced Example: `generate-process-advanced-example.yaml`

This more complex example demonstrates:
- **Step 1**: Selects a random theme for haikus
- **Step 2**: Uses the `generate` tag to create a workflow that:
  - Creates three haikus based on the theme
  - Combines them into a collection with a title
- **Step 3**: Uses the `process` tag to execute the generated workflow, passing the theme as a parameter

To run:
```bash
comanda process examples/generate-process-advanced-example.yaml
```

Expected behavior:
1. Randomly selects a theme (nature, technology, seasons, or emotions)
2. Generates a file called `generated_haiku_collection.yaml` with a multi-step workflow
3. Executes that workflow, which creates three haikus and saves them to `haiku_collection.txt`

Note: The generated `haiku_collection.txt` file can be viewed after the workflow completes.

## Key Concepts

### Generate Tag Structure
```yaml
step_name:
  input: [optional context]
  generate:
    model: [LLM to use for generation]
    action: [prompt describing the workflow to generate]
    output: [filename for the generated YAML]
```

### Process Tag Structure
```yaml
step_name:
  input: [optional input for the sub-workflow]
  process:
    workflow_file: [path to YAML file to execute]
    inputs: [optional key-value pairs to pass to sub-workflow]
```

### Variable Passing
- Parent workflow variables can be passed to child workflows using the `inputs` field
- In the child workflow, these are accessed as `$parent.variable_name`

## Use Cases

This meta-processing capability is useful for:
- Creating adaptive workflows based on input data
- Generating specialized processing pipelines on demand
- Building workflow templates that can be customized dynamically
- Creating self-modifying or self-improving workflows

## Clean Up

After running these examples, you may want to clean up the generated files:
```bash
rm generated_haiku_workflow.yaml
rm generated_haiku_collection.yaml
rm haiku_collection.txt
