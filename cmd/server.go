package cmd

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/server"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Run the HTTP server for remote workflow execution",
	Long: `Start the Comanda HTTP server or manage its configuration.

Running 'comanda server' without a subcommand starts the server on the
configured port (default: 8080).

Endpoints:
  POST /process                Execute a workflow (multipart form or JSON)
  GET  /health                 Health check endpoint

OpenAI-Compatible Endpoints (when enabled):
  GET  /v1/models              List available workflows as models
  POST /v1/chat/completions    Execute workflow via chat completion API`,
	Example: `  # Start the server
  comanda server

  # Start with verbose logging
  comanda server --verbose

  # View current configuration
  comanda server show

  # Enable OpenAI-compatible API mode
  comanda server openai-compat on`,
	Run: func(cmd *cobra.Command, args []string) {
		// Default behavior (no subcommand) is to start the server
		configPath := config.GetEnvPath()
		envConfig, err := config.LoadEnvConfigWithPassword(configPath)
		if err != nil {
			log.Printf("Error loading configuration: %v\n", err)
			return
		}

		if err := server.Run(envConfig); err != nil {
			log.Printf("Server failed to start: %v\n", err)
			return
		}
	},
}

var configureServerCmd = &cobra.Command{
	Use:   "configure",
	Short: "Configure server settings interactively",
	Long: `Interactively configure server settings including:
  - Port number
  - Data directory (where workflows are stored)
  - Bearer token authentication
  - CORS settings`,
	Run: func(cmd *cobra.Command, args []string) {
		configPath := config.GetEnvPath()
		envConfig, err := config.LoadEnvConfigWithPassword(configPath)
		if err != nil {
			log.Printf("Error loading configuration: %v\n", err)
			return
		}

		reader := bufio.NewReader(os.Stdin)
		if err := configureServer(reader, envConfig); err != nil {
			log.Printf("Error configuring server: %v\n", err)
			return
		}

		if err := config.SaveEnvConfig(configPath, envConfig); err != nil {
			log.Printf("Error saving configuration: %v\n", err)
			return
		}

		log.Printf("Server configuration saved successfully to %s!\n", configPath)
	},
}

var showServerCmd = &cobra.Command{
	Use:   "show",
	Short: "Display current server configuration",
	Long: `Display all server configuration settings including:
  - Port and data directory
  - Authentication status and bearer token
  - CORS configuration
  - OpenAI compatibility mode status`,
	Run: func(cmd *cobra.Command, args []string) {
		configPath := config.GetEnvPath()
		envConfig, err := config.LoadEnvConfigWithPassword(configPath)
		if err != nil {
			log.Printf("Error loading configuration: %v\n", err)
			return
		}

		server := envConfig.GetServerConfig()
		log.Printf("\nServer Configuration:\n")
		log.Printf("Port: %d\n", server.Port)
		log.Printf("Data Directory: %s\n", server.DataDir)
		log.Printf("Authentication Enabled: %v\n", server.Enabled)
		if server.BearerToken != "" {
			log.Printf("Bearer Token: %s\n", server.BearerToken)
		}

		// Display CORS configuration
		log.Printf("\nCORS Configuration:\n")
		log.Printf("Enabled: %v\n", server.CORS.Enabled)
		if server.CORS.Enabled {
			log.Printf("Allowed Origins: %s\n", strings.Join(server.CORS.AllowedOrigins, ", "))
			log.Printf("Allowed Methods: %s\n", strings.Join(server.CORS.AllowedMethods, ", "))
			log.Printf("Allowed Headers: %s\n", strings.Join(server.CORS.AllowedHeaders, ", "))
			log.Printf("Max Age: %d seconds\n", server.CORS.MaxAge)
		}

		// Display OpenAI compatibility configuration
		log.Printf("\nOpenAI Compatibility:\n")
		log.Printf("Enabled: %v\n", server.OpenAICompat.Enabled)
		if server.OpenAICompat.Enabled {
			prefix := server.OpenAICompat.Prefix
			if prefix == "" {
				prefix = "/v1"
			}
			log.Printf("API Prefix: %s\n", prefix)
			log.Printf("Endpoints: %s/models, %s/chat/completions\n", prefix, prefix)
		}
		log.Printf("\n")
	},
}

