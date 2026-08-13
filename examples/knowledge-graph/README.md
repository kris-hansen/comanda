# Knowledge Graph Examples

Turn a codebase index into a traversable knowledge graph stored in semantic
memory, then query the graph instead of grepping files — or feed it to a long
agentic loop as bounded codebase context. See the
[design doc](../../docs/design/knowledge-graph.md) for the full model.

## Build a graph

```bash
# Index + graph in one run
comanda index capture -n myproject --graph

# Or build/update from a registered index
comanda graph build myproject
comanda graph build myproject --enhance   # optional AI-inferred concepts/edges
comanda graph update myproject
```

## Query it

```bash
comanda graph explain Store                # a node and its connections
comanda graph path main Store              # shortest connection between nodes
comanda graph query "what uses the store?" # scoped subgraph for a question
comanda graph stats                        # counts and hub nodes
comanda graph export -o graph.json         # graphify-style JSON export
comanda graph visualize                     # search and browse locally in a browser
```

`visualize` serves a localhost-only, read-only navigator. Search for a symbol,
file, package, or concept and click a result to inspect its focused
neighborhood. The graph API is reusable by native clients:

```text
GET /graph?namespace=myproject
GET /graph/search?namespace=myproject&q=Store
GET /graph/subgraph?namespace=myproject&focus=store&depth=2
```

Edges are confidence-tagged: `EXTRACTED` (explicit in the source) vs.
`INFERRED` (name resolution or the AI pass).

## Recall it in a workflow

[graph-recall.yaml](graph-recall.yaml) shows a step that pulls relevant graph
nodes into context through the standard semantic memory mapping:

```bash
git diff | comanda process examples/knowledge-graph/graph-recall.yaml
```

## Use it as context for a long agentic loop

[codebase-context-loop.yaml](codebase-context-loop.yaml) is a stateful
improvement loop where every step recalls from the same namespace: knowledge
graph nodes (`graph_node`) for codebase structure plus durable project facts
(`decision`, `constraint`) seeded by hand. Recall is bounded
(`limit` + `max_chars`), so each iteration gets focused context instead of the
whole index.

One-time setup on the target repo:

```bash
# 1. Index + knowledge graph (nodes mirrored as graph_node memory records)
comanda index capture -n myproject --graph

# 2. Seed project facts the graph cannot know
comanda memory add --namespace myproject --type decision \
  --source architecture "Keep provider interfaces stable across refactors."
comanda memory add --namespace myproject --type constraint \
  --source contributing "No new third-party dependencies without an issue."
```

Run it (stateful — Ctrl-C and re-run to resume):

```bash
echo "Improve error handling in the storage layer" | \
  comanda process examples/knowledge-graph/codebase-context-loop.yaml
```

Adapt `allowed_paths`, the `tests` quality gate, and the namespace to your
repo. Note that block-style loop steps declare their own `memory:` mapping —
loops do not inherit memory from `agentic-loop.config`.
