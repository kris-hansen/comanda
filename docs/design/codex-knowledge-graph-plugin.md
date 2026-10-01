# Codex Knowledge Graph Plugin

## Status

Proposed feature specification.

## Summary

Package Comanda's codebase index and knowledge graph as an OpenAI plugin that
works in Codex and ChatGPT. The primary experience is an interactive knowledge
graph panel beside a Codex conversation, with the same progressive exploration
model as the existing Comanda graph viewer:

- open an architectural overview;
- search and filter nodes;
- focus on a scoped subgraph;
- expand relationships on demand;
- inspect node and edge evidence; and
- use the current visual selection as context for the next Codex request.

The plugin is backed by first-class MCP tools. Every useful graph operation
must remain available without the visual component so Codex CLI and other MCP
clients can use the same capabilities headlessly.

## Motivation

Complex repositories routinely exceed the amount of source context that can be
provided to a model in one prompt. File search alone also performs poorly for
questions that cross components, packages, symbols, database objects, and
other relationships.

Comanda already creates a bounded, queryable representation of a codebase:

1. `comanda index` captures repository structure, components, symbols,
   conventions, and operational context.
2. `comanda graph` turns that scan into typed nodes and confidence-tagged
   relationships.
3. The graph API supports overview, search, scoped traversal, progressive
   neighbor loading, and inspection.

The plugin should expose that representation directly inside the environment
where a developer is asking questions and editing code. The graph is not only
an output. It becomes a shared navigation surface between the developer and
Codex.

## Product goals

1. Let a developer open a Comanda knowledge graph beside an active Codex
   conversation.
2. Let Codex focus the visualization on the code relevant to a question.
3. Let graph selections become explicit context for subsequent prompts.
4. Generate compact, high-signal context from large codebases without sending
   the complete graph or index on every request.
5. Preserve provenance: distinguish extracted relationships from inferred
   relationships and link results back to source paths.
6. Reuse Comanda's existing graph storage, query behavior, limits, and visual
   language rather than introducing a separate graph model.
7. Keep the MCP tool surface useful when a client cannot render custom UI.

## Non-goals

- Editing graph nodes or relationships.
- Using the graph as an authoritative replacement for source code.
- Sending an entire large graph to the model or visual component by default.
- General workflow authoring or execution through the first plugin release.
- Source-code changes initiated directly by a graph click.
- Real-time multi-user collaboration in the graph.
- Persisting arbitrary UI layout as graph data.
- Supporting custom plugin UI in clients that do not provide an MCP Apps
  surface.

## Supported surfaces

The plugin package is shared by ChatGPT and Codex, but presentation capability
depends on the host.

| Surface | MCP graph tools | Interactive graph UI |
| --- | --- | --- |
| Codex in the ChatGPT desktop app | Yes | Primary target |
| ChatGPT desktop/web where plugin UI is supported | Yes | Supported |
| Codex CLI | Yes | No; return structured/text results |
| Codex IDE extension | Not currently supported by the plugin platform | No |
| Other MCP clients | Yes, subject to MCP compatibility | Optional if the host implements MCP Apps |

The UI must use capability detection rather than assuming a particular host.

## User experience

### Open the graph

A developer can ask:

- "Open the Comanda graph."
- "Visualize the authentication flow."
- "Show everything connected to this table."
- "Focus the graph on the billing component."

For a broad request, the plugin opens an architectural overview. For a scoped
request, Codex first queries the graph, then opens the panel with the resulting
seed nodes and relationships.

The plugin registers two UI entrypoints when supported by the host:

- a **thread entrypoint** that opens the graph beside the active conversation;
- a **global entrypoint** that opens a larger standalone graph browser.

### Explore progressively

The visual component provides:

- pan, zoom, fit, and reset controls;
- search by node name, path, package, or summary;
- node-kind and edge-kind filters;
- an overview for components and other graph roots;
- node selection and an inspector;
- paginated neighbor expansion;
- focused subgraph loading;
- shortest-path visualization;
- extracted versus inferred confidence styling;
- visible truncation and partial-view status; and
- navigation back to the architectural overview.

