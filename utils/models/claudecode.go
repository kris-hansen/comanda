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
	debugFile  string // Optional debug file path for streaming output
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

	// Expand paths (handle ~, $HOME, etc.) before validation
	expandedPaths, err := fileutil.ExpandPaths(allowedPaths)
	if err != nil {
		return "", fmt.Errorf("failed to expand allowed_paths: %w", err)
	}

	// Validate all allowed paths exist (do this first to fail fast on bad config)
	for i, path := range expandedPaths {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return "", fmt.Errorf("allowed_path does not exist: %s (expanded from %s)", path, allowedPaths[i])
		}
	}

	// Use expanded paths from here on
	allowedPaths = expandedPaths

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

// getModelFlag returns the --model flag value for a given model name, or empty string if none needed
func (c *ClaudeCodeProvider) getModelFlag(modelName string) string {
	if !strings.HasPrefix(strings.ToLower(modelName), "claude-code-") {
		return ""
	}
	variant := strings.TrimPrefix(strings.ToLower(modelName), "claude-code-")
	switch variant {
	case "opus":
		return "claude-opus-4-5-20251101"
	case "sonnet":
		return "claude-sonnet-4-5-20250929"
	case "haiku":
		return "claude-haiku-4-5-20251001"
	default:
		if strings.Contains(variant, "-") {
			return variant
		}
		return ""
	}
}

// buildArgs constructs the command line arguments for claude
func (c *ClaudeCodeProvider) buildArgs(modelName string, prompt string, workDir string) []string {
	args := []string{
		"--print", // Non-interactive mode, just print the response
	}

	if model := c.getModelFlag(modelName); model != "" {
		args = append(args, "--model", model)
	}

	// Add the prompt
	args = append(args, "-p", prompt)

	return args
}

// buildArgsAgentic constructs command line arguments for agentic mode (no --print)
func (c *ClaudeCodeProvider) buildArgsAgentic(modelName string, prompt string, allowedPaths []string, tools []string) []string {
	args := []string{} // NO --print flag - enables tool use

	// Add debug file for streaming visibility into tool calls
	if c.debugFile != "" {
		args = append(args, "--debug-file", c.debugFile)
	}

	// Add allowed paths for tool access scope
	for _, path := range allowedPaths {
		args = append(args, "--add-dir", path)
	}

	// Enable bypass permissions for non-interactive agentic use
	// Without this, Claude Code prompts for permission which blocks non-interactive execution
	if len(allowedPaths) > 0 {
		args = append(args, "--dangerously-skip-permissions")
	}

	// Restrict tools if specified
	if len(tools) > 0 {
		args = append(args, "--tools", strings.Join(tools, ","))
	}

	if model := c.getModelFlag(modelName); model != "" {
		args = append(args, "--model", model)
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

	c.debugf("Starting claude command in %s", workDir)
	startTime := time.Now()

	// Set a generous timeout for agentic tasks
	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	// Progress ticker - log every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// 30 minute timeout for complex tasks (increased from 10)
	timeout := 30 * time.Minute
	for {
		select {
		case err := <-done:
			elapsed := time.Since(startTime).Round(time.Second)
			if err != nil {
				stderrStr := stderr.String()
				c.debugf("Command failed after %v: %v, stderr: %s", elapsed, err, stderrStr)
				return "", fmt.Errorf("claude command failed: %w (stderr: %s)", err, stderrStr)
			}
			c.debugf("Command completed successfully in %v", elapsed)
			return stdout.String(), nil
		case <-ticker.C:
			elapsed := time.Since(startTime).Round(time.Second)
			stdoutLen := stdout.Len()
			stderrLen := stderr.Len()
			c.debugf("Claude still running... %v elapsed (stdout: %d bytes, stderr: %d bytes)", elapsed, stdoutLen, stderrLen)
		case <-time.After(timeout):
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			return "", fmt.Errorf("claude command timed out after %v", timeout)
		}
	}
}

// SetVerbose enables or disables verbose mode
func (c *ClaudeCodeProvider) SetVerbose(verbose bool) {
	c.verbose = verbose
}

// SetDebugFile sets the path for claude-code's debug output
// This enables --debug-file which streams tool calls and execution details
func (c *ClaudeCodeProvider) SetDebugFile(path string) {
	c.debugFile = path
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
