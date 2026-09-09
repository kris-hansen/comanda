package knowledgegraph

import (
	"fmt"
	"sort"
	"strings"

	"github.com/kris-hansen/comanda/utils/codebaseindex"
)

func semanticLocalID(f *codebaseindex.FileEntry, e codebaseindex.SemanticEntity) string {
	// Length prefixes avoid collisions between language, file and producer ID.
	prefix := fmt.Sprintf("semantic:%d:%s:", len(f.Language), f.Language)
	if e.Scope == "project" {
		return prefix + "project:" + e.ID
	}
	return prefix + fmt.Sprintf("file:%d:%s:%s", len(f.Path), f.Path, e.ID)
}

// addParserSemantics preserves typed, evidenced edges instead of encoding them
// as text in a signature. Named references never resolve to an unrelated file.
func addParserSemantics(g *Graph, scan *codebaseindex.ScanResult, packages, legacyImports map[string]string) {
	anchors := make(map[string]map[string]string)
	names := make(map[string]map[string][]string)
	packageFiles := make(map[string][]*codebaseindex.FileEntry)
	valid := make(map[string]bool)
	files := append([]*codebaseindex.FileEntry(nil), scan.Candidates...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	for _, f := range files {
		if f.Symbols == nil {
			continue
		}
		// Also guard callers constructing scans directly or loading old metadata.
		valid[f.Path] = codebaseindex.ValidateSemanticGraph(f.Symbols) == nil
		anchors[f.Path] = map[string]string{"$file": NodeID(g.Namespace, "file:"+f.Path)}
		names[f.Path] = make(map[string][]string)
		packageFiles[NodeID(g.Namespace, "pkg:"+packages[f.Path])] = append(packageFiles[NodeID(g.Namespace, "pkg:"+packages[f.Path])], f)
		for _, t := range f.Symbols.Types {
			id := NodeID(g.Namespace, fmt.Sprintf("type:%s@%s", t.Name, f.Path))
			anchors[f.Path]["$type:"+t.Name] = id
			names[f.Path]["$name:"+t.Name] = append(names[f.Path]["$name:"+t.Name], id)
		}
		for _, fn := range f.Symbols.Functions {
			name := fn.Name
			if fn.IsMethod && fn.Receiver != "" {
				name = fn.Receiver + "." + name
			}
			anchors[f.Path]["$function:"+name] = NodeID(g.Namespace, fmt.Sprintf("func:%s@%s", name, f.Path))
			names[f.Path]["$callable:"+name] = append(names[f.Path]["$callable:"+name], anchors[f.Path]["$function:"+name])
		}
		if f.Symbols.SemanticGraph == nil || !valid[f.Path] {
			continue
		}
		for _, e := range f.Symbols.SemanticGraph.Entities {
			local := semanticLocalID(f, e)
			path, pkg := f.Path, packages[f.Path]
			if e.Scope == "project" {
				path, pkg = "", ""
			}
			id := NodeID(g.Namespace, local)
			// First definition in sorted path order gives deterministic shared metadata.
			if g.Nodes[id] == nil {
				g.AddNode(local, e.Kind, e.Name, path, pkg, e.Summary)
			}
			anchors[f.Path][e.ID] = id
			if e.Referenceable {
				names[f.Path]["$name:"+e.Name] = append(names[f.Path]["$name:"+e.Name], id)
			}
		}
	}
	for _, f := range files {
		if f.Symbols == nil || f.Symbols.SemanticGraph == nil || !valid[f.Path] {
			continue
		}
		for _, imp := range f.Symbols.Imports {
			anchors[f.Path]["$import:"+imp] = resolveImport(g, g.Namespace, imp, legacyImports)
		}
		// A file's own names shadow imports; names across imports must be unique.
		imported := make(map[string]map[string]bool)
		seen := map[string]bool{f.Path: true}
		var visit func(*codebaseindex.FileEntry)
		visit = func(file *codebaseindex.FileEntry) {
			imports := file.Symbols.Imports
			if model := file.Symbols.SemanticGraph; model != nil && model.ScopeImports != nil {
				imports = model.ScopeImports
			}
			for _, imp := range imports {
				pkg := resolveImport(g, g.Namespace, imp, legacyImports)
				for _, dep := range packageFiles[pkg] {
					if seen[dep.Path] || dep.Language != f.Language {
						continue
					}
					seen[dep.Path] = true
					for name, ids := range names[dep.Path] {
						if imported[name] == nil {
							imported[name] = make(map[string]bool)
						}
						for _, id := range ids {
							imported[name][id] = true
						}
					}
					visit(dep)
				}
			}
		}
		visit(f)
		resolve := func(ref string) (string, bool) {
			if id := anchors[f.Path][ref]; id != "" {
				return id, false
			}
			if !strings.HasPrefix(ref, "$name:") && !strings.HasPrefix(ref, "$callable:") {
				return "", false
			}
			name := strings.SplitN(ref, ":", 2)[1]
			candidates := make(map[string]bool)
			for _, id := range names[f.Path][ref] {
				candidates[id] = true
			}
			if len(candidates) == 0 {
				candidates = imported[ref]
			}
			if len(candidates) == 1 {
				for id := range candidates {
					return id, true
				}
			}
			local := fmt.Sprintf("unresolved:%d:%s:%s", len(f.Path), f.Path, ref)
			summary := "Unresolved parser reference; no matching local or imported declaration"
			if len(candidates) > 1 {
				summary = "Ambiguous parser reference; multiple matching declarations"
			}
			g.AddNode(local, "unresolved_reference", name, f.Path, packages[f.Path], summary)
			return NodeID(g.Namespace, local), true
		}
		for _, r := range f.Symbols.SemanticGraph.Relations {
			source, inferredSource := resolve(r.Source)
			target, inferredTarget := resolve(r.Target)
			if source == "" || target == "" {
				continue
			}
			confidence := r.Confidence
			// A source-level read is explicit, but name binding is not compiler verified.
			if inferredSource || inferredTarget {
				confidence = ConfidenceInferred
			}
			g.AddEdge(source, target, r.Kind, confidence, r.Evidence)
		}
	}
}
