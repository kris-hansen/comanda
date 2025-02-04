package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/models"
)

// handleGetProviders returns the list of configured providers and their models
func (s *Server) handleGetProviders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !checkAuth(s.config, w, r) {
		return
	}

	providers := []ProviderInfo{}

	// Get provider instances
	providerList := []struct {
		name     string
		provider models.Provider
	}{
		{"openai", models.NewOpenAIProvider()},
		{"anthropic", models.NewAnthropicProvider()},
		{"google", models.NewGoogleProvider()},
		{"xai", models.NewXAIProvider()},
		{"ollama", models.NewOllamaProvider()},
	}

	// Get provider configurations
	for _, p := range providerList {
		if p.provider != nil {
			if provider, err := s.envConfig.GetProviderConfig(p.name); err == nil {
				providers = append(providers, ProviderInfo{
					Name:    p.name,
					Models:  getModelNames(provider.Models),
					Enabled: provider.APIKey != "",
				})
			}
		}
	}

	json.NewEncoder(w).Encode(ProviderListResponse{
		Success:   true,
		Providers: providers,
	})
}

// handleValidateProvider validates a provider's API key
func (s *Server) handleValidateProvider(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !checkAuth(s.config, w, r) {
		return
	}

	var req ProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.VerboseLog("Error decoding request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Invalid request format",
		})
		return
	}

	// Create provider instance
	var provider models.Provider
	switch req.Name {
	case "openai":
		provider = models.NewOpenAIProvider()
	case "anthropic":
		provider = models.NewAnthropicProvider()
	case "google":
		provider = models.NewGoogleProvider()
	case "xai":
		provider = models.NewXAIProvider()
	case "ollama":
		provider = models.NewOllamaProvider()
	default:
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("Unknown provider: %s", req.Name),
		})
		return
	}

	// Validate API key
	if err := provider.Configure(req.APIKey); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("Invalid API key: %v", err),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"message": "API key is valid",
	})
}

// getModelNames extracts model names from config.Model slice
func getModelNames(models []config.Model) []string {
	names := make([]string, len(models))
	for i, model := range models {
		names[i] = model.Name
	}
	return names
}

// handleUpdateProvider handles adding/updating a provider configuration
func (s *Server) handleUpdateProvider(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !checkAuth(s.config, w, r) {
		return
	}

	var req ProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		config.VerboseLog("Error decoding request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Invalid request format",
		})
		return
	}

	// Get existing provider or create new one
	provider, err := s.envConfig.GetProviderConfig(req.Name)
	if err != nil {
		// Provider doesn't exist, create new one
		provider = &config.Provider{
			Models: make([]config.Model, 0),
		}
		s.envConfig.AddProvider(req.Name, *provider)
	}

	// Update API key if provided
	if req.APIKey != "" {
		if err := s.envConfig.UpdateAPIKey(req.Name, req.APIKey); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("Error updating API key: %v", err),
			})
			return
		}

		// Configure the provider with the new API key
		var provider models.Provider
		switch req.Name {
		case "openai":
			provider = models.NewOpenAIProvider()
		case "anthropic":
			provider = models.NewAnthropicProvider()
		case "google":
			provider = models.NewGoogleProvider()
		case "xai":
			provider = models.NewXAIProvider()
		case "ollama":
			provider = models.NewOllamaProvider()
		default:
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("Unknown provider: %s", req.Name),
			})
			return
		}

		if provider != nil {
			if err := provider.Configure(req.APIKey); err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{
					"error": fmt.Sprintf("Error configuring provider: %v", err),
				})
				return
			}
		}
	}

	// Update models if provided
	if len(req.Models) > 0 {
		for _, modelName := range req.Models {
			model := config.Model{
				Name:  modelName,
				Modes: []config.ModelMode{config.TextMode}, // Default to text mode
			}
			if err := s.envConfig.AddModelToProvider(req.Name, model); err != nil {
				config.VerboseLog("Error adding model %s: %v", modelName, err)
				// Continue with other models
			}
		}
	}

	// Save the updated configuration
	if err := config.SaveEnvConfig(config.GetEnvPath(), s.envConfig); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("Error saving configuration: %v", err),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"message": fmt.Sprintf("Provider %s updated successfully", req.Name),
	})
}

// handleDeleteProvider handles removing a provider configuration
func (s *Server) handleDeleteProvider(w http.ResponseWriter, r *http.Request, providerName string) {
	w.Header().Set("Content-Type", "application/json")

	if !checkAuth(s.config, w, r) {
		return
	}

	// Remove provider from configuration
	if s.envConfig.Providers != nil {
		delete(s.envConfig.Providers, providerName)
	}

	// Save the updated configuration
	if err := config.SaveEnvConfig(config.GetEnvPath(), s.envConfig); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("Error saving configuration: %v", err),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"message": fmt.Sprintf("Provider %s removed successfully", providerName),
	})
}
