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

// getAnthropicModelsAndCategorize fetches and categorizes Anthropic models
// Primary models are the main Claude family models, others are additional models from the API
func getAnthropicModelsAndCategorize(apiKey string) ([]string, []string, error) {
	// Get primary models from the registry (these are the known, curated models)
	registry := models.GetRegistry()
	primaryModels := registry.GetModels("anthropic")

	// Try to fetch additional models from the API
	var otherModels []string
	if apiKey != "" {
		apiModels, err := models.ListAnthropicModels(apiKey)
		if err != nil {
			log.Printf("Warning: Could not fetch models from Anthropic API: %v\nUsing known models only.\n", err)
		} else {
			// Create a set of primary models for quick lookup
			primarySet := make(map[string]bool)
			for _, m := range primaryModels {
				primarySet[m] = true
			}

			// Add any API models not in the primary list to other models
			for _, apiModel := range apiModels {
				if !primarySet[apiModel] {
					otherModels = append(otherModels, apiModel)
				}
			}
			sort.Strings(otherModels)
		}
	}

	return primaryModels, otherModels, nil
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

		log.Printf("%s", prompt)
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

// promptForAnthropicModelSelection handles the paginated selection of Anthropic models
func promptForAnthropicModelSelection(primaryModels []string, otherModels []string) ([]string, error) {
	reader := bufio.NewReader(os.Stdin)

	// Display primary models first
	log.Printf("\nPrimary Anthropic Models:\n")
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
			prompt = "\nEnter model numbers, or 'm' to see more models from API: "
		} else {
			prompt = "\nEnter model numbers, or 'p' to see primary models: "
		}

		log.Printf("%s", prompt)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		// Handle pagination
		if input == "m" && showingPrimary {
			showingPrimary = false
			log.Printf("\nOther Anthropic Models (from API):\n")
			for i, model := range otherModels {
				log.Printf("%d. %s\n", i+len(primaryModels)+1, model)
			}
			continue
		} else if input == "p" && !showingPrimary {
			showingPrimary = true
			log.Printf("\nPrimary Anthropic Models:\n")
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

// getClaudeCodeModels returns the available Claude Code CLI models
func getClaudeCodeModels() []string {
	return []string{"claude-code", "claude-code-opus", "claude-code-sonnet", "claude-code-haiku"}
}

// getGeminiCLIModels returns the available Gemini CLI models
func getGeminiCLIModels() []string {
	return []string{"gemini-cli", "gemini-cli-pro", "gemini-cli-flash", "gemini-cli-flash-lite"}
}

// getOpenAICodexModels returns the available OpenAI Codex CLI models
func getOpenAICodexModels() []string {
	return []string{"openai-codex", "openai-codex-o3", "openai-codex-o4-mini", "openai-codex-mini", "openai-codex-gpt-4.1", "openai-codex-gpt-4o"}
}

// configureCLIAgent handles the configuration flow for CLI-based agents
func configureCLIAgent(reader *bufio.Reader, envConfig *config.EnvConfig, providerName string, displayName string, availableModels []string) {
	log.Printf("\n%s %s CLI is installed and ready to use!\n", greenCheckmark, displayName)
	log.Printf("\nAvailable models:\n")
	for i, model := range availableModels {
		log.Printf("  %d. %s\n", i+1, model)
	}

	log.Printf("\nThese models are automatically available for use in workflows.\n")
	log.Printf("No API key is required - the CLI handles authentication.\n")

	// Ask if user wants to set a default generation model
	log.Printf("\nWould you like to set one of these as your default generation model? (y/n): ")
	setDefault, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(setDefault)) == "y" {
		log.Printf("Enter the number of the model to set as default: ")
		selectionInput, _ := reader.ReadString('\n')
		selectionNum, err := strconv.Atoi(strings.TrimSpace(selectionInput))
		if err != nil || selectionNum < 1 || selectionNum > len(availableModels) {
			log.Printf("Invalid selection. Skipping default model configuration.\n")
			return
		}
		selectedModel := availableModels[selectionNum-1]
		envConfig.DefaultGenerationModel = selectedModel
		log.Printf("%s Default generation model set to '%s'\n", greenCheckmark, selectedModel)
	} else {
		log.Printf("\nYou can use %s models in workflows by specifying the model name.\n", displayName)
		log.Printf("Example: model: %s\n", availableModels[0])
	}

	// Always ensure config directory exists and save the config
	configPath := config.GetEnvPath()
	if dir := filepath.Dir(configPath); dir != "." && dir != "/" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			log.Printf("Error creating config directory: %v\n", err)
			return
		}
	}
	if err := config.SaveEnvConfig(configPath, envConfig); err != nil {
		log.Printf("Error saving configuration: %v\n", err)
		return
	}
	log.Printf("Configuration saved to %s\n", configPath)
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

