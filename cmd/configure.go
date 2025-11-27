package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
	memoryFlag                    string
	initMemoryFlag                bool
)

// Green checkmark for successful operations
const greenCheckmark = "\u2705"

// Primary OpenAI models based on the latest o-series and flagship models
var primaryOpenAIModels = []string{
	// GPT-5.1 series (latest)
	"gpt-5.1",
	"gpt-5.1-mini",
	"gpt-5.1-nano",
	// GPT-5 series
	"gpt-5",
	"gpt-5-mini",
	"gpt-5-nano",
	// GPT-4.1 and 4o series
	"gpt-4.1",
	"gpt-4o",
	"gpt-4o-audio-preview",
	"chatgpt-4o-latest",
	// o-series reasoning models
	"o3-pro",
	"o3",
	"o3-mini",
	"o1-pro",
	"o1",
	"o4-mini",
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
		log.Printf("Warning: Could not fetch models from OpenAI API: %v\nFalling back to known models.\n", err)
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
	// Get models from the central registry
	registry := models.GetRegistry()
	return registry.GetModels("anthropic")
}

func getXAIModels() []string {
	// Get models from the central registry
	registry := models.GetRegistry()
	return registry.GetModels("xai")
}

func getDeepseekModels() []string {
	// Get models from the central registry
	registry := models.GetRegistry()
	return registry.GetModels("deepseek")
}

func getGoogleModels() []string {
	// Get models from the central registry
	registry := models.GetRegistry()
	return registry.GetModels("google")
}

