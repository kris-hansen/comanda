![comanda](comanda-small.jpg)

# comanda

**Declarative AI pipelines for the command line.**

Define LLM workflows in YAML. Run Claude Code, Codex, and Gemini CLI in parallel. Version control everything.

🌐 **[comanda.sh](https://comanda.sh)** — Getting started, features, and templates

## Install

```bash
brew install kris-hansen/comanda/comanda
```

Or: `go install github.com/kris-hansen/comanda@latest` · [Releases](https://github.com/kris-hansen/comanda/releases)

## Quick Start

```bash
comanda configure                              # Set up API keys
comanda generate "review this code for bugs"   # Generate workflow from English
comanda process workflow.yaml                  # Run a workflow
```

## Example

```yaml
parallel-process:
  claude:
    input: STDIN
    model: claude-code
    action: "Analyze architecture"
    output: $CLAUDE

  gemini:
    input: STDIN
    model: gemini-cli
    action: "Identify patterns"
    output: $GEMINI

synthesize:
  input: "Claude: $CLAUDE\nGemini: $GEMINI"
  model: claude-code
  action: "Combine into recommendations"
  output: STDOUT
```

```bash
cat main.go | comanda process multi-agent.yaml
```

## Features

- **Multi-agent** — Claude Code, Gemini CLI, OpenAI Codex in parallel
- **Agentic loops** — Iterative refinement with tool use
- **Codebase indexing** — Persistent code context across workflows  
- **Git worktrees** — Parallel execution in isolated branches
- **All the I/O** — Files, URLs, databases, images, chunking

See [comanda.sh/features](https://comanda.sh/features) for full details.

## Documentation

- [Examples](examples/README.md)
- [Multi-Agent Patterns](examples/multi-agent/README.md)
- [Agentic Loops](examples/agentic-loop/)
- [Server API](docs/server-api.md)

## License

MIT

## Download History

[![Download History](https://skill-history.com/chart/kris-hansen/comanda.svg)](https://skill-history.com/kris-hansen/comanda)
