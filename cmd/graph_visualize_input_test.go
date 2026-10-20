package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/knowledgegraph"
)

func TestVisualizeExportedGraphWithoutRepository(t *testing.T) {
	input := knowledgegraph.ExportGraph{
		Namespace: "sample",
		Nodes: []knowledgegraph.ExportNode{
			{ID: "component:web", Kind: "component", Name: "Web"},
			{ID: "package:api", Kind: "package", Name: "API"},
		},
		Edges: []knowledgegraph.ExportEdge{{
			Source: "component:web", Target: "package:api",
			Kind: "contains", Confidence: "extracted",
		}},
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "graph.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	store, namespace, err := loadVisualizedGraph(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if namespace != "sample" {
		t.Fatalf("namespace = %q", namespace)
	}
	open := func(_ context.Context, requested string) (*knowledgegraph.Querier, func() error, error) {
		if requested != "" && requested != namespace {
			t.Fatalf("unexpected namespace request %q", requested)
		}
		return knowledgegraph.NewQuerier(store, namespace), func() error { return nil }, nil
	}
	handler, err := graphVisualizerHandler(open, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"/api/v1/graph", "/api/v1/search?q=API", "/api/v1/overview"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, route, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s: %d %s", route, response.Code, response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/graph", nil))
	var got knowledgegraph.ExportGraph
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Nodes) != 2 || len(got.Edges) != 1 || got.Edges[0].Target != "package:api" {
		t.Fatalf("imported graph = %+v", got)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/annotations", strings.NewReader(`{}`)))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("annotation POST status = %d", response.Code)
	}
}

func TestVisualizeExportRejectsDanglingEdge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "graph.json")
	data := `{"namespace":"sample","nodes":[{"id":"web","kind":"component","name":"Web"}],"edges":[{"source":"web","target":"missing","kind":"uses"}]}`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadVisualizedGraph(path); err == nil || !strings.Contains(err.Error(), "unknown target") {
		t.Fatalf("expected dangling edge error, got %v", err)
	}
}
