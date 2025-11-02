package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/kris-hansen/comanda/utils/config"    // Required for input.Input
	"github.com/kris-hansen/comanda/utils/models"    // Required for models.DetectProvider
	"github.com/kris-hansen/comanda/utils/processor" // Required for EmbeddedLLMGuide
	"github.com/spf13/cobra"
)

// version is a placeholder for the version string, which will be set at build time.
var version string

var verbose bool
var debug bool
var generateModelName string // Flag for specifying model in generateCmd

// envConfig holds the loaded environment configuration, available to all commands
var envConfig *config.EnvConfig

// logFile holds the log file handle for proper cleanup
var logFile *os.File

var rootCmd = &cobra.Command{
	Use:   "comanda",
	Short: "A workflow processor for handling model interactions",
	Long: `comanda is a command line tool that processes workflow configurations
for model interactions and executes the specified actions.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Configure log output format (remove timestamps for cleaner debug output)
		if verbose {
			log.SetFlags(0)

			// Optional: Set up file-based logging for debugging sessions
			// This preserves logs even after the session ends
			if logFileName := os.Getenv("COMANDA_LOG_FILE"); logFileName != "" {
				if file, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666); err == nil {
					logFile = file // Store for cleanup
					log.SetOutput(file)
					log.Printf("[INFO] Logging session started at %s\n", time.Now().Format(time.RFC3339))
				} else {
					// Fallback: warn user but continue with stdout logging
					log.Printf("[WARN] Failed to open log file '%s': %v. Continuing with stdout logging.\n", logFileName, err)
				}
			}
		}

		// Ensure log file is properly closed on application exit (outside conditional)
		defer func() {
			if logFile != nil {
				log.Printf("[INFO] Logging session ended at %s\n", time.Now().Format(time.RFC3339))
				if err := logFile.Sync(); err != nil {
					log.Printf("[WARN] Failed to sync log file: %v\n", err)
				}
				logFile.Close()
			}
		}()

		// Set global verbose and debug flags
		config.Verbose = verbose
		config.Debug = debug

		// Get environment file path from COMANDA_ENV or default
		envPath := config.GetEnvPath()
		if verbose {
			log.Printf("[DEBUG] Loading environment configuration from %s\n", envPath)
		}

		// Load environment configuration
		var err error
		envConfig, err = config.LoadEnvConfigWithPassword(envPath)
		if err != nil {
			return fmt.Errorf("error loading environment configuration: %w", err)
		}

		if verbose {
			log.Println("[DEBUG] Environment configuration loaded successfully")
		}

		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var generateCmd = &cobra.Command{
	Use:   "generate <output_filename.yaml> \"<prompt_for_workflow_generation>\"",
	Short: "Generate a new Comanda workflow YAML file using an LLM",
	Long: `Generates a new Comanda workflow YAML file based on a natural language prompt.
The generated workflow is saved to the specified output filename.
You can optionally specify a model to use for generation, otherwise the default_generation_model from your configuration will be used.`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return fmt.Errorf("requires exactly two arguments: <output_filename.yaml> and \"<prompt_for_workflow_generation>\"\nExample: comanda generate my_workflow.yaml \"Create a workflow to summarize a file and save it.\"")
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		outputFilename := args[0]
		userPrompt := args[1]

		// Use the centralized configuration that was loaded in PersistentPreRunE
		// No need to load it again

		modelForGeneration := generateModelName // From flag
		if modelForGeneration == "" {
			modelForGeneration = envConfig.DefaultGenerationModel
		}
		if modelForGeneration == "" {
			return fmt.Errorf("no model specified for generation and no default_generation_model configured. Use --model or configure a default")
		}

		log.Printf("Generating workflow using model: %s\n", modelForGeneration)
		log.Printf("Output file: %s\n", outputFilename)

		// Prepare the full prompt for the LLM
		// Use the embedded guide instead of reading from file
		dslGuide := processor.EmbeddedLLMGuide

		fullPrompt := fmt.Sprintf(`SYSTEM: You are a YAML generator. You MUST output ONLY valid YAML content. No explanations, no markdown, no code blocks, no commentary - just raw YAML.

--- BEGIN COMANDA DSL SPECIFICATION ---
%s
--- END COMANDA DSL SPECIFICATION ---

User's request: %s

CRITICAL INSTRUCTION: Your entire response must be valid YAML syntax that can be directly saved to a .yaml file. Do not include ANY text before or after the YAML content. Start your response with the first line of YAML and end with the last line of YAML.`,
			dslGuide, userPrompt)

		// Get the provider
		// Note: This assumes models.DetectProvider and provider.Configure are correctly set up.
		// The provider instance needs to be configured with an API key.
		// This logic might need to be more robust, potentially calling a configure method on the provider.
		provider := models.DetectProvider(modelForGeneration)
		if provider == nil {
			return fmt.Errorf("could not detect provider for model: %s", modelForGeneration)
		}

		// Attempt to configure the provider with API key from envConfig
		providerConfig, err := envConfig.GetProviderConfig(provider.Name())
		if err != nil {
			// If provider is not in envConfig, it might be a public one like Ollama, or an error
			log.Printf("Warning: Provider %s not found in env configuration. Assuming it does not require an API key or is pre-configured.\n", provider.Name())
		} else {
			if err := provider.Configure(providerConfig.APIKey); err != nil {
				return fmt.Errorf("failed to configure provider %s: %w", provider.Name(), err)
			}
		}
		provider.SetVerbose(verbose)

		// Call the LLM
		// The SendPrompt method is part of the models.Provider interface.
		generatedResponse, err := provider.SendPrompt(modelForGeneration, fullPrompt)
		if err != nil {
			return fmt.Errorf("LLM execution failed for model '%s': %w", modelForGeneration, err)
		}

		// Extract YAML content from the response
		yamlContent := generatedResponse

		// Check if the response contains code blocks
		if strings.Contains(generatedResponse, "```yaml") {
			// Extract content between ```yaml and ```
			startMarker := "```yaml"
			endMarker := "```"

			startIdx := strings.Index(generatedResponse, startMarker)
			if startIdx != -1 {
				startIdx += len(startMarker)
				// Find the next ``` after the start marker
				remaining := generatedResponse[startIdx:]
				endIdx := strings.Index(remaining, endMarker)
				if endIdx != -1 {
					yamlContent = strings.TrimSpace(remaining[:endIdx])
				}
			}
		} else if strings.Contains(generatedResponse, "```") {
			// Try generic code block
			parts := strings.Split(generatedResponse, "```")
			if len(parts) >= 3 {
				// Take the content of the first code block
				yamlContent = strings.TrimSpace(parts[1])
				// Remove language identifier if present (e.g., "yaml" at the start)
				lines := strings.Split(yamlContent, "\n")
				if len(lines) > 0 && !strings.Contains(lines[0], ":") {
					yamlContent = strings.Join(lines[1:], "\n")
				}
			}
		}

		// Save the generated YAML to the output file
		if err := os.WriteFile(outputFilename, []byte(yamlContent), 0644); err != nil {
			return fmt.Errorf("failed to write generated workflow to '%s': %w", outputFilename, err)
		}

		log.Printf("\n%s Workflow successfully generated and saved to %s\n", "\u2705", outputFilename)
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
	generateCmd.Flags().StringVarP(&generateModelName, "model", "m", "", "Model to use for workflow generation (optional, uses default if not set)")
	rootCmd.AddCommand(generateCmd)
	rootCmd.AddCommand(versionCmd) // Add the version command
}