// showMainMenu displays the main configuration menu and returns the user's choice
func showMainMenu(reader *bufio.Reader) string {
	log.Printf("\n")
	log.Printf("┌────────────────────────────────────────┐\n")
	log.Printf("│       Comanda Configuration            │\n")
	log.Printf("├────────────────────────────────────────┤\n")
	log.Printf("│  1. Models & Providers                 │\n")
	log.Printf("│  2. Server Settings                    │\n")
	log.Printf("│  3. Database Connections               │\n")
	log.Printf("│  4. Tool Settings (allowlist/denylist) │\n")
	log.Printf("│  5. Memory Settings                    │\n")
	log.Printf("│  6. Security (encrypt/decrypt)         │\n")
	log.Printf("│  7. View Current Configuration         │\n")
	log.Printf("│  0. Save & Exit                        │\n")
	log.Printf("└────────────────────────────────────────┘\n")
	log.Printf("\nEnter choice: ")
	choice, _ := reader.ReadString('\n')
	return strings.TrimSpace(choice)
}

// configureModelsAndProviders handles the models and providers submenu
func configureModelsAndProviders(reader *bufio.Reader, envConfig *config.EnvConfig) {
	for {
		log.Printf("\n")
		log.Printf("┌─────────────────────────────────────┐\n")
		log.Printf("│     Models & Providers              │\n")
		log.Printf("├─────────────────────────────────────┤\n")
		log.Printf("│  1. Add API Provider (OpenAI, etc.) │\n")
		log.Printf("│  2. Add Local Provider (Ollama)     │\n")
		log.Printf("│  3. Configure CLI Agents            │\n")
		log.Printf("│  4. Set Default Generation Model    │\n")
		log.Printf("│  5. Remove a Model                  │\n")
		log.Printf("│  6. Update API Key                  │\n")
		log.Printf("│  0. Back to Main Menu               │\n")
		log.Printf("└─────────────────────────────────────┘\n")
		log.Printf("\nEnter choice: ")
		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			configureAPIProvider(reader, envConfig)
		case "2":
			configureLocalProvider(reader, envConfig)
		case "3":
			configureCLIAgents(reader, envConfig)
		case "4":
			configureDefaultModel(reader, envConfig)
		case "5":
			removeModelInteractive(reader, envConfig)
		case "6":
			updateAPIKeyInteractive(reader, envConfig)
		case "0":
			return
		default:
			log.Printf("Invalid selection.\n")
		}
	}
}

// configureAPIProvider handles adding API-based providers
func configureAPIProvider(reader *bufio.Reader, envConfig *config.EnvConfig) {
	log.Printf("\nAvailable API providers: openai, anthropic, google, xai, deepseek, moonshot\n")
	log.Printf("Enter provider name: ")
	provider, _ := reader.ReadString('\n')
	provider = strings.TrimSpace(provider)

	validProviders := []string{"openai", "anthropic", "google", "xai", "deepseek", "moonshot"}
	isValid := false
	for _, vp := range validProviders {
		if provider == vp {
			isValid = true
			break
		}
	}
	if !isValid {
		log.Printf("Invalid provider. Please choose from: %v\n", validProviders)
		return
	}

	// Check if provider exists
	existingProvider, err := envConfig.GetProviderConfig(provider)
	var apiKey string
	if err != nil {
		log.Printf("Enter API key: ")
		apiKey, _ = reader.ReadString('\n')
		apiKey = strings.TrimSpace(apiKey)
		existingProvider = &config.Provider{
			APIKey: apiKey,
			Models: []config.Model{},
		}
		envConfig.AddProvider(provider, *existingProvider)
	} else {
		apiKey = existingProvider.APIKey
		log.Printf("Provider %s already configured. Adding more models...\n", provider)
	}

	// Get and select models based on provider
	var selectedModels []string
	var selectErr error
	switch provider {
	case "openai":
		primaryModels, otherModels, err := getOpenAIModelsAndCategorize(apiKey)
		if err != nil {
			log.Printf("Error fetching OpenAI models: %v\n", err)
			return
		}
		selectedModels, selectErr = promptForOpenAIModelSelection(primaryModels, otherModels)
	case "anthropic":
		primaryModels, otherModels, err := getAnthropicModelsAndCategorize(apiKey)
		if err != nil {
			log.Printf("Error fetching Anthropic models: %v\n", err)
			return
		}
		if len(otherModels) > 0 {
			selectedModels, selectErr = promptForAnthropicModelSelection(primaryModels, otherModels)
		} else {
			selectedModels, selectErr = promptForModelSelection(primaryModels)
		}
	case "google":
		selectedModels, selectErr = promptForModelSelection(getGoogleModels())
	case "xai":
		selectedModels, selectErr = promptForModelSelection(getXAIModels())
	case "deepseek":
		selectedModels, selectErr = promptForModelSelection(getDeepseekModels())
	case "moonshot":
		selectedModels, selectErr = promptForModelSelection(getMoonshotModels())
	}

	if selectErr != nil {
		log.Printf("Error selecting models: %v\n", selectErr)
		return
	}

	// Add selected models
	for _, modelName := range selectedModels {
		modes, err := promptForModes(reader, modelName)
		if err != nil {
			log.Printf("Error configuring modes for model %s: %v\n", modelName, err)
			continue
		}
		newModel := config.Model{
			Name:  modelName,
			Type:  "external",
			Modes: modes,
		}
		if err := envConfig.AddModelToProvider(provider, newModel); err != nil {
			log.Printf("Error adding model %s: %v\n", modelName, err)
		} else {
			log.Printf("%s Added model: %s\n", greenCheckmark, modelName)
		}
	}
}

