package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/models"
	"github.com/kris-hansen/comanda/utils/processor"
)

// GenerateRequest represents the request body for the generate endpoint
type GenerateRequest struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model,omitempty"`
}

// GenerateResponse represents the response for the generate endpoint
type GenerateResponse struct {
	Success bool   `json:"success"`
	YAML    string `json:"yaml,omitempty"`
	Error   string `json:"error,omitempty"`
	Model   string `json:"model,omitempty"`
}

// handleGenerate handles the generation of Comanda workflow YAML files using an LLM
func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			Error:   "Method not allowed. Use POST.",
		})
		return
	}

	var req GenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			Error:   fmt.Sprintf("Invalid request body: %v", err),
		})
		return
	}

	// Validate prompt
	if req.Prompt == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			Error:   "Prompt is required",
		})
		return
	}

	// Determine which model to use
	modelForGeneration := req.Model
	if modelForGeneration == "" {
		modelForGeneration = s.envConfig.DefaultGenerationModel
	}
	if modelForGeneration == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			Error:   "No model specified and no default_generation_model configured",
		})
		return
	}

	config.VerboseLog("Generating workflow using model: %s", modelForGeneration)
	config.DebugLog("Generate request: prompt_length=%d, model=%s", len(req.Prompt), modelForGeneration)

	// Prepare the full prompt for the LLM
	// Use the embedded guide with available models from the environment config
	// This ensures the LLM only uses models that are actually configured
	availableModels := s.envConfig.GetAllConfiguredModels()
	dslGuide := processor.GetEmbeddedLLMGuideWithModels(availableModels)

	fullPrompt := fmt.Sprintf(`SYSTEM: You are a YAML generator. You MUST output ONLY valid YAML content. No explanations, no markdown, no code blocks, no commentary - just raw YAML.

--- BEGIN COMANDA DSL SPECIFICATION ---
%s
--- END COMANDA DSL SPECIFICATION ---

User's request: %s

CRITICAL INSTRUCTION: Your entire response must be valid YAML syntax that can be directly saved to a .yaml file. Do not include ANY text before or after the YAML content. Start your response with the first line of YAML and end with the last line of YAML.`,
		dslGuide, req.Prompt)

	// Get the provider
	provider := models.DetectProvider(modelForGeneration)
	if provider == nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			Error:   fmt.Sprintf("Could not detect provider for model: %s", modelForGeneration),
		})
		return
	}

	// Attempt to configure the provider with API key from envConfig
	providerConfig, err := s.envConfig.GetProviderConfig(provider.Name())
	if err != nil {
		// If provider is not in envConfig, it might be a public one like Ollama
		config.VerboseLog("Provider %s not found in env configuration. Assuming it does not require an API key.", provider.Name())
	} else {
		if err := provider.Configure(providerConfig.APIKey); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(GenerateResponse{
				Success: false,
				Error:   fmt.Sprintf("Failed to configure provider %s: %v", provider.Name(), err),
			})
			return
		}
	}
	provider.SetVerbose(config.Verbose)

	// Call the LLM
	config.DebugLog("Sending prompt to LLM: model=%s, prompt_length=%d", modelForGeneration, len(fullPrompt))
	generatedResponse, err := provider.SendPrompt(modelForGeneration, fullPrompt)
	if err != nil {
		config.VerboseLog("LLM execution failed: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(GenerateResponse{
			Success: false,
			Error:   fmt.Sprintf("LLM execution failed: %v", err),
		})
		return
	}

	// Extract YAML content from the response
	yamlContent := generatedResponse

	// Check if the response contains code blocks
	if strings.Contains(generatedResponse, "```yaml") {
		// Extract content between ```yaml and ```
		startMarker := "```yaml"
		endMarker := "```"

		startIdx := strings.Index(generatedResponse, startMarker)
		if startIdx != -1 {
			startIdx += len(startMarker)
			// Find the next ``` after the start marker
			remaining := generatedResponse[startIdx:]
			endIdx := strings.Index(remaining, endMarker)
			if endIdx != -1 {
				yamlContent = strings.TrimSpace(remaining[:endIdx])
			}
		}
	} else if strings.Contains(generatedResponse, "```") {
		// Try generic code block
		parts := strings.Split(generatedResponse, "```")
		if len(parts) >= 3 {
			// Take the content of the first code block
			yamlContent = strings.TrimSpace(parts[1])
			// Remove language identifier if present (e.g., "yaml" at the start)
			lines := strings.Split(yamlContent, "\n")
			if len(lines) > 0 && !strings.Contains(lines[0], ":") {
				yamlContent = strings.Join(lines[1:], "\n")
			}
		}
	}

	config.VerboseLog("Successfully generated workflow YAML")
	config.DebugLog("Generated YAML length: %d bytes", len(yamlContent))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(GenerateResponse{
		Success: true,
		YAML:    yamlContent,
		Model:   modelForGeneration,
	})
}
