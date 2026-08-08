package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
)

func newValidateTestServer(t *testing.T) *Server {
	t.Helper()
	server := &Server{
		mux: http.NewServeMux(),
		config: &config.ServerConfig{
			DataDir:     t.TempDir(),
			BearerToken: "test-token",
			Enabled:     true,
		},
		envConfig: &config.EnvConfig{
			Providers: map[string]*config.Provider{
				"openai": {
					APIKey: "test-key",
					Models: []config.Model{
						{
							Name:  "gpt-4o",
							Modes: []config.ModelMode{config.TextMode},
						},
					},
				},
			},
		},
	}
	server.routes()
	return server
}

func postValidate(t *testing.T, server *Server, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/yaml/validate", bytes.NewReader(payload))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)
	return w
}

func TestHandleYAMLValidateValidWorkflow(t *testing.T) {
	server := newValidateTestServer(t)

	validYAML := `
analyze:
  input: STDIN
  model: gpt-4o
  action: "Analyze this text"
  output: STDOUT
`
	w := postValidate(t, server, ValidateRequest{Content: validYAML})

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ValidateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Success {
		t.Errorf("Expected success=true, got false: %s", resp.Error)
	}
	if !resp.Valid {
		t.Errorf("Expected valid=true, got false with errors: %+v", resp.Errors)
	}
}

func TestHandleYAMLValidateInvalidWorkflow(t *testing.T) {
	server := newValidateTestServer(t)

	// Step missing required model/action fields
	invalidYAML := `
broken_step:
  input: STDIN
`
	w := postValidate(t, server, ValidateRequest{Content: invalidYAML})

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ValidateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if !resp.Success {
		t.Errorf("Expected success=true, got false: %s", resp.Error)
	}
	if resp.Valid {
		t.Error("Expected valid=false for workflow with missing fields")
	}
	if len(resp.Errors) == 0 {
		t.Error("Expected validation errors to be reported")
	}
	for _, e := range resp.Errors {
		if e.Message == "" {
			t.Error("Validation errors should include a message")
		}
	}
}

func TestHandleYAMLValidateModelCheck(t *testing.T) {
	server := newValidateTestServer(t)

	yamlWithUnknownModel := `
analyze:
  input: STDIN
  model: totally-unknown-model
  action: "Analyze this text"
  output: STDOUT
`
	w := postValidate(t, server, ValidateRequest{Content: yamlWithUnknownModel, CheckModels: true})

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ValidateResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Valid {
		t.Error("Expected valid=false for workflow using an unconfigured model")
	}
	found := false
	for _, m := range resp.InvalidModels {
		if m == "totally-unknown-model" {
			found = true
		}
	}
	if !found {
		t.Errorf("Expected invalidModels to contain 'totally-unknown-model', got %v", resp.InvalidModels)
	}
}

func TestHandleYAMLValidateMissingContent(t *testing.T) {
	server := newValidateTestServer(t)

	w := postValidate(t, server, ValidateRequest{})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("Expected status 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleYAMLValidateMethodNotAllowed(t *testing.T) {
	server := newValidateTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/yaml/validate", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("Expected status 405, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHealthEndpointReportsVersionAndCapabilities(t *testing.T) {
	server := newValidateTestServer(t)
	SetVersion("v1.2.3-test")
	defer SetVersion("")

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp HealthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Status != "ok" {
		t.Errorf("Expected status ok, got %s", resp.Status)
	}
	if resp.Version != "v1.2.3-test" {
		t.Errorf("Expected version v1.2.3-test, got %s", resp.Version)
	}

	want := []string{"full-dsl", "yaml-validate", "log-events", "progress-metrics", "stream-no-timeout"}
	caps := make(map[string]bool)
	for _, c := range resp.Capabilities {
		caps[c] = true
	}
	for _, c := range want {
		if !caps[c] {
			t.Errorf("Expected capability %q in %v", c, resp.Capabilities)
		}
	}
	if caps["openai-compat"] {
		t.Error("openai-compat capability should not be reported when disabled")
	}
}