// configureLocalProvider handles adding local providers (Ollama, vLLM)
func configureLocalProvider(reader *bufio.Reader, envConfig *config.EnvConfig) {
	log.Printf("\nAvailable local providers: ollama, vllm\n")
	log.Printf("Enter provider name: ")
	provider, _ := reader.ReadString('\n')
	provider = strings.TrimSpace(provider)

	if provider != "ollama" && provider != "vllm" {
		log.Printf("Invalid provider. Choose 'ollama' or 'vllm'\n")
		return
	}

	if provider == "ollama" && !checkOllamaInstalled() {
		log.Printf("Error: Ollama is not installed or not running.\n")
		return
	}
	if provider == "vllm" && !checkVLLMInstalled() {
		log.Printf("Error: vLLM server is not running.\n")
		return
	}

	// Ensure provider exists
	if _, err := envConfig.GetProviderConfig(provider); err != nil {
		envConfig.AddProvider(provider, config.Provider{APIKey: "LOCAL", Models: []config.Model{}})
	}

	var modelNames []string
	if provider == "ollama" {
		ollamaModels, err := getOllamaModels()
		if err != nil {
			log.Printf("Error fetching Ollama models: %v\n", err)
			return
		}
		for _, m := range ollamaModels {
			modelNames = append(modelNames, m.Name)
		}
	} else {
		vllmModels, err := getVLLMModels()
		if err != nil {
			log.Printf("Error fetching vLLM models: %v\n", err)
			return
		}
		for _, m := range vllmModels {
			modelNames = append(modelNames, m.ID)
		}
	}

	if len(modelNames) == 0 {
		log.Printf("No models found. Please pull/load a model first.\n")
		return
	}

	selectedModels, err := promptForModelSelection(modelNames)
	if err != nil {
		log.Printf("Error selecting models: %v\n", err)
		return
	}

	for _, modelName := range selectedModels {
		modes, err := promptForModes(reader, modelName)
		if err != nil {
			continue
		}
		newModel := config.Model{Name: modelName, Type: "local", Modes: modes}
		if err := envConfig.AddModelToProvider(provider, newModel); err != nil {
			log.Printf("Error adding model %s: %v\n", modelName, err)
		} else {
			log.Printf("%s Added model: %s\n", greenCheckmark, modelName)
		}
	}
}

// configureCLIAgents handles CLI agent configuration
func configureCLIAgents(reader *bufio.Reader, envConfig *config.EnvConfig) {
	log.Printf("\nCLI Agents (auto-detected from PATH):\n")

	if models.IsClaudeCodeAvailable() {
		log.Printf("  %s Claude Code - available\n", greenCheckmark)
		log.Printf("     Models: %v\n", getClaudeCodeModels())
	} else {
		log.Printf("  ✗ Claude Code - not installed\n")
		log.Printf("     Install: npm install -g @anthropic-ai/claude-code\n")
	}

	if models.IsGeminiCLIAvailable() {
		log.Printf("  %s Gemini CLI - available\n", greenCheckmark)
		log.Printf("     Models: %v\n", getGeminiCLIModels())
	} else {
		log.Printf("  ✗ Gemini CLI - not installed\n")
		log.Printf("     Install: npm install -g @google/gemini-cli\n")
	}

	if models.IsOpenAICodexAvailable() {
		log.Printf("  %s OpenAI Codex - available\n", greenCheckmark)
		log.Printf("     Models: %v\n", getOpenAICodexModels())
	} else {
		log.Printf("  ✗ OpenAI Codex - not installed\n")
		log.Printf("     Install: npm install -g @openai/codex\n")
	}

	log.Printf("\nCLI agents are auto-configured. Install them and they're ready to use.\n")
	log.Printf("Press Enter to continue...")
	_, _ = reader.ReadString('\n')
}

