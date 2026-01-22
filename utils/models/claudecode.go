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

// ClaudeCodeProvider handles Claude Code CLI for agentic programming tasks
type ClaudeCodeProvider struct {
	verbose    bool
	binaryPath string
	mu         sync.Mutex
}

// NewClaudeCodeProvider creates a new Claude Code provider instance
func NewClaudeCodeProvider() *ClaudeCodeProvider {
	return &ClaudeCodeProvider{}
}

// Name returns the provider name
func (c *ClaudeCodeProvider) Name() string {
	return "claude-code"
}

// debugf prints debug information if verbose mode is enabled (thread-safe)
func (c *ClaudeCodeProvider) debugf(format string, args ...interface{}) {
	if c.verbose {
		c.mu.Lock()
		defer c.mu.Unlock()
		log.Printf("[DEBUG][ClaudeCode] "+format+"\n", args...)
	}
}

// SupportsModel checks if this provider supports the given model name
func (c *ClaudeCodeProvider) SupportsModel(modelName string) bool {
	c.debugf("Checking if model is supported: %s", modelName)

	// Support claude-code as the primary model name
	// Also support variations like claude-code-sonnet, claude-code-opus
	modelLower := strings.ToLower(modelName)
	supported := modelLower == "claude-code" ||
		strings.HasPrefix(modelLower, "claude-code-")

	if supported {
		c.debugf("Model %s is supported by Claude Code provider", modelName)
	}
	return supported
}

// Configure sets up the provider
// Claude Code uses its own authentication, so we accept "LOCAL" or empty
func (c *ClaudeCodeProvider) Configure(apiKey string) error {
	c.debugf("Configuring Claude Code provider")

	// Find the claude binary
	binaryPath, err := c.findClaudeBinary()
	if err != nil {
		return fmt.Errorf("claude binary not found: %w", err)
	}
	c.binaryPath = binaryPath
	c.debugf("Found claude binary at: %s", binaryPath)

	return nil
}

// findClaudeBinary locates the claude CLI binary
func (c *ClaudeCodeProvider) findClaudeBinary() (string, error) {
	// First check if it's in PATH
	path, err := exec.LookPath("claude")
	if err == nil {
		return path, nil
	}

	// Check common installation locations
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not find home directory: %w", err)
	}

	commonPaths := []string{
		filepath.Join(homeDir, ".claude", "local", "claude"),
		filepath.Join(homeDir, ".local", "bin", "claude"),
		"/usr/local/bin/claude",
		"/usr/bin/claude",
	}

	for _, p := range commonPaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("claude binary not found in PATH or common locations")
}

