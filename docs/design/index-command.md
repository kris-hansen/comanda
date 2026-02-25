# Comanda Index Command - Design Document

**Author:** Claudette  
**Date:** 2026-02-25  
**Status:** Implemented (Phase 1-3 complete)

## Overview

Add a top-level `comanda index` command that provides persistent, registry-aware codebase indexing for agentic workflows. The goal is to give AI tools and workflows rich code context awareness across multiple codebases.

## Motivation

The existing `codebase-index` step type works well within workflows but is:
- Transient (regenerates each run)
- Not discoverable outside workflow context
- Single-codebase focused

A dedicated CLI command with registry persistence enables:
- One-time capture with incremental updates
- Cross-codebase awareness in workflows
- Better tooling for agents (list, show, diff)

## Command Structure

```
comanda index <subcommand> [options]

Subcommands:
  capture    Generate and register a code index
  update     Incrementally update an existing index
  diff       Compare stored index vs current state
  show       Display a stored index
  list       List all registered indexes
  remove     Unregister an index (optionally delete file)
```

### Subcommand Details

#### `comanda index capture [path] [options]`

Generate a code index and register it in config.

```bash
# Index current directory, auto-derive name from repo
comanda index capture

# Index specific path with explicit name
comanda index capture ~/clawd/comanda --name comanda

# Custom output location
comanda index capture --output ./my-index.md

# Different formats
comanda index capture --format summary    # Compact (~2KB)
comanda index capture --format structured # Categorized (~20KB, default)
comanda index capture --format full       # Everything (~100KB)

# Store globally (useful for system-wide access)
comanda index capture --global

# Encrypt the output
comanda index capture --encrypt
```

**Options:**
| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--name` | `-n` | Index name (for registry) | Auto-derived from repo |
| `--output` | `-o` | Output file path | `.comanda/index.md` |
| `--format` | `-f` | Output format | `structured` |
| `--global` | `-g` | Store in ~/.comanda/indexes/ | false |
| `--encrypt` | `-e` | Encrypt output | false |
| `--force` | | Overwrite existing | false |

#### `comanda index update [name] [options]`

Incrementally update an existing index.

```bash
# Update index for current directory
comanda index update

# Update specific named index
comanda index update comanda

# Force full regeneration
comanda index update --full
```

**Behavior:**
1. Load existing index metadata (hash, file list)
2. Scan for changed files (modified time + hash comparison)
3. Re-extract only changed files
4. Merge with existing index
5. Update registry metadata

#### `comanda index diff [name] [options]`

Show what changed since last index.

```bash
# Diff current directory's index
comanda index diff

# Diff specific index
comanda index diff comanda

# Show changes since specific date
comanda index diff --since 2024-01-01

# Output as JSON (for tooling)
comanda index diff --json
```

**Output:**
```
Index: comanda (last updated: 2024-02-24)

Added (3):
  + cmd/index.go
  + utils/registry/registry.go
  + docs/design/index-command.md

Modified (2):
  ~ cmd/root.go
  ~ utils/config/env.go

Deleted (1):
  - old/deprecated.go

Summary: 3 added, 2 modified, 1 deleted
```

#### `comanda index show [name] [options]`

Display a stored index.

```bash
# Show index for current directory
comanda index show

# Show specific named index
comanda index show comanda

# Show just the summary section
comanda index show --summary

# Output raw markdown
comanda index show --raw
```

#### `comanda index list [options]`

List all registered indexes.

```bash
comanda index list
```

**Output:**
```
NAME        PATH                        LAST INDEXED         FORMAT      SIZE
comanda     ~/clawd/comanda             2024-02-25 14:20     structured  24KB
clawdbot    ~/clawd/clawdbot            2024-02-24 10:00     structured  18KB
erebor      ~/work/erebor               2024-02-20 09:15     full        89KB
```

**Options:**
| Flag | Description |
|------|-------------|
| `--json` | Output as JSON |
| `--paths` | Show full paths |

#### `comanda index remove <name> [options]`

Remove an index from the registry.

```bash
# Remove from registry only (keep file)
comanda index remove old-project

