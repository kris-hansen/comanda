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
	Short: "A workflow automation tool for orchestrating LLM interactions",
	Long: `Comanda is a workflow automation tool that orchestrates LLM interactions
through YAML-defined pipelines.

Getting Started:
  1. comanda configure        Set up your API keys and models
  2. comanda generate         Create a workflow from natural language
  3. comanda process          Execute a workflow

Configuration is stored in ~/.comanda/config.yaml
For documentation, visit: https://github.com/kris-hansen/comanda`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Configure log output format - remove timestamps for cleaner CLI output
		// Server mode sets its own log flags with timestamps
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
	Use:   "generate <output.yaml> \"<prompt>\"",
	Short: "Create a workflow from natural language",
	Long: `Generate a new Comanda workflow YAML file from a natural language description.

The LLM will create a valid workflow based on your prompt and save it to the
specified file. Uses default_generation_model from your config unless
overridden with --model.`,
	Example: `  # Generate a summarization workflow
  comanda generate summarize.yaml "Create a workflow that summarizes text input"

  # Generate with a specific model
  comanda generate analyze.yaml "Analyze sentiment of text" -m claude-sonnet-4-20250514

  # Generate a multi-step workflow
  comanda generate pipeline.yaml "Extract key points, translate to Spanish, format as bullets"`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) != 2 {
			return fmt.Errorf("requires two arguments: <output.yaml> and \"<prompt>\"")
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

		// Get available models from the environment config
		// This ensures the LLM only uses models that are actually configured
		availableModels := envConfig.GetAllConfiguredModels()

		// Add Claude Code models if the claude binary is available
		if models.IsClaudeCodeAvailable() {
			claudeCodeModels := []string{"claude-code", "claude-code-opus", "claude-code-sonnet", "claude-code-haiku"}
			availableModels = append(availableModels, claudeCodeModels...)
		}

		// Add Gemini CLI models if the gemini binary is available
		if models.IsGeminiCLIAvailable() {
			geminiCLIModels := []string{"gemini-cli", "gemini-cli-pro", "gemini-cli-flash", "gemini-cli-flash-lite"}
			availableModels = append(availableModels, geminiCLIModels...)
		}

		// Add OpenAI Codex models if the codex binary is available
		if models.IsOpenAICodexAvailable() {
			openaiCodexModels := []string{"openai-codex", "openai-codex-o3", "openai-codex-o4-mini", "openai-codex-mini", "openai-codex-gpt-4.1", "openai-codex-gpt-4o"}
			availableModels = append(availableModels, openaiCodexModels...)
		}

		dslGuide := processor.GetEmbeddedLLMGuideWithModels(availableModels)

		// Get the provider
		provider := models.DetectProvider(modelForGeneration)
		if provider == nil {
			return fmt.Errorf("could not detect provider for model: %s", modelForGeneration)
		}

		// Attempt to configure the provider with API key from envConfig
		providerConfig, err := envConfig.GetProviderConfig(provider.Name())
		if err != nil {
			log.Printf("Warning: Provider %s not found in env configuration. Assuming it does not require an API key or is pre-configured.\n", provider.Name())
		} else {
			if err := provider.Configure(providerConfig.APIKey); err != nil {
				return fmt.Errorf("failed to configure provider %s: %w", provider.Name(), err)
			}
		}
		provider.SetVerbose(verbose)

		// Generate workflow with validation and retry
		var yamlContent string
		var invalidModels []string
		maxAttempts := 2

		for attempt := 1; attempt <= maxAttempts; attempt++ {
			// Build the prompt
			var prompt string
			if attempt == 1 {
				prompt = buildGeneratePrompt(dslGuide, userPrompt, nil)
			} else {
				// Retry with specific feedback about invalid models
				log.Printf("Retrying generation due to invalid model(s): %v\n", invalidModels)
				prompt = buildGeneratePrompt(dslGuide, userPrompt, invalidModels)
			}

			// Call the LLM
			generatedResponse, err := provider.SendPrompt(modelForGeneration, prompt)
			if err != nil {
				return fmt.Errorf("LLM execution failed for model '%s': %w", modelForGeneration, err)
			}

			// Extract YAML content from the response
			yamlContent = extractYAMLContent(generatedResponse)

			// Validate model names in the generated workflow
			invalidModels = processor.ValidateWorkflowModels(yamlContent, availableModels)
			if len(invalidModels) == 0 {
				// All models are valid, we're done
				break
			}

			if attempt == maxAttempts {
				// Final attempt still has invalid models - warn but continue
				log.Printf("Warning: Generated workflow contains invalid model(s): %v. These may fail at runtime.\n", invalidModels)
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

// buildGeneratePrompt creates the prompt for workflow generation
func buildGeneratePrompt(dslGuide, userPrompt string, invalidModels []string) string {
	basePrompt := fmt.Sprintf(`SYSTEM: You are a YAML generator. You MUST output ONLY valid YAML content. No explanations, no markdown, no code blocks, no commentary - just raw YAML.

--- BEGIN COMANDA DSL SPECIFICATION ---
%s
--- END COMANDA DSL SPECIFICATION ---

User's request: %s

CRITICAL INSTRUCTION: Your entire response must be valid YAML syntax that can be directly saved to a .yaml file. Do not include ANY text before or after the YAML content. Start your response with the first line of YAML and end with the last line of YAML.`,
		dslGuide, userPrompt)

	if len(invalidModels) > 0 {
		basePrompt += fmt.Sprintf(`

IMPORTANT CORRECTION: Your previous response used invalid model name(s): %v
You MUST only use models from the "Supported Models" list in the specification above. Please regenerate the workflow using ONLY valid model names.`, invalidModels)
	}

	return basePrompt
}

// extractYAMLContent extracts YAML from an LLM response, handling code blocks
func extractYAMLContent(response string) string {
	yamlContent := response

	// Check if the response contains code blocks
	if strings.Contains(response, "```yaml") {
		startMarker := "```yaml"
		endMarker := "```"

		startIdx := strings.Index(response, startMarker)
		if startIdx != -1 {
			startIdx += len(startMarker)
			remaining := response[startIdx:]
			endIdx := strings.Index(remaining, endMarker)
			if endIdx != -1 {
				yamlContent = strings.TrimSpace(remaining[:endIdx])
			}
		}
	} else if strings.Contains(response, "```") {
		parts := strings.Split(response, "```")
		if len(parts) >= 3 {
			yamlContent = strings.TrimSpace(parts[1])
			lines := strings.Split(yamlContent, "\n")
			if len(lines) > 0 && !strings.Contains(lines[0], ":") {
				yamlContent = strings.Join(lines[1:], "\n")
			}
		}
	}

	return yamlContent
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
	generateCmd.Flags().StringVarP(&generateModelName, "model", "m", "", "Model to use for workflow generation (optional, uses default if not set)")
	rootCmd.AddCommand(generateCmd)
	rootCmd.AddCommand(versionCmd) // Add the version command
}

// getVersion returns the version string.
// Priority: build-time ldflags > VERSION file (for development)
func getVersion() string {
	// 1. Use build-time injected version if available (set via ldflags)
	if version != "" {
		return version
	}

	// 2. For local development: try to read VERSION file from project root
	// This allows `go run .` to show the correct version without ldflags
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		sourceDir := filepath.Dir(filename)
		projectRoot := filepath.Dir(sourceDir) // Go up one level from cmd to project root
		versionPath := filepath.Join(projectRoot, "VERSION")
		content, err := os.ReadFile(versionPath)
		if err == nil {
			return "v" + strings.TrimSpace(string(content)) + "-dev"
		}
	}

	return "unknown (build with: go build -ldflags \"-X 'github.com/kris-hansen/comanda/cmd.version=vX.Y.Z'\")"
}

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Long:  `Display the current Comanda version.`,
	Run: func(cmd *cobra.Command, args []string) {
		log.Printf("Comanda version: %s\n", getVersion())
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
