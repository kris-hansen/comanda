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

var processCmd = &cobra.Command{
	Use:   "process [files...]",
	Short: "Process YAML workflow files",
	Long:  `Process one or more workflow files and execute the specified actions.`,
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// Get environment file path
		envPath := config.GetEnvPath()

		// Load environment configuration
		if verbose {
			fmt.Printf("[DEBUG] Loading environment configuration from %s\n", envPath)
		}

		envConfig, err := config.LoadEnvConfigWithPassword(envPath)
		if err != nil {
			log.Fatalf("Error loading environment configuration: %v", err)
		}

		if verbose {
			fmt.Println("[DEBUG] Environment configuration loaded successfully")
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

			// Parse YAML while preserving order
			var node yaml.Node
			err = yaml.Unmarshal(yamlFile, &node)
			if err != nil {
				log.Printf("Error parsing YAML file %s: %v\n", file, err)
				continue
			}

			// Convert YAML nodes to Steps slice preserving order
			var dslConfig processor.DSLConfig
			dslConfig.ParallelSteps = make(map[string][]processor.Step)

			// The document node should have one child which is the mapping
			if len(node.Content) > 0 && node.Content[0].Kind == yaml.MappingNode {
				mapping := node.Content[0]
				// Each pair of nodes in the mapping represents a key and its value
				for i := 0; i < len(mapping.Content); i += 2 {
					name := mapping.Content[i].Value

					// Check if this is a parallel-process block
					if name == "parallel-process" {
						if verbose {
							fmt.Printf("[DEBUG] Found parallel-process block\n")
						}

						// This is a parallel process block, parse its steps
						if mapping.Content[i+1].Kind == yaml.MappingNode {
							parallelMapping := mapping.Content[i+1]
							var parallelSteps []processor.Step

							// Parse each step in the parallel-process block
							for j := 0; j < len(parallelMapping.Content); j += 2 {
								parallelStepName := parallelMapping.Content[j].Value
								var config processor.StepConfig
								err = parallelMapping.Content[j+1].Decode(&config)
								if err != nil {
									log.Printf("Error decoding parallel step %s in %s: %v\n", parallelStepName, file, err)
									continue
								}

								parallelSteps = append(parallelSteps, processor.Step{
									Name:   parallelStepName,
									Config: config,
								})

								if verbose {
									fmt.Printf("[DEBUG] Added parallel step: %s\n", parallelStepName)
								}
							}

							// Add the parallel steps to the map with a key of "parallel-process"
							dslConfig.ParallelSteps["parallel-process"] = parallelSteps
						}
					} else {
						// This is a regular step
						var config processor.StepConfig
						err = mapping.Content[i+1].Decode(&config)
						if err != nil {
							log.Printf("Error decoding step %s in %s: %v\n", name, file, err)
							continue
						}
						dslConfig.Steps = append(dslConfig.Steps, processor.Step{
							Name:   name,
							Config: config,
						})
					}
				}
			}

			// Create processor
			if verbose {
				fmt.Printf("[DEBUG] Creating processor for %s\n", file)
			}
			// Create basic server config for CLI processing
			serverConfig := &config.ServerConfig{
				Enabled: true,
			}
			proc := processor.NewProcessor(&dslConfig, envConfig, serverConfig, verbose)

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
					fmt.Printf("  - Action: %v\n", proc.NormalizeStringSlice(step.Config.Action))
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
				fmt.Printf("- Action: %v\n", proc.NormalizeStringSlice(step.Config.Action))
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
}
