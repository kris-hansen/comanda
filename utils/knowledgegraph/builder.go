package knowledgegraph

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/kris-hansen/comanda/utils/codebaseindex"
	"github.com/kris-hansen/comanda/utils/semanticmemory"
)

// minInferredNameLen bounds inferred `uses` resolution to names long enough to
// avoid noise (I, T, err, ...).
const minInferredNameLen = 3

// Build constructs a knowledge graph from a codebaseindex scan. Everything it
// emits is deterministic: structural edges are extracted from the scan, and
// `uses` edges between files and uniquely-named types are inferred from symbol
// name references.
func Build(scan *codebaseindex.ScanResult, namespace string) *Graph {
	g := NewGraph(namespace)

	// Pass 1: file and package nodes, belongs_to edges.
	for _, f := range scan.Candidates {
		summary := fileSummary(f)
		g.AddNode("file:"+f.Path, NodeFile, path.Base(f.Path), f.Path, symbolPackage(f), summary)
		if pkg := symbolPackage(f); pkg != "" {
			g.AddNode("pkg:"+pkg, NodePackage, pkg, "", pkg, "")
			g.AddEdge(NodeID(namespace, "file:"+f.Path), NodeID(namespace, "pkg:"+pkg),
				EdgeBelongsTo, ConfidenceExtracted, "package declaration")
		}
	}

	// Pass 2: symbol nodes and defines edges.
	for _, f := range scan.Candidates {
		if f.Symbols == nil {
			continue
		}
		fileID := NodeID(namespace, "file:"+f.Path)
		for _, t := range f.Symbols.Types {
			local := fmt.Sprintf("type:%s@%s", t.Name, f.Path)
			summary := strings.TrimSpace(t.Kind + " " + t.Comments)
			g.AddNode(local, NodeType, t.Name, f.Path, f.Symbols.Package, summary)
			g.AddEdge(fileID, NodeID(namespace, local), EdgeDefines, ConfidenceExtracted, "type declaration")
		}
		for _, fn := range f.Symbols.Functions {
			name := fn.Name
			if fn.IsMethod && fn.Receiver != "" {
				name = fn.Receiver + "." + fn.Name
			}
			local := fmt.Sprintf("func:%s@%s", name, f.Path)
			g.AddNode(local, NodeFunction, name, f.Path, f.Symbols.Package, strings.TrimSpace(fn.Comments))
			g.AddEdge(fileID, NodeID(namespace, local), EdgeDefines, ConfidenceExtracted, "function declaration")
		}
	}

	// Pass 3: component contains edges.
	for _, c := range scan.Components {
		g.AddNode("component:"+c.Name, NodeComponent, c.Name, c.Root, "",
			fmt.Sprintf("%s component (%d files)", c.Kind, c.FileCount))
		componentID := NodeID(namespace, "component:"+c.Name)
		for _, f := range scan.Candidates {
			if f.Path == c.Root || strings.HasPrefix(f.Path, strings.TrimSuffix(c.Root, "/")+"/") {
				g.AddEdge(componentID, NodeID(namespace, "file:"+f.Path), EdgeContains, ConfidenceExtracted, "component root")
			}
		}
	}

	// Pass 4: import edges. An import that resolves to a known local package
	// links to that package node; anything else becomes an external package
	// node so the graph keeps a record of third-party dependencies.
	for _, f := range scan.Candidates {
		if f.Symbols == nil {
			continue
		}
		fileID := NodeID(namespace, "file:"+f.Path)
		for _, imp := range f.Symbols.Imports {
			target := resolveImport(g, namespace, imp)
			g.AddEdge(fileID, target, EdgeImports, ConfidenceExtracted, "import "+imp)
		}
	}

	// Pass 5: inferred uses edges from symbol name references.
	inferUses(g, scan, namespace)

	return g
}

