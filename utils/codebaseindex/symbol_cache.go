package codebaseindex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
)

const symbolCacheVersion = 1

type symbolCacheFile struct {
	Version int                         `json:"version"`
	Root    string                      `json:"root"`
	Symbols map[string]SymbolCacheEntry `json:"symbols"`
}

// SymbolCachePath is the project-local sidecar used by graph builds to avoid
// re-extracting unchanged source symbols. It is deliberately separate from an
// index's metadata because graph builds can run without generating markdown.
func SymbolCachePath(root, namespace string) string {
	safe := regexp.MustCompile(`[^a-zA-Z0-9._-]+`).ReplaceAllString(namespace, "-")
	return filepath.Join(root, ".comanda", "memory", safe+".graph-symbols.json")
}

// LoadSymbolCache returns a cache only when it belongs to this exact root and
// schema version. A missing or malformed cache is a cache miss, never a graph
// build failure.
func LoadSymbolCache(root, namespace string) map[string]SymbolCacheEntry {
	data, err := os.ReadFile(SymbolCachePath(root, namespace))
	if err != nil {
		return nil
	}
	var cache symbolCacheFile
	if json.Unmarshal(data, &cache) != nil || cache.Version != symbolCacheVersion || cache.Root != root {
		return nil
	}
	return cache.Symbols
}

// SaveSymbolCache atomically persists the reusable extracted symbols from a
// successful graph scan.
func SaveSymbolCache(root, namespace string, candidates []*FileEntry) error {
	path := SymbolCachePath(root, namespace)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(symbolCacheFile{
		Version: symbolCacheVersion,
		Root:    root,
		Symbols: BuildSymbolCache(candidates),
	})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".graph-symbols-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
