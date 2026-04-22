package models

import (
	"path/filepath"
	"testing"
)

func TestLlamaCPPProviderName(t *testing.T) {
	provider := NewLlamaCPPProvider()
	if provider.Name() != "llama.cpp" {
		t.Fatalf("expected provider name llama.cpp, got %s", provider.Name())
	}
}

func TestLlamaCPPSupportsModel(t *testing.T) {
	provider := NewLlamaCPPProvider()

	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{"direct gguf path", "/models/tiny.gguf", true},
		{"prefixed gguf path", "llama.cpp:/models/tiny.gguf", true},
		{"alternate prefix", "llamacpp:./tiny.gguf", true},
		{"non-gguf model name", "llama3.2", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := provider.SupportsModel(tt.model); got != tt.expected {
				t.Fatalf("SupportsModel(%q) = %v, want %v", tt.model, got, tt.expected)
			}
		})
	}
}

func TestNormalizeLlamaCPPModel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"llama.cpp:/tmp/model.gguf", "/tmp/model.gguf"},
		{"llamacpp:./model.gguf", "./model.gguf"},
		{"./model.gguf", "./model.gguf"},
	}

	for _, tt := range tests {
		if got := normalizeLlamaCPPModel(tt.input); got != tt.want {
			t.Fatalf("normalizeLlamaCPPModel(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestLlamaCPPBuildArgs(t *testing.T) {
	provider := NewLlamaCPPProvider()
	modelPath := filepath.Join("/tmp", "model.gguf")
	args := provider.buildArgs(modelPath, "hello")

	expected := []string{"-m", modelPath, "--no-display-prompt", "--no-show-timings", "-n", "1024", "-st", "-p", "hello"}
	if len(args) < len(expected) {
		t.Fatalf("buildArgs returned too few args: %v", args)
	}
	for i, want := range expected {
		if args[i] != want {
			t.Fatalf("args[%d] = %q, want %q", i, args[i], want)
		}
	}
}
