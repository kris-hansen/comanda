# OpenAI Responses API Examples

These examples demonstrate how to use the OpenAI Responses API with comanda. The Responses API is OpenAI's most advanced interface for generating model responses, supporting text and image inputs, stateful interactions, and built-in tools.

## Features

- **Rich Input Support**: Accept text and image inputs
- **Stateful Interactions**: Maintain conversation state using previous_response_id
- **Built-in Tools**: Access file search, web search, computer use capabilities
- **Function Calling**: Allow models to call custom code and access external systems
- **Structured Output**: Return JSON or text responses

## Examples

### Basic Example

[basic-example.yaml](basic-example.yaml) demonstrates a simple use of the Responses API:

```yaml
analyze_text:
  type: openai-responses
  input:
    - examples/example_filename.txt
  model: gpt-4o
  instructions: "You are a helpful assistant specialized in business analysis."
  action:
    - analyze these company names and identify which are in the HVAC industry
  output:
    - STDOUT
```

### Web Search Example

[web-search-example.yaml](web-search-example.yaml) shows how to use the web search tool:

```yaml
research_topic:
  type: openai-responses
  input:
    - examples/research_topic.txt
  model: gpt-4o
  instructions: "You are a research assistant. Provide comprehensive information on the given topic."
  tools:
    - type: web_search
  action:
    - research this topic and provide a detailed summary with the latest information
  output:
    - research_results.md
```

### Multi-turn Conversation Example

[conversation-example.yaml](conversation-example.yaml) demonstrates stateful interactions:

```yaml
initial_query:
  type: openai-responses
  input:
    - examples/user_question.txt
  model: gpt-4o
  instructions: "You are a helpful assistant."
  action:
    - answer this question
  output:
    - initial_response.json

followup_query:
  type: openai-responses
  input:
    - examples/followup_question.txt
  model: gpt-4o
  previous_response_id: "$initial_query.response_id"
  action:
    - answer this followup question
  output:
    - final_response.txt
```

### Function Calling Example

[function-calling-example.yaml](function-calling-example.yaml) shows how to use function calling:

```yaml
weather_query:
  type: openai-responses
  input:
    - examples/weather_query.txt
  model: gpt-4o
  tools:
    - type: function
      function:
        name: get_weather
        description: "Get the current weather for a location"
        parameters:
          type: object
          properties:
            location:
              type: string
              description: "The city and state, e.g. San Francisco, CA"
            unit:
              type: string
              enum: ["celsius", "fahrenheit"]
              description: "The unit of temperature"
  action:
    - answer the user's question about the weather
  output:
    - STDOUT
```

## Usage

To run these examples, use the `comanda process` command:

```bash
comanda process examples/responses-api/basic-example.yaml
```

## Configuration

The OpenAI Responses API requires an OpenAI API key with access to the Responses API. Configure your API key using:

```bash
comanda configure
```

## Available Parameters

The `openai-responses` step type supports the following parameters:

- `type`: Must be set to "openai-responses"
- `input`: Input files or content
- `model`: OpenAI model to use (e.g., "gpt-4o")
- `instructions`: System message for the model
- `action`: Prompt for the model
- `output`: Output destination
- `previous_response_id`: ID of a previous response for stateful interactions
- `max_output_tokens`: Maximum number of tokens to generate
- `temperature`: Sampling temperature (0.0 to 2.0)
- `top_p`: Nucleus sampling parameter (0.0 to 1.0)
- `stream`: Whether to stream the response
- `tools`: Array of tools the model can use
- `response_format`: Format specification for the response (e.g., JSON)
