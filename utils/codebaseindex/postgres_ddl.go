package codebaseindex

import (
	"fmt"
	"sort"
	"strings"
)

// This file implements a tolerant, best-effort Postgres DDL reader. It is not
// a general SQL parser: it recognizes CREATE SCHEMA, CREATE TABLE, and
// ALTER TABLE ... ADD CONSTRAINT statements well enough to build a logical
// schema model, and conservatively skips everything else (other DDL, DML,
// and any construct it does not recognize) without failing extraction.

// pgTokKind identifies the lexical class of a pgToken.
type pgTokKind int

const (
	pgTokAtom pgTokKind = iota
	pgTokQuotedIdent
	pgTokString
	pgTokPunct
)

// pgToken is one lexical unit of Postgres DDL. Start/End are byte offsets
// into the original source, used to slice raw type/default expressions
// without lossy reconstruction from token text.
type pgToken struct {
	Kind  pgTokKind
	Text  string // atom/punct: raw text; quotedIdent: unescaped identifier; string: raw literal including quotes
	Start int
	End   int
	Line  int
}

// tokenizePostgres scans Postgres DDL into tokens, skipping whitespace and
// comments. It never returns an error: malformed input (an unterminated
// string, a truncated dollar-quoted block) degrades to consuming the rest of
// the input as part of the current token rather than panicking or looping.
func tokenizePostgres(src string) []pgToken {
	var tokens []pgToken
	n := len(src)
	i := 0
	line := 1
	advanceLines := func(s string) {
		line += strings.Count(s, "\n")
	}
	for i < n {
		c := src[i]
		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r' || c == '\v' || c == '\f':
			i++
		case c == '-' && i+1 < n && src[i+1] == '-':
			j := strings.IndexByte(src[i:], '\n')
			if j < 0 {
				i = n
			} else {
				i += j
			}
		case c == '/' && i+1 < n && src[i+1] == '*':
			depth := 1
			j := i + 2
			for j < n && depth > 0 {
				if j+1 < n && src[j] == '/' && src[j+1] == '*' {
					depth++
					j += 2
					continue
				}
				if j+1 < n && src[j] == '*' && src[j+1] == '/' {
					depth--
					j += 2
					continue
				}
				if src[j] == '\n' {
					line++
				}
				j++
			}
			i = j
		case c == '\'':
			start := i
			startLine := line
			i++
			for i < n {
				if src[i] == '\'' {
					if i+1 < n && src[i+1] == '\'' {
						i += 2
						continue
					}
					i++
					break
				}
				if src[i] == '\n' {
					line++
				}
				i++
			}
			tokens = append(tokens, pgToken{Kind: pgTokString, Text: src[start:i], Start: start, End: i, Line: startLine})
		case c == '"':
			start := i
			startLine := line
			i++
			var b strings.Builder
			for i < n {
				if src[i] == '"' {
					if i+1 < n && src[i+1] == '"' {
						b.WriteByte('"')
						i += 2
						continue
					}
					i++
					break
				}
				if src[i] == '\n' {
					line++
				}
				b.WriteByte(src[i])
				i++
			}
			tokens = append(tokens, pgToken{Kind: pgTokQuotedIdent, Text: b.String(), Start: start, End: i, Line: startLine})
		case c == '$':
			start := i
			startLine := line
			tagStart := i + 1
			j := tagStart
			if j < n && (isPgLetter(src[j]) || src[j] == '_') {
				j++
				for j < n && isPgIdentContinue(src[j]) {
					j++
				}
			}
			if j < n && src[j] == '$' {
				closer := "$" + src[tagStart:j] + "$"
				bodyStart := j + 1
				k := strings.Index(src[bodyStart:], closer)
				var end int
				if k < 0 {
					end = n
				} else {
					end = bodyStart + k + len(closer)
				}
				advanceLines(src[start:end])
				tokens = append(tokens, pgToken{Kind: pgTokString, Text: src[start:end], Start: start, End: end, Line: startLine})
				i = end
			} else {
				tokens = append(tokens, pgToken{Kind: pgTokPunct, Text: "$", Start: i, End: i + 1, Line: line})
				i++
			}
		case isPgLetter(c) || c == '_':
			start := i
			i++
			for i < n && isPgIdentContinue(src[i]) {
				i++
			}
			tokens = append(tokens, pgToken{Kind: pgTokAtom, Text: src[start:i], Start: start, End: i, Line: line})
		case c >= '0' && c <= '9':
			start := i
			i++
			for i < n && (src[i] >= '0' && src[i] <= '9' || src[i] == '.') {
				i++
			}
			if i < n && (src[i] == 'e' || src[i] == 'E') {
				k := i + 1
				if k < n && (src[k] == '+' || src[k] == '-') {
					k++
				}
				if k < n && src[k] >= '0' && src[k] <= '9' {
					for k < n && src[k] >= '0' && src[k] <= '9' {
						k++
					}
					i = k
				}
			}
			tokens = append(tokens, pgToken{Kind: pgTokAtom, Text: src[start:i], Start: start, End: i, Line: line})
		default:
			tokens = append(tokens, pgToken{Kind: pgTokPunct, Text: string(c), Start: i, End: i + 1, Line: line})
			i++
		}
	}
	return tokens
}

func isPgLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isPgIdentContinue(c byte) bool {
	return isPgLetter(c) || (c >= '0' && c <= '9') || c == '_' || c == '$'
}

// pgStatement is one semicolon-delimited statement's tokens plus its starting
// line, used for evidence.
type pgStatement struct {
	Tokens []pgToken
	Line   int
}

// splitPostgresStatements groups tokens into statements on top-level ';'
// punctuation (outside of parentheses; strings/comments are never tokenized
// as separate tokens so they cannot contribute a spurious terminator).
func splitPostgresStatements(tokens []pgToken) []pgStatement {
	var statements []pgStatement
	var current []pgToken
	depth := 0
	for _, tok := range tokens {
		if tok.Kind == pgTokPunct {
			switch tok.Text {
			case "(":
				depth++
			case ")":
				if depth > 0 {
					depth--
				}
			case ";":
				if depth == 0 {
					if len(current) > 0 {
						statements = append(statements, pgStatement{Tokens: current, Line: current[0].Line})
					}
					current = nil
					continue
				}
			}
		}
		current = append(current, tok)
	}
	if len(current) > 0 {
		statements = append(statements, pgStatement{Tokens: current, Line: current[0].Line})
	}
	return statements
}

// pgIdent is a single identifier as written, preserving whether it was
// double-quoted (case-sensitive, no folding) or bare (folded to lowercase).
type pgIdent struct {
	Raw    string
	Quoted bool
}

func (id pgIdent) normalize() (display, key string) {
	if id.Quoted {
		return id.Raw, id.Raw
	}
	return strings.ToLower(id.Raw), strings.ToLower(id.Raw)
}

// pgQualifiedName is a possibly schema-qualified SQL name.
type pgQualifiedName struct {
	Schema         pgIdent
	Name           pgIdent
	HasSchema      bool
	SchemaKey      string
	SchemaDisplay  string
	NameKey        string
	NameDisplay    string
	QualifiedKey   string // schemaKey.nameKey, always populated (defaults schema to "public")
	QualifiedShown string // display form; schema-qualified only when explicit
}

func resolveQualifiedName(schema, name pgIdent, hasSchema bool) pgQualifiedName {
	q := pgQualifiedName{Schema: schema, Name: name, HasSchema: hasSchema}
	q.NameDisplay, q.NameKey = name.normalize()
	if hasSchema {
		q.SchemaDisplay, q.SchemaKey = schema.normalize()
	} else {
		q.SchemaDisplay, q.SchemaKey = "public", "public"
	}
	q.QualifiedKey = q.SchemaKey + "." + q.NameKey
	if hasSchema {
		q.QualifiedShown = q.SchemaDisplay + "." + q.NameDisplay
	} else {
		q.QualifiedShown = q.NameDisplay
	}
	return q
}

// pgColumn is a parsed column definition.
type pgColumn struct {
	Name        pgIdent
	NameDisplay string
	NameKey     string
	Type        string
	NotNull     bool
	HasDefault  bool
	Default     string
	InlinePK    bool
	InlineUniq  bool
	FK          *pgInlineFK
	Line        int
}

type pgInlineFK struct {
	Target     pgQualifiedName
	TargetCols []pgIdent // may be empty when the reference omits an explicit column list
	Line       int
}

// pgConstraint is a table-level (or promoted inline) key constraint.
type pgConstraint struct {
	Name    string // "" when unnamed
	Kind    string // "primary_key", "unique", "foreign_key"
	Columns []pgIdent
	FK      *pgInlineFK
	Line    int
}