// resolveImport maps an import string to a package node ID, creating the
// package node when it is not defined locally.
func resolveImport(g *Graph, namespace, imp string) string {
	imp = strings.Trim(imp, "\"`")
	if imp == "" {
		return NodeID(namespace, "pkg:unknown")
	}
	// Local packages are keyed by their package clause name; the last path
	// segment of an import usually matches it.
	last := imp
	if idx := strings.LastIndex(imp, "/"); idx >= 0 {
		last = imp[idx+1:]
	}
	// Strip version suffixes like /v2 and dashed variants are kept as-is.
	if id := NodeID(namespace, "pkg:"+last); g.Nodes[id] != nil {
		return id
	}
	if id := NodeID(namespace, "pkg:"+imp); g.Nodes[id] != nil {
		return id
	}
	g.AddNode("pkg:"+imp, NodePackage, imp, "", "", "external dependency")
	return NodeID(namespace, "pkg:"+imp)
}

// inferUses links files to uniquely-named types they reference in function
// signatures, receivers, and struct fields. These edges are tagged inferred:
// the reference is a name match, not a resolved symbol.
func inferUses(g *Graph, scan *codebaseindex.ScanResult, namespace string) {
	// Registry of type names defined exactly once (ambiguous names are skipped).
	type def struct {
		nodeID  string
		defFile string
	}
	counts := make(map[string]int)
	defs := make(map[string]def)
	for _, f := range scan.Candidates {
		if f.Symbols == nil {
			continue
		}
		for _, t := range f.Symbols.Types {
			if len(t.Name) < minInferredNameLen {
				continue
			}
			counts[t.Name]++
			defs[t.Name] = def{nodeID: NodeID(namespace, fmt.Sprintf("type:%s@%s", t.Name, f.Path)), defFile: f.Path}
		}
	}

	for _, f := range scan.Candidates {
		if f.Symbols == nil {
			continue
		}
		// Normalize the signature text into exact identifier tokens once. The
		// previous approach searched every unique type name through every file's
		// text, which is quadratic for large repositories. A token set gives the
		// same whole-identifier semantics without lossy graph compression.
		tokens := identifierTokens(referenceText(f.Symbols))
		if len(tokens) == 0 {
			continue
		}
		fileID := NodeID(namespace, "file:"+f.Path)
		for name := range tokens {
			d, ok := defs[name]
			if !ok || counts[name] != 1 || d.defFile == f.Path {
				continue
			}
			g.AddEdge(fileID, d.nodeID, EdgeUses, ConfidenceInferred, "references "+name)
		}
	}
}

// referenceText flattens the parts of a file's symbols that can mention other
// types: signatures, receivers, fields, and method lists.
func referenceText(s *codebaseindex.SymbolInfo) string {
	var b strings.Builder
	for _, fn := range s.Functions {
		b.WriteString(fn.Signature)
		b.WriteString(" ")
		b.WriteString(fn.Receiver)
		b.WriteString(" ")
	}
	for _, t := range s.Types {
		for _, field := range t.Fields {
			b.WriteString(field)
			b.WriteString(" ")
		}
		for _, method := range t.Methods {
			b.WriteString(method)
			b.WriteString(" ")
		}
	}
	return b.String()
}

// identifierTokens splits language signatures and member lists into exact
// identifier-like tokens. It mirrors wordMatch's definition of a word
// character and therefore preserves inferred-edge behavior while avoiding a
// repeated full-text scan for each known type.
func identifierTokens(text string) map[string]struct{} {
	tokens := make(map[string]struct{})
	var token strings.Builder
	flush := func() {
		if token.Len() > 0 {
			tokens[token.String()] = struct{}{}
			token.Reset()
		}
	}
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			token.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return tokens
}

var wordBoundary = regexp.MustCompile(`[\p{L}\p{N}_]`)

// wordMatch reports whether name appears in text as a whole word.
func wordMatch(text, name string) bool {
	idx := 0
	for {
		i := strings.Index(text[idx:], name)
		if i < 0 {
			return false
		}
		start := idx + i
		end := start + len(name)
		leftOK := start == 0 || !wordBoundary.MatchString(text[start-1:start])
		rightOK := end == len(text) || !wordBoundary.MatchString(text[end:end+1])
		if leftOK && rightOK {
			return true
		}
		idx = start + 1
	}
}

func symbolPackage(f *codebaseindex.FileEntry) string {
	if f.Symbols == nil {
		return ""
	}
	return f.Symbols.Package
}

