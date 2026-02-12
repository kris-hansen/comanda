package processor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePathAtParseTime(t *testing.T) {
	homeDir, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "dot resolves to cwd",
			input:    ".",
			expected: cwd,
		},
		{
			name:     "tilde resolves to home",
			input:    "~",
			expected: homeDir,
		},
		{
			name:     "tilde path resolves",
			input:    "~/test/path",
			expected: filepath.Join(homeDir, "test/path"),
		},
		{
			name:     "absolute path unchanged",
			input:    "/absolute/path",
			expected: "/absolute/path",
		},
		{
			name:     "relative path becomes absolute",
			input:    "relative/path",
			expected: filepath.Join(cwd, "relative/path"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolvePathAtParseTime(tt.input)
			if result != tt.expected {
				t.Errorf("resolvePathAtParseTime(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
