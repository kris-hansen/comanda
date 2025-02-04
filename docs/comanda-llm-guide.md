# Comanda YAML DSL Guide

This guide explains how to create workflow files for comanda, a tool that orchestrates multi-step workflows involving LLMs, file processing, and data operations.

## Overview

Comanda uses a YAML-based Domain Specific Language (DSL) to define workflows. Each workflow consists of one or more steps that process inputs through models and actions to produce outputs.

## Basic Structure

A comanda workflow file follows this basic structure:

```yaml
step_name:
  input: [input source]
  model: [model name]
  action: [action to perform / prompt provided]
  output: [output destination]
```

Each step must include these four key elements:
- `input`: Source of data (file paths, STDIN, or special inputs)
- `model`: LLM model to use (or "NA" if no model needed)
- `action`: Instructions or operations to perform
- `output`: Where to send the results (file paths or STDOUT)

## Input Types

Inputs can be specified in several ways:

1. File paths:
```yaml
input: examples/myfile.txt
```

2. Previous step output:
```yaml
input: STDIN
```

3. Multiple inputs:
```yaml
input: 
  - file1.txt
  - file2.txt
```

4. Web scraping:
```yaml
input:
  url: "https://example.com"
```

5. Database queries:
```yaml
input:
  database:
    type: postgres
    query: "SELECT * FROM users"
```

## Models

The `model` field specifies which LLM to use:

```yaml
model: gpt-4o-mini  # OpenAI model
```

Special values:
- `NA`: No model needed (for non-LLM operations)
- Multiple models can be specified for comparison:
```yaml
model:
  - gpt-4o-mini
  - claude-instant
```

## Actions

Actions define what to do with the input:

1. Simple instructions:
```yaml
action: "Analyze this text and provide a summary"
```

2. Multiple actions:
```yaml
action:
  - "First analyze the content"
  - "Then extract key points"
```

3. Variable references:
```yaml
action: "Compare this with $previous_analysis"
```

## Outputs

Outputs define where to send results:

1. Console output:
```yaml
output: STDOUT
```

2. File output:
```yaml
output: results.txt
```

3. Database output:
```yaml
output:
  database:
    type: postgres
    table: results
```

## Multi-step Example

Here's a complete example that processes a CSV file through multiple steps:

```yaml
step_one:
  input: data.csv
  model: gpt-4o-mini
  action: "Analyze this CSV data and provide a summary of its contents."
  output: STDOUT

step_two:
  input: STDIN
  model: gpt-4o-mini
  action: "Based on the analysis, identify the top 5 insights"
  output: insights.txt
```

## Variables

You can store and reference values between steps:

```yaml
step_one:
  input: data.txt as $initial_data
  model: gpt-4o-mini
  action: "Analyze this text"
  output: STDOUT

step_two:
  input: STDIN
  model: gpt-4o-mini
  action: "Compare this analysis with $initial_data"
  output: STDOUT
```

## Validation Rules

1. Each step must have all four main elements: input, model, action, and output
2. Input tags must be present (can be empty or NA)
3. At least one model must be specified (can be NA)
4. At least one action is required
5. At least one output destination is required

## Best Practices

1. Use meaningful step names that describe their purpose
2. Break complex workflows into clear, focused steps
3. Use STDIN to chain step outputs together
4. Store intermediate results in variables when needed for later comparison
5. Use STDOUT for debugging and final results
6. Leverage specialized input types (database, URL) for data acquisition

## Chaining Workflow Steps

Comanda provides several patterns for chaining steps together. Here are the main approaches with examples:

### 1. Using STDIN/STDOUT Chain

The most direct way to chain steps is using STDOUT and STDIN:

```yaml
extract_data:
  input: source.txt
  model: gpt-4o-mini
  action: "Extract key information from this document"
  output: STDOUT

analyze_data:
  input: STDIN  # Uses output from previous step
  model: gpt-4o-mini
  action: "Analyze the extracted information and provide insights"
  output: STDOUT

summarize_insights:
  input: STDIN  # Uses output from previous step
  model: gpt-4o-mini
  action: "Create a concise summary of these insights"
  output: final_summary.txt
```

### 2. Using Intermediate Files

When you need to reference intermediate results later or process them separately:

```yaml
generate_analysis:
  input: raw_data.txt
  model: gpt-4o-mini
  action: "Perform detailed analysis of this data"
  output: analysis_results.txt  # Saved to file for later use

create_recommendations:
  input: analysis_results.txt  # Uses file from previous step
  model: gpt-4o-mini
  action: "Generate recommendations based on the analysis"
  output: recommendations.txt

summarize_all:
  input:  # Using multiple input files
    - analysis_results.txt
    - recommendations.txt
  model: gpt-4o-mini
  action: "Create an executive summary combining the analysis and recommendations"
  output: executive_summary.txt
```

### 3. Hybrid Approach (Files + STDIN)

Combining file-based and STDIN chaining for complex workflows:

```yaml
initial_analysis:
  input: data.txt
  model: gpt-4o-mini
  action: "Analyze this data and categorize findings"
  output: categories.txt  # Save categories for later reference

process_categories:
  input: categories.txt
  model: gpt-4o-mini
  action: "Process these categories and suggest improvements"
  output: STDOUT  # Pass directly to next step

refine_results:
  input: STDIN
  model: gpt-4o-mini
  action: "Refine these suggestions and create action items"
  output: action_items.txt
```

### 4. Parallel Processing with File Outputs

When you need to process data in parallel and combine results:

```yaml
analyze_section_1:
  input: data_part1.txt
  model: gpt-4o-mini
  action: "Analyze this section of data"
  output: section1_analysis.txt

analyze_section_2:
  input: data_part2.txt
  model: gpt-4o-mini
  action: "Analyze this section of data"
  output: section2_analysis.txt

combine_analyses:
  input:
    - section1_analysis.txt
    - section2_analysis.txt
  model: gpt-4o-mini
  action: "Compare and combine these analyses into a unified report"
  output: final_report.txt
```

## Common Patterns

1. Data Analysis:
```yaml
analyze:
  input: data.csv
  model: gpt-4o-mini
  action: "Analyze this data and provide insights"
  output: STDOUT
```

2. Content Generation:
```yaml
generate:
  input: requirements.txt
  model: gpt-4o-mini
  action: "Generate content based on these requirements"
  output: content.md
```

3. Multi-model Comparison:
```yaml
compare:
  input: prompt.txt
  model:
    - gpt-4o-mini
    - claude-instant
  action: "Solve this problem"
  output: comparison.txt
```

This guide covers the core concepts and syntax of comanda's YAML DSL. LLMs can use this structure to generate valid workflow files that orchestrate complex operations involving model interactions, file processing, and data handling.