// pgTable accumulates one CREATE TABLE's logical structure.
type pgTable struct {
	Qualified   pgQualifiedName
	Columns     []pgColumn
	Constraints []pgConstraint
	Line        int
}

// pgAlterConstraint captures one ALTER TABLE ... ADD [CONSTRAINT] action.
type pgAlterConstraint struct {
	Table      pgQualifiedName
	Constraint pgConstraint
}

// pgSchemaDecl captures an explicit CREATE SCHEMA statement.
type pgSchemaDecl struct {
	Qualified pgQualifiedName
	Line      int
}

// pgAccumulator holds everything extracted from one file's statements.
type pgAccumulator struct {
	Schemas []pgSchemaDecl
	Tables  []pgTable
	Alters  []pgAlterConstraint
}

var pgColumnConstraintKeywords = map[string]bool{
	"NOT": true, "NULL": true, "DEFAULT": true, "PRIMARY": true, "UNIQUE": true,
	"REFERENCES": true, "CHECK": true, "COLLATE": true, "CONSTRAINT": true,
	"GENERATED": true, "DEFERRABLE": true, "INITIALLY": true,
}

func atomEq(tok pgToken, kw string) bool {
	return tok.Kind == pgTokAtom && strings.EqualFold(tok.Text, kw)
}

func punctEq(tok pgToken, p string) bool {
	return tok.Kind == pgTokPunct && tok.Text == p
}

// parsePostgresDDL extracts a best-effort logical schema model from Postgres
// DDL source. It never returns an error: unrecognized or malformed statements
// are conservatively skipped.
func parsePostgresDDL(src string) *pgAccumulator {
	acc := &pgAccumulator{}
	tokens := tokenizePostgres(src)
	for _, stmt := range splitPostgresStatements(tokens) {
		parsePostgresStatement(acc, stmt, src)
	}
	return acc
}

func parsePostgresStatement(acc *pgAccumulator, stmt pgStatement, src string) {
	toks := stmt.Tokens
	if len(toks) == 0 {
		return
	}
	i := 0
	switch {
	case atomEq(toks[i], "CREATE"):
		parseCreateStatement(acc, toks, i+1, stmt.Line, src)
	case atomEq(toks[i], "ALTER") && i+1 < len(toks) && atomEq(toks[i+1], "TABLE"):
		parseAlterTable(acc, toks, i+2, stmt.Line)
	default:
		// DML and every other statement kind are intentionally skipped.
	}
}

// parseCreateStatement dispatches CREATE SCHEMA / CREATE TABLE and otherwise
// conservatively ignores the statement.
func parseCreateStatement(acc *pgAccumulator, toks []pgToken, i int, line int, src string) {
	// Skip optional table modifiers that can precede TABLE.
	for i < len(toks) && toks[i].Kind == pgTokAtom {
		switch strings.ToUpper(toks[i].Text) {
		case "GLOBAL", "LOCAL", "TEMP", "TEMPORARY", "UNLOGGED", "OR", "REPLACE":
			i++
			continue
		}
		break
	}
	if i >= len(toks) {
		return
	}
	switch {
	case atomEq(toks[i], "SCHEMA"):
		i++
		i = skipIfNotExists(toks, i)
		if i < len(toks) && atomEq(toks[i], "AUTHORIZATION") {
			return // "CREATE SCHEMA AUTHORIZATION role" with no explicit name: skip.
		}
		if i < len(toks) && (toks[i].Kind == pgTokAtom || toks[i].Kind == pgTokQuotedIdent) {
			name := identFromToken(toks[i])
			q := resolveQualifiedName(pgIdent{}, name, false)
			acc.Schemas = append(acc.Schemas, pgSchemaDecl{Qualified: q, Line: line})
		}
	case atomEq(toks[i], "TABLE"):
		i++
		i = skipIfNotExists(toks, i)
		var qname pgQualifiedName
		qname, i = parseQualifiedName(toks, i)
		if i >= len(toks) || !punctEq(toks[i], "(") {
			return // typed/partition tables and other forms are not supported; skip.
		}
		body, _ := extractParenGroup(toks, i)
		table := pgTable{Qualified: qname, Line: line}
		parseTableBody(&table, body, src)
		acc.Tables = append(acc.Tables, table)
	default:
		// CREATE INDEX/VIEW/FUNCTION/TYPE/EXTENSION/... are out of scope.
	}
}

