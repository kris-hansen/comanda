package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings" // Added for path splitting

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/discovery" // Added discovery package
	"github.com/kris-hansen/comanda/utils/models"
)

// sendJSONError sends a JSON error response with the given status code and message
func sendJSONError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{
		Success: false,
		Error:   message,
	})
}

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

// handleGetConfiguredModels returns the list of models configured for a specific provider
func (s *Server) handleGetConfiguredModels(w http.ResponseWriter, r *http.Request, providerName string) {
	w.Header().Set("Content-Type", "application/json")

	if !checkAuth(s.config, w, r) {
		return
	}

	providerConfig, err := s.envConfig.GetProviderConfig(providerName)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, fmt.Sprintf("Provider '%s' not found", providerName))
		return
	}

	configuredModels := make([]ConfiguredModel, len(providerConfig.Models))
	for i, m := range providerConfig.Models {
		configuredModels[i] = ConfiguredModel{
			Name:  m.Name,
			Type:  m.Type,
			Modes: m.Modes,
		}
	}

	json.NewEncoder(w).Encode(ConfiguredModelListResponse{
		Success: true,
		Models:  configuredModels,
	})
}

// handleGetAvailableModels returns the list of models available from the provider's service
func (s *Server) handleGetAvailableModels(w http.ResponseWriter, r *http.Request, providerName string) {
	w.Header().Set("Content-Type", "application/json")

	if !checkAuth(s.config, w, r) {
		return
	}

	// Get API key if needed (e.g., for OpenAI)
	providerConfig, err := s.envConfig.GetProviderConfig(providerName)
	apiKey := ""
	if err == nil { // Provider exists, get its key
		apiKey = providerConfig.APIKey
	} else if providerName != "ollama" && providerName != "anthropic" && providerName != "google" && providerName != "xai" && providerName != "deepseek" {
		// If provider doesn't exist and requires a key, we can't proceed
		sendJSONError(w, http.StatusBadRequest, fmt.Sprintf("Provider '%s' not configured or requires an API key to list models", providerName))
		return
	}

	// Fetch available models using the discovery package
	modelNames, err := discovery.GetAvailableModels(providerName, apiKey)
	if err != nil {
		sendJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Error fetching available models for %s: %v", providerName, err))
		return
	}

	availableModels := make([]AvailableModel, len(modelNames))
	for i, name := range modelNames {
		availableModels[i] = AvailableModel{Name: name} // Description is omitted for now
	}

	json.NewEncoder(w).Encode(AvailableModelListResponse{
		Success: true,
		Models:  availableModels,
	})
}

// handleAddModel adds a new model to a provider's configuration
func (s *Server) handleAddModel(w http.ResponseWriter, r *http.Request, providerName string) {
	w.Header().Set("Content-Type", "application/json")

	if !checkAuth(s.config, w, r) {
		return
	}

	var req AddModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" {
		sendJSONError(w, http.StatusBadRequest, "Model name is required")
		return
	}
	if len(req.Modes) == 0 {
		sendJSONError(w, http.StatusBadRequest, "At least one model mode is required")
		return
	}

	// Validate modes
	for _, mode := range req.Modes {
		if !config.ValidateModelMode(mode) {
			sendJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid model mode: %s", mode))
			return
		}
	}

	// Determine model type
	modelType := "external"
	if providerName == "ollama" {
		modelType = "local"
	}

	newModel := config.Model{
		Name:  req.Name,
		Type:  modelType,
		Modes: req.Modes,
	}

	// Add model to config
	if err := s.envConfig.AddModelToProvider(providerName, newModel); err != nil {
		// Check if it's a "provider not found" error
		if strings.Contains(err.Error(), "not found") {
			sendJSONError(w, http.StatusNotFound, err.Error())
		} else {
			sendJSONError(w, http.StatusBadRequest, err.Error()) // e.g., model already exists
		}
		return
	}

	// Save the updated configuration
	if err := config.SaveEnvConfig(config.GetEnvPath(), s.envConfig); err != nil {
		sendJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Error saving configuration: %v", err))
		return
	}

	json.NewEncoder(w).Encode(SuccessResponse{
		Success: true,
		Message: fmt.Sprintf("Model '%s' added to provider '%s'", req.Name, providerName),
	})
}

// handleUpdateModel updates the modes for a configured model
func (s *Server) handleUpdateModel(w http.ResponseWriter, r *http.Request, providerName string, modelName string) {
	w.Header().Set("Content-Type", "application/json")

	if !checkAuth(s.config, w, r) {
		return
	}

	var req UpdateModelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if len(req.Modes) == 0 {
		sendJSONError(w, http.StatusBadRequest, "At least one model mode is required")
		return
	}

	// Validate modes before attempting update
	for _, mode := range req.Modes {
		if !config.ValidateModelMode(mode) {
			sendJSONError(w, http.StatusBadRequest, fmt.Sprintf("Invalid model mode: %s", mode))
			return
		}
	}

	// Update model modes in config
	if err := s.envConfig.UpdateModelModes(providerName, modelName, req.Modes); err != nil {
		// Check if it's a "not found" error
		if strings.Contains(err.Error(), "not found") {
			sendJSONError(w, http.StatusNotFound, err.Error())
		} else {
			sendJSONError(w, http.StatusInternalServerError, err.Error()) // Should not happen if validation passed
		}
		return
	}

	// Save the updated configuration
	if err := config.SaveEnvConfig(config.GetEnvPath(), s.envConfig); err != nil {
		sendJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Error saving configuration: %v", err))
		return
	}

	json.NewEncoder(w).Encode(SuccessResponse{
		Success: true,
		Message: fmt.Sprintf("Modes for model '%s' in provider '%s' updated", modelName, providerName),
	})
}

// handleDeleteModel removes a model from a provider's configuration
func (s *Server) handleDeleteModel(w http.ResponseWriter, r *http.Request, providerName string, modelName string) {
	w.Header().Set("Content-Type", "application/json")

	if !checkAuth(s.config, w, r) {
		return
	}

	providerConfig, err := s.envConfig.GetProviderConfig(providerName)
	if err != nil {
		sendJSONError(w, http.StatusNotFound, fmt.Sprintf("Provider '%s' not found", providerName))
		return
	}

	found := false
	newModels := []config.Model{}
	for _, model := range providerConfig.Models {
		if model.Name == modelName {
			found = true
		} else {
			newModels = append(newModels, model)
		}
	}

	if !found {
		sendJSONError(w, http.StatusNotFound, fmt.Sprintf("Model '%s' not found for provider '%s'", modelName, providerName))
		return
	}

	// Update the provider's model list
	providerConfig.Models = newModels

	// Save the updated configuration
	if err := config.SaveEnvConfig(config.GetEnvPath(), s.envConfig); err != nil {
		sendJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Error saving configuration: %v", err))
		return
	}

	json.NewEncoder(w).Encode(SuccessResponse{
		Success: true,
		Message: fmt.Sprintf("Model '%s' removed from provider '%s'", modelName, providerName),
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

	// Validate provider name
	if providerName == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "Provider name is required",
		})
		return
	}

	// Remove provider from configuration and save
	if s.envConfig.Providers != nil {
		delete(s.envConfig.Providers, providerName)

		// Save the updated configuration
		if err := config.SaveEnvConfig(config.GetEnvPath(), s.envConfig); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("Error saving configuration: %v", err),
			})
			return
		}
	}

	// Always return success, even if provider didn't exist
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(SuccessResponse{
		Success: true,
		Message: fmt.Sprintf("Provider %s removed successfully", providerName),
	})
}
