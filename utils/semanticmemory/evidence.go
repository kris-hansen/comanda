package semanticmemory

import (
	"context"
	"fmt"
	"time"
)

// UpsertGraphEvidence saves one source citation without discarding other
// evidence for the same edge.
func (s *Store) UpsertGraphEvidence(ctx context.Context, evidence GraphEvidence) (GraphEvidence, error) {
	if evidence.ID == "" || evidence.EdgeID == "" {
		return GraphEvidence{}, fmt.Errorf("evidence ID and edge ID are required")
	}
	evidence.Namespace = normalizeNamespace(evidence.Namespace)
	evidence.CreatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `INSERT INTO graph_evidence (
        id, namespace, edge_id, source_path, start_line, end_line, quote,
        extractor, parser_version, created_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    ON CONFLICT(id) DO UPDATE SET source_path=excluded.source_path,
        start_line=excluded.start_line, end_line=excluded.end_line,
        quote=excluded.quote, extractor=excluded.extractor,
        parser_version=excluded.parser_version`,
		evidence.ID, evidence.Namespace, evidence.EdgeID, evidence.SourcePath,
		evidence.StartLine, evidence.EndLine, evidence.Quote, evidence.Extractor,
		evidence.ParserVersion, evidence.CreatedAt.Format(time.RFC3339Nano))
	return evidence, err
}

// GraphEvidence returns every cited passage in a namespace in stable order.
func (s *Store) GraphEvidence(ctx context.Context, namespace string) ([]GraphEvidence, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, namespace, edge_id, source_path,
        start_line, end_line, quote, extractor, parser_version, created_at
        FROM graph_evidence WHERE namespace = ? ORDER BY edge_id, id`, normalizeNamespace(namespace))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var evidence []GraphEvidence
	for rows.Next() {
		var item GraphEvidence
		var createdAt string
		if err := rows.Scan(&item.ID, &item.Namespace, &item.EdgeID, &item.SourcePath,
			&item.StartLine, &item.EndLine, &item.Quote, &item.Extractor,
			&item.ParserVersion, &createdAt); err != nil {
			return nil, err
		}
		item.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		evidence = append(evidence, item)
	}
	return evidence, rows.Err()
}
