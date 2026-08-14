package graphviewer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/knowledgegraph"
	"github.com/kris-hansen/comanda/utils/semanticmemory"
)

func TestAPIExportsSearchesAndFocusesGraph(t *testing.T) {
	store, err := semanticmemory.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	alpha, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{ID: "demo|alpha", Namespace: "demo", Kind: semanticmemory.GraphNodeFunction, Name: "Alpha", Path: "alpha.go", Summary: "entry point"})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{ID: "demo|beta", Namespace: "demo", Kind: semanticmemory.GraphNodeType, Name: "Beta", Path: "beta.go", Summary: "shared type"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGraphEdge(ctx, semanticmemory.GraphEdge{ID: "demo|edge", Namespace: "demo", SourceID: alpha.ID, TargetID: beta.ID, Kind: semanticmemory.GraphEdgeUses, Confidence: semanticmemory.GraphConfidenceExtracted}); err != nil {
		t.Fatal(err)
	}
	gamma, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{ID: "demo|gamma", Namespace: "demo", Kind: semanticmemory.GraphNodeFunction, Name: "Gamma", Path: "gamma.go", Summary: "secondary entry point"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGraphEdge(ctx, semanticmemory.GraphEdge{ID: "demo|edge-two", Namespace: "demo", SourceID: alpha.ID, TargetID: gamma.ID, Kind: semanticmemory.GraphEdgeUses, Confidence: semanticmemory.GraphConfidenceExtracted}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{ID: "demo|component", Namespace: "demo", Kind: semanticmemory.GraphNodeComponent, Name: "app", Path: ".", Summary: "application component"}); err != nil {
		t.Fatal(err)
	}
	if err := store.RefreshGraphDegrees(ctx, "demo"); err != nil {
		t.Fatal(err)
	}

	api := NewAPI(func(_ context.Context, namespace string) (*knowledgegraph.Querier, func() error, error) {
		if namespace != "" && namespace != "demo" {
			return nil, nil, &notFoundError{namespace}
		}
		return knowledgegraph.NewQuerier(store, "demo"), func() error { return nil }, nil
	})

	graph := requestJSON(t, api, "/api/v1/graph")
	if got := len(graph["nodes"].([]any)); got != 4 {
		t.Fatalf("graph nodes = %d, want 4", got)
	}
	overview := requestJSON(t, api, "/api/v1/overview")
	if got := len(overview["nodes"].([]any)); got != 1 {
		t.Fatalf("overview node count = %d, want 1", got)
	}
	search := requestJSON(t, api, "/api/v1/search?q=Alpha")
	if got := len(search["nodes"].([]any)); got != 1 {
		t.Fatalf("search node count = %d, want 1", got)
	}
	focused := requestJSON(t, api, "/api/v1/subgraph?focus=alpha&depth=2")
	subgraph := focused["graph"].(map[string]any)
	if got := len(subgraph["nodes"].([]any)); got != 3 {
		t.Fatalf("focused graph nodes = %d, want 3", got)
	}
	neighbors := requestJSON(t, api, "/api/v1/neighbors?focus=alpha&limit=1")
	if hasMore, ok := neighbors["has_more"].(bool); !ok || !hasMore {
		t.Fatalf("neighbors has_more = %#v, want true", neighbors["has_more"])
	}
	neighborGraph := neighbors["graph"].(map[string]any)
	if got := len(neighborGraph["nodes"].([]any)); got != 2 {
		t.Fatalf("neighbor page nodes = %d, want 2", got)
	}
}

func TestAPIValidatesSearchAndFocus(t *testing.T) {
	api := NewAPI(func(context.Context, string) (*knowledgegraph.Querier, func() error, error) {
		return nil, func() error { return nil }, nil
	})
	for _, path := range []string{"/api/v1/search", "/api/v1/subgraph", "/api/v1/neighbors"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want %d", path, w.Code, http.StatusBadRequest)
		}
	}
}

func TestViewerUIKeepsNodeClicksSeparateFromCanvasPanning(t *testing.T) {
	page, err := webFiles.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`data-node-id=`,
		`role="button"`,
		`if(nodeID(e))return`,
		`status.classList.add('loading')`,
		`Opening ${node?.name||'node'}…`,
		`prefers-color-scheme:dark`,
		`id="back"`,
		`Node label density`,
		`factor=e.deltaY<0?1.04:.96`,
		`padTop=155`,
		`Map — stable code topology`,
		`input → selected system → output flow`,
		`marker-end="url(#arrow)"`,
		`rowGap=170`,
		`layerGap=132`,
	} {
		if !strings.Contains(string(page), want) {
			t.Errorf("visualizer UI is missing %q", want)
		}
	}
}

func requestJSON(t *testing.T, api http.Handler, path string) map[string]any {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	api.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, body = %s", path, w.Code, w.Body.String())
	}
	var out map[string]any
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

type notFoundError struct{ namespace string }

func (e *notFoundError) Error() string { return "namespace not found: " + e.namespace }
