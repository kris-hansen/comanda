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

// Variable substitution flags
var varsFlags []string

// Stream log file for real-time monitoring
var streamLogFile string

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
  comanda process workflow.yaml --runtime-dir ./data

  # Use variable substitution
  comanda process workflow.yaml --vars filename=/path/to/file.txt

  # Map STDIN to a variable
  cat data.txt | comanda process workflow.yaml --vars data=STDIN

  # Monitor long-running agentic loops in real-time
  comanda process agentic.yaml --stream-log /tmp/progress.log
  # In another terminal: tail -f /tmp/progress.log`,
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

			// Parse CLI variables
			cliVars := parseVarsFlags(varsFlags, stdinData)

			proc := processor.NewProcessor(&dslConfig, envConfig, serverConfig, verbose, runtimeDir, cliVars)

			// Set up stream logging if requested
			if streamLogFile != "" {
				if err := proc.SetStreamLog(streamLogFile); err != nil {
					log.Printf("Warning: Failed to set up stream log: %v\n", err)
				} else {
					log.Printf("Stream logging to: %s (use 'tail -f %s' to monitor)\n", streamLogFile, streamLogFile)
				}
				defer proc.CloseStreamLog()
			}

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

				// Handle codebase-index steps specially
				isCodebaseIndex := step.Config.Type == "codebase-index" || step.Config.CodebaseIndex != nil
				if isCodebaseIndex && step.Config.CodebaseIndex != nil {
					ci := step.Config.CodebaseIndex
					log.Printf("- Type: codebase-index\n")
					log.Printf("- Root: %s\n", ci.Root)
					if ci.Output != nil {
						if ci.Output.Path != "" {
							log.Printf("- Output Path: %s\n", ci.Output.Path)
						}
						log.Printf("- Store: %s\n", ci.Output.Store)
						if ci.Output.Encrypt {
							log.Printf("- Encrypt: true\n")
						}
					}
					if ci.Expose != nil && ci.Expose.WorkflowVariable {
						log.Printf("- Expose: workflow variable\n")
					}
					if ci.MaxOutputKB > 0 {
						log.Printf("- Max Output: %d KB\n", ci.MaxOutputKB)
					}
				} else {
					// Standard step display
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
				}

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

	// Add variable substitution flag
	processCmd.Flags().StringArrayVar(&varsFlags, "vars", []string{}, "Variable substitution in format key=value (can be repeated, use STDIN as value to map stdin)")

	// Add stream log flag for real-time monitoring of long-running operations
	processCmd.Flags().StringVar(&streamLogFile, "stream-log", "", "Write real-time progress to file (use with tail -f for monitoring)")
}

// parseVarsFlags parses the --vars flags into a map, handling STDIN as a special value
func parseVarsFlags(flags []string, stdinData string) map[string]string {
	vars := make(map[string]string)
	for _, f := range flags {
		if idx := strings.Index(f, "="); idx > 0 {
			key := f[:idx]
			value := f[idx+1:]
			if value == "STDIN" {
				vars[key] = stdinData
			} else {
				vars[key] = value
			}
		}
	}
	return vars
}
