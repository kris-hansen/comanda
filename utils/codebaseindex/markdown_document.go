package codebaseindex

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	goldmarktext "github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

const MarkdownParserVersion = "markdown-document-v1"

var (
	explicitHeadingID = regexp.MustCompile(`\s+\{#([A-Za-z0-9_.:-]+)\}\s*$`)
	wikiLinkPattern   = regexp.MustCompile(`\[\[([^\]|]+)(?:\|[^\]]*)?\]\]`)
)

type markdownSectionBuilder struct {
	section DocumentSection
	parts   []string
}

func extractMarkdownDocument(path string, content []byte) (*SymbolInfo, error) {
	normalized := strings.ReplaceAll(string(content), "\r\n", "\n")
	body, bodyStartLine, properties, diagnostics := parseDocumentFrontmatter(normalized)
	document := &DocumentInfo{
		Properties:  map[string]any{},
		Diagnostics: diagnostics,
		Role:        "documentation",
	}
	if IsAgentInstructionFile(path) {
		document.Role = "agent_instruction"
	}
	applyDocumentFrontmatter(document, properties)

	parser := goldmark.New(goldmark.WithExtensions(extension.GFM))
	root := parser.Parser().Parse(goldmarktext.NewReader([]byte(body)))
	builders := buildDocumentSections(document, root, []byte(body), bodyStartLine, path)
	collectDocumentLinks(document, root, []byte(body), builders, bodyStartLine)

	document.Sections = make([]DocumentSection, 0, len(builders))
	for _, builder := range builders {
		builder.section.Content = strings.TrimSpace(strings.Join(builder.parts, "\n\n"))
		builder.section.ContentHash = fmt.Sprintf("%x", sha256.Sum256([]byte(builder.section.Content)))
		if builder.section.Level == 0 && builder.section.Content == "" {
			continue
		}
		document.Sections = append(document.Sections, builder.section)
	}

	info := &SymbolInfo{Document: document, Package: document.Title}
	for _, section := range document.Sections {
		if section.Level > 0 {
			info.Types = append(info.Types, TypeInfo{Name: section.Title, Kind: "h" + strconv.Itoa(section.Level), IsExported: true})
		}
	}
	for _, link := range document.Links {
		if link.Kind == "markdown" {
			info.Imports = appendUnique(info.Imports, link.Target)
		}
	}

	return info, nil
}

func parseDocumentFrontmatter(content string) (string, int, map[string]any, []DocumentDiagnostic) {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return content, 1, nil, nil
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != "---" {
			continue
		}
		var properties map[string]any
		var diagnostics []DocumentDiagnostic
		if err := yaml.Unmarshal([]byte(strings.Join(lines[1:i], "\n")), &properties); err != nil {
			diagnostics = append(diagnostics, DocumentDiagnostic{Kind: "invalid_frontmatter", Message: err.Error(), Line: 1})
			properties = nil
		}
		body := strings.Join(lines[i+1:], "\n")
		bodyStartLine := i + 2
		for strings.HasPrefix(body, "\n") {
			body = strings.TrimPrefix(body, "\n")
			bodyStartLine++
		}
		return body, bodyStartLine, properties, diagnostics
	}
	return content, 1, nil, []DocumentDiagnostic{{
		Kind: "invalid_frontmatter", Message: "unclosed frontmatter: missing closing ---", Line: 1,
	}}
}

func applyDocumentFrontmatter(document *DocumentInfo, properties map[string]any) {
	for key, value := range properties {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "id":
			document.ID = strings.TrimSpace(fmt.Sprint(value))
		case "title":
			document.Title = strings.TrimSpace(fmt.Sprint(value))
		case "role", "type":
			if role := strings.TrimSpace(fmt.Sprint(value)); role != "" {
				document.Role = role
			}
		case "aliases":
			document.Aliases = frontmatterStrings(value)
		case "tags":
			document.Tags = frontmatterStrings(value)
		default:
			document.Properties[key] = value
		}
	}
	for _, kind := range []string{"supersedes", "depends_on", "related_to", "describes", "guides"} {
		for key, value := range properties {
			if strings.ToLower(strings.TrimSpace(key)) != kind {
				continue
			}
			for _, target := range frontmatterStrings(value) {
				if target = strings.TrimSpace(target); target != "" {
					document.Relations = append(document.Relations, DocumentRelation{Kind: kind, Target: target})
				}
			}
		}
	}
	for key := range properties {
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "supersedes", "depends_on", "related_to", "describes", "guides":
			delete(document.Properties, key)
		}
	}
	sort.Strings(document.Aliases)
	sort.Strings(document.Tags)
	sort.SliceStable(document.Relations, func(i, j int) bool {
		if document.Relations[i].Kind != document.Relations[j].Kind {
			return document.Relations[i].Kind < document.Relations[j].Kind
		}
		return document.Relations[i].Target < document.Relations[j].Target
	})
}