func fileSummary(f *codebaseindex.FileEntry) string {
	if f.Symbols == nil {
		return f.Language + " file"
	}
	parts := []string{}
	if f.Symbols.Package != "" {
		parts = append(parts, "package "+f.Symbols.Package)
	}
	if n := len(f.Symbols.Types); n > 0 {
		parts = append(parts, fmt.Sprintf("%d types", n))
	}
	if n := len(f.Symbols.Functions); n > 0 {
		parts = append(parts, fmt.Sprintf("%d functions", n))
	}
	if len(parts) == 0 {
		return f.Language + " file"
	}
	return strings.Join(parts, ", ")
}

// EnhanceFunc matches the index enhancement hook signature: it takes a prompt
// and returns the model's response.
type EnhanceFunc func(prompt string) (string, error)

// llmGraph is the JSON shape the enhancement pass is asked to produce.
type llmGraph struct {
	Nodes []struct {
		Name    string `json:"name"`
		Summary string `json:"summary"`
	} `json:"nodes"`
	Edges []struct {
		Source   string `json:"source"`
		Target   string `json:"target"`
		Kind     string `json:"kind"`
		Evidence string `json:"evidence"`
	} `json:"edges"`
}

// Enhance asks a model for concept nodes and semantic edges over the existing
// graph outline. Everything it adds is tagged inferred. A malformed response
// is an error for the caller to warn about, never a partial mutation.
func Enhance(g *Graph, fn EnhanceFunc) error {
	prompt := buildEnhancePrompt(g)
	response, err := fn(prompt)
	if err != nil {
		return fmt.Errorf("enhancement model call failed: %w", err)
	}

	var parsed llmGraph
	if err := json.Unmarshal([]byte(stripCodeFence(response)), &parsed); err != nil {
		return fmt.Errorf("enhancement response was not valid JSON: %w", err)
	}

	for _, n := range parsed.Nodes {
		name := strings.TrimSpace(n.Name)
		if name == "" {
			continue
		}
		g.AddNode("concept:"+slug(name), NodeConcept, name, "", "", strings.TrimSpace(n.Summary))
	}

	// Edges resolve endpoints by node name. Exact case matches win, then more
	// specific kinds (a type named "Store" beats a package named "store").
	byNameExact := make(map[string]string, len(g.Nodes))
	byNameLower := make(map[string]string, len(g.Nodes))
	sorted := make([]*semanticmemory.GraphNode, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		sorted = append(sorted, node)
	}
	sort.Slice(sorted, func(i, j int) bool {
		pi, pj := resolvePriority(sorted[i].Kind), resolvePriority(sorted[j].Kind)
		if pi != pj {
			return pi < pj
		}
		return sorted[i].Name < sorted[j].Name
	})
	for _, node := range sorted {
		if _, taken := byNameExact[node.Name]; !taken {
			byNameExact[node.Name] = node.ID
		}
		key := strings.ToLower(node.Name)
		if _, taken := byNameLower[key]; !taken {
			byNameLower[key] = node.ID
		}
	}
	resolveName := func(name string) (string, bool) {
		if id, ok := byNameExact[name]; ok {
			return id, true
		}
		id, ok := byNameLower[strings.ToLower(name)]
		return id, ok
	}
	for _, e := range parsed.Edges {
		kind := strings.ToLower(strings.TrimSpace(e.Kind))
		if kind != EdgeUses && kind != EdgeReference {
			continue
		}
		src, okSrc := resolveName(strings.TrimSpace(e.Source))
		tgt, okTgt := resolveName(strings.TrimSpace(e.Target))
		if !okSrc || !okTgt {
			continue
		}
		g.AddEdge(src, tgt, kind, ConfidenceInferred, strings.TrimSpace(e.Evidence))
	}
	return nil
}

// resolvePriority ranks node kinds for name resolution: lower wins.
func resolvePriority(kind string) int {
	switch kind {
	case NodeConcept:
		return 0
	case NodeType:
		return 1
	case NodeFunction:
		return 2
	case NodeComponent:
		return 3
	case NodeFile:
		return 4
	default: // package and anything else
		return 5
	}
}