// SendPrompt sends a prompt to Claude Code and returns the response
func (c *ClaudeCodeProvider) SendPrompt(modelName string, prompt string) (string, error) {
	c.debugf("Preparing to send prompt via Claude Code")
	c.debugf("Prompt length: %d characters", len(prompt))

	if c.binaryPath == "" {
		if err := c.Configure("LOCAL"); err != nil {
			return "", err
		}
	}

	// Build the command arguments
	args := c.buildArgs(modelName, prompt, "")

	c.debugf("Executing: %s %v", c.binaryPath, args)

	// Use retry mechanism for execution
	result, err := retry.WithRetry(
		func() (interface{}, error) {
			return c.executeCommand(args, "")
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
	c.debugf("Command completed, response length: %d characters", len(response))
	return response, nil
}

// SendPromptWithFile sends a prompt along with file context to Claude Code
func (c *ClaudeCodeProvider) SendPromptWithFile(modelName string, prompt string, file FileInput) (string, error) {
	c.debugf("Preparing to send prompt with file to Claude Code")
	c.debugf("File path: %s", file.Path)

	if c.binaryPath == "" {
		if err := c.Configure("LOCAL"); err != nil {
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
	args := c.buildArgs(modelName, combinedPrompt, filepath.Dir(file.Path))

	c.debugf("Executing with file context: %s", file.Path)

	result, err := retry.WithRetry(
		func() (interface{}, error) {
			return c.executeCommand(args, filepath.Dir(file.Path))
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
	c.debugf("Command completed, response length: %d characters", len(response))
	return response, nil
}

// SendPromptAgentic sends a prompt with full tool access for agentic mode
func (c *ClaudeCodeProvider) SendPromptAgentic(modelName string, prompt string, allowedPaths []string, tools []string, workDir string) (string, error) {
	c.debugf("Preparing to send agentic prompt via Claude Code")
	c.debugf("Prompt length: %d characters, allowed paths: %v, tools: %v", len(prompt), allowedPaths, tools)

	if c.binaryPath == "" {
		if err := c.Configure("LOCAL"); err != nil {
			return "", err
		}
	}

	// Build agentic args (no --print flag)
	args := c.buildArgsAgentic(modelName, prompt, allowedPaths, tools)

	c.debugf("Executing agentic: %s %v", c.binaryPath, args)

	// Use retry mechanism for execution
	result, err := retry.WithRetry(
		func() (interface{}, error) {
			return c.executeCommand(args, workDir)
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
	c.debugf("Agentic command completed, response length: %d characters", len(response))
	return response, nil
}

// buildArgs constructs the command line arguments for claude
func (c *ClaudeCodeProvider) buildArgs(modelName string, prompt string, workDir string) []string {
	args := []string{
		"--print", // Non-interactive mode, just print the response
	}

	// Extract model variant if specified (e.g., claude-code-opus -> opus)
	if strings.HasPrefix(strings.ToLower(modelName), "claude-code-") {
		variant := strings.TrimPrefix(strings.ToLower(modelName), "claude-code-")
		// Map to actual Claude model names
		switch variant {
		case "opus":
			args = append(args, "--model", "claude-opus-4-5-20251101")
		case "sonnet":
			args = append(args, "--model", "claude-sonnet-4-5-20250929")
		case "haiku":
			args = append(args, "--model", "claude-haiku-4-5-20251001")
		default:
			// Use the variant as-is if it looks like a full model name
			if strings.Contains(variant, "-") {
				args = append(args, "--model", variant)
			}
		}
	}

	// Add the prompt
	args = append(args, "-p", prompt)

	return args
}

// buildArgsAgentic constructs command line arguments for agentic mode (no --print)
func (c *ClaudeCodeProvider) buildArgsAgentic(modelName string, prompt string, allowedPaths []string, tools []string) []string {
	args := []string{} // NO --print flag - enables tool use

	// Add allowed paths for tool access scope
	for _, path := range allowedPaths {
		args = append(args, "--add-dir", path)
	}

	// Restrict tools if specified
	if len(tools) > 0 {
		args = append(args, "--tools", strings.Join(tools, ","))
	}

	// Extract model variant if specified (e.g., claude-code-opus -> opus)
	if strings.HasPrefix(strings.ToLower(modelName), "claude-code-") {
		variant := strings.TrimPrefix(strings.ToLower(modelName), "claude-code-")
		// Map to actual Claude model names
		switch variant {
		case "opus":
			args = append(args, "--model", "claude-opus-4-5-20251101")
		case "sonnet":
			args = append(args, "--model", "claude-sonnet-4-5-20250929")
		case "haiku":
			args = append(args, "--model", "claude-haiku-4-5-20251001")
		default:
			// Use the variant as-is if it looks like a full model name
			if strings.Contains(variant, "-") {
				args = append(args, "--model", variant)
			}
		}
	}

	// Add the prompt
	args = append(args, "-p", prompt)

	return args
}

// executeCommand runs the claude binary and captures output
func (c *ClaudeCodeProvider) executeCommand(args []string, workDir string) (string, error) {
	cmd := exec.Command(c.binaryPath, args...)

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
			c.debugf("Command failed: %v, stderr: %s", err, stderrStr)
			return "", fmt.Errorf("claude command failed: %w (stderr: %s)", err, stderrStr)
		}
		return stdout.String(), nil
	case <-time.After(timeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return "", fmt.Errorf("claude command timed out after %v", timeout)
	}
}

// SetVerbose enables or disables verbose mode
func (c *ClaudeCodeProvider) SetVerbose(verbose bool) {
	c.verbose = verbose
}

// ValidateModel checks if the model is valid for Claude Code
func (c *ClaudeCodeProvider) ValidateModel(modelName string) bool {
	return c.SupportsModel(modelName)
}

// IsClaudeCodeAvailable checks if the claude binary is available on the system
func IsClaudeCodeAvailable() bool {
	provider := NewClaudeCodeProvider()
	_, err := provider.findClaudeBinary()
	if err != nil {
		config.DebugLog("[Provider] Claude Code binary not found: %v", err)
		return false
	}
	config.DebugLog("[Provider] Claude Code binary is available")
	return true
}
