package models

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/fileutil"
	"github.com/kris-hansen/comanda/utils/retry"
)

// OpenAICodexProvider handles OpenAI Codex CLI for agentic programming tasks
type OpenAICodexProvider struct {
	verbose    bool
	binaryPath string
	mu         sync.Mutex
}

// NewOpenAICodexProvider creates a new OpenAI Codex provider instance
func NewOpenAICodexProvider() *OpenAICodexProvider {
	return &OpenAICodexProvider{}
}

// Name returns the provider name
func (o *OpenAICodexProvider) Name() string {
	return "openai-codex"
}

// debugf prints debug information if verbose mode is enabled (thread-safe)
func (o *OpenAICodexProvider) debugf(format string, args ...interface{}) {
	if o.verbose {
		o.mu.Lock()
		defer o.mu.Unlock()
		log.Printf("[DEBUG][OpenAICodex] "+format+"\n", args...)
	}
}

// SupportsModel checks if this provider supports the given model name
func (o *OpenAICodexProvider) SupportsModel(modelName string) bool {
	o.debugf("Checking if model is supported: %s", modelName)

	// Support openai-codex as the primary model name
	// Also support variations like openai-codex-mini, openai-codex-o3
	modelLower := strings.ToLower(modelName)
	supported := modelLower == "openai-codex" ||
		strings.HasPrefix(modelLower, "openai-codex-")

	if supported {
		o.debugf("Model %s is supported by OpenAI Codex provider", modelName)
	}
	return supported
}

// Configure sets up the provider
// OpenAI Codex uses its own authentication via OPENAI_API_KEY env var
func (o *OpenAICodexProvider) Configure(apiKey string) error {
	o.debugf("Configuring OpenAI Codex provider")

	// Find the codex binary
	binaryPath, err := o.findCodexBinary()
	if err != nil {
		return fmt.Errorf("codex binary not found: %w", err)
	}
	o.binaryPath = binaryPath
	o.debugf("Found codex binary at: %s", binaryPath)

	return nil
}

// findCodexBinary locates the codex CLI binary
func (o *OpenAICodexProvider) findCodexBinary() (string, error) {
	// First check if it's in PATH
	path, err := exec.LookPath("codex")
	if err == nil {
		return path, nil
	}

	// Check common installation locations
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not find home directory: %w", err)
	}

	commonPaths := []string{
		// npm global install locations
		filepath.Join(homeDir, ".npm-global", "bin", "codex"),
		filepath.Join(homeDir, "node_modules", ".bin", "codex"),
		filepath.Join(homeDir, ".local", "bin", "codex"),
		"/usr/local/bin/codex",
		"/usr/bin/codex",
		// Homebrew locations
		"/opt/homebrew/bin/codex",
	}

	for _, p := range commonPaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("codex binary not found in PATH or common locations")
}

// SendPrompt sends a prompt to OpenAI Codex and returns the response
func (o *OpenAICodexProvider) SendPrompt(modelName string, prompt string) (string, error) {
	o.debugf("Preparing to send prompt via OpenAI Codex")
	o.debugf("Prompt length: %d characters", len(prompt))

	if o.binaryPath == "" {
		if err := o.Configure("LOCAL"); err != nil {
			return "", err
		}
	}

	// Build the command arguments
	args := o.buildArgs(modelName, prompt, "")

	o.debugf("Executing: %s %v", o.binaryPath, args)

	// Use retry mechanism for execution
	result, err := retry.WithRetry(
		func() (interface{}, error) {
			return o.executeCommand(args, "")
		},
		func(err error) bool {
			// Retry on timeout or transient errors
			return strings.Contains(err.Error(), "timeout") ||
				strings.Contains(err.Error(), "connection")
		},
		retry.DefaultRetryConfig,
	)

	if err != nil {
		return "", err
	}

	response := result.(string)
	o.debugf("Command completed, response length: %d characters", len(response))
	return response, nil
}

