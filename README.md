![robot image](comanda-small.jpg)

# COMandA

**Declarative AI pipelines for the command line.** Define LLM workflows in YAML, run them anywhere, version control everything.

```bash
# Pipe code through multiple AI agents
cat main.go | comanda process code-review.yaml

# Compare how different models solve a problem
comanda process model-comparison.yaml

# Run Claude Code, Codex, and Gemini CLI in parallel
echo "Design a REST API" | comanda process multi-agent/architecture.yaml
```

## Why comanda?

**For AI-powered development workflows:**
- Run Claude Code, OpenAI Codex, and Gemini CLI side-by-side
- Chain multiple agents for code review → test generation → documentation
- Get diverse perspectives on architecture decisions

**For reproducible AI pipelines:**
- YAML workflows you can version control and share
- Same workflow runs locally, in CI, or on a server
- Switch providers without changing your pipeline

**For command-line power users:**
- Pipes, redirects, scripts—works like `grep` or `jq`
- Process files, URLs, databases, screenshots
- Batch operations with wildcards and parallel execution

## Quick Start

### Install

```bash
# macOS
brew install kris-hansen/comanda/comanda

# Or via Go
go install github.com/kris-hansen/comanda@latest

# Or download from GitHub Releases
```

### Configure

```bash
comanda configure
# Select providers (OpenAI, Anthropic, Google, Ollama, Claude Code, etc.)
# Enter API keys where needed
```

### Run Your First Workflow

Create `hello.yaml`:
```yaml
generate:
  input: NA
  model: gpt-4o
  action: Write a haiku about programming
  output: STDOUT
```

```bash
comanda process hello.yaml
```

## Multi-Agent Workflows

Run multiple agentic coding tools in parallel and synthesize their outputs:

```yaml
parallel-process:
  claude-analysis:
    input: STDIN
    model: claude-code
    action: "Analyze architecture and trade-offs"
    output: $CLAUDE_RESULT

  gemini-analysis:
    input: STDIN
    model: gemini-cli
    action: "Identify patterns and best practices"
    output: $GEMINI_RESULT

  codex-analysis:
    input: STDIN
    model: openai-codex
    action: "Focus on implementation structure"
    output: $CODEX_RESULT

synthesize:
  input: |
    Claude: $CLAUDE_RESULT
    Gemini: $GEMINI_RESULT
    Codex: $CODEX_RESULT
  model: claude-code
  action: "Combine into unified recommendation"
  output: STDOUT
```

```bash
echo "Design a real-time collaborative editor" | comanda process architecture.yaml
```

### Supported Agents

| Agent | Model Names | Best For |
|-------|-------------|----------|
| **Claude Code** | `claude-code`, `claude-code-opus`, `claude-code-sonnet` | Deep reasoning, synthesis |
| **Gemini CLI** | `gemini-cli`, `gemini-cli-pro`, `gemini-cli-flash` | Broad knowledge, patterns |
| **OpenAI Codex** | `openai-codex`, `openai-codex-o3` | Implementation, code structure |

No API keys needed for these—they use their own CLI authentication.

## Common Use Cases

### Code Review Pipeline

```yaml
review:
  input: "src/*.go"
  model: claude-code
  action: "Review for bugs, security issues, and improvements"
  output: review.md
```

### Data Analysis

```bash
comanda process analyze.yaml < quarterly_data.csv
```

### Model Comparison

```yaml
parallel-process:
  gpt4:
    input: NA
    model: gpt-4o
    action: "Write a function to parse JSON"
    output: gpt4-solution.py

  claude:
    input: NA
    model: claude-3-5-sonnet-latest
    action: "Write a function to parse JSON"
    output: claude-solution.py

compare:
  input: [gpt4-solution.py, claude-solution.py]
  model: gpt-4o-mini
  action: "Compare these implementations"
  output: STDOUT
```

### Server Mode

Turn any workflow into an HTTP API:

```bash
comanda server
curl -X POST "http://localhost:8080/process?filename=review.yaml" \
  -d '{"input": "code to review"}'
```

## Features

