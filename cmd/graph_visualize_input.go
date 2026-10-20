package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/kris-hansen/comanda/utils/graphviewer"
	"github.com/kris-hansen/comanda/utils/knowledgegraph"
	"github.com/kris-hansen/comanda/utils/semanticmemory"
)

// loadVisualizedGraph reads the public graph export format into an ephemeral
// store. It never creates a repository index or a persistent memory database.
func loadVisualizedGraph(path string) (*semanticmemory.Store, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("open graph input: %w", err)
	}
	defer file.Close()

	const maxGraphBytes = 256 << 20
	info, err := file.Stat()
	if err != nil {
		return nil, "", fmt.Errorf("inspect graph input: %w", err)
	}
	if info.Size() > maxGraphBytes {
		return nil, "", fmt.Errorf("graph input exceeds 256 MiB")
	}
	var graph knowledgegraph.ExportGraph
	decoder := json.NewDecoder(io.LimitReader(file, maxGraphBytes+1))
	if err := decoder.Decode(&graph); err != nil {
		return nil, "", fmt.Errorf("decode graph input: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, "", fmt.Errorf("graph input must contain one JSON object")
	}
	namespace := strings.TrimSpace(graph.Namespace)
	if namespace == "" {
		return nil, "", fmt.Errorf("graph input has no namespace")
	}
	if len(graph.Nodes) == 0 {
		return nil, "", fmt.Errorf("graph input has no nodes")
	}

	nodes := make([]semanticmemory.GraphNode, 0, len(graph.Nodes))
	ids := make(map[string]struct{}, len(graph.Nodes))
	for _, node := range graph.Nodes {
		if strings.TrimSpace(node.ID) == "" || strings.TrimSpace(node.Name) == "" || strings.TrimSpace(node.Kind) == "" {
			return nil, "", fmt.Errorf("graph nodes need an id, kind, and name")
		}
		if _, exists := ids[node.ID]; exists {
			return nil, "", fmt.Errorf("duplicate graph node id %q", node.ID)
		}
		ids[node.ID] = struct{}{}
		nodes = append(nodes, semanticmemory.GraphNode{
			ID: knowledgegraph.NodeID(namespace, node.ID), Namespace: namespace,
			Kind: node.Kind, Name: node.Name, Path: node.Path,
			Package: node.Package, Summary: node.Summary,
		})
	}
	edges := make([]semanticmemory.GraphEdge, 0, len(graph.Edges))
	for _, edge := range graph.Edges {
		if _, exists := ids[edge.Source]; !exists {
			return nil, "", fmt.Errorf("graph edge refers to unknown source %q", edge.Source)
		}
		if _, exists := ids[edge.Target]; !exists {
			return nil, "", fmt.Errorf("graph edge refers to unknown target %q", edge.Target)
		}
		if strings.TrimSpace(edge.Kind) == "" {
			return nil, "", fmt.Errorf("graph edges need a kind")
		}
		source := knowledgegraph.NodeID(namespace, edge.Source)
		target := knowledgegraph.NodeID(namespace, edge.Target)
		edges = append(edges, semanticmemory.GraphEdge{
			ID:        knowledgegraph.EdgeID(namespace, source, target, edge.Kind),
			Namespace: namespace, SourceID: source, TargetID: target,
			Kind: edge.Kind, Confidence: edge.Confidence, Evidence: edge.Evidence,
		})
	}
	store, err := semanticmemory.Open(":memory:")
	if err != nil {
		return nil, "", fmt.Errorf("open in-memory graph: %w", err)
	}
	if err := store.ReplaceGraph(context.Background(), namespace, nodes, edges, nil); err != nil {
		store.Close()
		return nil, "", fmt.Errorf("load graph input: %w", err)
	}
	return store, namespace, nil
}

func runImportedGraphVisualize(path string) error {
	store, namespace, err := loadVisualizedGraph(path)
	if err != nil {
		return err
	}
	defer store.Close()
	open := func(_ context.Context, requested string) (*knowledgegraph.Querier, func() error, error) {
		if requested != "" && requested != namespace {
			return nil, nil, fmt.Errorf("this visualizer is bound to namespace %q", namespace)
		}
		return knowledgegraph.NewQuerier(store, namespace), func() error { return nil }, nil
	}
	var graphOpen graphviewer.OpenQuerier = open
	return serveGraphVisualizer(graphOpen, true)
}