// SendPromptWithFile sends a prompt along with file context to OpenAI Codex
func (o *OpenAICodexProvider) SendPromptWithFile(modelName string, prompt string, file FileInput) (string, error) {
	o.debugf("Preparing to send prompt with file to OpenAI Codex")
	o.debugf("File path: %s", file.Path)

	if o.binaryPath == "" {
		if err := o.Configure("LOCAL"); err != nil {
			return "", err
		}
	}

	// Read file content
	fileData, err := fileutil.SafeReadFile(file.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// For text files, include content in prompt
	// For binary/image files, we'd need different handling
	fileContent := string(fileData)
	combinedPrompt := fmt.Sprintf("File: %s\n\n```\n%s\n```\n\nTask: %s", file.Path, fileContent, prompt)

	// Build args - use the file's directory as working directory
	args := o.buildArgs(modelName, combinedPrompt, filepath.Dir(file.Path))

	o.debugf("Executing with file context: %s", file.Path)

	result, err := retry.WithRetry(
		func() (interface{}, error) {
			return o.executeCommand(args, filepath.Dir(file.Path))
		},
		func(err error) bool {
			return strings.Contains(err.Error(), "timeout") ||
				strings.Contains(err.Error(), "connection")
		},
		retry.DefaultRetryConfig,
	)

	if err != nil {
		return "", err
	}

	response := result.(string)
	o.debugf("Command completed, response length: %d characters", len(response))
	return response, nil
}

// buildArgs constructs the command line arguments for codex
func (o *OpenAICodexProvider) buildArgs(modelName string, prompt string, workDir string) []string {
	// Use "exec" subcommand for non-interactive mode
	// Add --skip-git-repo-check to allow running outside git repos
	args := []string{"exec", "--skip-git-repo-check"}

	// Extract model variant if specified (e.g., openai-codex-mini -> o4-mini)
	if strings.HasPrefix(strings.ToLower(modelName), "openai-codex-") {
		variant := strings.TrimPrefix(strings.ToLower(modelName), "openai-codex-")
		// Map to actual OpenAI model names
		switch variant {
		case "o3":
			args = append(args, "-m", "o3")
		case "o4-mini", "mini":
			args = append(args, "-m", "o4-mini")
		case "gpt-4.1", "gpt4.1":
			args = append(args, "-m", "gpt-4.1")
		case "gpt-4o", "gpt4o":
			args = append(args, "-m", "gpt-4o")
		default:
			// Use the variant as-is if it looks like a full model name
			if strings.Contains(variant, "-") || strings.Contains(variant, ".") {
				args = append(args, "-m", variant)
			}
		}
	}

	// Add the prompt
	args = append(args, prompt)

	return args
}

// executeCommand runs the codex binary and captures output
func (o *OpenAICodexProvider) executeCommand(args []string, workDir string) (string, error) {
	// Create a temp file to capture the final response cleanly
	tmpFile, err := os.CreateTemp("", "codex-output-*.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Add --output-last-message flag to capture clean output
	args = append(args[:len(args)-1], "--output-last-message", tmpPath, args[len(args)-1])

	cmd := exec.Command(o.binaryPath, args...)

	if workDir != "" {
		cmd.Dir = workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Set a generous timeout for agentic tasks
	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	// 10 minute timeout for complex tasks
	timeout := 10 * time.Minute
	select {
	case err := <-done:
		if err != nil {
			stderrStr := stderr.String()
			o.debugf("Command failed: %v, stderr: %s", err, stderrStr)
			return "", fmt.Errorf("codex command failed: %w (stderr: %s)", err, stderrStr)
		}
		// Read the clean output from the temp file
		outputBytes, err := os.ReadFile(tmpPath)
		if err != nil {
			// Fall back to stdout if temp file read fails
			o.debugf("Failed to read output file, falling back to stdout: %v", err)
			return stdout.String(), nil
		}
		output := strings.TrimSpace(string(outputBytes))
		if output == "" {
			// Fall back to stdout if output file is empty
			return stdout.String(), nil
		}
		return output, nil
	case <-time.After(timeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return "", fmt.Errorf("codex command timed out after %v", timeout)
	}
}

// SetVerbose enables or disables verbose mode
func (o *OpenAICodexProvider) SetVerbose(verbose bool) {
	o.verbose = verbose
}

// ValidateModel checks if the model is valid for OpenAI Codex
func (o *OpenAICodexProvider) ValidateModel(modelName string) bool {
	return o.SupportsModel(modelName)
}

// IsOpenAICodexAvailable checks if the codex binary is available on the system
func IsOpenAICodexAvailable() bool {
	provider := NewOpenAICodexProvider()
	_, err := provider.findCodexBinary()
	if err != nil {
		config.DebugLog("[Provider] OpenAI Codex binary not found: %v", err)
		return false
	}
	config.DebugLog("[Provider] OpenAI Codex binary is available")
	return true
}
