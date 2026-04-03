package models

import (
	"testing"
)

func TestBedrockProvider_SupportsModel(t *testing.T) {
	provider := NewBedrockProvider()

	tests := []struct {
		modelName string
		expected  bool
	}{
		{"bedrock/anthropic.claude-3-5-sonnet-20241022-v2:0", true},
		{"bedrock/us.anthropic.claude-3-5-sonnet-20241022-v2:0", true},
		{"bedrock/us.amazon.nova-pro-v1:0", true},
		{"bedrock/us.meta.llama3-2-90b-instruct-v1:0", true},
		{"BEDROCK/anthropic.claude-3-5-sonnet-20241022-v2:0", true}, // case insensitive
		{"claude-3-5-sonnet-20241022", false},                       // no bedrock/ prefix
		{"gpt-4o", false},
		{"gemini-2.5-pro", false},
	}

	for _, tt := range tests {
		t.Run(tt.modelName, func(t *testing.T) {
			result := provider.SupportsModel(tt.modelName)
			if result != tt.expected {
				t.Errorf("SupportsModel(%q) = %v, want %v", tt.modelName, result, tt.expected)
			}
		})
	}
}

func TestBedrockProvider_ExtractModelID(t *testing.T) {
	provider := NewBedrockProvider()

	tests := []struct {
		input    string
		expected string
	}{
		{"bedrock/anthropic.claude-3-5-sonnet-20241022-v2:0", "anthropic.claude-3-5-sonnet-20241022-v2:0"},
		{"bedrock/us.amazon.nova-pro-v1:0", "us.amazon.nova-pro-v1:0"},
		{"BEDROCK/us.meta.llama3-2-90b-instruct-v1:0", "us.meta.llama3-2-90b-instruct-v1:0"},
		{"some-other-model", "some-other-model"}, // no prefix, returns as-is
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := provider.extractModelID(tt.input)
			if result != tt.expected {
				t.Errorf("extractModelID(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestBedrockProvider_MimeToImageFormat(t *testing.T) {
	provider := NewBedrockProvider()

	tests := []struct {
		mimeType string
		expected string
	}{
		{"image/jpeg", "jpeg"},
		{"image/jpg", "jpeg"},
		{"image/png", "png"},
		{"image/gif", "gif"},
		{"image/webp", "webp"},
		{"image/bmp", ""},  // unsupported
		{"text/plain", ""}, // not an image
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			result := string(provider.mimeToImageFormat(tt.mimeType))
			if result != tt.expected {
				t.Errorf("mimeToImageFormat(%q) = %q, want %q", tt.mimeType, result, tt.expected)
			}
		})
	}
}

func TestBedrockProvider_Name(t *testing.T) {
	provider := NewBedrockProvider()
	if provider.Name() != "bedrock" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "bedrock")
	}
}

func TestBedrockProvider_SetConfig(t *testing.T) {
	provider := NewBedrockProvider()

	newConfig := ModelConfig{
		Temperature: 0.5,
		MaxTokens:   4000,
		TopP:        0.9,
	}

	provider.SetConfig(newConfig)
	gotConfig := provider.GetConfig()

	if gotConfig.Temperature != newConfig.Temperature {
		t.Errorf("Temperature = %v, want %v", gotConfig.Temperature, newConfig.Temperature)
	}
	if gotConfig.MaxTokens != newConfig.MaxTokens {
		t.Errorf("MaxTokens = %v, want %v", gotConfig.MaxTokens, newConfig.MaxTokens)
	}
	if gotConfig.TopP != newConfig.TopP {
		t.Errorf("TopP = %v, want %v", gotConfig.TopP, newConfig.TopP)
	}
}
