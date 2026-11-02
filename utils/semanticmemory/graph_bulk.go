package semanticmemory

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// GraphWriteProgress reports bounded milestones from a bulk graph replace.
type GraphWriteProgress struct {
	Phase     string
	Current   string
	Completed int
	Total     int
}

// ReplaceGraph atomically replaces one namespace using a single SQLite transaction.
func (s *Store) ReplaceGraph(ctx context.Context, namespace string, nodes []GraphNode, edges []GraphEdge, progress func(GraphWriteProgress)) error {
	return s.ReplaceGraphWithEvidence(ctx, namespace, nodes, edges, nil, progress)
}

// ReplaceGraphWithEvidence atomically replaces graph structure and all source
// citations for a namespace.
func (s *Store) ReplaceGraphWithEvidence(ctx context.Context, namespace string, nodes []GraphNode, edges []GraphEdge, evidence []GraphEvidence, progress func(GraphWriteProgress)) error {
	namespace = normalizeNamespace(namespace)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("start graph replacement transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if progress != nil {
		progress(GraphWriteProgress{Phase: "Removing stale graph data"})
	}
	for _, stmt := range []struct {
		query string
		args  []any
	}{
		{`DELETE FROM graph_evidence WHERE namespace = ?`, []any{namespace}},
		{`DELETE FROM graph_edges WHERE namespace = ?`, []any{namespace}},
		{`DELETE FROM graph_nodes_fts WHERE namespace = ?`, []any{namespace}},
		{`DELETE FROM graph_nodes WHERE namespace = ?`, []any{namespace}},
		{`DELETE FROM memory_fts WHERE id IN (SELECT id FROM memories WHERE namespace = ? AND source_ref = ?)`, []any{namespace, graphMirrorSourceRef(namespace)}},
		{`DELETE FROM memories WHERE namespace = ? AND source_ref = ?`, []any{namespace, graphMirrorSourceRef(namespace)}},
	} {
		if _, err := tx.ExecContext(ctx, stmt.query, stmt.args...); err != nil {
			return fmt.Errorf("delete graph namespace: %w", err)
		}
	}

	nodeStmt, err := tx.PrepareContext(ctx, `INSERT INTO graph_nodes (
        id, namespace, kind, name, path, package, summary, content, properties,
        start_line, end_line, degree, created_at, updated_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare graph node write: %w", err)
	}
	defer nodeStmt.Close()
	graphFTSStmt, err := tx.PrepareContext(ctx, `INSERT INTO graph_nodes_fts(id, namespace, name, summary, content) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare graph node index: %w", err)
	}
	defer graphFTSStmt.Close()
	memoryStmt, err := tx.PrepareContext(ctx, `INSERT INTO memories (
        id, namespace, type, priority, confidence, content, source_run_id, source_step, source_ref, created_at, updated_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare graph memory mirror: %w", err)
	}
	defer memoryStmt.Close()
	memoryFTSStmt, err := tx.PrepareContext(ctx, `INSERT INTO memory_fts(id, namespace, content) VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare graph memory index: %w", err)
	}
	defer memoryFTSStmt.Close()
	edgeStmt, err := tx.PrepareContext(ctx, `INSERT INTO graph_edges (
        id, namespace, source_id, target_id, kind, confidence, evidence, created_at, updated_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare graph edge write: %w", err)
	}
	defer edgeStmt.Close()
	evidenceStmt, err := tx.PrepareContext(ctx, `INSERT INTO graph_evidence (
        id, namespace, edge_id, source_path, start_line, end_line, quote,
        extractor, parser_version, created_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare graph evidence write: %w", err)
	}
	defer evidenceStmt.Close()

	total := len(nodes) + len(edges) + len(evidence)
	completed := 0
	report := func(phase, current string) {
		if progress != nil && (completed%100 == 0 || completed == total) {
			progress(GraphWriteProgress{Phase: phase, Current: current, Completed: completed, Total: total})
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, node := range nodes {
		node.Namespace = namespace
		node.Kind = strings.TrimSpace(strings.ToLower(node.Kind))
		node.Name = strings.TrimSpace(node.Name)
		if node.ID == "" || node.Name == "" {
			return fmt.Errorf("graph node ID and name are required")
		}
		if node.Properties == "" {
			node.Properties = "{}"
		}
		if _, err := nodeStmt.ExecContext(ctx, node.ID, namespace, node.Kind, node.Name, node.Path, node.Package,
			node.Summary, node.Content, node.Properties, node.StartLine, node.EndLine, 0, now, now); err != nil {
			return fmt.Errorf("write graph node: %w", err)
		}
		if _, err := graphFTSStmt.ExecContext(ctx, node.ID, namespace, node.Name, node.Summary, node.Content); err != nil {
			return fmt.Errorf("index graph node: %w", err)
		}
		content := graphNodeMemoryContent(node)
		mirrorID := graphMirrorRecordPrefix + node.ID
		if _, err := memoryStmt.ExecContext(ctx, mirrorID, namespace, "graph_node", 50, 1.0, content, "", "", graphMirrorSourceRef(namespace), now, now); err != nil {
			return fmt.Errorf("mirror graph node into memory: %w", err)
		}
		if _, err := memoryFTSStmt.ExecContext(ctx, mirrorID, namespace, content); err != nil {
			return fmt.Errorf("index graph memory mirror: %w", err)
		}
		completed++
		report("Writing graph nodes", node.Name)
	}
	edgeIDs := make(map[string]bool, len(edges))
	for _, edge := range edges {
		edge.Namespace = namespace
		edge.Kind = strings.TrimSpace(strings.ToLower(edge.Kind))
		if edge.ID == "" || edge.SourceID == "" || edge.TargetID == "" {
			return fmt.Errorf("graph edge ID, source, and target are required")
		}
		confidence := edge.Confidence
		if confidence == "" {
			confidence = GraphConfidenceExtracted
		}
		if _, err := edgeStmt.ExecContext(ctx, edge.ID, namespace, edge.SourceID, edge.TargetID, edge.Kind, confidence, edge.Evidence, now, now); err != nil {
			return fmt.Errorf("write graph edge: %w", err)
		}
		edgeIDs[edge.ID] = true
		completed++
		report("Writing graph edges", edge.Kind)
	}
	for _, item := range evidence {
		if item.ID == "" || !edgeIDs[item.EdgeID] {
			return fmt.Errorf("graph evidence requires an ID and valid edge")
		}
		if _, err := evidenceStmt.ExecContext(ctx, item.ID, namespace, item.EdgeID, item.SourcePath,
			item.StartLine, item.EndLine, item.Quote, item.Extractor, item.ParserVersion, now); err != nil {
			return fmt.Errorf("write graph evidence: %w", err)
		}
		completed++
		report("Writing graph evidence", item.SourcePath)
	}
	if progress != nil {
		progress(GraphWriteProgress{Phase: "Refreshing graph relationships", Completed: completed, Total: total})
	}
	if _, err := tx.ExecContext(ctx, `WITH endpoint_counts AS (
        SELECT source_id AS id, COUNT(*) AS degree FROM graph_edges WHERE namespace = ? GROUP BY source_id
        UNION ALL
        SELECT target_id AS id, COUNT(*) AS degree FROM graph_edges WHERE namespace = ? GROUP BY target_id
    ), degrees AS (
        SELECT id, SUM(degree) AS degree FROM endpoint_counts GROUP BY id
    )
    UPDATE graph_nodes
    SET degree = COALESCE((SELECT degree FROM degrees WHERE degrees.id = graph_nodes.id), 0)
    WHERE namespace = ?`, namespace, namespace, namespace); err != nil {
		return fmt.Errorf("refresh graph degrees: %w", err)
	}
	if progress != nil {
		progress(GraphWriteProgress{Phase: "Committing graph transaction", Completed: completed, Total: total})
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit graph replacement: %w", err)
	}
	return nil
}
