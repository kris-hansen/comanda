package knowledgegraph

import (
	"crypto/sha256"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/kris-hansen/comanda/utils/codebaseindex"
	"github.com/kris-hansen/comanda/utils/semanticmemory"
)

const maxDocumentBlockRunes = 4000

type documentGraphIndex struct {
	files    map[string]*codebaseindex.FileEntry
	docs     map[string]*codebaseindex.FileEntry
	wiki     map[string][]string
	sections map[string]map[string]string
}

func addDocumentStructure(g *Graph, scan *codebaseindex.ScanResult) *documentGraphIndex {
	index := &documentGraphIndex{
		files: map[string]*codebaseindex.FileEntry{}, docs: map[string]*codebaseindex.FileEntry{},
		wiki: map[string][]string{}, sections: map[string]map[string]string{},
	}
	for _, file := range scan.Candidates {
		index.files[file.Path] = file
		if !isMarkdownFile(file) || file.Symbols == nil || file.Symbols.Document == nil {
			continue
		}
		document := file.Symbols.Document
		index.docs[file.Path] = file
		index.sections[file.Path] = map[string]string{}
		for _, key := range documentLookupKeys(file.Path, document) {
			index.wiki[key] = append(index.wiki[key], file.Path)
		}
		for _, diagnostic := range document.Diagnostics {
			g.addDiagnostic(diagnostic.Kind, file.Path, diagnostic.Line, "%s", diagnostic.Message)
		}

		documentNode := g.Nodes[NodeID(g.Namespace, "file:"+file.Path)]
		properties := map[string]any{
			"document_id": document.ID, "role": document.Role, "aliases": document.Aliases,
			"tags": document.Tags, "parser_version": codebaseindex.MarkdownParserVersion,
		}
		for key, value := range document.Properties {
			properties[key] = value
		}
		setNodeProperties(documentNode, properties)

		for _, section := range document.Sections {
			local := sectionLocalID(file.Path, section.ID)
			index.sections[file.Path][section.Anchor] = local
			node := g.AddNode(local, NodeSection, section.Title, file.Path, "", section.HeadingPath)
			node.StartLine = section.StartLine
			node.EndLine = section.BodyEndLine
			setNodeProperties(node, map[string]any{
				"anchor": section.Anchor, "heading_path": section.HeadingPath, "level": section.Level,
				"content_hash": section.ContentHash, "parser_version": codebaseindex.MarkdownParserVersion,
			})
			parentID := documentNode.ID
			if section.ParentID != "" {
				parentID = NodeID(g.Namespace, sectionLocalID(file.Path, section.ParentID))
			}
			g.AddEdge(parentID, node.ID, EdgeContains, ConfidenceExtracted, "document section")
			g.AddEvidence(EdgeID(g.Namespace, parentID, node.ID, EdgeContains), semanticmemory.GraphEvidence{
				SourcePath: file.Path, StartLine: section.StartLine, EndLine: section.BodyEndLine,
				Quote: section.Title, ParserVersion: codebaseindex.MarkdownParserVersion,
			})

			chunks := splitDocumentContent(section.Content, maxDocumentBlockRunes)
			if len(chunks) <= 1 {
				node.Content = section.Content
				continue
			}
			for i, chunk := range chunks {
				blockLocal := fmt.Sprintf("%s:%d", local, i+1)
				block := g.AddNode(blockLocal, NodeBlock, fmt.Sprintf("%s [part %d/%d]", section.Title, i+1, len(chunks)),
					file.Path, "", section.HeadingPath)
				block.Content = chunk
				block.StartLine = section.StartLine
				block.EndLine = section.BodyEndLine
				setNodeProperties(block, map[string]any{
					"heading_path": section.HeadingPath, "part": i + 1, "parts": len(chunks),
					"parser_version": codebaseindex.MarkdownParserVersion,
				})
				g.AddEdge(node.ID, block.ID, EdgeContains, ConfidenceExtracted, "section content block")
				g.AddEvidence(EdgeID(g.Namespace, node.ID, block.ID, EdgeContains), semanticmemory.GraphEvidence{
					SourcePath: file.Path, StartLine: section.StartLine, EndLine: section.BodyEndLine,
					Quote: boundedEvidenceQuote(chunk), ParserVersion: codebaseindex.MarkdownParserVersion,
				})
			}
		}
	}
	for key := range index.wiki {
		sort.Strings(index.wiki[key])
	}
	return index
}

