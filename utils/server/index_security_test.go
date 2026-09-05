package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
)

func TestYAMLProcessConfinesIndexRoots(t *testing.T) {
	for _, tc := range []struct {
		name, root, project string
		auth, allowed       bool
	}{
		{name: "outside absolute root", root: "outside", auth: true},
		{name: "relative traversal", root: "../outside", auth: true},
		{name: "outside root without authentication", root: "outside"},
		{name: "registered project escape", root: "outside", project: "approved", auth: true},
		{name: "default data directory", allowed: true, auth: true},
		{name: "default data directory without authentication", allowed: true},
		{name: "default registered project", project: "approved", allowed: true, auth: true},
		{name: "unknown project", project: "unknown", auth: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dataDir, projectDir, outside := t.TempDir(), t.TempDir(), t.TempDir()
			for _, dir := range []string{dataDir, projectDir, outside} {
				if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.test/project\ngo 1.25\n"), 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {}\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			root := tc.root
			if root == "outside" {
				root = outside
			}
			workflow := "index:\n  step_type: codebase-index\n"
			if root != "" {
				workflow += fmt.Sprintf("  codebase_index:\n    root: %q\n", root)
			} else {
				workflow += "  codebase_index: {}\n"
			}
			body, err := json.Marshal(map[string]any{"content": workflow})
			if err != nil {
				t.Fatal(err)
			}
			s := &Server{config: &config.ServerConfig{Enabled: tc.auth, BearerToken: "test-token", DataDir: dataDir}, envConfig: &config.EnvConfig{Indexes: map[string]*config.IndexEntry{"approved": {Path: projectDir}}}}
			req := httptest.NewRequest(http.MethodPost, "/yaml/process?runtimeDir=../../outside&project="+tc.project, bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer test-token")
			w := httptest.NewRecorder()
			s.handleYAMLProcess(w, req)
			var response ProcessResponse
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode response: %v: %s", err, w.Body.String())
			}
			if response.Success != tc.allowed {
				t.Fatalf("allowed=%v: status %d response %+v", tc.allowed, w.Code, response)
			}
			if !tc.allowed && tc.project != "unknown" && !strings.Contains(response.Error, "escapes the selected project root") {
				t.Fatalf("unexpected rejection: %s", response.Error)
			}
			if _, err := os.Stat(filepath.Join(outside, ".comanda")); !os.IsNotExist(err) {
				t.Fatalf("outside index was written: %v", err)
			}
			if tc.allowed {
				base := dataDir
				if tc.project != "" {
					base = projectDir
				}
				matches, err := filepath.Glob(filepath.Join(base, ".comanda", "*.meta.json"))
				if err != nil || len(matches) != 1 {
					t.Fatalf("legitimate index not generated: %v, %v", matches, err)
				}
			}
		})
	}
}
