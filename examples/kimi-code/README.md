# Kimi Code Provider Examples

These examples demonstrate using the Kimi Code CLI as a provider in comanda workflows.
Instead of calling an API directly, these workflows leverage the locally installed
`kimi` CLI binary for agentic programming tasks.

## Prerequisites

- Kimi Code CLI installed: `npm install -g @moonshot-ai/kimi-code`
  (or `curl -L code.kimi.com/install.sh | bash`)
- Authenticated once, outside comanda: `kimi login` (device-code OAuth, works
  headless) or an API key in `~/.kimi-code/config.toml`
- Run `kimi --version` to verify installation. comanda stores no credentials —
  the CLI handles auth itself.

## Available Models

| Model | Description |
|-------|-------------|
| `kimi-code` | Default model from your `~/.kimi-code/config.toml` (`default_model`) |
| `kimi-code-<alias>` | Any model alias defined in your `~/.kimi-code/config.toml` (aliases are user-defined; passed through to `kimi --model`) |

## Examples

### Basic Code Review

```yaml
review-code:
  input: "src/main.go"
  model: kimi-code
  action: "Review this code for bugs, security issues, and suggest improvements"
  output: STDOUT
```

### Multi-file Analysis

```yaml
analyze-project:
  input: "*.go"
  model: kimi-code
  action: "Analyze these Go files and provide a summary of the project architecture"
  output: "analysis.md"
```

## Billing Notes

- Usage through the Kimi Code CLI draws on **Kimi membership Kimi Code benefits**
  (weekly credit cycle) — an officially supported path.
- The separate **Kimi Open Platform** (`api.moonshot.ai`) is pay-as-you-go with
  independent accounts and keys; that path is comanda's `moonshot` provider,
  not this one.

## Agentic Mode

`kimi-code` supports comanda's agentic loops (see `agentic-loop.yaml`):
`allowed_paths` map to `--add-dir` workspace directories and the loop's working
directory becomes the session cwd. Note that kimi's `-p` mode auto-approves
routine actions (writes, shell commands) and has no per-tool allowlist flag, so
the loop `tools:` list is advisory only — scope access with `allowed_paths`.

## Configuration

No special configuration needed in `.env`. The provider automatically detects
if the `kimi` binary is available in your PATH (or common install locations
such as `~/.kimi-code/bin/kimi` and `~/.local/bin/kimi`).
