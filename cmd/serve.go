package cmd

import (
	"log"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/server"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start HTTP server for processing YAML files",
	Long:  `Start an HTTP server that processes YAML DSL configuration files via HTTP requests.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Load environment configuration
		envConfig, err := config.LoadEnvConfigWithPassword(config.GetEnvPath())
		if err != nil {
			log.Fatalf("Error loading environment configuration: %v", err)
		}

		// Start the server
		if err := server.Run(envConfig); err != nil {
			log.Fatalf("Server error: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