func frontmatterStrings(value any) []string {
	var values []string
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			values = append(values, strings.TrimSpace(fmt.Sprint(item)))
		}
	case []string:
		values = append(values, typed...)
	case string:
		values = append(values, typed)
	case nil:
	default:
		values = append(values, fmt.Sprint(typed))
	}
	seen := map[string]bool{}
	result := values[:0]
	for _, item := range values {
		if item = strings.TrimSpace(item); item != "" && !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

func buildDocumentSections(document *DocumentInfo, root ast.Node, source []byte, firstLine int, path string) []*markdownSectionBuilder {
	var builders []*markdownSectionBuilder
	var stack []*markdownSectionBuilder
	preamble := &markdownSectionBuilder{section: DocumentSection{
		ID: "preamble", Level: 0, StartLine: firstLine, BodyEndLine: firstLine, EndLine: firstLine,
	}}
	current := preamble
	anchorCounts := map[string]int{}
	totalLines := firstLine + strings.Count(string(source), "\n")

	for node := root.FirstChild(); node != nil; node = node.NextSibling() {
		startLine := sourceLine(source, nodeStart(node), firstLine)
		if heading, ok := node.(*ast.Heading); ok {
			current.section.BodyEndLine = maxLine(firstLine, startLine-1)
			for len(stack) > 0 && stack[len(stack)-1].section.Level >= heading.Level {
				stack[len(stack)-1].section.EndLine = maxLine(firstLine, startLine-1)
				stack = stack[:len(stack)-1]
			}

			title := normalizeInlineText(plainMarkdownNode(heading, source))
			anchor := ""
			if match := explicitHeadingID.FindStringSubmatch(title); match != nil {
				anchor = match[1]
				title = strings.TrimSpace(explicitHeadingID.ReplaceAllString(title, ""))
			}
			if anchor == "" {
				anchor = headingSlug(title)
			}
			if anchor == "" {
				anchor = "section"
			}
			anchorCounts[anchor]++
			if anchorCounts[anchor] > 1 {
				anchor += "-" + strconv.Itoa(anchorCounts[anchor])
			}

			headingPath := title
			parentID := ""
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parentID = parent.section.ID
				if parent.section.HeadingPath != "" {
					headingPath = parent.section.HeadingPath + " > " + title
				}
			}
			builder := &markdownSectionBuilder{section: DocumentSection{
				ID: anchor, ParentID: parentID, Title: title, HeadingPath: headingPath,
				Anchor: anchor, Level: heading.Level, StartLine: startLine,
				BodyEndLine: startLine, EndLine: startLine,
			}}
			builders = append(builders, builder)
			stack = append(stack, builder)
			current = builder
			if document.Title == "" && heading.Level == 1 {
				document.Title = title
			}
			continue
		}

		if text := strings.TrimSpace(plainMarkdownNode(node, source)); text != "" {
			current.parts = append(current.parts, text)
		}
		endLine := sourceLine(source, nodeEnd(node), firstLine)
		if endLine > current.section.BodyEndLine {
			current.section.BodyEndLine = endLine
		}
	}

	for _, builder := range stack {
		builder.section.EndLine = totalLines
	}
	preamble.section.EndLine = totalLines
	if document.Title == "" && len(builders) > 0 {
		document.Title = builders[0].section.Title
	}
	if document.Title == "" && len(preamble.parts) > 0 {
		document.Title = boundedTitle(preamble.parts[0])
	}
	if document.Title == "" {
		document.Title = filepath.Base(path)
	}
	preamble.section.Title = document.Title
	preamble.section.HeadingPath = document.Title
	if len(preamble.parts) > 0 {
		builders = append([]*markdownSectionBuilder{preamble}, builders...)
	}
	return builders
}

