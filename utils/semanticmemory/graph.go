package semanticmemory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Graph node kinds.
const (
	GraphNodeComponent = "component"
	GraphNodePackage   = "package"
	GraphNodeFile      = "file"
	GraphNodeType      = "type"
	GraphNodeFunction  = "function"
	GraphNodeConcept   = "concept"
)

// Graph edge kinds.
const (
	GraphEdgeContains  = "contains"
	GraphEdgeBelongsTo = "belongs_to"
	GraphEdgeImports   = "imports"
	GraphEdgeDefines   = "defines"
	GraphEdgeUses      = "uses"
	GraphEdgeReference = "references"
)

// Graph edge confidence tags: extracted edges are explicit in the source,
// inferred edges come from name resolution or an LLM pass.
const (
	GraphConfidenceExtracted = "extracted"
	GraphConfidenceInferred  = "inferred"
)

// graphMirrorRecordPrefix is the ID prefix for memory records that mirror
// graph nodes into the FTS search path.
const graphMirrorRecordPrefix = "gnode_"

// graphMirrorSourceRef marks mirrored records so they can be cleaned up with
// the graph namespace that produced them.
func graphMirrorSourceRef(namespace string) string { return "graph/" + namespace }

// GraphNode is one vertex in a knowledge graph stored alongside durable memory.
type GraphNode struct {
	ID        string
	Namespace string
	Kind      string
	Name      string
	Path      string
	Package   string
	Summary   string
	Degree    int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// GraphEdge is one typed, confidence-tagged connection between two nodes.
type GraphEdge struct {
	ID         string
	Namespace  string
	SourceID   string
	TargetID   string
	Kind       string
	Confidence string
	Evidence   string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

var graphMigrateStatements = []string{
	`CREATE TABLE IF NOT EXISTS graph_nodes (
        id TEXT PRIMARY KEY,
        namespace TEXT NOT NULL,
        kind TEXT NOT NULL,
        name TEXT NOT NULL,
        path TEXT NOT NULL DEFAULT '',
        package TEXT NOT NULL DEFAULT '',
        summary TEXT NOT NULL DEFAULT '',
        degree INTEGER NOT NULL DEFAULT 0,
        created_at TEXT NOT NULL,
        updated_at TEXT NOT NULL
    )`,
	`CREATE INDEX IF NOT EXISTS idx_graph_nodes_namespace_name ON graph_nodes(namespace, name)`,
	`CREATE VIRTUAL TABLE IF NOT EXISTS graph_nodes_fts USING fts5(
        id UNINDEXED,
        namespace UNINDEXED,
        name,
        summary,
        tokenize='unicode61'
    )`,
	`CREATE TABLE IF NOT EXISTS graph_edges (
        id TEXT PRIMARY KEY,
        namespace TEXT NOT NULL,
        source_id TEXT NOT NULL,
        target_id TEXT NOT NULL,
        kind TEXT NOT NULL,
        confidence TEXT NOT NULL DEFAULT 'extracted',
        evidence TEXT NOT NULL DEFAULT '',
        created_at TEXT NOT NULL,
        updated_at TEXT NOT NULL
    )`,
	`CREATE INDEX IF NOT EXISTS idx_graph_edges_namespace_source ON graph_edges(namespace, source_id)`,
	`CREATE INDEX IF NOT EXISTS idx_graph_edges_namespace_target ON graph_edges(namespace, target_id)`,
}

// UpsertGraphNode writes a graph node, refreshes its FTS entry, and mirrors it
// as a memory record (type graph_node) so memory search and workflow recall
// can find graph concepts. Callers should pass a stable ID so rebuilds update
// in place instead of duplicating.
func (s *Store) UpsertGraphNode(ctx context.Context, node GraphNode) (GraphNode, error) {
	node.Namespace = normalizeNamespace(node.Namespace)
	node.Kind = strings.TrimSpace(strings.ToLower(node.Kind))
	node.Name = strings.TrimSpace(node.Name)
	if node.ID == "" {
		return GraphNode{}, fmt.Errorf("graph node ID is required")
	}
	if node.Name == "" {
		return GraphNode{}, fmt.Errorf("graph node name is required")
	}
	now := time.Now().UTC()
	if node.CreatedAt.IsZero() {
		node.CreatedAt = now
	}
	node.UpdatedAt = now

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return GraphNode{}, fmt.Errorf("start graph node transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `INSERT INTO graph_nodes (
        id, namespace, kind, name, path, package, summary, degree, created_at, updated_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    ON CONFLICT(id) DO UPDATE SET
        namespace=excluded.namespace, kind=excluded.kind, name=excluded.name,
        path=excluded.path, package=excluded.package, summary=excluded.summary,
        updated_at=excluded.updated_at`,
		node.ID, node.Namespace, node.Kind, node.Name, node.Path, node.Package, node.Summary,
		node.Degree, node.CreatedAt.Format(time.RFC3339Nano), node.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return GraphNode{}, fmt.Errorf("write graph node: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM graph_nodes_fts WHERE id = ?`, node.ID); err != nil {
		return GraphNode{}, fmt.Errorf("refresh graph node index: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO graph_nodes_fts(id, namespace, name, summary) VALUES (?, ?, ?, ?)`,
		node.ID, node.Namespace, node.Name, node.Summary); err != nil {
		return GraphNode{}, fmt.Errorf("index graph node: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return GraphNode{}, fmt.Errorf("commit graph node transaction: %w", err)
	}

	// Mirror into durable memory so FTS search/recall sees graph concepts.
	content := node.Name
	if node.Summary != "" {
		content = node.Name + " — " + node.Summary
	}
	if _, err := s.Upsert(ctx, Record{
		ID:        graphMirrorRecordPrefix + node.ID,
		Namespace: node.Namespace,
		Type:      "graph_node",
		Content:   content,
		SourceRef: graphMirrorSourceRef(node.Namespace),
	}); err != nil {
		return GraphNode{}, fmt.Errorf("mirror graph node into memory: %w", err)
	}
	return node, nil
}

// UpsertGraphEdge writes a graph edge. Callers should pass a stable ID derived
// from (source, target, kind) so rebuilds update in place.
func (s *Store) UpsertGraphEdge(ctx context.Context, edge GraphEdge) (GraphEdge, error) {
	edge.Namespace = normalizeNamespace(edge.Namespace)
	edge.Kind = strings.TrimSpace(strings.ToLower(edge.Kind))
	if edge.ID == "" {
		return GraphEdge{}, fmt.Errorf("graph edge ID is required")
	}
	if edge.SourceID == "" || edge.TargetID == "" {
		return GraphEdge{}, fmt.Errorf("graph edge source and target are required")
	}
	if edge.Confidence == "" {
		edge.Confidence = GraphConfidenceExtracted
	}
	now := time.Now().UTC()
	if edge.CreatedAt.IsZero() {
		edge.CreatedAt = now
	}
	edge.UpdatedAt = now

	_, err := s.db.ExecContext(ctx, `INSERT INTO graph_edges (
        id, namespace, source_id, target_id, kind, confidence, evidence, created_at, updated_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
    ON CONFLICT(id) DO UPDATE SET
        namespace=excluded.namespace, source_id=excluded.source_id, target_id=excluded.target_id,
        kind=excluded.kind, confidence=excluded.confidence, evidence=excluded.evidence,
        updated_at=excluded.updated_at`,
		edge.ID, edge.Namespace, edge.SourceID, edge.TargetID, edge.Kind, edge.Confidence, edge.Evidence,
		edge.CreatedAt.Format(time.RFC3339Nano), edge.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return GraphEdge{}, fmt.Errorf("write graph edge: %w", err)
	}
	return edge, nil
}

// RefreshGraphDegrees recomputes the degree column for every node in a
// namespace from the current edge set. Call once after a batch of edge writes.
func (s *Store) RefreshGraphDegrees(ctx context.Context, namespace string) error {
	namespace = normalizeNamespace(namespace)
	_, err := s.db.ExecContext(ctx, `UPDATE graph_nodes SET degree = (
        SELECT COUNT(*) FROM graph_edges
        WHERE graph_edges.namespace = graph_nodes.namespace
          AND (graph_edges.source_id = graph_nodes.id OR graph_edges.target_id = graph_nodes.id)
    ) WHERE namespace = ?`, namespace)
	if err != nil {
		return fmt.Errorf("refresh graph degrees: %w", err)
	}
	return nil
}

// GetGraphNode returns a graph node by ID.
func (s *Store) GetGraphNode(ctx context.Context, id string) (GraphNode, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, namespace, kind, name, path, package, summary, degree, created_at, updated_at
        FROM graph_nodes WHERE id = ?`, id)
	return scanGraphNode(row)
}

// GraphNodes returns all nodes in a namespace.
func (s *Store) GraphNodes(ctx context.Context, namespace string) ([]GraphNode, error) {
	namespace = normalizeNamespace(namespace)
	rows, err := s.db.QueryContext(ctx, `SELECT id, namespace, kind, name, path, package, summary, degree, created_at, updated_at
        FROM graph_nodes WHERE namespace = ? ORDER BY degree DESC, name`, namespace)
	if err != nil {
		return nil, fmt.Errorf("list graph nodes: %w", err)
	}
	defer rows.Close()
	return collectGraphNodes(rows)
}

// GraphEdges returns all edges in a namespace.
func (s *Store) GraphEdges(ctx context.Context, namespace string) ([]GraphEdge, error) {
	namespace = normalizeNamespace(namespace)
	rows, err := s.db.QueryContext(ctx, `SELECT id, namespace, source_id, target_id, kind, confidence, evidence, created_at, updated_at
        FROM graph_edges WHERE namespace = ?`, namespace)
	if err != nil {
		return nil, fmt.Errorf("list graph edges: %w", err)
	}
	defer rows.Close()
	return collectGraphEdges(rows)
}

// GraphNeighbors returns the edges touching a node (in either direction) and
// the nodes at the other end of those edges.
func (s *Store) GraphNeighbors(ctx context.Context, namespace, nodeID string) ([]GraphEdge, []GraphNode, error) {
	namespace = normalizeNamespace(namespace)
	rows, err := s.db.QueryContext(ctx, `SELECT id, namespace, source_id, target_id, kind, confidence, evidence, created_at, updated_at
        FROM graph_edges WHERE namespace = ? AND (source_id = ? OR target_id = ?)`, namespace, nodeID, nodeID)
	if err != nil {
		return nil, nil, fmt.Errorf("list graph neighbors: %w", err)
	}
	edges, err := collectGraphEdges(rows)
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	if len(edges) == 0 {
		return edges, nil, nil
	}

	seen := make(map[string]bool, len(edges))
	var ids []string
	for _, edge := range edges {
		other := edge.TargetID
		if other == nodeID {
			other = edge.SourceID
		}
		if !seen[other] {
			seen[other] = true
			ids = append(ids, other)
		}
	}
	idsJSON, err := json.Marshal(ids)
	if err != nil {
		return nil, nil, fmt.Errorf("encode neighbor IDs: %w", err)
	}
	nodeRows, err := s.db.QueryContext(ctx, `SELECT id, namespace, kind, name, path, package, summary, degree, created_at, updated_at
        FROM graph_nodes WHERE id IN (SELECT value FROM json_each(?))`, string(idsJSON))
	if err != nil {
		return nil, nil, fmt.Errorf("load graph neighbor nodes: %w", err)
	}
	defer nodeRows.Close()
	nodes, err := collectGraphNodes(nodeRows)
	if err != nil {
		return nil, nil, err
	}
	return edges, nodes, nil
}

// GraphFindNodes looks nodes up by name/summary with FTS5, degrading to a
// LIKE search when the query cannot be expressed as FTS syntax.
func (s *Store) GraphFindNodes(ctx context.Context, namespace, query string, limit int) ([]GraphNode, error) {
	namespace = normalizeNamespace(namespace)
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	ftsQuery := buildFTSQuery(query)
	if ftsQuery == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT n.id, n.namespace, n.kind, n.name, n.path, n.package, n.summary, n.degree, n.created_at, n.updated_at
        FROM graph_nodes_fts f JOIN graph_nodes n ON n.id = f.id
        WHERE graph_nodes_fts MATCH ? AND n.namespace = ?
        ORDER BY bm25(graph_nodes_fts), n.degree DESC LIMIT ?`, ftsQuery, namespace, limit)
	if err == nil {
		defer rows.Close()
		return collectGraphNodes(rows)
	}
	rows, fallbackErr := s.db.QueryContext(ctx, `SELECT id, namespace, kind, name, path, package, summary, degree, created_at, updated_at
        FROM graph_nodes WHERE namespace = ? AND (name LIKE ? OR summary LIKE ?)
        ORDER BY degree DESC, name LIMIT ?`, namespace, "%"+strings.TrimSpace(query)+"%", "%"+strings.TrimSpace(query)+"%", limit)
	if fallbackErr != nil {
		return nil, fmt.Errorf("find graph nodes: %w", err)
	}
	defer rows.Close()
	return collectGraphNodes(rows)
}

// DeleteGraphNamespace removes all nodes, edges, and mirrored memory records
// for a namespace.
func (s *Store) DeleteGraphNamespace(ctx context.Context, namespace string) error {
	namespace = normalizeNamespace(namespace)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("start graph delete transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	statements := []struct {
		query string
		args  []any
	}{
		{`DELETE FROM graph_edges WHERE namespace = ?`, []any{namespace}},
		{`DELETE FROM graph_nodes_fts WHERE namespace = ?`, []any{namespace}},
		{`DELETE FROM graph_nodes WHERE namespace = ?`, []any{namespace}},
		{`DELETE FROM memory_fts WHERE id IN (SELECT id FROM memories WHERE namespace = ? AND source_ref = ?)`,
			[]any{namespace, graphMirrorSourceRef(namespace)}},
		{`DELETE FROM memories WHERE namespace = ? AND source_ref = ?`, []any{namespace, graphMirrorSourceRef(namespace)}},
	}
	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, stmt.query, stmt.args...); err != nil {
			return fmt.Errorf("delete graph namespace: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit graph delete transaction: %w", err)
	}
	return nil
}

func scanGraphNode(row rowScanner) (GraphNode, error) {
	var node GraphNode
	var createdAt, updatedAt string
	if err := row.Scan(&node.ID, &node.Namespace, &node.Kind, &node.Name, &node.Path, &node.Package,
		&node.Summary, &node.Degree, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GraphNode{}, fmt.Errorf("graph node not found")
		}
		return GraphNode{}, err
	}
	node.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	node.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return node, nil
}

func collectGraphNodes(rows *sql.Rows) ([]GraphNode, error) {
	var nodes []GraphNode
	for rows.Next() {
		node, err := scanGraphNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nodes, nil
}

func scanGraphEdge(row rowScanner) (GraphEdge, error) {
	var edge GraphEdge
	var createdAt, updatedAt string
	if err := row.Scan(&edge.ID, &edge.Namespace, &edge.SourceID, &edge.TargetID, &edge.Kind,
		&edge.Confidence, &edge.Evidence, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return GraphEdge{}, fmt.Errorf("graph edge not found")
		}
		return GraphEdge{}, err
	}
	edge.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	edge.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return edge, nil
}

func collectGraphEdges(rows *sql.Rows) ([]GraphEdge, error) {
	var edges []GraphEdge
	for rows.Next() {
		edge, err := scanGraphEdge(rows)
		if err != nil {
			return nil, err
		}
		edges = append(edges, edge)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return edges, nil
}
