package models

import (
	"testing"
)

func TestMoonshotSupportsModel(t *testing.T) {
	provider := NewMoonshotProvider()

	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		// Moonshot models
		{"moonshot-v1-8k", "moonshot-v1-8k", true},
		{"moonshot-v1-32k", "moonshot-v1-32k", true},
		{"moonshot-v1-128k", "moonshot-v1-128k", true},
		{"moonshot-v1-auto", "moonshot-v1-auto", true},

		// Future models with the same prefix should also be supported
		{"moonshot-v2-future", "moonshot-v2-future", true},

		// Invalid models
		{"empty string", "", false},
		{"invalid prefix", "invalid-model", false},
		{"partial match", "not-moonshot-v1", false},
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

func TestMoonshotTemperatureClamp(t *testing.T) {
	provider := NewMoonshotProvider()

	// Test default temperature
	config := provider.GetConfig()
	if config.Temperature != 0.3 {
		t.Errorf("Default temperature = %v, want 0.3", config.Temperature)
	}

	// Test temperature clamping
	newConfig := ModelConfig{
		Temperature:         1.5, // Should be clamped to 1.0
		MaxTokens:           2000,
		MaxCompletionTokens: 2000,
		TopP:                1.0,
	}
	provider.SetConfig(newConfig)

	// Create a request to test temperature clamping
	req := provider.createChatCompletionRequest("moonshot-v1-8k", nil)

	if req.Temperature > 1.0 {
		t.Errorf("Temperature = %v, want <= 1.0", req.Temperature)
	}
}
