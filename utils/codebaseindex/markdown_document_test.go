package codebaseindex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractMarkdownDocumentStructure(t *testing.T) {
	content := []byte(`---
id: guide
role: decision
title: Knowledge Guide
aliases: [Guide, Architecture Guide]
tags: [knowledge, graph]
owner: documentation
supersedes: [old-guide]
---

# Guide

Intro with [section](other.md#install), [code](main.go), and [web](https://example.com/spec#v2).

## Install

First installation passage with unique searchable prose.

## Install

Second heading must remain distinct.

Details
-------

Read [[Other Note]].

` + "```markdown\n# Not a real heading\n```\n")

	info, err := extractMarkdownDocument("docs/guide.md", content)
	if err != nil {
		t.Fatal(err)
	}
	document := info.Document
	if document == nil {
		t.Fatal("missing document structure")
	}
	if document.ID != "guide" || document.Title != "Knowledge Guide" || document.Role != "decision" {
		t.Fatalf("unexpected frontmatter identity: %#v", document)
	}
	if document.Properties["owner"] != "documentation" {
		t.Fatalf("custom property missing: %#v", document.Properties)
	}
	if len(document.Relations) != 1 || document.Relations[0].Kind != "supersedes" || document.Relations[0].Target != "old-guide" {
		t.Fatalf("relations = %#v", document.Relations)
	}

	sections := make(map[string]DocumentSection)
	for _, section := range document.Sections {
		sections[section.ID] = section
		if section.Title == "Not a real heading" {
			t.Fatal("fenced code produced a heading")
		}
	}
	for _, id := range []string{"guide", "install", "install-2", "details"} {
		if _, ok := sections[id]; !ok {
			t.Errorf("missing section %q in %#v", id, sections)
		}
	}
	if sections["guide"].StartLine != 11 {
		t.Errorf("guide starts at line %d, want 11", sections["guide"].StartLine)
	}
	if sections["install"].ParentID != "guide" || sections["details"].Level != 2 {
		t.Errorf("heading hierarchy not retained: %#v", sections)
	}
	if sections["install"].Content == sections["install-2"].Content || !strings.Contains(sections["install"].Content, "unique searchable prose") {
		t.Errorf("duplicate sections lost content: %#v", sections)
	}

	links := make(map[string]DocumentLink)
	for _, link := range document.Links {
		links[link.Kind+":"+link.Target] = link
	}
	if link := links["markdown:other.md"]; link.Fragment != "install" || link.SourceSection != "guide" {
		t.Errorf("Markdown link lost provenance: %#v", link)
	}
	if _, ok := links["file:main.go"]; !ok {
		t.Error("source-file link was not retained")
	}
	if link := links["external:https://example.com/spec"]; link.Fragment != "v2" {
		t.Errorf("external link fragment lost: %#v", link)
	}
	if link := links["markdown:Other Note"]; !link.Wiki || link.SourceSection != "details" {
		t.Errorf("wiki link not retained: %#v", link)
	}
}

func TestExtractorVersionInvalidatesSymbolCache(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "README.md")
	if err := os.WriteFile(path, []byte("# Fresh title\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entry := &FileEntry{Path: "README.md", Language: "markdown", ExtractorVersion: MarkdownParserVersion, Hash: "same", Size: 14}
	manager := &Manager{
		config: &Config{Root: root, SymbolCache: map[string]SymbolCacheEntry{
			"README.md": {
				Language: "markdown", ExtractorVersion: "legacy", Hash: "same", Size: 14,
				Symbols: &SymbolInfo{Package: "stale"},
			},
		}},
		adapters: []Adapter{&MarkdownAdapter{}},
	}
	if err := manager.extractSymbols([]*FileEntry{entry}); err != nil {
		t.Fatal(err)
	}
	if entry.Symbols == nil || entry.Symbols.Document == nil || entry.Symbols.Package != "Fresh title" {
		t.Fatalf("legacy cache was reused: %#v", entry.Symbols)
	}
}
