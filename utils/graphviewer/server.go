// Package graphviewer exposes a local, read-only HTTP API for navigating a
// Comanda knowledge graph. Both the browser visualizer and future native
// clients (such as Canvas) consume this contract.
package graphviewer

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"strings"

	"github.com/kris-hansen/comanda/utils/knowledgegraph"
	"github.com/kris-hansen/comanda/utils/semanticmemory"
)

// OpenQuerier opens a read-only graph query session for namespace. A blank
// namespace asks the caller to select its default graph.
type OpenQuerier func(context.Context, string) (*knowledgegraph.Querier, func() error, error)

// API provides the graph navigation contract. Annotation writes are limited to
// durable human guidance on existing graph nodes; it never exposes files.
type API struct {
	open OpenQuerier
}

// NewAPI creates a graph navigation API with these endpoints:
//
//	GET /api/v1/overview?namespace=<name>&scope=<all|database>
//	GET /api/v1/graph?namespace=<name>
//	GET /api/v1/search?q=<text>&limit=<n>&namespace=<name>
//	GET /api/v1/query?question=<text>&namespace=<name>
//	GET /api/v1/neighbors?focus=<node-id-or-name>&limit=<n>&offset=<n>&namespace=<name>&scope=<all|database>
//	GET /api/v1/subgraph?focus=<node-id-or-name>&depth=1..3&namespace=<name>&scope=<all|database>
//
// scope=database switches overview/neighbors/subgraph into the database-only
// view: overview returns the schema/table architecture map instead of
// component/package nodes, and neighbors/subgraph restrict traversal to
// database node kinds so drilling into a schema or table only ever surfaces
// its database neighborhood.
func NewAPI(open OpenQuerier) *API {
	return &API{open: open}
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if r.Method == http.MethodPost && r.URL.Path != "/api/v1/annotations" {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	switch r.URL.Path {
	case "/api/v1/overview":
		a.handleOverview(w, r)
	case "/api/v1/graph":
		a.handleGraph(w, r)
	case "/api/v1/search":
		a.handleSearch(w, r)
	case "/api/v1/query":
		a.handleQuery(w, r)
	case "/api/v1/annotations":
		a.handleAnnotations(w, r)
	case "/api/v1/neighbors":
		a.handleNeighbors(w, r)
	case "/api/v1/subgraph":
		a.handleSubgraph(w, r)
	default:
		writeError(w, http.StatusNotFound, "graph API endpoint not found")
	}
}

func (a *API) handleAnnotations(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		nodeID := strings.TrimSpace(r.URL.Query().Get("node_id"))
		if nodeID == "" {
			writeError(w, http.StatusBadRequest, "node_id is required")
			return
		}
		a.withQuerier(w, r, func(q *knowledgegraph.Querier) {
			node, err := q.NodeByID(r.Context(), nodeID)
			if err != nil {
				writeError(w, http.StatusNotFound, err.Error())
				return
			}
			annotations, err := q.Annotations(r.Context(), *node)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"node_id": knowledgegraph.LocalID(node.ID), "annotations": annotations})
		})
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var input struct {
		NodeID  string `json:"node_id"`
		Content string `json:"content"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "annotation JSON must contain node_id and content")
		return
	}
	if strings.TrimSpace(input.NodeID) == "" || strings.TrimSpace(input.Content) == "" {
		writeError(w, http.StatusBadRequest, "node_id and content are required")
		return
	}
	a.withQuerier(w, r, func(q *knowledgegraph.Querier) {
		node, err := q.NodeByID(r.Context(), input.NodeID)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		annotation, err := q.Annotate(r.Context(), *node, input.Content)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, annotation)
	})
}

func (a *API) handleOverview(w http.ResponseWriter, r *http.Request) {
	databaseOnly := strings.TrimSpace(r.URL.Query().Get("scope")) == "database"
	a.withQuerier(w, r, func(q *knowledgegraph.Querier) {
		var overview *knowledgegraph.Overview
		var err error
		if databaseOnly {
			overview, err = q.DatabaseOverview(r.Context())
		} else {
			overview, err = q.Overview(r.Context())
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, overview)
	})
}

func (a *API) withQuerier(w http.ResponseWriter, r *http.Request, run func(*knowledgegraph.Querier)) {
	q, closeStore, err := a.open(r.Context(), r.URL.Query().Get("namespace"))
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	defer func() { _ = closeStore() }()
	run(q)
}

func (a *API) handleGraph(w http.ResponseWriter, r *http.Request) {
	a.withQuerier(w, r, func(q *knowledgegraph.Querier) {
		graph, err := q.Export(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, graph)
	})
}

func (a *API) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeError(w, http.StatusBadRequest, "q is required")
		return
	}
	limit := parseBoundedInt(r.URL.Query().Get("limit"), 20, 1, 50)
	a.withQuerier(w, r, func(q *knowledgegraph.Querier) {
		nodes, err := q.FindNodes(r.Context(), query, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out := make([]knowledgegraph.ExportNode, 0, len(nodes))
		for _, node := range nodes {
			out = append(out, exportNode(node))
		}
		writeJSON(w, http.StatusOK, map[string]any{"namespace": q.Namespace(), "query": query, "nodes": out})
	})
}

// handleQuery turns a natural-language question into the same compact graph
// explanation as `comanda graph query`, together with the subgraph that backs
// it. The browser can therefore keep searching and querying in one field
// without inventing a second, UI-only query language.
func (a *API) handleQuery(w http.ResponseWriter, r *http.Request) {
	question := strings.TrimSpace(r.URL.Query().Get("question"))
	if question == "" {
		writeError(w, http.StatusBadRequest, "question is required")
		return
	}
	a.withQuerier(w, r, func(q *knowledgegraph.Querier) {
		answer, err := q.Query(r.Context(), question)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		seeds, err := q.FindNodes(r.Context(), question, 5)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		graph, err := q.ScopedExport(r.Context(), seeds, 1)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"namespace": q.Namespace(),
			"question":  question,
			"answer":    answer,
			"graph":     graph,
		})
	})
}

func (a *API) handleNeighbors(w http.ResponseWriter, r *http.Request) {
	focus := strings.TrimSpace(r.URL.Query().Get("focus"))
	if focus == "" {
		writeError(w, http.StatusBadRequest, "focus is required")
		return
	}
	limit := parseBoundedInt(r.URL.Query().Get("limit"), 160, 1, 500)
	offset := parseBoundedInt(r.URL.Query().Get("offset"), 0, 0, 1_000_000)
	databaseOnly := strings.TrimSpace(r.URL.Query().Get("scope")) == "database"
	a.withQuerier(w, r, func(q *knowledgegraph.Querier) {
		node, err := resolveFocus(r.Context(), q, focus)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		page, err := q.NeighborPage(r.Context(), *node, limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if databaseOnly {
			page.Graph = filterGraphToKinds(page.Graph, page.Focus.ID, knowledgegraph.DatabaseNodeKinds)
		}
		writeJSON(w, http.StatusOK, page)
	})
}

// filterGraphToKinds keeps keepID (the focus node, regardless of its own
// kind) and any other node whose kind is in allowKinds, then drops edges
// whose endpoint was removed so the result never dangles. It backs the
// "database only" neighbor page: GraphNeighborPage itself is a generic,
// kind-agnostic bounded query, so the scope filter is applied once on the
// small, already-bounded page it returns rather than plumbed into the store.
func filterGraphToKinds(g knowledgegraph.ExportGraph, keepID string, allowKinds map[string]bool) knowledgegraph.ExportGraph {
	kept := make(map[string]bool, len(g.Nodes))
	nodes := make([]knowledgegraph.ExportNode, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		if n.ID == keepID || allowKinds[n.Kind] {
			kept[n.ID] = true
			nodes = append(nodes, n)
		}
	}
	edges := make([]knowledgegraph.ExportEdge, 0, len(g.Edges))
	for _, e := range g.Edges {
		if kept[e.Source] && kept[e.Target] {
			edges = append(edges, e)
		}
	}
	return knowledgegraph.ExportGraph{Namespace: g.Namespace, Nodes: nodes, Edges: edges, Truncated: g.Truncated}
}

func (a *API) handleSubgraph(w http.ResponseWriter, r *http.Request) {
	focus := strings.TrimSpace(r.URL.Query().Get("focus"))
	if focus == "" {
		writeError(w, http.StatusBadRequest, "focus is required")
		return
	}
	depth := parseBoundedInt(r.URL.Query().Get("depth"), 1, 1, 3)
	databaseOnly := strings.TrimSpace(r.URL.Query().Get("scope")) == "database"
	a.withQuerier(w, r, func(q *knowledgegraph.Querier) {
		node, err := resolveFocus(r.Context(), q, focus)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		var graph *knowledgegraph.ExportGraph
		if databaseOnly {
			graph, err = q.ScopedExportKinds(r.Context(), []semanticmemory.GraphNode{*node}, depth, knowledgegraph.DatabaseNodeKinds)
		} else {
			graph, err = q.ScopedExport(r.Context(), []semanticmemory.GraphNode{*node}, depth)
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"focus": exportNode(*node), "depth": depth, "graph": graph})
	})
}

func resolveFocus(ctx context.Context, q *knowledgegraph.Querier, focus string) (*semanticmemory.GraphNode, error) {
	node, err := q.NodeByID(ctx, focus)
	if err != nil {
		node, err = q.Resolve(ctx, focus)
	}
	return node, err
}

func exportNode(node semanticmemory.GraphNode) knowledgegraph.ExportNode {
	return knowledgegraph.ExportNode{ID: knowledgegraph.LocalID(node.ID), Kind: node.Kind, Name: node.Name, Path: node.Path, Package: node.Package, Summary: node.Summary, Degree: node.Degree}
}

func parseBoundedInt(raw string, fallback, min, max int) int {
	value, err := strconv.Atoi(raw)
	if err != nil || value == 0 {
		return fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

//go:embed web/*
var webFiles embed.FS

// VisualizationHandler serves the interactive browser UI and its API from one
// localhost origin. The API remains usable independently through NewAPI.
func VisualizationHandler(open OpenQuerier) (http.Handler, error) {
	assets, err := fs.Sub(webFiles, "web")
	if err != nil {
		return nil, fmt.Errorf("load graph visualizer assets: %w", err)
	}
	api := NewAPI(open)
	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	mux.Handle("/", http.FileServer(http.FS(assets)))
	return mux, nil
}
