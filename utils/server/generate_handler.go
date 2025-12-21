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

	// Get available models from the environment config
	availableModels := s.envConfig.GetAllConfiguredModels()

	// Add Claude Code models if the claude binary is available
	if models.IsClaudeCodeAvailable() {
		claudeCodeModels := []string{"claude-code", "claude-code-opus", "claude-code-sonnet", "claude-code-haiku"}
		availableModels = append(availableModels, claudeCodeModels...)
	}

	// Add Gemini CLI models if the gemini binary is available
	if models.IsGeminiCLIAvailable() {
		geminiCLIModels := []string{"gemini-cli", "gemini-cli-pro", "gemini-cli-flash", "gemini-cli-flash-lite"}
		availableModels = append(availableModels, geminiCLIModels...)
	}

	// Add OpenAI Codex models if the codex binary is available
	if models.IsOpenAICodexAvailable() {
		openaiCodexModels := []string{"openai-codex", "openai-codex-o3", "openai-codex-o4-mini", "openai-codex-mini", "openai-codex-gpt-4.1", "openai-codex-gpt-4o"}
		availableModels = append(availableModels, openaiCodexModels...)
	}

	dslGuide := processor.GetEmbeddedLLMGuideWithModels(availableModels)

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

	// Generate workflow with validation and retry
	var yamlContent string
	var invalidModels []string
	maxAttempts := 2

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Build the prompt
		prompt := buildServerGeneratePrompt(dslGuide, req.Prompt, invalidModels)

		// Call the LLM
		config.DebugLog("Sending prompt to LLM (attempt %d): model=%s, prompt_length=%d", attempt, modelForGeneration, len(prompt))
		generatedResponse, err := provider.SendPrompt(modelForGeneration, prompt)
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
		yamlContent = extractServerYAMLContent(generatedResponse)

		// Validate model names in the generated workflow
		invalidModels = processor.ValidateWorkflowModels(yamlContent, availableModels)
		if len(invalidModels) == 0 {
			break
		}

		if attempt < maxAttempts {
			config.VerboseLog("Retrying generation due to invalid model(s): %v", invalidModels)
		} else {
			config.VerboseLog("Warning: Generated workflow contains invalid model(s): %v", invalidModels)
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

// buildServerGeneratePrompt creates the prompt for workflow generation
func buildServerGeneratePrompt(dslGuide, userPrompt string, invalidModels []string) string {
	basePrompt := fmt.Sprintf(`SYSTEM: You are a YAML generator. You MUST output ONLY valid YAML content. No explanations, no markdown, no code blocks, no commentary - just raw YAML.

--- BEGIN COMANDA DSL SPECIFICATION ---
%s
--- END COMANDA DSL SPECIFICATION ---

User's request: %s

CRITICAL INSTRUCTION: Your entire response must be valid YAML syntax that can be directly saved to a .yaml file. Do not include ANY text before or after the YAML content. Start your response with the first line of YAML and end with the last line of YAML.`,
		dslGuide, userPrompt)

	if len(invalidModels) > 0 {
		basePrompt += fmt.Sprintf(`

IMPORTANT CORRECTION: Your previous response used invalid model name(s): %v
You MUST only use models from the "Supported Models" list in the specification above. Please regenerate the workflow using ONLY valid model names.`, invalidModels)
	}

	return basePrompt
}

// extractServerYAMLContent extracts YAML from an LLM response, handling code blocks
func extractServerYAMLContent(response string) string {
	yamlContent := response

	if strings.Contains(response, "```yaml") {
		startMarker := "```yaml"
		endMarker := "```"

		startIdx := strings.Index(response, startMarker)
		if startIdx != -1 {
			startIdx += len(startMarker)
			remaining := response[startIdx:]
			endIdx := strings.Index(remaining, endMarker)
			if endIdx != -1 {
				yamlContent = strings.TrimSpace(remaining[:endIdx])
			}
		}
	} else if strings.Contains(response, "```") {
		parts := strings.Split(response, "```")
		if len(parts) >= 3 {
			yamlContent = strings.TrimSpace(parts[1])
			lines := strings.Split(yamlContent, "\n")
			if len(lines) > 0 && !strings.Contains(lines[0], ":") {
				yamlContent = strings.Join(lines[1:], "\n")
			}
		}
	}

	return yamlContent
}