func skipIfNotExists(toks []pgToken, i int) int {
	if i < len(toks) && atomEq(toks[i], "IF") && i+2 < len(toks) && atomEq(toks[i+1], "NOT") && atomEq(toks[i+2], "EXISTS") {
		return i + 3
	}
	return i
}

func identFromToken(tok pgToken) pgIdent {
	return pgIdent{Raw: tok.Text, Quoted: tok.Kind == pgTokQuotedIdent}
}

// parseQualifiedName reads `name` or `schema.name` starting at i.
func parseQualifiedName(toks []pgToken, i int) (pgQualifiedName, int) {
	if i >= len(toks) || (toks[i].Kind != pgTokAtom && toks[i].Kind != pgTokQuotedIdent) {
		return pgQualifiedName{}, i
	}
	first := identFromToken(toks[i])
	i++
	if i < len(toks) && punctEq(toks[i], ".") && i+1 < len(toks) && (toks[i+1].Kind == pgTokAtom || toks[i+1].Kind == pgTokQuotedIdent) {
		second := identFromToken(toks[i+1])
		return resolveQualifiedName(first, second, true), i + 2
	}
	return resolveQualifiedName(pgIdent{}, first, false), i
}

// extractParenGroup returns the tokens strictly inside the matching
// parenthesis starting at toks[i] (which must be "(") and the index just
// after the matching ")".
func extractParenGroup(toks []pgToken, i int) ([]pgToken, int) {
	if i >= len(toks) || !punctEq(toks[i], "(") {
		return nil, i
	}
	depth := 1
	j := i + 1
	for j < len(toks) && depth > 0 {
		if punctEq(toks[j], "(") {
			depth++
		} else if punctEq(toks[j], ")") {
			depth--
			if depth == 0 {
				break
			}
		}
		j++
	}
	inner := toks[i+1 : j]
	after := j + 1
	if j >= len(toks) {
		after = len(toks) // unterminated group: consume the rest conservatively.
	}
	return inner, after
}

// splitTopLevel splits tokens on a top-level punctuation separator,
// respecting nested parentheses.
func splitTopLevel(toks []pgToken, sep string) [][]pgToken {
	var groups [][]pgToken
	var current []pgToken
	depth := 0
	for _, tok := range toks {
		if tok.Kind == pgTokPunct {
			if tok.Text == "(" {
				depth++
			} else if tok.Text == ")" {
				if depth > 0 {
					depth--
				}
			} else if tok.Text == sep && depth == 0 {
				groups = append(groups, current)
				current = nil
				continue
			}
		}
		current = append(current, tok)
	}
	groups = append(groups, current)
	return groups
}

func parseTableBody(table *pgTable, body []pgToken, src string) {
	for _, group := range splitTopLevel(body, ",") {
		group = trimTokens(group)
		if len(group) == 0 {
			continue
		}
		if constraint, ok := parseTableLevelConstraint(group); ok {
			table.Constraints = append(table.Constraints, constraint)
			continue
		}
		if isSkippedTableItem(group[0]) {
			continue
		}
		if col, ok := parseColumnDef(group, src); ok {
			table.Columns = append(table.Columns, col)
		}
	}
}

func isSkippedTableItem(first pgToken) bool {
	switch {
	case atomEq(first, "CHECK"), atomEq(first, "EXCLUDE"), atomEq(first, "LIKE"):
		return true
	}
	return false
}

// parseTableLevelConstraint recognizes (optionally named) PRIMARY KEY,
// UNIQUE, and FOREIGN KEY table-level constraints.
func parseTableLevelConstraint(group []pgToken) (pgConstraint, bool) {
	i := 0
	name := ""
	if atomEq(group[i], "CONSTRAINT") && i+1 < len(group) {
		name = tokenIdentText(group[i+1])
		i += 2
	}
	if i >= len(group) {
		return pgConstraint{}, false
	}
	line := group[0].Line
	switch {
	case atomEq(group[i], "PRIMARY") && i+1 < len(group) && atomEq(group[i+1], "KEY"):
		cols, _ := columnsAfterParen(group, i+2)
		return pgConstraint{Name: name, Kind: "primary_key", Columns: cols, Line: line}, true
	case atomEq(group[i], "UNIQUE"):
		cols, _ := columnsAfterParen(group, i+1)
		return pgConstraint{Name: name, Kind: "unique", Columns: cols, Line: line}, true
	case atomEq(group[i], "FOREIGN") && i+1 < len(group) && atomEq(group[i+1], "KEY"):
		cols, next := columnsAfterParen(group, i+2)
		fk, ok := parseReferences(group, next)
		if !ok {
			return pgConstraint{}, false
		}
		return pgConstraint{Name: name, Kind: "foreign_key", Columns: cols, FK: fk, Line: line}, true
	}
	if name != "" {
		// A named constraint we don't otherwise recognize (e.g. CHECK): skip.
		return pgConstraint{}, false
	}
	return pgConstraint{}, false
}

