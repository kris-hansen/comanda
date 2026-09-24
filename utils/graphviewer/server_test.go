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
	emptyAnnotations := requestJSON(t, api, "/api/v1/annotations?node_id=alpha")
	if items, ok := emptyAnnotations["annotations"].([]any); !ok || len(items) != 0 {
		t.Fatalf("empty annotations = %#v, want an empty array", emptyAnnotations["annotations"])
	}

	annotationRequest := httptest.NewRequest(http.MethodPost, "/api/v1/annotations", strings.NewReader(`{"node_id":"alpha","content":"Do not change this boundary without a migration."}`))
	annotationRequest.Header.Set("Content-Type", "application/json")
	annotationResponse := httptest.NewRecorder()
	api.ServeHTTP(annotationResponse, annotationRequest)
	if annotationResponse.Code != http.StatusCreated {
		t.Fatalf("POST annotation status = %d, body = %s", annotationResponse.Code, annotationResponse.Body.String())
	}
	annotations := requestJSON(t, api, "/api/v1/annotations?node_id=alpha")
	items := annotations["annotations"].([]any)
	if len(items) != 1 {
		t.Fatalf("annotation count = %d, want 1", len(items))
	}
	if got := items[0].(map[string]any)["content"]; got != "Do not change this boundary without a migration." {
		t.Fatalf("annotation content = %q", got)
	}
}

func TestAPIValidatesSearchAndFocus(t *testing.T) {
	api := NewAPI(func(context.Context, string) (*knowledgegraph.Querier, func() error, error) {
		return nil, func() error { return nil }, nil
	})
	for _, path := range []string{"/api/v1/search", "/api/v1/query", "/api/v1/subgraph", "/api/v1/neighbors"} {
		r := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		api.ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want %d", path, w.Code, http.StatusBadRequest)
		}
	}
}

func TestAPIOnlyAllowsAnnotationWrites(t *testing.T) {
	api := NewAPI(func(context.Context, string) (*knowledgegraph.Querier, func() error, error) {
		return nil, func() error { return nil }, nil
	})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/graph", nil)
	w := httptest.NewRecorder()
	api.ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST graph status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, http.MethodPost) {
		t.Fatalf("CORS methods = %q, want POST", got)
	}
}

func TestAPIQueriesGraphWithSupportingSubgraph(t *testing.T) {
	store, err := semanticmemory.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	alpha, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{ID: "demo|alpha", Namespace: "demo", Kind: semanticmemory.GraphNodeFunction, Name: "Alpha", Summary: "entry point"})
	if err != nil {
		t.Fatal(err)
	}
	beta, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{ID: "demo|beta", Namespace: "demo", Kind: semanticmemory.GraphNodeType, Name: "Beta", Summary: "shared type"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGraphEdge(ctx, semanticmemory.GraphEdge{ID: "demo|edge", Namespace: "demo", SourceID: alpha.ID, TargetID: beta.ID, Kind: semanticmemory.GraphEdgeUses, Confidence: semanticmemory.GraphConfidenceExtracted}); err != nil {
		t.Fatal(err)
	}

	api := NewAPI(func(_ context.Context, namespace string) (*knowledgegraph.Querier, func() error, error) {
		if namespace != "" && namespace != "demo" {
			return nil, nil, &notFoundError{namespace}
		}
		return knowledgegraph.NewQuerier(store, "demo"), func() error { return nil }, nil
	})
	response := requestJSON(t, api, "/api/v1/query?question=Alpha")
	if !strings.Contains(response["answer"].(string), `Subgraph for "Alpha"`) {
		t.Fatalf("query answer = %q", response["answer"])
	}
	graph := response["graph"].(map[string]any)
	if got := len(graph["nodes"].([]any)); got != 2 {
		t.Fatalf("query graph nodes = %d, want 2", got)
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
		`Labels stay visible`,
		`Search or query`,
		`id="ask"`,
		`/api/v1/query?question=`,
		`labelPitch`,
		`factor=e.deltaY<0?1.04:.96`,
		`padTop=155`,
		`Map — stable code topology`,
		`input → selected system → output flow`,
		`marker-end="url(#arrow)"`,
		`rowGap=Math.max(170`,
		`layerGap=Math.max(132`,
		`Human guidance`,
		`id="save-annotation"`,
		`/api/v1/annotations`,
		`Array.isArray(data.annotations)`,
		`Terminal ${esc(node.kind)}`,
		`function goBack()`,
		`history.push({shown,mode,selected,transform`,
		`id="focus-subgraph"`,
		`id="expand-neighbors"`,
		`/api/v1/subgraph?focus=`,
		`function expandNeighbors(id)`,
		`.result b,.result small`,
		`text-overflow:ellipsis`,
	} {
		if !strings.Contains(string(page), want) {
			t.Errorf("visualizer UI is missing %q", want)
		}
	}
}

