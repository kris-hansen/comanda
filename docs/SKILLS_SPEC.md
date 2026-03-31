# Comanda Skills Specification

**Status:** Draft  
**Author:** Claudette  
**Date:** 2026-03-31

## Overview

This specification defines a skills system for Comanda that is compatible with Claude Code's skill format. Skills are reusable workflow fragments defined in markdown files with YAML frontmatter, allowing users to create shareable, discoverable workflow components.

## Goals

1. **Claude Code Compatibility** — Skills use the same markdown + YAML frontmatter format as Claude Code, enabling skill sharing between tools
2. **Composability** — Skills can be invoked as steps in Comanda workflows
3. **Discoverability** — Skills are loaded from standard directories and can be listed/searched
4. **Extensibility** — Skills support arguments, model overrides, and execution options

## Directory Structure

Skills are loaded from these locations (in priority order):

```
~/.comanda/skills/           # User-level skills (highest priority)
.comanda/skills/             # Project-level skills
<comanda-install>/skills/    # Bundled skills (lowest priority)
```

Each skill is a markdown file (`*.md`) with YAML frontmatter.

## Skill File Format

### Basic Example

```markdown
---
name: code-review
description: Review code for bugs, security issues, and best practices
when_to_use: When the user asks to review code or wants feedback on implementation
model: claude-sonnet-4
allowed-tools:
  - FileRead
  - Grep
  - Glob
---

# Code Review

You are an expert code reviewer. Analyze the provided code for:

1. **Bugs** — Logic errors, edge cases, potential crashes
2. **Security** — Injection vulnerabilities, auth issues, data exposure
3. **Performance** — Inefficiencies, unnecessary allocations
4. **Style** — Consistency, naming, documentation

Be specific. Reference line numbers. Suggest fixes.

${USER_INPUT}
```

### Frontmatter Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | No | Display name (defaults to filename without extension) |
| `description` | string | Yes | Short description for listing/search |
| `when_to_use` | string | No | Guidance for when to invoke this skill |
| `model` | string | No | Model override (e.g., `claude-sonnet-4`, `gpt-4o`, `gemini-2.5-pro`) |
| `allowed-tools` | string[] | No | Tools this skill can use (Claude Code compatibility) |
| `arguments` | string[] | No | Named arguments that can be substituted |
| `argument-hint` | string | No | Usage hint shown in help |
| `version` | string | No | Semantic version for the skill |
| `effort` | string | No | Effort level: `low`, `medium`, `high`, `max` |
| `context` | string | No | Execution context: `inline` (default) or `fork` |
| `paths` | string[] | No | Glob patterns for files this skill applies to |
| `user-invocable` | bool | No | Whether users can invoke directly (default: true) |

### Variable Substitution

Skills support these variables in the markdown body:

| Variable | Description |
|----------|-------------|
| `${USER_INPUT}` | The user's input/prompt when invoking the skill |
| `${ARG_NAME}` | Named argument (from `arguments` frontmatter) |
| `${COMANDA_SKILL_DIR}` | Absolute path to the skill's directory |
| `${TIMESTAMP}` | Current ISO timestamp |
| `${STDIN}` | Content from standard input (if piped) |

## CLI Commands

### List Skills

```bash
comanda skills list [--format json|table]
```

Shows all available skills with source, description, and model.

### Show Skill Details

```bash
comanda skills show <name>
```

Displays full skill content with frontmatter parsed.

### Invoke Skill Directly

```bash
comanda skills run <name> [--arg key=value ...] [input]
```

Runs a skill as a standalone operation.

### Create Skill from Workflow

```bash
comanda skills create <name> --from workflow.yaml
```

Extracts a step from a workflow into a reusable skill.

## Workflow Integration

Skills can be invoked in workflows using the `skill:` key:

```yaml
review-step:
  skill: code-review
  input: $SOURCE_CODE
  output: $REVIEW_RESULT
```

Or with arguments:

```yaml
translate-step:
  skill: translate
  args:
    target_language: Spanish
    tone: formal
  input: $DOCUMENT
  output: $TRANSLATED
```

