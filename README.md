![comanda](comanda-small.jpg)

# comanda

Declarative AI workflows for the command line.

Comanda turns repeatable AI work into YAML pipelines you can run, review, and
version control. Use it to generate workflows from natural language, run
multi-model pipelines, orchestrate agentic loops, process files, call tools, and
wire Claude Code, Gemini CLI, OpenAI Codex, Kimi Code, and API models together.

For the full guide, feature tour, and copy-ready workflow templates, start at
[comanda.sh](https://comanda.sh):

- [Features](https://comanda.sh/features)
- [Templates](https://comanda.sh/templates)
- [GitHub examples](examples/README.md)

## Install

```bash
brew install kris-hansen/comanda/comanda
```

Or install with Go:

```bash
go install github.com/kris-hansen/comanda@latest
```

See [GitHub Releases](https://github.com/kris-hansen/comanda/releases) for
prebuilt binaries.

## Quick Start

```bash
comanda configure
comanda generate workflow.yaml "review this code for bugs"
comanda process workflow.yaml
```

Pipe input through a workflow:

```bash
cat main.go | comanda process workflow.yaml
```

Inspect or iterate on a workflow:

```bash
comanda chart workflow.yaml
comanda improve workflow.yaml "Add security findings and suggested fixes"
```

## Minimal Workflow

```yaml
summarize:
  input: STDIN
  model: gpt-4o
  action: "Summarize the input in three bullets."
  output: STDOUT
```

Run it:

```bash
cat notes.md | comanda process summarize.yaml
```

## What You Can Build

- Multi-agent reviews with Claude Code, Gemini CLI, OpenAI Codex, Kimi Code, and API models
- Agentic loops that iterate until work is complete
- File, URL, image, PDF, database, and batch-processing workflows
- Tool-enabled workflows with explicit command allowlists
- Codebase indexes for persistent project context
- Git worktree workflows for parallel isolated implementation
- Server-backed workflows callable over HTTP

## More Examples

Most examples and walkthroughs live on [comanda.sh](https://comanda.sh):

- [Browse workflow templates](https://comanda.sh/templates)
- [Explore all features](https://comanda.sh/features)
- [Local examples directory](examples/README.md)
- [Multi-agent patterns](examples/multi-agent/README.md)
- [Agentic loops](examples/agentic-loop/)
- [Tool use](examples/tool-use/README.md)
- [Server API](docs/server-api.md)

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
