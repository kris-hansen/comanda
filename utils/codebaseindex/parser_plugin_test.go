package codebaseindex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestParserPluginHelper is executed in a child test process by
// TestParserPluginIndexesCustomExtension. It models a private executable
// parser without putting one in the repository.
func TestParserPluginHelper(t *testing.T) {
	if os.Getenv("COMANDA_TEST_PARSER_PLUGIN") != "1" {
		return
	}
	var request ParserPluginRequest
	if err := json.NewDecoder(os.Stdin).Decode(&request); err != nil {
		os.Exit(2)
	}
	if request.Version != 1 || request.Path == "" {
		os.Exit(3)
	}
	if len(request.Capabilities) != 1 || request.Capabilities[0] != "semantic_graph_v1" {
		os.Exit(4)
	}
	_ = json.NewEncoder(os.Stdout).Encode(ParserPluginResponse{Symbols: &SymbolInfo{
		Package: "private-context",
		Functions: []FunctionInfo{{
			Name:      "CustomDirective",
			Signature: "directive CustomDirective()",
		}},
		SemanticGraph: &SemanticGraph{
			Entities:  []SemanticEntity{{ID: "resource", Kind: "resource", Name: "Resource"}},
			Relations: []SemanticRelation{{Source: "$function:CustomDirective", Target: "resource", Kind: "reads", Confidence: "extracted", Evidence: "sample:1: directive reads resource"}},
		},
	}})
	os.Exit(0)
}

func TestParserPluginIndexesCustomExtension(t *testing.T) {
	t.Setenv("COMANDA_TEST_PARSER_PLUGIN", "1")
	root := t.TempDir()
	for _, name := range []string{"sample.ctx", "uppercase.CTX", "mixed.CtX"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("directive custom"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	cfg := DefaultConfig()
	cfg.Root = root
	cfg.ParserPlugins = []ParserPluginConfig{{
		Name:       "private-context",
		Command:    os.Args[0],
		Args:       []string{"-test.run=^TestParserPluginHelper$", "--"},
		Extensions: []string{"ctx"},
	}}
	manager, err := NewManager(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	scan, languages, err := manager.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(languages) != 1 || languages[0] != "private-context" {
		t.Fatalf("languages = %v, want private-context", languages)
	}
	if len(scan.Candidates) != 3 {
		t.Fatalf("candidates = %#v, want all three extension variants", scan.Candidates)
	}
	for _, candidate := range scan.Candidates {
		if candidate.Language != "private-context" || candidate.Symbols == nil || len(candidate.Symbols.Functions) != 1 {
			t.Fatalf("candidate = %#v, want plugin language and extracted symbols", candidate)
		}
		if got := candidate.Symbols.Functions[0].Name; got != "CustomDirective" {
			t.Fatalf("plugin function = %q, want CustomDirective", got)
		}
		if candidate.Symbols.SemanticGraph == nil || len(candidate.Symbols.SemanticGraph.Relations) != 1 {
			t.Fatal("plugin semantic relationships were not decoded")
		}
	}
}

func TestLoadParserPluginManifest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private-parser.yaml")
	manifest := []byte("name: private-template\ncommand: echo\nextensions: [templatex]\ntimeout_ms: 250\n")
	if err := os.WriteFile(path, manifest, 0600); err != nil {
		t.Fatal(err)
	}
	plugins, err := LoadParserPluginManifests([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	if len(plugins) != 1 || plugins[0].Extensions[0] != ".templatex" || plugins[0].TimeoutMS != 250 {
		t.Fatalf("plugins = %#v", plugins)
	}
}

func TestParserPluginRejectsBuiltinName(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ParserPlugins = []ParserPluginConfig{{
		Name:       "go",
		Command:    os.Args[0],
		Extensions: []string{".privatego"},
	}}
	if _, err := NewManager(cfg, false); err == nil {
		t.Fatal("expected built-in adapter conflict")
	}
}
