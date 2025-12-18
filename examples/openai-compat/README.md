# OpenAI Compatibility Mode Examples

These workflows are designed for use with Comanda's OpenAI API compatibility mode,
allowing tools like Cline to integrate with Comanda workflows.

## Setup

1. Enable OpenAI compatibility mode:
   ```bash
   comanda server openai-compat on
   ```

2. Start the server:
   ```bash
   comanda server
   ```

3. Configure your OpenAI-compatible client (e.g., Cline):
   - API Provider: OpenAI Compatible
   - Base URL: `http://localhost:8080/v1`
   - API Key: Your Comanda bearer token (run `comanda server show` to see it)
   - Model: The workflow name (e.g., `openai-compat/assistant`)

## Available Workflows

### assistant.yaml
A simple general-purpose assistant using Claude Opus 4.5.
- Uses memory for conversation context
- Single-step processing

### dual-llm.yaml
An advanced workflow that queries both GPT-4.1 and Claude Opus 4.5,
then synthesizes the best response from both.
- Parallel model queries
- Memory-enabled for conversation context
- Synthesis step for optimal responses

## API Endpoints

When OpenAI compatibility mode is enabled:

- `GET /v1/models` - Lists all available workflows as models
- `POST /v1/chat/completions` - Executes a workflow with chat input

## Example API Calls

### List models
```bash
curl -H "Authorization: Bearer $TOKEN" http://localhost:8080/v1/models
```

### Chat completion
```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "openai-compat/assistant",
    "messages": [
      {"role": "user", "content": "Hello, how are you?"}
    ],
    "stream": false
  }'
```

### Streaming
```bash
curl -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "openai-compat/assistant",
    "messages": [
      {"role": "user", "content": "Write a short poem about coding"}
    ],
    "stream": true
  }'
```

## Creating Your Own Workflows

Any YAML workflow in your data directory can be used as a model. Key patterns:

1. **Use STDIN for input**: The last user message is passed as STDIN
2. **Enable memory**: Set `memory: true` to receive conversation history
3. **Output to STDOUT**: The final step should output to STDOUT for the response

Example minimal workflow:
```yaml
respond:
  input: STDIN
  model: your-preferred-model
  memory: true
  action: |
    Your system prompt here
  output: STDOUT
```
