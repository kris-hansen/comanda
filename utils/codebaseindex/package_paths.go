package codebaseindex

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
)

// Resolve Go packages through their nearest module, including nested modules.
// Other adapters retain their package declarations and get directory scoping
// in the graph builder when a canonical import path is unavailable.
func (m *Manager) resolvePackagePaths(files []*FileEntry) error {
	root, err := os.OpenRoot(m.config.Root)
	if err != nil {
		return fmt.Errorf("open index root for module resolution: %w", err)
	}
	defer root.Close()
	type module struct{ root, name string }
	modules := make(map[string]module)
	var findModule func(string) (module, error)
	findModule = func(dir string) (module, error) {
		if found, ok := modules[dir]; ok {
			return found, nil
		}
		var found module
		modulePath := filepath.Join(filepath.FromSlash(dir), "go.mod")
		if data, err := readModuleInRoot(root, modulePath); err == nil {
			found = module{dir, modfile.ModulePath(data)}
		} else if !os.IsNotExist(err) {
			return module{}, fmt.Errorf("read module %q within index root: %w", modulePath, err)
		} else if dir != "." {
			found, err = findModule(path.Dir(dir))
			if err != nil {
				return module{}, err
			}
		}
		modules[dir] = found
		return found, nil
	}
	for _, f := range files {
		if f.Language != "go" || f.Symbols == nil {
			continue
		}
		if !filepath.IsLocal(f.Path) {
			return fmt.Errorf("source path %q escapes the index root", f.Path)
		}
		dir := path.Dir(filepath.ToSlash(f.Path))
		mod, err := findModule(dir)
		if err != nil {
			return err
		}
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
	return nil
}

func readModuleInRoot(root *os.Root, name string) ([]byte, error) {
	data, err := root.ReadFile(name)
	if err == nil || os.IsNotExist(err) {
		return data, err
	}
	// os.Root rejects absolute symlinks, including safe in-tree targets. Resolve
	// that spelling, then read its relative target through the same confined
	// handle so a subsequent symlink swap cannot redirect the read outside it.
	canonicalRoot, rootErr := filepath.EvalSymlinks(root.Name())
	if rootErr != nil {
		return nil, err
	}
	target, targetErr := filepath.EvalSymlinks(filepath.Join(root.Name(), name))
	if targetErr != nil {
		return nil, err
	}
	relative, relErr := filepath.Rel(canonicalRoot, target)
	if relErr != nil || !filepath.IsLocal(relative) {
		return nil, err
	}
	return root.ReadFile(relative)
}
