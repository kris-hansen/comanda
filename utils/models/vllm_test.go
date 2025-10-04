package models

import (
	"testing"
)

func TestVLLMProviderName(t *testing.T) {
	provider := NewVLLMProvider()
	if provider.Name() != "vllm" {
		t.Errorf("Expected provider name 'vllm', got '%s'", provider.Name())
	}
}

func TestVLLMSupportsModel(t *testing.T) {
	provider := NewVLLMProvider()

	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		// vLLM can support any model - it's determined by what's loaded on the server
		{"any model name", "llama-2-7b", true},
		{"another model", "mistral-7b-instruct", true},
		{"gpt-style name", "gpt-oss-20b", true},
		{"empty string", "", true}, // Even empty returns true, server check happens elsewhere
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

func TestVLLMConfigure(t *testing.T) {
	provider := NewVLLMProvider()

	tests := []struct {
		name      string
		apiKey    string
		shouldErr bool
	}{
		{"valid LOCAL key", "LOCAL", false},
		{"invalid key", "some-api-key", true},
		{"empty key", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := provider.Configure(tt.apiKey)
			if (err != nil) != tt.shouldErr {
				t.Errorf("Configure(%q) error = %v, shouldErr %v", tt.apiKey, err, tt.shouldErr)
			}
		})
	}
}

func TestVLLMSetVerbose(t *testing.T) {
	provider := NewVLLMProvider()

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

func TestVLLMEndpoint(t *testing.T) {
	provider := NewVLLMProvider()

	// Test default endpoint
	endpoint := provider.getVLLMEndpoint()
	if endpoint != "http://localhost:8000" {
		t.Errorf("Expected default endpoint 'http://localhost:8000', got '%s'", endpoint)
	}
}

func TestVLLMValidateModel(t *testing.T) {
	provider := NewVLLMProvider()

	// Since ValidateModel checks the registry first and we don't have models registered,
	// it should fall back to SupportsModel which always returns true for vLLM
	tests := []struct {
		name  string
		model string
	}{
		{"llama model", "llama-2-7b"},
		{"mistral model", "mistral-7b-instruct"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ValidateModel should return true for any model name
			result := provider.ValidateModel(tt.model)
			if !result {
				t.Errorf("ValidateModel(%q) = false, want true", tt.model)
			}
		})
	}
}
