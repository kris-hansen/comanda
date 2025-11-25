# Tool Use Examples

This directory contains examples demonstrating the tool use feature in Comanda workflows.

## Security Warning

Tool use allows execution of shell commands within workflows. This is a powerful feature that comes with significant security implications:

- Commands run with your user permissions
- Malicious workflows could attempt data exfiltration
- Even "safe" commands can be dangerous with certain arguments

**Only run workflows from trusted sources.**

## How It Works

### Tool Input

Use shell commands to generate input for workflow steps:

```yaml
step_name:
  input: "tool: ls -la"          # Run command, use stdout as input
  # or
  input: "tool: STDIN|grep foo"  # Pipe previous output through command
```

### Tool Output

Pipe workflow output through shell commands:

```yaml
step_name:
  output: "tool: jq '.data'"     # Pipe output through jq
  # or
  output: "STDOUT|grep pattern"  # Explicit STDOUT prefix
```

### Allowlist/Denylist

Control which commands can be executed per-step:

```yaml
step_name:
  input: "tool: bd list"
  tool:
    allowlist:
      - bd
      - grep
      - jq
    denylist:
      - rm
      - sudo
    timeout: 60  # seconds
```

## Default Security

By default, Comanda:
- **Denies** dangerous commands: rm, sudo, chmod, bash, curl, wget, etc.
- **Allows** safe read-only commands: ls, cat, grep, jq, date, etc.

See `utils/processor/tool_executor.go` for the complete default lists.

## Examples

- `beads-workflow-example.yaml` - Using the beads (bd) tool
- `tool-input-example.yaml` - Various tool input patterns
- `tool-output-example.yaml` - Various tool output patterns

## Custom Tools

To use custom tools not in the default allowlist, explicitly add them:

```yaml
my_step:
  input: "tool: my-custom-tool --arg"
  tool:
    allowlist:
      - my-custom-tool
```
