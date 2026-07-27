![comanda](comanda-small.jpg)

# comanda

> **Make coding agents earn their exit.**
>
> Comanda is the terminal-native runtime for durable, self-improving agent work in a repository. Describe a workflow in English, inspect the generated program, run the coding agents you already use, and stop only when your own quality gates say the work is done.

[![GitHub Stars](https://img.shields.io/github/stars/kris-hansen/comanda?style=social)](https://github.com/kris-hansen/comanda)
[![Release](https://img.shields.io/github/v/release/kris-hansen/comanda)](https://github.com/kris-hansen/comanda/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

## From an idea to a governed workflow

Comanda starts with English, but ends with a YAML program you can inspect, version, and improve.

```bash
# Describe the outcome
comanda generate feature-loop.yaml \
  "Implement this feature until tests and security checks pass"

# Inspect the generated workflow as a graph
comanda chart feature-loop.yaml --format mermaid

# Run it in your repository
comanda process feature-loop.yaml

# Evolve the program with plain-English feedback
comanda improve feature-loop.yaml \
  "Add a Codex reviewer and require typecheck before completion"
```

**Describe it. Inspect it. Run it. Improve it. Commit it.**

## A loop does not finish because an agent says “DONE”

Long-running work needs an observable exit criterion. Comanda's agentic loops persist state, refine subsequent prompts from prior results, and run automated quality gates after each iteration. Interrupt a run and resume it from its last checkpoint.

```yaml
agentic-loop:
  config:
    name: code-quality-improvement
    stateful: true
    checkpoint_interval: 2
    max_iterations: 10
    prompt_improvement:
      enabled: true
    quality_gates:
      - name: syntax-check
        type: syntax
        on_fail: abort
      - name: tests
        command: "make test"
        on_fail: retry
    allowed_paths: ["./src", "./tests"]
```

Run the complete [code-quality loop](examples/agentic-loop/code-quality-loop.yaml), then inspect or resume it:

```bash
comanda process examples/agentic-loop/code-quality-loop.yaml
comanda loop status code-quality-improvement
comanda loop resume code-quality-improvement
```

Comanda supports retry, abort, and skip policies; syntax, security, and custom-command gates; bounded or indefinite runs; and creator/checker or dependent multi-loop workflows. See [agentic-loop examples](examples/agentic-loop/README.md).

## Use the agents you already have

Coordinate Claude Code, Gemini CLI, OpenAI Codex, Kimi Code, API models, and local models in one workflow. Give agents distinct roles, pass work through files or standard input/output, and keep the results visible in your repository.

```yaml
parallel-process:
  architecture:
    input: STDIN
    model: claude-code
    action: "Review the design and identify implementation risks."
    output: .comanda/architecture-review.md

  implementation:
    input: STDIN
    model: openai-codex
    action: "Review the implementation plan and identify practical risks."
    output: .comanda/implementation-review.md

synthesize:
  input:
    - .comanda/architecture-review.md
    - .comanda/implementation-review.md
  model: gemini-cli
  action: "Produce one prioritized recommendation."
  output: STDOUT
```

Run this exact [parallel-review workflow](examples/multi-agent/parallel-review.yaml) with `git diff | comanda process examples/multi-agent/parallel-review.yaml`, or explore [multi-agent workflows](examples/multi-agent/README.md) and [parallel processing](examples/parallel-processing/).

## Keep agent work inside the repository model

- **Git worktrees** — run parallel implementations in isolated branches, then inspect and compare their diffs.
- **Codebase indexes** — capture repository structure, symbols, conventions, operational notes, and risk areas for later agent work; incrementally update or diff an index as the code changes.
- **Explicit boundaries** — constrain agentic loops to allowed paths and named tools.
- **Quality gates** — use your own tests, linters, and security checks as the definition of done.

```bash
comanda index capture --enhance
comanda index diff my-project
comanda index update my-project
```

See [codebase-index examples](examples/codebase-index/README.md).

## Build a workflow once. Run it everywhere.

Workflows are plain files. Review them in pull requests, run them from the terminal or CI, serve them over HTTP, or expose them to MCP clients.

```bash
# Expose checked-in workflows as MCP tools and skills as MCP prompts
comanda mcp

# Run a workflow as part of a shell pipeline
git diff | comanda process review.yaml
```

See the [MCP server guide](docs/mcp-server.md), [server API](docs/server-api.md), and [skills examples](examples/skills/README.md).

## Why Comanda?

| If you need to… | Reach for… |
| --- | --- |
| Build an AI-powered application in code | An agent framework such as LangGraph, CrewAI, or an SDK |
| Design a business automation on a visual canvas | A visual workflow platform |
| Ask one coding agent to complete a task | Its native CLI |
| Generate, govern, resume, and reuse durable agent work in a repository | **Comanda** |

Comanda sits between a coding agent and an agent framework: it turns agent work into a durable, reviewable program that runs where developers already work.

## Install

```bash
# macOS
brew install kris-hansen/comanda/comanda

# Go
go install github.com/kris-hansen/comanda@latest
```

See [GitHub Releases](https://github.com/kris-hansen/comanda/releases) for prebuilt binaries for macOS, Linux, and Windows.

## More capabilities

- Generate and improve workflows from natural-language feedback
- Render workflow structure as terminal or Mermaid graphs with validation
- Process files, URLs, images, PDFs, databases, and batches with chunking
- Use shell tools with explicit allowlists
- Call OpenAI, Anthropic, Google, xAI, DeepSeek, Moonshot, Sakana, Ollama, vLLM, llama.cpp, and compatible providers
- Run workflows as HTTP endpoints or OpenAI-compatible server routes

Browse [all examples](examples/README.md) or visit [comanda.sh](https://comanda.sh) for documentation and templates.

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
