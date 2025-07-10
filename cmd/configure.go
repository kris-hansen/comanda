package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/database"
	"github.com/kris-hansen/comanda/utils/models"
	openai "github.com/sashabaranov/go-openai"
	"github.com/spf13/cobra"
)

var (
	listFlag                      bool
	encryptFlag                   bool
	decryptFlag                   bool
	removeFlag                    string
	updateKeyFlag                 string
	databaseFlag                  bool
	setDefaultGenerationModelFlag string
	defaultFlag                   bool
)

// Green checkmark for successful operations
const greenCheckmark = "\u2705"

// Primary OpenAI models based on the latest o-series and flagship models
var primaryOpenAIModels = []string{
	"gpt-4o",               // Appears first in user's list
	"gpt-4o-audio-preview", // Appears second
	"o1",                   // Appears third
	"o3-mini",              // Appears fourth
	"o1-pro",               // Appears fifth
	"o4-mini",              // Appears sixth
	"gpt-4.1",              // Appears seventh
	// Models from the image that might not be returned by ListModels API
	"o3-pro",            // Responses API only model
	"o3",                // Base model
	"chatgpt-4o-latest", // ChatGPT variant
}

// Patterns for unsupported model types that should be excluded from selection
var unsupportedModelPatterns = []string{
	"dall-e",      // Image generation
	"tts-",        // Text-to-speech
	"whisper-",    // Speech-to-text
	"embedding",   // Text embeddings
	"moderation",  // Content moderation
	"babbage-002", // Older completion models
	"davinci-002", // Older completion models
}

type OllamaModel struct {
	Name    string `json:"name"`
	ModTime string `json:"modified_at"`
	Size    int64  `json:"size"`
}

// isUnsupportedModel checks if a model should be excluded from selection
func isUnsupportedModel(modelName string) bool {
	modelName = strings.ToLower(modelName)

	// Check against unsupported patterns
	for _, pattern := range unsupportedModelPatterns {
		if strings.Contains(modelName, pattern) {
			return true
		}
	}

	return false
}

// isPrimaryOpenAIModel checks if a model is in the primary models list
func isPrimaryOpenAIModel(modelName string) bool {
	for _, primaryModel := range primaryOpenAIModels {
		if primaryModel == modelName {
			return true
		}
	}
	return false
}

// getOpenAIModelsAndCategorize fetches and categorizes OpenAI models
func getOpenAIModelsAndCategorize(apiKey string) ([]string, []string, error) {
	client := openai.NewClient(apiKey)
	modelsList, err := client.ListModels(context.Background())

	// Create a map of models from API for quick lookup
	apiModels := make(map[string]bool)
	if err == nil {
		for _, model := range modelsList.Models {
			apiModels[model.ID] = true
		}
	} else {
		fmt.Printf("Warning: Could not fetch models from OpenAI API: %v\nFalling back to known models.\n", err)
	}

	var primaryModels []string
	var otherModels []string
	processedModels := make(map[string]bool)

	// Create a temporary OpenAI provider to check model support
	provider := models.NewOpenAIProvider()

	// First, process primary models in the order they're defined
	for _, modelID := range primaryOpenAIModels {
		// Skip unsupported models
		if isUnsupportedModel(modelID) {
			continue
		}

		// Check if the model is supported by our backend
		if !provider.SupportsModel(modelID) {
			continue
		}

		// Add to primary models list
		primaryModels = append(primaryModels, modelID)
		processedModels[modelID] = true
	}

	// Then process any other models from the API
	if err == nil {
		for _, model := range modelsList.Models {
			modelID := model.ID

			// Skip if already processed as primary
			if processedModels[modelID] {
				continue
			}

			// Skip unsupported models
			if isUnsupportedModel(modelID) {
				continue
			}

			// Check if the model is supported by our backend
			if !provider.SupportsModel(modelID) {
				continue
			}

			// Add to other models list
			otherModels = append(otherModels, modelID)
		}

		// Sort other models alphabetically
		sort.Strings(otherModels)
	}

	return primaryModels, otherModels, nil
}

// getOpenAIModels is kept for backward compatibility
func getOpenAIModels(apiKey string) ([]string, error) {
	client := openai.NewClient(apiKey)
	modelsList, err := client.ListModels(context.Background())
	if err != nil {
		return nil, fmt.Errorf("error fetching OpenAI models: %v", err)
	}

	var allModels []string
	for _, model := range modelsList.Models {
		allModels = append(allModels, model.ID)
	}

	return allModels, nil
}