func getMoonshotModels() []string {
	// Get models from the central registry
	registry := models.GetRegistry()
	return registry.GetModels("moonshot")
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

// VLLMModel represents a model from vLLM server
type VLLMModel struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

func getVLLMModels() ([]VLLMModel, error) {
	endpoint := "http://localhost:8000"
	resp, err := http.Get(endpoint + "/v1/models")
	if err != nil {
		return nil, fmt.Errorf("error connecting to vLLM API: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vLLM API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var response struct {
		Data []VLLMModel `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("error decoding vLLM response: %v", err)
	}

	return response.Data, nil
}

func checkVLLMInstalled() bool {
	endpoint := "http://localhost:8000"
	resp, err := http.Get(endpoint + "/v1/models")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func validatePassword(password string) error {
	if len(password) < 6 {
		return fmt.Errorf("password must be at least 6 characters long")
	}
	return nil
}

func configureDatabase(reader *bufio.Reader, envConfig *config.EnvConfig) error {
	log.Print("Enter database name: ")
	dbName, _ := reader.ReadString('\n')
	dbName = strings.TrimSpace(dbName)

	// Create new database config
	dbConfig := config.DatabaseConfig{
		Type:     config.PostgreSQL, // Currently only supporting PostgreSQL
		Database: dbName,            // Use the same name for both config and connection
	}

	// Get database connection details
	log.Print("Enter database host (default: localhost): ")
	host, _ := reader.ReadString('\n')
	host = strings.TrimSpace(host)
	if host == "" {
		host = "localhost"
	}
	dbConfig.Host = host

	log.Print("Enter database port (default: 5432): ")
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

	log.Print("Enter database user: ")
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
	log.Print("Would you like to test the database connection? (y/n): ")
	testConn, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(testConn)) == "y" {
		// Create a database handler and test the connection
		dbHandler := database.NewHandler(envConfig)
		if err := dbHandler.TestConnection(dbName); err != nil {
			return fmt.Errorf("connection test failed: %v", err)
		}
		log.Printf("%s Database connection successful!\n", greenCheckmark)
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
				log.Printf("Removed model '%s' from provider '%s'\n", modelName, providerName)
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
	log.Printf("\nAvailable models:\n")
	for i, model := range models {
		log.Printf("%d. %s\n", i+1, model)
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		log.Printf("\nEnter model numbers (comma-separated, ranges allowed e.g., 1,2,4-6): ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		selected, err := parseModelSelection(input, len(models))
		if err != nil {
			log.Printf("Error: %v\nPlease try again.\n", err)
			continue
		}

		if len(selected) == 0 {
			log.Printf("No valid selections made. Please try again.\n")
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
	log.Printf("\nPrimary OpenAI Models:\n")
	for i, model := range primaryModels {
		log.Printf("%d. %s\n", i+1, model)
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

		log.Printf(prompt)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		// Handle pagination
		if input == "m" && showingPrimary {
			showingPrimary = false
			log.Printf("\nOther OpenAI Models:\n")
			for i, model := range otherModels {
				log.Printf("%d. %s\n", i+len(primaryModels)+1, model)
			}
			continue
		} else if input == "p" && !showingPrimary {
			showingPrimary = true
			log.Printf("\nPrimary OpenAI Models:\n")
			for i, model := range primaryModels {
				log.Printf("%d. %s\n", i+1, model)
			}
			continue
		}

		// Parse selection
		selected, err := parseModelSelection(input, len(allModels))
		if err != nil {
			log.Printf("Error: %v\nPlease try again.\n", err)
			continue
		}

		if len(selected) == 0 {
			log.Printf("No valid selections made. Please try again.\n")
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
	log.Printf("\nConfiguring modes for %s\n", modelName)
	log.Printf("Available modes:\n")
	log.Printf("1. text - Text processing mode")
	log.Printf("2. vision - Image and vision processing mode")
	log.Printf("3. multi - Multi-modal processing")
	log.Printf("4. file - File processing mode")
	log.Printf("\nEnter mode numbers (comma-separated, e.g., 1,2): ")
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
				log.Printf("Warning: Invalid mode number '%s' ignored\n", num)
			}
		}
	}

	if len(modes) == 0 {
		// Default to text mode if no modes selected
		modes = append(modes, config.TextMode)
		log.Printf("No valid modes selected. Defaulting to text mode.")
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

		// Handle init-memory flag
		if initMemoryFlag {
			memPath, err := config.InitializeUserMemoryFile()
			if err != nil {
				log.Printf("Error initializing memory file: %v\n", err)
				return
			}
			log.Printf("%s Memory file initialized at: %s\n", greenCheckmark, memPath)
			log.Printf("\nYou can now use memory in your workflows with:")
			log.Printf("  memory: true  # In step config to read from memory")
			log.Printf("  output: MEMORY  # To write to memory")
			return
		}

		// Handle memory flag
		if memoryFlag != "" {
			envCfg, err := config.LoadEnvConfigWithPassword(configPath)
			if err != nil {
				log.Printf("Error loading configuration: %v\n", err)
				return
			}

			// Validate the memory file path
			absPath, err := filepath.Abs(memoryFlag)
			if err != nil {
				log.Printf("Error resolving memory file path: %v\n", err)
				return
			}

			// Check if the file exists or if the parent directory is accessible
			if _, err := os.Stat(absPath); err != nil {
				if os.IsNotExist(err) {
					// Check if parent directory exists and is writable
					parentDir := filepath.Dir(absPath)
					if stat, err := os.Stat(parentDir); err != nil {
						log.Printf("Error: Parent directory %s does not exist or is not accessible: %v\n", parentDir, err)
						return
					} else if !stat.IsDir() {
						log.Printf("Error: %s is not a directory\n", parentDir)
						return
					}
					// Check if directory is writable
					if err := os.WriteFile(filepath.Join(parentDir, ".write_test"), []byte{}, 0644); err != nil {
						log.Printf("Error: Directory %s is not writable: %v\n", parentDir, err)
						return
					}
					os.Remove(filepath.Join(parentDir, ".write_test"))
					log.Printf("Warning: Memory file does not exist at %s. It will be created when needed.\n", absPath)
				} else {
					log.Printf("Error accessing memory file: %v\n", err)
					return
				}
			} else {
				// File exists, check if it's readable
				if _, err := os.ReadFile(absPath); err != nil {
					log.Printf("Error: Memory file is not readable: %v\n", err)
					return
				}
			}

			envCfg.MemoryFile = absPath
			if err := config.SaveEnvConfig(configPath, envCfg); err != nil {
				log.Printf("Error saving configuration: %v\n", err)
				return
			}

			log.Printf("%s Memory file path set to: %s\n", greenCheckmark, absPath)
			return
		}

		if encryptFlag {
			password, err := config.PromptPassword("Enter encryption password (minimum 6 characters): ")
			if err != nil {
				log.Printf("Error reading password: %v\n", err)
				return
			}

			if err := validatePassword(password); err != nil {
				log.Printf("Error: %v\n", err)
				return
			}

			confirmPassword, err := config.PromptPassword("Confirm encryption password: ")
			if err != nil {
				log.Printf("Error reading password: %v\n", err)
				return
			}

			if password != confirmPassword {
				log.Printf("Passwords do not match")
				return
			}

			if err := config.EncryptConfig(configPath, password); err != nil {
				log.Printf("Error encrypting configuration: %v\n", err)
				return
			}
			log.Printf("Configuration encrypted successfully!")
			return
		}

		if decryptFlag {
			// Check if file exists and is encrypted
			data, err := os.ReadFile(configPath)
			if err != nil {
				log.Printf("Error reading configuration file: %v\n", err)
				return
			}

			if !config.IsEncrypted(data) {
				log.Printf("Configuration file is not encrypted")
				return
			}

			password, err := config.PromptPassword("Enter decryption password: ")
			if err != nil {
				log.Printf("Error reading password: %v\n", err)
				return
			}

			// Decrypt the configuration
			decrypted, err := config.DecryptConfig(data, password)
			if err != nil {
				log.Printf("Error decrypting configuration: %v\n", err)
				return
			}

			// Write the decrypted data back to the file
			if err := os.WriteFile(configPath, decrypted, 0644); err != nil {
				log.Printf("Error writing decrypted configuration: %v\n", err)
				return
			}

			log.Printf("Configuration decrypted successfully!")
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
					log.Printf("Error reading password: %v\n", err)
					return
				}
				decryptionPassword = password
			}
		}

		// The environment configuration is already loaded in rootCmd's PersistentPreRunE
		// and available in the package-level envConfig variable

		if updateKeyFlag != "" {
			reader := bufio.NewReader(os.Stdin)
			log.Printf("Enter new API key: ")
			apiKey, _ := reader.ReadString('\n')
			apiKey = strings.TrimSpace(apiKey)

			if err := envConfig.UpdateAPIKey(updateKeyFlag, apiKey); err != nil {
				log.Printf("Error updating API key: %v\n", err)
				return
			}
			log.Printf("Successfully updated API key for provider '%s'\n", updateKeyFlag)
		} else if removeFlag != "" {
			if err := removeModel(envConfig, removeFlag); err != nil {
				log.Printf("Error: %v\n", err)
				return
			}
		} else if databaseFlag {
			reader := bufio.NewReader(os.Stdin)
			if err := configureDatabase(reader, envConfig); err != nil {
				log.Printf("Error configuring database: %v\n", err)
				return
			}
		} else if setDefaultGenerationModelFlag != "" {
			envConfig.DefaultGenerationModel = setDefaultGenerationModelFlag
			log.Printf("Default generation model set to '%s'\n", setDefaultGenerationModelFlag)
		} else if defaultFlag {
			// Interactive mode to set default generation model
			reader := bufio.NewReader(os.Stdin)
			allModels := getAllConfiguredModelNames(envConfig)
			if len(allModels) == 0 {
				log.Printf("No models are currently configured. Please configure models first using 'comanda configure'.")
				return
			}

			log.Printf("Available configured models:")
			for i, modelName := range allModels {
				log.Printf("%d. %s", i+1, modelName)
				if envConfig.DefaultGenerationModel == modelName {
					log.Printf(" (current default)")
				}
				log.Printf("\n")
			}

			var selectedDefaultModel string
			for {
				log.Printf("\nEnter the number of the model to set as default for generation: ")
				selectionInput, _ := reader.ReadString('\n')
				selectionNum, err := strconv.Atoi(strings.TrimSpace(selectionInput))
				if err != nil || selectionNum < 1 || selectionNum > len(allModels) {
					log.Printf("Invalid selection. Please enter a number from the list.")
					continue
				}
				selectedDefaultModel = allModels[selectionNum-1]
				break
			}

			envConfig.DefaultGenerationModel = selectedDefaultModel
			log.Printf("%s Default generation model set to '%s'\n", greenCheckmark, selectedDefaultModel)
		} else {
			reader := bufio.NewReader(os.Stdin)
			// Prompt for provider
			var provider string
			for {
				log.Printf("Enter provider (openai/anthropic/ollama/vllm/google/xai/deepseek/moonshot): ")
				provider, _ = reader.ReadString('\n')
				provider = strings.TrimSpace(provider)
				if provider == "openai" || provider == "anthropic" || provider == "ollama" || provider == "vllm" || provider == "google" || provider == "xai" || provider == "deepseek" || provider == "moonshot" {
					break
				}
				log.Printf("Invalid provider. Please enter 'openai', 'anthropic', 'ollama', 'vllm', 'google', 'xai', 'deepseek', or 'moonshot'")
			}

			// Special handling for local providers
			if provider == "ollama" {
				if !checkOllamaInstalled() {
					log.Printf("Error: Ollama is not installed or not running. Please install ollama and try again.")
					return
				}
			}
			if provider == "vllm" {
				if !checkVLLMInstalled() {
					log.Printf("Error: vLLM server is not running. Please start vLLM server and try again.")
					return
				}
			}

			// Check if provider exists
			existingProvider, err := envConfig.GetProviderConfig(provider)
			var apiKey string
			if err != nil {
				if provider != "ollama" && provider != "vllm" {
					// Only prompt for API key if not local providers
					log.Printf("Enter API key: ")
					apiKey, _ = reader.ReadString('\n')
					apiKey = strings.TrimSpace(apiKey)
				} else {
					// For local providers (ollama, vllm), use "LOCAL" as the API key
					apiKey = "LOCAL"
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
					log.Printf("Error: API key is required for OpenAI")
					return
				}

				// Fetch and categorize OpenAI models
				primaryModels, otherModels, err := getOpenAIModelsAndCategorize(apiKey)
				if err != nil {
					log.Printf("Error fetching OpenAI models: %v\n", err)
					return
				}

				// Use the paginated selection for OpenAI
				selectedModels, err = promptForOpenAIModelSelection(primaryModels, otherModels)
				if err != nil {
					log.Printf("Error selecting models: %v\n", err)
					return
				}

			case "anthropic":
				if apiKey == "" {
					log.Printf("Error: API key is required for Anthropic")
					return
				}
				models := getAnthropicModels()
				selectedModels, err = promptForModelSelection(models)
				if err != nil {
					log.Printf("Error selecting models: %v\n", err)
					return
				}

			case "xai":
				if apiKey == "" {
					log.Printf("Error: API key is required for X.AI")
					return
				}
				models := getXAIModels()
				selectedModels, err = promptForModelSelection(models)
				if err != nil {
					log.Printf("Error selecting models: %v\n", err)
					return
				}

			case "deepseek":
				if apiKey == "" {
					log.Printf("Error: API key is required for Deepseek")
					return
				}
				models := getDeepseekModels()
				selectedModels, err = promptForModelSelection(models)
				if err != nil {
					log.Printf("Error selecting models: %v\n", err)
					return
				}

			case "google":
				if apiKey == "" {
					log.Printf("Error: API key is required for Google")
					return
				}
				models := getGoogleModels()
				selectedModels, err = promptForModelSelection(models)
				if err != nil {
					log.Printf("Error selecting models: %v\n", err)
					return
				}

			case "moonshot":
				if apiKey == "" {
					log.Printf("Error: API key is required for Moonshot")
					return
				}
				models := getMoonshotModels()
				selectedModels, err = promptForModelSelection(models)
				if err != nil {
					log.Printf("Error selecting models: %v\n", err)
					return
				}

			case "ollama":
				models, err := getOllamaModels()
				if err != nil {
					log.Printf("Error fetching Ollama models: %v\n", err)
					return
				}
				modelNames := make([]string, len(models))
				for i, model := range models {
					modelNames[i] = model.Name
				}
				if len(modelNames) == 0 {
					log.Printf("No models found. Please pull a model first using 'ollama pull <model>'")
					return
				}
				selectedModels, err = promptForModelSelection(modelNames)
				if err != nil {
					log.Printf("Error selecting models: %v\n", err)
					return
				}

			case "vllm":
				models, err := getVLLMModels()
				if err != nil {
					log.Printf("Error fetching vLLM models: %v\n", err)
					return
				}
				modelNames := make([]string, len(models))
				for i, model := range models {
					modelNames[i] = model.ID
				}
				if len(modelNames) == 0 {
					log.Printf("No models found. Please start vLLM server with a model first")
					return
				}
				selectedModels, err = promptForModelSelection(modelNames)
				if err != nil {
					log.Printf("Error selecting models: %v\n", err)
					return
				}
			}

			// Add new models to provider
			modelType := "external"
			if provider == "ollama" || provider == "vllm" {
				modelType = "local"
			}

			for _, modelName := range selectedModels {
				// Prompt for modes for each model
				modes, err := promptForModes(reader, modelName)
				if err != nil {
					log.Printf("Error configuring modes for model %s: %v\n", modelName, err)
					continue
				}

				newModel := config.Model{
					Name:  modelName,
					Type:  modelType,
					Modes: modes,
				}

				if err := envConfig.AddModelToProvider(provider, newModel); err != nil {
					log.Printf("Error adding model %s: %v\n", modelName, err)
					continue
				}
			}

			// Prompt to set default generation model if not using a specific flag for it
			if setDefaultGenerationModelFlag == "" {
				log.Printf("\nDo you want to set or update the default model for workflow generation? (y/n): ")
				setDefaultGenModelInput, _ := reader.ReadString('\n')
				if strings.TrimSpace(strings.ToLower(setDefaultGenModelInput)) == "y" {
					allModels := getAllConfiguredModelNames(envConfig)
					if len(allModels) == 0 {
						log.Printf("No models are currently configured. Cannot set a default generation model.")
					} else {
						log.Printf("\nAvailable configured models for default generation:")
						for i, modelName := range allModels {
							log.Printf("%d. %s\n", i+1, modelName)
						}
						var selectedDefaultModel string
						for {
							log.Printf("Enter the number of the model to set as default: ")
							selectionInput, _ := reader.ReadString('\n')
							selectionNum, err := strconv.Atoi(strings.TrimSpace(selectionInput))
							if err != nil || selectionNum < 1 || selectionNum > len(allModels) {
								log.Printf("Invalid selection. Please enter a number from the list.")
								continue
							}
							selectedDefaultModel = allModels[selectionNum-1]
							break
						}
						envConfig.DefaultGenerationModel = selectedDefaultModel
						log.Printf("%s Default generation model set to '%s'\n", greenCheckmark, selectedDefaultModel)
					}
				}
			}
		}

		// Create parent directory if it doesn't exist
		if dir := filepath.Dir(configPath); dir != "." && dir != "/" {
			if err := os.MkdirAll(dir, 0755); err != nil {
				log.Printf("Error creating directory: %v\n", err)
				return
			}
		}

		// Save configuration
		if err := config.SaveEnvConfig(configPath, envConfig); err != nil {
			log.Printf("Error saving configuration: %v\n", err)
			return
		}

		// Re-encrypt if it was encrypted before
		if wasEncrypted {
			if err := config.EncryptConfig(configPath, decryptionPassword); err != nil {
				log.Printf("Error re-encrypting configuration: %v\n", err)
				return
			}
		}

		log.Printf("Configuration saved successfully to %s!\n", configPath)
	},
}

// listConfiguration displays the current configuration
// It can be called either from the command (using the package-level envConfig)
// or directly from tests (with a provided config)
func listConfiguration() {
	// For tests that call this function directly, we need to load the config
	// since the package-level envConfig won't be initialized
	var cfg *config.EnvConfig
	var err error

	if envConfig == nil {
		// We're being called from a test, load the config
		configPath := config.GetEnvPath()
		cfg, err = config.LoadEnvConfigWithPassword(configPath)
		if err != nil {
			log.Printf("Error loading configuration: %v\n", err)
			return
		}
	} else {
		// We're being called from the command, use the package-level config
		cfg = envConfig
	}

	// Get the config path for display purposes
	configPath := config.GetEnvPath()
	log.Printf("Configuration from %s:\n\n", configPath)

	// List default generation model
	if cfg.DefaultGenerationModel != "" {
		log.Printf("Default Generation Model: %s\n\n", cfg.DefaultGenerationModel)
	}

	// List memory file if configured
	memoryPath := config.GetMemoryPath(cfg)
	if memoryPath != "" {
		log.Printf("Memory File: %s\n\n", memoryPath)
	}

	// List server configuration if it exists
	if server := cfg.GetServerConfig(); server != nil {
		log.Printf("Server Configuration:")
		log.Printf("Port: %d\n", server.Port)
		log.Printf("Data Directory: %s\n", server.DataDir)
		log.Printf("Authentication Enabled: %v\n", server.Enabled)
		if server.BearerToken != "" {
			log.Printf("Bearer Token: %s\n", server.BearerToken)
		}
		log.Printf("\n")
	}

	// List databases if they exist
	if len(cfg.Databases) > 0 {
		log.Printf("Database Configurations:")
		for name, db := range cfg.Databases {
			log.Printf("\n%s:\n", name)
			log.Printf("  Type: %s\n", db.Type)
			log.Printf("  Host: %s\n", db.Host)
			log.Printf("  Port: %d\n", db.Port)
			log.Printf("  User: %s\n", db.User)
			log.Printf("  Database: %s\n", db.Database)
		}
		log.Printf("\n")
	}

	// List providers
	if len(cfg.Providers) == 0 {
		log.Printf("No providers configured.")
		return
	}

	log.Printf("Configured Providers:")
	for name, provider := range cfg.Providers {
		log.Printf("\n%s:\n", name)
		if len(provider.Models) == 0 {
			log.Printf("  No models configured")
			continue
		}
		for _, model := range provider.Models {
			log.Printf("  - %s (%s)\n", model.Name, model.Type)
			if len(model.Modes) > 0 {
				modeStr := make([]string, len(model.Modes))
				for i, mode := range model.Modes {
					modeStr[i] = string(mode)
				}
				log.Printf("    Modes: %s\n", strings.Join(modeStr, ", "))
			} else {
				log.Printf("    Modes: none\n")
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
	configureCmd.Flags().StringVar(&memoryFlag, "memory", "", "Set path to COMANDA.md memory file")
	configureCmd.Flags().BoolVar(&initMemoryFlag, "init-memory", false, "Initialize a new memory file at ~/.comanda/COMANDA.md")
	rootCmd.AddCommand(configureCmd)
}