// configureDefaultModel handles setting the default generation model
func configureDefaultModel(reader *bufio.Reader, envConfig *config.EnvConfig) {
	allModels := getAllConfiguredModelNames(envConfig)

	// Add CLI agent models if available
	var cliModels []string
	if models.IsClaudeCodeAvailable() {
		cliModels = append(cliModels, getClaudeCodeModels()...)
	}
	if models.IsGeminiCLIAvailable() {
		cliModels = append(cliModels, getGeminiCLIModels()...)
	}
	if models.IsOpenAICodexAvailable() {
		cliModels = append(cliModels, getOpenAICodexModels()...)
	}

	if len(allModels) == 0 && len(cliModels) == 0 {
		log.Printf("No models available. Configure providers first.\n")
		return
	}

	log.Printf("\nAvailable models:\n")
	currentIdx := 0
	if len(allModels) > 0 {
		log.Printf("API/Local models:\n")
		for i, modelName := range allModels {
			currentIdx = i + 1
			marker := ""
			if envConfig.DefaultGenerationModel == modelName {
				marker = " (current default)"
			}
			log.Printf("  %d. %s%s\n", currentIdx, modelName, marker)
		}
	}
	if len(cliModels) > 0 {
		log.Printf("CLI Agent models:\n")
		for i, modelName := range cliModels {
			idx := len(allModels) + i + 1
			marker := ""
			if envConfig.DefaultGenerationModel == modelName {
				marker = " (current default)"
			}
			log.Printf("  %d. %s%s\n", idx, modelName, marker)
		}
	}

	combinedModels := append(allModels, cliModels...)
	for {
		log.Printf("\nEnter number to set as default (0 to cancel): ")
		input, _ := reader.ReadString('\n')
		num, err := strconv.Atoi(strings.TrimSpace(input))
		if err != nil || num < 0 || num > len(combinedModels) {
			log.Printf("Invalid selection.\n")
			continue
		}
		if num == 0 {
			return
		}
		envConfig.DefaultGenerationModel = combinedModels[num-1]
		log.Printf("%s Default model set to: %s\n", greenCheckmark, combinedModels[num-1])
		return
	}
}

// removeModelInteractive handles removing a model
func removeModelInteractive(reader *bufio.Reader, envConfig *config.EnvConfig) {
	allModels := getAllConfiguredModelNames(envConfig)
	if len(allModels) == 0 {
		log.Printf("No models configured.\n")
		return
	}

	log.Printf("\nConfigured models:\n")
	for i, name := range allModels {
		log.Printf("  %d. %s\n", i+1, name)
	}
	log.Printf("\nEnter number to remove (0 to cancel): ")
	input, _ := reader.ReadString('\n')
	num, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || num < 0 || num > len(allModels) {
		log.Printf("Invalid selection.\n")
		return
	}
	if num == 0 {
		return
	}
	modelName := allModels[num-1]
	if err := removeModel(envConfig, modelName); err != nil {
		log.Printf("Error removing model: %v\n", err)
	} else {
		log.Printf("%s Removed model: %s\n", greenCheckmark, modelName)
	}
}

// updateAPIKeyInteractive handles updating an API key
func updateAPIKeyInteractive(reader *bufio.Reader, envConfig *config.EnvConfig) {
	if len(envConfig.Providers) == 0 {
		log.Printf("No providers configured.\n")
		return
	}

	log.Printf("\nConfigured providers:\n")
	var providers []string
	i := 1
	for name := range envConfig.Providers {
		providers = append(providers, name)
		log.Printf("  %d. %s\n", i, name)
		i++
	}

	log.Printf("\nEnter number to update API key (0 to cancel): ")
	input, _ := reader.ReadString('\n')
	num, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil || num < 0 || num > len(providers) {
		log.Printf("Invalid selection.\n")
		return
	}
	if num == 0 {
		return
	}

	providerName := providers[num-1]
	log.Printf("Enter new API key for %s: ", providerName)
	newKey, _ := reader.ReadString('\n')
	newKey = strings.TrimSpace(newKey)

	if err := envConfig.UpdateAPIKey(providerName, newKey); err != nil {
		log.Printf("Error updating API key: %v\n", err)
	} else {
		log.Printf("%s API key updated for %s\n", greenCheckmark, providerName)
	}
}

// configureServerSettings handles server configuration
func configureServerSettings(reader *bufio.Reader, envConfig *config.EnvConfig) error {
	serverCfg := envConfig.GetServerConfig()

	log.Printf("\nCurrent server settings:\n")
	log.Printf("  Enabled: %v\n", serverCfg.Enabled)
	log.Printf("  Port: %d\n", serverCfg.Port)
	log.Printf("  Data Directory: %s\n", serverCfg.DataDir)
	if serverCfg.BearerToken != "" {
		log.Printf("  Bearer Token: (configured)\n")
	} else {
		log.Printf("  Bearer Token: (not set)\n")
	}

	log.Printf("\nEnable server? (y/n, current: %v): ", serverCfg.Enabled)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "y" {
		serverCfg.Enabled = true
	} else if input == "n" {
		serverCfg.Enabled = false
	}

	log.Printf("Server port (current: %d, press Enter to keep): ", serverCfg.Port)
	portInput, _ := reader.ReadString('\n')
	portInput = strings.TrimSpace(portInput)
	if portInput != "" {
		if port, err := strconv.Atoi(portInput); err == nil && port > 0 && port < 65536 {
			serverCfg.Port = port
		}
	}

	log.Printf("Generate new bearer token? (y/n): ")
	tokenInput, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(tokenInput)) == "y" {
		token, err := config.GenerateBearerToken()
		if err != nil {
			return fmt.Errorf("failed to generate token: %w", err)
		}
		serverCfg.BearerToken = token
		log.Printf("%s Bearer token generated\n", greenCheckmark)
	}

	envConfig.UpdateServerConfig(*serverCfg)
	log.Printf("%s Server settings updated\n", greenCheckmark)
	return nil
}

