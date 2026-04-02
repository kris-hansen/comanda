package cmd

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/processor"
	"github.com/kris-hansen/comanda/utils/tui"
)

// Runtime directory flag
var runtimeDir string

// Variable substitution flags
var varsFlags []string

// Stream log file for real-time monitoring
var streamLogFile string

// Live TUI dashboard mode
var processLive bool

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
		// Handle live TUI mode
		if processLive {
			runLiveProcess(args)
			return
		}

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

			// Check if workflow uses .comanda paths and prompt for creation if needed
			if err := ensureComandaDirIfNeeded(&dslConfig); err != nil {
				log.Printf("Error: %v\n", err)
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

			// Default runtimeDir to workflow file's directory if not specified
			// This ensures Claude Code runs in the project directory, not a temp folder
			effectiveRuntimeDir := runtimeDir
			if effectiveRuntimeDir == "" {
				effectiveRuntimeDir = filepath.Dir(file)
				if effectiveRuntimeDir == "." {
					// Get absolute path for current directory
					if cwd, err := os.Getwd(); err == nil {
						effectiveRuntimeDir = cwd
					}
				} else {
					// Resolve to absolute path
					if absPath, err := filepath.Abs(effectiveRuntimeDir); err == nil {
						effectiveRuntimeDir = absPath
					}
				}
				if verbose {
					log.Printf("[DEBUG] No --runtime-dir specified, using workflow directory: %s\n", effectiveRuntimeDir)
				}
			}

			// Parse CLI variables
			cliVars := parseVarsFlags(varsFlags, stdinData)

			proc := processor.NewProcessor(&dslConfig, envConfig, serverConfig, verbose, effectiveRuntimeDir, cliVars)

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

	// Add live TUI dashboard mode
	processCmd.Flags().BoolVar(&processLive, "live", false, "Show live TUI dashboard with progress, resources, and activity")
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

// runLiveProcess runs the workflow with a live TUI dashboard
func runLiveProcess(args []string) {
	if len(args) == 0 {
		log.Fatal("No workflow file specified")
	}

	workflowFile := args[0]
	workflowName := filepath.Base(workflowFile)

	// Create temp stream log file for monitoring
	tmpFile, err := os.CreateTemp("", "comanda-stream-*.log")
	if err != nil {
		log.Fatalf("Failed to create temp stream log: %v", err)
	}
	tmpLogPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpLogPath)

	// Create progress reporter
	reporter := tui.NewProgressReporter()
	defer reporter.Close()

	// Create log watcher
	watcher := tui.NewLogWatcher(tmpLogPath, reporter)

	// Try to extract model from workflow for better token estimation
	if model := extractModelFromWorkflow(workflowFile); model != "" {
		watcher.SetModel(model)
	}

	watcher.Start()
	defer watcher.Stop()

	// Set up debug output capture when debug/verbose mode is enabled
	var debugWriter *tui.DebugWriter
	var model *tui.DashboardModel
	var program *tea.Program

	if config.Debug || verbose {
		// Create debug writer to capture log output
		debugWriter = tui.NewDebugWriter(os.Stderr, 500)

		// Redirect log output to our debug writer
		log.SetOutput(debugWriter)
		defer log.SetOutput(os.Stderr) // Restore on exit

		// Create dashboard with debug panel
		model, program = tui.RunDashboardWithDebug(workflowName, reporter, debugWriter)
	} else {
		// Standard dashboard without debug panel
		model, program = tui.RunDashboard(workflowName, reporter)
	}
	_ = model // model available for future use

	// Run the processor in a goroutine
	processDone := make(chan error, 1)
	go func() {
		err := runWorkflowWithStreamLog(workflowFile, tmpLogPath, true) // true = disable spinner for TUI mode
		reporter.Complete(err)
		processDone <- err
	}()

	// Run the TUI (blocks until quit)
	if _, err := program.Run(); err != nil {
		log.Printf("Dashboard error: %v", err)
	}

	// Wait for processing to complete
	if err := <-processDone; err != nil {
		log.Printf("\nWorkflow error: %v\n", err)
		os.Exit(1)
	}
}

