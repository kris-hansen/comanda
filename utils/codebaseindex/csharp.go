package codebaseindex

import (
	"path/filepath"
	"strings"
)

// CSharpAdapter indexes C# source without requiring a .NET installation or
// executing project code. Extraction is lexical, like the other built-in adapters.
type CSharpAdapter struct{}

func (a *CSharpAdapter) Name() string { return "csharp" }
func (a *CSharpAdapter) DetectionFiles() []string {
	return []string{"*.sln", "*.slnx", "*.csproj", "global.json", "Directory.Build.props", "Directory.Build.targets"}
}
func (a *CSharpAdapter) FileExtensions() []string { return []string{".cs"} }
func (a *CSharpAdapter) IgnoreDirs() []string {
	return []string{"bin", "obj", ".vs", "TestResults", "packages"}
}
func (a *CSharpAdapter) IgnoreGlobs() []string {
	return []string{"*.g.cs", "*.g.i.cs", "*.generated.cs", "*.Designer.cs", "*.AssemblyInfo.cs", "*.GlobalUsings.g.cs"}
}
func (a *CSharpAdapter) EntrypointPatterns() []string { return []string{"Program.cs"} }
func (a *CSharpAdapter) ConfigPatterns() []string {
	return []string{"*.sln", "*.slnx", "*.csproj", "*.props", "*.targets", "global.json", "NuGet.Config", "packages.lock.json"}
}
func (a *CSharpAdapter) ScoreFile(_ string, depth int, isEntrypoint, isConfig bool) int {
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
func (a *CSharpAdapter) ExtractSymbols(path string, content []byte) (*SymbolInfo, error) {
	return extractCSharpSymbols(path, content), nil
}

type csharpToken struct {
	text       string
	start, end int
	literal    bool
}

// csharpTokens hides comments and literal bodies while retaining source offsets
// for signatures and documentation comments.
func csharpTokens(source string) []csharpToken {
	var tokens []csharpToken
	for i := 0; i < len(source); {
		start := i
		c := source[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case c == '#' && csharpDirectiveStart(source, i):
			i = csharpLineEnd(source, i)
		case c == '/' && i+1 < len(source) && source[i+1] == '/':
			i = csharpLineEnd(source, i)
		case c == '/' && i+1 < len(source) && source[i+1] == '*':
			i += 2
			for i+1 < len(source) && (source[i] != '*' || source[i+1] != '/') {
				i++
			}
			if i+1 < len(source) {
				i += 2
			} else {
				i = len(source)
			}
		case c == '"' || c == '\'' || c == '$' || c == '@':
			if end, ok := csharpLiteralEnd(source, i); ok {
				i = end
				tokens = append(tokens, csharpToken{start: start, end: i, literal: true})
			} else if c == '@' && i+1 < len(source) && csharpWord(source[i+1]) {
				i += 2
				for i < len(source) && csharpWord(source[i]) {
					i++
				}
				tokens = append(tokens, csharpToken{text: source[start:i], start: start, end: i})
			} else {
				i++
				tokens = append(tokens, csharpToken{text: source[start:i], start: start, end: i})
			}
		case csharpWord(c):
			i++
			for i < len(source) && csharpWord(source[i]) {
				i++
			}
			tokens = append(tokens, csharpToken{text: source[start:i], start: start, end: i})
		default:
			i++
			if i < len(source) && ((c == ':' && source[i] == ':') || (c == '=' && source[i] == '>')) {
				i++
			}
			tokens = append(tokens, csharpToken{text: source[start:i], start: start, end: i})
		}
	}
	return tokens
}

func csharpDirectiveStart(source string, offset int) bool {
	for i := offset - 1; i >= 0 && source[i] != '\n'; i-- {
		if source[i] != ' ' && source[i] != '\t' && source[i] != '\r' {
			return false
		}
	}
	return true
}

func csharpLineEnd(source string, offset int) int {
	if end := strings.IndexByte(source[offset:], '\n'); end >= 0 {
		return offset + end
	}
	return len(source)
}

func csharpWord(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c >= 128
}

// csharpLiteralEnd handles regular, verbatim, interpolated, and raw strings,
// plus character literals. Interpolation bodies are intentionally hidden too.
func csharpLiteralEnd(source string, offset int) (int, bool) {
	if source[offset] == '\'' {
		for i := offset + 1; i < len(source); i++ {
			if source[i] == '\\' {
				i++
				continue
			}
			if source[i] == '\'' {
				return i + 1, true
			}
		}
		return len(source), true
	}

	i := offset
	for i < len(source) && source[i] == '$' {
		i++
	}
	verbatim := false
	if i < len(source) && source[i] == '@' {
		verbatim = true
		i++
		if i < len(source) && source[i] == '$' {
			i++
		}
	}
	if i >= len(source) || source[i] != '"' {
		return offset, false
	}
	quotes := 1
	for i+quotes < len(source) && source[i+quotes] == '"' {
		quotes++
	}
	if quotes >= 3 {
		i += quotes
		for i < len(source) {
			if source[i] != '"' {
				i++
				continue
			}
			end := i
			for end < len(source) && source[end] == '"' {
				end++
			}
			if end-i >= quotes {
				return i + quotes, true
			}
			i = end
		}
		return len(source), true
	}

	for i++; i < len(source); i++ {
		if source[i] == '\\' && !verbatim {
			i++
			continue
		}
		if source[i] != '"' {
			continue
		}
		if verbatim && i+1 < len(source) && source[i+1] == '"' {
			i++
			continue
		}
		return i + 1, true
	}
	return len(source), true
}

type csharpExtractor struct {
	source string
	tokens []csharpToken
	info   *SymbolInfo
}

func extractCSharpSymbols(path string, content []byte) *SymbolInfo {
	p := &csharpExtractor{
		source: string(content),
		info:   &SymbolInfo{Package: filepath.ToSlash(filepath.Dir(path))},
	}
	p.tokens = csharpTokens(p.source)
	p.parseContainer(0, len(p.tokens), "", "", -1)
	p.detectFrameworksAndRisks()
	return p.info
}

func (p *csharpExtractor) text(i int) string {
	if i < 0 || i >= len(p.tokens) || p.tokens[i].literal {
		return ""
	}
	return p.tokens[i].text
}

func (p *csharpExtractor) parseContainer(start, end int, namespace, ownerName string, ownerType int) {
	for i := start; i < end; {
		if ownerType < 0 {
			using := i
			if p.text(i) == "global" && p.text(i+1) == "using" {
				using = i + 1
			}
			if p.text(using) == "using" {
				if next, ok := p.usingAt(using, end); ok {
					i = next
					continue
				}
			}
			if p.text(i) == "namespace" {
				if next, updated, ok := p.namespaceAt(i, end, namespace); ok {
					namespace = updated
					i = next
					continue
				}
			}
		}
		if csharpTypeKeyword(p.text(i)) || p.text(i) == "delegate" {
			if next, ok := p.typeAt(i, end, namespace, ownerName, ownerType); ok {
				i = next
				continue
			}
		}
		if ownerType >= 0 {
			if next, ok := p.memberAt(i, end, namespace, ownerName, ownerType); ok {
				i = next
				continue
			}
		}
		i++
	}
}

func (p *csharpExtractor) usingAt(i, end int) (int, bool) {
	j := i + 1
	if p.text(j) == "static" || p.text(j) == "unsafe" {
		j++
	}
	equals := -1
	semicolon := -1
	for ; j < end; j++ {
		switch p.text(j) {
		case "=":
			if equals >= 0 || semicolon >= 0 {
				return i, false
			}
			equals = j
		case ";":
			semicolon = j
		}
		if semicolon >= 0 {
			break
		}
	}
	if semicolon < 0 {
		return i, false
	}
	start := i + 1
	if p.text(start) == "static" || p.text(start) == "unsafe" {
		start++
	}
	if equals >= 0 {
		start = equals + 1
	}
	name := p.qualifiedName(start, semicolon)
	if name == "" {
		return i, false
	}
	p.info.Imports = appendUnique(p.info.Imports, name)
	return semicolon + 1, true
}

func (p *csharpExtractor) qualifiedName(start, end int) string {
	var b strings.Builder
	needName := true
	for i := start; i < end; i++ {
		t := p.text(i)
		if t == "global" && needName && b.Len() == 0 {
			continue
		}
		if t == "::" && !needName {
			continue
		}
		if csharpIdentifier(t) {
			if !needName {
				return ""
			}
			b.WriteString(strings.TrimPrefix(t, "@"))
			needName = false
			continue
		}
		if (t == "." || t == "::") && !needName {
			b.WriteByte('.')
			needName = true
			continue
		}
		return ""
	}
	if needName {
		return ""
	}
	return b.String()
}

func (p *csharpExtractor) namespaceAt(i, end int, parent string) (int, string, bool) {
	j := i + 1
	for ; j < end; j++ {
		if p.text(j) == "{" || p.text(j) == ";" {
			break
		}
	}
	if j >= end {
		return i, parent, false
	}
	name := p.qualifiedName(i+1, j)
	if name == "" {
		return i, parent, false
	}
	if parent != "" {
		name = parent + "." + name
	}
	p.info.Package = name
	if p.text(j) == ";" {
		return j + 1, name, true
	}
	close := p.match(j, "{", "}", end)
	p.parseContainer(j+1, close, name, "", -1)
	return close + 1, parent, true
}

func csharpTypeKeyword(text string) bool {
	switch text {
	case "class", "struct", "interface", "enum", "record":
		return true
	}
	return false
}

func (p *csharpExtractor) typeAt(i, end int, namespace, ownerName string, ownerType int) (int, bool) {
	declaration := p.declarationStart(i)
	kind := p.text(i)
	j := i + 1
	if kind == "record" && (p.text(j) == "class" || p.text(j) == "struct") {
		j++
	}
	if kind == "delegate" {
		for j < end && p.text(j) != "(" && p.text(j) != ";" {
			j++
		}
		if j >= end || p.text(j) != "(" {
			return i, false
		}
	} else if !csharpIdentifier(p.text(j)) {
		return i, false
	}

	nameIndex := j
	if kind == "delegate" {
		nameIndex = p.nameBeforeParen(j)
	}
	name := strings.TrimPrefix(p.text(nameIndex), "@")
	if name == "" {
		return i, false
	}
	qualified := csharpQualify(namespace, ownerName, name)
	typeIndex := p.addType(qualified, kind, p.exported(declaration, i, ""), p.comments(declaration))
	if kind == "delegate" {
		for j < end && p.text(j) != ";" {
			j++
		}
		if j < end {
			return j + 1, true
		}
		return end, true
	}

	for j = nameIndex + 1; j < end; j++ {
		switch p.text(j) {
		case "(", "[":
			open, close := p.text(j), ")"
			if open == "[" {
				close = "]"
			}
			j = p.match(j, open, close, end)
		case ";":
			return j + 1, true
		case "{":
			close := p.match(j, "{", "}", end)
			if kind == "enum" {
				p.enumBody(j+1, close, qualified, typeIndex)
			} else {
				p.parseContainer(j+1, close, namespace, qualified, typeIndex)
			}
			return close + 1, true
		}
	}
	return end, true
}

func (p *csharpExtractor) addType(name, kind string, exported bool, comments string) int {
	for i := range p.info.Types {
		if p.info.Types[i].Name == name && p.info.Types[i].Kind == kind {
			p.info.Types[i].IsExported = p.info.Types[i].IsExported || exported
			if p.info.Types[i].Comments == "" {
				p.info.Types[i].Comments = comments
			}
			return i
		}
	}
	p.info.Types = append(p.info.Types, TypeInfo{Name: name, Kind: kind, IsExported: exported, Comments: comments})
	return len(p.info.Types) - 1
}

func csharpQualify(namespace, owner, name string) string {
	if owner != "" {
		return owner + "." + name
	}
	if namespace != "" {
		return namespace + "." + name
	}
	return name
}

func (p *csharpExtractor) enumBody(start, end int, owner string, typeIndex int) {
	expectName := true
	for i := start; i < end; i++ {
		if expectName && csharpIdentifier(p.text(i)) {
			name := strings.TrimPrefix(p.text(i), "@")
			qualified := owner + "." + name
			p.info.Constants = appendUnique(p.info.Constants, qualified)
			p.info.Types[typeIndex].Fields = appendUnique(p.info.Types[typeIndex].Fields, name)
			expectName = false
			continue
		}
		if p.text(i) == "," {
			expectName = true
		}
	}
}

func (p *csharpExtractor) memberAt(start, end int, namespace, ownerName string, ownerType int) (int, bool) {
	for i := start; i < end; i++ {
		t := p.text(i)
		if csharpTypeKeyword(t) || t == "delegate" || t == "namespace" || t == "using" {
			return start, false
		}
		switch t {
		case "[":
			i = p.match(i, "[", "]", end)
		case "(":
			return p.methodAt(start, i, end, ownerName, ownerType)
		case "{", ";", "=", "=>":
			return p.fieldAt(start, i, end, ownerName, ownerType)
		}
	}
	return start, false
}

func (p *csharpExtractor) methodAt(start, open, end int, ownerName string, ownerType int) (int, bool) {
	nameIndex := p.nameBeforeParen(open)
	name := strings.TrimPrefix(p.text(nameIndex), "@")
	if nameIndex > 0 && p.text(nameIndex-1) == "~" {
		name = "~" + name
	}
	if nameIndex > 0 && p.text(nameIndex-1) == "." {
		qualifier := p.text(nameIndex - 2)
		if csharpIdentifier(qualifier) {
			name = strings.TrimPrefix(qualifier, "@") + "." + name
		}
	}
	if operator := p.previousToken(nameIndex, "operator", start); operator >= 0 {
		name = "operator " + name
	}
	if !csharpMemberName(name) || csharpMemberKeyword(name) {
		return start, false
	}
	close := p.match(open, "(", ")", end)
	if close >= end || close >= len(p.tokens) {
		return end, true
	}
	fn := FunctionInfo{
		Name:       name,
		Signature:  strings.TrimSpace(p.source[p.tokens[start].start:p.tokens[close].end]),
		IsMethod:   true,
		Receiver:   ownerName,
		IsExported: p.exported(start, nameIndex, p.info.Types[ownerType].Kind),
		Comments:   p.comments(start),
	}
	p.info.Functions = append(p.info.Functions, fn)
	p.info.Types[ownerType].Methods = appendUnique(p.info.Types[ownerType].Methods, name)
	for i := close + 1; i < end; i++ {
		switch p.text(i) {
		case ";":
			return i + 1, true
		case "=>":
			return p.statementEnd(i+1, end), true
		case "{":
			return p.match(i, "{", "}", end) + 1, true
		}
	}
	return end, true
}

func (p *csharpExtractor) fieldAt(start, delimiter, end int, ownerName string, ownerType int) (int, bool) {
	nameIndex := delimiter - 1
	if p.text(nameIndex) == "]" {
		nameIndex = p.matchBackward(nameIndex, "[", "]", start) - 1
	}
	name := strings.TrimPrefix(p.text(nameIndex), "@")
	if !csharpMemberName(name) || csharpMemberKeyword(name) {
		return start, false
	}
	p.info.Types[ownerType].Fields = appendUnique(p.info.Types[ownerType].Fields, name)
	if p.hasModifier(start, delimiter, "const") {
		p.info.Constants = appendUnique(p.info.Constants, ownerName+"."+name)
	}
	switch p.text(delimiter) {
	case ";":
		return delimiter + 1, true
	case "=", "=>":
		return p.statementEnd(delimiter+1, end), true
	case "{":
		next := p.match(delimiter, "{", "}", end) + 1
		if p.text(next) == "=" {
			next = p.statementEnd(next+1, end)
		}
		return next, true
	}
	return start, false
}

func (p *csharpExtractor) nameBeforeParen(open int) int {
	i := open - 1
	if p.text(i) != ">" {
		return i
	}
	depth := 0
	for ; i >= 0; i-- {
		switch p.text(i) {
		case ">":
			depth++
		case "<":
			depth--
			if depth == 0 {
				return i - 1
			}
		}
	}
	return open - 1
}

func (p *csharpExtractor) previousToken(index int, text string, start int) int {
	for i := index - 1; i >= start; i-- {
		if p.text(i) == text {
			return i
		}
		if p.text(i) == ";" || p.text(i) == "{" || p.text(i) == "}" {
			break
		}
	}
	return -1
}

func csharpIdentifier(text string) bool {
	if text == "" {
		return false
	}
	c := text[0]
	if c == '@' {
		return len(text) > 1 && csharpWord(text[1])
	}
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c >= 128
}

func csharpMemberName(text string) bool {
	if strings.HasPrefix(text, "operator ") || strings.HasPrefix(text, "~") {
		return true
	}
	if dot := strings.LastIndexByte(text, '.'); dot >= 0 {
		text = text[dot+1:]
	}
	return csharpIdentifier(text)
}

func csharpMemberKeyword(text string) bool {
	switch text {
	case "if", "for", "foreach", "while", "switch", "catch", "using", "lock", "return", "new", "throw", "typeof", "nameof", "default", "checked", "unchecked", "fixed", "get", "set", "add", "remove", "public", "private", "protected", "internal", "static":
		return true
	}
	return false
}

func (p *csharpExtractor) match(open int, openText, closeText string, end int) int {
	depth := 0
	for i := open; i < end; i++ {
		switch p.text(i) {
		case openText:
			depth++
		case closeText:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return end
}

func (p *csharpExtractor) matchBackward(open int, openText, closeText string, start int) int {
	depth := 0
	for i := open; i >= start; i-- {
		switch p.text(i) {
		case closeText:
			depth++
		case openText:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return start
}

func (p *csharpExtractor) statementEnd(start, end int) int {
	parens, brackets, braces := 0, 0, 0
	for i := start; i < end; i++ {
		switch p.text(i) {
		case "(":
			parens++
		case ")":
			parens--
		case "[":
			brackets++
		case "]":
			brackets--
		case "{":
			braces++
		case "}":
			braces--
		case ";":
			if parens <= 0 && brackets <= 0 && braces <= 0 {
				return i + 1
			}
		}
	}
	return end
}

func (p *csharpExtractor) declarationStart(i int) int {
	for i > 0 {
		previous := p.text(i - 1)
		if csharpModifier(previous) {
			i--
			continue
		}
		if previous == "]" {
			i = p.matchBackward(i-1, "[", "]", 0)
			continue
		}
		break
	}
	return i
}

func csharpModifier(text string) bool {
	switch text {
	case "public", "private", "protected", "internal", "static", "sealed", "abstract", "partial", "readonly", "ref", "unsafe", "new", "file", "async", "extern", "virtual", "override", "required", "volatile", "event", "const", "scoped":
		return true
	}
	return false
}

func (p *csharpExtractor) hasModifier(start, end int, modifier string) bool {
	for i := start; i < end; i++ {
		if p.text(i) == modifier {
			return true
		}
	}
	return false
}

func (p *csharpExtractor) exported(start, end int, ownerKind string) bool {
	if p.hasModifier(start, end, "public") {
		return true
	}
	return ownerKind == "interface" && !p.hasModifier(start, end, "private")
}

func (p *csharpExtractor) comments(start int) string {
	if start < 0 || start >= len(p.tokens) {
		return ""
	}
	before := strings.TrimSpace(p.source[:p.tokens[start].start])
	if before == "" {
		return ""
	}
	if strings.HasSuffix(before, "*/") {
		if open := strings.LastIndex(before, "/*"); open >= 0 {
			body := before[open+2 : len(before)-2]
			var lines []string
			for _, line := range strings.Split(body, "\n") {
				line = strings.TrimSpace(line)
				line = strings.TrimSpace(strings.TrimPrefix(line, "*"))
				lines = append(lines, line)
			}
			return strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	lines := strings.Split(before, "\n")
	var comments []string
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "//") {
			break
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "//"))
		line = strings.TrimSpace(strings.TrimPrefix(line, "/"))
		comments = append(comments, line)
	}
	for left, right := 0, len(comments)-1; left < right; left, right = left+1, right-1 {
		comments[left], comments[right] = comments[right], comments[left]
	}
	return strings.Join(comments, "\n")
}

func (p *csharpExtractor) detectFrameworksAndRisks() {
	for _, imp := range p.info.Imports {
		lower := strings.ToLower(imp)
		switch {
		case strings.HasPrefix(lower, "microsoft.aspnetcore"):
			p.info.Frameworks = appendUnique(p.info.Frameworks, "aspnetcore")
		case strings.HasPrefix(lower, "microsoft.entityframeworkcore"):
			p.info.Frameworks = appendUnique(p.info.Frameworks, "entityframework")
		case strings.HasPrefix(lower, "xunit"):
			p.info.Frameworks = appendUnique(p.info.Frameworks, "xunit")
		case strings.HasPrefix(lower, "nunit"):
			p.info.Frameworks = appendUnique(p.info.Frameworks, "nunit")
		case strings.HasPrefix(lower, "microsoft.visualstudio.testtools"):
			p.info.Frameworks = appendUnique(p.info.Frameworks, "mstest")
		case strings.HasPrefix(lower, "grpc."):
			p.info.Frameworks = appendUnique(p.info.Frameworks, "grpc")
		}

		switch {
		case strings.HasPrefix(lower, "system.data"), strings.HasPrefix(lower, "microsoft.data"), strings.HasPrefix(lower, "microsoft.entityframeworkcore"), lower == "dapper", strings.HasPrefix(lower, "dapper."), strings.HasPrefix(lower, "npgsql"), strings.HasPrefix(lower, "mysql"), strings.HasPrefix(lower, "mongodb"):
			p.info.RiskTags = appendUnique(p.info.RiskTags, "database")
		}
		switch {
		case strings.HasPrefix(lower, "system.security.cryptography"):
			p.info.RiskTags = appendUnique(p.info.RiskTags, "crypto")
		}
		switch {
		case strings.HasPrefix(lower, "system.net"), strings.HasPrefix(lower, "grpc."):
			p.info.RiskTags = appendUnique(p.info.RiskTags, "network")
		}
		switch {
		case strings.Contains(lower, "authentication"), strings.Contains(lower, "authorization"), strings.HasPrefix(lower, "microsoft.identity"), strings.HasPrefix(lower, "system.identitymodel"):
			p.info.RiskTags = appendUnique(p.info.RiskTags, "auth")
		}
		switch {
		case lower == "system.io" || strings.HasPrefix(lower, "system.io."):
			p.info.RiskTags = appendUnique(p.info.RiskTags, "filesystem")
		}
		switch {
		case strings.HasPrefix(lower, "system.threading"):
			p.info.RiskTags = appendUnique(p.info.RiskTags, "concurrency")
		}
		switch {
		case lower == "system.diagnostics" || strings.HasPrefix(lower, "system.diagnostics."):
			p.info.RiskTags = appendUnique(p.info.RiskTags, "subprocess")
		}
		switch {
		case lower == "system.reflection" || strings.HasPrefix(lower, "system.reflection."):
			p.info.RiskTags = appendUnique(p.info.RiskTags, "reflection")
		}
	}
}
