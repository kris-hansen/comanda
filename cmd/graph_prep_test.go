package cmd

import (
	"strings"
	"testing"
)

func TestGraphPrepContextSelectsDedicatedSection(t *testing.T) {
	index := "## AI Macro Analysis\n### Conventions\n" + strings.Repeat("detail ", 3000) + "\n### Graph Prep\napp.go reads users\n### Unknowns\nother"
	got := graphPrepContext(index)
	if got != "### Graph Prep\napp.go reads users" {
		t.Fatalf("graph prep context = %q", got)
	}
	if graphPrepContext("# ordinary index") != "" {
		t.Fatal("ordinary index should not supply graph prep")
	}
}
