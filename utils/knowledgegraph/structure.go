package knowledgegraph

import (
	"fmt"
	"path"

	"github.com/kris-hansen/comanda/utils/codebaseindex"
)

func legacyImportPaths(scan *codebaseindex.ScanResult, packages map[string]string, namespace string) map[string]string {
	paths := make(map[string]string)
	for _, f := range scan.Candidates {
		if f.PackagePath != "" || symbolPackage(f) == "" || path.Dir(f.Path) == "." {
			continue
		}
		dir := path.Dir(f.Path)
		id := NodeID(namespace, "pkg:"+packages[f.Path])
		if previous, exists := paths[dir]; exists && previous != id {
			paths[dir] = ""
		} else if !exists {
			paths[dir] = id
		}
	}
	return paths
}

func packageKeys(files []*codebaseindex.FileEntry) map[string]string {
	roots := make(map[string]map[string]bool)
	for _, f := range files {
		pkg := symbolPackage(f)
		if roots[pkg] == nil {
			roots[pkg] = make(map[string]bool)
		}
		roots[pkg][f.Language+":"+path.Dir(f.Path)] = true
	}
	keys := make(map[string]string, len(files))
	for _, f := range files {
		key := f.PackagePath
		if key == "" {
			key = symbolPackage(f)
			if len(roots[key]) > 1 {
				key += "@" + f.Language + ":" + path.Dir(f.Path)
			}
		}
		keys[f.Path] = key
	}
	return keys
}

// A receiver type can live in another file in the same package. Ambiguous
// declarations are skipped instead of connecting a method to an arbitrary type.
func linkMethods(g *Graph, scan *codebaseindex.ScanResult, namespace string, packages map[string]string) {
	types := make(map[string][]string)
	for _, f := range scan.Candidates {
		if f.Symbols == nil {
			continue
		}
		for _, t := range f.Symbols.Types {
			key := packages[f.Path] + "|" + t.Name
			types[key] = append(types[key], NodeID(namespace, fmt.Sprintf("type:%s@%s", t.Name, f.Path)))
		}
	}
	for _, f := range scan.Candidates {
		if f.Symbols == nil {
			continue
		}
		for _, fn := range f.Symbols.Functions {
			if !fn.IsMethod || fn.Receiver == "" {
				continue
			}
			owners := types[packages[f.Path]+"|"+fn.Receiver]
			if len(owners) == 1 {
				id := NodeID(namespace, fmt.Sprintf("func:%s.%s@%s", fn.Receiver, fn.Name, f.Path))
				g.AddEdge(owners[0], id, EdgeDefines, ConfidenceExtracted, "method receiver "+fn.Receiver)
			}
		}
	}
}
