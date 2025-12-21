package models

import (
	"testing"
)

func TestOpenAICodexProviderName(t *testing.T) {
	provider := NewOpenAICodexProvider()
	if provider.Name() != "openai-codex" {
		t.Errorf("Expected provider name 'openai-codex', got '%s'", provider.Name())
	}
}

func TestOpenAICodexSupportsModel(t *testing.T) {
	provider := NewOpenAICodexProvider()

	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{"base openai-codex", "openai-codex", true},
		{"openai-codex-o3", "openai-codex-o3", true},
		{"openai-codex-o4-mini", "openai-codex-o4-mini", true},
		{"openai-codex-mini", "openai-codex-mini", true},
		{"openai-codex-gpt-4.1", "openai-codex-gpt-4.1", true},
		{"openai-codex-gpt-4o", "openai-codex-gpt-4o", true},
		{"uppercase", "OPENAI-CODEX", true},
		{"mixed case", "OpenAI-Codex", true},
		{"openai-codex with custom model", "openai-codex-custom", true},
		{"regular openai model", "gpt-4o", false},
		{"claude model", "claude-sonnet", false},
		{"gemini model", "gemini-2.5-pro", false},
		{"empty string", "", false},
		{"partial match", "openai", false},
		{"codex only", "codex", false},
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

func TestOpenAICodexBuildArgs(t *testing.T) {
	provider := NewOpenAICodexProvider()

	tests := []struct {
		name     string
		model    string
		prompt   string
		contains []string
	}{
		{
			name:     "base model",
			model:    "openai-codex",
			prompt:   "hello",
			contains: []string{"exec", "--skip-git-repo-check", "hello"},
		},
		{
			name:     "o3 variant",
			model:    "openai-codex-o3",
			prompt:   "test",
			contains: []string{"exec", "--skip-git-repo-check", "-m", "o3", "test"},
		},
		{
			name:     "o4-mini variant",
			model:    "openai-codex-o4-mini",
			prompt:   "test",
			contains: []string{"exec", "--skip-git-repo-check", "-m", "o4-mini", "test"},
		},
		{
			name:     "mini variant",
			model:    "openai-codex-mini",
			prompt:   "test",
			contains: []string{"exec", "--skip-git-repo-check", "-m", "o4-mini", "test"},
		},
		{
			name:     "gpt-4.1 variant",
			model:    "openai-codex-gpt-4.1",
			prompt:   "test",
			contains: []string{"exec", "--skip-git-repo-check", "-m", "gpt-4.1", "test"},
		},
		{
			name:     "gpt-4o variant",
			model:    "openai-codex-gpt-4o",
			prompt:   "test",
			contains: []string{"exec", "--skip-git-repo-check", "-m", "gpt-4o", "test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := provider.buildArgs(tt.model, tt.prompt, "")
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

func TestOpenAICodexSetVerbose(t *testing.T) {
	provider := NewOpenAICodexProvider()

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

func TestOpenAICodexValidateModel(t *testing.T) {
	provider := NewOpenAICodexProvider()

	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{"openai-codex", "openai-codex", true},
		{"openai-codex-o3", "openai-codex-o3", true},
		{"gpt-4o (not supported)", "gpt-4o", false},
		{"claude-code (not supported)", "claude-code", false},
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

func TestIsOpenAICodexAvailable(t *testing.T) {
	// This test just ensures the function doesn't panic
	// The actual result depends on whether codex is installed
	available := IsOpenAICodexAvailable()
	t.Logf("OpenAI Codex binary available: %v", available)
}
