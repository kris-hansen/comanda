package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/models"
	"github.com/kris-hansen/comanda/utils/processor"
)

// ValidateRequest represents the request body for the validate endpoint
type ValidateRequest struct {
	Content string `json:"content"`
	// CheckModels also validates model names against the models configured
	// on this server (plus any detected CLI providers)
	CheckModels bool `json:"checkModels,omitempty"`
}

// ValidationIssue represents a single validation error with actionable feedback
type ValidationIssue struct {
	Line    int    `json:"line,omitempty"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
}

// ValidateResponse represents the response for the validate endpoint
type ValidateResponse struct {
	Success       bool              `json:"success"`
	Valid         bool              `json:"valid"`
	Errors        []ValidationIssue `json:"errors,omitempty"`
	InvalidModels []string          `json:"invalidModels,omitempty"`
	Error         string            `json:"error,omitempty"`
}

// collectAvailableModels returns all models configured on this server plus
// models provided by locally detected CLI providers
func collectAvailableModels(envConfig *config.EnvConfig) []string {
	availableModels := envConfig.GetAllConfiguredModels()

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
		availableModels = append(availableModels, models.GetOpenAICodexModels()...)
	}

	// Add Kimi Code models if the kimi binary is available
	if models.IsKimiCodeAvailable() {
		kimiCodeModels := []string{"kimi-code"}
		availableModels = append(availableModels, kimiCodeModels...)
	}

	return availableModels
}

// handleYAMLValidate validates workflow YAML without executing it.
// It runs the same structural validation used by the generate endpoint's
// retry loop and, optionally, checks model names against this server's
// configured models.
func (s *Server) handleYAMLValidate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(ValidateResponse{
			Success: false,
			Error:   "Method not allowed. Use POST.",
		})
		return
	}

	var req ValidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ValidateResponse{
			Success: false,
			Error:   fmt.Sprintf("Invalid request body: %v", err),
		})
		return
	}

	if req.Content == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ValidateResponse{
			Success: false,
			Error:   "content is required",
		})
		return
	}

	result := processor.ValidateWorkflowStructure(req.Content)

	response := ValidateResponse{
		Success: true,
		Valid:   result.Valid,
	}
	for _, e := range result.Errors {
		response.Errors = append(response.Errors, ValidationIssue{
			Line:    e.Line,
			Field:   e.Field,
			Message: e.Message,
			Fix:     e.Fix,
		})
	}

	if req.CheckModels {
		availableModels := collectAvailableModels(s.envConfig)
		invalidModels := processor.ValidateWorkflowModels(req.Content, availableModels)
		if len(invalidModels) > 0 {
			response.Valid = false
			response.InvalidModels = invalidModels
			for _, m := range invalidModels {
				response.Errors = append(response.Errors, ValidationIssue{
					Field:   m,
					Message: fmt.Sprintf("Model '%s' is not configured on this server", m),
					Fix:     "Use a model from this server's configured providers, or configure the model first",
				})
			}
		}
	}

	config.DebugLog("Validated workflow YAML: valid=%v errors=%d", response.Valid, len(response.Errors))
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
