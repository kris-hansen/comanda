package codebaseindex

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A Go file larger than the old 32KB read cap used to be truncated, fail to
// parse, and fall back to regex extraction with no imports or receivers.
func TestLargeGoFileKeepsImportsAndReceivers(t *testing.T) {
	root := t.TempDir()
	var src strings.Builder
	src.WriteString("package big\n\nimport (\n\t\"context\"\n\t\"fmt\"\n)\n\ntype Engine struct{}\n\n")
	for i := 0; src.Len() < 96*1024; i++ {
		fmt.Fprintf(&src, "// Step%d does one unit of work.\nfunc (e *Engine) Step%d(ctx context.Context) error {\n\treturn fmt.Errorf(\"step %%d\", %d)\n}\n\n", i, i, i)
	}
	src.WriteString("func (e *Engine) Last() {}\n")
	if err := os.WriteFile(filepath.Join(root, "big.go"), []byte(src.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/big\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.Root = root
	manager, err := NewManager(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	scan, _, err := manager.Scan()
	if err != nil {
		t.Fatal(err)
	}
	var symbols *SymbolInfo
	for _, f := range scan.Candidates {
		if f.Path == "big.go" {
			symbols = f.Symbols
		}
	}
	if symbols == nil {
		t.Fatal("big.go has no symbols")
	}
	if len(symbols.Imports) != 2 {
		t.Fatalf("imports = %v, want context and fmt", symbols.Imports)
	}
	last := symbols.Functions[len(symbols.Functions)-1]
	if last.Name != "Last" || last.Receiver != "Engine" {
		t.Fatalf("last function = %+v, want method Last on Engine", last)
	}
}

// A syntax error must not discard what the parser did read.
func TestExtractGoSymbolsKeepsPartialASTOnSyntaxError(t *testing.T) {
	src := []byte("package svc\n\nimport \"net/http\"\n\ntype Server struct{}\n\nfunc (s *Server) Handle(w http.ResponseWriter) {}\n\nfunc broken( {\n")
	symbols, err := extractGoSymbols("svc.go", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(symbols.Imports) != 1 || symbols.Imports[0] != "net/http" {
		t.Fatalf("imports = %v, want [net/http]", symbols.Imports)
	}
	var found bool
	for _, fn := range symbols.Functions {
		if fn.Name == "Handle" && fn.Receiver == "Server" {
			found = true
		}
	}
	if !found {
		t.Fatalf("method Handle on Server not extracted: %+v", symbols.Functions)
	}
}

func writeSampleRepo(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		"go.mod":                    "module example.com/sample\n\ngo 1.21\n",
		"main.go":                   "package main\n\nfunc main() {}\n",
		"Makefile":                  "build:\n\tgo build ./...\n",
		"internal/api/handler.go":   "package api\n\nimport \"crypto/sha256\"\n\ntype Handler struct{}\n\nfunc (h *Handler) Serve() { _ = sha256.New() }\n",
		"internal/api/routes.go":    "package api\n\nfunc Routes() []string { return nil }\n",
		"internal/auth/token.go":    "package auth\n\nfunc Token() string { password := \"x\"; return password }\n",
		"internal/config/config.go": "package config\n\ntype Config struct{ Name string }\n",
		"internal/models/user.go":   "package models\n\ntype User struct{ ID int }\n",
		"internal/server/server.go": "package server\n\nfunc Start() { go func() {}() }\n",
		"utils/config/env.go":       "package config\n\nfunc Env() string { return \"\" }\n",
		"utils/models/provider.go":  "package models\n\ntype Provider interface{ Name() string }\n",
		"cmd/tool/main.go":          "package main\n\nfunc main() {}\n",
		"docs/guide.md":             "# Guide\n",
	}
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// stripVolatile removes the footer lines that legitimately differ per run.
func stripVolatile(content string) string {
	var kept []string
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "Scan time:") || strings.Contains(line, "generated at") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// Files with equal scores and randomized map iteration used to reorder (and
// recategorize) the index on every run.
func TestGenerateIsDeterministic(t *testing.T) {
	root := t.TempDir()
	writeSampleRepo(t, root)

	var first string
	for i := 0; i < 8; i++ {
		cfg := DefaultConfig()
		cfg.Root = root
		cfg.OutputPath = filepath.Join(t.TempDir(), "index.md")
		manager, err := NewManager(cfg, false)
		if err != nil {
			t.Fatal(err)
		}
		result, err := manager.Generate()
		if err != nil {
			t.Fatal(err)
		}
		got := stripVolatile(result.Content)
		if i == 0 {
			first = got
			continue
		}
		if got != first {
			t.Fatalf("run %d produced a different index than run 0", i)
		}
	}
}

// Updating after a change used to rebuild from changed files only, dropping
// symbols and the component map for everything else.
func TestGenerateIncrementalMatchesFullGeneration(t *testing.T) {
	root := t.TempDir()
	writeSampleRepo(t, root)

	cfg := DefaultConfig()
	cfg.Root = root
	cfg.OutputPath = filepath.Join(t.TempDir(), "index.md")
	manager, err := NewManager(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	initial, err := manager.Generate()
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() { println(1) }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	updater, _ := NewManager(cfg, false)
	updated, _, err := updater.GenerateIncremental(initial.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Updated {
		t.Fatal("expected the index to be regenerated after a change")
	}

	fullCfg := cfg
	fullCfg.OutputPath = filepath.Join(t.TempDir(), "full.md")
	fullManager, _ := NewManager(fullCfg, false)
	full, err := fullManager.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if stripVolatile(updated.Content) != stripVolatile(full.Content) {
		t.Fatalf("incremental update differs from a full generation:\n--- incremental ---\n%s\n--- full ---\n%s", updated.Content, full.Content)
	}
}

// Updating an encrypted index passes the stored ".enc" path back as the
// output path; it must be rewritten in place, not as ".enc.enc".
func TestEncryptedOutputPathIsStableAcrossUpdates(t *testing.T) {
	root := t.TempDir()
	writeSampleRepo(t, root)

	cfg := DefaultConfig()
	cfg.Root = root
	cfg.OutputPath = filepath.Join(t.TempDir(), "index.md")
	cfg.Encrypt = true
	cfg.EncryptionKey = "test-key"
	manager, err := NewManager(cfg, false)
	if err != nil {
		t.Fatal(err)
	}
	first, err := manager.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(first.OutputPath, "index.md.enc") {
		t.Fatalf("first output path = %q", first.OutputPath)
	}

	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() { println(2) }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg.OutputPath = first.OutputPath
	updater, _ := NewManager(cfg, false)
	second, _, err := updater.GenerateIncremental(first.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if second.OutputPath != first.OutputPath {
		t.Fatalf("update wrote %q, want %q", second.OutputPath, first.OutputPath)
	}
	if _, err := os.Stat(first.OutputPath + ".enc"); err == nil {
		t.Fatal("update created a doubly-suffixed .enc.enc file")
	}
}
