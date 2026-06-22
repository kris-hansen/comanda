package models

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/kris-hansen/comanda/utils/fileutil"
	"github.com/kris-hansen/comanda/utils/retry"
	openai "github.com/sashabaranov/go-openai"
)

const sakanaRequestTimeout = 120 * time.Second

// SakanaProvider handles Sakana Fugu models through Sakana's OpenAI-compatible API.
type SakanaProvider struct {
	apiKey  string
	config  ModelConfig
	verbose bool
	mu      sync.Mutex
}

// NewSakanaProvider creates a new Sakana provider instance.
func NewSakanaProvider() *SakanaProvider {
	return &SakanaProvider{
		config: ModelConfig{
			Temperature:         0.7,
			MaxTokens:           2000,
			MaxCompletionTokens: 2000,
			TopP:                1.0,
		},
	}
}

// Name returns the provider name.
func (s *SakanaProvider) Name() string {
	return "sakana"
}

// debugf prints debug information if verbose mode is enabled.
func (s *SakanaProvider) debugf(format string, args ...interface{}) {
	if s.verbose {
		s.mu.Lock()
		defer s.mu.Unlock()
		log.Printf("[DEBUG][Sakana] "+format+"\n", args...)
	}
}

// SupportsModel checks if the given model name is supported by Sakana.
func (s *SakanaProvider) SupportsModel(modelName string) bool {
	s.debugf("Checking if model is supported: %s", modelName)
	modelName = strings.ToLower(strings.TrimSpace(modelName))

	registry := GetRegistry()
	for _, model := range registry.GetModels("sakana") {
		if modelName == strings.ToLower(model) {
			s.debugf("Model %s is supported (exact match)", modelName)
			return true
		}
	}

	for _, prefix := range registry.GetFamilies("sakana") {
		if strings.HasPrefix(modelName, prefix) {
			s.debugf("Model %s is supported (matches prefix %s)", modelName, prefix)
			return true
		}
	}

	s.debugf("Model %s is not supported by Sakana provider", modelName)
	return false
}

// Configure sets up the provider with necessary credentials.
func (s *SakanaProvider) Configure(apiKey string) error {
	s.debugf("Configuring Sakana provider")
	if apiKey == "" {
		return fmt.Errorf("API key is required for Sakana provider")
	}
	s.apiKey = apiKey
	s.debugf("API key configured successfully")
	return nil
}

func (s *SakanaProvider) client() *openai.Client {
	config := openai.DefaultConfig(s.apiKey)
	config.BaseURL = "https://api.sakana.ai/v1"
	return openai.NewClientWithConfig(config)
}

func (s *SakanaProvider) createChatCompletionRequest(modelName string, messages []openai.ChatCompletionMessage) openai.ChatCompletionRequest {
	req := openai.ChatCompletionRequest{
		Model:       modelName,
		Messages:    messages,
		MaxTokens:   s.config.MaxTokens,
		Temperature: float32(s.config.Temperature),
		TopP:        float32(s.config.TopP),
	}

	return req
}

// SendPrompt sends a prompt to the specified model and returns the response.
func (s *SakanaProvider) SendPrompt(modelName string, prompt string) (string, error) {
	s.debugf("Preparing to send prompt to model: %s", modelName)
	s.debugf("Prompt length: %d characters", len(prompt))

	if s.apiKey == "" {
		return "", fmt.Errorf("Sakana provider not configured: missing API key")
	}

	if !s.SupportsModel(modelName) {
		return "", fmt.Errorf("invalid Sakana model: %s", modelName)
	}

	client := s.client()
	result, err := retry.WithRetry(
		func() (interface{}, error) {
			ctx, cancel := context.WithTimeout(context.Background(), sakanaRequestTimeout)
			defer cancel()

			req := s.createChatCompletionRequest(modelName, []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: prompt,
				},
			})

			resp, err := client.CreateChatCompletion(ctx, req)
			if err != nil {
				if ctx.Err() == context.DeadlineExceeded {
					return "", fmt.Errorf("Sakana API request timed out after %v", sakanaRequestTimeout)
				}
				return "", fmt.Errorf("Sakana API error: %v", err)
			}

			if len(resp.Choices) == 0 {
				return "", fmt.Errorf("no response choices returned from Sakana")
			}

			return resp.Choices[0].Message.Content, nil
		},
		retry.Is429Error,
		retry.DefaultRetryConfig,
	)
	if err != nil {
		return "", err
	}

	response := result.(string)
	s.debugf("API call completed, response length: %d characters", len(response))
	return response, nil
}

