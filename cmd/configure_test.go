package cmd

import (
	"sort"
	"testing"

	"github.com/kris-hansen/comanda/utils/models"
)

// TestModelConsistency ensures that the models available for configuration
// match exactly with the models that can be used at runtime
func TestModelConsistency(t *testing.T) {
	t.Run("Google Models Consistency", func(t *testing.T) {
		// Get models from configure
		configModels := getGoogleModels()
		sort.Strings(configModels)

		// Get models from provider
		provider := models.NewGoogleProvider()

		// Get all models that are valid in provider
		providerModels := make([]string, 0)
		for _, model := range configModels {
			if provider.ValidateModel(model) {
				providerModels = append(providerModels, model)
			} else {
				t.Errorf("Model %s is available in configure but not valid in GoogleProvider", model)
			}
		}
		sort.Strings(providerModels)

		// Compare lengths
		if len(configModels) != len(providerModels) {
			t.Errorf("Number of models mismatch: configure has %d models, provider validates %d models",
				len(configModels), len(providerModels))
		}

		// Compare each model
		for i := range configModels {
			if i >= len(providerModels) {
				t.Errorf("Missing model in provider: %s", configModels[i])
				continue
			}
			if configModels[i] != providerModels[i] {
				t.Errorf("Model mismatch at position %d: configure has %s, provider has %s",
					i, configModels[i], providerModels[i])
			}
		}
	})

	t.Run("Latest OpenAI Models Consistency", func(t *testing.T) {
		provider := models.NewOpenAIProvider()
		for _, model := range []string{"gpt-5.6", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna"} {
			if !isPrimaryOpenAIModel(model) {
				t.Errorf("latest OpenAI model %s is missing from configure", model)
			}
			if !provider.ValidateModel(model) {
				t.Errorf("latest OpenAI model %s is not valid at runtime", model)
			}
		}
	})

	t.Run("Latest XAI Models Consistency", func(t *testing.T) {
		provider := models.NewXAIProvider()
		configured := make(map[string]bool)
		for _, model := range getXAIModels() {
			configured[model] = true
		}

		for _, model := range []string{"grok-4.5", "grok-4.3", "grok-latest"} {
			if !configured[model] {
				t.Errorf("latest XAI model %s is missing from configure", model)
			}
			if !provider.ValidateModel(model) {
				t.Errorf("latest XAI model %s is not valid at runtime", model)
			}
		}
	})
}
