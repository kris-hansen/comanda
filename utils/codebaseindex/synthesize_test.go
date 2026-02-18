package codebaseindex

import (
	"strings"
	"testing"
)

// TestOutputFormats verifies that different output formats produce
// appropriately sized and structured content
func TestOutputFormats(t *testing.T) {
	// Create a mock scan result
	scan := createMockScanResult()

	tests := []struct {
		name           string
		format         OutputFormat
		minLen         int
		maxLen         int
		mustContain    []string
		mustNotContain []string
	}{
		{
			name:    "summary format is compact",
			format:  FormatSummary,
			minLen:  100,
			maxLen:  5000, // ~3KB max for summary
			mustContain: []string{
				"Quick Reference",
			},
			mustNotContain: []string{
				"Important Files", // Full details not in summary
			},
		},
		{
			name:    "structured format has categories",
			format:  FormatStructured,
			minLen:  100,
			maxLen:  100000, // up to ~80KB for structured
			mustContain: []string{
				"File Categories",
				"Repository Layout",
			},
		},
		{
			name:    "full format has all sections",
			format:  FormatFull,
			minLen:  100,
			maxLen:  300000, // up to ~250KB for full
			mustContain: []string{
				"Repository Layout",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig()
			config.OutputFormat = tt.format
			config.RepoFileSlug = "testproject"
			config.RepoVarSlug = "TESTPROJECT"

			manager := &Manager{
				config:  config,
				adapters: []Adapter{&mockAdapter{name: "Go"}},
			}

			content, err := manager.synthesize(scan)
			if err != nil {
				t.Fatalf("synthesize failed: %v", err)
			}

			// Check length bounds
			if len(content) < tt.minLen {
				t.Errorf("content too short: got %d bytes, want at least %d", len(content), tt.minLen)
			}
			if len(content) > tt.maxLen {
				t.Errorf("content too long: got %d bytes, want at most %d", len(content), tt.maxLen)
			}

			// Check required content
			for _, must := range tt.mustContain {
				if !strings.Contains(content, must) {
					t.Errorf("content missing required text: %q", must)
				}
			}

			// Check excluded content
			for _, mustNot := range tt.mustNotContain {
				if strings.Contains(content, mustNot) {
					t.Errorf("content contains unexpected text: %q", mustNot)
				}
			}
		})
	}
}

