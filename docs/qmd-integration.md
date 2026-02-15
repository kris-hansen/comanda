# qmd Integration

comanda integrates with [qmd](https://github.com/tobi/qmd) (Query Markup Documents) to provide local search capabilities for your knowledge bases, documentation, and codebase indexes.

## What is qmd?

qmd is an on-device search engine that combines:
- **BM25 full-text search** - Fast keyword matching
- **Vector semantic search** - Find conceptually similar content
- **LLM re-ranking** - Highest quality results via hybrid search

All processing happens locally using GGUF models—no API keys required for search.

## Installation

```bash
# Install qmd
bun install -g @tobilu/qmd

# Verify installation
qmd --version
```

## Integration Features

### 1. Automatic Collection Registration

When using `codebase-index`, you can automatically register the generated index with qmd:

```yaml
index-codebase:
  type: codebase-index
  config:
    root: ./src
    qmd:
      collection: myproject        # Collection name in qmd
      embed: false                 # Run qmd embed after (optional, slow)
      context: "Source code for my project"  # Description for better search
```

This creates a qmd collection that you can search:

```bash
qmd search "authentication" -c myproject
```

### 2. qmd-search Step

Query qmd collections directly in your workflows:

```yaml
find-context:
  type: qmd-search
  qmd_search:
    query: "${USER_QUESTION}"      # Supports variable substitution
    collection: myproject          # Optional: search specific collection
    mode: search                   # search (BM25), vsearch (vector), query (hybrid)
    limit: 5                       # Number of results
    min_score: 0.3                 # Minimum relevance score
    format: text                   # text, json, or files
  output: CONTEXT
```

### Search Modes

| Mode | Speed | Quality | Use Case |
|------|-------|---------|----------|
| `search` | ⚡ Instant | Good | Default, keyword matching |
| `vsearch` | 🐢 Slow | Better | Semantic similarity (needs embeddings) |
| `query` | 🐌 Slowest | Best | Hybrid + LLM reranking |

**Recommendation:** Start with `search` (BM25). Only use `vsearch` or `query` when keyword matching fails.

## Example Workflows

### RAG (Retrieval-Augmented Generation)

```yaml
# Index your docs first
index-docs:
  type: codebase-index
  config:
    root: ./docs
    qmd:
      collection: docs
      context: "Project documentation and guides"

# Search for relevant context
retrieve:
  type: qmd-search
  qmd_search:
    query: "${QUESTION}"
    collection: docs
    limit: 5
  output: CONTEXT

# Generate answer with context
answer:
  input: |
    Context from documentation:
    ${CONTEXT}
    
    Question: ${QUESTION}
  model: claude-sonnet
  action: Answer the question using only the provided context. Cite sources.
  output: STDOUT
```

### Code-Aware Assistant

```yaml
# Index codebase with qmd registration
index:
  type: codebase-index
  config:
    root: .
    qmd:
      collection: codebase
      context: "Application source code"

# Find relevant code
find-code:
  type: qmd-search
  qmd_search:
    query: "error handling middleware"
    collection: codebase
    mode: search
    limit: 10
  output: RELEVANT_CODE

# Analyze with LLM
analyze:
  input: |
    Relevant code snippets:
    ${RELEVANT_CODE}
    
    Task: ${TASK}
  model: gpt-4o
  action: Analyze the code and suggest improvements.
  output: STDOUT
```

### Multi-Collection Search

```yaml
# Search across multiple knowledge bases
search-all:
  type: qmd-search
  qmd_search:
    query: "deployment configuration"
    # No collection = search all
    limit: 10
    min_score: 0.4
  output: RESULTS
```

## CLI Usage

You can also use qmd directly:

```bash
# Index a directory
qmd collection add ./docs --name docs --mask "**/*.md"
qmd context add qmd://docs "Project documentation"

# Generate embeddings (for semantic search)
qmd embed

# Search
qmd search "how to deploy"           # BM25 (fast)
qmd vsearch "deployment process"     # Vector (semantic)
qmd query "deployment best practices" # Hybrid + reranking

# Get a specific document
qmd get docs/deployment.md
```

## Tips

1. **Start simple**: Use `mode: search` (BM25) first—it's instant and often sufficient
2. **Run `qmd embed`**: Only needed once per collection for semantic search
3. **Use collections**: Organize knowledge into collections for focused searches
4. **Add context**: Descriptive context helps qmd rank results better
5. **Refresh regularly**: Run `qmd update` to index new files

## Environment Variables

| Variable | Description |
|----------|-------------|
| `PATH` | Must include `~/.bun/bin` for qmd |
| `XDG_CACHE_HOME` | Override qmd cache location (default: `~/.cache/qmd`) |

## Troubleshooting

### qmd not found
```bash
export PATH="$HOME/.bun/bin:$PATH"
```

### Slow searches
- Use `mode: search` instead of `vsearch` or `query`
- Reduce `limit` to fetch fewer results
- Consider running qmd as an MCP server for warm model loading

### No results
- Check collection exists: `qmd status`
- Lower `min_score` threshold
- Try broader search terms
