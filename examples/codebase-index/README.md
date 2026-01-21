# Codebase Index Examples

The `codebase-index` step type scans a repository and generates a compact Markdown index optimized for LLM consumption. This enables downstream workflow steps to understand codebase structure without processing every file.

## Quick Start

```bash
# Generate an index for the sample project
comanda run basic-index.yaml

# Index and analyze with an LLM
comanda run index-and-analyze.yaml
```

## How It Works

1. **Language Detection**: Automatically detects Go, Python, TypeScript, and Flutter codebases
2. **Smart Scanning**: Uses parallel workers with early pruning for performance
3. **Symbol Extraction**: Extracts functions, types, and imports using AST (Go) or regex
4. **Markdown Synthesis**: Generates a structured index with key sections
5. **Variable Export**: Exposes the index as workflow variables for downstream steps

## Workflow Variables

After the `codebase-index` step runs, these variables are available:

| Variable | Description |
|----------|-------------|
| `<REPO>_INDEX` | Full Markdown content of the index |
| `<REPO>_INDEX_PATH` | Path to the saved index file |
| `<REPO>_INDEX_SHA` | Hash of the index content |
| `<REPO>_INDEX_UPDATED` | `true` if index was regenerated |

The `<REPO>` prefix is derived from the repository name (e.g., `SAMPLE_PROJECT_INDEX`).

## Examples

### Basic Index Generation

```yaml
index_codebase:
  step_type: codebase-index
  codebase_index:
    root: ./my-project
    output:
      store: repo
    expose:
      workflow_variable: true
```

### Using Index with LLM

```yaml
index_codebase:
  step_type: codebase-index
  codebase_index:
    root: ./my-project

analyze:
  model: claude-code
  input: STDIN
  action: |
    Here is the codebase index:
    {{ env "MY_PROJECT_INDEX" }}

    Please analyze the architecture.
  output: STDOUT
```

### Custom Configuration

```yaml
index_codebase:
  step_type: codebase-index
  codebase_index:
    root: ./my-project
    output:
      path: docs/INDEX.md      # Custom output path
      store: repo              # Where to store: repo, config, both
      encrypt: false           # Enable AES-256 encryption
    expose:
      workflow_variable: true
      memory:
        enabled: true          # Register as memory source
        key: project.index
    adapters:
      go:
        ignore_dirs:
          - vendor
          - testdata
        priority_files:
          - cmd/**/*.go
    max_output_kb: 100         # Limit output size
```

## Configuration Reference

### `root`
Repository path to scan. Defaults to current directory.

### `output`
- `path`: Custom output file path (default: `.comanda/<repo>_INDEX.md`)
- `store`: Where to save - `repo` (in repo), `config` (~/.comanda/), or `both`
- `encrypt`: Enable AES-256 GCM encryption (saves as `.enc`)

### `expose`
- `workflow_variable`: Export as workflow variable (default: true)
- `memory.enabled`: Register as memory source
- `memory.key`: Key name for memory access

### `adapters`
Per-language configuration overrides:
- `ignore_dirs`: Additional directories to ignore
- `ignore_globs`: File patterns to ignore (e.g., `*.generated.go`)
- `priority_files`: Files to prioritize in scoring
- `replace_defaults`: Replace default ignores instead of extending

### `max_output_kb`
Maximum size of generated index in KB (default: 100).

## Index Output Structure

The generated index includes these sections (when data is available):

1. **Purpose** - Languages detected, file counts
2. **Repository Layout** - Directory tree (depth-limited)
3. **Primary Capabilities** - Inferred from directory names
4. **Entry Points** - main.go, index.ts, etc.
5. **Key Modules** - Grouped by directory
6. **Important Files** - Top-scored files with symbols
7. **Operational Notes** - Build, test, CI files
8. **Risk/Caution Areas** - Auth, crypto, database code
9. **Navigation Hints** - Conventions detected
10. **Footer** - Generation timestamp and scan time

## Supported Languages

| Language | Detection Files | Symbol Extraction |
|----------|-----------------|-------------------|
| Go | `go.mod`, `go.sum` | AST parsing |
| Python | `pyproject.toml`, `requirements.txt` | Regex |
| TypeScript | `tsconfig.json`, `package.json` | Regex |
| Flutter | `pubspec.yaml` | Regex |

## Sample Project

The `sample-project/` directory contains a minimal Go API to demonstrate indexing:

```
sample-project/
  go.mod
  cmd/server/main.go      # Entry point
  pkg/handlers/users.go   # HTTP handlers
  pkg/models/user.go      # Data models
  internal/config/        # Configuration
```

Run the examples against this project to see the index output.
