package codebaseindex

import (
	"strings"
	"testing"
)

func TestCompressDuplicateSections(t *testing.T) {
	index1 := "## Overview\nThis is a Go project.\n\n## Key Files\nfile1.go\nfile2.go"
	index2 := "## Overview\nThis is a Go project.\n\n## Dependencies\nmodule1\nmodule2"

	result := CompressAggregatedContent([]string{index1, index2})

	// "Overview" section should appear only once
	count := strings.Count(result, "## Overview")
	if count != 1 {
		t.Errorf("expected 1 occurrence of '## Overview', got %d", count)
	}

	// Both unique sections should be present
	if !strings.Contains(result, "## Key Files") {
		t.Error("expected '## Key Files' section to be present")
	}
	if !strings.Contains(result, "## Dependencies") {
		t.Error("expected '## Dependencies' section to be present")
	}
}

func TestCompressAllUnique(t *testing.T) {
	index1 := "## Section A\nContent A"
	index2 := "## Section B\nContent B"
	index3 := "## Section C\nContent C"

	result := CompressAggregatedContent([]string{index1, index2, index3})

	if !strings.Contains(result, "## Section A") {
		t.Error("missing Section A")
	}
	if !strings.Contains(result, "## Section B") {
		t.Error("missing Section B")
	}
	if !strings.Contains(result, "## Section C") {
		t.Error("missing Section C")
	}
}

func TestCompressPreservesOrder(t *testing.T) {
	index1 := "## First\nContent 1\n\n## Shared\nShared content"
	index2 := "## Shared\nShared content\n\n## Last\nContent 2"

	result := CompressAggregatedContent([]string{index1, index2})

	firstIdx := strings.Index(result, "## First")
	sharedIdx := strings.Index(result, "## Shared")
	lastIdx := strings.Index(result, "## Last")

	if firstIdx == -1 || sharedIdx == -1 || lastIdx == -1 {
		t.Fatalf("missing sections in result: %s", result)
	}
	if firstIdx > sharedIdx {
		t.Error("First should come before Shared")
	}
	if sharedIdx > lastIdx {
		t.Error("Shared should come before Last")
	}
}

func TestCompressWhitespaceVariants(t *testing.T) {
	// Same content with different whitespace should be deduplicated
	index1 := "## Overview\nThis is a Go project."
	index2 := "## Overview\nThis  is  a  Go  project."

	result := CompressAggregatedContent([]string{index1, index2})

	count := strings.Count(result, "## Overview")
	if count != 1 {
		t.Errorf("expected 1 occurrence of '## Overview' (whitespace normalization), got %d", count)
	}
}

func TestCompressEmptyInput(t *testing.T) {
	t.Run("empty_slice", func(t *testing.T) {
		result := CompressAggregatedContent([]string{})
		if result != "" {
			t.Errorf("expected empty string, got %q", result)
		}
	})

	t.Run("single_content", func(t *testing.T) {
		input := "## Test\nContent"
		result := CompressAggregatedContent([]string{input})
		if result != input {
			t.Errorf("single content should pass through unchanged")
		}
	})
}

func TestSplitSections(t *testing.T) {
	content := "preamble text\n\n## Section 1\nContent 1\n\n## Section 2\nContent 2"
	sections := splitSections(content)

	if len(sections) != 3 {
		t.Fatalf("expected 3 sections (preamble + 2 headed), got %d", len(sections))
	}
	if !strings.Contains(sections[0], "preamble") {
		t.Error("first section should contain preamble")
	}
	if !strings.Contains(sections[1], "## Section 1") {
		t.Error("second section should contain Section 1 header")
	}
	if !strings.Contains(sections[2], "## Section 2") {
		t.Error("third section should contain Section 2 header")
	}
}