// SendPromptWithFile sends a prompt along with a file to the specified model and returns the response.
func (s *SakanaProvider) SendPromptWithFile(modelName string, prompt string, file FileInput) (string, error) {
	s.debugf("Preparing to send prompt with file to model: %s", modelName)
	s.debugf("File path: %s", file.Path)

	if s.apiKey == "" {
		return "", fmt.Errorf("Sakana provider not configured: missing API key")
	}

	if !s.SupportsModel(modelName) {
		return "", fmt.Errorf("invalid Sakana model: %s", modelName)
	}

	fileData, err := fileutil.SafeReadFile(file.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	client := s.client()
	if strings.HasPrefix(file.MimeType, "image/") {
		return s.sendVisionPrompt(client, modelName, prompt, file.MimeType, fileData)
	}

	combinedPrompt := fmt.Sprintf("File content:\n%s\n\nUser prompt: %s", string(fileData), prompt)
	return s.SendPrompt(modelName, combinedPrompt)
}

func (s *SakanaProvider) sendVisionPrompt(client *openai.Client, modelName string, prompt string, mimeType string, fileData []byte) (string, error) {
	result, err := retry.WithRetry(
		func() (interface{}, error) {
			ctx, cancel := context.WithTimeout(context.Background(), sakanaRequestTimeout)
			defer cancel()

			content := []openai.ChatMessagePart{
				{
					Type: openai.ChatMessagePartTypeText,
					Text: prompt,
				},
				{
					Type: openai.ChatMessagePartTypeImageURL,
					ImageURL: &openai.ChatMessageImageURL{
						URL: fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(fileData)),
					},
				},
			}

			req := s.createChatCompletionRequest(modelName, []openai.ChatCompletionMessage{
				{
					Role:         openai.ChatMessageRoleUser,
					MultiContent: content,
				},
			})

			resp, err := client.CreateChatCompletion(ctx, req)
			if err != nil {
				if ctx.Err() == context.DeadlineExceeded {
					return "", fmt.Errorf("Sakana Vision API request timed out after %v", sakanaRequestTimeout)
				}
				return "", fmt.Errorf("Sakana Vision API error: %v", err)
			}

			if len(resp.Choices) == 0 {
				return "", fmt.Errorf("no response choices returned from Sakana Vision")
			}

			return resp.Choices[0].Message.Content, nil
		},
		retry.Is429Error,
		retry.DefaultRetryConfig,
	)
	if err != nil {
		return "", err
	}

	return result.(string), nil
}

// SetConfig updates the provider configuration.
func (s *SakanaProvider) SetConfig(config ModelConfig) {
	s.debugf("Updating provider configuration")
	s.config = config
}

// GetConfig returns the current provider configuration.
func (s *SakanaProvider) GetConfig() ModelConfig {
	return s.config
}

// ValidateModel checks if the specific Sakana model variant is valid.
func (s *SakanaProvider) ValidateModel(modelName string) bool {
	isValid := GetRegistry().ValidateModel("sakana", modelName)
	if !isValid {
		isValid = s.SupportsModel(modelName)
	}
	return isValid
}

// SetVerbose enables or disables verbose mode.
func (s *SakanaProvider) SetVerbose(verbose bool) {
	s.verbose = verbose
}