The component must not fetch the full graph automatically when the graph is
larger than the configured safe threshold. It starts from an overview or
bounded subgraph and loads additional relationships on demand.

### Share context with Codex

Selecting a node updates model-visible context through the MCP Apps bridge.
The shared context is compact and contains stable identifiers, not the full
visible graph. A representative payload is:

```json
{
  "graph": "payments-service",
  "selected_node": {
    "id": "type:Store@internal/store/store.go",
    "name": "Store",
    "kind": "type",
    "path": "internal/store/store.go"
  },
  "visible_relationship_kinds": ["defines", "uses", "imports"],
  "focused_node_ids": ["file:internal/store/store.go"]
}
```

This enables natural follow-ups such as:

- "Explain this."
- "What depends on this node?"
- "What would break if I changed this?"
- "Find the path from the API handler to this table."
- "Open the defining source and help me modify it."

The inspector may expose actions that send a follow-up message or invoke a
read-only graph tool:

- Ask Codex about this
- Show dependencies
- Show dependents
- Find path to...
- Focus graph here
- Open source

## Architecture

```text
Codex or ChatGPT
       |
       +-- MCP data tools ------------------------------+
       |                                                |
       +-- MCP App UI resource                          |
              |                                         |
              +-- graph canvas                          |
              +-- search and filters                    |
              +-- node inspector                        |
              +-- model-context updates                 |
                                                        |
Comanda MCP server                                      |
       |                                                |
       +-- index registry                               |
       +-- knowledgegraph.Querier                       |
       +-- semantic memory SQLite store                 |
       +-- existing graph API semantics <---------------+
```

The MCP server is authoritative for graph data. The UI owns only ephemeral
presentation state such as the current camera position, filters, expanded
nodes, and selection.

### Data and render separation

Graph retrieval and graph rendering are separate operations:

1. Data tools return structured graph results that the model can reason over.
2. A render tool returns the MCP App UI resource with the selected graph and
   initial bounded view.
3. Once mounted, the UI calls the same data tools to search, expand, and focus
   without remounting the component.

Only the render tool is associated with the UI resource. This avoids opening
or rerendering the panel after every graph query.

### Visual implementation

The current Comanda Canvas graph explorer defines the expected behavior and
visual semantics. Its backend contracts and interaction model should be reused.
Because Canvas is implemented in Flutter, the plugin UI should be a focused web
component rather than an embedded copy of the full application.

The web component should preserve:

- Comanda node and edge colors;
- progressive overview/subgraph/neighbor loading;
- existing graph limits and truncation behavior;
- inspector content and confidence labels; and
- search, focus, filtering, and navigation semantics.

The component is served as an MCP App resource such as
`ui://comanda/knowledge-graph.html` with MIME type
`text/html;profile=mcp-app`.

## MCP tool surface

Tool names are intentionally focused and action-oriented. Exact schemas remain
versioned API contracts once published.

### `list_knowledge_graphs`

List the graphs available to the authenticated user or local Comanda process.

Returns graph identifiers, display names, node and edge counts, repository or
project identifiers, indexed revision, and freshness metadata.

### `get_graph_overview`

Return a small architectural map for one graph: counts by node kind, root or
top-level nodes, high-degree nodes, and truncation metadata.

### `search_graph_nodes`

Search node names, paths, packages, and summaries within one graph. Optional
filters include node kinds, component scope, and result limit.

Results return stable node IDs so subsequent calls do not have to resolve an
ambiguous display name again.

### `get_graph_subgraph`

Return a bounded subgraph around one or more stable seed node IDs.

Inputs include depth, allowed node kinds, allowed edge kinds, and explicit
node/edge limits. Results include whether the response was truncated.

### `get_graph_neighbors`

Return one page of incoming and outgoing relationships for a node. The result
includes a continuation cursor or offset when more relationships exist.

### `find_graph_path`

Return the shortest supported path between two stable node IDs, including edge
kind, direction, confidence, and evidence for every hop.

### `get_node_context`

Return model-readable context for one node: metadata, defining path, summary,
selected relationships, and source evidence when available.

