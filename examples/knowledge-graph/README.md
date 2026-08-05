# Knowledge Graph Examples

Turn a codebase index into a traversable knowledge graph stored in semantic
memory, then query the graph instead of grepping files. See the
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
```

Edges are confidence-tagged: `EXTRACTED` (explicit in the source) vs.
`INFERRED` (name resolution or the AI pass).

## Recall it in a workflow

[graph-recall.yaml](graph-recall.yaml) shows a step that pulls relevant graph
nodes into context through the standard semantic memory mapping:

```bash
git diff | comanda process examples/knowledge-graph/graph-recall.yaml
```