// configureToolSettings handles global tool configuration
func configureToolSettings(reader *bufio.Reader, envConfig *config.EnvConfig) {
	toolCfg := envConfig.GetToolConfig()
	if toolCfg == nil {
		toolCfg = &config.ToolConfig{}
	}

	log.Printf("\nGlobal Tool Settings\n")
	log.Printf("These settings apply to all workflows unless overridden at the step level.\n\n")
	log.Printf("Current settings:\n")
	log.Printf("  Additional allowlist: %v\n", toolCfg.Allowlist)
	log.Printf("  Additional denylist: %v\n", toolCfg.Denylist)
	log.Printf("  Default timeout: %d seconds (0 = use system default of 30s)\n", toolCfg.Timeout)

	log.Printf("\nCommands to add to global allowlist (comma-separated, or Enter to skip):\n")
	log.Printf("Example: bd,mytool,customcli\n")
	log.Printf("> ")
	allowInput, _ := reader.ReadString('\n')
	allowInput = strings.TrimSpace(allowInput)
	if allowInput != "" {
		parts := strings.Split(allowInput, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				// Check if already in list
				found := false
				for _, existing := range toolCfg.Allowlist {
					if existing == p {
						found = true
						break
					}
				}
				if !found {
					toolCfg.Allowlist = append(toolCfg.Allowlist, p)
				}
			}
		}
	}

	log.Printf("\nCommands to add to global denylist (comma-separated, or Enter to skip):\n")
	log.Printf("> ")
	denyInput, _ := reader.ReadString('\n')
	denyInput = strings.TrimSpace(denyInput)
	if denyInput != "" {
		parts := strings.Split(denyInput, ",")
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				found := false
				for _, existing := range toolCfg.Denylist {
					if existing == p {
						found = true
						break
					}
				}
				if !found {
					toolCfg.Denylist = append(toolCfg.Denylist, p)
				}
			}
		}
	}

	log.Printf("\nDefault timeout in seconds (current: %d, Enter to keep, 0 for system default): ", toolCfg.Timeout)
	timeoutInput, _ := reader.ReadString('\n')
	timeoutInput = strings.TrimSpace(timeoutInput)
	if timeoutInput != "" {
		if timeout, err := strconv.Atoi(timeoutInput); err == nil && timeout >= 0 {
			toolCfg.Timeout = timeout
		}
	}

	envConfig.SetToolConfig(toolCfg)
	log.Printf("\n%s Tool settings updated\n", greenCheckmark)
	log.Printf("  Allowlist: %v\n", toolCfg.Allowlist)
	log.Printf("  Denylist: %v\n", toolCfg.Denylist)
	log.Printf("  Timeout: %d\n", toolCfg.Timeout)
}

// configureMemorySettings handles memory file configuration
func configureMemorySettings(reader *bufio.Reader, envConfig *config.EnvConfig) {
	currentPath := envConfig.MemoryFile
	if currentPath == "" {
		currentPath = "(not configured)"
	}
	log.Printf("\nMemory Settings\n")
	log.Printf("Current memory file: %s\n", currentPath)

	log.Printf("\nOptions:\n")
	log.Printf("  1. Set memory file path\n")
	log.Printf("  2. Initialize default memory file (~/.comanda/COMANDA.md)\n")
	log.Printf("  0. Back\n")
	log.Printf("\nChoice: ")

	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	switch choice {
	case "1":
		log.Printf("Enter path to memory file: ")
		path, _ := reader.ReadString('\n')
		path = strings.TrimSpace(path)
		if path != "" {
			absPath, err := filepath.Abs(path)
			if err != nil {
				log.Printf("Error resolving path: %v\n", err)
				return
			}
			envConfig.MemoryFile = absPath
			log.Printf("%s Memory file set to: %s\n", greenCheckmark, absPath)
		}
	case "2":
		memPath, err := config.InitializeUserMemoryFile()
		if err != nil {
			log.Printf("Error initializing memory file: %v\n", err)
			return
		}
		envConfig.MemoryFile = memPath
		log.Printf("%s Memory file initialized at: %s\n", greenCheckmark, memPath)
	}
}