// TestCategorizeFiles verifies that files are correctly categorized by domain
func TestCategorizeFiles(t *testing.T) {
	config := DefaultConfig()
	config.RepoFileSlug = "testproject"

	manager := &Manager{
		config: config,
	}

	scan := &ScanResult{
		Candidates: []*FileEntry{
			{Path: "api/handlers/user.go"},
			{Path: "web/components/Button.tsx"},
			{Path: "frontend/pages/Home.tsx"},
			{Path: "db/migrations/001_init.sql"},
			{Path: "models/user.go"},
			{Path: "cmd/main.go"},
			{Path: "utils/helpers.go"},
			{Path: "tests/integration_test.go"},
			{Path: "scripts/deploy.sh"},
			{Path: "docs/README.md"},
			{Path: "random_file.txt"},
		},
	}

	categories := manager.categorizeFiles(scan)

	// Verify key files are categorized correctly
	// Note: Order of category matching can vary, so we check specific clear-cut cases
	testCases := []struct {
		file         string
		wantCategory string
	}{
		{"api/handlers/user.go", "Backend / API"},
		{"web/components/Button.tsx", "Frontend / UI"},
		{"db/migrations/001_init.sql", "Database / Storage"},
		{"models/user.go", "Domain / Models"},
		{"cmd/main.go", "CLI / Commands"},
		{"utils/helpers.go", "Utilities / Helpers"},
		{"tests/integration_test.go", "Testing"},
		{"docs/README.md", "Documentation"},
		{"random_file.txt", "Other"},
	}

	for _, tc := range testCases {
		actualFiles, exists := categories[tc.wantCategory]
		if !exists {
			t.Errorf("missing category: %s", tc.wantCategory)
			continue
		}

		found := false
		for _, f := range actualFiles {
			if f == tc.file {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("file %q not found in category %q (got: %v)", tc.file, tc.wantCategory, actualFiles)
		}
	}

	// Verify no files are lost (all files should be in some category)
	totalCategorized := 0
	for _, files := range categories {
		totalCategorized += len(files)
	}
	if totalCategorized != len(scan.Candidates) {
		t.Errorf("categorized %d files, want %d", totalCategorized, len(scan.Candidates))
	}
}

// TestSynthesizeSummary verifies summary format specifics
func TestSynthesizeSummary(t *testing.T) {
	config := DefaultConfig()
	config.OutputFormat = FormatSummary
	config.RepoFileSlug = "myproject"
	config.RepoVarSlug = "MYPROJECT"

	manager := &Manager{
		config:   config,
		adapters: []Adapter{&mockAdapter{name: "Go"}, &mockAdapter{name: "TypeScript"}},
	}

	scan := createMockScanResult()
	content, err := manager.synthesizeSummary(scan)
	if err != nil {
		t.Fatalf("synthesizeSummary failed: %v", err)
	}

	// Should mention both languages
	if !strings.Contains(content, "Go") || !strings.Contains(content, "TypeScript") {
		t.Error("summary should list detected languages")
	}

	// Should have quick reference header
	if !strings.Contains(content, "Quick Reference") {
		t.Error("summary should have 'Quick Reference' header")
	}

	// Should suggest qmd for more details
	if !strings.Contains(content, "qmd") {
		t.Error("summary should mention qmd for detailed exploration")
	}
}

// TestSynthesizeStructured verifies structured format has all category sections
func TestSynthesizeStructured(t *testing.T) {
	config := DefaultConfig()
	config.OutputFormat = FormatStructured
	config.RepoFileSlug = "myproject"
	config.RepoVarSlug = "MYPROJECT"

	manager := &Manager{
		config:   config,
		adapters: []Adapter{&mockAdapter{name: "Go"}},
	}

	scan := &ScanResult{
		TotalFiles: 100,
		Candidates: []*FileEntry{
			{Path: "api/handler.go"},
			{Path: "web/app.tsx"},
			{Path: "db/queries.go"},
		},
		DirTree: &DirNode{
			Name: ".",
			Children: []*DirNode{
				{Name: "api"},
				{Name: "web"},
				{Name: "db"},
			},
		},
	}

	content, err := manager.synthesizeStructured(scan)
	if err != nil {
		t.Fatalf("synthesizeStructured failed: %v", err)
	}

	// Should have File Categories section
	if !strings.Contains(content, "File Categories") {
		t.Error("structured format should have 'File Categories' section")
	}

	// Should have organized description
	if !strings.Contains(content, "organized by domain") {
		t.Error("structured format should mention domain organization")
	}
}

// TestDefaultFormatIsStructured verifies the default format is structured
func TestDefaultFormatIsStructured(t *testing.T) {
	if DefaultFormat != FormatStructured {
		t.Errorf("DefaultFormat = %v, want %v", DefaultFormat, FormatStructured)
	}
}

// Helper types and functions

type mockAdapter struct {
	name string
}

func (m *mockAdapter) Name() string {
	return m.name
}

func (m *mockAdapter) DetectionFiles() []string {
	return []string{"go.mod"}
}

func (m *mockAdapter) FileExtensions() []string {
	return []string{".go"}
}

func (m *mockAdapter) IgnoreDirs() []string {
	return nil
}

func (m *mockAdapter) IgnoreGlobs() []string {
	return nil
}

func (m *mockAdapter) EntrypointPatterns() []string {
	return []string{"main.go"}
}

func (m *mockAdapter) ConfigPatterns() []string {
	return []string{"go.mod"}
}

func (m *mockAdapter) ExtractSymbols(path string, content []byte) (*SymbolInfo, error) {
	return nil, nil
}

func (m *mockAdapter) ScoreFile(path string, depth int, isEntrypoint, isConfig bool) int {
	return 50
}

func createMockScanResult() *ScanResult {
	return &ScanResult{
		TotalFiles: 50,
		TotalDirs:  10,
		Candidates: []*FileEntry{
			{Path: "cmd/main.go", IsEntrypoint: true},
			{Path: "api/handlers/user.go"},
			{Path: "api/handlers/auth.go"},
			{Path: "internal/service/user.go"},
			{Path: "web/components/App.tsx"},
			{Path: "db/migrations/init.sql"},
		},
		DirTree: &DirNode{
			Name: ".",
			Children: []*DirNode{
				{Name: "cmd", Files: []string{"main.go"}},
				{Name: "api", Children: []*DirNode{{Name: "handlers"}}},
				{Name: "internal", Children: []*DirNode{{Name: "service"}}},
				{Name: "web", Children: []*DirNode{{Name: "components"}}},
				{Name: "db", Children: []*DirNode{{Name: "migrations"}}},
			},
		},
	}
}
