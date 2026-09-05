package codebaseindex

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

// Resolve Go packages through their nearest module, including nested modules.
// Other adapters retain their package declarations and get directory scoping
// in the graph builder when a canonical import path is unavailable.
func (m *Manager) resolvePackagePaths(files []*FileEntry) {
	type module struct{ root, name string }
	modules := make(map[string]module)
	var findModule func(string) module
	findModule = func(dir string) module {
		if found, ok := modules[dir]; ok {
			return found
		}
		var found module
		if data, err := os.ReadFile(filepath.Join(m.config.Root, filepath.FromSlash(dir), "go.mod")); err == nil {
			found = module{dir, modfile.ModulePath(data)}
		} else if dir != "." {
			found = findModule(path.Dir(dir))
		}
		modules[dir] = found
		return found
	}
	for _, f := range files {
		if f.Language != "go" || f.Symbols == nil {
			continue
		}
		dir := path.Dir(filepath.ToSlash(f.Path))
		mod := findModule(dir)
		if mod.name == "" {
			continue
		}
		rel, err := filepath.Rel(filepath.FromSlash(mod.root), filepath.FromSlash(dir))
		if err == nil {
			f.PackagePath = path.Join(mod.name, filepath.ToSlash(rel))
			if strings.HasSuffix(f.Symbols.Package, "_test") {
				f.PackagePath += "_test"
			}
		}
	}
}
