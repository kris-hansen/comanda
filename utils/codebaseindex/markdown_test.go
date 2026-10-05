package codebaseindex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractMarkdownHeadings(t *testing.T) {
	content := []byte(`# Project Title

Some intro text.

## Getting Started

### Install

` + "```markdown\n# Not A Heading\n## Also Not\n```" + `

## Usage

Text with # a trailing hash is not a heading.

###### Deep Heading
`)

	info, err := extractMarkdownSymbols("README.md", content)
	if err != nil {
		t.Fatalf("extractMarkdownSymbols failed: %v", err)
	}

	if info.Package != "Project Title" {
		t.Errorf("Package should be first H1 'Project Title', got %q", info.Package)
	}

	got := make(map[string]string)
	for _, ti := range info.Types {
		got[ti.Name] = ti.Kind
	}
	want := map[string]string{
		"Project Title":   "h1",
		"Getting Started": "h2",
		"Install":         "h3",
		"Usage":           "h2",
		"Deep Heading":    "h6",
	}
	for name, kind := range want {
		if got[name] != kind {
			t.Errorf("heading %q kind = %q, want %q (all: %#v)", name, got[name], kind, got)
		}
	}
	if _, ok := got["Not A Heading"]; ok {
		t.Error("heading inside fenced code block should be ignored")
	}
	if _, ok := got["Also Not"]; ok {
		t.Error("heading inside fenced code block should be ignored")
	}
}

func TestExtractMarkdownTitleFallsBackToFirstLine(t *testing.T) {
	info, err := extractMarkdownSymbols("notes.md", []byte("\nScratch notes about retries.\n\nMore detail here.\n"))
	if err != nil {
		t.Fatalf("extractMarkdownSymbols failed: %v", err)
	}
	if info.Package != "Scratch notes about retries." {
		t.Errorf("Package should fall back to first non-empty line, got %q", info.Package)
	}

	long := strings.Repeat("a", 200) + "\n"
	info, err = extractMarkdownSymbols("long.md", []byte(long))
	if err != nil {
		t.Fatalf("extractMarkdownSymbols failed: %v", err)
	}
	if len(info.Package) > 120 {
		t.Errorf("Package fallback should be bounded to 120 chars, got %d", len(info.Package))
	}
}

func TestExtractMarkdownLinks(t *testing.T) {
	content := []byte(`# Notes

See the [guide](docs/guide.md) and [setup](./SETUP.md#install) for details.
External [site](https://example.com/page.md) and [anchor](#notes) are ignored.
Images like [logo](logo.png) are not markdown links.

[reference]: references/deep.markdown
See the [reference link][reference].

Wiki links: [[Other Note]] and [[page.md|Page Label]].

` + "```\n[fenced](fenced/ignored.md)\n```" + `
`)

	info, err := extractMarkdownSymbols("notes.md", content)
	if err != nil {
		t.Fatalf("extractMarkdownSymbols failed: %v", err)
	}

	imports := make(map[string]bool)
	for _, imp := range info.Imports {
		imports[imp] = true
	}

	wantPresent := []string{
		"docs/guide.md",
		"./SETUP.md",
		"references/deep.markdown",
		"Other Note",
		"page.md",
	}
	for _, want := range wantPresent {
		if !imports[want] {
			t.Errorf("missing import %q (got %v)", want, info.Imports)
		}
	}

	wantAbsent := []string{
		"https://example.com/page.md",
		"#notes",
		"logo.png",
		"fenced/ignored.md",
	}
	for _, unwanted := range wantAbsent {
		if imports[unwanted] {
			t.Errorf("unexpected import %q (got %v)", unwanted, info.Imports)
		}
	}
}