func tokenIdentText(tok pgToken) string {
	if tok.Kind == pgTokAtom || tok.Kind == pgTokQuotedIdent {
		return tok.Text
	}
	return ""
}

// columnsAfterParen expects tok[i] == "(" and returns the column identifiers
// inside plus the index just past the closing paren.
func columnsAfterParen(toks []pgToken, i int) ([]pgIdent, int) {
	if i >= len(toks) || !punctEq(toks[i], "(") {
		return nil, i
	}
	inner, after := extractParenGroup(toks, i)
	var cols []pgIdent
	for _, group := range splitTopLevel(inner, ",") {
		group = trimTokens(group)
		if len(group) == 0 {
			continue
		}
		if group[0].Kind == pgTokAtom || group[0].Kind == pgTokQuotedIdent {
			cols = append(cols, identFromToken(group[0]))
		}
	}
	return cols, after
}

// parseReferences expects tok[i] == "REFERENCES" and reads the target table,
// optional column list, and trailing clauses (MATCH/ON DELETE/ON
// UPDATE/DEFERRABLE/...), which are consumed but not otherwise interpreted.
func parseReferences(toks []pgToken, i int) (*pgInlineFK, bool) {
	if i >= len(toks) || !atomEq(toks[i], "REFERENCES") {
		return nil, false
	}
	i++
	target, i := parseQualifiedName(toks, i)
	if target.NameKey == "" {
		return nil, false
	}
	var targetCols []pgIdent
	if i < len(toks) && punctEq(toks[i], "(") {
		targetCols, _ = columnsAfterParen(toks, i)
	}
	line := 0
	if len(toks) > 0 {
		line = toks[0].Line
	}
	return &pgInlineFK{Target: target, TargetCols: targetCols, Line: line}, true
}

