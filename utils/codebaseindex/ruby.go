package codebaseindex

import (
	"path/filepath"
	"strings"
)

// RubyAdapter indexes Ruby source without requiring a Ruby installation or
// executing project code. Extraction is lexical, like the other script adapters.
type RubyAdapter struct{}

func (a *RubyAdapter) Name() string { return "ruby" }
func (a *RubyAdapter) DetectionFiles() []string {
	return []string{"Gemfile", "Rakefile", ".ruby-version", "config.ru"}
}
func (a *RubyAdapter) FileExtensions() []string {
	return []string{".rb", ".rake", ".gemspec", ".ru"}
}
func (a *RubyAdapter) IgnoreDirs() []string {
	return []string{"vendor", ".bundle", "coverage", "tmp", "log"}
}
func (a *RubyAdapter) IgnoreGlobs() []string { return []string{"*.gem"} }
func (a *RubyAdapter) EntrypointPatterns() []string {
	return []string{"app.rb", "config.ru", "Rakefile", "config/application.rb", "config/environment.rb"}
}
func (a *RubyAdapter) ConfigPatterns() []string {
	return []string{"Gemfile", "Gemfile.lock", "Rakefile", ".ruby-version", "*.gemspec"}
}

// MatchesFilename supplies source language ownership for extensionless DSLs.
func (a *RubyAdapter) MatchesFilename(name string) bool {
	return name == "Gemfile" || name == "Rakefile"
}
func (a *RubyAdapter) ScoreFile(_ string, depth int, isEntrypoint, isConfig bool) int {
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
func (a *RubyAdapter) ExtractSymbols(path string, content []byte) (*SymbolInfo, error) {
	return extractRubySymbols(path, content), nil
}

type rubyToken struct {
	text       string
	start, end int
	literal    bool
	static     bool
}

// rubyTokens hides literal bodies and comments while retaining byte positions
// for signatures. Newlines and semicolons delimit statements; quoted strings
// remain single tokens so only literal require/gem arguments become imports.
func rubyTokens(source string) []rubyToken {
	var tokens []rubyToken
	var heredocs []string
	for i := 0; i < len(source); {
		start := i
		c := source[i]
		if (i == 0 || source[i-1] == '\n') && strings.TrimSuffix(rubyLine(source[i:]), "\r") == "__END__" {
			break
		}
		if (i == 0 || source[i-1] == '\n') && rubyBlockCommentStart(rubyLine(source[i:])) {
			for i < len(source) {
				line := rubyLine(source[i:])
				i += len(line)
				if i < len(source) {
					i++
				}
				if strings.TrimSpace(line) == "=end" {
					break
				}
			}
			continue
		}
		switch {
		case c == '\n':
			tokens = append(tokens, rubyToken{text: "\n", start: i, end: i + 1})
			i++
			for _, delimiter := range heredocs {
				for i < len(source) {
					line := rubyLine(source[i:])
					i += len(line)
					if i < len(source) {
						i++
					}
					if strings.TrimSpace(line) == delimiter {
						break
					}
				}
			}
			heredocs = nil
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '#':
			i += len(rubyLine(source[i:]))
		case c == '\'' || c == '"' || c == '`':
			i = rubyLiteralEnd(source, i+1, c, c)
			text := source[start+1 : i]
			if len(text) > 0 && text[len(text)-1] == c {
				text = text[:len(text)-1]
			}
			tokens = append(tokens, rubyToken{text: text, start: start, end: i, literal: true, static: c != '`'})
		case c == '%' && rubyPercentOffset(source[i:]) > 0:
			offset := rubyPercentOffset(source[i:])
			open := source[i+offset]
			close := rubyClosing(open)
			i = rubyLiteralEnd(source, i+offset+1, open, close)
			text := source[start+offset+1 : i]
			if len(text) > 0 && text[len(text)-1] == close {
				text = text[:len(text)-1]
			}
			tokens = append(tokens, rubyToken{text: text, start: start, end: i, literal: true, static: offset == 1 || source[start+1] == 'q' || source[start+1] == 'Q'})
		case c == '/' && rubyRegexStart(tokens):
			i = rubyRegexEnd(source, i+1)
			tokens = append(tokens, rubyToken{start: start, end: i, literal: true})
		case c == '<' && i+1 < len(source) && source[i+1] == '<':
			delimiter, end := rubyHeredoc(source, i+2)
			if len(tokens) > 0 && tokens[len(tokens)-1].text == "class" {
				delimiter = ""
			}
			if delimiter != "" {
				heredocs = append(heredocs, delimiter)
				i = end
				tokens = append(tokens, rubyToken{text: "", start: start, end: i, literal: true})
			} else {
				tokens = append(tokens, rubyToken{text: "<<", start: i, end: i + 2})
				i += 2
			}
		case rubyWord(c):
			for i < len(source) && rubyWord(source[i]) {
				i++
			}
			if i < len(source) && (source[i] == '?' || source[i] == '!') {
				i++
			}
			tokens = append(tokens, rubyToken{text: source[start:i], start: start, end: i})
		default:
			i++
			if i < len(source) && strings.Contains(" :: ** == != <= >= => && || =~ !~ ", " "+source[start:i+1]+" ") {
				i++
			}
			tokens = append(tokens, rubyToken{text: source[start:i], start: start, end: i})
		}
	}
	return tokens
}

func rubyBlockCommentStart(line string) bool {
	return line == "=begin" || strings.HasPrefix(line, "=begin ") || strings.HasPrefix(line, "=begin\t") || line == "=begin\r"
}

func rubyPercentOffset(s string) int {
	if len(s) > 1 && strings.ContainsRune("([{<", rune(s[1])) {
		return 1
	}
	if len(s) > 2 && strings.ContainsRune("qQwWiIxrs", rune(s[1])) && !rubyWord(s[2]) && !strings.ContainsRune(" \t\r\n", rune(s[2])) {
		return 2
	}
	return 0
}
func rubyRegexStart(tokens []rubyToken) bool {
	if len(tokens) == 0 {
		return true
	}
	t := tokens[len(tokens)-1]
	return !t.literal && strings.Contains(" = ( [ , =~ !~ return when ; \n ", " "+t.text+" ")
}
func rubyRegexEnd(s string, i int) int {
	inClass := false
	for i < len(s) {
		c := s[i]
		i++
		if c == '\\' && i < len(s) {
			i++
		} else if c == '[' {
			inClass = true
		} else if c == ']' {
			inClass = false
		} else if c == '/' && !inClass {
			break
		}
	}
	return i
}

func rubyLine(s string) string {
	if end := strings.IndexByte(s, '\n'); end >= 0 {
		return s[:end]
	}
	return s
}
func rubyWord(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c >= 128
}
func rubyClosing(c byte) byte {
	switch c {
	case '(':
		return ')'
	case '[':
		return ']'
	case '{':
		return '}'
	case '<':
		return '>'
	}
	return c
}
func rubyLiteralEnd(s string, i int, open, close byte) int {
	depth := 1
	for i < len(s) {
		c := s[i]
		i++
		if c == '\\' && i < len(s) {
			i++
			continue
		}
		if c == close {
			depth--
			if depth == 0 {
				break
			}
		} else if open != close && c == open {
			depth++
		}
	}
	return i
}
func rubyHeredoc(s string, i int) (string, int) {
	if i < len(s) && (s[i] == '-' || s[i] == '~') {
		i++
	}
	start := i
	quote := byte(0)
	if i < len(s) && (s[i] == '\'' || s[i] == '"' || s[i] == '`') {
		quote = s[i]
		i++
		start = i
	}
	// Unquoted identifiers conventionally use uppercase; this avoids treating
	// common shifts (value << count) and class << self as heredocs.
	if i == len(s) || (quote == 0 && (s[i] < 'A' || s[i] > 'Z')) {
		return "", start
	}
	for i < len(s) && rubyWord(s[i]) {
		i++
	}
	name := s[start:i]
	if quote != 0 {
		if i == len(s) || s[i] != quote {
			return "", start
		}
		i++
	}
	return name, i
}

type rubyScope struct {
	kind, name string
	private    bool
	typeIndex  int
}
type rubyExtractor struct {
	source string
	tokens []rubyToken
	info   *SymbolInfo
	scopes []rubyScope
}

func extractRubySymbols(path string, content []byte) *SymbolInfo {
	p := &rubyExtractor{source: string(content), info: &SymbolInfo{Package: filepath.ToSlash(filepath.Dir(path))}}
	p.tokens = rubyTokens(p.source)
	statementStart := true
	loopHeader := false
	for i := 0; i < len(p.tokens); i++ {
		t := p.tokens[i]
		if t.literal {
			statementStart = false
			continue
		}
		// Keyword spellings can also be method names, symbols, or hash keys.
		label := p.text(i+1) == ":" && p.tokens[i+1].start == t.end
		if p.text(i-1) == "." || p.text(i-1) == "::" || p.text(i-1) == ":" || label {
			statementStart = false
			continue
		}
		switch t.text {
		case "\n", ";", "then", "else":
			statementStart = true
			loopHeader = false
			continue
		case "end":
			if len(p.scopes) > 0 {
				p.scopes = p.scopes[:len(p.scopes)-1]
			}
		case "class", "module":
			i = p.declaration(i)
		case "def":
			i = p.method(i)
		case "do":
			if !loopHeader {
				p.scopes = append(p.scopes, rubyScope{kind: "block"})
			}
			loopHeader = false
		case "begin", "case", "for":
			p.scopes = append(p.scopes, rubyScope{kind: "block"})
			loopHeader = t.text == "for"
		case "if", "unless", "while", "until":
			if statementStart || p.text(i-1) == "=" || p.text(i-1) == "(" {
				p.scopes = append(p.scopes, rubyScope{kind: "block"})
				loopHeader = t.text == "while" || t.text == "until"
			}
		case "require", "require_relative", "load", "gem":
			if statementStart {
				p.importAt(i)
			}
		case "private", "protected", "public":
			if statementStart {
				p.visibility(i)
			}
		case "attr_reader", "attr_writer", "attr_accessor":
			if statementStart {
				p.attributes(i)
			}
		default:
			if statementStart && len(t.text) > 0 && t.text[0] >= 'A' && t.text[0] <= 'Z' && p.text(i+1) == "=" {
				p.info.Constants = appendUnique(p.info.Constants, p.qualify(t.text))
			}
		}
		statementStart = false
	}
	p.detectFrameworks()
	return p.info
}

func (p *rubyExtractor) text(i int) string {
	if i < 0 || i >= len(p.tokens) || p.tokens[i].literal {
		return ""
	}
	return p.tokens[i].text
}
func (p *rubyExtractor) owner() *rubyScope {
	for i := len(p.scopes) - 1; i >= 0; i-- {
		if p.scopes[i].kind == "class" || p.scopes[i].kind == "module" || p.scopes[i].kind == "singleton" {
			return &p.scopes[i]
		}
	}
	return nil
}
func (p *rubyExtractor) qualify(name string) string {
	if strings.HasPrefix(name, "::") {
		return strings.TrimPrefix(name, "::")
	}
	if owner := p.owner(); owner != nil && owner.name != "" {
		return owner.name + "::" + name
	}
	return name
}
func (p *rubyExtractor) declaration(i int) int {
	kind := p.text(i)
	j := i + 1
	if p.text(j) == "<<" {
		if owner := p.owner(); owner != nil && p.text(j+1) == "self" {
			p.scopes = append(p.scopes, rubyScope{kind: "singleton", name: owner.name, typeIndex: owner.typeIndex})
		} else {
			p.scopes = append(p.scopes, rubyScope{kind: "block"})
		}
		return j + 1
	}
	name := ""
	if p.text(j) == "::" {
		name = "::"
		j++
	}
	for j < len(p.tokens) && len(p.text(j)) > 0 && (p.text(j)[0] >= 'A' && p.text(j)[0] <= 'Z' || p.text(j) == "::") {
		name += p.text(j)
		j++
	}
	if name == "" {
		return i
	}
	name = p.qualify(name)
	index := -1
	for n, typ := range p.info.Types {
		if typ.Name == name {
			index = n
			break
		}
	}
	if index < 0 {
		index = len(p.info.Types)
		p.info.Types = append(p.info.Types, TypeInfo{Name: name, Kind: kind, IsExported: true, Comments: p.comments(i)})
	}
	p.scopes = append(p.scopes, rubyScope{kind: kind, name: name, typeIndex: index})
	return j - 1
}

func (p *rubyExtractor) method(i int) int {
	j := i + 1
	nameStart := j
	for j < len(p.tokens) && p.text(j) != "\n" && p.text(j) != ";" && p.text(j) != "(" && p.text(j) != "=" {
		if j > nameStart && p.tokens[j].start > p.tokens[j-1].end {
			break
		}
		j++
	}
	if j == nameStart {
		return i
	}
	name := p.source[p.tokens[nameStart].start:p.tokens[j-1].end]
	if p.text(j) == "=" && p.tokens[j].start == p.tokens[j-1].end {
		name += "="
		j++
	}
	endless := p.text(j) == "="
	if p.text(j) == "(" {
		depth := 0
		for j < len(p.tokens) {
			switch p.text(j) {
			case "(":
				depth++
			case ")":
				depth--
			}
			j++
			if depth == 0 {
				break
			}
		}
		endless = p.text(j) == "="
	} else if !endless {
		for j < len(p.tokens) && p.text(j) != "\n" && p.text(j) != ";" {
			j++
		}
	}
	fn := FunctionInfo{Name: name, Signature: strings.TrimSpace(p.source[p.tokens[i].start:p.tokens[j-1].end]), Comments: p.comments(i)}
	owner := p.owner()
	if owner != nil {
		fn.IsMethod, fn.Receiver, fn.IsExported = true, owner.name, !owner.private
	}
	if dot := strings.LastIndex(name, "."); dot >= 0 {
		receiver := name[:dot]
		fn.Name = name[dot+1:]
		if receiver == "self" && owner != nil {
			receiver = owner.name
		}
		fn.Receiver, fn.IsMethod, fn.IsExported = receiver+".self", true, true
	} else if owner != nil && owner.kind == "singleton" {
		fn.Receiver += ".self"
	}
	if p.text(i-1) == "private" || p.text(i-1) == "protected" {
		fn.IsExported = false
	}
	if fn.Name == "initialize" {
		fn.IsExported = false
	}
	p.info.Functions = append(p.info.Functions, fn)
	if owner != nil && owner.typeIndex >= 0 {
		methodName := fn.Name
		if strings.HasSuffix(fn.Receiver, ".self") {
			methodName = "self." + methodName
		}
		p.info.Types[owner.typeIndex].Methods = appendUnique(p.info.Types[owner.typeIndex].Methods, methodName)
	}
	if !endless {
		p.scopes = append(p.scopes, rubyScope{kind: "def"})
	}
	return j - 1
}

func (p *rubyExtractor) visibility(i int) {
	if p.text(i+1) == "\n" || p.text(i+1) == ";" || i+1 == len(p.tokens) {
		if owner := p.owner(); owner != nil {
			owner.private = p.text(i) != "public"
		}
		return
	}
	owner := p.owner()
	if owner == nil {
		return
	}
	for j := i + 1; j < len(p.tokens) && p.text(j) != "\n" && p.text(j) != ";"; j++ {
		if p.text(j-1) != ":" && !p.tokens[j].static {
			continue
		}
		for n := range p.info.Functions {
			fn := &p.info.Functions[n]
			if fn.Receiver == owner.name && fn.Name == p.tokens[j].text {
				fn.IsExported = p.text(i) == "public"
			}
		}
	}
}
func (p *rubyExtractor) attributes(i int) {
	owner := p.owner()
	if owner == nil || owner.typeIndex < 0 {
		return
	}
	for j := i + 1; j < len(p.tokens) && p.text(j) != "\n" && p.text(j) != ";"; j++ {
		if p.text(j-1) == ":" || p.tokens[j].static {
			name := p.tokens[j].text
			p.info.Types[owner.typeIndex].Fields = appendUnique(p.info.Types[owner.typeIndex].Fields, name)
		}
	}
}
func (p *rubyExtractor) comments(i int) string {
	before := p.source[:p.tokens[i].start]
	lineStart := strings.LastIndexByte(before, '\n')
	if lineStart < 0 {
		return ""
	}
	var lines []string
	for end := lineStart; end > 0; {
		start := strings.LastIndexByte(before[:end], '\n') + 1
		line := strings.TrimSpace(before[start:end])
		if !strings.HasPrefix(line, "#") {
			break
		}
		lines = append(lines, strings.TrimSpace(strings.TrimPrefix(line, "#")))
		end = start - 1
	}
	for l, r := 0, len(lines)-1; l < r; l, r = l+1, r-1 {
		lines[l], lines[r] = lines[r], lines[l]
	}
	return strings.Join(lines, "\n")
}
func (p *rubyExtractor) importAt(i int) {
	j := i + 1
	if p.text(j) == "(" {
		j++
	}
	if j >= len(p.tokens) || !p.tokens[j].static || strings.Contains(p.tokens[j].text, "#{") || p.tokens[j].text == "" {
		return
	}
	// Concatenated/dynamic arguments cannot be resolved statically.
	next := p.text(j + 1)
	if j+1 < len(p.tokens) && p.tokens[j+1].literal {
		return
	}
	if next != "" && next != "\n" && next != ";" && next != ")" && next != "," && next != "if" && next != "unless" {
		return
	}
	p.info.Imports = appendUnique(p.info.Imports, p.tokens[j].text)
}
func (p *rubyExtractor) detectFrameworks() {
	for _, imp := range p.info.Imports {
		root := strings.ToLower(strings.SplitN(imp, "/", 2)[0])
		switch root {
		case "rails", "sinatra", "roda", "hanami", "rspec", "minitest", "sidekiq":
			p.info.Frameworks = appendUnique(p.info.Frameworks, root)
		case "active_record", "activerecord":
			p.info.Frameworks = appendUnique(p.info.Frameworks, "activerecord")
			p.info.RiskTags = appendUnique(p.info.RiskTags, "database")
		case "pg", "mysql2", "sqlite3", "sequel":
			p.info.RiskTags = appendUnique(p.info.RiskTags, "database")
		case "openssl", "digest":
			p.info.RiskTags = appendUnique(p.info.RiskTags, "crypto")
		case "devise", "bcrypt", "jwt", "omniauth":
			p.info.RiskTags = appendUnique(p.info.RiskTags, "auth")
		}
	}
	for _, t := range p.tokens {
		if !t.literal && t.text == "Rails" {
			p.info.Frameworks = appendUnique(p.info.Frameworks, "rails")
		}
	}
}
