![robot image](comanda-small.jpg)

# comanda

**Declarative AI pipelines for the command line.** Define LLM workflows in YAML, run them anywhere, version control everything.

🌐 [comanda.sh](https://comanda.sh) · 📖 [Examples](examples/README.md) · ⭐ Star to support!

```bash
cat main.go | comanda process code-review.yaml      # Pipe code through AI
comanda process multi-agent/architecture.yaml       # Run multiple agents in parallel
comanda generate "review this PR for security"      # Generate workflows from English
```

## Install

```bash
brew install kris-hansen/comanda/comanda    # macOS
go install github.com/kris-hansen/comanda@latest   # or via Go
```

```bash
comanda configure   # Set up API keys
comanda --version   # Verify install
```

## Quick Start

**hello.yaml:**
```yaml
hello:
  input: NA
  model: gpt-4o
  action: Write a haiku about programming
  output: STDOUT
```

```bash
comanda process hello.yaml
```

## Core Features

### Multi-Agent Orchestration

Run Claude Code, Codex, and Gemini CLI in parallel:

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

### Agentic Loops

Iterate until the LLM decides work is complete:

```yaml
implement:
  agentic_loop:
    max_iterations: 5
    exit_condition: llm_decides
    allowed_paths: [./src, ./tests]
    tools: [Read, Write, Edit, Bash]
  input: STDIN
  model: claude-code
  action: "Implement and test. Say DONE when complete."
  output: STDOUT
```

### Codebase Indexing

Persistent code context for AI workflows:

```bash
comanda index capture ~/project -n myproject   # Index once
comanda index list                              # See all indexes
comanda index diff myproject                    # What changed?
```

```yaml
analyze:
  codebase_index:
    use: [project1, project2]   # Load from registry
    aggregate: true
  model: claude
  action: "Compare these codebases"
```

### Git Worktrees

Parallel Claude Code execution in isolated worktrees:

```yaml
worktrees:
  repo: .
  trees:
    - name: feature-a
      new_branch: true
    - name: feature-b
      new_branch: true

parallel-process:
  implement-a:
    worktree: feature-a
    model: claude-code
    action: "Implement feature A"

  implement-b:
    worktree: feature-b
    model: claude-code
    action: "Implement feature B"
```

### Workflow Visualization

```bash
comanda chart workflow.yaml
```

```
+================================================+
| PARALLEL: parallel-process (3 steps)           |
+------------------------------------------------+
  ├─ claude-analysis
  ├─ gemini-analysis
  └─ codex-analysis
+================================================+
                        |
                        v
+------------------------------------------------+
| synthesize                                     |
+------------------------------------------------+
```

## Supported Providers

| Type | Providers |
|------|-----------|
| **Cloud APIs** | OpenAI, Anthropic, Google, X.AI, DeepSeek, Moonshot |
| **Local** | Ollama, vLLM, any OpenAI-compatible endpoint |
| **Agentic CLIs** | Claude Code, Gemini CLI, OpenAI Codex |

## All Features

| Feature | Description |
|---------|-------------|
| Multi-provider | Cloud APIs, local models, agentic CLIs |
| Parallel execution | Run steps concurrently |
| Agentic loops | Iterative refinement with exit conditions |
| Tool execution | Shell commands within workflows |
| Codebase indexing | Persistent code context across workflows |
| Git worktrees | Parallel branches for isolated execution |
| File operations | Wildcards, chunking, batch processing |
| Vision | Analyze images and screenshots |
| Web scraping | Fetch and process URLs |
| Database I/O | PostgreSQL read/write |
| Memory | Persistent context via COMANDA.md |
| qmd integration | Local semantic search |
| Server mode | HTTP API for any workflow |
| Visualization | ASCII workflow charts |

## Documentation

- [Examples](examples/README.md) — Sample workflows
- [Multi-Agent Patterns](examples/multi-agent/README.md)
- [Agentic Loops](examples/agentic-loop/)
- [Tool Use Guide](examples/tool-use/README.md)
- [Server API](docs/server-api.md)

## Server Mode

```bash
comanda server
curl -X POST "http://localhost:8080/process?filename=review.yaml" \
  -d '{"input": "code to review"}'
```

## Development

```bash
make deps && make build && make test
```

## License

MIT — see [LICENSE](LICENSE)
