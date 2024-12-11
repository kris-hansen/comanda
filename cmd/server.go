package cmd

import (
	"bufio"
	"fmt"
	"os"
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
			fmt.Printf("Error loading configuration: %v\n", err)
			return
		}

		if err := server.Run(envConfig); err != nil {
			fmt.Printf("Server failed to start: %v\n", err)
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
			fmt.Printf("Error loading configuration: %v\n", err)
			return
		}

		reader := bufio.NewReader(os.Stdin)
		if err := configureServer(reader, envConfig); err != nil {
			fmt.Printf("Error configuring server: %v\n", err)
			return
		}

		if err := config.SaveEnvConfig(configPath, envConfig); err != nil {
			fmt.Printf("Error saving configuration: %v\n", err)
			return
		}

		fmt.Printf("Server configuration saved successfully to %s!\n", configPath)
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
			fmt.Printf("Error loading configuration: %v\n", err)
			return
		}

		server := envConfig.GetServerConfig()
		fmt.Println("\nServer Configuration:")
		fmt.Printf("Port: %d\n", server.Port)
		fmt.Printf("Data Directory: %s\n", server.DataDir)
		fmt.Printf("Authentication Enabled: %v\n", server.Enabled)
		if server.BearerToken != "" {
			fmt.Printf("Bearer Token: %s\n", server.BearerToken)
		}
		fmt.Println()
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
			fmt.Printf("Error: Invalid port number: %v\n", err)
			return
		}

		configPath := config.GetEnvPath()
		envConfig, err := config.LoadEnvConfigWithPassword(configPath)
		if err != nil {
			fmt.Printf("Error loading configuration: %v\n", err)
			return
		}

		serverConfig := envConfig.GetServerConfig()
		serverConfig.Port = port
		envConfig.UpdateServerConfig(*serverConfig)

		if err := config.SaveEnvConfig(configPath, envConfig); err != nil {
			fmt.Printf("Error saving configuration: %v\n", err)
			return
		}

		fmt.Printf("Server port updated to %d\n", port)
	},
}

var updateDataDirCmd = &cobra.Command{
	Use:   "datadir [path]",
	Short: "Update data directory",
	Long:  `Update the directory path where server data is stored`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		dataDir := args[0]

		configPath := config.GetEnvPath()
		envConfig, err := config.LoadEnvConfigWithPassword(configPath)
		if err != nil {
			fmt.Printf("Error loading configuration: %v\n", err)
			return
		}

		serverConfig := envConfig.GetServerConfig()
		serverConfig.DataDir = dataDir
		envConfig.UpdateServerConfig(*serverConfig)

		// Create data directory if it doesn't exist
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			fmt.Printf("Error creating data directory: %v\n", err)
			return
		}

		if err := config.SaveEnvConfig(configPath, envConfig); err != nil {
			fmt.Printf("Error saving configuration: %v\n", err)
			return
		}

		fmt.Printf("Data directory updated to %s\n", dataDir)
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
			fmt.Println("Error: Please specify either 'on' or 'off'")
			return
		}

		configPath := config.GetEnvPath()
		envConfig, err := config.LoadEnvConfigWithPassword(configPath)
		if err != nil {
			fmt.Printf("Error loading configuration: %v\n", err)
			return
		}

		serverConfig := envConfig.GetServerConfig()
		serverConfig.Enabled = enable == "on"

		// Generate new bearer token if enabling auth
		if serverConfig.Enabled && serverConfig.BearerToken == "" {
			token, err := config.GenerateBearerToken()
			if err != nil {
				fmt.Printf("Error generating bearer token: %v\n", err)
				return
			}
			serverConfig.BearerToken = token
			fmt.Printf("Generated new bearer token: %s\n", token)
		}

		envConfig.UpdateServerConfig(*serverConfig)

		if err := config.SaveEnvConfig(configPath, envConfig); err != nil {
			fmt.Printf("Error saving configuration: %v\n", err)
			return
		}

		fmt.Printf("Server authentication %s\n", map[bool]string{true: "enabled", false: "disabled"}[serverConfig.Enabled])
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
			fmt.Printf("Error loading configuration: %v\n", err)
			return
		}

		serverConfig := envConfig.GetServerConfig()
		token, err := config.GenerateBearerToken()
		if err != nil {
			fmt.Printf("Error generating bearer token: %v\n", err)
			return
		}

		serverConfig.BearerToken = token
		envConfig.UpdateServerConfig(*serverConfig)

		if err := config.SaveEnvConfig(configPath, envConfig); err != nil {
			fmt.Printf("Error saving configuration: %v\n", err)
			return
		}

		fmt.Printf("Generated new bearer token: %s\n", token)
	},
}

// configureServer handles the interactive server configuration
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

func init() {
	serverCmd.AddCommand(configureServerCmd)
	serverCmd.AddCommand(showServerCmd)
	serverCmd.AddCommand(updatePortCmd)
	serverCmd.AddCommand(updateDataDirCmd)
	serverCmd.AddCommand(toggleAuthCmd)
	serverCmd.AddCommand(newTokenCmd)
	rootCmd.AddCommand(serverCmd)
}