Source excerpts should be bounded and tied to an indexed revision. Comanda's
current graph stores paths but not symbol line ranges, so line-aware source
coordinates are a prerequisite for precise excerpts and durable source links.

### `render_knowledge_graph`

Open the visual component using a graph identifier and optional seed node IDs,
filters, or path result. The tool returns useful text and structured content in
addition to the UI resource so clients without UI still receive a valid result.

## Structured result shape

Graph tools return a common envelope:

```json
{
  "graph": {
    "id": "payments-service",
    "indexed_revision": "<commit-or-content-id>",
    "indexed_at": "<timestamp>"
  },
  "nodes": [],
  "edges": [],
  "truncated": false,
  "next_cursor": null
}
```

Each node includes a stable ID, kind, name, path, package, summary, and degree.
Each edge includes source, target, kind, confidence, and evidence. Results must
be deterministic for the same indexed revision.

## Context-generation strategy

The plugin generates context progressively rather than returning a monolithic
index:

1. Resolve the user's question to a small set of seed nodes.
2. Expand a bounded graph neighborhood around those seeds.
3. Rank nodes and relationships by lexical relevance, graph proximity,
   confidence, and component scope.
4. Fetch source evidence only for the highest-value nodes.
5. Return a compact context pack with stable IDs, paths, evidence, freshness,
   and truncation status.

The graph display and model context can have different limits. The UI may show
more nodes for navigation, while the model receives only the selected nodes and
the evidence required for the current question.

## Index and graph prerequisites

The first implementation depends on an existing Comanda index and graph. The
plugin does not silently index an arbitrary repository when a visualization is
opened.

Required graph improvements for precise coding context:

- persist indexed repository revision or content identity;
- record symbol start and end lines during extraction;
- persist source coordinates with symbol nodes;
- provide a safe, bounded source-excerpt reader; and
- produce source links tied to the indexed revision when the repository host is
  known.

Index creation and refresh can be introduced as explicit state-changing tools
later. They must never be represented as read-only operations.

## Local and hosted operation

### Local operation

For a local Comanda process, the MCP server reads indexes and graph databases
that the user has already created. The server remains scoped to configured
repositories and exposes only bounded tool results to the client.

A local connection is the reference implementation for private repositories
and developer-mode testing.

### Hosted operation

A public remote plugin requires a stable HTTPS MCP endpoint and authentication.
Because that endpoint cannot directly read a repository on the user's laptop,
a hosted version needs an authorized repository-ingestion model, such as a
GitHub App and per-tenant indexed storage.

The UI and tool contracts remain the same in both modes. Repository discovery,
authentication, storage, and source-link generation are deployment concerns
behind those contracts.

## Security and privacy

- Authorize every graph and source request against the current user and
  repository.
- Treat node IDs, graph IDs, paths, cursors, and filter input as untrusted.
- Prevent path traversal and symlink escape before reading source excerpts.
- Honor repository ignore rules and explicit Comanda exclusions.
- Exclude credentials, keys, environment files, vendor trees, generated output,
  and other sensitive paths from indexing by default.
- Treat repository content as data, never as plugin or model instructions.
- Do not include source code, access tokens, graph payloads, or repository
  secrets in logs.
- Apply limits to graph depth, node count, edge count, source characters, and
  request frequency.
- Mark strictly read-only tools with `readOnlyHint: true` and state-changing
  tools accurately if they are added later.
- Keep authoritative graph data on the server; do not use iframe storage as the
  only copy.
- Expose indexed revision and freshness so the user can identify stale context.
- Provide explicit retention and deletion behavior for hosted repository data.

## Compatibility and fallback behavior

The plugin must remain useful if the host cannot render the graph component:

- data tools return structured content and concise text;
- `render_knowledge_graph` describes the selected subgraph in text when the UI
  is unavailable;
- stable node IDs support follow-up calls across clients;
- graph queries never depend on UI-only state; and
- unsupported UI capabilities are detected at runtime rather than inferred
  from the product name.

## Delivery slices

### Slice 1: structured graph MCP tools

