package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kris-hansen/comanda/utils/semanticmemory"
)

func TestGraphAPIProxiesRegisteredGraphForAuthenticatedClients(t *testing.T) {
	root := t.TempDir()
	indexPath := filepath.Join(root, ".comanda", "index.md")
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, []byte("# Index"), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := semanticmemory.DefaultPath(root, "demo")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	store, err := semanticmemory.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.UpsertGraphNode(context.Background(), semanticmemory.GraphNode{
		ID: "demo|server", Namespace: "demo", Kind: semanticmemory.GraphNodeComponent, Name: "server",
	}); err != nil {
		t.Fatal(err)
	}

	server := newContextTestServer(t, root, indexPath)
	req := httptest.NewRequest(http.MethodGet, "/graph/api/v1/overview?namespace=demo", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	server.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("graph overview status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Namespace string `json:"namespace"`
		Nodes     []struct {
			Name string `json:"name"`
		} `json:"nodes"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Namespace != "demo" || len(response.Nodes) != 1 || response.Nodes[0].Name != "server" {
		t.Fatalf("unexpected graph overview: %#v", response)
	}
}
