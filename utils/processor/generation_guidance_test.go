package processor

import (
	"strings"
	"testing"
)

func TestGenerationGuideSelectsOnlyRelevantModules(t *testing.T) {
	models := []string{"openai-codex"}
	guide := GetGenerationGuideWithModels(models, "Summarize the release notes in CHANGELOG.md")

	if !strings.Contains(guide, "# Comanda YAML generation contract") {
		t.Fatal("base generation contract is missing")
	}
	for _, unexpected := range []string{"## qmd retrieval module", "## Agentic-loop module", "## Worktree module"} {
		if strings.Contains(guide, unexpected) {
			t.Errorf("simple request included irrelevant module %q", unexpected)
		}
	}
}

func TestGenerationGuideAddsRequestedModules(t *testing.T) {
	guide := GetGenerationGuideWithModels(
		[]string{"openai-codex"},
		"Use qmd to retrieve local documentation, then iterate with a quality gate using semantic memory.",
	)

	for _, want := range []string{
		"## qmd retrieval module",
		"## Agentic-loop module",
		"## Semantic-memory module",
		"Set collection only when the request supplies one",
	} {
		if !strings.Contains(guide, want) {
			t.Errorf("guide missing requested module content %q", want)
		}
	}
}

func TestGenerationGuideIsCompact(t *testing.T) {
	guide := GetGenerationGuideWithModels([]string{"openai-codex"}, "summarize a document")
	if len(guide) > 6000 {
		t.Fatalf("base generation guide is %d bytes, want at most 6000", len(guide))
	}
}
