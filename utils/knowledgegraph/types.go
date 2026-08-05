// Package knowledgegraph turns a codebaseindex scan into a typed,
// confidence-tagged knowledge graph persisted in the semantic memory store.
// Extraction is deterministic and local first (components, packages, files,
// symbols, imports); an optional LLM pass can add inferred concept nodes and
// edges. Inspired by github.com/Graphify-Labs/graphify.
package knowledgegraph

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"

	"github.com/kris-hansen/comanda/utils/semanticmemory"
)

// Node and edge kinds are re-exported from semanticmemory so callers use one
// vocabulary end to end.
const (
	NodeComponent = semanticmemory.GraphNodeComponent
	NodePackage   = semanticmemory.GraphNodePackage
	NodeFile      = semanticmemory.GraphNodeFile
	NodeType      = semanticmemory.GraphNodeType
	NodeFunction  = semanticmemory.GraphNodeFunction
	NodeConcept   = semanticmemory.GraphNodeConcept

	EdgeContains  = semanticmemory.GraphEdgeContains
	EdgeBelongsTo = semanticmemory.GraphEdgeBelongsTo
	EdgeImports   = semanticmemory.GraphEdgeImports
	EdgeDefines   = semanticmemory.GraphEdgeDefines
	EdgeUses      = semanticmemory.GraphEdgeUses
	EdgeReference = semanticmemory.GraphEdgeReference

	ConfidenceExtracted = semanticmemory.GraphConfidenceExtracted
	ConfidenceInferred  = semanticmemory.GraphConfidenceInferred
)

// Graph is an in-memory knowledge graph under construction.
type Graph struct {
	Namespace string
	Nodes     map[string]*semanticmemory.GraphNode
	Edges     map[string]*semanticmemory.GraphEdge
}

// NewGraph creates an empty graph for a namespace.
func NewGraph(namespace string) *Graph {
	return &Graph{
		Namespace: namespace,
		Nodes:     make(map[string]*semanticmemory.GraphNode),
		Edges:     make(map[string]*semanticmemory.GraphEdge),
	}
}

// NodeID builds a stable, namespace-qualified node ID so rebuilds upsert in
// place and separate graphs never collide on the primary key.
func NodeID(namespace, local string) string {
	return namespace + "|" + local
}

// EdgeID builds a stable edge ID from its endpoints and kind.
func EdgeID(namespace, sourceID, targetID, kind string) string {
	sum := sha1.Sum([]byte(sourceID + ">" + targetID + "|" + kind))
	return namespace + "|e" + hex.EncodeToString(sum[:8])
}

// AddNode inserts or replaces a node. The node's ID and namespace are set
// from the local ID and graph namespace.
func (g *Graph) AddNode(localID, kind, name, path, pkg, summary string) *semanticmemory.GraphNode {
	id := NodeID(g.Namespace, localID)
	node, ok := g.Nodes[id]
	if !ok {
		node = &semanticmemory.GraphNode{ID: id, Namespace: g.Namespace}
		g.Nodes[id] = node
	}
	node.Kind = kind
	node.Name = name
	node.Path = path
	node.Package = pkg
	if summary != "" {
		node.Summary = summary
	}
	return node
}

// AddEdge inserts an edge between two node IDs (already namespace-qualified).
// Duplicate (source, target, kind) edges collapse; the first confidence wins
// unless the existing one was inferred and the new one is extracted.
func (g *Graph) AddEdge(sourceID, targetID, kind, confidence, evidence string) {
	if sourceID == targetID {
		return
	}
	id := EdgeID(g.Namespace, sourceID, targetID, kind)
	if existing, ok := g.Edges[id]; ok {
		if existing.Confidence == ConfidenceInferred && confidence == ConfidenceExtracted {
			existing.Confidence = confidence
			existing.Evidence = evidence
		}
		return
	}
	g.Edges[id] = &semanticmemory.GraphEdge{
		ID:         id,
		Namespace:  g.Namespace,
		SourceID:   sourceID,
		TargetID:   targetID,
		Kind:       kind,
		Confidence: confidence,
		Evidence:   evidence,
	}
}

// LocalID strips the namespace prefix from a stored node or edge ID.
func LocalID(id string) string {
	for i := len(id) - 1; i >= 0; i-- {
		if id[i] == '|' {
			return id[i+1:]
		}
	}
	return id
}

// ExportNode is the JSON export shape for a node (graphify-style graph.json).
type ExportNode struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Path    string `json:"path,omitempty"`
	Package string `json:"package,omitempty"`
	Summary string `json:"summary,omitempty"`
	Degree  int    `json:"degree"`
}

// ExportEdge is the JSON export shape for an edge.
type ExportEdge struct {
	Source     string `json:"source"`
	Target     string `json:"target"`
	Kind       string `json:"kind"`
	Confidence string `json:"confidence"`
	Evidence   string `json:"evidence,omitempty"`
}

// ExportGraph is the top-level graph.json shape.
type ExportGraph struct {
	Namespace string       `json:"namespace"`
	Nodes     []ExportNode `json:"nodes"`
	Edges     []ExportEdge `json:"edges"`
}

// String renders a node for display.
func NodeLabel(n *semanticmemory.GraphNode) string {
	if n.Path != "" {
		return fmt.Sprintf("%s (%s, %s)", n.Name, n.Kind, n.Path)
	}
	return fmt.Sprintf("%s (%s)", n.Name, n.Kind)
}
