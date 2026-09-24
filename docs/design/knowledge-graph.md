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

# From the project root or any subdirectory (including .comanda)
comanda index update
comanda graph build
comanda graph update
```

Unnamed index and graph commands select the nearest registered project root.
Nested registered projects take precedence over their parents. If multiple
indexes share that nearest root, specify the exact registered name from
`comanda index list`. Explicit names remain valid from any directory.

`comanda graph list` shows built graphs with node/edge counts and database paths
across registered indexes; indexes without graphs are omitted. Use `--json` for
machine-readable output, `-n <namespace>` to filter, or `--db <path>` to list all
graphs in a custom database. Custom database locations are not registered
automatically, so they require `--db` when listing or querying.

`--graph` also works on `comanda index update`. Encrypted indexes are skipped
(graph data is plain SQLite). The graph is rebuilt deterministically on each
run — nodes and edges have stable IDs, so rebuilds upsert in place and stale
nodes from deleted files are removed.

Index updates automatically refresh an existing graph in the project's memory database,
including updates with no source changes. Use `--graph` to create a graph when
one does not exist. Graphs built with a custom `graph --db` path still require an
explicit `graph update --db <path>`.

## Shared Index Structure

The indexer produces two views from one scan: bounded markdown for LLM context
and complete structural metadata for graph construction. `--max-files` limits
the markdown file selection; graph extraction covers every source/config file
included by the adapters and ignore rules. Source extraction and update hashes
read complete files, so declarations after 32 KB and changes after 1 MB are no
longer silently missed.

The existing `.meta.json` sidecar retains its original fields and adds a schema
version, repository root, per-file language, package identity, and extracted
symbols. Old markdown indexes remain readable. The next `index update`
automatically regenerates legacy metadata, even with no source changes; no
recapture or `--full` is required. Subsequent updates reuse content-validated
symbols for unchanged files and rebuild component information. Encrypted indexes
omit source-derived symbols from the plaintext sidecar.

Go package identities use the nearest `go.mod`, including nested modules, so
same-named packages stay distinct and external imports are not linked to a local
package merely because their last path segment matches. Go declarations retain
complete signatures, field types, interface methods, and documentation. Receiver
types link to their methods across files. Root components contain their files,
and duplicate component names are distinguished by root. Existing file, type,
and function IDs are preserved; package IDs with resolved module paths become
qualified on rebuild. Parser plugins configured for an index also run in graph
builds. Other languages continue to use their existing adapters, and name-based
type references remain marked `inferred`.

Workflow indexing on the HTTP server is confined to the selected registered
project, or to the configured data directory when no project is selected.
An omitted `codebase_index.root` selects that same approved root. The
`/yaml/process` endpoint accepts the same `?project=<registered-name>` selection
as `/process`; `runtimeDir` does not grant access to additional source trees.
Preflight and nested workflows apply the same rules. CLI indexing continues to
accept an explicitly chosen repository anywhere the local user can access.
Go module reads are confined to the index root, including symlink resolution;
an escaping module symlink fails the scan instead of reading outside the tree.

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
current directory or its nearest registered ancestor); `--db <path>` points at a specific database.
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

## PostgreSQL schema support

`.sql` files containing PostgreSQL DDL are recognized automatically during
`comanda index capture` — no parser plugin manifest is required. A built-in
adapter reads `CREATE SCHEMA`, `CREATE TABLE`, and
`ALTER TABLE ... ADD CONSTRAINT` statements and emits the same
[parser semantic graph](../../examples/codebase-index/README.md#optional-parser-semantic-graph)
contract used by external parser plugins, so it flows through the existing
graph builder, storage, export, and visualizer unchanged.

Extracted for each schema file:

- **Schemas/namespaces** — only when explicitly present, either a
  `CREATE SCHEMA` statement or a schema-qualified name (`app.orders`).
  Unqualified tables default internally to `public` for stable IDs, but no
  `public` schema node is invented.
- **Tables** — one `table` node per schema-qualified table, deduplicated by
  qualified name across every scanned file (a table created in one migration
  and referenced from another resolves to the same node).
- **Columns** — one `column` node per table column, with normalized SQL type,
  nullability, and default expression (when present) in its summary.
- **Primary keys, unique constraints** — inline (`id serial primary key`),
  table-level (`CONSTRAINT ... PRIMARY KEY (...)`), and `ALTER TABLE ... ADD
  CONSTRAINT` / `ADD PRIMARY KEY` / `ADD UNIQUE` forms, including composite
  keys (one edge per column).
- **Foreign keys** — inline column references, table-level
  `FOREIGN KEY (...) REFERENCES ...`, and `ALTER TABLE ... ADD CONSTRAINT ...
  FOREIGN KEY` forms, as both a table-level edge and, when the referenced
  columns are named explicitly, a column-level edge.

Node kinds: `schema`, `table`, `column` (in addition to the existing
`component`/`package`/`file`/`type`/`function`/`concept`). Edge kinds:
`contains` (schema→table, table→column), `primary_key` and `unique`
(table→column), `foreign_key` (table→table), and `references`
(column→column). Every edge is tagged `extracted`, since these relationships
are read directly from the DDL rather than inferred.

A foreign key referencing a table that is not itself defined anywhere in the
scan (an external or not-yet-indexed table) still resolves safely: a
lightweight placeholder node is created for it rather than dropping the edge
or failing the scan. Every entity and edge carries `path:line` evidence back
to the originating statement. Quoted identifiers (`"MixedCase"`) preserve
their exact case; unquoted identifiers fold to lowercase, matching Postgres.
Ordinary DML, views, functions, indexes, and any DDL form the reader does not
recognize are conservatively skipped — they never fail extraction.

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
