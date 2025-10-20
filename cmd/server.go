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
	Short: "Start and manage the HTTP server",
	Long:  `Start the HTTP server for processing YAML files, or manage server configuration`,
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
	Short: "Configure server settings",
	Long:  `Configure server settings including port, data directory, and authentication`,
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
	Short: "Show current server configuration",
	Long:  `Display the current server configuration settings`,
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
	log.Printf("\n")
	},
}

var updatePortCmd = &cobra.Command{
	Use:   "port [port]",
	Short: "Update server port",
	Long:  `Update the port number that the server listens on`,
	Args:  cobra.ExactArgs(1),
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
	Use:   "datadir [path]",
	Short: "Update data directory",
	Long:  `Update the directory path where server data is stored`,
	Args:  cobra.ExactArgs(1),
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
	Use:   "auth [on|off]",
	Short: "Toggle authentication",
	Long:  `Enable or disable server authentication`,
	Args:  cobra.ExactArgs(1),
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
	Short: "Generate new bearer token",
	Long:  `Generate a new bearer token for server authentication`,
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
	Short: "Configure CORS settings",
	Long:  `Configure Cross-Origin Resource Sharing (CORS) settings for the server`,
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

func init() {
	serverCmd.AddCommand(configureServerCmd)
	serverCmd.AddCommand(showServerCmd)
	serverCmd.AddCommand(updatePortCmd)
	serverCmd.AddCommand(updateDataDirCmd)
	serverCmd.AddCommand(toggleAuthCmd)
	serverCmd.AddCommand(newTokenCmd)
	serverCmd.AddCommand(corsCmd)
	rootCmd.AddCommand(serverCmd)
}
