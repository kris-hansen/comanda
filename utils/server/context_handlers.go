package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/processor"
	"github.com/kris-hansen/comanda/utils/semanticmemory"
	"gopkg.in/yaml.v3"
)

const contextAPIVersion = 1

// ContextProject is a project that the server can resolve without accepting a
// client-provided absolute path. Today projects are backed by registered
// indexes; that gives an existing Comanda installation a useful, safe default.
type ContextProject struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	SourceRoot string `json:"source_root"`
	Available  bool   `json:"available"`
}

// ContextIndex is the Canvas-facing metadata for a registered codebase index.
type ContextIndex struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	SourceRoot  string `json:"source_root"`
	IndexPath   string `json:"index_path"`
	Status      string `json:"status"`
	LastIndexed string `json:"last_indexed,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
	FileCount   int    `json:"file_count"`
	SizeBytes   int64  `json:"size_bytes"`
	Encrypted   bool   `json:"encrypted"`
	Languages   string `json:"languages,omitempty"`
}

// ContextKnowledgeGraph describes the graph that belongs to an index/project.
type ContextKnowledgeGraph struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	IndexID   string `json:"index_id"`
	Status    string `json:"status"`
	NodeCount int    `json:"node_count"`
	EdgeCount int    `json:"edge_count"`
}

type ContextResponse struct {
	APIVersion      int                     `json:"api_version"`
	Capabilities    []string                `json:"capabilities"`
	Projects        []ContextProject        `json:"projects"`
	Indexes         []ContextIndex          `json:"indexes"`
	KnowledgeGraphs []ContextKnowledgeGraph `json:"knowledge_graphs"`
}

type PreflightRequest struct {
	Workflow   string `json:"workflow"`
	ProjectID  string `json:"project_id,omitempty"`
	RuntimeDir string `json:"runtime_dir,omitempty"`
}

type PreflightIssue struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

type PreflightResponse struct {
	Ready              bool             `json:"ready"`
	Project            *ContextProject  `json:"project,omitempty"`
	ResolvedSourceRoot string           `json:"resolved_source_root,omitempty"`
	RuntimeDir         string           `json:"runtime_dir,omitempty"`
	Issues             []PreflightIssue `json:"issues"`
}

func (s *Server) handleContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	response := s.contextInventory()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (s *Server) handlePreflight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	var request PreflightRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid preflight request")
		return
	}
	if strings.TrimSpace(request.Workflow) == "" {
		sendJSONError(w, http.StatusBadRequest, "workflow is required")
		return
	}

	response := s.preflight(request)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// handleValidate performs the same non-processing preflight as
// `comanda validate`, scoped to the server's runtime directory and optional
// registered project. It deliberately does not execute workflow actions.
func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	var request PreflightRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid validate request")
		return
	}
	if strings.TrimSpace(request.Workflow) == "" {
		sendJSONError(w, http.StatusBadRequest, "workflow is required")
		return
	}

	response := s.validate(request)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (s *Server) contextInventory() ContextResponse {
	response := ContextResponse{
		APIVersion:   contextAPIVersion,
		Capabilities: []string{"project-context", "codebase-indexes", "knowledge-graphs", "knowledge-graph-api", "run-preflight", "workflow-validate"},
	}
	if s.envConfig == nil {
		return response
	}

	names := make([]string, 0, len(s.envConfig.Indexes))
	for name := range s.envConfig.Indexes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entry := s.envConfig.Indexes[name]
		if entry == nil {
			continue
		}
		rootAvailable := isDirectory(entry.Path)
		indexAvailable := isRegularFile(entry.IndexPath)
		status := "ready"
		if !rootAvailable || !indexAvailable {
			status = "missing"
		}
		project := ContextProject{
			ID: name, Name: name, SourceRoot: entry.Path, Available: rootAvailable,
		}
		response.Projects = append(response.Projects, project)
		response.Indexes = append(response.Indexes, ContextIndex{
			ID: name, ProjectID: name, SourceRoot: entry.Path, IndexPath: entry.IndexPath,
			Status: status, LastIndexed: entry.LastIndexed, ContentHash: entry.ContentHash,
			FileCount: entry.FileCount, SizeBytes: entry.SizeBytes, Encrypted: entry.Encrypted,
			Languages: entry.Languages,
		})
		response.KnowledgeGraphs = append(response.KnowledgeGraphs, graphInventory(name, entry.Path))
	}
	return response
}

func graphInventory(indexName, root string) ContextKnowledgeGraph {
	graph := ContextKnowledgeGraph{ID: indexName, ProjectID: indexName, IndexID: indexName, Status: "missing"}
	dbPath := semanticmemory.DefaultPath(root, indexName)
	if !isRegularFile(dbPath) {
		return graph
	}
	store, err := semanticmemory.Open(dbPath)
	if err != nil {
		graph.Status = "unavailable"
		return graph
	}
	defer func() { _ = store.Close() }()
	nodes, nodeErr := store.GraphNodes(context.Background(), indexName)
	edges, edgeErr := store.GraphEdges(context.Background(), indexName)
	if nodeErr != nil || edgeErr != nil {
		graph.Status = "unavailable"
		return graph
	}
	graph.NodeCount = len(nodes)
	graph.EdgeCount = len(edges)
	if len(nodes) > 0 {
		graph.Status = "ready"
	}
	return graph
}

func (s *Server) preflight(request PreflightRequest) PreflightResponse {
	response := PreflightResponse{RuntimeDir: request.RuntimeDir}
	add := func(severity, code, message string) {
		response.Issues = append(response.Issues, PreflightIssue{Severity: severity, Code: code, Message: message})
	}

	var project *ContextProject
	if request.ProjectID != "" {
		resolved, err := s.resolveProject(request.ProjectID)
		if err != nil {
			add("error", "project_unavailable", err.Error())
		} else {
			project = &resolved
			response.Project = project
			response.ResolvedSourceRoot = project.SourceRoot
		}
	}

	validation := processor.ValidateWorkflowStructure(request.Workflow)
	for _, issue := range validation.Errors {
		add("error", "workflow_invalid", issue.Message)
	}

	var dsl processor.DSLConfig
	if err := yaml.Unmarshal([]byte(request.Workflow), &dsl); err != nil {
		add("error", "workflow_unreadable", fmt.Sprintf("Workflow YAML cannot be parsed: %v", err))
	} else {
		for _, step := range dsl.Steps {
			if ci := step.Config.CodebaseIndex; ci != nil {
				s.preflightIndexStep(add, project, ci)
			}
		}
	}

	if request.RuntimeDir != "" {
		if _, err := s.validatePath(filepath.Join(request.RuntimeDir, "workflow.yaml")); err != nil {
			add("error", "runtime_invalid", "Runtime directory is not safe for this server")
		}
	}
	response.Ready = true
	for _, issue := range response.Issues {
		if issue.Severity == "error" {
			response.Ready = false
			break
		}
	}
	return response
}

func (s *Server) validate(request PreflightRequest) PreflightResponse {
	response := s.preflight(request)
	add := func(severity, code, message string) {
		response.Issues = append(response.Issues, PreflightIssue{Severity: severity, Code: code, Message: message})
	}

	var dsl processor.DSLConfig
	if err := yaml.Unmarshal([]byte(request.Workflow), &dsl); err != nil {
		response.Ready = false
		return response
	}

	processorInstance := processor.NewProcessor(
		&dsl,
		s.envConfig,
		s.config,
		false,
		request.RuntimeDir,
	)
	if response.Project != nil {
		processorInstance.SetSourceRoot(response.Project.SourceRoot)
	}
	if err := processorInstance.Preflight(); err != nil {
		add("error", "workflow_not_runnable", err.Error())
	}
	response.Ready = true
	for _, issue := range response.Issues {
		if issue.Severity == "error" {
			response.Ready = false
			break
		}
	}
	return response
}

func (s *Server) preflightIndexStep(add func(string, string, string), project *ContextProject, index *processor.CodebaseIndexConfig) {
	if index.Use != nil {
		for _, name := range indexNames(index.Use) {
			entry, ok := s.envConfig.Indexes[name]
			if !ok || entry == nil || !isRegularFile(entry.IndexPath) {
				add("error", "index_missing", fmt.Sprintf("Codebase index %q is not available on this server", name))
			}
		}
	}
	if project != nil && index.Root != "" {
		root, err := processor.ResolveProjectPath(project.SourceRoot, index.Root)
		if err != nil {
			add("error", "source_invalid", err.Error())
			return
		}
		if !isDirectory(root) {
			add("error", "source_missing", fmt.Sprintf("Codebase root %q is not present in project %q", index.Root, project.Name))
		}
	}
}

func indexNames(use interface{}) []string {
	switch value := use.(type) {
	case string:
		return []string{value}
	case []string:
		return value
	case []interface{}:
		var names []string
		for _, item := range value {
			if name, ok := item.(string); ok {
				names = append(names, name)
			}
		}
		return names
	default:
		return nil
	}
}

func (s *Server) resolveProject(id string) (ContextProject, error) {
	return resolveProject(s.envConfig, id)
}

func resolveProject(envConfig *config.EnvConfig, id string) (ContextProject, error) {
	if envConfig == nil || envConfig.Indexes == nil {
		return ContextProject{}, fmt.Errorf("project %q is not registered", id)
	}
	entry, ok := envConfig.Indexes[id]
	if !ok || entry == nil {
		return ContextProject{}, fmt.Errorf("project %q is not registered", id)
	}
	if !isDirectory(entry.Path) {
		return ContextProject{}, fmt.Errorf("project %q source root is unavailable", id)
	}
	return ContextProject{ID: id, Name: id, SourceRoot: entry.Path, Available: true}, nil
}

func isDirectory(path string) bool {
	// Registered project roots originate in the server administrator's index
	// configuration, never in a request. The request can select only an index
	// name; resolveProject does not accept a filesystem path from the client.
	// lgtm [go/path-injection]
	info, err := os.Stat(filepath.Clean(path))
	return err == nil && info.IsDir()
}

func isRegularFile(path string) bool {
	info, err := os.Stat(filepath.Clean(path))
	return err == nil && !info.IsDir()
}

// projectSourceRoot deliberately accepts only a registered project ID. It is
// used by /process so a remote client cannot turn this API into arbitrary host
// filesystem access by supplying an absolute path.
func projectSourceRoot(envConfig *config.EnvConfig, id string) (string, error) {
	if id == "" {
		return "", nil
	}
	project, err := resolveProject(envConfig, id)
	if err != nil {
		return "", err
	}
	return project.SourceRoot, nil
}
