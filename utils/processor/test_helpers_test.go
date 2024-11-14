package processor

import "github.com/kris-hansen/comanda/utils/config"

// Helper function to create a test environment config
func createTestEnvConfig() *config.EnvConfig {
	return &config.EnvConfig{
		Providers: map[string]*config.Provider{
			"openai": {
				APIKey: "test-openai-key",
				Models: []config.Model{
					{Name: "gpt-4", Type: "text", Modes: []config.ModelMode{config.TextMode, config.VisionMode, config.FileMode, config.MultiMode}},
					{Name: "gpt-4o", Type: "text", Modes: []config.ModelMode{config.TextMode, config.VisionMode, config.FileMode, config.MultiMode}},
					{Name: "gpt-4o-mini", Type: "text", Modes: []config.ModelMode{config.TextMode, config.VisionMode, config.FileMode, config.MultiMode}},
					{Name: "o1-preview", Type: "text", Modes: []config.ModelMode{config.TextMode, config.VisionMode, config.FileMode, config.MultiMode}},
					{Name: "o1-mini", Type: "text", Modes: []config.ModelMode{config.TextMode, config.VisionMode, config.FileMode, config.MultiMode}},
				},
			},
			"anthropic": {
				APIKey: "test-anthropic-key",
				Models: []config.Model{
					{Name: "claude-3-5-sonnet-latest", Type: "text", Modes: []config.ModelMode{config.TextMode, config.VisionMode, config.FileMode, config.MultiMode}},
					{Name: "claude-3-5-haiku-latest", Type: "text", Modes: []config.ModelMode{config.TextMode, config.VisionMode, config.FileMode, config.MultiMode}},
				},
			},
		},
	}
}