// parseColumnDef parses `name type [column_constraints...]`.
func parseColumnDef(group []pgToken, src string) (pgColumn, bool) {
	if group[0].Kind != pgTokAtom && group[0].Kind != pgTokQuotedIdent {
		return pgColumn{}, false
	}
	name := identFromToken(group[0])
	display, key := name.normalize()
	col := pgColumn{Name: name, NameDisplay: display, NameKey: key, Line: group[0].Line}

	i := 1
	typeStart := -1
	typeEnd := -1
	for i < len(group) {
		tok := group[i]
		if punctEq(tok, "(") {
			_, after := extractParenGroup(group, i)
			if typeStart < 0 {
				typeStart = tok.Start
			}
			if after-1 >= 0 && after-1 < len(group) {
				typeEnd = group[after-1].End
			} else if len(group) > 0 {
				typeEnd = group[len(group)-1].End
			}
			i = after
			continue
		}
		if punctEq(tok, "[") {
			j := i + 1
			for j < len(group) && !punctEq(group[j], "]") {
				j++
			}
			if typeStart < 0 {
				typeStart = tok.Start
			}
			if j < len(group) {
				typeEnd = group[j].End
				i = j + 1
			} else {
				typeEnd = group[len(group)-1].End
				i = len(group)
			}
			continue
		}
		if tok.Kind == pgTokAtom && pgColumnConstraintKeywords[strings.ToUpper(tok.Text)] {
			break
		}
		if typeStart < 0 {
			typeStart = tok.Start
		}
		typeEnd = tok.End
		i++
	}
	if typeStart >= 0 && typeEnd > typeStart {
		col.Type = strings.ToLower(collapseSpace(src[typeStart:typeEnd]))
	}

	for i < len(group) {
		tok := group[i]
		switch {
		case atomEq(tok, "NOT") && i+1 < len(group) && atomEq(group[i+1], "NULL"):
			col.NotNull = true
			i += 2
		case atomEq(tok, "NULL"):
			col.NotNull = false
			i++
		case atomEq(tok, "DEFAULT"):
			i++
			start := -1
			end := -1
			for i < len(group) {
				t := group[i]
				if punctEq(t, "(") {
					_, after := extractParenGroup(group, i)
					if start < 0 {
						start = t.Start
					}
					if after-1 >= 0 && after-1 < len(group) {
						end = group[after-1].End
					}
					i = after
					continue
				}
				if t.Kind == pgTokAtom && t.Text != "" && strings.ToUpper(t.Text) != "NULL" && pgColumnConstraintKeywords[strings.ToUpper(t.Text)] {
					break
				}
				if start < 0 {
					start = t.Start
				}
				end = t.End
				i++
			}
			if start >= 0 && end > start {
				col.HasDefault = true
				col.Default = collapseSpace(src[start:end])
			}
		case atomEq(tok, "PRIMARY") && i+1 < len(group) && atomEq(group[i+1], "KEY"):
			col.InlinePK = true
			i += 2
		case atomEq(tok, "UNIQUE"):
			col.InlineUniq = true
			i++
		case atomEq(tok, "REFERENCES"):
			fk, ok := parseReferences(group, i)
			if ok {
				fk.Line = col.Line
				col.FK = fk
			}
			i = len(group) // trailing MATCH/ON DELETE/ON UPDATE clauses are not parsed further.
		case atomEq(tok, "CHECK"):
			i++
			if i < len(group) && punctEq(group[i], "(") {
				_, after := extractParenGroup(group, i)
				i = after
			}
		case atomEq(tok, "COLLATE"):
			i++
			if i < len(group) {
				_, i = parseQualifiedName(group, i)
			}
		case atomEq(tok, "CONSTRAINT"):
			i += 2 // skip inline constraint name; the following keyword is handled next loop.
		case atomEq(tok, "GENERATED"):
			i = len(group) // generated-column details are out of scope; stop parsing constraints.
		default:
			i++
		}
	}
	return col, true
}

func trimTokens(toks []pgToken) []pgToken {
	start, end := 0, len(toks)
	for start < end && toks[start].Kind == pgTokPunct && strings.TrimSpace(toks[start].Text) == "" {
		start++
	}
	return toks[start:end]
}

