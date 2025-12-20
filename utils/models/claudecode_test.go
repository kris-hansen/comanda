package models

import (
	"testing"
)

func TestClaudeCodeProviderName(t *testing.T) {
	provider := NewClaudeCodeProvider()
	if provider.Name() != "claude-code" {
		t.Errorf("Expected provider name 'claude-code', got '%s'", provider.Name())
	}
}

func TestClaudeCodeSupportsModel(t *testing.T) {
	provider := NewClaudeCodeProvider()

	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{"base claude-code", "claude-code", true},
		{"claude-code-opus", "claude-code-opus", true},
		{"claude-code-sonnet", "claude-code-sonnet", true},
		{"claude-code-haiku", "claude-code-haiku", true},
		{"uppercase", "CLAUDE-CODE", true},
		{"mixed case", "Claude-Code", true},
		{"claude-code with custom model", "claude-code-custom", true},
		{"regular claude model", "claude-4-sonnet", false},
		{"openai model", "gpt-4o", false},
		{"empty string", "", false},
		{"partial match", "claude", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.SupportsModel(tt.model)
			if result != tt.expected {
				t.Errorf("SupportsModel(%q) = %v, want %v", tt.model, result, tt.expected)
			}
		})
	}
}

func TestClaudeCodeBuildArgs(t *testing.T) {
	provider := NewClaudeCodeProvider()

	tests := []struct {
		name     string
		model    string
		prompt   string
		contains []string
	}{
		{
			name:     "base model",
			model:    "claude-code",
			prompt:   "hello",
			contains: []string{"--print", "-p", "hello"},
		},
		{
			name:     "opus variant",
			model:    "claude-code-opus",
			prompt:   "test",
			contains: []string{"--print", "--model", "claude-opus-4-5-20251101", "-p", "test"},
		},
		{
			name:     "sonnet variant",
			model:    "claude-code-sonnet",
			prompt:   "test",
			contains: []string{"--print", "--model", "claude-sonnet-4-5-20250929", "-p", "test"},
		},
		{
			name:     "haiku variant",
			model:    "claude-code-haiku",
			prompt:   "test",
			contains: []string{"--print", "--model", "claude-haiku-4-5-20251001", "-p", "test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := provider.buildArgs(tt.model, tt.prompt, "")
			argsStr := ""
			for _, arg := range args {
				argsStr += arg + " "
			}
			for _, expected := range tt.contains {
				found := false
				for _, arg := range args {
					if arg == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("buildArgs(%q, %q) missing expected arg %q, got: %v", tt.model, tt.prompt, expected, args)
				}
			}
		})
	}
}

func TestClaudeCodeSetVerbose(t *testing.T) {
	provider := NewClaudeCodeProvider()

	// Test setting verbose
	provider.SetVerbose(true)
	if !provider.verbose {
		t.Error("Expected verbose to be true")
	}

	provider.SetVerbose(false)
	if provider.verbose {
		t.Error("Expected verbose to be false")
	}
}

func TestClaudeCodeValidateModel(t *testing.T) {
	provider := NewClaudeCodeProvider()

	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{"claude-code", "claude-code", true},
		{"claude-code-opus", "claude-code-opus", true},
		{"claude-sonnet (not supported)", "claude-sonnet", false},
		{"gpt-4 (not supported)", "gpt-4", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.ValidateModel(tt.model)
			if result != tt.expected {
				t.Errorf("ValidateModel(%q) = %v, want %v", tt.model, result, tt.expected)
			}
		})
	}
}

func TestIsClaudeCodeAvailable(t *testing.T) {
	// This test just ensures the function doesn't panic
	// The actual result depends on whether claude is installed
	available := IsClaudeCodeAvailable()
	t.Logf("Claude Code binary available: %v", available)
}
