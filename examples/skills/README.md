# Skills Examples

This directory contains example workflows that use Comanda's skills system.

## What are Skills?

Skills are reusable workflow fragments defined in markdown files with YAML frontmatter.
They are compatible with Claude Code's skill format, enabling skill sharing between tools.

## Skill Directories

Skills are loaded from these locations (in priority order):

1. `~/.comanda/skills/` — User-level skills (highest priority)
2. `.comanda/skills/` — Project-level skills
3. `<install-dir>/skills/` — Bundled skills (lowest priority)

## CLI Commands

```bash
# List all available skills
comanda skills list

# Show details of a specific skill
comanda skills show summarize

# Run a skill directly (renders the prompt with substitutions)
comanda skills run summarize "Some text to summarize"
comanda skills run summarize --arg length=short "Some text"
```

## Workflow Integration

Use `skill:` as a step type in your workflows:

```yaml
my-step:
  skill: summarize
  input: document.txt
  args:
    length: detailed
  output: summary.txt
```

## Files

- `skill-workflow.yaml` — Example workflow using the bundled `summarize` skill