func collectDocumentLinks(document *DocumentInfo, root ast.Node, source []byte, sections []*markdownSectionBuilder, firstLine int) {
	seen := map[string]bool{}
	add := func(link DocumentLink) {
		if link.Target == "" && link.Kind != "anchor" {
			return
		}
		if link.SourceSection == "" {
			link.SourceSection = sectionIDForLine(sections, link.Line)
		}
		key := fmt.Sprintf("%s|%s|%s|%s|%d", link.SourceSection, link.Kind, link.Target, link.Fragment, link.Line)
		if !seen[key] {
			seen[key] = true
			document.Links = append(document.Links, link)
		}
	}

	_ = ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch typed := node.(type) {
		case *ast.Link:
			line := sourceLine(source, nodeStart(typed), firstLine)
			add(classifyDocumentLink(string(typed.Destination), line, false))
		case *ast.AutoLink:
			line := sourceLine(source, nodeStart(typed), firstLine)
			add(classifyDocumentLink(string(typed.URL(source)), line, false))
		case *ast.Paragraph:
			line := sourceLine(source, nodeStart(typed), firstLine)
			for _, match := range wikiLinkPattern.FindAllStringSubmatch(plainMarkdownNode(typed, source), -1) {
				add(classifyDocumentLink(strings.TrimSpace(match[1]), line, true))
			}
		}
		return ast.WalkContinue, nil
	})
	sort.SliceStable(document.Links, func(i, j int) bool {
		if document.Links[i].Line != document.Links[j].Line {
			return document.Links[i].Line < document.Links[j].Line
		}
		return document.Links[i].Target < document.Links[j].Target
	})
}

func classifyDocumentLink(raw string, line int, wiki bool) DocumentLink {
	link := DocumentLink{Target: strings.TrimSpace(raw), Line: line, Wiki: wiki}
	if index := strings.Index(link.Target, "#"); index >= 0 {
		link.Fragment = strings.TrimSpace(link.Target[index+1:])
		link.Target = strings.TrimSpace(link.Target[:index])
	}
	if decoded, err := url.PathUnescape(link.Target); err == nil {
		link.Target = decoded
	}
	if decoded, err := url.PathUnescape(link.Fragment); err == nil {
		link.Fragment = decoded
	}
	if link.Target == "" {
		link.Kind = "anchor"
		return link
	}
	parsed, err := url.Parse(link.Target)
	if strings.HasPrefix(link.Target, "//") || err == nil && parsed.Scheme != "" {
		link.Kind = "external"
		return link
	}
	lower := strings.ToLower(link.Target)
	if wiki || strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown") {
		link.Kind = "markdown"
	} else {
		link.Kind = "file"
	}
	return link
}

func plainMarkdownNode(node ast.Node, source []byte) string {
	var builder strings.Builder
	_ = ast.Walk(node, func(current ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch typed := current.(type) {
		case *ast.Text:
			builder.Write(typed.Value(source))
			if typed.SoftLineBreak() || typed.HardLineBreak() {
				builder.WriteByte('\n')
			}
		case *ast.CodeBlock:
			writeSegments(&builder, typed.Lines(), source)
		case *ast.FencedCodeBlock:
			writeSegments(&builder, typed.Lines(), source)
		case *ast.HTMLBlock:
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return builder.String()
}

func writeSegments(builder *strings.Builder, segments *goldmarktext.Segments, source []byte) {
	if segments == nil {
		return
	}
	for i := 0; i < segments.Len(); i++ {
		segment := segments.At(i)
		builder.Write(segment.Value(source))
	}
}

func nodeStart(node ast.Node) int {
	if position := node.Pos(); position >= 0 {
		return position
	}
	if lines := node.Lines(); lines != nil && lines.Len() > 0 {
		return lines.At(0).Start
	}
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if position := nodeStart(child); position >= 0 {
			return position
		}
	}
	return 0
}

func nodeEnd(node ast.Node) int {
	if lines := node.Lines(); lines != nil && lines.Len() > 0 {
		segment := lines.At(lines.Len() - 1)
		return segment.Stop
	}
	for child := node.LastChild(); child != nil; child = child.PreviousSibling() {
		if position := nodeEnd(child); position > 0 {
			return position
		}
	}
	return nodeStart(node)
}

func sourceLine(source []byte, offset, firstLine int) int {
	if offset < 0 {
		offset = 0
	}
	if offset > len(source) {
		offset = len(source)
	}
	return firstLine + strings.Count(string(source[:offset]), "\n")
}

func sectionIDForLine(sections []*markdownSectionBuilder, line int) string {
	best := ""
	bestStart := -1
	for _, builder := range sections {
		section := builder.section
		if line >= section.StartLine && line <= section.EndLine && section.StartLine >= bestStart {
			best = section.ID
			bestStart = section.StartLine
		}
	}
	return best
}

func normalizeInlineText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func boundedTitle(value string) string {
	value = normalizeInlineText(value)
	runes := []rune(value)
	if len(runes) > 120 {
		return string(runes[:117]) + "..."
	}
	return value
}

func headingSlug(value string) string {
	var builder strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(value) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
			builder.WriteRune(r)
			lastDash = false
		case unicode.IsSpace(r) || r == '-':
			if builder.Len() > 0 && !lastDash {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
}

func maxLine(left, right int) int {
	if left > right {
		return left
	}
	return right
}
