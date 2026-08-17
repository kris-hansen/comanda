package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/kris-hansen/comanda/utils/graphviewer"
	"github.com/kris-hansen/comanda/utils/knowledgegraph"
	"github.com/kris-hansen/comanda/utils/semanticmemory"
)

// graphQuerier opens only graphs registered in this server's Comanda config.
// This keeps the HTTP API from becoming a path-based database reader.
func (s *Server) graphQuerier(_ context.Context, namespace string) (*knowledgegraph.Querier, func() error, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return nil, nil, fmt.Errorf("namespace is required")
	}
	entry, ok := s.envConfig.Indexes[namespace]
	if !ok || entry == nil {
		return nil, nil, fmt.Errorf("no registered index named %q", namespace)
	}
	dbPath := semanticmemory.DefaultPath(entry.Path, namespace)
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("no graph exists for namespace %q. Build it with: comanda graph build %s", namespace, namespace)
		}
		return nil, nil, fmt.Errorf("inspect graph database: %w", err)
	}
	store, err := semanticmemory.Open(dbPath)
	if err != nil {
		return nil, nil, err
	}
	return knowledgegraph.NewQuerier(store, namespace), store.Close, nil
}

// handleGraphAPI maps the server's stable /graph endpoints to the same
// read-only navigation contract consumed by `comanda graph visualize`.
func (s *Server) handleGraphAPI(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/graph")
	clone := r.Clone(r.Context())
	clone.URL.Path = path
	graphviewer.NewAPI(s.graphQuerier).ServeHTTP(w, clone)
}