func TestOverviewExposesDatabaseKindCountsForFilterSeeding(t *testing.T) {
	store, err := semanticmemory.Open(filepath.Join(t.TempDir(), "graph.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()

	schema, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{ID: "demo|schema:public", Namespace: "demo", Kind: "schema", Name: "public"})
	if err != nil {
		t.Fatal(err)
	}
	table, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{ID: "demo|table:public.users", Namespace: "demo", Kind: "table", Name: "public.users"})
	if err != nil {
		t.Fatal(err)
	}
	column, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{ID: "demo|column:public.users.id", Namespace: "demo", Kind: "column", Name: "public.users.id"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGraphNode(ctx, semanticmemory.GraphNode{ID: "demo|fn:main", Namespace: "demo", Kind: semanticmemory.GraphNodeFunction, Name: "main"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGraphEdge(ctx, semanticmemory.GraphEdge{ID: "demo|e1", Namespace: "demo", SourceID: schema.ID, TargetID: table.ID, Kind: semanticmemory.GraphEdgeContains, Confidence: semanticmemory.GraphConfidenceExtracted}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGraphEdge(ctx, semanticmemory.GraphEdge{ID: "demo|e2", Namespace: "demo", SourceID: table.ID, TargetID: column.ID, Kind: semanticmemory.GraphEdgeContains, Confidence: semanticmemory.GraphConfidenceExtracted}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.UpsertGraphEdge(ctx, semanticmemory.GraphEdge{ID: "demo|e3", Namespace: "demo", SourceID: table.ID, TargetID: column.ID, Kind: "primary_key", Confidence: semanticmemory.GraphConfidenceExtracted}); err != nil {
		t.Fatal(err)
	}

	api := NewAPI(func(_ context.Context, namespace string) (*knowledgegraph.Querier, func() error, error) {
		if namespace != "" && namespace != "demo" {
			return nil, nil, &notFoundError{namespace}
		}
		return knowledgegraph.NewQuerier(store, "demo"), func() error { return nil }, nil
	})

	overview := requestJSON(t, api, "/api/v1/overview")
	counts, ok := overview["node_kind_counts"].(map[string]any)
	if !ok {
		t.Fatalf("node_kind_counts missing or wrong shape: %#v", overview["node_kind_counts"])
	}
	for _, kind := range []string{"schema", "table", "column", semanticmemory.GraphNodeFunction} {
		if _, present := counts[kind]; !present {
			t.Errorf("node_kind_counts missing DB kind %q: %#v", kind, counts)
		}
	}

	subgraph := requestJSON(t, api, "/api/v1/subgraph?focus="+schema.ID+"&depth=2")
	graph := subgraph["graph"].(map[string]any)
	nodes := graph["nodes"].([]any)
	present := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		present[n.(map[string]any)["id"].(string)] = true
	}
	for _, e := range graph["edges"].([]any) {
		edge := e.(map[string]any)
		if !present[edge["source"].(string)] || !present[edge["target"].(string)] {
			t.Fatalf("dangling edge in scoped subgraph: %#v", edge)
		}
	}
}

func TestViewerUIStylesDatabaseKindsWithAccessibleControls(t *testing.T) {
	page, err := webFiles.ReadFile("web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		// A database palette distinct from the code palette, with light and
		// dark variants.
		`--db-schema:`,
		`--db-table:`,
		`--db-column:`,
		`schema:'var(--db-schema)'`,
		`table:'var(--db-table)'`,
		`column:'var(--db-column)'`,
		`.block.schema .top`,
		`.block.table .top`,
		`.block.column .top`,
		// DB-specific relationships (contains among DB nodes, key
		// constraints, foreign keys/references) get distinct, non-color-only
		// styling (dash patterns), not just the generic edge style.
		`DB_KEY_EDGES=new Set(['primary_key','unique'])`,
		`DB_FK_EDGES=new Set(['foreign_key','references'])`,
		`.edge.edge-db {`,
		`.edge.edge-db-key {`,
		`stroke-dasharray:1 3`,
		`.edge.edge-db-fk {`,
		`stroke-dasharray:7 3`,
		// Node kinds filters are seeded from the compact overview kind-count
		// metadata (not a full-graph load) as well as scoped data.
		`Object.keys(overview.node_kind_counts||{}).forEach(k=>kinds.add(k))`,
		// One-click Database only / All-mixed controls with accessible
		// labels, plus per-checkbox toggle labels.
		`id="kinds-db-only"`,
		`id="kinds-all"`,
		`aria-label="Show database nodes only and hide code nodes and relationships"`,
		`aria-label="Show all node kinds, mixing code and database"`,
		`setKindsChecked(k=>DB_KINDS.has(k))`,
		`aria-label="Toggle ${esc(kind)} nodes`,
		`aria-hidden="true" style="color:`,
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
