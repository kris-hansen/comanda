# qmd Integration Examples

These examples demonstrate comanda's integration with [qmd](https://github.com/tobi/qmd), a local search engine for markdown documents and knowledge bases.

## Prerequisites

```bash
# Install qmd
bun install -g @tobilu/qmd

# Add to PATH (add to ~/.zshrc or ~/.bashrc)
export PATH="$HOME/.bun/bin:$PATH"
```

## Examples

### 1. Index with qmd Registration (`index-with-qmd.yaml`)

Index a codebase and automatically register it as a qmd collection:

```bash
comanda process examples/qmd/index-with-qmd.yaml
```

### 2. RAG Workflow (`rag-workflow.yaml`)

Retrieval-Augmented Generation using qmd for context retrieval:

```bash
# First, create a collection (run once)
qmd collection add ./docs --name docs --mask "**/*.md"

# Then run the RAG workflow
echo "How do I configure authentication?" | comanda process examples/qmd/rag-workflow.yaml
```

### 3. Code Search (`code-search.yaml`)

Search indexed code and analyze it with an LLM:

```bash
# Set up your codebase collection first
qmd collection add ./src --name mycode --mask "**/*.{go,py,ts,js}"

# Search and analyze
echo "error handling" | comanda process examples/qmd/code-search.yaml
```

## Workflow Patterns

### Pattern 1: Index → Search → Generate

```yaml
# Index codebase with qmd registration
index:
  type: codebase-index
  config:
    root: ./src
    qmd:
      collection: code
      context: "Application source code"

# Search for context
search:
  type: qmd-search
  qmd_search:
    query: "user authentication"
    collection: code
    limit: 5
  output: CONTEXT

# Generate with context
generate:
  input: "Context:\n${CONTEXT}\n\nTask: Explain the auth flow"
  model: claude-sonnet
  action: Generate explanation
  output: STDOUT
```

### Pattern 2: Multi-Collection RAG

```yaml
# Search docs
search-docs:
  type: qmd-search
  qmd_search:
    query: "${QUESTION}"
    collection: docs
    limit: 3
  output: DOC_CONTEXT

# Search code
search-code:
  type: qmd-search
  qmd_search:
    query: "${QUESTION}"
    collection: code
    limit: 3
  output: CODE_CONTEXT

# Combine and answer
answer:
  input: |
    Documentation:
    ${DOC_CONTEXT}
    
    Code:
    ${CODE_CONTEXT}
    
    Question: ${QUESTION}
  model: gpt-4o
  action: Answer comprehensively
  output: STDOUT
```

## Search Modes

| Mode | Command | Speed | Use Case |
|------|---------|-------|----------|
| BM25 | `mode: search` | Instant | Keyword matching (default) |
| Vector | `mode: vsearch` | Slow | Semantic similarity |
| Hybrid | `mode: query` | Slowest | Best quality, uses LLM reranking |

**Tip:** Always start with `search` (BM25). Only use `vsearch` or `query` if keyword matching doesn't find what you need.

## Tips

1. **Run `qmd embed` once** after creating a collection to enable vector search
2. **Use descriptive context** when registering collections
3. **Set appropriate `min_score`** to filter low-relevance results
4. **Use `format: json`** when you need structured output for further processing