### Skill Step Schema

```yaml
<step-name>:
  skill: <skill-name>          # Required: name of skill to invoke
  input: <input>               # Optional: input to pass (default: none)
  args:                        # Optional: named arguments
    <key>: <value>
  model: <model>               # Optional: override skill's model
  output: <variable>           # Optional: where to store result
```

## Implementation Details

### Skill Loading

1. On startup (or `skills list`), scan all skill directories
2. Parse frontmatter from each `.md` file
3. Build index: `name → {path, frontmatter, source}`
4. Cache for quick lookup

### Skill Execution

1. Load full skill content from file
2. Substitute variables (`${...}`)
3. Determine model (skill → workflow → config fallback)
4. Create temporary single-step workflow
5. Execute via existing processor

### Claude Code Compatibility Notes

- **`allowed-tools`**: Comanda doesn't have the same tool system, but we preserve this field for portability. Future: map to workflow capabilities.
- **`context: fork`**: Maps to `isolation: true` in Comanda (separate worktree/branch)
- **`paths`**: Can be used with `comanda process --skill-auto` to auto-select skills based on input files
- **`effort`**: Maps to thinking/iteration budget

## File Structure Changes

```
comanda/
├── cmd/
│   ├── skills.go          # NEW: skills subcommand
│   └── ...
├── utils/
│   ├── skills/
│   │   ├── loader.go      # NEW: skill file parsing
│   │   ├── executor.go    # NEW: skill execution
│   │   └── index.go       # NEW: skill discovery/caching
│   └── ...
├── skills/                 # NEW: bundled skills
│   ├── code-review.md
│   ├── summarize.md
│   └── ...
└── ...
```

## Example Skills

### summarize.md

```markdown
---
name: summarize
description: Summarize text or documents concisely
when_to_use: When asked to summarize, condense, or get key points from content
arguments:
  - length
argument-hint: "summarize [--length short|medium|detailed]"
---

# Summarize

Create a ${length:-medium} summary of the following content.

Focus on:
- Key points and main arguments
- Important facts and figures
- Actionable conclusions

${USER_INPUT}
```

### refactor.md

```markdown
---
name: refactor
description: Refactor code for clarity, performance, or maintainability
model: claude-sonnet-4
paths:
  - "**/*.go"
  - "**/*.ts"
  - "**/*.py"
---

# Code Refactoring

Refactor this code with focus on:

1. **Readability** — Clear names, small functions, good structure
2. **Performance** — Efficient algorithms, minimal allocations
3. **Maintainability** — Easy to test, extend, and debug

Show the refactored code with brief explanations for each change.

${USER_INPUT}
```

## Migration Path

1. **Phase 1**: Basic skill loading and `skills list/show` commands
2. **Phase 2**: `skill:` step type in workflows
3. **Phase 3**: `skills run` for direct invocation
4. **Phase 4**: `skills create` for workflow→skill extraction
5. **Phase 5**: Skill auto-selection based on `paths`

## Testing

- Unit tests for frontmatter parsing
- Integration tests for skill invocation in workflows
- Compatibility tests with actual Claude Code skills

## Open Questions

1. Should skills support `!` shell injection like Claude Code? (Security implications)
2. How to handle skill dependencies (one skill invoking another)?
3. Should we support remote skill repositories (like npm)?

---

## Appendix: Claude Code Frontmatter Reference

Full list of Claude Code frontmatter fields for compatibility reference:

```yaml
---
name: string                    # Display name
description: string             # Short description
when_to_use: string            # Usage guidance
model: string                  # Model override (or "inherit")
allowed-tools: string[]        # Permitted tools
arguments: string[]            # Named arguments
argument-hint: string          # Usage hint
version: string                # Semantic version
effort: string                 # low|medium|high|max or integer
context: inline|fork           # Execution context
agent: string                  # Agent type for fork context
paths: string[]                # File path patterns
user-invocable: bool           # Can users invoke directly
hooks: object                  # Lifecycle hooks
shell: bash|powershell         # Shell for ! commands
---
```
