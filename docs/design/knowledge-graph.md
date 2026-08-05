# Knowledge Graphs

Comanda can turn a codebase index into a knowledge graph and store it in the
semantic memory database, inspired by [Graphify](https://github.com/Graphify-Labs/graphify).

The pipeline:

```
comanda index capture  →  outline (components, packages, files, symbols)
comanda graph build    →  typed nodes + edges in .comanda/memory/<index>.db
comanda graph query    →  traverse instead of grep
```

## Graph model

- **Nodes** — `component`, `package`, `file`, `type`, `function`, `concept`
  (concepts come only from the optional AI pass).
- **Edges** — `contains`, `belongs_to`, `imports`, `defines` (structural),
  `uses`, `references` (semantic).
- **Confidence** — every edge is tagged `extracted` (explicit in the source:
  imports, declarations, component roots) or `inferred` (symbol-name
  resolution or the AI pass). You always know what was read vs. guessed.

## Building

```bash
# One shot: index + graph
comanda index capture -n myproject --graph

# Or later, from the registered index
comanda graph build myproject
comanda graph build myproject --enhance          # add AI-inferred concepts/edges
comanda graph update myproject                   # rebuild from a fresh scan
```

`--graph` also works on `comanda index update`. Encrypted indexes are skipped
(graph data is plain SQLite). The graph is rebuilt deterministically on each
run — nodes and edges have stable IDs, so rebuilds upsert in place and stale
nodes from deleted files are removed.

The graph lives in the project's semantic memory database,
`.comanda/memory/<index-name>.db`, in `graph_nodes` / `graph_edges` tables.
Every node is additionally mirrored as a `graph_node` memory record, so
`comanda memory search` and workflow recall see graph concepts.

## Querying

```bash
comanda graph explain Store                  # node + connections, confidence-tagged
comanda graph path main Store                # BFS shortest connection
comanda graph query "what uses the store?"   # scoped subgraph for a question
comanda graph stats                          # counts by kind + hub nodes
comanda graph export -o graph.json           # graphify-style JSON
```

`-n <namespace>` selects a graph (default: the index registered for the
current directory); `--db <path>` points at a specific database.
`explain`, `path`, and `query` accept `--json`.

## Workflow recall

No new step type is needed — graph nodes are memory records:

```yaml
review:
  input: STDIN
  model: openai-codex
  memory:
    namespace: myproject
    recall:
      query: input
      types: [graph_node, decision]
      limit: 8
  action: "Review this change; cite relevant graph node IDs."
  output: STDOUT
```

## Design notes and limits

- Symbol extraction reuses the codebaseindex language adapters (regex-based,
  Go/Python/TypeScript/Flutter/Java) — no tree-sitter dependency. Edge
  precision is lower than AST-based tools; the `inferred` tag makes that
  explicit, and only type names defined exactly once in the scan participate
  in inferred `uses` resolution.
- Storage adds `graph_nodes` / `graph_edges` tables (plus a node FTS index) to
  the existing semantic memory SQLite database — no new dependencies.
- Hub ranking (`graph stats`) is computed at query time. Community detection,
  HTML visualization, MCP serving, and a dedicated `knowledge_graph.use`
  workflow step type are deliberately out of scope for v1.
