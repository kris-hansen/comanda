package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
)

func TestContextInventoryExposesRegisteredIndexAsProject(t *testing.T) {
	root := t.TempDir()
	indexPath := filepath.Join(root, ".comanda", "index.md")
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, []byte("# Index"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := newContextTestServer(t, root, indexPath)

	req := httptest.NewRequest(http.MethodGet, "/context", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	server.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("context status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response ContextResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.APIVersion != contextAPIVersion || len(response.Projects) != 1 || len(response.Indexes) != 1 {
		t.Fatalf("unexpected context response: %#v", response)
	}
	if response.Projects[0].SourceRoot != root || response.Indexes[0].Status != "ready" {
		t.Fatalf("project/index did not report ready source context: %#v", response)
	}
	if !hasCapability(response.Capabilities, "knowledge-graph-api") {
		t.Fatalf("context capabilities = %v, want knowledge-graph-api", response.Capabilities)
	}
}

func hasCapability(capabilities []string, want string) bool {
	for _, capability := range capabilities {
		if capability == want {
			return true
		}
	}
	return false
}

func TestPreflightFindsMissingIndexBeforeRun(t *testing.T) {
	root := t.TempDir()
	indexPath := filepath.Join(root, ".comanda", "index.md")
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, []byte("# Index"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := newContextTestServer(t, root, indexPath)
	workflow := `load_index:
  type: codebase-index
  codebase_index:
    use: missing-index
`
	body, err := json.Marshal(PreflightRequest{Workflow: workflow, ProjectID: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/preflight", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	server.mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("preflight status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response PreflightResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Ready || response.ResolvedSourceRoot != root {
		t.Fatalf("expected missing index to block run: %#v", response)
	}
	if len(response.Issues) == 0 || response.Issues[0].Code != "index_missing" {
		t.Fatalf("expected actionable index issue, got %#v", response.Issues)
	}
}

func TestPreflightRejectsCodebaseRootOutsideProject(t *testing.T) {
	root := t.TempDir()
	indexPath := filepath.Join(root, ".comanda", "index.md")
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, []byte("# Index"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := newContextTestServer(t, root, indexPath)
	workflow := `index:
  type: codebase-index
  codebase_index:
    root: ../outside
`
	body, err := json.Marshal(PreflightRequest{Workflow: workflow, ProjectID: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/preflight", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()
	server.mux.ServeHTTP(rec, req)

	var response PreflightResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Ready || len(response.Issues) != 1 || response.Issues[0].Code != "source_invalid" {
		t.Fatalf("expected unsafe source root to block run, got %#v", response)
	}
}

func newContextTestServer(t *testing.T, root, indexPath string) *Server {
	t.Helper()
	server := &Server{
		mux:    http.NewServeMux(),
		config: &config.ServerConfig{DataDir: t.TempDir(), BearerToken: "test-token", Enabled: true},
		envConfig: &config.EnvConfig{Indexes: map[string]*config.IndexEntry{
			"demo": {Path: root, IndexPath: indexPath, FileCount: 4, SizeBytes: 7},
		}},
	}
	server.routes()
	return server
}