func getAnthropicModels() []string {
	return []string{
		"claude-3-5-sonnet-20241022",
		"claude-3-5-sonnet-latest",
		"claude-3-5-haiku-latest",
		"claude-3-7-sonnet-20250219",
		"claude-3-7-sonnet-latest",
		"claude-3-5-haiku-20241022",
		"claude-opus-4-20250514",
		"claude-sonnet-4-20250514",
	}
}

func getXAIModels() []string {
	return []string{
		"grok-beta",
		"grok-vision-beta",
		"grok-4",
		"grok-4-heavy",
	}
}

func getDeepseekModels() []string {
	return []string{
		"deepseek-chat",
		"deepseek-coder",
		"deepseek-vision",
		"deepseek-reasoner",
	}
}

func getGoogleModels() []string {
	// List based on user input and existing models, matching utils/models/google.go
	return []string{
		// From user input
		"gemini-2.5-pro-exp-03-25",
		"gemini-2.0-flash",
		"gemini-2.0-flash-lite",
		"gemini-1.5-flash",
		"gemini-1.5-flash-8b",
		"gemini-1.5-pro",
		"gemini-2.5-pro-preview-03-25", // Added new model
		"gemini-2.5-pro-preview-05-06", // Added new model
		"gemini-embedding-exp",

		// Existing models not explicitly in user list but kept for compatibility/completeness
		"gemini-1.0-pro",
		"gemini-2.0-flash-exp",                // Experimental version
		"gemini-2.0-flash-001",                // Specific version
		"gemini-2.0-pro-exp-02-05",            // Experimental version
		"gemini-2.0-flash-lite-preview-02-05", // Preview version
		"gemini-2.0-flash-thinking-exp-01-21", // Experimental version
		"aqa",                                 // Attributed Question Answering model
	}
}