func addDocumentLinksAndRelations(g *Graph, scan *codebaseindex.ScanResult, index *documentGraphIndex) {
	for _, file := range scan.Candidates {
		if !isMarkdownFile(file) || file.Symbols == nil || file.Symbols.Document == nil {
			continue
		}
		document := file.Symbols.Document
		documentID := NodeID(g.Namespace, "file:"+file.Path)
		for _, link := range document.Links {
			sourceID := documentID
			if link.SourceSection != "" {
				sourceID = NodeID(g.Namespace, sectionLocalID(file.Path, link.SourceSection))
			}
			if g.Nodes[sourceID] == nil {
				sourceID = documentID
			}
			targetID, ok := index.resolveLink(g, file, link)
			if !ok {
				continue
			}
			g.AddEdge(sourceID, targetID, EdgeLinksTo, ConfidenceExtracted, "document link "+link.Target)
			g.AddEvidence(EdgeID(g.Namespace, sourceID, targetID, EdgeLinksTo), semanticmemory.GraphEvidence{
				SourcePath: file.Path, StartLine: link.Line, EndLine: link.Line,
				Quote: rawDocumentLink(link), ParserVersion: codebaseindex.MarkdownParserVersion,
			})
		}

		for _, relation := range document.Relations {
			targetID, ok := index.resolveRelationTarget(g, file.Path, relation)
			if !ok {
				continue
			}
			target := g.Nodes[targetID]
			if target == nil {
				continue
			}
			if err := ValidateDocumentRelation(relation.Kind, target.Kind); err != nil {
				g.addDiagnostic("invalid_relation", file.Path, 1, "%s", err)
				continue
			}
			g.AddEdge(documentID, targetID, relation.Kind, ConfidenceExtracted, "frontmatter "+relation.Kind)
			g.AddEvidence(EdgeID(g.Namespace, documentID, targetID, relation.Kind), semanticmemory.GraphEvidence{
				SourcePath: file.Path, StartLine: 1, EndLine: 1, Quote: relation.Kind + ": " + relation.Target,
				Extractor: "frontmatter", ParserVersion: codebaseindex.MarkdownParserVersion,
			})
		}
	}
}

func (index *documentGraphIndex) resolveLink(g *Graph, file *codebaseindex.FileEntry, link codebaseindex.DocumentLink) (string, bool) {
	if link.Kind == "external" {
		raw := rawDocumentLink(link)
		sum := sha256.Sum256([]byte(raw))
		local := fmt.Sprintf("external:%x", sum[:12])
		node := g.AddNode(local, NodeExternalResource, raw, "", "", "External resource")
		setNodeProperties(node, map[string]any{"url": raw})
		return node.ID, true
	}

	documentPath := file.Path
	if link.Target != "" {
		if link.Wiki && len(index.wiki[documentLookupKey(link.Target)]) > 1 {
			g.addDiagnostic("ambiguous_link", file.Path, link.Line, "wiki link %q matches multiple documents", rawDocumentLink(link))
			return "", false
		}
		resolved, ok := index.resolveDocumentPath(file, link)
		if !ok {
			if link.Kind == "file" {
				cleaned := path.Clean(path.Join(path.Dir(file.Path), link.Target))
				if target := index.files[cleaned]; target != nil {
					id := NodeID(g.Namespace, "file:"+cleaned)
					return id, true
				}
			}
			g.addDiagnostic("unresolved_link", file.Path, link.Line, "cannot resolve %q", rawDocumentLink(link))
			return "", false
		}
		documentPath = resolved
	}

	targetID := NodeID(g.Namespace, "file:"+documentPath)
	if link.Fragment != "" {
		if section, ok := index.sections[documentPath][link.Fragment]; ok {
			targetID = NodeID(g.Namespace, section)
		} else {
			g.addDiagnostic("unresolved_anchor", file.Path, link.Line, "cannot resolve anchor %q in %s", link.Fragment, documentPath)
		}
	}
	return targetID, true
}

