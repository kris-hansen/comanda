package codebaseindex

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

var componentMarkers = map[string]string{
	"go.mod":              "go",
	"package.json":        "typescript",
	"tsconfig.json":       "typescript",
	"pyproject.toml":      "python",
	"requirements.txt":    "python",
	"setup.py":            "python",
	"Pipfile":             "python",
	"pubspec.yaml":        "flutter",
	"pom.xml":             "java",
	"build.gradle":        "java",
	"build.gradle.kts":    "java",
	"settings.gradle":     "java",
	"settings.gradle.kts": "java",
}

var frontendFrameworks = map[string]bool{
	"react": true, "vue": true, "angular": true, "svelte": true, "nextjs": true,
}

var backendFrameworks = map[string]bool{
	"gin": true, "echo": true, "fiber": true, "gorilla": true, "stdlib-http": true,
	"grpc": true, "graphql": true, "express": true, "fastify": true, "nestjs": true,
	"flask": true, "django": true, "fastapi": true, "starlette": true, "spring": true,
}

// analyzeComponents infers macro component boundaries after symbol extraction.
// It deliberately favors manifest/config roots over arbitrary folder names so a
// large monorepo can expose its actual frontend/backend/package split.
func (m *Manager) analyzeComponents(scan *ScanResult) {
	if scan == nil {
		return
	}

	components := map[string]*CodebaseComponent{}
	rootMarkerCount := 0
	markerRoots := map[string]bool{}
	languages := map[string]bool{}

	for _, f := range allScanFiles(scan) {
		base := filepath.Base(f.Path)
		lang, ok := componentMarkers[base]
		if !ok {
			continue
		}

		root := filepath.ToSlash(filepath.Dir(f.Path))
		if root == "." {
			root = "."
		} else {
			rootMarkerCount++
		}
		markerRoots[root] = true
		languages[lang] = true

		key := root + "\x00" + lang
		c := components[key]
		if c == nil {
			c = &CodebaseComponent{
				Name:     componentName(root, lang),
				Root:     root,
				Language: lang,
				Kind:     "unknown",
			}
			components[key] = c
		}
		c.ConfigFiles = appendUniqueString(c.ConfigFiles, filepath.ToSlash(f.Path))
		c.Evidence = appendUniqueString(c.Evidence, filepath.ToSlash(f.Path))
	}

	// Fallback for single-language repos without included manifest candidates.
	if len(components) == 0 {
		for _, f := range allScanFiles(scan) {
			if f.Language == "" {
				continue
			}
			key := ".\x00" + f.Language
			if components[key] == nil {
				components[key] = &CodebaseComponent{Name: componentName(".", f.Language), Root: ".", Language: f.Language, Kind: "unknown"}
			}
			languages[f.Language] = true
		}
	}

	for _, c := range components {
		m.populateComponentDetails(scan, c)
		c.Kind = inferComponentKind(c)
	}

	result := make([]*CodebaseComponent, 0, len(components))
	for _, c := range components {
		result = append(result, c)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Root == result[j].Root {
			return result[i].Language < result[j].Language
		}
		return result[i].Root < result[j].Root
	})

	scan.Components = result
	scan.IsMonorepo = len(result) > 1 || rootMarkerCount > 0 || len(markerRoots) > 1 || len(languages) > 1
}

func (m *Manager) populateComponentDetails(scan *ScanResult, c *CodebaseComponent) {
	seenDirs := map[string]bool{}
	for _, f := range allScanFiles(scan) {
		if !pathInComponent(f.Path, c.Root) {
			continue
		}
		if f.Language == c.Language || f.Language == "" || f.IsConfig {
			c.FileCount++
		}
		if f.IsEntrypoint {
			c.EntryPoints = appendUniqueString(c.EntryPoints, filepath.ToSlash(f.Path))
			c.Evidence = appendUniqueString(c.Evidence, filepath.ToSlash(f.Path))
		}
		if f.Symbols != nil {
			for _, fw := range f.Symbols.Frameworks {
				c.Frameworks = appendUniqueString(c.Frameworks, fw)
			}
		}
		if dir := componentRelativeTopDir(f.Path, c.Root); dir != "" && !seenDirs[dir] {
			seenDirs[dir] = true
			c.KeyDirs = append(c.KeyDirs, dir)
		}
	}
	sort.Strings(c.Frameworks)
	sort.Strings(c.ConfigFiles)
	sort.Strings(c.EntryPoints)
	sort.Strings(c.KeyDirs)
	if len(c.KeyDirs) > 8 {
		c.KeyDirs = c.KeyDirs[:8]
	}
	if len(c.Evidence) > 8 {
		c.Evidence = c.Evidence[:8]
	}
}

func pathInComponent(path, root string) bool {
	path = filepath.ToSlash(path)
	root = filepath.ToSlash(root)
	if root == "." || root == "" {
		return true
	}
	return path == root || strings.HasPrefix(path, root+"/")
}

func componentRelativeTopDir(path, root string) string {
	path = filepath.ToSlash(path)
	root = filepath.ToSlash(root)
	if root != "." && strings.HasPrefix(path, root+"/") {
		path = strings.TrimPrefix(path, root+"/")
	}
	parts := strings.Split(path, "/")
	if len(parts) <= 1 {
		return ""
	}
	return parts[0] + "/"
}

func componentName(root, language string) string {
	if root == "." || root == "" {
		return fmt.Sprintf("root %s component", language)
	}
	return filepath.Base(root)
}

func inferComponentKind(c *CodebaseComponent) string {
	root := strings.ToLower(c.Root)
	name := strings.ToLower(c.Name)
	for _, fw := range c.Frameworks {
		fw = strings.ToLower(fw)
		if frontendFrameworks[fw] {
			return "frontend"
		}
	}
	for _, fw := range c.Frameworks {
		fw = strings.ToLower(fw)
		if backendFrameworks[fw] {
			return "backend"
		}
	}
	if c.Language == "flutter" || strings.Contains(root, "mobile") || strings.Contains(name, "mobile") || strings.Contains(root, "app") {
		return "mobile"
	}
	if strings.Contains(root, "frontend") || strings.Contains(root, "web") || strings.Contains(root, "client") || strings.Contains(root, "ui") {
		return "frontend"
	}
	if strings.Contains(root, "backend") || strings.Contains(root, "api") || strings.Contains(root, "server") || strings.Contains(root, "service") || strings.Contains(root, "worker") {
		return "backend"
	}
	if c.Language == "go" {
		for _, ep := range c.EntryPoints {
			if strings.HasPrefix(ep, "cmd/") || strings.Contains(ep, "/cmd/") {
				return "cli"
			}
		}
	}
	if strings.Contains(root, "shared") || strings.Contains(root, "common") || strings.Contains(root, "pkg") || strings.Contains(root, "lib") {
		return "shared-library"
	}
	if strings.Contains(root, "infra") || strings.Contains(root, "deploy") || strings.Contains(root, "terraform") {
		return "infrastructure"
	}
	return "unknown"
}

func appendUniqueString(items []string, item string) []string {
	if item == "" {
		return items
	}
	for _, existing := range items {
		if existing == item {
			return items
		}
	}
	return append(items, item)
}
