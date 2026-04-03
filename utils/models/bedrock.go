package models

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"
	appconfig "github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/fileutil"
)

// BedrockProvider handles AWS Bedrock models via the Converse API
type BedrockProvider struct {
	client  *bedrockruntime.Client
	region  string
	config  ModelConfig
	verbose bool
	mu      sync.Mutex
}

// NewBedrockProvider creates a new Bedrock provider instance
func NewBedrockProvider() *BedrockProvider {
	return &BedrockProvider{
		config: ModelConfig{
			Temperature: 0.7,
			MaxTokens:   2000,
			TopP:        1.0,
		},
	}
}

// debugf prints debug information if verbose mode is enabled (thread-safe)
func (b *BedrockProvider) debugf(format string, args ...interface{}) {
	if b.verbose {
		b.mu.Lock()
		defer b.mu.Unlock()
		log.Printf("[DEBUG][Bedrock] "+format+"\n", args...)
	}
}

// Name returns the provider name
func (b *BedrockProvider) Name() string {
	return "bedrock"
}

// SupportsModel checks if the given model name is supported by Bedrock
func (b *BedrockProvider) SupportsModel(modelName string) bool {
	b.debugf("Checking if model is supported: %s", modelName)
	modelName = strings.ToLower(modelName)
	// Support bedrock/ prefix for explicit routing
	isSupported := strings.HasPrefix(modelName, "bedrock/")
	b.debugf("Model %s support result: %v", modelName, isSupported)
	return isSupported
}

// Configure sets up the provider with AWS credentials
// For Bedrock, we use the standard AWS credential chain, so apiKey is ignored
// but we still need to initialize the client
func (b *BedrockProvider) Configure(apiKey string) error {
	b.debugf("Configuring Bedrock provider")

	// Determine region from environment or default
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		region = "us-east-1" // Default region for Bedrock
	}
	b.region = region
	b.debugf("Using AWS region: %s", region)

	// Load AWS config using default credential chain
	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
	)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %v", err)
	}

	b.client = bedrockruntime.NewFromConfig(cfg)
	b.debugf("Bedrock client configured successfully")
	return nil
}

// extractModelID extracts the actual model ID from the bedrock/ prefixed name
func (b *BedrockProvider) extractModelID(modelName string) string {
	// Remove bedrock/ prefix
	if strings.HasPrefix(strings.ToLower(modelName), "bedrock/") {
		return modelName[8:] // len("bedrock/") = 8
	}
	return modelName
}

// SendPrompt sends a prompt to the specified model and returns the response
func (b *BedrockProvider) SendPrompt(modelName string, prompt string) (string, error) {
	b.debugf("Preparing to send prompt to model: %s", modelName)
	b.debugf("Prompt length: %d characters", len(prompt))

	if b.client == nil {
		// Auto-configure if not already done
		if err := b.Configure(""); err != nil {
			return "", fmt.Errorf("Bedrock provider not configured: %v", err)
		}
	}

	modelID := b.extractModelID(modelName)
	b.debugf("Using model ID: %s", modelID)

	// Build the message for Converse API
	messages := []types.Message{
		{
			Role: types.ConversationRoleUser,
			Content: []types.ContentBlock{
				&types.ContentBlockMemberText{
					Value: prompt,
				},
			},
		},
	}

	// Build inference configuration
	inferenceConfig := &types.InferenceConfiguration{
		MaxTokens:   aws.Int32(int32(b.config.MaxTokens)),
		Temperature: aws.Float32(float32(b.config.Temperature)),
		TopP:        aws.Float32(float32(b.config.TopP)),
	}

	// Call Converse API
	input := &bedrockruntime.ConverseInput{
		ModelId:         aws.String(modelID),
		Messages:        messages,
		InferenceConfig: inferenceConfig,
	}

	b.debugf("Calling Bedrock Converse API")
	output, err := b.client.Converse(context.Background(), input)
	if err != nil {
		return "", fmt.Errorf("Bedrock API call failed: %v", err)
	}

	// Extract response text
	responseText := b.extractResponseText(output)
	b.debugf("API call completed, response length: %d characters", len(responseText))

	return responseText, nil
}