func (index *documentGraphIndex) resolveDocumentPath(file *codebaseindex.FileEntry, link codebaseindex.DocumentLink) (string, bool) {
	if link.Wiki {
		if index.docs[link.Target] != nil {
			return link.Target, true
		}
		key := documentLookupKey(link.Target)
		candidates := index.wiki[key]
		if len(candidates) == 1 {
			return candidates[0], true
		}
		return "", false
	}
	cleaned := path.Clean(path.Join(path.Dir(file.Path), link.Target))
	if index.docs[cleaned] != nil {
		return cleaned, true
	}
	for _, extension := range []string{".md", ".markdown"} {
		if index.docs[cleaned+extension] != nil {
			return cleaned + extension, true
		}
	}
	return "", false
}

func (index *documentGraphIndex) resolveRelationTarget(g *Graph, fromPath string, relation codebaseindex.DocumentRelation) (string, bool) {
	link := codebaseindex.DocumentLink{Target: relation.Target, Kind: "markdown", Wiki: true, Line: 1}
	if resolved, ok := index.resolveDocumentPath(&codebaseindex.FileEntry{Path: fromPath}, link); ok {
		return NodeID(g.Namespace, "file:"+resolved), true
	}

	var matches []string
	for id, node := range g.Nodes {
		if node.Name == relation.Target || node.Path == relation.Target || LocalID(id) == relation.Target {
			matches = append(matches, id)
		}
	}
	sort.Strings(matches)
	if len(matches) == 1 {
		return matches[0], true
	}
	if len(matches) > 1 {
		g.addDiagnostic("ambiguous_relation", fromPath, 1, "relation %s target %q is ambiguous", relation.Kind, relation.Target)
	} else {
		g.addDiagnostic("unresolved_relation", fromPath, 1, "cannot resolve relation %s target %q", relation.Kind, relation.Target)
	}
	return "", false
}

func documentLookupKeys(filePath string, document *codebaseindex.DocumentInfo) []string {
	values := []string{filePath, path.Base(filePath), strings.TrimSuffix(path.Base(filePath), path.Ext(filePath)), document.ID}
	values = append(values, document.Aliases...)
	seen := map[string]bool{}
	var keys []string
	for _, value := range values {
		if key := documentLookupKey(value); key != "" && !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	return keys
}

func documentLookupKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimSuffix(strings.TrimSuffix(value, ".markdown"), ".md")
	return value
}

func sectionLocalID(documentPath, sectionID string) string {
	return "section:" + documentPath + "#" + sectionID
}

func rawDocumentLink(link codebaseindex.DocumentLink) string {
	if link.Fragment != "" {
		return link.Target + "#" + link.Fragment
	}
	return link.Target
}

func boundedEvidenceQuote(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > 500 {
		return string(runes[:499]) + "…"
	}
	return string(runes)
}

func splitDocumentContent(content string, limit int) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	if len([]rune(content)) <= limit {
		return []string{content}
	}
	var chunks []string
	var current []string
	currentLen := 0
	flush := func() {
		if len(current) > 0 {
			chunks = append(chunks, strings.Join(current, "\n\n"))
			current = nil
			currentLen = 0
		}
	}
	for _, paragraph := range strings.Split(content, "\n\n") {
		paragraphLen := len([]rune(paragraph))
		if paragraphLen > limit {
			flush()
			runes := []rune(paragraph)
			for len(runes) > limit {
				chunks = append(chunks, string(runes[:limit]))
				runes = runes[limit:]
			}
			if len(runes) > 0 {
				current = []string{string(runes)}
				currentLen = len(runes)
			}
			continue
		}
		if currentLen+paragraphLen+2 > limit {
			flush()
		}
		current = append(current, paragraph)
		currentLen += paragraphLen + 2
	}
	flush()
	return chunks
}
