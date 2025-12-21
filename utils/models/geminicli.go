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

// GeminiCLIProvider handles Gemini CLI for agentic programming tasks
type GeminiCLIProvider struct {
	verbose    bool
	binaryPath string
	mu         sync.Mutex
}

// NewGeminiCLIProvider creates a new Gemini CLI provider instance
func NewGeminiCLIProvider() *GeminiCLIProvider {
	return &GeminiCLIProvider{}
}

// Name returns the provider name
func (g *GeminiCLIProvider) Name() string {
	return "gemini-cli"
}

// debugf prints debug information if verbose mode is enabled (thread-safe)
func (g *GeminiCLIProvider) debugf(format string, args ...interface{}) {
	if g.verbose {
		g.mu.Lock()
		defer g.mu.Unlock()
		log.Printf("[DEBUG][GeminiCLI] "+format+"\n", args...)
	}
}

// SupportsModel checks if this provider supports the given model name
func (g *GeminiCLIProvider) SupportsModel(modelName string) bool {
	g.debugf("Checking if model is supported: %s", modelName)

	// Support gemini-cli as the primary model name
	// Also support variations like gemini-cli-pro, gemini-cli-flash
	modelLower := strings.ToLower(modelName)
	supported := modelLower == "gemini-cli" ||
		strings.HasPrefix(modelLower, "gemini-cli-")

	if supported {
		g.debugf("Model %s is supported by Gemini CLI provider", modelName)
	}
	return supported
}

// Configure sets up the provider
// Gemini CLI uses its own authentication via GEMINI_API_KEY env var
func (g *GeminiCLIProvider) Configure(apiKey string) error {
	g.debugf("Configuring Gemini CLI provider")

	// Find the gemini binary
	binaryPath, err := g.findGeminiBinary()
	if err != nil {
		return fmt.Errorf("gemini binary not found: %w", err)
	}
	g.binaryPath = binaryPath
	g.debugf("Found gemini binary at: %s", binaryPath)

	return nil
}

// findGeminiBinary locates the gemini CLI binary
func (g *GeminiCLIProvider) findGeminiBinary() (string, error) {
	// First check if it's in PATH
	path, err := exec.LookPath("gemini")
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
		filepath.Join(homeDir, ".npm-global", "bin", "gemini"),
		filepath.Join(homeDir, "node_modules", ".bin", "gemini"),
		"/usr/local/bin/gemini",
		"/usr/bin/gemini",
		// Homebrew locations
		"/opt/homebrew/bin/gemini",
		"/usr/local/Cellar/gemini-cli/bin/gemini",
	}

	for _, p := range commonPaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("gemini binary not found in PATH or common locations")
}

// SendPrompt sends a prompt to Gemini CLI and returns the response
func (g *GeminiCLIProvider) SendPrompt(modelName string, prompt string) (string, error) {
	g.debugf("Preparing to send prompt via Gemini CLI")
	g.debugf("Prompt length: %d characters", len(prompt))

	if g.binaryPath == "" {
		if err := g.Configure("LOCAL"); err != nil {
			return "", err
		}
	}

	// Build the command arguments
	args := g.buildArgs(modelName, prompt, "")

	g.debugf("Executing: %s %v", g.binaryPath, args)

	// Use retry mechanism for execution
	result, err := retry.WithRetry(
		func() (interface{}, error) {
			return g.executeCommand(args, "")
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
	g.debugf("Command completed, response length: %d characters", len(response))
	return response, nil
}

// SendPromptWithFile sends a prompt along with file context to Gemini CLI
func (g *GeminiCLIProvider) SendPromptWithFile(modelName string, prompt string, file FileInput) (string, error) {
	g.debugf("Preparing to send prompt with file to Gemini CLI")
	g.debugf("File path: %s", file.Path)

	if g.binaryPath == "" {
		if err := g.Configure("LOCAL"); err != nil {
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
	args := g.buildArgs(modelName, combinedPrompt, filepath.Dir(file.Path))

	g.debugf("Executing with file context: %s", file.Path)

	result, err := retry.WithRetry(
		func() (interface{}, error) {
			return g.executeCommand(args, filepath.Dir(file.Path))
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
	g.debugf("Command completed, response length: %d characters", len(response))
	return response, nil
}

// buildArgs constructs the command line arguments for gemini
func (g *GeminiCLIProvider) buildArgs(modelName string, prompt string, workDir string) []string {
	args := []string{}

	// Extract model variant if specified (e.g., gemini-cli-flash -> flash)
	if strings.HasPrefix(strings.ToLower(modelName), "gemini-cli-") {
		variant := strings.TrimPrefix(strings.ToLower(modelName), "gemini-cli-")
		// Map to actual Gemini model names
		switch variant {
		case "pro":
			args = append(args, "-m", "gemini-2.5-pro")
		case "flash":
			args = append(args, "-m", "gemini-2.5-flash")
		case "flash-lite":
			args = append(args, "-m", "gemini-2.5-flash-lite")
		default:
			// Use the variant as-is if it looks like a full model name
			if strings.Contains(variant, "-") || strings.Contains(variant, ".") {
				args = append(args, "-m", variant)
			}
		}
	}

	// Add the prompt using -p flag for non-interactive mode
	args = append(args, "-p", prompt)

	return args
}

// executeCommand runs the gemini binary and captures output
func (g *GeminiCLIProvider) executeCommand(args []string, workDir string) (string, error) {
	cmd := exec.Command(g.binaryPath, args...)

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
			g.debugf("Command failed: %v, stderr: %s", err, stderrStr)
			return "", fmt.Errorf("gemini command failed: %w (stderr: %s)", err, stderrStr)
		}
		return stdout.String(), nil
	case <-time.After(timeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return "", fmt.Errorf("gemini command timed out after %v", timeout)
	}
}

// SetVerbose enables or disables verbose mode
func (g *GeminiCLIProvider) SetVerbose(verbose bool) {
	g.verbose = verbose
}

// ValidateModel checks if the model is valid for Gemini CLI
func (g *GeminiCLIProvider) ValidateModel(modelName string) bool {
	return g.SupportsModel(modelName)
}

// IsGeminiCLIAvailable checks if the gemini binary is available on the system
func IsGeminiCLIAvailable() bool {
	provider := NewGeminiCLIProvider()
	_, err := provider.findGeminiBinary()
	if err != nil {
		config.DebugLog("[Provider] Gemini CLI binary not found: %v", err)
		return false
	}
	config.DebugLog("[Provider] Gemini CLI binary is available")
	return true
}
