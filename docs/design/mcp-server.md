# Comanda MCP Server - Design Document

**Author:** Claude
**Date:** 2026-07-18
**Status:** Implemented (v1)

## Overview

Add a top-level `comanda mcp` command that runs a [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server. The server exposes comanda **workflows as MCP tools** and **skills as MCP prompts**, so MCP-native agents (Claude Code, Kimi Code, Codex, Cursor) can trigger comanda pipelines as first-class tools instead of shelling out to `comanda process`.

## Motivation

Today, an agent that wants to run a comanda workflow must invoke the CLI (`comanda process workflow.yaml`), parse human-oriented console output, and manage input passing itself. MCP is the standard tool/prompt interface for agent clients. Serving comanda over MCP provides:

- Discoverability: clients list tools/prompts and see names, descriptions, and input schemas
- Structured invocation: typed arguments instead of `--vars` string munging
- Clean output capture: the workflow's final output is returned as tool content
- Skill reuse: comanda skills map naturally onto MCP prompt templates

## Mapping Model

### Workflows → Tools

Each discovered `.yaml`/`.yml` workflow file becomes one MCP tool.

| Aspect | Rule |
|--------|------|
| Name | Sanitized file base: `code-review.yaml` → `code_review`; must match `^[a-zA-Z0-9_-]{1,64}$` |
| Description | First `#` comment line in the file, else `Run comanda workflow '<base>'` |
| Input schema | One optional string property per `{{ var }}` placeholder, plus a reserved optional `input` string |
| Result | The workflow's final output (`proc.LastOutput()`) as text content |
| Errors | Workflow failures → tool-error results (`IsError: true`), never JSON-RPC failures |

Placeholder extraction reuses the CLI variable substitution pattern from `utils/processor/dsl.go` (`\{\{\s*([^}]+?)\s*\}\}`). Processor-reserved names are filtered out: `loop.*`, `current_chunk`, `chunk_index`, `total_chunks`, `file_index`, `total_files`.

Execution reuses the programmatic path shared with the HTTP server handlers: read file → `yaml.Unmarshal` into `processor.DSLConfig` → `processor.NewProcessor(&dslConfig, envConfig, &config.ServerConfig{Enabled: false}, verbose, runtimeDir, cliVars)` → optional `proc.SetLastOutput(input)` → `proc.Process()` → `proc.LastOutput()`. Tool arguments other than the reserved `input` are passed as CLI variables; `input` is fed via `SetLastOutput` (STDIN-style). Duplicate tool names across directories are a startup error listing both paths.

### Skills → Prompts

MCP prompts are argumented prompt templates, which is exactly what comanda skills are: `skills.Prepare()` renders the body and never calls an LLM.

| Aspect | Rule |
|--------|------|
| Name | Skill display name (frontmatter `name` or file base) |
| Description | Frontmatter `description`, else `Comanda skill '<name>'` |
| Arguments | Frontmatter `arguments` list; `argument-hint` becomes each argument's description |
| Handler | Returns a single user message containing the rendered body |

Skills are loaded through the existing `skills.Index` (priority: `~/.comanda/skills/` → `.comanda/skills/` → bundled). `--no-skills` disables prompt registration.

## Command Structure

```
comanda mcp [flags]

Flags:
  --dir strings       Directory to scan for workflows (repeatable; adds to defaults)
  --workflow strings  Individual workflow file to expose (repeatable)
  --http string       Serve streamable HTTP on this address (e.g. :8080) instead of stdio
  --no-skills         Do not expose skills as MCP prompts
  --name string       Server name reported to clients (default "comanda")
```

Discovery defaults: `~/.comanda/workflows/` and `.comanda/workflows/` when they exist. The server exits with a clear error when nothing (neither workflows nor skills) is discovered.

## Transports

- **stdio** (default): `mcp.StdioTransport` from the official SDK. This is how MCP clients launch local servers as child processes.
- **HTTP** (`--http :port`): the SDK's streamable HTTP handler (`mcp.NewStreamableHTTPHandler`). No authentication — documented as localhost-trusted only.

The server is built on the official SDK `github.com/modelcontextprotocol/go-sdk`, pinned to v1.6.1. Dynamic registration uses `Server.AddTool`/`Server.AddPrompt` with explicit schemas (`map[string]any` input schemas), which bypasses the generic `AddTool[In, Out]` path so argument unmarshaling stays under our control (`json.RawMessage` → `map[string]string`).

## Stdio Hygiene

In stdio mode, stdout carries MCP protocol frames exclusively:

1. `cmd/mcp.go` sets `log.SetOutput(os.Stderr)` at command start, covering all `log.Printf` output (per `.rules/logging.md`, everything goes through `log`).
2. The spinner and progress display are disabled on the processor.
3. During each `proc.Process()` call, `utils/mcp/execute.go` reassigns the `os.Stdout` variable to an `os.Pipe` whose read end is copied to `os.Stderr`, restoring it afterward. Go's `fmt.Print*` reads `os.Stdout` dynamically, so stray workflow output is redirected. The SDK captured the real stdout when the transport connected, so protocol frames are unaffected. A mutex serializes the redirection across concurrent tool calls (HTTP mode).

## Implementation Layout

```
cmd/mcp.go              Command, flags, discovery wiring, log redirection
utils/mcp/workflows.go  Discovery, name sanitization, var/description extraction
utils/mcp/execute.go    Runner (workflow execution adapter), stdout guard
utils/mcp/server.go     Server construction, tool registration, transports
utils/mcp/prompts.go    Skill → prompt registration
```

## Testing

- `workflows_test.go`: sanitization, var extraction (incl. reserved-name filtering), description extraction, directory/file discovery, duplicate-name startup error
- `execute_test.go`: execution adapter with `model: NA` workflows (no provider keys needed), variable substitution, error paths
- `server_test.go`: smoke tests over the SDK's in-memory transport — initialize, `tools/list`, `tools/call` (success and tool-error paths), `prompts/list`, `prompts/get`
- Manual: initialize handshake + `tools/call` over real stdio, verifying stdout contains only JSON-RPC frames; HTTP initialize over the streamable transport

## Future Work

- `server.mcp` config block and `comanda server mcp on|off` subcommands, mirroring `openai_compat`
- MCP resources (e.g. codebase indexes exposed as resources)
- Workflow `capture_outputs` support for richer tool results
- Authentication for HTTP mode (bearer token), enabling non-localhost binding

## Non-Goals (v1)

- Config-file persistence (`server.mcp` block)
- MCP resources, elicitation, and sampling
- Remote authentication for `--http` mode