func TestIsContextFile(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"AGENTS.md", true},
		{"CLAUDE.md", true},
		{"README.md", true},
		{"readme.md", true},
		{"CONTRIBUTING.md", true},
		{"CHANGELOG.md", true},
		{".cursorrules", true},
		{".windsurfrules", true},
		{".github/copilot-instructions.md", true},
		{".cursor/rules/style.md", true},
		{"docs/guide.md", false},
		{"notes.md", false},
		{"docs/analysis/random-notes.md", false},
	}
	for _, tc := range cases {
		if got := IsContextFile(tc.path); got != tc.want {
			t.Errorf("IsContextFile(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestMarkdownScoringPrefersContextFiles(t *testing.T) {
	manager := &Manager{
		config:   &Config{},
		adapters: []Adapter{&MarkdownAdapter{}},
	}

	contextFile := &FileEntry{
		Path:         "AGENTS.md",
		Language:     "markdown",
		Depth:        0,
		IsConfig:     true,
		IsEntrypoint: true,
	}
	deepDoc := &FileEntry{
		Path:     "docs/archive/2020/notes.md",
		Language: "markdown",
		Depth:    3,
	}

	contextScore := manager.scoreFile(contextFile)
	deepScore := manager.scoreFile(deepDoc)
	if contextScore <= deepScore {
		t.Errorf("root AGENTS.md score %d should outrank deep notes.md score %d", contextScore, deepScore)
	}

	// Ordinary docs still score positively at shallow depth so small doc trees
	// are indexable, but below a recognized context file.
	shallowDoc := &FileEntry{Path: "docs/guide.md", Language: "markdown", Depth: 1}
	shallowScore := manager.scoreFile(shallowDoc)
	if shallowScore <= 0 {
		t.Errorf("shallow doc score = %d, want positive", shallowScore)
	}
	if shallowScore >= contextScore {
		t.Errorf("shallow doc score %d should stay below context file score %d", shallowScore, contextScore)
	}
}

func TestMarkdownOnlyRepoScan(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md", "# Agent Rules\n\nSee [setup](docs/setup.md).\n")
	writeFile(t, root, "docs/setup.md", "# Setup\n\nBack to [[AGENTS.md]].\n")
	writeFile(t, root, "docs/analysis/notes.md", "# Notes\n\nPlain notes.\n")

	cfg := DefaultConfig()
	cfg.Root = root
	manager, err := NewManager(cfg, false)
	if err != nil {
		t.Fatal(err)
	}

	scan, languages, err := manager.Scan()
	if err != nil {
		t.Fatalf("Scan on markdown-only repo failed: %v", err)
	}

	foundMarkdown := false
	for _, lang := range languages {
		if lang == "markdown" {
			foundMarkdown = true
		}
	}
	if !foundMarkdown {
		t.Errorf("markdown adapter not detected, languages = %v", languages)
	}

	byPath := make(map[string]*FileEntry)
	for _, f := range scan.Files {
		byPath[filepath.ToSlash(f.Path)] = f
	}
	for _, want := range []string{"AGENTS.md", "docs/setup.md", "docs/analysis/notes.md"} {
		entry, ok := byPath[want]
		if !ok {
			t.Errorf("expected %s in scan files (got %v)", want, keysOf(byPath))
			continue
		}
		if entry.Language != "markdown" {
			t.Errorf("%s language = %q, want markdown", want, entry.Language)
		}
	}

	agents := byPath["AGENTS.md"]
	if agents != nil {
		if !agents.IsConfig {
			t.Error("AGENTS.md should be flagged as config")
		}
		if agents.Symbols == nil || agents.Symbols.Package != "Agent Rules" {
			t.Errorf("AGENTS.md symbols = %#v, want package 'Agent Rules'", agents.Symbols)
		}
	}

	setup := byPath["docs/setup.md"]
	if setup != nil && setup.Symbols != nil {
		foundImport := false
		for _, imp := range setup.Symbols.Imports {
			if imp == "AGENTS.md" {
				foundImport = true
			}
		}
		if !foundImport {
			t.Errorf("docs/setup.md imports = %v, want wiki link to AGENTS.md", setup.Symbols.Imports)
		}
	}
}

func TestMarkdownAdapterDetectedAlongsideGo(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "go.mod", "module example.com/test\n")
	writeFile(t, root, "main.go", "package main\n\nfunc main() {}\n")
	writeFile(t, root, "AGENTS.md", "# Rules\n\nBe careful.\n")

	cfg := DefaultConfig()
	cfg.Root = root
	manager, err := NewManager(cfg, false)
	if err != nil {
		t.Fatal(err)
	}

	scan, languages, err := manager.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	langs := make(map[string]bool)
	for _, lang := range languages {
		langs[lang] = true
	}
	if !langs["go"] || !langs["markdown"] {
		t.Errorf("expected go and markdown adapters, got %v", languages)
	}

	var agents *FileEntry
	for _, f := range scan.Files {
		if filepath.ToSlash(f.Path) == "AGENTS.md" {
			agents = f
		}
	}
	if agents == nil {
		t.Fatal("AGENTS.md missing from scan files")
	}
	if !agents.IsConfig {
		t.Error("AGENTS.md should be flagged as config")
	}
	if agents.Language != "markdown" {
		t.Errorf("AGENTS.md language = %q, want markdown", agents.Language)
	}
}

func TestContextGuidanceSection(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "AGENTS.md", "# Agent Rules\n\nFollow these rules.\n")
	writeFile(t, root, "docs/architecture/CLAUDE.md", "# Architecture Notes\n\nLayered design.\n")
	writeFile(t, root, "docs/random.md", "# Random\n\nNot a context file.\n")

	cfg := DefaultConfig()
	cfg.Root = root
	cfg.OutputFormat = FormatStructured
	manager, err := NewManager(cfg, false)
	if err != nil {
		t.Fatal(err)
	}

	scan, _, err := manager.Scan()
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	content, err := manager.synthesizeStructured(scan)
	if err != nil {
		t.Fatalf("synthesizeStructured failed: %v", err)
	}

	for _, must := range []string{
		"## Context & Guidance",
		"### Repository Root",
		"### Subdirectories",
		"`AGENTS.md` — Agent Rules",
		"`docs/architecture/CLAUDE.md` — Architecture Notes",
	} {
		if !strings.Contains(content, must) {
			t.Errorf("index missing %q\n%s", must, content)
		}
	}

	// Non-context docs are listed elsewhere but not in this section.
	section := content[strings.Index(content, "## Context & Guidance"):]
	if end := strings.Index(section, "\n## "); end > 0 {
		section = section[:end]
	}
	if strings.Contains(section, "docs/random.md") {
		t.Errorf("non-context file should not appear in Context & Guidance section:\n%s", section)
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func keysOf(m map[string]*FileEntry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
