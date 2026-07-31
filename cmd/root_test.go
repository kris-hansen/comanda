package cmd

import (
	"os"
	"strings"
	"testing"
)

func TestExpandPathsInYAML(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skip("Could not get home directory")
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "expands root path",
			input:    "codebase_index:\n  root: ~/example-project/core",
			expected: "codebase_index:\n  root: " + homeDir + "/example-project/core",
		},
		{
			name:     "expands allowed_paths list",
			input:    "allowed_paths:\n  - ~/example-project/core\n  - .",
			expected: "allowed_paths:\n  - " + homeDir + "/example-project/core\n  - .",
		},
		{
			name:     "expands output path",
			input:    "output: ~/docs/output.md",
			expected: "output: " + homeDir + "/docs/output.md",
		},
		{
			name:     "preserves non-tilde paths",
			input:    "root: /absolute/path\nallowed_paths:\n  - .\n  - /tmp",
			expected: "root: /absolute/path\nallowed_paths:\n  - .\n  - /tmp",
		},
		{
			name:     "expands multiple occurrences",
			input:    "root: ~/a\npath: ~/b\nallowed_paths:\n  - ~/c",
			expected: "root: " + homeDir + "/a\npath: " + homeDir + "/b\nallowed_paths:\n  - " + homeDir + "/c",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := expandPathsInYAML(tt.input)
			if result != tt.expected {
				t.Errorf("expandPathsInYAML() =\n%s\nwant:\n%s", result, tt.expected)
			}
		})
	}
}

func TestGenerationPromptsExplainSemanticMemoryPlacement(t *testing.T) {
	for name, prompt := range map[string]string{
		"generate": buildGeneratePrompt("guide", "build a long improvement loop that reuses project decisions", nil, "", nil),
		"improve":  buildImprovePrompt("guide", "existing: workflow", "reuse prior decisions in the loop", nil, "", nil),
	} {
		t.Run(name, func(t *testing.T) {
			for _, want := range []string{
				"DURABLE SEMANTIC MEMORY — USE DELIBERATELY:",
				"types: [decision, constraint, failure]",
				"Never put it under agentic-loop.config",
				"memory: true is the separate legacy mode",
			} {
				if !strings.Contains(prompt, want) {
					t.Errorf("prompt missing semantic-memory guidance %q", want)
				}
			}
		})
	}
}