func getOllamaModels() ([]OllamaModel, error) {
	resp, err := http.Get("http://localhost:11434/api/tags")
	if err != nil {
		return nil, fmt.Errorf("error connecting to Ollama API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var models struct {
		Models []OllamaModel `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&models); err != nil {
		return nil, fmt.Errorf("error decoding Ollama response: %v", err)
	}

	return models.Models, nil
}

func checkOllamaInstalled() bool {
	cmd := exec.Command("ollama", "list")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func validatePassword(password string) error {
	if len(password) < 6 {
		return fmt.Errorf("password must be at least 6 characters long")
	}
	return nil
}

func configureDatabase(reader *bufio.Reader, envConfig *config.EnvConfig) error {
	fmt.Print("Enter database name: ")
	dbName, _ := reader.ReadString('\n')
	dbName = strings.TrimSpace(dbName)

	// Create new database config
	dbConfig := config.DatabaseConfig{
		Type:     config.PostgreSQL, // Currently only supporting PostgreSQL
		Database: dbName,            // Use the same name for both config and connection
	}

	// Get database connection details
	fmt.Print("Enter database host (default: localhost): ")
	host, _ := reader.ReadString('\n')
	host = strings.TrimSpace(host)
	if host == "" {
		host = "localhost"
	}
	dbConfig.Host = host

	fmt.Print("Enter database port (default: 5432): ")
	portStr, _ := reader.ReadString('\n')
	portStr = strings.TrimSpace(portStr)
	if portStr == "" {
		dbConfig.Port = 5432
	} else {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("invalid port number: %v", err)
		}
		dbConfig.Port = port
	}

	fmt.Print("Enter database user: ")
	user, _ := reader.ReadString('\n')
	dbConfig.User = strings.TrimSpace(user)

	// Use secure password prompt
	password, err := config.PromptPassword("Enter database password: ")
	if err != nil {
		return fmt.Errorf("error reading password: %v", err)
	}
	dbConfig.Password = password

	// Add database configuration
	envConfig.AddDatabase(dbName, dbConfig)

	// Ask if user wants to test the connection
	fmt.Print("Would you like to test the database connection? (y/n): ")
	testConn, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(testConn)) == "y" {
		// Create a database handler and test the connection
		dbHandler := database.NewHandler(envConfig)
		if err := dbHandler.TestConnection(dbName); err != nil {
			return fmt.Errorf("connection test failed: %v", err)
		}
		fmt.Printf("%s Database connection successful!\n", greenCheckmark)
	}

	return nil
}

func removeModel(envConfig *config.EnvConfig, modelName string) error {
	removed := false
	for providerName, provider := range envConfig.Providers {
		for i, model := range provider.Models {
			if model.Name == modelName {
				// Remove the model from the slice
				provider.Models = append(provider.Models[:i], provider.Models[i+1:]...)
				removed = true
				fmt.Printf("Removed model '%s' from provider '%s'\n", modelName, providerName)
				break
			}
		}
		if removed {
			break
		}
	}

	if !removed {
		return fmt.Errorf("model '%s' not found in any provider", modelName)
	}
	return nil
}

func parseModelSelection(input string, maxNum int) ([]int, error) {
	var selected []int
	parts := strings.Split(input, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Check if it's a range (e.g., "1-5")
		if strings.Contains(part, "-") {
			rangeParts := strings.Split(part, "-")
			if len(rangeParts) != 2 {
				return nil, fmt.Errorf("invalid range format: %s", part)
			}

			start, err := strconv.Atoi(strings.TrimSpace(rangeParts[0]))
			if err != nil {
				return nil, fmt.Errorf("invalid range start: %s", rangeParts[0])
			}

			end, err := strconv.Atoi(strings.TrimSpace(rangeParts[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid range end: %s", rangeParts[1])
			}

			if start > end {
				start, end = end, start // Swap if start is greater than end
			}

			if start < 1 || end > maxNum {
				return nil, fmt.Errorf("range %d-%d is out of bounds (1-%d)", start, end, maxNum)
			}

			for i := start; i <= end; i++ {
				selected = append(selected, i)
			}
		} else {
			// Single number
			num, err := strconv.Atoi(part)
			if err != nil {
				return nil, fmt.Errorf("invalid number: %s", part)
			}

			if num < 1 || num > maxNum {
				return nil, fmt.Errorf("number %d is out of bounds (1-%d)", num, maxNum)
			}

			selected = append(selected, num)
		}
	}

	// Remove duplicates while preserving order
	seen := make(map[int]bool)
	var unique []int
	for _, num := range selected {
		if !seen[num] {
			seen[num] = true
			unique = append(unique, num)
		}
	}

	return unique, nil
}

func promptForModelSelection(models []string) ([]string, error) {
	fmt.Println("\nAvailable models:")
	for i, model := range models {
		fmt.Printf("%d. %s\n", i+1, model)
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("\nEnter model numbers (comma-separated, ranges allowed e.g., 1,2,4-6): ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		selected, err := parseModelSelection(input, len(models))
		if err != nil {
			fmt.Printf("Error: %v\nPlease try again.\n", err)
			continue
		}

		if len(selected) == 0 {
			fmt.Println("No valid selections made. Please try again.")
			continue
		}

		// Convert selected numbers to model names
		selectedModels := make([]string, len(selected))
		for i, num := range selected {
			selectedModels[i] = models[num-1]
		}

		return selectedModels, nil
	}
}

// promptForOpenAIModelSelection handles the paginated selection of OpenAI models
func promptForOpenAIModelSelection(primaryModels []string, otherModels []string) ([]string, error) {
	reader := bufio.NewReader(os.Stdin)

	// Display primary models first
	fmt.Println("\nPrimary OpenAI Models:")
	for i, model := range primaryModels {
		fmt.Printf("%d. %s\n", i+1, model)
	}

	// Combined list for selection validation
	allModels := append([]string{}, primaryModels...)
	allModels = append(allModels, otherModels...)

	// Track which page we're on
	showingPrimary := true

	for {
		var prompt string
		if showingPrimary && len(otherModels) > 0 {
			prompt = "\nEnter model numbers, or 'm' to see more models: "
		} else {
			prompt = "\nEnter model numbers, or 'p' to see primary models: "
		}

		fmt.Print(prompt)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		// Handle pagination
		if input == "m" && showingPrimary {
			showingPrimary = false
			fmt.Println("\nOther OpenAI Models:")
			for i, model := range otherModels {
				fmt.Printf("%d. %s\n", i+len(primaryModels)+1, model)
			}
			continue
		} else if input == "p" && !showingPrimary {
			showingPrimary = true
			fmt.Println("\nPrimary OpenAI Models:")
			for i, model := range primaryModels {
				fmt.Printf("%d. %s\n", i+1, model)
			}
			continue
		}

		// Parse selection
		selected, err := parseModelSelection(input, len(allModels))
		if err != nil {
			fmt.Printf("Error: %v\nPlease try again.\n", err)
			continue
		}

		if len(selected) == 0 {
			fmt.Println("No valid selections made. Please try again.")
			continue
		}

		// Convert selected numbers to model names
		selectedModels := make([]string, len(selected))
		for i, num := range selected {
			selectedModels[i] = allModels[num-1]
		}

		return selectedModels, nil
	}
}

func promptForModes(reader *bufio.Reader, modelName string) ([]config.ModelMode, error) {
	fmt.Printf("\nConfiguring modes for %s\n", modelName)
	fmt.Println("Available modes:")
	fmt.Println("1. text - Text processing mode")
	fmt.Println("2. vision - Image and vision processing mode")
	fmt.Println("3. multi - Multi-modal processing")
	fmt.Println("4. file - File processing mode")
	fmt.Print("\nEnter mode numbers (comma-separated, e.g., 1,2): ")
	modesInput, _ := reader.ReadString('\n')
	modesInput = strings.TrimSpace(modesInput)

	var modes []config.ModelMode
	if modesInput != "" {
		modeNumbers := strings.Split(modesInput, ",")
		for _, num := range modeNumbers {
			num = strings.TrimSpace(num)
			switch num {
			case "1":
				modes = append(modes, config.TextMode)
			case "2":
				modes = append(modes, config.VisionMode)
			case "3":
				modes = append(modes, config.MultiMode)
			case "4":
				modes = append(modes, config.FileMode)
			default:
				fmt.Printf("Warning: Invalid mode number '%s' ignored\n", num)
			}
		}
	}

	if len(modes) == 0 {
		// Default to text mode if no modes selected
		modes = append(modes, config.TextMode)
		fmt.Println("No valid modes selected. Defaulting to text mode.")
	}

	return modes, nil
}

// getAllConfiguredModelNames retrieves a list of all unique model names from the configuration.
func getAllConfiguredModelNames(envConfig *config.EnvConfig) []string {
	var modelNames []string
	seen := make(map[string]bool) // Used to ensure uniqueness

	for _, provider := range envConfig.Providers {
		for _, model := range provider.Models {
			if !seen[model.Name] {
				modelNames = append(modelNames, model.Name)
				seen[model.Name] = true
			}
		}
	}
	// Consider sorting modelNames alphabetically here if desired for UX, e.g., sort.Strings(modelNames)
	return modelNames
}

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Configure model settings",
	Long:  `Configure model settings including provider model name and API key`,
	Run: func(cmd *cobra.Command, args []string) {
		if listFlag {
			listConfiguration()
			return
		}

		configPath := config.GetEnvPath()

		if encryptFlag {
			password, err := config.PromptPassword("Enter encryption password (minimum 6 characters): ")
			if err != nil {
				fmt.Printf("Error reading password: %v\n", err)
				return
			}

			if err := validatePassword(password); err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}

			confirmPassword, err := config.PromptPassword("Confirm encryption password: ")
			if err != nil {
				fmt.Printf("Error reading password: %v\n", err)
				return
			}

			if password != confirmPassword {
				fmt.Println("Passwords do not match")
				return
			}

			if err := config.EncryptConfig(configPath, password); err != nil {
				fmt.Printf("Error encrypting configuration: %v\n", err)
				return
			}
			fmt.Println("Configuration encrypted successfully!")
			return
		}

		if decryptFlag {
			// Check if file exists and is encrypted
			data, err := os.ReadFile(configPath)
			if err != nil {
				fmt.Printf("Error reading configuration file: %v\n", err)
				return
			}

			if !config.IsEncrypted(data) {
				fmt.Println("Configuration file is not encrypted")
				return
			}

			password, err := config.PromptPassword("Enter decryption password: ")
			if err != nil {
				fmt.Printf("Error reading password: %v\n", err)
				return
			}

			// Decrypt the configuration
			decrypted, err := config.DecryptConfig(data, password)
			if err != nil {
				fmt.Printf("Error decrypting configuration: %v\n", err)
				return
			}

			// Write the decrypted data back to the file
			if err := os.WriteFile(configPath, decrypted, 0644); err != nil {
				fmt.Printf("Error writing decrypted configuration: %v\n", err)
				return
			}

			fmt.Println("Configuration decrypted successfully!")
			return
		}

		// Check if file exists and is encrypted before loading
		var wasEncrypted bool
		var decryptionPassword string
		if _, err := os.Stat(configPath); err == nil {
			data, err := os.ReadFile(configPath)
			if err == nil && config.IsEncrypted(data) {
				wasEncrypted = true
				password, err := config.PromptPassword("Enter decryption password: ")
				if err != nil {
					fmt.Printf("Error reading password: %v\n", err)
					return
				}
				decryptionPassword = password
			}
		}

		// Load existing configuration
		envConfig, err := config.LoadEnvConfigWithPassword(configPath)
		if err != nil {
			fmt.Printf("Error loading configuration: %v\n", err)
			return
		}

		if updateKeyFlag != "" {
			reader := bufio.NewReader(os.Stdin)
			fmt.Print("Enter new API key: ")
			apiKey, _ := reader.ReadString('\n')
			apiKey = strings.TrimSpace(apiKey)

			if err := envConfig.UpdateAPIKey(updateKeyFlag, apiKey); err != nil {
				fmt.Printf("Error updating API key: %v\n", err)
				return
			}
			fmt.Printf("Successfully updated API key for provider '%s'\n", updateKeyFlag)
		} else if removeFlag != "" {
			if err := removeModel(envConfig, removeFlag); err != nil {
				fmt.Printf("Error: %v\n", err)
				return
			}
		} else if databaseFlag {
			reader := bufio.NewReader(os.Stdin)
			if err := configureDatabase(reader, envConfig); err != nil {
				fmt.Printf("Error configuring database: %v\n", err)
				return
			}
		} else if setDefaultGenerationModelFlag != "" {
			envConfig.DefaultGenerationModel = setDefaultGenerationModelFlag
			fmt.Printf("Default generation model set to '%s'\n", setDefaultGenerationModelFlag)
		} else if defaultFlag {
			// Interactive mode to set default generation model
			reader := bufio.NewReader(os.Stdin)
			allModels := getAllConfiguredModelNames(envConfig)
			if len(allModels) == 0 {
				fmt.Println("No models are currently configured. Please configure models first using 'comanda configure'.")
				return
			}

			fmt.Println("Available configured models:")
			for i, modelName := range allModels {
				fmt.Printf("%d. %s", i+1, modelName)
				if envConfig.DefaultGenerationModel == modelName {
					fmt.Print(" (current default)")
				}
				fmt.Println()
			}

			var selectedDefaultModel string
			for {
				fmt.Print("\nEnter the number of the model to set as default for generation: ")
				selectionInput, _ := reader.ReadString('\n')
				selectionNum, err := strconv.Atoi(strings.TrimSpace(selectionInput))
				if err != nil || selectionNum < 1 || selectionNum > len(allModels) {
					fmt.Println("Invalid selection. Please enter a number from the list.")
					continue
				}
				selectedDefaultModel = allModels[selectionNum-1]
				break
			}

			envConfig.DefaultGenerationModel = selectedDefaultModel
			fmt.Printf("%s Default generation model set to '%s'\n", greenCheckmark, selectedDefaultModel)
		} else {
			reader := bufio.NewReader(os.Stdin)
			// Prompt for provider
			var provider string
			for {
				fmt.Print("Enter provider (openai/anthropic/ollama/google/xai/deepseek): ")
				provider, _ = reader.ReadString('\n')
				provider = strings.TrimSpace(provider)
				if provider == "openai" || provider == "anthropic" || provider == "ollama" || provider == "google" || provider == "xai" || provider == "deepseek" {
					break
				}
				fmt.Println("Invalid provider. Please enter 'openai', 'anthropic', 'ollama', 'google', 'xai', or 'deepseek'")
			}

			// Special handling for ollama provider
			if provider == "ollama" {
				if !checkOllamaInstalled() {
					fmt.Println("Error: Ollama is not installed or not running. Please install ollama and try again.")
					return
				}
			}

			// Check if provider exists
			existingProvider, err := envConfig.GetProviderConfig(provider)
			var apiKey string
			if err != nil {
				if provider != "ollama" {
					// Only prompt for API key if not ollama
					fmt.Print("Enter API key: ")
					apiKey, _ = reader.ReadString('\n')
					apiKey = strings.TrimSpace(apiKey)
				}
				existingProvider = &config.Provider{
					APIKey: apiKey,
					Models: []config.Model{},
				}
				envConfig.AddProvider(provider, *existingProvider)
			} else {
				apiKey = existingProvider.APIKey
			}

			// Get available models based on provider
			var selectedModels []string
			switch provider {
			case "openai":
				if apiKey == "" {
					fmt.Println("Error: API key is required for OpenAI")
					return
				}

				// Fetch and categorize OpenAI models
				primaryModels, otherModels, err := getOpenAIModelsAndCategorize(apiKey)
				if err != nil {
					fmt.Printf("Error fetching OpenAI models: %v\n", err)
					return
				}

				// Use the paginated selection for OpenAI
				selectedModels, err = promptForOpenAIModelSelection(primaryModels, otherModels)
				if err != nil {
					fmt.Printf("Error selecting models: %v\n", err)
					return
				}

			case "anthropic":
				if apiKey == "" {
					fmt.Println("Error: API key is required for Anthropic")
					return
				}
				models := getAnthropicModels()
				selectedModels, err = promptForModelSelection(models)
				if err != nil {
					fmt.Printf("Error selecting models: %v\n", err)
					return
				}

			case "xai":
				if apiKey == "" {
					fmt.Println("Error: API key is required for X.AI")
					return
				}
				models := getXAIModels()
				selectedModels, err = promptForModelSelection(models)
				if err != nil {
					fmt.Printf("Error selecting models: %v\n", err)
					return
				}

			case "deepseek":
				if apiKey == "" {
					fmt.Println("Error: API key is required for Deepseek")
					return
				}
				models := getDeepseekModels()
				selectedModels, err = promptForModelSelection(models)
				if err != nil {
					fmt.Printf("Error selecting models: %v\n", err)
					return
				}

			case "google":
				if apiKey == "" {
					fmt.Println("Error: API key is required for Google")
					return
				}
				models := getGoogleModels()
				selectedModels, err = promptForModelSelection(models)
				if err != nil {
					fmt.Printf("Error selecting models: %v\n", err)
					return
				}

			case "ollama":
				models, err := getOllamaModels()
				if err != nil {
					fmt.Printf("Error fetching Ollama models: %v\n", err)
					return
				}
				modelNames := make([]string, len(models))
				for i, model := range models {
					modelNames[i] = model.Name
				}
				if len(modelNames) == 0 {
					fmt.Println("No models found. Please pull a model first using 'ollama pull <model>'")
					return
				}
				selectedModels, err = promptForModelSelection(modelNames)
				if err != nil {
					fmt.Printf("Error selecting models: %v\n", err)
					return
				}
			}

			// Add new models to provider
			modelType := "external"
			if provider == "ollama" {
				modelType = "local"
			}

			for _, modelName := range selectedModels {
				// Prompt for modes for each model
				modes, err := promptForModes(reader, modelName)
				if err != nil {
					fmt.Printf("Error configuring modes for model %s: %v\n", modelName, err)
					continue
				}

				newModel := config.Model{
					Name:  modelName,
					Type:  modelType,
					Modes: modes,
				}

				if err := envConfig.AddModelToProvider(provider, newModel); err != nil {
					fmt.Printf("Error adding model %s: %v\n", modelName, err)
					continue
				}
			}

			// Prompt to set default generation model if not using a specific flag for it
			if setDefaultGenerationModelFlag == "" {
				fmt.Print("\nDo you want to set or update the default model for workflow generation? (y/n): ")
				setDefaultGenModelInput, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(setDefaultGenModelInput)) == "y" {
					allModels := getAllConfiguredModelNames(envConfig)
					if len(allModels) == 0 {
						fmt.Println("No models are currently configured. Cannot set a default generation model.")
					} else {
						fmt.Println("\nAvailable configured models for default generation:")
						for i, modelName := range allModels {
							fmt.Printf("%d. %s\n", i+1, modelName)
						}
						var selectedDefaultModel string
						for {
							fmt.Print("Enter the number of the model to set as default: ")
							selectionInput, _ := reader.ReadString('\n')
							selectionNum, err := strconv.Atoi(strings.TrimSpace(selectionInput))
							if err != nil || selectionNum < 1 || selectionNum > len(allModels) {
								fmt.Println("Invalid selection. Please enter a number from the list.")
								continue
							}
							selectedDefaultModel = allModels[selectionNum-1]
							break
						}
						envConfig.DefaultGenerationModel = selectedDefaultModel
						fmt.Printf("%s Default generation model set to '%s'\n", greenCheckmark, selectedDefaultModel)
					}
				}
			}
		}

		// Create parent directory if it doesn't exist
		if dir := filepath.Dir(configPath); dir != "." && dir != "/" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				fmt.Printf("Error creating directory: %v\n", err)
				return
			}
		}

		// Save configuration
		if err := config.SaveEnvConfig(configPath, envConfig); err != nil {
			fmt.Printf("Error saving configuration: %v\n", err)
			return
		}

		// Re-encrypt if it was encrypted before
		if wasEncrypted {
			if err := config.EncryptConfig(configPath, decryptionPassword); err != nil {
				fmt.Printf("Error re-encrypting configuration: %v\n", err)
				return
			}
		}

		fmt.Printf("Configuration saved successfully to %s!\n", configPath)
	},
}

func listConfiguration() {
	configPath := config.GetEnvPath()
	envConfig, err := config.LoadEnvConfigWithPassword(configPath)
	if err != nil {
		fmt.Printf("Error loading configuration: %v\n", err)
		return
	}

	fmt.Printf("Configuration from %s:\n\n", configPath)

	// List default generation model
	if envConfig.DefaultGenerationModel != "" {
		fmt.Printf("Default Generation Model: %s\n\n", envConfig.DefaultGenerationModel)
	}

	// List server configuration if it exists
	if server := envConfig.GetServerConfig(); server != nil {
		fmt.Println("Server Configuration:")
		fmt.Printf("Port: %d\n", server.Port)
		fmt.Printf("Data Directory: %s\n", server.DataDir)
		fmt.Printf("Authentication Enabled: %v\n", server.Enabled)
		if server.BearerToken != "" {
			fmt.Printf("Bearer Token: %s\n", server.BearerToken)
		}
		fmt.Println()
	}

	// List databases if they exist
	if len(envConfig.Databases) > 0 {
		fmt.Println("Database Configurations:")
		for name, db := range envConfig.Databases {
			fmt.Printf("\n%s:\n", name)
			fmt.Printf("  Type: %s\n", db.Type)
			fmt.Printf("  Host: %s\n", db.Host)
			fmt.Printf("  Port: %d\n", db.Port)
			fmt.Printf("  User: %s\n", db.User)
			fmt.Printf("  Database: %s\n", db.Database)
		}
		fmt.Println()
	}

	// List providers
	if len(envConfig.Providers) == 0 {
		fmt.Println("No providers configured.")
		return
	}

	fmt.Println("Configured Providers:")
	for name, provider := range envConfig.Providers {
		fmt.Printf("\n%s:\n", name)
		if len(provider.Models) == 0 {
			fmt.Println("  No models configured")
			continue
		}
		for _, model := range provider.Models {
			fmt.Printf("  - %s (%s)\n", model.Name, model.Type)
			if len(model.Modes) > 0 {
				modeStr := make([]string, len(model.Modes))
				for i, mode := range model.Modes {
					modeStr[i] = string(mode)
				}
				fmt.Printf("    Modes: %s\n", strings.Join(modeStr, ", "))
			} else {
				fmt.Printf("    Modes: none\n")
			}
		}
	}
}

func init() {
	configureCmd.Flags().BoolVar(&listFlag, "list", false, "List all configured providers and models")
	configureCmd.Flags().BoolVar(&encryptFlag, "encrypt", false, "Encrypt the configuration file")
	configureCmd.Flags().BoolVar(&decryptFlag, "decrypt", false, "Decrypt the configuration file")
	configureCmd.Flags().StringVar(&removeFlag, "remove", "", "Remove a model by name")
	configureCmd.Flags().StringVar(&updateKeyFlag, "update-key", "", "Update API key for specified provider")
	configureCmd.Flags().BoolVar(&databaseFlag, "database", false, "Configure database settings")
	configureCmd.Flags().StringVar(&setDefaultGenerationModelFlag, "set-default-generation-model", "", "Set the default model for workflow generation")
	configureCmd.Flags().BoolVar(&defaultFlag, "default", false, "Interactively set the default model for workflow generation")
	rootCmd.AddCommand(configureCmd)
}