| Feature | Description |
|---------|-------------|
| **Multi-provider** | OpenAI, Anthropic, Google, X.AI, Ollama, vLLM, Claude Code, Codex, Gemini CLI |
| **Parallel processing** | Run independent steps concurrently |
| **Tool execution** | Run shell commands (`ls`, `jq`, `grep`, custom CLIs) within workflows |
| **File operations** | Read/write files, wildcards, batch processing |
| **Vision support** | Analyze images and screenshots |
| **Web scraping** | Fetch and process URLs |
| **Database I/O** | Read from and write to PostgreSQL |
| **Chunking** | Auto-split large files for processing |
| **Memory** | Persistent context via COMANDA.md |
| **Branching** | Conditional workflows with `defer:` |
| **Visualization** | ASCII workflow charts with `comanda chart` |

## Visualize Workflows

See the structure of any workflow at a glance:

```bash
comanda chart workflow.yaml
```

```
+================================================+
| WORKFLOW: examples/parallel-data-processing... |
+================================================+

+------------------------------------------------+
|            INPUT: examples/test.csv            |
+------------------------------------------------+
                        |
                        v
+================================================+
| PARALLEL: parallel-process (3 steps)           |
+------------------------------------------------+
  +----------------------------------------------+
  | [OK] analyze_csv                             |
  | Model:  gpt-4o-mini                          |
  | Action: Analyze this CSV data and            |
  +----------------------------------------------+
  ~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~
  +----------------------------------------------+
  | [OK] extract_entities                        |
  | Model:  gpt-4o-mini                          |
  | Action: Extract all named entities           |
  +----------------------------------------------+
+================================================+
                        |
                        v
+------------------------------------------------+
| [OK] consolidate_results                       |
| Model:  gpt-4o                                 |
| Action: Create a comprehensive data analysis   |
+------------------------------------------------+

                        |
                        v
+------------------------------------------------+
|     OUTPUT: comprehensive-report.txt           |
+------------------------------------------------+

+================================================+
| STATISTICS                                     |
|------------------------------------------------|
| Steps: 4 total, 3 parallel                     |
| Valid: 4/4                                     |
+================================================+
```

## Documentation

- **[Examples](examples/README.md)** — Sample workflows for common tasks
- **[Multi-Agent Workflows](examples/multi-agent/README.md)** — Claude Code + Codex + Gemini CLI patterns
- **[Claude Code Examples](examples/claude-code/README.md)** — Agentic coding workflows
- **[Tool Use Guide](examples/tool-use/README.md)** — Execute shell commands in workflows
- **[Server API](docs/server-api.md)** — HTTP endpoints reference
- **[Configuration Guide](docs/adding-new-model-guide.md)** — Adding models and providers

## Installation Options

### Homebrew (macOS)
```bash
brew install kris-hansen/comanda/comanda
```

### Go Install
```bash
go install github.com/kris-hansen/comanda@latest
```

### Pre-built Binaries
Download from [GitHub Releases](https://github.com/kris-hansen/comanda/releases) for Windows, macOS, and Linux.

### Build from Source
```bash
git clone https://github.com/kris-hansen/comanda.git
cd comanda && go build
```

## Configuration

### Provider Setup

```bash
comanda configure
```

Interactive prompts for:
- Provider selection (OpenAI, Anthropic, Google, X.AI, Ollama, vLLM)
- API keys
- Model names and capabilities

### Agentic CLI Tools

These use their own authentication—no comanda configuration needed:

```bash
# Claude Code
claude --version  # Verify installed

# Gemini CLI
gemini --version

# OpenAI Codex
codex --version
```

### Environment File

Configuration stored in `.env` (current directory) or custom path:

```bash
export COMANDA_ENV=/path/to/.env
```

### Encryption

Protect your API keys:

```bash
comanda configure --encrypt
```

## Development

```bash
make deps      # Install dependencies
make build     # Build binary
make test      # Run tests
make lint      # Run linter
make dev       # Full dev cycle
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution guidelines.

## License

MIT — see [LICENSE](LICENSE)

## Acknowledgments

- OpenAI, Anthropic, and Google for their APIs
- Ollama and vLLM for local model support
- The Go community
