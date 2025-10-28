package models

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/google/generative-ai-go/genai"
	"github.com/kris-hansen/comanda/utils/fileutil"
	"github.com/kris-hansen/comanda/utils/retry"
	"google.golang.org/api/option"
)

// GoogleProvider handles Google AI (Gemini) family of models
type GoogleProvider struct {
	apiKey  string
	config  ModelConfig
	verbose bool
	mu      sync.Mutex
}

// NewGoogleProvider creates a new Google provider instance
func NewGoogleProvider() *GoogleProvider {
	return &GoogleProvider{
		config: ModelConfig{
			Temperature: 0.7,
			MaxTokens:   2000,
			TopP:        1.0,
		},
	}
}

// Name returns the provider name
func (g *GoogleProvider) Name() string {
	return "google"
}

// debugf prints debug information if verbose mode is enabled (thread-safe)
func (g *GoogleProvider) debugf(format string, args ...interface{}) {
	if g.verbose {
		g.mu.Lock()
		defer g.mu.Unlock()
		log.Printf("[DEBUG][Google] "+format+"\n", args...)
	}
}

// ValidateModel checks if the specific Google model variant is valid
func (g *GoogleProvider) ValidateModel(modelName string) bool {
	g.debugf("Validating model: %s", modelName)

	// Use the central model registry for validation
	isValid := GetRegistry().ValidateModel("google", modelName)

	if isValid {
		g.debugf("Model %s validation succeeded", modelName)
	} else {
		g.debugf("Model %s validation failed - no matches found", modelName)
	}

	return isValid
}

// SupportsModel checks if the given model name is supported by Google
func (g *GoogleProvider) SupportsModel(modelName string) bool {
	return g.ValidateModel(modelName)
}

// Configure sets up the provider with necessary credentials
func (g *GoogleProvider) Configure(apiKey string) error {
	g.debugf("Configuring Google provider")
	if apiKey == "" {
		return fmt.Errorf("API key is required for Google provider")
	}
	g.apiKey = apiKey
	g.debugf("API key configured successfully")
	return nil
}

// SendPrompt sends a prompt to the specified model and returns the response
func (g *GoogleProvider) SendPrompt(modelName string, prompt string) (string, error) {
	g.debugf("Preparing to send prompt to model: %s", modelName)
	g.debugf("Prompt length: %d characters", len(prompt))

	if g.apiKey == "" {
		return "", fmt.Errorf("Google provider not configured: missing API key")
	}

	if !g.ValidateModel(modelName) {
		return "", fmt.Errorf("invalid Google model: %s", modelName)
	}

	g.debugf("Model validation passed, preparing API call")
	g.debugf("Using configuration: Temperature=%.2f, MaxTokens=%d, TopP=%.2f",
		g.config.Temperature, g.config.MaxTokens, g.config.TopP)

	// Use retry mechanism for API calls
	result, err := retry.WithRetry(
		func() (interface{}, error) {
			ctx := context.Background()
			client, err := genai.NewClient(ctx, option.WithAPIKey(g.apiKey))
			if err != nil {
				return "", fmt.Errorf("failed to create Google AI client: %v", err)
			}
			defer client.Close()

			// Initialize the model
			model := client.GenerativeModel(modelName)
			model.SetTemperature(float32(g.config.Temperature))
			model.SetTopP(float32(g.config.TopP))
			model.SetMaxOutputTokens(int32(g.config.MaxTokens))

			// Generate content
			resp, err := model.GenerateContent(ctx, genai.Text(prompt))
			if err != nil {
				return "", fmt.Errorf("Google AI API error: %v", err)
			}

			if len(resp.Candidates) == 0 {
				return "", fmt.Errorf("no response candidates returned from Google AI")
			}

			// Extract the response text from the first candidate
			var response string
			for _, part := range resp.Candidates[0].Content.Parts {
				if text, ok := part.(genai.Text); ok {
					response += string(text)
				}
			}

			return response, nil
		},
		retry.Is429Error,
		retry.DefaultRetryConfig,
	)

	if err != nil {
		return "", err
	}

	response := result.(string)
	g.debugf("API call completed, response length: %d characters", len(response))

	return response, nil
}

// SendPromptWithFile sends a prompt along with a file to the specified model and returns the response
func (g *GoogleProvider) SendPromptWithFile(modelName string, prompt string, file FileInput) (string, error) {
	g.debugf("Preparing to send prompt with file to model: %s", modelName)
	g.debugf("File path: %s", file.Path)

	if g.apiKey == "" {
		return "", fmt.Errorf("Google provider not configured: missing API key")
	}

	if !g.ValidateModel(modelName) {
		return "", fmt.Errorf("invalid Google model: %s", modelName)
	}

	// Read the file content with size check - do this outside the retry loop
	fileData, err := fileutil.SafeReadFile(file.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	// Use retry mechanism for API calls
	result, err := retry.WithRetry(
		func() (interface{}, error) {
			ctx := context.Background()
			client, err := genai.NewClient(ctx, option.WithAPIKey(g.apiKey))
			if err != nil {
				return "", fmt.Errorf("failed to create Google AI client: %v", err)
			}
			defer client.Close()

			// Initialize the model
			model := client.GenerativeModel(modelName)
			model.SetTemperature(float32(g.config.Temperature))
			model.SetTopP(float32(g.config.TopP))
			model.SetMaxOutputTokens(int32(g.config.MaxTokens))

			// Generate content with file
			resp, err := model.GenerateContent(ctx,
				genai.Text(prompt),
				genai.Blob{
					MIMEType: file.MimeType,
					Data:     fileData,
				})
			if err != nil {
				// Check if it's an encoding error
				if strings.Contains(err.Error(), "invalid UTF-8") {
					return "", fmt.Errorf("encoding error in file %s: invalid UTF-8 characters detected", file.Path)
				}
				return "", fmt.Errorf("Google AI API error: %v", err)
			}

			if len(resp.Candidates) == 0 {
				return "", fmt.Errorf("no response candidates returned from Google AI")
			}

			// Extract the response text from the first candidate
			var response string
			for _, part := range resp.Candidates[0].Content.Parts {
				if text, ok := part.(genai.Text); ok {
					response += string(text)
				}
			}

			return response, nil
		},
		retry.Is429Error,
		retry.DefaultRetryConfig,
	)

	if err != nil {
		return "", err
	}

	response := result.(string)
	g.debugf("API call completed, response length: %d characters", len(response))

	return response, nil
}

// SetVerbose enables or disables verbose mode
func (g *GoogleProvider) SetVerbose(verbose bool) {
	g.verbose = verbose
}