// configureSecuritySettings handles encryption/decryption
func configureSecuritySettings(reader *bufio.Reader, envConfig *config.EnvConfig, configPath string) {
	for {
		log.Printf("\nSecurity Settings\n")

		// Check current encryption status
		data, err := os.ReadFile(configPath)
		isEncrypted := err == nil && config.IsEncrypted(data)

		if isEncrypted {
			log.Printf("Configuration is currently: ENCRYPTED\n")
		} else {
			log.Printf("Configuration is currently: NOT ENCRYPTED\n")
		}

		// Show index encryption key status
		if envConfig.IndexEncryptionKey != "" {
			log.Printf("Index encryption key: CONFIGURED\n")
		} else {
			log.Printf("Index encryption key: NOT SET\n")
		}

		log.Printf("\nOptions:\n")
		if isEncrypted {
			log.Printf("  1. Decrypt configuration\n")
		} else {
			log.Printf("  1. Encrypt configuration\n")
		}
		log.Printf("  2. Set index encryption key\n")
		log.Printf("  0. Back\n")
		log.Printf("\nChoice: ")

		choice, _ := reader.ReadString('\n')
		choice = strings.TrimSpace(choice)

		switch choice {
		case "0":
			return
		case "1":
			if isEncrypted {
				// Decrypt
				password, err := config.PromptPassword("Enter decryption password: ")
				if err != nil {
					log.Printf("Error reading password: %v\n", err)
					continue
				}
				decrypted, err := config.DecryptConfig(data, password)
				if err != nil {
					log.Printf("Error decrypting: %v\n", err)
					continue
				}
				if err := os.WriteFile(configPath, decrypted, 0644); err != nil {
					log.Printf("Error writing decrypted config: %v\n", err)
					continue
				}
				log.Printf("%s Configuration decrypted\n", greenCheckmark)
			} else {
				// Encrypt
				password, err := config.PromptPassword("Enter encryption password (min 6 chars): ")
				if err != nil {
					log.Printf("Error reading password: %v\n", err)
					continue
				}
				if err := validatePassword(password); err != nil {
					log.Printf("Error: %v\n", err)
					continue
				}
				confirm, err := config.PromptPassword("Confirm password: ")
				if err != nil {
					log.Printf("Error reading password: %v\n", err)
					continue
				}
				if password != confirm {
					log.Printf("Passwords do not match\n")
					continue
				}
				// Need to save first, then encrypt
				if err := config.SaveEnvConfig(configPath, envConfig); err != nil {
					log.Printf("Error saving before encryption: %v\n", err)
					continue
				}
				if err := config.EncryptConfig(configPath, password); err != nil {
					log.Printf("Error encrypting: %v\n", err)
					continue
				}
				log.Printf("%s Configuration encrypted\n", greenCheckmark)
			}
		case "2":
			configureIndexEncryptionKey(reader, envConfig)
		}
	}
}

// configureIndexEncryptionKey handles setting the index encryption key
func configureIndexEncryptionKey(reader *bufio.Reader, envConfig *config.EnvConfig) {
	log.Printf("\nIndex Encryption Key\n")
	log.Printf("This key is used to encrypt codebase indexes stored on disk.\n")
	log.Printf("The key can also be set via COMANDA_INDEX_KEY environment variable.\n\n")

	if envConfig.IndexEncryptionKey != "" {
		log.Printf("Current key is set (hidden for security)\n")
		log.Printf("Enter new key (or press Enter to keep current, 'clear' to remove): ")
	} else {
		log.Printf("Enter encryption key (min 6 chars): ")
	}

	key, _ := reader.ReadString('\n')
	key = strings.TrimSpace(key)

	if key == "" {
		log.Printf("Key unchanged\n")
		return
	}

	if key == "clear" {
		envConfig.IndexEncryptionKey = ""
		log.Printf("%s Index encryption key cleared\n", greenCheckmark)
		return
	}

	if len(key) < 6 {
		log.Printf("Key must be at least 6 characters\n")
		return
	}

	envConfig.IndexEncryptionKey = key
	log.Printf("%s Index encryption key set\n", greenCheckmark)
}

