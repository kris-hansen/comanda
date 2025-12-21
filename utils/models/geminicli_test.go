package models

import (
	"testing"
)

func TestGeminiCLIProviderName(t *testing.T) {
	provider := NewGeminiCLIProvider()
	if provider.Name() != "gemini-cli" {
		t.Errorf("Expected provider name 'gemini-cli', got '%s'", provider.Name())
	}
}

func TestGeminiCLISupportsModel(t *testing.T) {
	provider := NewGeminiCLIProvider()

	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{"base gemini-cli", "gemini-cli", true},
		{"gemini-cli-pro", "gemini-cli-pro", true},
		{"gemini-cli-flash", "gemini-cli-flash", true},
		{"gemini-cli-flash-lite", "gemini-cli-flash-lite", true},
		{"uppercase", "GEMINI-CLI", true},
		{"mixed case", "Gemini-CLI", true},
		{"gemini-cli with custom model", "gemini-cli-custom", true},
		{"regular gemini model", "gemini-2.5-pro", false},
		{"openai model", "gpt-4o", false},
		{"claude model", "claude-sonnet", false},
		{"empty string", "", false},
		{"partial match", "gemini", false},
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

func TestGeminiCLIBuildArgs(t *testing.T) {
	provider := NewGeminiCLIProvider()

	tests := []struct {
		name     string
		model    string
		prompt   string
		contains []string
	}{
		{
			name:     "base model",
			model:    "gemini-cli",
			prompt:   "hello",
			contains: []string{"-p", "hello"},
		},
		{
			name:     "pro variant",
			model:    "gemini-cli-pro",
			prompt:   "test",
			contains: []string{"-m", "gemini-2.5-pro", "-p", "test"},
		},
		{
			name:     "flash variant",
			model:    "gemini-cli-flash",
			prompt:   "test",
			contains: []string{"-m", "gemini-2.5-flash", "-p", "test"},
		},
		{
			name:     "flash-lite variant",
			model:    "gemini-cli-flash-lite",
			prompt:   "test",
			contains: []string{"-m", "gemini-2.5-flash-lite", "-p", "test"},
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

func TestGeminiCLISetVerbose(t *testing.T) {
	provider := NewGeminiCLIProvider()

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

func TestGeminiCLIValidateModel(t *testing.T) {
	provider := NewGeminiCLIProvider()

	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{"gemini-cli", "gemini-cli", true},
		{"gemini-cli-pro", "gemini-cli-pro", true},
		{"gemini-2.5-pro (not supported)", "gemini-2.5-pro", false},
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

func TestIsGeminiCLIAvailable(t *testing.T) {
	// This test just ensures the function doesn't panic
	// The actual result depends on whether gemini is installed
	available := IsGeminiCLIAvailable()
	t.Logf("Gemini CLI binary available: %v", available)
}
