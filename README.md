![comanda](comanda-small.jpg)

# Comanda

> **The CLI-native orchestrator for AI agent workflows.**
>
> Define multi-model AI pipelines in YAML. Run them from the terminal with pipes, redirects, and version control. No Python boilerplate. No GUI lock-in. Just workflows that work where you already work.

[![GitHub Stars](https://img.shields.io/github/stars/kris-hansen/comanda?style=social)](https://github.com/kris-hansen/comanda)
[![Release](https://img.shields.io/github/v/release/kris-hansen/comanda)](https://github.com/kris-hansen/comanda/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## Demo: Multi-Agent Code Review

```yaml
parallel-process:
  architecture:
    input: STDIN
    model: claude-code
    action: "Analyze the input for architecture and security issues."
    output: .comanda/claude-review.md

  performance:
    input: STDIN
    model: gemini-cli
    action: "Identify performance bottlenecks and unnecessary complexity."
    output: .comanda/gemini-review.md

synthesize:
  input:
    - .comanda/claude-review.md
    - .comanda/gemini-review.md
  model: gpt-4o
  action: "Combine the reviews into prioritized recommendations."
  output: STDOUT
```

Run it:

```bash
cat main.go | comanda process code-review.yaml
```

**[See more examples →](https://comanda.sh/templates)** — templates, walkthroughs, and full documentation.

## Install

```bash
# macOS
brew install kris-hansen/comanda/comanda

# Go
go install github.com/kris-hansen/comanda@latest
```

See [GitHub Releases](https://github.com/kris-hansen/comanda/releases) for prebuilt binaries for macOS, Linux, and Windows.

## Why Comanda?

Most AI workflow tools fall into two camps:

| Tool | Approach | Best for |
| --- | --- | --- |
| LangChain / CrewAI | Python SDKs and frameworks | Building AI-powered applications |
| Dify / n8n | Visual GUI builders | Drag-and-drop automation |
| **Comanda** | **CLI-native YAML + pipes** | **Developers who live in terminals, repos, and CI/CD** |

Comanda is not a framework you import. It is a harness you run. Treat AI workflows as infrastructure-as-code: declarative, version-controlled, and composable with standard Unix primitives.

## What You Can Build

- **Multi-agent code reviews** — Claude Code, Gemini CLI, Codex, Kimi Code, and API models working in parallel or sequence
- **Agentic loops** — iterative refinement with quality gates, retries, and state management
- **Batch processing** — files, URLs, images, PDFs, and databases with wildcards and chunking
- **Git worktree workflows** — parallel isolated implementation in separate branches
- **CI/CD pipelines** — AI-powered checks in GitHub Actions, GitLab CI, or any shell environment
- **MCP server mode** — expose workflows as tools for Claude Code, Codex, Kimi Code, Cursor, and other MCP clients

## Quick Start

```bash
# Configure API keys
comanda configure

# Generate a workflow from English
comanda generate review.yaml "review this code for bugs"

# Run it
comanda process review.yaml

# Pipe input through it
cat main.go | comanda process review.yaml

# Visualize workflow structure
comanda chart review.yaml

# Improve an existing workflow
comanda improve review.yaml "Add security findings and suggested fixes"
```

## Example: Summarize Notes

```yaml
summarize:
  input: STDIN
  model: gpt-4o
  action: "Summarize the input in three bullets."
  output: STDOUT
```

```bash
cat notes.md | comanda process summarize.yaml
```

## Features

- **Multi-Model Orchestration** — Claude Code, Gemini CLI, OpenAI Codex, Kimi Code, and API models in parallel or sequence
- **YAML-Native** — workflows are plain text: version, share, and review them in pull requests
- **Agentic Loops** — iterative refinement with quality gates, retries, and state management
- **Unix Philosophy** — works with pipes (`|`), redirects (`>`), and shell scripts
- **Codebase Indexing** — generate agent-ready architecture, conventions, and project context
- **Workflow Improvement** — refine YAML from plain-English feedback with `comanda improve`
- **Reusable Skills** — package common patterns and share them across projects
- **Model Choice** — one interface for OpenAI, Anthropic, Google, Sakana, Ollama, xAI, and AWS Bedrock
- **MCP Server** — run `comanda mcp` to expose workflows as tools for agent clients

## Documentation

- [Website & templates](https://comanda.sh)
- [Local examples](examples/README.md)
- [Multi-agent code review](examples/multi-agent/code-review.yaml)
- [Multi-agent patterns](examples/multi-agent/README.md)
- [Agentic loops](examples/agentic-loop/)
- [Tool use](examples/tool-use/README.md)
- [Server API](docs/server-api.md)
- [MCP server](docs/mcp-server.md)

## Development

```bash
make deps
make build
make test
```

## License

MIT

## Download History

[![Download History](https://skill-history.com/chart/kris-hansen/comanda.svg)](https://skill-history.com/kris-hansen/comanda)
