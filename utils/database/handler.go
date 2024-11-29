package database

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/kris-hansen/comanda/utils/config"
	_ "github.com/lib/pq"
)

// Operation represents the type of database operation
type Operation int

const (
	ReadOperation Operation = iota
	WriteOperation
)

// Handler manages database connections and operations
type Handler struct {
	envConfig *config.EnvConfig
	dbs       map[string]*sql.DB
}

// NewHandler creates a new database handler
func NewHandler(envConfig *config.EnvConfig) *Handler {
	return &Handler{
		envConfig: envConfig,
		dbs:       make(map[string]*sql.DB),
	}
}

// ValidateOperation validates that the SQL statement matches the expected operation type
func (h *Handler) ValidateOperation(sql string, operation Operation) error {
	sql = strings.TrimSpace(strings.ToUpper(sql))

	// Regular expressions for different SQL operations
	selectRegex := regexp.MustCompile(`^SELECT\s+`)
	insertRegex := regexp.MustCompile(`^INSERT\s+INTO\s+`)
	updateRegex := regexp.MustCompile(`^UPDATE\s+`)
	deleteRegex := regexp.MustCompile(`^DELETE\s+FROM\s+`)

	switch operation {
	case ReadOperation:
		if !selectRegex.MatchString(sql) {
			return fmt.Errorf("expected SELECT statement for read operation, got: %s", sql)
		}
	case WriteOperation:
		if !insertRegex.MatchString(sql) && !updateRegex.MatchString(sql) && !deleteRegex.MatchString(sql) {
			return fmt.Errorf("expected INSERT/UPDATE/DELETE statement for write operation, got: %s", sql)
		}
	default:
		return fmt.Errorf("invalid database operation type")
	}

	return nil
}

// TestConnection attempts to establish a connection to the database and verify it works
func (h *Handler) TestConnection(dbName string) error {
	db, err := h.getConnection(dbName)
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}

	// Test the connection with a simple query
	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping test failed: %w", err)
	}

	return nil
}

// getConnection gets or creates a database connection
func (h *Handler) getConnection(dbName string) (*sql.DB, error) {
	// Check if connection already exists
	if db, exists := h.dbs[dbName]; exists {
		if err := db.Ping(); err == nil {
			return db, nil
		}
		// Connection is stale, remove it
		delete(h.dbs, dbName)
	}

	// Get database configuration
	dbConfig, err := h.envConfig.GetDatabaseConfig(dbName)
	if err != nil {
		return nil, fmt.Errorf("failed to get database config: %w", err)
	}

	// Create new connection
	db, err := sql.Open("postgres", dbConfig.GetConnectionString())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Store connection for reuse
	h.dbs[dbName] = db
	return db, nil
}

// ExecuteRead executes a read operation (SELECT) and returns the results
func (h *Handler) ExecuteRead(dbName string, query string) ([]map[string]interface{}, error) {
	if err := h.ValidateOperation(query, ReadOperation); err != nil {
		return nil, err
	}

	db, err := h.getConnection(dbName)
	if err != nil {
		return nil, err
	}

	// Execute query
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get column names: %w", err)
	}

	// Prepare result slice
	var result []map[string]interface{}

	// Prepare value holders
	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	// Iterate through rows
	for rows.Next() {
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Create map for this row
		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			if b, ok := val.([]byte); ok {
				// Convert []byte to string
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		result = append(result, row)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error during row iteration: %w", err)
	}

	return result, nil
}

// ExecuteWrite executes a write operation (INSERT/UPDATE/DELETE) and returns affected rows
func (h *Handler) ExecuteWrite(dbName string, query string) (int64, error) {
	if err := h.ValidateOperation(query, WriteOperation); err != nil {
		return 0, err
	}

	db, err := h.getConnection(dbName)
	if err != nil {
		return 0, err
	}

	// Execute query
	result, err := db.Exec(query)
	if err != nil {
		return 0, fmt.Errorf("failed to execute query: %w", err)
	}

	// Get number of affected rows
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get affected rows: %w", err)
	}

	return affected, nil
}

// Close closes all database connections
func (h *Handler) Close() error {
	var errors []string
	for name, db := range h.dbs {
		if err := db.Close(); err != nil {
			errors = append(errors, fmt.Sprintf("failed to close %s: %v", name, err))
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("errors closing databases: %s", strings.Join(errors, "; "))
	}
	return nil
}