var updatePortCmd = &cobra.Command{
	Use:     "port <port-number>",
	Short:   "Set the server port",
	Long:    `Set the port number that the server listens on (default: 8080).`,
	Example: `  comanda server port 3000`,
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		port, err := strconv.Atoi(args[0])
		if err != nil {
			log.Printf("Error: Invalid port number: %v\n", err)
			return
		}

		configPath := config.GetEnvPath()
		envConfig, err := config.LoadEnvConfigWithPassword(configPath)
		if err != nil {
			log.Printf("Error loading configuration: %v\n", err)
			return
		}

		serverConfig := envConfig.GetServerConfig()
		serverConfig.Port = port
		envConfig.UpdateServerConfig(*serverConfig)

		if err := config.SaveEnvConfig(configPath, envConfig); err != nil {
			log.Printf("Error saving configuration: %v\n", err)
			return
		}

		log.Printf("Server port updated to %d\n", port)
	},
}

var updateDataDirCmd = &cobra.Command{
	Use:   "datadir <path>",
	Short: "Set the workflow data directory",
	Long: `Set the directory where workflow YAML files are stored.

The server looks for workflow files in this directory when processing requests.
The directory will be created if it doesn't exist.`,
	Example: `  comanda server datadir /var/comanda/workflows
  comanda server datadir ./my-workflows`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dataDir := args[0]

		// Clean and resolve the path
		absPath, err := filepath.Abs(dataDir)
		if err != nil {
			log.Printf("Error: Invalid directory path: %v\n", err)
			return
		}

		// Check if path is valid for the OS
		if !filepath.IsAbs(absPath) {
			log.Printf("Error: Path must be absolute: %s\n", absPath)
			return
		}

		// Check if parent directory exists and is accessible
		parentDir := filepath.Dir(absPath)
		if _, err := os.Stat(parentDir); err != nil {
			if os.IsNotExist(err) {
				log.Printf("Error: Parent directory does not exist: %s\n", parentDir)
			} else {
				log.Printf("Error: Cannot access parent directory: %v\n", err)
			}
			return
		}

		// Try to create the directory to verify write permissions
		if err := os.MkdirAll(absPath, 0755); err != nil {
			log.Printf("Error: Cannot create directory (check permissions): %v\n", err)
			return
		}

		// Verify the directory is writable by creating a test file
		testFile := filepath.Join(absPath, ".write_test")
		if err := os.WriteFile(testFile, []byte(""), 0644); err != nil {
			log.Printf("Error: Directory is not writable: %v\n", err)
			return
		}
		os.Remove(testFile) // Clean up test file

		configPath := config.GetEnvPath()
		envConfig, err := config.LoadEnvConfigWithPassword(configPath)
		if err != nil {
			log.Printf("Error loading configuration: %v\n", err)
			return
		}

		serverConfig := envConfig.GetServerConfig()
		serverConfig.DataDir = absPath
		envConfig.UpdateServerConfig(*serverConfig)

		if err := config.SaveEnvConfig(configPath, envConfig); err != nil {
			log.Printf("Error saving configuration: %v\n", err)
			return
		}

		log.Printf("Data directory updated to %s\n", absPath)
	},
}

var toggleAuthCmd = &cobra.Command{
	Use:   "auth <on|off>",
	Short: "Enable or disable bearer token authentication",
	Long: `Enable or disable bearer token authentication for the HTTP server.

When enabled, all requests must include the header:
  Authorization: Bearer <token>

A token is automatically generated when auth is first enabled.
Use 'comanda server newtoken' to generate a new token.
Use 'comanda server show' to view the current token.`,
	Example: `  comanda server auth on
  comanda server auth off`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		enable := strings.ToLower(args[0])
		if enable != "on" && enable != "off" {
			log.Printf("Error: Please specify either 'on' or 'off'\n")
			return
		}

		configPath := config.GetEnvPath()
		envConfig, err := config.LoadEnvConfigWithPassword(configPath)
		if err != nil {
			log.Printf("Error loading configuration: %v\n", err)
			return
		}

		serverConfig := envConfig.GetServerConfig()
		serverConfig.Enabled = enable == "on"

		// Generate new bearer token if enabling auth
		if serverConfig.Enabled && serverConfig.BearerToken == "" {
			token, err := config.GenerateBearerToken()
			if err != nil {
				log.Printf("Error generating bearer token: %v\n", err)
				return
			}
			serverConfig.BearerToken = token
			log.Printf("Generated new bearer token: %s\n", token)
		}

		envConfig.UpdateServerConfig(*serverConfig)

		if err := config.SaveEnvConfig(configPath, envConfig); err != nil {
			log.Printf("Error saving configuration: %v\n", err)
			return
		}

		log.Printf("Server authentication %s\n", map[bool]string{true: "enabled", false: "disabled"}[serverConfig.Enabled])
	},
}

