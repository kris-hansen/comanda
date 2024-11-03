package cmd

import (
	"fmt"
	"log"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/processor"
)

var processCmd = &cobra.Command{
	Use:   "process [files...]",
	Short: "Process YAML DSL configuration files",
	Long:  `Process one or more DSL configuration files and execute the specified actions.`,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Get environment file path
		envPath := config.GetEnvPath()

		// Load environment configuration
		if verbose {
			fmt.Printf("[DEBUG] Loading environment configuration from %s\n", envPath)
		}
		envConfig, err := config.LoadEnvConfig(envPath)
		if err != nil {
			if os.IsNotExist(err) {
				log.Fatalf("Environment file not found at %s. Set COMANDA_ENV environment variable to specify a different path.", envPath)
			}
			log.Fatalf("Error loading environment configuration: %v", err)
		}
		if verbose {
			fmt.Println("[DEBUG] Environment configuration loaded successfully")
		}

		for _, file := range args {
			fmt.Printf("\nProcessing DSL file: %s\n", file)

			// Read YAML file
			if verbose {
				fmt.Printf("[DEBUG] Reading YAML file: %s\n", file)
			}
			yamlFile, err := os.ReadFile(file)
			if err != nil {
				log.Printf("Error reading YAML file %s: %v\n", file, err)
				continue
			}

			// Parse YAML into DSL config
			if verbose {
				fmt.Printf("[DEBUG] Parsing YAML content for %s\n", file)
			}
			var dslConfig processor.DSLConfig
			err = yaml.Unmarshal(yamlFile, &dslConfig)
			if err != nil {
				log.Printf("Error parsing YAML file %s: %v\n", file, err)
				continue
			}

			// Create processor
			if verbose {
				fmt.Printf("[DEBUG] Creating processor for %s\n", file)
			}
			proc := processor.NewProcessor(&dslConfig, envConfig, verbose)

			// Print configuration summary before processing
			fmt.Println("\nConfiguration:")
			for stepName, step := range dslConfig {
				fmt.Printf("\nStep: %s\n", stepName)
				inputs := proc.NormalizeStringSlice(step.Input)
				if len(inputs) > 0 && inputs[0] != "NA" {
					fmt.Printf("- Input: %v\n", inputs)
				}
				fmt.Printf("- Model: %v\n", proc.NormalizeStringSlice(step.Model))
				fmt.Printf("- Action: %v\n", proc.NormalizeStringSlice(step.Action))
				fmt.Printf("- Output: %v\n", proc.NormalizeStringSlice(step.Output))
				nextActions := proc.NormalizeStringSlice(step.NextAction)
				if len(nextActions) > 0 {
					fmt.Printf("- Next Action: %v\n", nextActions)
				}
			}
			fmt.Println()

			// Run processor
			if err := proc.Process(); err != nil {
				log.Printf("Error processing DSL file %s: %v\n", file, err)
				continue
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(processCmd)
}