// SendPromptWithFile sends a prompt along with a file to the specified model
func (b *BedrockProvider) SendPromptWithFile(modelName string, prompt string, file FileInput) (string, error) {
	b.debugf("Preparing to send prompt with file to model: %s", modelName)
	b.debugf("File path: %s, MIME type: %s", file.Path, file.MimeType)

	if b.client == nil {
		if err := b.Configure(""); err != nil {
			return "", fmt.Errorf("Bedrock provider not configured: %v", err)
		}
	}

	modelID := b.extractModelID(modelName)

	// Read the file content
	fileData, err := fileutil.SafeReadFile(file.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %v", err)
	}

	var contentBlocks []types.ContentBlock

	// Handle different file types
	switch {
	case strings.HasPrefix(file.MimeType, "image/"):
		// Convert MIME type to Bedrock image format
		imageFormat := b.mimeToImageFormat(file.MimeType)
		if imageFormat == "" {
			return "", fmt.Errorf("unsupported image format: %s", file.MimeType)
		}

		contentBlocks = []types.ContentBlock{
			&types.ContentBlockMemberImage{
				Value: types.ImageBlock{
					Format: imageFormat,
					Source: &types.ImageSourceMemberBytes{
						Value: fileData,
					},
				},
			},
			&types.ContentBlockMemberText{
				Value: prompt,
			},
		}

	case file.MimeType == "application/pdf":
		// PDF support via document block
		contentBlocks = []types.ContentBlock{
			&types.ContentBlockMemberDocument{
				Value: types.DocumentBlock{
					Format: types.DocumentFormatPdf,
					Name:   aws.String(file.Path),
					Source: &types.DocumentSourceMemberBytes{
						Value: fileData,
					},
				},
			},
			&types.ContentBlockMemberText{
				Value: prompt,
			},
		}

	default:
		// For text files, include content in the prompt
		fileContent := string(fileData)
		contentBlocks = []types.ContentBlock{
			&types.ContentBlockMemberText{
				Value: fmt.Sprintf("File content:\n%s\n\nUser prompt: %s", fileContent, prompt),
			},
		}
	}

	messages := []types.Message{
		{
			Role:    types.ConversationRoleUser,
			Content: contentBlocks,
		},
	}

	inferenceConfig := &types.InferenceConfiguration{
		MaxTokens:   aws.Int32(int32(b.config.MaxTokens)),
		Temperature: aws.Float32(float32(b.config.Temperature)),
		TopP:        aws.Float32(float32(b.config.TopP)),
	}

	input := &bedrockruntime.ConverseInput{
		ModelId:         aws.String(modelID),
		Messages:        messages,
		InferenceConfig: inferenceConfig,
	}

	output, err := b.client.Converse(context.Background(), input)
	if err != nil {
		return "", fmt.Errorf("Bedrock API call failed: %v", err)
	}

	responseText := b.extractResponseText(output)
	b.debugf("API call completed, response length: %d characters", len(responseText))

	return responseText, nil
}

// extractResponseText extracts the text content from a Converse API response
func (b *BedrockProvider) extractResponseText(output *bedrockruntime.ConverseOutput) string {
	if output == nil || output.Output == nil {
		return ""
	}

	// Extract from message output
	msgOutput, ok := output.Output.(*types.ConverseOutputMemberMessage)
	if !ok {
		return ""
	}

	var result strings.Builder
	for _, block := range msgOutput.Value.Content {
		if textBlock, ok := block.(*types.ContentBlockMemberText); ok {
			result.WriteString(textBlock.Value)
		}
	}

	return result.String()
}

// mimeToImageFormat converts a MIME type to Bedrock's ImageFormat enum
func (b *BedrockProvider) mimeToImageFormat(mimeType string) types.ImageFormat {
	switch mimeType {
	case "image/jpeg", "image/jpg":
		return types.ImageFormatJpeg
	case "image/png":
		return types.ImageFormatPng
	case "image/gif":
		return types.ImageFormatGif
	case "image/webp":
		return types.ImageFormatWebp
	default:
		return ""
	}
}

// ValidateModel checks if the specific Bedrock model is valid
func (b *BedrockProvider) ValidateModel(modelName string) bool {
	b.debugf("Validating model: %s", modelName)

	// Use the central model registry for validation
	isValid := GetRegistry().ValidateModel("bedrock", modelName)

	if isValid {
		b.debugf("Model %s validation succeeded", modelName)
	} else {
		// For Bedrock, we're more permissive since new models are added frequently
		// Accept any bedrock/ prefixed model
		if strings.HasPrefix(strings.ToLower(modelName), "bedrock/") {
			b.debugf("Model %s accepted (bedrock/ prefix)", modelName)
			return true
		}
		b.debugf("Model %s validation failed", modelName)
	}

	return isValid
}

// SetConfig updates the provider configuration
func (b *BedrockProvider) SetConfig(config ModelConfig) {
	b.debugf("Updating provider configuration")
	b.config = config
}

// GetConfig returns the current provider configuration
func (b *BedrockProvider) GetConfig() ModelConfig {
	return b.config
}

// SetVerbose enables or disables verbose mode
func (b *BedrockProvider) SetVerbose(verbose bool) {
	b.verbose = verbose
}

// IsBedrockAvailable checks if AWS credentials are configured
func IsBedrockAvailable() bool {
	// Check if AWS credentials are available via environment or credential file
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		appconfig.DebugLog("[Bedrock] AWS config not available: %v", err)
		return false
	}

	// Try to retrieve credentials to verify they're configured
	creds, err := cfg.Credentials.Retrieve(context.Background())
	if err != nil {
		appconfig.DebugLog("[Bedrock] AWS credentials not available: %v", err)
		return false
	}

	if creds.AccessKeyID == "" {
		appconfig.DebugLog("[Bedrock] AWS credentials empty")
		return false
	}

	appconfig.DebugLog("[Bedrock] AWS credentials available")
	return true
}

// Helper function for base64 encoding (used for potential future features)
func encodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}