// getVersionFromFile attempts to read the version from the VERSION file
func getVersionFromFile() string {
	// Try to find the VERSION file in the executable's directory first
	execPath, err := os.Executable()
	if err == nil {
		execDir := filepath.Dir(execPath)
		versionPath := filepath.Join(execDir, "VERSION")
		content, err := os.ReadFile(versionPath)
		if err == nil {
			return strings.TrimSpace(string(content))
		}
	}

	// If not found, try the current working directory
	cwd, err := os.Getwd()
	if err == nil {
		versionPath := filepath.Join(cwd, "VERSION")
		content, err := os.ReadFile(versionPath)
		if err == nil {
			return strings.TrimSpace(string(content))
		}
	}

	// If still not found, try relative to the source file (for development)
	// This assumes the VERSION file is in the project root
	// and cmd/root.go is in the cmd directory
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		sourceDir := filepath.Dir(filename)
		projectRoot := filepath.Dir(sourceDir) // Go up one level from cmd to project root
		versionPath := filepath.Join(projectRoot, "VERSION")
		content, err := os.ReadFile(versionPath)
		if err == nil {
			return strings.TrimSpace(string(content))
		}
	}

	// Fallback to the build-time version or unknown
	if version != "" {
		return version
	}

	return "unknown"
}

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of Comanda",
	Long:  `All software has versions. This is Comanda's.`,
	Run: func(cmd *cobra.Command, args []string) {
		versionStr := getVersionFromFile()
		log.Printf("Comanda version: %s\n", versionStr)
	},
}

func Execute() {
	rootCmd.SilenceErrors = true
	rootCmd.SilenceUsage = true

	err := rootCmd.Execute()
	if err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "unknown command") {
			cmdPath := strings.Trim(strings.TrimPrefix(errMsg, "unknown command"), `"`+` for "comanda"`)
			// Check if the unknown command might be a filename intended for 'process'
			if _, statErr := os.Stat(cmdPath); statErr == nil || os.IsNotExist(statErr) { // if it exists or looks like a path
				log.Printf("To process a file, use the 'process' command:\n\n   comanda process %s\n\n", cmdPath)
			} else {
				fmt.Fprintln(os.Stderr, err) // Default error for other unknown commands
			}
		} else {
			fmt.Fprintln(os.Stderr, err)
		}
		os.Exit(1)
	}
}