// maxEnhanceNodes bounds how much of the outline is sent to the model.
const maxEnhanceNodes = 150

func buildEnhancePrompt(g *Graph) string {
	var b strings.Builder
	b.WriteString("You are enriching a knowledge graph extracted from a codebase. ")
	b.WriteString("Below are the known nodes (format: name [kind]).\n\n")
	count := 0
	for _, node := range g.Nodes {
		if count >= maxEnhanceNodes {
			fmt.Fprintf(&b, "... and %d more nodes\n", len(g.Nodes)-count)
			break
		}
		fmt.Fprintf(&b, "- %s [%s]\n", node.Name, node.Kind)
		count++
	}
	b.WriteString(`
Respond with JSON only (no prose, no code fences) in this exact shape:
{
  "nodes": [{"name": "Concept Name", "summary": "one sentence"}],
  "edges": [{"source": "Node Name", "target": "Node Name", "kind": "references", "evidence": "why"}]
}
Rules:
- "nodes" are high-level concepts, subsystems, or cross-cutting concerns (at most 10).
- "edges" connect EXISTING node names or new concept nodes; kind is "references" or "uses".
- Only add edges you are confident about.`)
	return b.String()
}

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if idx := strings.Index(s, "\n"); idx >= 0 {
			s = s[idx+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return strings.TrimSpace(s)
}

func slug(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

// ProgressEvent describes graph persistence work. Total covers nodes and
// edges, while Current identifies the graph object most recently written.
type ProgressEvent struct {
	Phase     string
	Current   string
	Completed int
	Total     int
}

// ProgressFunc receives optional graph persistence milestones.
type ProgressFunc func(ProgressEvent)

// Save persists a graph into the semantic memory store, upserting all nodes
// and edges and refreshing degree counts. It does not delete stale nodes; use
// Rebuild for a clean slate.
func Save(ctx context.Context, store *semanticmemory.Store, g *Graph) error {
	return save(ctx, store, g, nil)
}

func save(ctx context.Context, store *semanticmemory.Store, g *Graph, progress ProgressFunc) error {
	total := len(g.Nodes) + len(g.Edges)
	completed := 0
	for _, node := range g.Nodes {
		if _, err := store.UpsertGraphNode(ctx, *node); err != nil {
			return err
		}
		completed++
		if progress != nil {
			progress(ProgressEvent{Phase: "Writing graph nodes", Current: node.Name, Completed: completed, Total: total})
		}
	}
	for _, edge := range g.Edges {
		if _, err := store.UpsertGraphEdge(ctx, *edge); err != nil {
			return err
		}
		completed++
		if progress != nil {
			progress(ProgressEvent{Phase: "Writing graph edges", Current: edge.Kind, Completed: completed, Total: total})
		}
	}
	if progress != nil {
		progress(ProgressEvent{Phase: "Refreshing graph relationships", Completed: completed, Total: total})
	}
	return store.RefreshGraphDegrees(ctx, g.Namespace)
}

// Rebuild replaces the stored graph for a namespace with the given one.
func Rebuild(ctx context.Context, store *semanticmemory.Store, g *Graph) error {
	return RebuildWithProgress(ctx, store, g, nil)
}

// RebuildWithProgress replaces a graph and reports its persistence phases.
func RebuildWithProgress(ctx context.Context, store *semanticmemory.Store, g *Graph, progress ProgressFunc) error {
	nodes := make([]semanticmemory.GraphNode, 0, len(g.Nodes))
	for _, node := range g.Nodes {
		nodes = append(nodes, *node)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	edges := make([]semanticmemory.GraphEdge, 0, len(g.Edges))
	for _, edge := range g.Edges {
		edges = append(edges, *edge)
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })

	return store.ReplaceGraph(ctx, g.Namespace, nodes, edges, func(event semanticmemory.GraphWriteProgress) {
		if progress != nil {
			progress(ProgressEvent{
				Phase:     event.Phase,
				Current:   event.Current,
				Completed: event.Completed,
				Total:     event.Total,
			})
		}
	})
}