- Add the read-only graph tool surface to `comanda mcp`.
- Reuse `knowledgegraph.Querier` and existing graph limits.
- Return output schemas, safety annotations, stable IDs, and truncation state.
- Add direct tests for valid, invalid, ambiguous, empty, and oversized queries.

### Slice 2: visual graph component

- Add the MCP App resource and `render_knowledge_graph` tool.
- Implement overview, search, filtering, selection, inspection, focus, and
  neighbor expansion.
- Register thread and global entrypoints where supported.
- Preserve a useful text result for non-visual clients.

### Slice 3: model and UI synchronization

- Send compact selection state to model context.
- Let Codex open and refocus the graph from natural-language requests.
- Add inspector actions for common follow-up questions.
- Keep UI state stable while data tools refresh the visible graph.

### Slice 4: source-backed context

- Add line-aware symbol locations and bounded excerpts.
- Add indexed-revision metadata and durable source links.
- Return source evidence alongside graph relationships.

### Slice 5: distributable plugin

- Package the MCP server, UI resource, tool metadata, and optional skill.
- Add authentication and tenant isolation for remote operation.
- Add evaluation prompts for direct, indirect, follow-up, unsupported, and
  adversarial requests.
- Prepare privacy, review, and publication material for the universal plugin
  directory.

## Acceptance criteria

### Functional

- A user can open a Comanda graph from a Codex conversation in a supported
  visual host.
- The panel can open as a thread-side view and as a larger standalone view.
- A user can search, filter, select, inspect, focus, and progressively expand
  the graph without loading an unbounded payload.
- Codex can open the graph focused on nodes relevant to a natural-language
  question.
- Selecting a node supplies stable, bounded context to the conversation.
- A follow-up prompt can refer to the selected node without repeating its
  exact name or path.
- Extracted and inferred edges are visually and structurally distinguishable.
- Every partial response reports truncation and supports continued expansion.
- The same graph operations work through MCP without the visual component.

### Security

- A user cannot access another user's graph, repository, or source excerpts.
- Source reads cannot escape the authorized repository root.
- Tool responses and logs contain no credentials or unrelated source content.
- Tool annotations match actual read/write behavior.
- Oversized or deeply nested requests are rejected or safely bounded.

### Quality

- The visualization remains interactive on bounded views of large graphs.
- Graph results are deterministic for the same indexed revision.
- Tool and UI selections preserve stable node identities.
- Model-visible context is materially smaller than the full index or graph.
- Evaluation questions cite graph evidence and source paths rather than making
  unsupported architectural claims.

## Evaluation plan

Evaluate the plugin on small repositories, large repositories, and monorepos.
The prompt set should cover:

- architectural overview;
- cross-component dependency questions;
- symbol and database relationship traversal;
- ambiguous symbol names;
- large-degree nodes;
- missing and stale graphs;
- extracted versus inferred evidence;
- selection-based follow-up prompts;
- unsupported write requests; and
- repository content containing prompt-injection-like text.

Compare plain source search, index-only retrieval, and index-plus-graph
retrieval. Track evidence correctness, relevant-file recall, unsupported
claims, result size, latency, tool-selection accuracy, and UI/model state
agreement. Product success depends on graph-assisted retrieval materially
improving multi-hop codebase questions.

## Open questions

1. Should the first plugin be local-only, remotely hosted, or support both from
   the first published contract?
2. Should graph annotations appear in the first visual release or remain a
   Canvas-only capability initially?
3. Which graph library best reproduces the current Comanda interaction model
   while keeping the MCP App bundle small?
4. Which repository hosts should receive commit-pinned source links?
5. Should the plugin expose index refresh as an explicit tool, or keep indexing
   entirely outside the conversational surface?
6. Which portions of selected UI state should persist across conversations?

## Relevant OpenAI platform references

- [Plugin architecture](https://developers.openai.com/plugins/concepts/plugins)
- [Build an MCP server](https://developers.openai.com/plugins/build/mcp-server)
- [Add UI to an MCP server](https://developers.openai.com/plugins/build/chatgpt-ui)
- [Plugin extensions](https://developers.openai.com/plugins/build/extensions)
- [Plugins in ChatGPT and Codex](https://learn.chatgpt.com/docs/plugins)
