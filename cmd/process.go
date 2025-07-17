package cmd

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/processor"
)

// Runtime directory flag
var runtimeDir string

var processCmd = &cobra.Command{
	Use:   "process [files...]",
	Short: "Process YAML workflow files",
	Long:  `Process one or more workflow files and execute the specified actions.`,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// The environment configuration is already loaded in rootCmd's PersistentPreRunE
		// and available in the package-level envConfig variable

		if verbose {
			fmt.Println("[DEBUG] Using centralized environment configuration")
		}

		// Check if there's data on STDIN
		stat, _ := os.Stdin.Stat()
		var stdinData string
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			// Read from STDIN
			reader := bufio.NewReader(os.Stdin)
			var builder strings.Builder
			for {
				input, err := reader.ReadString('\n')
				if err != nil && err != io.EOF {
					log.Fatalf("Error reading from STDIN: %v", err)
				}
				builder.WriteString(input)
				if err == io.EOF {
					break
				}
			}
			stdinData = builder.String()
		}

		for _, file := range args {
			fmt.Printf("\nProcessing workflow file: %s\n", file)

			// Read YAML file
			if verbose {
				fmt.Printf("[DEBUG] Reading YAML file: %s\n", file)
			}
			yamlFile, err := os.ReadFile(file)
			if err != nil {
				log.Printf("Error reading YAML file %s: %v\n", file, err)
				continue
			}

			// Unmarshal YAML into the DSLConfig struct, which will use the custom unmarshaler
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
			// Create basic server config for CLI processing
			serverConfig := &config.ServerConfig{
				Enabled: false, // Disable server mode for CLI processing
			}
			proc := processor.NewProcessor(&dslConfig, envConfig, serverConfig, verbose, runtimeDir)

			// If we have STDIN data, set it as initial output
			if stdinData != "" {
				proc.SetLastOutput(stdinData)
			}

			// Print configuration summary before processing
			fmt.Println("\nConfiguration:")

			// Print parallel steps if any
			for groupName, parallelSteps := range dslConfig.ParallelSteps {
				fmt.Printf("\nParallel Process Group: %s\n", groupName)
				for _, step := range parallelSteps {
					fmt.Printf("\n  Parallel Step: %s\n", step.Name)
					inputs := proc.NormalizeStringSlice(step.Config.Input)
					if len(inputs) > 0 && inputs[0] != "NA" {
						fmt.Printf("  - Input: %v\n", inputs)
					}
					fmt.Printf("  - Model: %v\n", proc.NormalizeStringSlice(step.Config.Model))

					// Display instructions for openai-responses type steps, otherwise display action
					if step.Config.Type == "openai-responses" && step.Config.Instructions != "" {
						fmt.Printf("  - Instructions: %v\n", step.Config.Instructions)
					} else {
						fmt.Printf("  - Action: %v\n", proc.NormalizeStringSlice(step.Config.Action))
					}

					fmt.Printf("  - Output: %v\n", proc.NormalizeStringSlice(step.Config.Output))
					nextActions := proc.NormalizeStringSlice(step.Config.NextAction)
					if len(nextActions) > 0 {
						fmt.Printf("  - Next Action: %v\n", nextActions)
					}
				}
			}

			// Print sequential steps
			for _, step := range dslConfig.Steps {
				fmt.Printf("\nStep: %s\n", step.Name)
				inputs := proc.NormalizeStringSlice(step.Config.Input)
				if len(inputs) > 0 && inputs[0] != "NA" {
					fmt.Printf("- Input: %v\n", inputs)
				}
				fmt.Printf("- Model: %v\n", proc.NormalizeStringSlice(step.Config.Model))

				// Display instructions for openai-responses type steps, otherwise display action
				if step.Config.Type == "openai-responses" && step.Config.Instructions != "" {
					fmt.Printf("- Instructions: %v\n", step.Config.Instructions)
				} else {
					fmt.Printf("- Action: %v\n", proc.NormalizeStringSlice(step.Config.Action))
				}

				fmt.Printf("- Output: %v\n", proc.NormalizeStringSlice(step.Config.Output))
				nextActions := proc.NormalizeStringSlice(step.Config.NextAction)
				if len(nextActions) > 0 {
					fmt.Printf("- Next Action: %v\n", nextActions)
				}
			}
			fmt.Println()

			// Run processor
			if err := proc.Process(); err != nil {
				log.Printf("Error processing workflow file %s: %v\n", file, err)
				continue
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(processCmd)

	// Add runtime directory flag
	processCmd.Flags().StringVar(&runtimeDir, "runtime-dir", "", "Runtime directory for file operations (relative to data directory)")
}
