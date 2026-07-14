package models

import "testing"

func TestXAISupportsCurrentAndFutureGrokModels(t *testing.T) {
	provider := NewXAIProvider()

	tests := []struct {
		model string
		want  bool
	}{
		{model: "grok-4.5", want: true},
		{model: "grok-4.5-latest", want: true},
		{model: "grok-4.3", want: true},
		{model: "grok-latest", want: true},
		{model: "grok-5", want: true},
		{model: "not-grok-4.5", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := provider.SupportsModel(tt.model); got != tt.want {
				t.Fatalf("SupportsModel(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestDetectProviderForLatestRemoteModels(t *testing.T) {
	tests := []struct {
		model    string
		provider string
	}{
		{model: "gpt-5.6-sol", provider: "openai"},
		{model: "gpt-5.6-terra", provider: "openai"},
		{model: "gpt-5.6-luna", provider: "openai"},
		{model: "grok-4.5", provider: "xai"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			provider := defaultDetectProvider(tt.model)
			if provider == nil {
				t.Fatalf("defaultDetectProvider(%q) returned nil", tt.model)
			}
			if got := provider.Name(); got != tt.provider {
				t.Fatalf("defaultDetectProvider(%q) = %q, want %q", tt.model, got, tt.provider)
			}
		})
	}
}
