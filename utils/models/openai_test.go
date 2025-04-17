package models

import (
	"testing"
)

func TestSupportsModel(t *testing.T) {
	provider := NewOpenAIProvider()

	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		// GPT models
		{"gpt-4", "gpt-4", true},
		{"gpt-3.5-turbo", "gpt-3.5-turbo", true},
		
		// O1 models
		{"o1-pro", "o1-pro", true},
		{"o1-pro-2025-03-19", "o1-pro-2025-03-19", true},
		{"o1-preview", "o1-preview", true},
		{"o1-preview-2024-09-12", "o1-preview-2024-09-12", true},
		
		// O3 models
		{"o3-mini", "o3-mini", true},
		{"o3-mini-2025-01-31", "o3-mini-2025-01-31", true},
		
		// O4 models
		{"o4-mini", "o4-mini", true},
		{"o4-mini-2025-04-16", "o4-mini-2025-04-16", true},
		
		// GPT-4O variants
		{"gpt-4o-mini-realtime-preview", "gpt-4o-mini-realtime-preview", true},
		{"gpt-4o-mini-search-preview", "gpt-4o-mini-search-preview", true},
		{"gpt-4o-mini-audio-preview", "gpt-4o-mini-audio-preview", true},
		{"gpt-4o-mini-tts", "gpt-4o-mini-tts", true},
		{"gpt-4o-transcribe", "gpt-4o-transcribe", true},
		{"gpt-4o-mini-transcribe", "gpt-4o-mini-transcribe", true},
		{"gpt-4.1-mini", "gpt-4.1-mini", true},
		{"gpt-4.1-nano", "gpt-4.1-nano", true},
		
		// Invalid models
		{"empty string", "", false},
		{"invalid prefix", "invalid-model", false},
		{"o5 prefix", "o5-model", false},
		{"partial match", "not-gpt-4", false},
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

func TestIsNewModelSeries(t *testing.T) {
	provider := NewOpenAIProvider()

	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		// New series models
		{"o4-mini", "o4-mini", true},
		{"o4-mini-2025-04-16", "o4-mini-2025-04-16", true},
		{"o1-pro", "o1-pro", true},
		{"o1-preview", "o1-preview", true},
		{"o3-mini", "o3-mini", true},
		{"gpt-4o-mini", "gpt-4o-mini", true},
		
		// Legacy models
		{"gpt-4", "gpt-4", false},
		{"gpt-3.5-turbo", "gpt-3.5-turbo", false},
		{"invalid model", "invalid-model", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.isNewModelSeries(tt.model)
			if result != tt.expected {
				t.Errorf("isNewModelSeries(%q) = %v, want %v", tt.model, result, tt.expected)
			}
		})
	}
}
