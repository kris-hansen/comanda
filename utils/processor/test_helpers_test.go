package processor

import "github.com/kris-hansen/comanda/utils/config"

// Helper function to create a test environment config
func createTestEnvConfig() *config.EnvConfig {
	return &config.EnvConfig{
		Providers: map[string]*config.Provider{
			"openai": {
				APIKey: "test-openai-key",
				Models: []config.Model{
					{Name: "gpt-4", Type: "chat"},
					{Name: "gpt-4o", Type: "chat"},
					{Name: "gpt-4o-mini", Type: "chat"},
					{Name: "o1-preview", Type: "chat"},
					{Name: "o1-mini", Type: "chat"},
				},
			},
			"anthropic": {
				APIKey: "test-anthropic-key",
				Models: []config.Model{
					{Name: "claude-3-5-sonnet-latest", Type: "chat"},
					{Name: "claude-3-5-haiku-latest", Type: "chat"},
				},
			},
		},
	}
}
