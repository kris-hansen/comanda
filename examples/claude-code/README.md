# Claude Code Provider Examples

These examples demonstrate using the Claude Code CLI as a provider in comanda workflows.
Instead of calling the Anthropic API directly, these workflows leverage the locally installed
`claude` CLI binary for agentic programming tasks.

## Prerequisites

- Claude Code CLI installed and authenticated (`claude` command available in PATH)
- Run `claude --version` to verify installation

## Available Models

| Model | Description |
|-------|-------------|
| `claude-code` | Default Claude Code model |
| `claude-code-opus` | Uses Claude Opus 4.5 |
| `claude-code-sonnet` | Uses Claude Sonnet 4.5 |
| `claude-code-haiku` | Uses Claude Haiku 4.5 |

## Examples

### Basic Code Review

```yaml
review-code:
  input: "src/main.go"
  model: claude-code
  action: "Review this code for bugs, security issues, and suggest improvements"
  output: STDOUT
```

### Multi-file Analysis

```yaml
analyze-project:
  input: "*.go"
  model: claude-code-sonnet
  action: "Analyze these Go files and provide a summary of the project architecture"
  output: "analysis.md"
```

### Using with File Processing

```yaml
# First step: get list of files
list-files:
  input: "tool: find . -name '*.go' -type f"
  model: NA
  action: "pass through"
  output: STDOUT

# Second step: analyze with Claude Code
analyze:
  input: STDIN
  model: claude-code
  action: "These are Go files in the project. Summarize the main functionality."
  output: STDOUT
```

## Benefits Over Direct API

1. **No API key management** - Uses Claude Code's existing authentication
2. **Agentic capabilities** - Claude Code can read files, search, and execute commands
3. **Local-first** - Works offline with cached auth
4. **Familiar interface** - Same experience as using Claude Code directly

## Configuration

No special configuration needed in `.env`. The provider automatically detects
if the `claude` binary is available in your PATH.

If using a custom installation location, set the Claude Code binary path
to be discoverable via standard PATH mechanisms.
