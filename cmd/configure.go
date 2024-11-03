package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/spf13/cobra"
)

type Provider struct {
	APIKey string  `yaml:"api_key"`
	Models []Model `yaml:"models"`
}

type Model struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}

type Config struct {
	Providers map[string]Provider `yaml:"providers"`
}

var (
	listFlag   bool
	serverFlag bool
)

func checkOllamaInstalled() bool {
	cmd := exec.Command("ollama", "list")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func isValidOllamaModel(modelName string) bool {
	cmd := exec.Command("ollama", "list")
	output, err := cmd.Output()
	if err != nil {
		return false
	}

	// Convert output to string and check if model exists
	outputStr := string(output)
	return strings.Contains(outputStr, modelName)
}

func configureServer(reader *bufio.Reader, envConfig *config.EnvConfig) error {
	serverConfig := envConfig.GetServerConfig()

	// Prompt for port
	fmt.Printf("Enter server port (default: %d): ", serverConfig.Port)
	portStr, _ := reader.ReadString('\n')
	portStr = strings.TrimSpace(portStr)
	if portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("invalid port number: %v", err)
		}
		serverConfig.Port = port
	}

	// Prompt for data directory
	fmt.Printf("Enter data directory path (default: %s): ", serverConfig.DataDir)
	dataDir, _ := reader.ReadString('\n')
	dataDir = strings.TrimSpace(dataDir)
	if dataDir != "" {
		serverConfig.DataDir = dataDir
	}

	// Create data directory if it doesn't exist
	if err := os.MkdirAll(serverConfig.DataDir, 0755); err != nil {
		return fmt.Errorf("error creating data directory: %v", err)
	}

	// Prompt for bearer token generation
	fmt.Print("Generate new bearer token? (y/n): ")
	genToken, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(genToken)) == "y" {
		token, err := config.GenerateBearerToken()
		if err != nil {
			return fmt.Errorf("error generating bearer token: %v", err)
		}
		serverConfig.BearerToken = token
		fmt.Printf("Generated bearer token: %s\n", token)
	}

	// Prompt for server enable/disable
	fmt.Print("Enable server authentication? (y/n): ")
	enableStr, _ := reader.ReadString('\n')
	serverConfig.Enabled = strings.TrimSpace(strings.ToLower(enableStr)) == "y"

	envConfig.UpdateServerConfig(*serverConfig)
	return nil
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

		reader := bufio.NewReader(os.Stdin)

		// Get config path from environment or default
		configPath := config.GetEnvPath()

		// Load existing configuration
		envConfig, err := config.LoadEnvConfig(configPath)
		if err != nil {
			if os.IsNotExist(err) {
				envConfig = &config.EnvConfig{
					Providers: make(map[string]*config.Provider),
				}
			} else {
				fmt.Printf("Error loading configuration: %v\n", err)
				return
			}
		}

		if serverFlag {
			if err := configureServer(reader, envConfig); err != nil {
				fmt.Printf("Error configuring server: %v\n", err)
				return
			}
		} else {
			// Prompt for provider
			var provider string
			for {
				fmt.Print("Enter provider (openai/anthropic/ollama): ")
				provider, _ = reader.ReadString('\n')
				provider = strings.TrimSpace(provider)
				if provider == "openai" || provider == "anthropic" || provider == "ollama" {
					break
				}
				fmt.Println("Invalid provider. Please enter 'openai', 'anthropic' or 'ollama'")
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

			// Prompt for model name
			var modelName string
			for {
				if provider == "ollama" {
					fmt.Print("Enter model name (must be pulled in ollama): ")
				} else {
					fmt.Print("Enter model name: ")
				}
				modelName, _ = reader.ReadString('\n')
				modelName = strings.TrimSpace(modelName)

				if provider == "ollama" {
					if !isValidOllamaModel(modelName) {
						fmt.Printf("Model '%s' is not available in ollama. Please pull it first using 'ollama pull %s'\n", modelName, modelName)
						continue
					}
				}
				break
			}

			// Add new model to provider
			modelType := "external"
			if provider == "ollama" {
				modelType = "local"
			}

			newModel := config.Model{
				Name: modelName,
				Type: modelType,
			}

			if err := envConfig.AddModelToProvider(provider, newModel); err != nil {
				fmt.Printf("Error adding model: %v\n", err)
				return
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

		fmt.Printf("Configuration saved successfully to %s!\n", configPath)
	},
}

func listConfiguration() {
	configPath := config.GetEnvPath()
	envConfig, err := config.LoadEnvConfig(configPath)
	if err != nil {
		fmt.Printf("No configuration found at %s\n", configPath)
		return
	}

	fmt.Printf("Configuration from %s:\n\n", configPath)

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
		}
	}
}

func init() {
	configureCmd.Flags().BoolVar(&listFlag, "list", false, "List all configured providers and models")
	configureCmd.Flags().BoolVar(&serverFlag, "server", false, "Configure server settings")
	rootCmd.AddCommand(configureCmd)
}
