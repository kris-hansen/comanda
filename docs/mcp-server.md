# Comanda MCP Server

## Overview

`comanda mcp` starts a [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server that exposes comanda to MCP-native agents such as Claude Code, Kimi Code, Codex, and Cursor:

- **Workflows become MCP tools** — each discovered `.yaml`/`.yml` workflow file becomes one callable tool.
- **Skills become MCP prompts** — each comanda skill becomes an argumented prompt template.

The server speaks MCP over stdio by default (how MCP clients launch local servers) or over the streamable HTTP transport with `--http`.

## Starting the Server

```bash
# Serve workflows from the default directories over stdio
comanda mcp

# Serve specific workflow files
comanda mcp --workflow summarize.yaml --workflow review.yaml

# Add a directory of workflows
comanda mcp --dir ./workflows

# Disable skill prompts
comanda mcp --no-skills

# Serve over HTTP on localhost port 8080
comanda mcp --http :8080
```

| Flag | Description | Default |
|------|-------------|---------|
| `--dir` | Directory to scan for workflow `.yaml`/`.yml` files (repeatable) | — |
| `--workflow` | Individual workflow file to expose (repeatable) | — |
| `--http` | Serve the streamable HTTP transport on this address instead of stdio | stdio |
| `--no-skills` | Do not expose skills as MCP prompts | skills on |
| `--name` | Server name reported to MCP clients | `comanda` |

## Workflow Discovery

Workflow files are discovered from:

1. `~/.comanda/workflows/` (user level), when it exists
2. `.comanda/workflows/` (project level), when it exists
3. Any directories added with `--dir`
4. Any files added with `--workflow`

If no workflows and no skills are discovered, the server exits with a clear error.

Each discovered file becomes one tool:

- **Name**: the sanitized file base — `code-review.yaml` becomes `code_review`. Names must match `^[a-zA-Z0-9_-]{1,64}$`. Two files that sanitize to the same name produce a startup error listing both paths.
- **Description**: the first `#` comment line in the file, or `Run comanda workflow '<base>'` as a fallback.

## Tools: Calling Workflows

A tool's input schema is built from the workflow's `{{ var }}` placeholders:

- Each placeholder becomes an optional string argument. Processor-reserved names (`loop.*`, `current_chunk`, `chunk_index`, `total_chunks`, `file_index`, `total_files`) are never exposed.
- A reserved optional `input` string is always available; it is fed to the workflow as STDIN-style input (equivalent to `echo "..." | comanda process workflow.yaml`).

Example: a workflow containing `Summarize {{ filename }} in {{ style }} style` produces a tool with the optional string arguments `filename`, `style`, and `input`.

The tool result is the workflow's final output (`LastOutput`), returned as text content. Workflow failures are reported as tool errors, so the MCP session stays alive.

## Prompts: Using Skills

Each skill (from `~/.comanda/skills/`, `.comanda/skills/`, or the bundled set) becomes an MCP prompt:

- **Name** and **description** come from the skill's frontmatter.
- **Arguments** come from the skill's `arguments` list; the skill's `argument-hint` is used as the argument description.
- Getting the prompt renders the skill body with the supplied arguments (no LLM is called) and returns it as a single user message.

## Client Setup

### Claude Code

```bash
claude mcp add comanda -- comanda mcp
```

Or with specific workflows:

```bash
claude mcp add comanda -- comanda mcp --dir ~/workflows
```

### Kimi Code

Add an entry to `~/.kimi-code/mcp.json` (user level) or `.kimi-code/mcp.json` (project level):

```json
{
  "mcpServers": {
    "comanda": {
      "command": "comanda",
      "args": ["mcp"]
    }
  }
}
```

### Codex

```bash
codex mcp add comanda -- comanda mcp
```

Or add to `~/.codex/config.toml` directly:

```toml
[mcp_servers.comanda]
command = "comanda"
args = ["mcp"]
```

## HTTP Mode

`comanda mcp --http :8080` serves the MCP streamable HTTP transport. There is no authentication — bind to localhost and treat it as trusted-local only. Point HTTP-capable clients at `http://localhost:8080/`.

## Security Notes

- In stdio mode, stdout carries MCP protocol frames only; all logging and workflow console output is redirected to stderr.
- Workflows run with the permissions of the user launching the server, including any `tool:` shell steps the workflow defines. Only expose workflows from trusted sources, and prefer explicit `tool.allowlist` entries in workflows.
- Project-level workflow directories (`.comanda/workflows/`) are picked up from the server's working directory. Review them before connecting a client.
