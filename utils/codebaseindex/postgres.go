package codebaseindex

// PostgresAdapter recognizes PostgreSQL DDL in repository .sql files without
// requiring a parser plugin manifest. It extracts a logical schema model
// (schemas, tables, columns, primary/unique/foreign keys) as a SemanticGraph
// so the knowledge graph builder can persist navigable database
// relationships alongside code structure. Unsupported or malformed SQL is
// skipped conservatively; extraction never fails a scan.
type PostgresAdapter struct{}

func (a *PostgresAdapter) Name() string             { return "postgresql" }
func (a *PostgresAdapter) DetectionFiles() []string { return nil }
func (a *PostgresAdapter) FileExtensions() []string { return []string{".sql"} }
func (a *PostgresAdapter) IgnoreDirs() []string     { return nil }
func (a *PostgresAdapter) IgnoreGlobs() []string    { return nil }
func (a *PostgresAdapter) EntrypointPatterns() []string {
	return []string{"schema.sql", "init.sql"}
}
func (a *PostgresAdapter) ConfigPatterns() []string { return nil }

func (a *PostgresAdapter) ScoreFile(_ string, depth int, isEntrypoint, isConfig bool) int {
	score := 0
	if isEntrypoint {
		score += 40
	}
	if isConfig {
		score += 30
	}
	if depth <= 2 {
		score += 60
	}
	return score
}

func (a *PostgresAdapter) ExtractSymbols(path string, content []byte) (*SymbolInfo, error) {
	return extractPostgresSymbols(path, content)
}

// extractPostgresSymbols never returns an error: DDL extraction is
// best-effort, and files with no recognizable schema DDL (ordinary queries,
// seed data, unsupported statements) simply produce an empty SymbolInfo.
func extractPostgresSymbols(path string, content []byte) (*SymbolInfo, error) {
	acc := parsePostgresDDL(string(content))
	graph := buildPostgresSemanticGraph(path, acc)
	info := &SymbolInfo{SemanticGraph: graph}
	return info, nil
}
