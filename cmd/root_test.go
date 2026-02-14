package cmd

import (
	"os"
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
			input:    "codebase_index:\n  root: ~/erebor/core",
			expected: "codebase_index:\n  root: " + homeDir + "/erebor/core",
		},
		{
			name:     "expands allowed_paths list",
			input:    "allowed_paths:\n  - ~/erebor/core\n  - .",
			expected: "allowed_paths:\n  - " + homeDir + "/erebor/core\n  - .",
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
