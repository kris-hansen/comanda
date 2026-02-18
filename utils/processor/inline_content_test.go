package processor

import (
	"strings"
	"testing"
)

// TestIsInlineContent verifies detection of inline content vs file paths
func TestIsInlineContent(t *testing.T) {
	config := &DSLConfig{}
	processor := NewProcessor(config, nil, nil, false, "")

	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{
			name:     "file path - absolute",
			input:    "/path/to/file.txt",
			expected: false,
		},
		{
			name:     "file path - relative",
			input:    "./relative/path.md",
			expected: false,
		},
		{
			name:     "file path - hidden file",
			input:    ".comanda/INDEX.md",
			expected: false,
		},
		{
			name:     "file path - home directory",
			input:    "~/documents/file.txt",
			expected: false,
		},
		{
			name:     "content - contains newlines",
			input:    "line1\nline2\nline3",
			expected: true,
		},
		{
			name:     "content - markdown with newlines",
			input:    "# Header\n\n- item 1\n- item 2\n\nParagraph text here.",
			expected: true,
		},
		{
			name:     "content - long string without path prefix",
			input:    strings.Repeat("This is content that looks like text. ", 20),
			expected: true,
		},
		{
			name:     "short string - not content",
			input:    "short.txt",
			expected: false,
		},
		{
			name:     "variable reference - not substituted",
			input:    "$SOME_VAR",
			expected: false,
		},
		{
			name:     "STDIN - special input",
			input:    "STDIN",
			expected: false,
		},
		{
			name:     "NA - special input",
			input:    "NA",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.isInlineContent(tt.input)
			if result != tt.expected {
				t.Errorf("isInlineContent(%q) = %v, want %v", tt.input[:min(50, len(tt.input))], result, tt.expected)
			}
		})
	}
}

// TestInlineContentFromVariableSubstitution verifies the full flow:
// 1. Variable is set with content
// 2. Input references the variable
// 3. Variable is substituted
// 4. Content is detected and handled properly
func TestInlineContentFromVariableSubstitution(t *testing.T) {
	config := &DSLConfig{}
	processor := NewProcessor(config, nil, nil, true, "")

	// Simulate codebase-index setting a variable with content
	indexContent := `# Project Index

## Files

- src/main.go
- src/util.go
- pkg/handler/handler.go

## Summary

This is a sample project index with multiple files.
The content spans multiple lines and contains markdown.
`
	processor.variables["PROJECT_INDEX"] = indexContent

	// Test that the variable gets substituted
	input := "$PROJECT_INDEX"
	substituted := processor.substituteVariables(input)

	if substituted != indexContent {
		t.Errorf("Variable substitution failed: got %d bytes, want %d bytes", len(substituted), len(indexContent))
	}

	// Test that it's detected as inline content
	if !processor.isInlineContent(substituted) {
		t.Error("Substituted variable content should be detected as inline content")
	}
}

// TestProcessInputsWithInlineContent verifies that inline content is handled
func TestProcessInputsWithInlineContent(t *testing.T) {
	config := &DSLConfig{}
	processor := NewProcessor(config, nil, nil, true, "")

	// Content that should trigger inline content handling
	content := "# Test Content\n\nThis is inline content with newlines.\n\n- Item 1\n- Item 2\n"

	// Process the content as input
	err := processor.processInputs([]string{content})
	if err != nil {
		t.Errorf("processInputs with inline content failed: %v", err)
	}

	// Verify the handler received the content
	inputs := processor.handler.GetInputs()
	if len(inputs) == 0 {
		t.Error("Expected handler to have processed the inline content")
	}
}
