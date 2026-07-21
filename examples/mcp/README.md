# MCP Server Example

This example demonstrates `comanda mcp`, which runs a Model Context Protocol (MCP) server that exposes comanda workflows as MCP tools and comanda skills as MCP prompts. MCP-native agents (Claude Code, Kimi Code, Codex, Cursor) can then call your workflows as first-class tools.

## Files

- `echo.yaml` — echoes its input back using `model: NA`, so it works without any configured LLM provider. Good for a first smoke test.
- `summarize.yaml` — summarizes STDIN input using a `{{ style }}` variable. Requires a configured model (edit the `model:` field to one you have set up).

## How it Works

1. Each discovered `.yaml`/`.yml` workflow file becomes one MCP tool, named after the sanitized file base (`echo.yaml` → `echo`).
2. The tool's input schema is derived from the workflow's `{{ var }}` placeholders, plus a reserved `input` argument that is fed to the workflow as STDIN-style input.
3. Calling the tool runs the workflow and returns its final output as text content.
4. Skills from `~/.comanda/skills/`, `.comanda/skills/`, and the bundled set become MCP prompts (disable with `--no-skills`).

## Running the Server

Serve these two workflows over stdio:

```bash
comanda mcp --workflow examples/mcp/echo.yaml --workflow examples/mcp/summarize.yaml
```

Or copy them into one of the default discovery directories (`~/.comanda/workflows/` or `.comanda/workflows/`) and simply run:

```bash
comanda mcp
```

## Connecting a Client

Claude Code:

```bash
claude mcp add comanda -- comanda mcp --workflow examples/mcp/echo.yaml
```

Kimi Code (`~/.kimi-code/mcp.json` or `.kimi-code/mcp.json`):

```json
{
  "mcpServers": {
    "comanda": {
      "command": "comanda",
      "args": ["mcp", "--workflow", "examples/mcp/echo.yaml"]
    }
  }
}
```

Codex:

```bash
codex mcp add comanda -- comanda mcp --workflow examples/mcp/echo.yaml
```

## Trying It Out

Once connected, ask the agent to use the `echo` tool — it returns whatever `input` you pass, no API keys required:

> Call the comanda `echo` tool with input "hello from MCP"

The agent invokes `tools/call` with `{"input": "hello from MCP"}` and gets `hello from MCP` back as the tool result.

For `summarize`, the agent can pass both arguments, e.g. `{"input": "<long text>", "style": "bullet-point"}`; `style` is substituted into the action's `{{ style }}` placeholder before the workflow runs.

## Verifying Stdio Manually

You can drive the raw protocol yourself to see the framing:

```bash
{
  printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"demo","version":"0.1"}}}'
  printf '%s\n' '{"jsonrpc":"2.0","method":"notifications/initialized"}'
  printf '%s\n' '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'
  printf '%s\n' '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo","arguments":{"input":"hello"}}}'
  sleep 1
} | comanda mcp --workflow examples/mcp/echo.yaml
```

Stdout carries only JSON-RPC frames; all logging and workflow console output goes to stderr.

See [docs/mcp-server.md](../../docs/mcp-server.md) for the full reference, including HTTP mode and security notes.
