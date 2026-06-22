package models

import "testing"

func TestSakanaSupportsModel(t *testing.T) {
	provider := NewSakanaProvider()

	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{"fugu", "fugu", true},
		{"fugu-ultra", "fugu-ultra", true},
		{"future fugu family model", "fugu-next", true},
		{"case insensitive", "FUGU-ULTRA", true},
		{"empty string", "", false},
		{"invalid prefix", "invalid-model", false},
		{"partial match", "not-fugu", false},
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

func TestSakanaProviderConfig(t *testing.T) {
	provider := NewSakanaProvider()

	if provider.Name() != "sakana" {
		t.Errorf("Name() = %q, want sakana", provider.Name())
	}

	if err := provider.Configure(""); err == nil {
		t.Fatal("Configure(\"\") succeeded, want error")
	}

	if err := provider.Configure("test-key"); err != nil {
		t.Fatalf("Configure() failed: %v", err)
	}

	newConfig := ModelConfig{
		Temperature:         0.2,
		MaxTokens:           1234,
		MaxCompletionTokens: 1234,
		TopP:                0.9,
	}
	provider.SetConfig(newConfig)

	req := provider.createChatCompletionRequest("fugu", nil)
	if req.Model != "fugu" {
		t.Errorf("request model = %q, want fugu", req.Model)
	}
	if req.MaxTokens != newConfig.MaxTokens {
		t.Errorf("request MaxTokens = %d, want %d", req.MaxTokens, newConfig.MaxTokens)
	}
	if req.Temperature != float32(newConfig.Temperature) {
		t.Errorf("request Temperature = %v, want %v", req.Temperature, newConfig.Temperature)
	}
	if req.TopP != float32(newConfig.TopP) {
		t.Errorf("request TopP = %v, want %v", req.TopP, newConfig.TopP)
	}
}
