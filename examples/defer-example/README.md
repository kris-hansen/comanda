# Defer Example

This example demonstrates the use of the `defer:` tag in the YAML DSL. The `defer:` tag allows you to define steps that are excluded from execution unless they are explicitly called from the output of another step. This provides for logic based branching and execution using LLMs to drive the logic and execute a new workflow step when the required conditions are met.

## How it Works

1. Steps defined under the `defer:` section in the YAML file are not executed by default.
2. A regular step can "call" a deferred step by returning a JSON response with the format:
   ```json
   {
     "step": "step_name_to_execute",
     "input": "string_optional"
   }
   ```
3. When such a response is detected, the system looks up the deferred step and executes it with the provided input.
4. After the deferred step is completed, processing returns to the next step in the execution flow.

## Example

The `defer.yaml` file in this directory demonstrates a poem analysis workflow:

1. The main step (`determine_poem_type`) analyzes a poem and determines if it's a haiku or a sonnet.
2. Based on the analysis, it returns a JSON response with the appropriate deferred step to execute (`analyze_haiku` or `analyze_sonnet`).
3. The system then executes the deferred step, which provides a detailed analysis of the poem based on its type.

## Running the Example

To run the example with a haiku:

```bash
echo "An old silent pond... A frog jumps into the pond, splash! Silence again." | comanda process examples/defer-example/defer.yaml
```

To run the example with a sonnet:

```bash
echo "Shall I compare thee to a summer's day? Thou art more lovely and more temperate: Rough winds do shake the darling buds of May, And summer's lease hath all too short a date;" | comanda process examples/defer-example/defer.yaml
```

## Use Cases

The `defer:` tag enables branching and conditional execution in the YAML DSL, making it more flexible and powerful for complex workflows. Some use cases include:

1. **Conditional Processing**: Execute different steps based on the content or classification of the input.
2. **Error Handling**: Define special steps to handle errors or edge cases.
3. **Looping**: Create recursive workflows by having a step call itself with modified input.
4. **Multi-path Workflows**: Design workflows with multiple possible execution paths based on intermediate results.
5. **Dynamic Step Selection**: Allow the model to decide which processing step is most appropriate for a given input.

This feature significantly enhances the flexibility and power of the YAML DSL, enabling more complex and dynamic workflows.