func collapseSpace(s string) string {
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

// parseAlterTable reads ALTER TABLE [IF EXISTS] [ONLY] name action[, action...]
// and records only the ADD CONSTRAINT / ADD PRIMARY KEY / ADD UNIQUE / ADD
// FOREIGN KEY forms; every other action (ADD COLUMN, DROP ..., ALTER COLUMN,
// RENAME ..., OWNER TO, ...) is conservatively skipped.
func parseAlterTable(acc *pgAccumulator, toks []pgToken, i int, stmtLine int) {
	i = skipIfExists(toks, i)
	if i < len(toks) && atomEq(toks[i], "ONLY") {
		i++
	}
	var table pgQualifiedName
	table, i = parseQualifiedName(toks, i)
	if table.NameKey == "" {
		return
	}
	for _, action := range splitTopLevel(toks[i:], ",") {
		action = trimTokens(action)
		if len(action) == 0 || !atomEq(action[0], "ADD") {
			continue
		}
		line := stmtLine
		if action[0].Line > 0 {
			line = action[0].Line
		}
		j := 1
		name := ""
		if j < len(action) && atomEq(action[j], "CONSTRAINT") && j+1 < len(action) {
			name = tokenIdentText(action[j+1])
			j += 2
		}
		if j >= len(action) {
			continue
		}
		switch {
		case atomEq(action[j], "PRIMARY") && j+1 < len(action) && atomEq(action[j+1], "KEY"):
			cols, _ := columnsAfterParen(action, j+2)
			acc.Alters = append(acc.Alters, pgAlterConstraint{Table: table, Constraint: pgConstraint{Name: name, Kind: "primary_key", Columns: cols, Line: line}})
		case atomEq(action[j], "UNIQUE"):
			cols, _ := columnsAfterParen(action, j+1)
			acc.Alters = append(acc.Alters, pgAlterConstraint{Table: table, Constraint: pgConstraint{Name: name, Kind: "unique", Columns: cols, Line: line}})
		case atomEq(action[j], "FOREIGN") && j+1 < len(action) && atomEq(action[j+1], "KEY"):
			cols, next := columnsAfterParen(action, j+2)
			fk, ok := parseReferences(action, next)
			if !ok {
				continue
			}
			fk.Line = line
			acc.Alters = append(acc.Alters, pgAlterConstraint{Table: table, Constraint: pgConstraint{Name: name, Kind: "foreign_key", Columns: cols, FK: fk, Line: line}})
		default:
			// ADD COLUMN and anything else: not modeled.
		}
	}
}

func skipIfExists(toks []pgToken, i int) int {
	if i < len(toks) && atomEq(toks[i], "IF") && i+1 < len(toks) && atomEq(toks[i+1], "EXISTS") {
		return i + 2
	}
	return i
}

// --- Semantic graph construction -------------------------------------------------

// semBuilder assembles one file's SemanticEntity/SemanticRelation lists,
// deduplicating entity declarations by ID (first declaration in file order
// wins, matching the project-scope contract's cross-file merge semantics).
type semBuilder struct {
	path      string
	entities  []SemanticEntity
	declared  map[string]bool
	relations []SemanticRelation
}

func newSemBuilder(path string) *semBuilder {
	return &semBuilder{path: path, declared: make(map[string]bool)}
}

func (b *semBuilder) ensure(id, kind, name, summary string) {
	if b.declared[id] {
		return
	}
	b.declared[id] = true
	b.entities = append(b.entities, SemanticEntity{ID: id, Kind: kind, Name: name, Scope: "project", Summary: summary})
}

func (b *semBuilder) relate(source, target, kind, evidence string) {
	if source == "" || target == "" || source == target {
		return
	}
	b.relations = append(b.relations, SemanticRelation{Source: source, Target: target, Kind: kind, Confidence: "extracted", Evidence: evidence})
}

func schemaEntityID(schemaKey string) string   { return "schema:" + schemaKey }
func tableEntityID(qualifiedKey string) string { return "table:" + qualifiedKey }
func columnEntityID(qualifiedKey, colKey string) string {
	return "column:" + qualifiedKey + "." + colKey
}

// ensureSchema declares a schema entity only when the table name was
// explicitly schema-qualified in the source (schemas are otherwise not
// "explicitly present" per the extraction contract) and links it to the
// table with a `contains` edge.
func (b *semBuilder) ensureSchemaContainsTable(q pgQualifiedName, tableID string) {
	if !q.HasSchema {
		return
	}
	schemaID := schemaEntityID(q.SchemaKey)
	b.ensure(schemaID, "schema", q.SchemaDisplay, "")
	b.relate(schemaID, tableID, "contains", fmt.Sprintf("%s: schema %s", b.path, q.SchemaDisplay))
}

// ensureTablePlaceholder declares a table entity with only its qualified
// name, used when a file references a table it does not itself define.
func (b *semBuilder) ensureTablePlaceholder(q pgQualifiedName) string {
	id := tableEntityID(q.QualifiedKey)
	b.ensure(id, "table", q.QualifiedShown, "")
	b.ensureSchemaContainsTable(q, id)
	return id
}

func (b *semBuilder) ensureColumnPlaceholder(q pgQualifiedName, tableID string, col pgIdent) string {
	display, key := col.normalize()
	id := columnEntityID(q.QualifiedKey, key)
	b.ensure(id, "column", q.QualifiedShown+"."+display, "")
	b.relate(tableID, id, "contains", fmt.Sprintf("%s: column %s.%s", b.path, q.QualifiedShown, display))
	return id
}

func columnSummary(col pgColumn) string {
	var parts []string
	if col.Type != "" {
		parts = append(parts, col.Type)
	}
	if col.NotNull {
		parts = append(parts, "not null")
	}
	if col.HasDefault {
		parts = append(parts, "default "+col.Default)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ", ")
}

// buildPostgresSemanticGraph converts one file's parsed accumulator into the
// SemanticGraph contract shared with parser plugins (see semantic.go).
// Evidence always carries "path:line" provenance back to the source DDL.
func buildPostgresSemanticGraph(path string, acc *pgAccumulator) *SemanticGraph {
	b := newSemBuilder(path)

	for _, s := range acc.Schemas {
		id := schemaEntityID(s.Qualified.NameKey)
		b.ensure(id, "schema", s.Qualified.NameDisplay, "")
	}

	for _, table := range acc.Tables {
		tableID := tableEntityID(table.Qualified.QualifiedKey)
		summary := fmt.Sprintf("%d columns", len(table.Columns))
		b.ensure(tableID, "table", table.Qualified.QualifiedShown, summary)
		b.ensureSchemaContainsTable(table.Qualified, tableID)

		colIDs := make(map[string]string, len(table.Columns))
		for _, col := range table.Columns {
			colID := columnEntityID(table.Qualified.QualifiedKey, col.NameKey)
			b.ensure(colID, "column", table.Qualified.QualifiedShown+"."+col.NameDisplay, columnSummary(col))
			colIDs[col.NameKey] = colID
			evidence := fmt.Sprintf("%s:%d: column %s", path, col.Line, col.NameDisplay)
			b.relate(tableID, colID, "contains", evidence)
			if col.InlinePK {
				b.relate(tableID, colID, "primary_key", evidence)
			}
			if col.InlineUniq {
				b.relate(tableID, colID, "unique", evidence)
			}
			if col.FK != nil {
				linkForeignKey(b, table.Qualified, tableID, []string{colID}, *col.FK, path)
			}
		}
		for _, c := range table.Constraints {
			applyTableConstraint(b, table.Qualified, tableID, colIDs, c, path)
		}
	}

	for _, alter := range acc.Alters {
		tableID := b.ensureTablePlaceholder(alter.Table)
		colIDs := map[string]string{}
		for _, col := range alter.Constraint.Columns {
			_, key := col.normalize()
			colIDs[key] = b.ensureColumnPlaceholder(alter.Table, tableID, col)
		}
		applyTableConstraint(b, alter.Table, tableID, colIDs, alter.Constraint, path)
	}

	if len(b.entities) == 0 {
		return nil
	}
	sort.SliceStable(b.relations, func(i, j int) bool {
		if b.relations[i].Source != b.relations[j].Source {
			return b.relations[i].Source < b.relations[j].Source
		}
		if b.relations[i].Target != b.relations[j].Target {
			return b.relations[i].Target < b.relations[j].Target
		}
		return b.relations[i].Kind < b.relations[j].Kind
	})
	return &SemanticGraph{Entities: b.entities, Relations: b.relations}
}

func applyTableConstraint(b *semBuilder, table pgQualifiedName, tableID string, colIDs map[string]string, c pgConstraint, path string) {
	name := c.Name
	if name == "" {
		name = "unnamed"
	}
	switch c.Kind {
	case "primary_key":
		for _, col := range c.Columns {
			id := resolveColumnID(b, table, tableID, colIDs, col)
			b.relate(tableID, id, "primary_key", fmt.Sprintf("%s:%d: constraint %s primary key", path, c.Line, name))
		}
	case "unique":
		for _, col := range c.Columns {
			id := resolveColumnID(b, table, tableID, colIDs, col)
			b.relate(tableID, id, "unique", fmt.Sprintf("%s:%d: constraint %s unique", path, c.Line, name))
		}
	case "foreign_key":
		if c.FK == nil {
			return
		}
		colIDList := make([]string, 0, len(c.Columns))
		for _, col := range c.Columns {
			colIDList = append(colIDList, resolveColumnID(b, table, tableID, colIDs, col))
		}
		linkForeignKey(b, table, tableID, colIDList, *c.FK, path)
	}
}

func resolveColumnID(b *semBuilder, table pgQualifiedName, tableID string, colIDs map[string]string, col pgIdent) string {
	_, key := col.normalize()
	if id, ok := colIDs[key]; ok {
		return id
	}
	return b.ensureColumnPlaceholder(table, tableID, col)
}

func linkForeignKey(b *semBuilder, sourceTable pgQualifiedName, sourceTableID string, sourceColIDs []string, fk pgInlineFK, path string) {
	targetTableID := b.ensureTablePlaceholder(fk.Target)
	evidence := fmt.Sprintf("%s:%d: foreign key %s -> %s", path, fk.Line, sourceTable.QualifiedShown, fk.Target.QualifiedShown)
	b.relate(sourceTableID, targetTableID, "foreign_key", evidence)
	if len(fk.TargetCols) == 0 || len(fk.TargetCols) != len(sourceColIDs) {
		return
	}
	for idx, targetCol := range fk.TargetCols {
		targetColID := b.ensureColumnPlaceholder(fk.Target, targetTableID, targetCol)
		b.relate(sourceColIDs[idx], targetColID, "references", evidence)
	}
}
