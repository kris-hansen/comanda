package knowledgegraph

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/kris-hansen/comanda/utils/codebaseindex"
	"github.com/kris-hansen/comanda/utils/semanticmemory"
)

// Bounds for file-derived annotations: trivially short sections carry no
// guidance worth attaching, and one bounded section keeps an oversized
// document from swamping the node it annotates.
const (
	minDocSectionLen    = 20
	maxDocAnnotationLen = 2000
)

// isMarkdownFile identifies files indexed by the markdown adapter. The
// language name covers extensionless context files (.cursorrules) that the
// adapter claims through its config patterns.
func isMarkdownFile(f *codebaseindex.FileEntry) bool {
	if f.Language == "markdown" {
		return true
	}
	lower := strings.ToLower(f.Path)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")
}

// withinDir reports whether p sits inside dir, where "." is the repository
// root containing everything.
func withinDir(dir, p string) bool {
	return dir == "." || p == dir || strings.HasPrefix(p, strings.TrimSuffix(dir, "/")+"/")
}

// addDocumentGuides connects recognized agent-context documents to the
// packages and components they apply to. The scoping rule is uniform and
// prefix-based: a document guides every package and component rooted at or
// below the document's own directory, so a root-level AGENTS.md guides the
// whole repository rather than special-casing the root. Ordinary markdown
// documents get no guides edges. It returns the guided node IDs per document
// path so annotation derivation can attach a document's lead section to the
// nodes it guides.
func addDocumentGuides(g *Graph, scan *codebaseindex.ScanResult, packages map[string]string, componentIDsByRoot map[string][]string) map[string][]string {
	guided := make(map[string][]string)
	for _, doc := range scan.Candidates {
		if !isMarkdownFile(doc) || !codebaseindex.IsContextFile(doc.Path) {
			continue
		}
		docID := NodeID(g.Namespace, "file:"+doc.Path)
		dir := path.Dir(doc.Path)
		targets := make(map[string]bool)
		for _, f := range scan.Candidates {
			if isMarkdownFile(f) || symbolPackage(f) == "" || !withinDir(dir, f.Path) {
				continue
			}
			targets["pkg:"+packages[f.Path]] = true
		}
		for root, ids := range componentIDsByRoot {
			if !withinDir(dir, root) {
				continue
			}
			for _, id := range ids {
				targets[LocalID(id)] = true
			}
		}
		for local := range targets {
			targetID := NodeID(g.Namespace, local)
			if g.Nodes[targetID] == nil {
				continue
			}
			g.AddEdge(docID, targetID, EdgeGuides, ConfidenceExtracted, "context scope "+dir)
			guided[doc.Path] = append(guided[doc.Path], targetID)
		}
	}
	return guided
}

// docLinkIndex maps scanned markdown documents for link resolution: exact
// repo-relative paths plus a case-insensitive basename lookup (without
// extension) for wiki-style links.
type docLinkIndex struct {
	byPath map[string]bool
	byBase map[string]string
}

func newDocLinkIndex(scan *codebaseindex.ScanResult) docLinkIndex {
	var paths []string
	for _, f := range scan.Candidates {
		if isMarkdownFile(f) {
			paths = append(paths, f.Path)
		}
	}
	// Sorted order keeps basename collisions deterministic: first path wins.
	sort.Strings(paths)
	index := docLinkIndex{byPath: make(map[string]bool, len(paths)), byBase: make(map[string]string, len(paths))}
	for _, p := range paths {
		index.byPath[p] = true
		base := strings.ToLower(path.Base(p))
		base = strings.TrimSuffix(strings.TrimSuffix(base, ".markdown"), ".md")
		if _, taken := index.byBase[base]; !taken {
			index.byBase[base] = p
		}
	}
	return index
}

// resolveDocLink maps a raw markdown import (relative link or wiki-link) to a
// document node ID. Relative targets resolve against the importing file's
// directory; wiki targets match by basename. Unresolved targets are skipped,
// mirroring how pass 4 treats imports outside the indexed set.
func (index docLinkIndex) resolve(namespace, fromPath, target string) (string, bool) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", false
	}
	cleaned := path.Clean(path.Join(path.Dir(fromPath), target))
	if index.byPath[cleaned] {
		return NodeID(namespace, "file:"+cleaned), true
	}
	for _, ext := range []string{".md", ".markdown"} {
		if index.byPath[cleaned+ext] {
			return NodeID(namespace, "file:"+cleaned+ext), true
		}
	}
	base := strings.ToLower(path.Base(target))
	base = strings.TrimSuffix(strings.TrimSuffix(base, ".markdown"), ".md")
	if p, ok := index.byBase[base]; ok {
		return NodeID(namespace, "file:"+p), true
	}
	return "", false
}

// deriveDocAnnotations reads each recognized context document from disk and
// turns its sections into file-layer annotations: every section annotates the
// document node itself, and the lead section also annotates each node the
// document guides. Content-hashed IDs keep rebuilds idempotent.
func deriveDocAnnotations(g *Graph, scan *codebaseindex.ScanResult, root string, guided map[string][]string) {
	g.Annotations = []semanticmemory.GraphAnnotation{}
	for _, f := range scan.Candidates {
		if !isMarkdownFile(f) || !codebaseindex.IsContextFile(f.Path) {
			continue
		}
		// Best-effort like the adapter's own extraction: an unreadable doc
		// contributes nodes and edges but no annotations.
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f.Path)))
		if err != nil {
			continue
		}
		sections := markdownDocSections(string(data))
		if len(sections) == 0 {
			continue
		}
		docNodeID := NodeID(g.Namespace, "file:"+f.Path)
		for _, section := range sections {
			g.Annotations = append(g.Annotations, fileAnnotation(g.Namespace, docNodeID, f.Path, section))
		}
		for _, targetID := range guided[f.Path] {
			g.Annotations = append(g.Annotations, fileAnnotation(g.Namespace, targetID, f.Path, sections[0]))
		}
	}
}

func fileAnnotation(namespace, nodeID, sourcePath, content string) semanticmemory.GraphAnnotation {
	return semanticmemory.GraphAnnotation{
		ID:         annotationID(nodeID, content),
		Namespace:  namespace,
		NodeID:     nodeID,
		Content:    content,
		Source:     semanticmemory.GraphAnnotationFile,
		SourcePath: sourcePath,
	}
}

var docSectionHeading = regexp.MustCompile(`^#{1,2}[ \t]`)

// markdownDocSections splits a document at its top-level (# and ##) headings,
// ignoring headings inside code fences. The preamble before the first heading
// is its own section. Trivially short sections are dropped and each section
// is bounded so one oversized document cannot flood a node.
func markdownDocSections(content string) []string {
	var sections []string
	var current strings.Builder
	inFence := false
	fenceMarker := ""
	flush := func() {
		section := strings.TrimSpace(current.String())
		current.Reset()
		if len(section) >= minDocSectionLen {
			sections = append(sections, boundDocAnnotation(section))
		}
	}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			marker := trimmed[:3]
			switch {
			case !inFence:
				inFence = true
				fenceMarker = marker
			case marker == fenceMarker:
				inFence = false
			}
		} else if !inFence && docSectionHeading.MatchString(line) {
			flush()
		}
		current.WriteString(line)
		current.WriteByte('\n')
	}
	flush()
	return sections
}

func boundDocAnnotation(content string) string {
	runes := []rune(content)
	if len(runes) <= maxDocAnnotationLen {
		return content
	}
	return string(runes[:maxDocAnnotationLen-1]) + "…"
}
