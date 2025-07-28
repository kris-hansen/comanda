package models

import (
	"strings"
	"sync"
)

// ModelRegistry is a centralized registry for all supported models across providers
type ModelRegistry struct {
	// Map of provider name to list of supported models
	models map[string][]string
	// Map of provider name to list of model families (prefixes)
	families map[string][]string
	// Mutex for thread safety
	mu sync.RWMutex
}

// Global instance of the model registry
var globalRegistry = NewModelRegistry()

// NewModelRegistry creates a new model registry
func NewModelRegistry() *ModelRegistry {
	registry := &ModelRegistry{
		models:   make(map[string][]string),
		families: make(map[string][]string),
	}

	// Initialize with default models
	registry.initializeDefaultModels()
	return registry
}

// initializeDefaultModels populates the registry with the default models for each provider
func (r *ModelRegistry) initializeDefaultModels() {
	// Anthropic models
	r.RegisterModels("anthropic", []string{
		"claude-3-5-sonnet-20241022",
		"claude-3-5-sonnet-latest",
		"claude-3-5-haiku-latest",
		"claude-3-7-sonnet-20250219",
		"claude-3-7-sonnet-latest",
		"claude-3-5-haiku-20241022",
		"claude-opus-4-20250514",
		"claude-sonnet-4-20250514",
	})
	r.RegisterFamilies("anthropic", []string{
		"claude-3-5-sonnet",
		"claude-3-5-haiku",
		"claude-3-7-sonnet",
		"claude-opus-4",
		"claude-sonnet-4",
	})

	// OpenAI models - primary models only, the full list is fetched from the API
	r.RegisterModels("openai", []string{
		"gpt-4o",
		"gpt-4o-audio-preview",
		"o1",
		"o3-mini",
		"o1-pro",
		"o4-mini",
		"gpt-4.1",
		"o3-pro",
		"o3",
		"chatgpt-4o-latest",
	})

	// X.AI models
	r.RegisterModels("xai", []string{
		"grok-beta",
		"grok-vision-beta",
		"grok-4",
		"grok-4-heavy",
	})

	// Deepseek models
	r.RegisterModels("deepseek", []string{
		"deepseek-chat",
		"deepseek-coder",
		"deepseek-vision",
		"deepseek-reasoner",
	})

	// Google models
	r.RegisterModels("google", []string{
		"gemini-2.5-pro",
		"gemini-2.5-flash",
		"gemini-2.5-flash-lite",
		"gemini-1.5-flash",
		"gemini-1.5-pro",
		"gemini-1.0-pro",
		"aqa",
	})
	r.RegisterFamilies("google", []string{
		"gemini-1.5",
		"gemini-2.5",
	})

	// Moonshot models
	r.RegisterModels("moonshot", []string{
		"moonshot-v1-8k",
		"moonshot-v1-32k",
		"moonshot-v1-128k",
		"moonshot-v1-auto",
	})
	r.RegisterFamilies("moonshot", []string{
		"moonshot-",
	})
}

// RegisterModels adds models to the registry for a specific provider
func (r *ModelRegistry) RegisterModels(provider string, models []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.models[provider] = append(r.models[provider], models...)
}

// RegisterFamilies adds model families (prefixes) to the registry for a specific provider
func (r *ModelRegistry) RegisterFamilies(provider string, families []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.families[provider] = append(r.families[provider], families...)
}

// GetModels returns the list of models for a specific provider
func (r *ModelRegistry) GetModels(provider string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.models[provider]
}

// GetFamilies returns the list of model families for a specific provider
func (r *ModelRegistry) GetFamilies(provider string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.families[provider]
}

// ValidateModel checks if a model is valid for a specific provider
func (r *ModelRegistry) ValidateModel(provider string, modelName string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Trim whitespace and convert to lowercase
	modelName = strings.TrimSpace(strings.ToLower(modelName))

	// Check exact matches
	for _, valid := range r.models[provider] {
		if modelName == valid {
			return true
		}
	}

	// Check model families for flexibility
	for _, family := range r.families[provider] {
		if strings.HasPrefix(modelName, family) {
			return true
		}
	}

	return false
}

// GetAllModels returns a map of all models for all providers
func (r *ModelRegistry) GetAllModels() map[string][]string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Create a deep copy to avoid race conditions
	result := make(map[string][]string)
	for provider, models := range r.models {
		result[provider] = append([]string{}, models...)
	}
	return result
}

// GetAllModelsList returns a flat list of all models from all providers
func (r *ModelRegistry) GetAllModelsList() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var allModels []string
	for _, models := range r.models {
		allModels = append(allModels, models...)
	}
	return allModels
}

// GetRegistry returns the global model registry instance
func GetRegistry() *ModelRegistry {
	return globalRegistry
}