var newTokenCmd = &cobra.Command{
	Use:   "newtoken",
	Short: "Generate a new bearer token",
	Long: `Generate a new bearer token for server authentication.

This replaces any existing token. The new token is displayed and saved
to the configuration file.`,
	Run: func(cmd *cobra.Command, args []string) {
		configPath := config.GetEnvPath()
		envConfig, err := config.LoadEnvConfigWithPassword(configPath)
		if err != nil {
			log.Printf("Error loading configuration: %v\n", err)
			return
		}

		serverConfig := envConfig.GetServerConfig()
		token, err := config.GenerateBearerToken()
		if err != nil {
			log.Printf("Error generating bearer token: %v\n", err)
			return
		}

		serverConfig.BearerToken = token
		envConfig.UpdateServerConfig(*serverConfig)

		if err := config.SaveEnvConfig(configPath, envConfig); err != nil {
			log.Printf("Error saving configuration: %v\n", err)
			return
		}

		log.Printf("Generated new bearer token: %s\n", token)
	},
}

// configureServer handles the interactive server configuration
func configureServer(reader *bufio.Reader, envConfig *config.EnvConfig) error {
	serverConfig := envConfig.GetServerConfig()

	// Prompt for port
	log.Printf("Enter server port (default: %d): ", serverConfig.Port)
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
	log.Printf("Enter data directory path (default: %s): ", serverConfig.DataDir)
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
	log.Printf("Generate new bearer token? (y/n): ")
	genToken, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(genToken)) == "y" {
		token, err := config.GenerateBearerToken()
		if err != nil {
			return fmt.Errorf("error generating bearer token: %v", err)
		}
		serverConfig.BearerToken = token
		log.Printf("Generated bearer token: %s\n", token)
	}

	// Prompt for server enable/disable
	log.Printf("Enable server authentication? (y/n): ")
	enableStr, _ := reader.ReadString('\n')
	serverConfig.Enabled = strings.TrimSpace(strings.ToLower(enableStr)) == "y"

	// Configure CORS settings
	if err := configureCORS(reader, envConfig); err != nil {
		return fmt.Errorf("error configuring CORS: %v", err)
	}

	envConfig.UpdateServerConfig(*serverConfig)
	return nil
}

var corsCmd = &cobra.Command{
	Use:   "cors",
	Short: "Configure CORS settings interactively",
	Long: `Configure Cross-Origin Resource Sharing (CORS) settings for the server.

CORS controls which web domains can make requests to the server from browsers.
This is required when integrating Comanda with web applications.

The interactive prompts will ask for:
  - Enable/disable CORS
  - Allowed origins (domains that can access the API)
  - Allowed HTTP methods
  - Allowed headers
  - Max age (how long browsers cache CORS preflight responses)`,
	Run: func(cmd *cobra.Command, args []string) {
		configPath := config.GetEnvPath()
		envConfig, err := config.LoadEnvConfigWithPassword(configPath)
		if err != nil {
			log.Printf("Error loading configuration: %v\n", err)
			return
		}

		reader := bufio.NewReader(os.Stdin)
		if err := configureCORS(reader, envConfig); err != nil {
			log.Printf("Error configuring CORS: %v\n", err)
			return
		}

		if err := config.SaveEnvConfig(configPath, envConfig); err != nil {
			log.Printf("Error saving configuration: %v\n", err)
			return
		}

		log.Printf("CORS configuration saved successfully!\n")
	},
}

