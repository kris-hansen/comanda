package cmd

import (
	"bufio"
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
	Use:   "process <workflow.yaml> [additional workflows...]",
	Short: "Execute one or more workflow files",
	Long: `Execute one or more Comanda workflow files. Each workflow defines a pipeline
of steps that process input through configured LLM models.

Input can be provided via:
  - File paths specified in the workflow
  - STDIN (pipe data into a workflow)
  - Previous step outputs (chained workflows)`,
	Example: `  # Run a single workflow
  comanda process summarize.yaml

  # Run multiple workflows in sequence
  comanda process extract.yaml transform.yaml

  # Pipe input to a workflow
  echo "Analyze this text" | comanda process analyze.yaml

  # Use a runtime directory for file operations
  comanda process workflow.yaml --runtime-dir ./data`,
	Args: cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// The environment configuration is already loaded in rootCmd's PersistentPreRunE
		// and available in the package-level envConfig variable

		if verbose {
			log.Println("[DEBUG] Using centralized environment configuration")
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
			log.Printf("\nProcessing workflow file: %s\n", file)

			// Read YAML file
			if verbose {
				log.Printf("[DEBUG] Reading YAML file: %s\n", file)
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
				log.Printf("[DEBUG] Creating processor for %s\n", file)
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
			log.Printf("\nConfiguration:\n")

			// Print parallel steps if any
			for groupName, parallelSteps := range dslConfig.ParallelSteps {
				log.Printf("\nParallel Process Group: %s\n", groupName)
				for _, step := range parallelSteps {
					log.Printf("\n  Parallel Step: %s\n", step.Name)
					inputs := proc.NormalizeStringSlice(step.Config.Input)
					if len(inputs) > 0 && inputs[0] != "NA" {
						log.Printf("  - Input: %v\n", inputs)
					}
					log.Printf("  - Model: %v\n", proc.NormalizeStringSlice(step.Config.Model))

					// Display instructions for openai-responses type steps, otherwise display action
					if step.Config.Type == "openai-responses" && step.Config.Instructions != "" {
						log.Printf("  - Instructions: %v\n", step.Config.Instructions)
					} else {
						log.Printf("  - Action: %v\n", proc.NormalizeStringSlice(step.Config.Action))
					}

					log.Printf("  - Output: %v\n", proc.NormalizeStringSlice(step.Config.Output))

					// Display memory information if enabled
					if step.Config.Memory {
						memoryPath := proc.GetMemoryFilePath()
						if memoryPath != "" {
							log.Printf("  - Memory: [Using %s]\n", memoryPath)
						} else {
							log.Printf("  - Memory: [ENABLED but no memory file configured]\n")
						}
					}

					nextActions := proc.NormalizeStringSlice(step.Config.NextAction)
					if len(nextActions) > 0 {
						log.Printf("  - Next Action: %v\n", nextActions)
					}
				}
			}

			// Print sequential steps
			for _, step := range dslConfig.Steps {
				log.Printf("\nStep: %s\n", step.Name)
				inputs := proc.NormalizeStringSlice(step.Config.Input)
				if len(inputs) > 0 && inputs[0] != "NA" {
					log.Printf("- Input: %v\n", inputs)
				}
				log.Printf("- Model: %v\n", proc.NormalizeStringSlice(step.Config.Model))

				// Display instructions for openai-responses type steps, otherwise display action
				if step.Config.Type == "openai-responses" && step.Config.Instructions != "" {
					log.Printf("- Instructions: %v\n", step.Config.Instructions)
				} else {
					log.Printf("- Action: %v\n", proc.NormalizeStringSlice(step.Config.Action))
				}

				log.Printf("- Output: %v\n", proc.NormalizeStringSlice(step.Config.Output))

				// Display memory information if enabled
				if step.Config.Memory {
					memoryPath := proc.GetMemoryFilePath()
					if memoryPath != "" {
						log.Printf("- Memory: [Using %s]\n", memoryPath)
					} else {
						log.Printf("- Memory: [ENABLED but no memory file configured]\n")
					}
				}

				nextActions := proc.NormalizeStringSlice(step.Config.NextAction)
				if len(nextActions) > 0 {
					log.Printf("- Next Action: %v\n", nextActions)
				}
			}
			log.Printf("\n")

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