# Remove from registry and delete index file
comanda index remove old-project --delete
```

## Registry Schema

Stored in `~/.comanda/config.yaml` under `indexes` key:

```yaml
indexes:
  comanda:
    # Source repository
    path: /Users/kris/clawd/comanda
    
    # Where index is stored
    index_path: /Users/kris/clawd/comanda/.comanda/index.md
    
    # Metadata
    last_indexed: 2024-02-25T14:20:00Z
    content_hash: "xxh3:abc123def456"
    format: structured
    file_count: 142
    size_bytes: 24576
    
    # Variable prefix for workflow injection
    var_prefix: COMANDA
    
    # Encryption status
    encrypted: false
    
  clawdbot:
    path: /Users/kris/clawd/clawdbot
    index_path: /Users/kris/.comanda/indexes/clawdbot.md
    last_indexed: 2024-02-24T10:00:00Z
    content_hash: "xxh3:789xyz"
    format: structured
    file_count: 89
    size_bytes: 18432
    var_prefix: CLAWDBOT
    encrypted: false
```

## Workflow Integration

### Using Registry Indexes in Workflows

New `use` field for `codebase_index` step:

```yaml
steps:
  analyze:
    codebase_index:
      use: comanda                    # Single index from registry
    model: claude
    action: "Analyze the architecture"

  compare:
    codebase_index:
      use: [comanda, clawdbot]        # Multiple indexes
    model: claude  
    action: "Compare patterns between these codebases"
```

**Behavior:**
1. Resolve names from registry
2. Load index content from stored paths
3. Inject as variables: `${COMANDA_INDEX}`, `${CLAWDBOT_INDEX}`
4. Optionally check freshness and warn if stale

### Freshness Checking

```yaml
steps:
  review:
    codebase_index:
      use: comanda
      max_age: 24h                    # Warn if older than 24 hours
      auto_update: true               # Auto-update if stale
```

### Aggregated Context

For multi-codebase analysis, provide aggregated view:

```yaml
steps:
  cross_repo_analysis:
    codebase_index:
      use: [comanda, clawdbot, erebor]
      aggregate: true                 # Combine into single context
      aggregate_format: summary       # Use summary of each
    model: claude
    action: "How do these projects interact?"
```

This injects:
```
${AGGREGATED_INDEX}:
  # comanda - CLI for AI workflow orchestration
  [summary content]
  
  # clawdbot - Personal AI assistant framework  
  [summary content]
  
  # erebor - [description]
  [summary content]
```

## Implementation Plan

### Phase 1: Core Command Structure
1. Create `cmd/index.go` with Cobra command structure
2. Implement `capture` subcommand (wraps existing Manager)
3. Add registry persistence to config
4. Implement `list` and `show` subcommands

### Phase 2: Incremental & Diff
5. Add incremental update support to Manager
6. Implement `update` subcommand
7. Implement `diff` subcommand
8. Add `remove` subcommand

### Phase 3: Workflow Integration
9. Add `use` field to CodebaseIndexConfig
10. Implement registry lookup in processor
11. Add freshness checking
12. Add aggregate mode

### File Changes

**New Files:**
- `cmd/index.go` - Main index command and subcommands
- `utils/registry/registry.go` - Index registry management
- `utils/registry/registry_test.go` - Tests

**Modified Files:**
- `utils/config/env.go` - Add `Indexes` field to config
- `utils/processor/types.go` - Add `Use` field to CodebaseIndexConfig
- `utils/processor/codebase_index_handler.go` - Support registry lookup
- `utils/codebaseindex/manager.go` - Add incremental update support
- `utils/codebaseindex/types.go` - Add ChangeSet handling

## Open Questions

1. **Conflict handling**: What if two repos have the same auto-derived name?
   - Proposed: Error and require explicit `--name`

2. **Stale index behavior**: Warn, error, or auto-update?
   - Proposed: Warn by default, configurable per-workflow

3. **Global vs local storage**: Should `.comanda/` in repo be the default?
   - Proposed: Yes, keeps index with code (version-controllable)

4. **Index format in registry**: Store format or auto-detect from file?
   - Proposed: Store in registry for quick access

### Phase 4: Documentation & Examples
13. Update README.md with `comanda index` section
14. Add example workflows using registry indexes
15. Update website documentation

## Success Criteria

- [ ] `comanda index capture` works and registers index
- [ ] `comanda index list` shows all registered indexes
- [ ] `comanda index show <name>` displays index content
- [ ] `comanda index update` performs incremental update
- [ ] `comanda index diff` shows changes since last index
- [ ] Workflows can use `codebase_index.use: [name]` syntax
- [ ] Multi-index workflows aggregate context correctly
- [ ] README.md updated with index command documentation
- [ ] Example workflows added to `examples/` directory
- [ ] Website documentation updated (comanda.dev)
