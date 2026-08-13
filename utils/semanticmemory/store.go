// Package semanticmemory provides a local, inspectable durable-memory store
// for Comanda workflows. It intentionally uses SQLite FTS5 as its first
// retrieval engine so memory remains local, portable, and easy to debug.
package semanticmemory

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"modernc.org/sqlite"
)

const defaultNamespace = "default"

// Record is one durable, independently understandable fact. Source fields
// preserve a route back to the workflow run or artifact that established it.
type Record struct {
	ID          string
	Namespace   string
	Type        string
	Priority    int
	Confidence  float64
	Content     string
	SourceRunID string
	SourceStep  string
	SourceRef   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// SearchOptions bounds and filters a memory lookup.
type SearchOptions struct {
	Namespace string
	Types     []string
	Limit     int
}

// Store owns an SQLite database and its FTS index.
type Store struct {
	db *sql.DB
}

// Open creates or opens a durable memory store at path.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("memory database path is required")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open memory database: %w", err)
	}
	// Workflows may have parallel recall steps. WAL plus a bounded busy wait
	// keeps a schema migration or write from turning that into a spurious
	// "database is locked" workflow failure.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		db.Close()
		return nil, configureMemoryDatabaseError(err)
	}
	if _, err := db.Exec(`PRAGMA journal_mode = WAL`); err != nil {
		db.Close()
		return nil, configureMemoryDatabaseError(err)
	}
	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func configureMemoryDatabaseError(err error) error {
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) && sqliteErr.Code()&0xff == 14 {
		// SQLite result code 14 is SQLITE_CANTOPEN. Never report it as an
		// allocation failure: that sends users looking for memory pressure when
		// the actual issue is a missing or inaccessible database path.
		return fmt.Errorf("configure memory database: unable to open database file (SQLITE_CANTOPEN, code 14)")
	}
	return fmt.Errorf("configure memory database: %w", err)
}

// Close releases the database connection.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS memories (
            id TEXT PRIMARY KEY,
            namespace TEXT NOT NULL,
            type TEXT NOT NULL,
            priority INTEGER NOT NULL DEFAULT 50,
            confidence REAL NOT NULL DEFAULT 1.0,
            content TEXT NOT NULL,
            source_run_id TEXT NOT NULL DEFAULT '',
            source_step TEXT NOT NULL DEFAULT '',
            source_ref TEXT NOT NULL DEFAULT '',
            created_at TEXT NOT NULL,
            updated_at TEXT NOT NULL
        )`,
		`CREATE INDEX IF NOT EXISTS idx_memories_namespace_updated ON memories(namespace, updated_at DESC)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
            id UNINDEXED,
            namespace UNINDEXED,
            content,
            tokenize='unicode61'
        )`,
	}
	statements = append(statements, graphMigrateStatements...)
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize memory store: %w", err)
		}
	}
	return nil
}

// Upsert writes a fact and refreshes its FTS entry. Callers can pass a stable
// ID to update an existing fact; otherwise a new durable ID is generated.
func (s *Store) Upsert(ctx context.Context, record Record) (Record, error) {
	record.Namespace = normalizeNamespace(record.Namespace)
	record.Type = normalizeType(record.Type)
	record.Content = strings.TrimSpace(record.Content)
	if record.Content == "" {
		return Record{}, fmt.Errorf("memory content is required")
	}
	if record.ID == "" {
		id, err := newID()
		if err != nil {
			return Record{}, err
		}
		record.ID = id
	}
	if record.Priority == 0 {
		record.Priority = 50
	}
	if record.Confidence == 0 {
		record.Confidence = 1
	}
	now := time.Now().UTC()
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}
	record.UpdatedAt = now

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Record{}, fmt.Errorf("start memory transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `INSERT INTO memories (
        id, namespace, type, priority, confidence, content, source_run_id, source_step, source_ref, created_at, updated_at
    ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
    ON CONFLICT(id) DO UPDATE SET
        namespace=excluded.namespace, type=excluded.type, priority=excluded.priority,
        confidence=excluded.confidence, content=excluded.content, source_run_id=excluded.source_run_id,
        source_step=excluded.source_step, source_ref=excluded.source_ref, updated_at=excluded.updated_at`,
		record.ID, record.Namespace, record.Type, record.Priority, record.Confidence, record.Content,
		record.SourceRunID, record.SourceStep, record.SourceRef,
		record.CreatedAt.Format(time.RFC3339Nano), record.UpdatedAt.Format(time.RFC3339Nano))
	if err != nil {
		return Record{}, fmt.Errorf("write memory: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM memory_fts WHERE id = ?`, record.ID); err != nil {
		return Record{}, fmt.Errorf("refresh memory index: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO memory_fts(id, namespace, content) VALUES (?, ?, ?)`, record.ID, record.Namespace, record.Content); err != nil {
		return Record{}, fmt.Errorf("index memory: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Record{}, fmt.Errorf("commit memory transaction: %w", err)
	}
	return record, nil
}

// Get returns a memory by its durable ID.
func (s *Store) Get(ctx context.Context, id string) (Record, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, namespace, type, priority, confidence, content, source_run_id, source_step, source_ref, created_at, updated_at FROM memories WHERE id = ?`, id)
	return scanRecord(row)
}

// Search performs an FTS5 lookup. If a query cannot be expressed safely as
// FTS syntax, it degrades to a scoped LIKE search instead of failing a run.
func (s *Store) Search(ctx context.Context, query string, options SearchOptions) ([]Record, error) {
	options.Namespace = normalizeNamespace(options.Namespace)
	if options.Limit <= 0 {
		options.Limit = 5
	}
	if options.Limit > 50 {
		options.Limit = 50
	}
	ftsQuery := buildFTSQuery(query)
	if ftsQuery == "" {
		return nil, nil
	}

	typesJSON, err := encodedTypes(options.Types)
	if err != nil {
		return nil, fmt.Errorf("encode memory type filter: %w", err)
	}
	const ftsSearchQuery = `SELECT m.id, m.namespace, m.type, m.priority, m.confidence, m.content, m.source_run_id, m.source_step, m.source_ref, m.created_at, m.updated_at
        FROM memory_fts f JOIN memories m ON m.id = f.id
        WHERE memory_fts MATCH ? AND m.namespace = ?
          AND (? = '[]' OR m.type IN (SELECT value FROM json_each(?)))
        ORDER BY bm25(memory_fts), m.priority DESC, m.updated_at DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, ftsSearchQuery, ftsQuery, options.Namespace, typesJSON, typesJSON, options.Limit)
	if err == nil {
		defer rows.Close()
		return collect(rows)
	}

	const fallbackSearchQuery = `SELECT m.id, m.namespace, m.type, m.priority, m.confidence, m.content, m.source_run_id, m.source_step, m.source_ref, m.created_at, m.updated_at
        FROM memories m WHERE m.content LIKE ? AND m.namespace = ?
          AND (? = '[]' OR m.type IN (SELECT value FROM json_each(?)))
        ORDER BY m.priority DESC, m.updated_at DESC LIMIT ?`
	rows, fallbackErr := s.db.QueryContext(ctx, fallbackSearchQuery, "%"+strings.TrimSpace(query)+"%", options.Namespace, typesJSON, typesJSON, options.Limit)
	if fallbackErr != nil {
		return nil, fmt.Errorf("search memory: %w", err)
	}
	defer rows.Close()
	return collect(rows)
}