// configureCORS handles the interactive CORS configuration
func configureCORS(reader *bufio.Reader, envConfig *config.EnvConfig) error {
	serverConfig := envConfig.GetServerConfig()

	// Prompt for CORS enable/disable
	log.Printf("Enable CORS? (y/n): ")
	enableStr, _ := reader.ReadString('\n')
	serverConfig.CORS.Enabled = strings.TrimSpace(strings.ToLower(enableStr)) == "y"

	if serverConfig.CORS.Enabled {
		// Prompt for allowed origins
		log.Printf("Enter allowed origins (comma-separated, * for all, default: *): ")
		originsStr, _ := reader.ReadString('\n')
		originsStr = strings.TrimSpace(originsStr)
		if originsStr != "" && originsStr != "*" {
			serverConfig.CORS.AllowedOrigins = strings.Split(originsStr, ",")
			for i := range serverConfig.CORS.AllowedOrigins {
				serverConfig.CORS.AllowedOrigins[i] = strings.TrimSpace(serverConfig.CORS.AllowedOrigins[i])
			}
		} else {
			serverConfig.CORS.AllowedOrigins = []string{"*"}
		}

		// Prompt for allowed methods
		log.Printf("Enter allowed methods (comma-separated, default: GET,POST,PUT,DELETE,OPTIONS): ")
		methodsStr, _ := reader.ReadString('\n')
		methodsStr = strings.TrimSpace(methodsStr)
		if methodsStr != "" {
			serverConfig.CORS.AllowedMethods = strings.Split(methodsStr, ",")
			for i := range serverConfig.CORS.AllowedMethods {
				serverConfig.CORS.AllowedMethods[i] = strings.TrimSpace(serverConfig.CORS.AllowedMethods[i])
			}
		} else {
			serverConfig.CORS.AllowedMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
		}

		// Prompt for allowed headers
		log.Printf("Enter allowed headers (comma-separated, default: Authorization,Content-Type): ")
		headersStr, _ := reader.ReadString('\n')
		headersStr = strings.TrimSpace(headersStr)
		if headersStr != "" {
			serverConfig.CORS.AllowedHeaders = strings.Split(headersStr, ",")
			for i := range serverConfig.CORS.AllowedHeaders {
				serverConfig.CORS.AllowedHeaders[i] = strings.TrimSpace(serverConfig.CORS.AllowedHeaders[i])
			}
		} else {
			serverConfig.CORS.AllowedHeaders = []string{
				"Authorization",
				"Content-Type",
				"Cache-Control",
				"Last-Event-ID",
				"X-Accel-Buffering",
				"X-Requested-With",
				"Accept",
			}
		}

		// Prompt for max age
		log.Printf("Enter max age in seconds (default: 3600): ")
		maxAgeStr, _ := reader.ReadString('\n')
		maxAgeStr = strings.TrimSpace(maxAgeStr)
		if maxAgeStr != "" {
			maxAge, err := strconv.Atoi(maxAgeStr)
			if err != nil {
				return fmt.Errorf("invalid max age: %v", err)
			}
			serverConfig.CORS.MaxAge = maxAge
		} else {
			serverConfig.CORS.MaxAge = 3600
		}
	}

	envConfig.UpdateServerConfig(*serverConfig)
	return nil
}

var openaiCompatCmd = &cobra.Command{
	Use:   "openai-compat <on|off>",
	Short: "Enable or disable OpenAI-compatible API endpoints",
	Long: `Enable or disable OpenAI-compatible API endpoints (/v1/models, /v1/chat/completions).

When enabled, tools like Cline can connect to Comanda using the OpenAI API format.
Workflows are exposed as models, and chat completions execute the specified workflow.

Example configuration for Cline:
  API Provider: OpenAI Compatible
  Base URL: http://localhost:8080/v1
  API Key: <your-comanda-bearer-token>
  Model: <workflow-name>`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		enable := strings.ToLower(args[0])
		if enable != "on" && enable != "off" {
			log.Printf("Error: Please specify either 'on' or 'off'\n")
			return
		}

		configPath := config.GetEnvPath()
		envConfig, err := config.LoadEnvConfigWithPassword(configPath)
		if err != nil {
			log.Printf("Error loading configuration: %v\n", err)
			return
		}

		serverConfig := envConfig.GetServerConfig()
		serverConfig.OpenAICompat.Enabled = enable == "on"

		// Set default prefix if not set
		if serverConfig.OpenAICompat.Prefix == "" {
			serverConfig.OpenAICompat.Prefix = "/v1"
		}

		envConfig.UpdateServerConfig(*serverConfig)

		if err := config.SaveEnvConfig(configPath, envConfig); err != nil {
			log.Printf("Error saving configuration: %v\n", err)
			return
		}

		if serverConfig.OpenAICompat.Enabled {
			log.Printf("OpenAI compatibility mode enabled at %s\n", serverConfig.OpenAICompat.Prefix)
		} else {
			log.Printf("OpenAI compatibility mode disabled\n")
		}
	},
}

func init() {
	serverCmd.AddCommand(configureServerCmd)
	serverCmd.AddCommand(showServerCmd)
	serverCmd.AddCommand(updatePortCmd)
	serverCmd.AddCommand(updateDataDirCmd)
	serverCmd.AddCommand(toggleAuthCmd)
	serverCmd.AddCommand(newTokenCmd)
	serverCmd.AddCommand(corsCmd)
	serverCmd.AddCommand(openaiCompatCmd)
	rootCmd.AddCommand(serverCmd)
}