var configureCmd = &cobra.Command{
	Use:   "configure",
	Short: "Manage API keys, models, and settings",
	Long: `Configure Comanda settings including API keys, models, and preferences.

Running without flags starts interactive configuration mode, which guides you
through setting up providers (Anthropic, OpenAI, Ollama, etc.) and their models.

Comanda supports two types of providers:
  API-based:  OpenAI, Anthropic, Google, X.AI, Deepseek, Moonshot (require API keys)
  Local:      Ollama, vLLM (require running servers)
  CLI Agents: Claude Code, Gemini CLI, OpenAI Codex (require installed binaries)

CLI agents are agentic coding assistants that run locally. They don't require
API key configuration - just install the CLI tool and Comanda will detect it.

Configuration is stored in ~/.comanda/config.yaml (legacy .env also supported)

Flag Groups:
  Model Management    --list, --remove, --set-default-generation-model, --default
  Security            --encrypt, --decrypt
  Database            --database
  Memory              --memory, --init-memory`,
	Example: `  # Interactive setup (recommended for first-time users)
  comanda configure

  # List all configured providers and models (includes CLI agents)
  comanda configure --list

  # Update an API key for a provider
  comanda configure --update-key anthropic

  # Set the default model for workflow generation (can use CLI agent models)
  comanda configure --set-default-generation-model claude-code-sonnet

  # Encrypt configuration file (protects API keys)
  comanda configure --encrypt

  # Remove a model
  comanda configure --remove "my-old-model"`,
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

			// Get configured models from config file
			allModels := getAllConfiguredModelNames(envConfig)

			// Add CLI agent models if available
			var cliModels []string
			if models.IsClaudeCodeAvailable() {
				cliModels = append(cliModels, getClaudeCodeModels()...)
			}
			if models.IsGeminiCLIAvailable() {
				cliModels = append(cliModels, getGeminiCLIModels()...)
			}
			if models.IsOpenAICodexAvailable() {
				cliModels = append(cliModels, getOpenAICodexModels()...)
			}

			if len(allModels) == 0 && len(cliModels) == 0 {
				log.Printf("No models are currently available.")
				log.Printf("Either configure API-based providers with 'comanda configure'")
				log.Printf("or install a CLI agent (claude-code, gemini-cli, openai-codex).")
				return
			}

			// Display configured models
			currentIdx := 0
			if len(allModels) > 0 {
				log.Printf("\nConfigured API/Local models:")
				for i, modelName := range allModels {
					currentIdx = i + 1
					marker := ""
					if envConfig.DefaultGenerationModel == modelName {
						marker = " (current default)"
					}
					log.Printf("  %d. %s%s\n", currentIdx, modelName, marker)
				}
			}

			// Display CLI agent models
			if len(cliModels) > 0 {
				log.Printf("\nCLI Agent models (auto-detected):")
				for i, modelName := range cliModels {
					idx := len(allModels) + i + 1
					marker := ""
					if envConfig.DefaultGenerationModel == modelName {
						marker = " (current default)"
					}
					log.Printf("  %d. %s%s\n", idx, modelName, marker)
				}
			}

			// Combine all models for selection
			combinedModels := append(allModels, cliModels...)

			var selectedDefaultModel string
			for {
				log.Printf("\nEnter the number of the model to set as default for generation: ")
				selectionInput, _ := reader.ReadString('\n')
				selectionNum, err := strconv.Atoi(strings.TrimSpace(selectionInput))
				if err != nil || selectionNum < 1 || selectionNum > len(combinedModels) {
					log.Printf("Invalid selection. Please enter a number from the list.")
					continue
				}
				selectedDefaultModel = combinedModels[selectionNum-1]
				break
			}

			envConfig.DefaultGenerationModel = selectedDefaultModel
			log.Printf("%s Default generation model set to '%s'\n", greenCheckmark, selectedDefaultModel)
		} else {
			reader := bufio.NewReader(os.Stdin)

			// Show main menu
			for {
				choice := showMainMenu(reader)
				switch choice {
				case "1": // Models & Providers
					configureModelsAndProviders(reader, envConfig)
				case "2": // Server
					if err := configureServerSettings(reader, envConfig); err != nil {
						log.Printf("Error configuring server: %v\n", err)
					}
				case "3": // Database
					if err := configureDatabase(reader, envConfig); err != nil {
						log.Printf("Error configuring database: %v\n", err)
					}
				case "4": // Tools
					configureToolSettings(reader, envConfig)
				case "5": // Memory
					configureMemorySettings(reader, envConfig)
				case "6": // Security
					configureSecuritySettings(reader, envConfig, configPath)
				case "7": // View Config
					listConfiguration()
				case "q", "Q", "0": // Exit
					goto saveAndExit
				default:
					log.Printf("Invalid selection. Please choose from the menu.\n")
				}
			}
		saveAndExit:
			// Skip the old provider selection flow
			goto skipProviderConfig
		}

		// Old provider selection flow (kept for backwards compatibility with flags)
		if false {
			reader := bufio.NewReader(os.Stdin)
			// Prompt for provider with CLI agents as options
			var provider string
			for {
				log.Printf("\n")
				log.Printf("Available provider types:\n")
				log.Printf("  API-based:   openai, anthropic, google, xai, deepseek, moonshot\n")
				log.Printf("  Local:       ollama, vllm\n")
				log.Printf("  CLI Agents:  claude-code, gemini-cli, openai-codex\n")
				log.Printf("\nEnter provider: ")
				provider, _ = reader.ReadString('\n')
				provider = strings.TrimSpace(provider)
				validProviders := []string{"openai", "anthropic", "ollama", "vllm", "google", "xai", "deepseek", "moonshot", "claude-code", "gemini-cli", "openai-codex"}
				isValid := false
				for _, vp := range validProviders {
					if provider == vp {
						isValid = true
						break
					}
				}
				if isValid {
					break
				}
				log.Printf("Invalid provider. Please choose from the options above.")
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

			// Special handling for CLI agents
			if provider == "claude-code" {
				if !models.IsClaudeCodeAvailable() {
					log.Printf("Error: Claude Code CLI is not installed.\n")
					log.Printf("Install it from: https://docs.anthropic.com/en/docs/claude-code\n")
					log.Printf("Or run: npm install -g @anthropic-ai/claude-code\n")
					return
				}
				// CLI agents are auto-configured, just show status and available models
				configureCLIAgent(reader, envConfig, provider, "claude-code", getClaudeCodeModels())
				return
			}
			if provider == "gemini-cli" {
				if !models.IsGeminiCLIAvailable() {
					log.Printf("Error: Gemini CLI is not installed.\n")
					log.Printf("Install it via: npm install -g @google/gemini-cli\n")
					return
				}
				configureCLIAgent(reader, envConfig, provider, "gemini-cli", getGeminiCLIModels())
				return
			}
			if provider == "openai-codex" {
				if !models.IsOpenAICodexAvailable() {
					log.Printf("Error: OpenAI Codex CLI is not installed.\n")
					log.Printf("Install it via: npm install -g @openai/codex\n")
					return
				}
				configureCLIAgent(reader, envConfig, provider, "openai-codex", getOpenAICodexModels())
				return
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

				// Fetch and categorize Anthropic models (API discovery + registry)
				primaryModels, otherModels, err := getAnthropicModelsAndCategorize(apiKey)
				if err != nil {
					log.Printf("Error fetching Anthropic models: %v\n", err)
					return
				}

				// Use paginated selection if there are other models from the API
				if len(otherModels) > 0 {
					selectedModels, err = promptForAnthropicModelSelection(primaryModels, otherModels)
				} else {
					selectedModels, err = promptForModelSelection(primaryModels)
				}
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
					// Get configured models from config file
					allModels := getAllConfiguredModelNames(envConfig)

					// Add CLI agent models if available
					var cliModels []string
					if models.IsClaudeCodeAvailable() {
						cliModels = append(cliModels, getClaudeCodeModels()...)
					}
					if models.IsGeminiCLIAvailable() {
						cliModels = append(cliModels, getGeminiCLIModels()...)
					}
					if models.IsOpenAICodexAvailable() {
						cliModels = append(cliModels, getOpenAICodexModels()...)
					}

					if len(allModels) == 0 && len(cliModels) == 0 {
						log.Printf("No models are currently available. Cannot set a default generation model.")
					} else {
						// Display configured models
						if len(allModels) > 0 {
							log.Printf("\nConfigured API/Local models:")
							for i, modelName := range allModels {
								log.Printf("  %d. %s\n", i+1, modelName)
							}
						}

						// Display CLI agent models
						if len(cliModels) > 0 {
							log.Printf("\nCLI Agent models (auto-detected):")
							for i, modelName := range cliModels {
								log.Printf("  %d. %s\n", len(allModels)+i+1, modelName)
							}
						}

						// Combine all models for selection
						combinedModels := append(allModels, cliModels...)

						var selectedDefaultModel string
						for {
							log.Printf("\nEnter the number of the model to set as default: ")
							selectionInput, _ := reader.ReadString('\n')
							selectionNum, err := strconv.Atoi(strings.TrimSpace(selectionInput))
							if err != nil || selectionNum < 1 || selectionNum > len(combinedModels) {
								log.Printf("Invalid selection. Please enter a number from the list.")
								continue
							}
							selectedDefaultModel = combinedModels[selectionNum-1]
							break
						}
						envConfig.DefaultGenerationModel = selectedDefaultModel
						log.Printf("%s Default generation model set to '%s'\n", greenCheckmark, selectedDefaultModel)
					}
				}
			}
		}

	skipProviderConfig:
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

	// List CLI Agents (these don't require config, just binary availability)
	log.Printf("CLI Agents (auto-detected):\n")
	cliAgentFound := false

	if models.IsClaudeCodeAvailable() {
		cliAgentFound = true
		log.Printf("\n%s claude-code: INSTALLED\n", greenCheckmark)
		log.Printf("  Available models:\n")
		for _, model := range getClaudeCodeModels() {
			log.Printf("    - %s\n", model)
		}
	} else {
		log.Printf("\n✗ claude-code: NOT INSTALLED\n")
		log.Printf("  Install: https://docs.anthropic.com/en/docs/claude-code\n")
	}

	if models.IsGeminiCLIAvailable() {
		cliAgentFound = true
		log.Printf("\n%s gemini-cli: INSTALLED\n", greenCheckmark)
		log.Printf("  Available models:\n")
		for _, model := range getGeminiCLIModels() {
			log.Printf("    - %s\n", model)
		}
	} else {
		log.Printf("\n✗ gemini-cli: NOT INSTALLED\n")
		log.Printf("  Install: npm install -g @google/gemini-cli\n")
	}

	if models.IsOpenAICodexAvailable() {
		cliAgentFound = true
		log.Printf("\n%s openai-codex: INSTALLED\n", greenCheckmark)
		log.Printf("  Available models:\n")
		for _, model := range getOpenAICodexModels() {
			log.Printf("    - %s\n", model)
		}
	} else {
		log.Printf("\n✗ openai-codex: NOT INSTALLED\n")
		log.Printf("  Install: npm install -g @openai/codex\n")
	}

	if !cliAgentFound {
		log.Printf("\n  No CLI agents installed. Install one to use agentic coding capabilities.\n")
	}

	// List providers
	log.Printf("\n")
	if len(cfg.Providers) == 0 {
		log.Printf("Configured Providers: none\n")
		log.Printf("\nRun 'comanda configure' to set up API-based providers.\n")
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