func encodedTypes(types []string) (string, error) {
	normalized := make([]string, 0, len(types))
	for _, memoryType := range types {
		normalized = append(normalized, normalizeType(memoryType))
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func collect(rows *sql.Rows) ([]Record, error) {
	var records []Record
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRecord(row rowScanner) (Record, error) {
	var record Record
	var createdAt, updatedAt string
	if err := row.Scan(&record.ID, &record.Namespace, &record.Type, &record.Priority, &record.Confidence, &record.Content,
		&record.SourceRunID, &record.SourceStep, &record.SourceRef, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Record{}, fmt.Errorf("memory not found")
		}
		return Record{}, err
	}
	record.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	record.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return record, nil
}

func normalizeNamespace(namespace string) string {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return defaultNamespace
	}
	return namespace
}

func normalizeType(memoryType string) string {
	memoryType = strings.TrimSpace(strings.ToLower(memoryType))
	if memoryType == "" {
		return "finding"
	}
	return memoryType
}

var queryWord = regexp.MustCompile(`[\pL\pN_]{2,}`)

func buildFTSQuery(query string) string {
	words := queryWord.FindAllString(strings.ToLower(query), -1)
	if len(words) == 0 {
		return ""
	}
	// Input-derived recall queries are often full sentences. OR keeps one
	// incidental word from turning a useful retrieval into an empty result;
	// FTS5's BM25 rank still favors records matching the most terms.
	return strings.Join(words, " OR ")
}

func newID() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate memory ID: %w", err)
	}
	return "mem_" + hex.EncodeToString(bytes), nil
}

// DefaultPath returns the project-local database path for a namespace.
func DefaultPath(root, namespace string) string {
	namespace = normalizeNamespace(namespace)
	safe := regexp.MustCompile(`[^a-zA-Z0-9._-]+`).ReplaceAllString(namespace, "-")
	return filepath.Join(root, ".comanda", "memory", safe+".db")
}