// runWorkflowWithStreamLog runs a single workflow with stream logging
// If disableSpinner is true, the CLI spinner is disabled (for TUI mode)
func runWorkflowWithStreamLog(workflowFile, streamLogPath string, disableSpinner ...bool) error {
	// Read YAML file
	yamlFile, err := os.ReadFile(workflowFile)
	if err != nil {
		return err
	}

	// Unmarshal YAML
	var dslConfig processor.DSLConfig
	if err := yaml.Unmarshal(yamlFile, &dslConfig); err != nil {
		return err
	}

	// Create processor
	serverConfig := &config.ServerConfig{Enabled: false}
	cliVars := parseVarsFlags(varsFlags, "")

	// Default runtimeDir to workflow file's directory if not specified
	effectiveRuntimeDir := runtimeDir
	if effectiveRuntimeDir == "" {
		effectiveRuntimeDir = filepath.Dir(workflowFile)
		if effectiveRuntimeDir == "." {
			if cwd, err := os.Getwd(); err == nil {
				effectiveRuntimeDir = cwd
			}
		} else {
			if absPath, err := filepath.Abs(effectiveRuntimeDir); err == nil {
				effectiveRuntimeDir = absPath
			}
		}
	}

	proc := processor.NewProcessor(&dslConfig, envConfig, serverConfig, verbose, effectiveRuntimeDir, cliVars)

	// Disable spinner and progress display if requested (TUI mode handles progress display)
	if len(disableSpinner) > 0 && disableSpinner[0] {
		proc.DisableSpinner()
		proc.DisableProgressDisplay()
	}

	// Set up stream logging
	if err := proc.SetStreamLog(streamLogPath); err != nil {
		return err
	}
	defer proc.CloseStreamLog()

	// Run processor
	return proc.Process()
}

// ensureComandaDirIfNeeded checks if the workflow uses .comanda paths and
// prompts the user to create the directory if it doesn't exist.
func ensureComandaDirIfNeeded(dslConfig *processor.DSLConfig) error {
	// Check if .comanda already exists
	if _, err := os.Stat(".comanda"); err == nil {
		return nil // Already exists, nothing to do
	}

	// Check if workflow uses .comanda paths
	usesComanda := false

	// Check regular steps
	for _, step := range dslConfig.Steps {
		if workflowUsesComandaPath(step.Config) {
			usesComanda = true
			break
		}
	}

	// Check agentic loop steps
	if !usesComanda {
		for _, loopConfig := range dslConfig.Loops {
			for _, path := range loopConfig.AllowedPaths {
				if strings.Contains(path, ".comanda") {
					usesComanda = true
					break
				}
			}
			if usesComanda {
				break
			}
			for _, step := range loopConfig.Steps {
				if workflowUsesComandaPath(step.Config) {
					usesComanda = true
					break
				}
			}
			if usesComanda {
				break
			}
		}
	}

	if !usesComanda {
		return nil // Workflow doesn't use .comanda paths
	}

	// Prompt user
	log.Printf("\nThis workflow uses .comanda/ paths, but the directory doesn't exist.\n")
	log.Printf("Create .comanda/ directory? [Y/n]: ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		// If we can't read (e.g., piped input), default to yes
		response = "y"
	}
	response = strings.TrimSpace(strings.ToLower(response))

	if response == "" || response == "y" || response == "yes" {
		if err := os.MkdirAll(".comanda", 0755); err != nil {
			return fmt.Errorf("failed to create .comanda directory: %w", err)
		}
		log.Printf("Created .comanda/ directory\n\n")
	} else {
		return fmt.Errorf("workflow requires .comanda/ directory - aborting")
	}

	return nil
}

// workflowUsesComandaPath checks if a step config references .comanda paths
func workflowUsesComandaPath(config processor.StepConfig) bool {
	// Check output
	if output, ok := config.Output.(string); ok {
		if strings.Contains(output, ".comanda") {
			return true
		}
	}

	// Check input
	if inputs := normalizeInput(config.Input); len(inputs) > 0 {
		for _, input := range inputs {
			if strings.Contains(input, ".comanda") {
				return true
			}
		}
	}

	return false
}

// normalizeInput converts input to string slice for checking
func normalizeInput(input interface{}) []string {
	if input == nil {
		return nil
	}
	switch v := input.(type) {
	case string:
		return []string{v}
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case []string:
		return v
	}
	return nil
}

// extractModelFromWorkflow tries to find the model from the workflow YAML
func extractModelFromWorkflow(workflowFile string) string {
	yamlFile, err := os.ReadFile(workflowFile)
	if err != nil {
		return ""
	}

	var dslConfig processor.DSLConfig
	if err := yaml.Unmarshal(yamlFile, &dslConfig); err != nil {
		return ""
	}

	// Check each step for a model
	for _, step := range dslConfig.Steps {
		if step.Config.Model != nil {
			// Model can be string or []string
			switch m := step.Config.Model.(type) {
			case string:
				if m != "" {
					return m
				}
			case []interface{}:
				if len(m) > 0 {
					if s, ok := m[0].(string); ok && s != "" {
						return s
					}
				}
			}
		}

	}

	return ""
}
