package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/fileutil"
	"github.com/kris-hansen/comanda/utils/retry"
)

// kimiStreamEvent mirrors one NDJSON line emitted by `kimi --output-format stream-json`.
// Only the fields we actually consume are declared. A typical stream is:
//
//	{"role":"assistant","tool_calls":[...]}          (zero or more tool-call rounds)
//	{"role":"tool","tool_call_id":"...","content":...}
//	{"role":"assistant","content":"final answer"}    (the result we want)
//	{"role":"meta","type":"session.resume_hint",...} (trailing metadata)
type kimiStreamEvent struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// extractKimiCodeResult parses the NDJSON produced by `--output-format stream-json`
// and returns the content of the last assistant text message, which is the final
// answer. Returns the input unchanged if no assistant message parses (so unexpected
// format changes degrade safely).
//
// We use stream-json rather than the default text format because the text renderer
// decorates output for humans (a "• " prefix on the first line, indented
// continuation lines), which would corrupt generated code and other content that
// comanda writes to files or matches against loop exit conditions.
func extractKimiCodeResult(stdout string) string {
	result := ""
	for _, line := range strings.Split(stdout, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed[0] != '{' {
			continue
		}
		var ev kimiStreamEvent
		if err := json.Unmarshal([]byte(trimmed), &ev); err != nil {
			continue
		}
		if ev.Role == "assistant" && ev.Content != "" {
			result = ev.Content
		}
	}
	if result == "" {
		return stdout
	}
	return result
}

// kimiVersionPattern matches the bare semver printed by `kimi --version` (e.g.
// "0.27.0"). It is anchored so the legacy Python kimi-cli's click-style output
// ("kimi, version 0.x.y") does not match.
var kimiVersionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// KimiCodeProvider handles the Kimi Code CLI (kimi) for agentic programming tasks.
// Authentication is delegated to the CLI itself (kimi login / ~/.kimi-code/config.toml);
// comanda never sees or stores a credential.
//
// Unlike `claude --print`, `kimi -p` takes the prompt as an argv value rather than
// on stdin. This has two consequences:
//   - Prompt text is visible in process listings while the subprocess runs.
//   - Large prompts can hit OS argument-size limits (notably Linux's 128 KiB
//     per-argument MAX_ARG_STRLEN). SendPromptWithFile avoids this by referencing
//     files by path (kimi's read-only tools auto-execute in -p mode) instead of
//     inlining their contents.
type KimiCodeProvider struct {
	verbose    bool
	binaryPath string
	mu         sync.Mutex
}

// NewKimiCodeProvider creates a new Kimi Code provider instance
func NewKimiCodeProvider() *KimiCodeProvider {
	return &KimiCodeProvider{}
}

// Name returns the provider name
func (k *KimiCodeProvider) Name() string {
	return "kimi-code"
}

// debugf prints debug information if verbose mode is enabled (thread-safe)
func (k *KimiCodeProvider) debugf(format string, args ...interface{}) {
	if k.verbose {
		k.mu.Lock()
		defer k.mu.Unlock()
		log.Printf("[DEBUG][KimiCode] "+format+"\n", args...)
	}
}

// SupportsModel checks if this provider supports the given model name
func (k *KimiCodeProvider) SupportsModel(modelName string) bool {
	k.debugf("Checking if model is supported: %s", modelName)

	// Support kimi-code as the primary model name
	// Also support variations like kimi-code-<alias> where <alias> is a model
	// alias from the user's ~/.kimi-code/config.toml
	modelLower := strings.ToLower(modelName)
	supported := modelLower == "kimi-code" ||
		strings.HasPrefix(modelLower, "kimi-code-")

	if supported {
		k.debugf("Model %s is supported by Kimi Code provider", modelName)
	}
	return supported
}

// Configure sets up the provider
// Kimi Code uses its own authentication (kimi login), so we accept "LOCAL" or empty
func (k *KimiCodeProvider) Configure(apiKey string) error {
	k.debugf("Configuring Kimi Code provider")

	// Find the kimi binary
	binaryPath, err := k.findKimiBinary()
	if err != nil {
		return fmt.Errorf("kimi binary not found: %w", err)
	}

	// Reject the legacy Python kimi-cli, which shares the `kimi` binary name
	if err := verifyKimiBinary(binaryPath); err != nil {
		return err
	}

	k.binaryPath = binaryPath
	k.debugf("Found kimi binary at: %s", binaryPath)

	return nil
}

// findKimiBinary locates the kimi CLI binary
func (k *KimiCodeProvider) findKimiBinary() (string, error) {
	// First check if it's in PATH
	path, err := exec.LookPath("kimi")
	if err == nil {
		return path, nil
	}

	// Check common installation locations
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not find home directory: %w", err)
	}

	commonPaths := []string{
		// Official install script (curl -L code.kimi.com/install.sh | bash)
		filepath.Join(homeDir, ".kimi-code", "bin", "kimi"),
		filepath.Join(homeDir, ".local", "bin", "kimi"),
		// npm global install locations
		filepath.Join(homeDir, ".npm-global", "bin", "kimi"),
		"/usr/local/bin/kimi",
		"/usr/bin/kimi",
		// Homebrew
		"/opt/homebrew/bin/kimi",
	}

	for _, p := range commonPaths {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	return "", fmt.Errorf("kimi binary not found in PATH or common locations. Install Kimi Code CLI via 'npm install -g @moonshot-ai/kimi-code' or 'curl -L code.kimi.com/install.sh | bash'")
}

// verifyKimiBinary confirms the binary is Kimi Code and not the legacy Python
// kimi-cli, which uses the same `kimi` binary name but is a different,
// incompatible tool (kimi-code even ships `kimi migrate` for upgrading old
// installs). Kimi Code prints a bare semver from --version.
func verifyKimiBinary(binaryPath string) error {
	cmd := exec.Command(binaryPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to run 'kimi --version': %w", err)
	}
	if !kimiVersionPattern.MatchString(strings.TrimSpace(string(output))) {
		return fmt.Errorf("the 'kimi' binary at %s does not look like Kimi Code CLI (--version output: %q); it may be the legacy Python kimi-cli. Install Kimi Code via 'npm install -g @moonshot-ai/kimi-code' or 'curl -L code.kimi.com/install.sh | bash'",
			binaryPath, strings.TrimSpace(string(output)))
	}
	return nil
}

// SendPrompt sends a prompt to Kimi Code and returns the response
func (k *KimiCodeProvider) SendPrompt(modelName string, prompt string) (string, error) {
	k.debugf("Preparing to send prompt via Kimi Code")
	k.debugf("Prompt length: %d characters", len(prompt))

	if k.binaryPath == "" {
		if err := k.Configure("LOCAL"); err != nil {
			return "", err
		}
	}

	// Build the command arguments. The prompt travels as an argv value:
	// `kimi -p` has no stdin mode (a missing value is an arg-parse error).
	args := k.buildArgs(modelName, prompt)

	k.debugf("Executing: %s %v", k.binaryPath, args)

	// Use retry mechanism for execution
	result, err := retry.WithRetry(
		func() (interface{}, error) {
			return k.executeCommand(args, "")
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

	response := extractKimiCodeResult(result.(string))
	k.debugf("Command completed, response length: %d characters", len(response))
	return response, nil
}

// SendPromptWithFile sends a prompt along with file context to Kimi Code
func (k *KimiCodeProvider) SendPromptWithFile(modelName string, prompt string, file FileInput) (string, error) {
	k.debugf("Preparing to send prompt with file to Kimi Code")
	k.debugf("File path: %s", file.Path)

	if k.binaryPath == "" {
		if err := k.Configure("LOCAL"); err != nil {
			return "", err
		}
	}

	var args []string
	var workDir string

	// Prefer referencing the file by path over inlining its contents: kimi is an
	// agentic CLI whose read-only tools auto-execute in -p mode, and reading via
	// path keeps large files off argv (OS argument-size limit). Run in the file's
	// directory so the read stays inside the session workspace.
	if absPath, err := filepath.Abs(file.Path); err == nil {
		if _, statErr := os.Stat(absPath); statErr == nil {
			args = k.buildArgs(modelName, fmt.Sprintf("Read the file %s and then do the following:\n\n%s", absPath, prompt))
			workDir = filepath.Dir(absPath)
		}
	}

	// Fall back to inlining the content when the path cannot be resolved
	if args == nil {
		fileData, err := fileutil.SafeReadFile(file.Path)
		if err != nil {
			return "", fmt.Errorf("failed to read file: %w", err)
		}
		combinedPrompt := fmt.Sprintf("File: %s\n\n```\n%s\n```\n\nTask: %s", file.Path, string(fileData), prompt)
		args = k.buildArgs(modelName, combinedPrompt)
		workDir = filepath.Dir(file.Path)
	}

	k.debugf("Executing with file context: %s", file.Path)

	result, err := retry.WithRetry(
		func() (interface{}, error) {
			return k.executeCommand(args, workDir)
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

	response := extractKimiCodeResult(result.(string))
	k.debugf("Command completed, response length: %d characters", len(response))
	return response, nil
}

// getModelFlag returns the --model flag value for a given model name, or empty
// string if none needed. kimi-code-<alias> maps to the model alias <alias> from
// the user's ~/.kimi-code/config.toml; aliases are user-defined, so the variant
// is passed through as-is (preserving case, which config.toml keys may rely on).
func (k *KimiCodeProvider) getModelFlag(modelName string) string {
	if !strings.HasPrefix(strings.ToLower(modelName), "kimi-code-") {
		return ""
	}
	return modelName[len("kimi-code-"):]
}

// buildArgs constructs the command line arguments for kimi's non-interactive mode
func (k *KimiCodeProvider) buildArgs(modelName string, prompt string) []string {
	args := []string{
		"--prompt", prompt, // Run one prompt non-interactively
		"--output-format", "stream-json", // Clean NDJSON; text mode decorates output (see extractKimiCodeResult)
	}

	if model := k.getModelFlag(modelName); model != "" {
		args = append(args, "--model", model)
	}

	return args
}

// executeCommand runs the kimi binary and captures output
func (k *KimiCodeProvider) executeCommand(args []string, workDir string) (string, error) {
	cmd := exec.Command(k.binaryPath, args...)

	if workDir != "" {
		cmd.Dir = workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	k.debugf("Starting kimi command in %s", workDir)
	startTime := time.Now()

	// Set a generous timeout for agentic tasks
	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	// Progress ticker - log every 30 seconds
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	// 30 minute timeout for complex tasks
	timeout := time.After(30 * time.Minute)
	for {
		select {
		case err := <-done:
			elapsed := time.Since(startTime).Round(time.Second)
			if err != nil {
				stderrStr := stderr.String()
				k.debugf("Command failed after %v: %v, stderr: %s", elapsed, err, stderrStr)
				// Map known failure modes to actionable messages
				if strings.Contains(stderrStr, "No model configured") {
					return "", fmt.Errorf("Kimi Code CLI is not authenticated: run 'kimi login' (or configure an API key in ~/.kimi-code/config.toml)")
				}
				if strings.Contains(err.Error(), "argument list too long") {
					return "", fmt.Errorf("prompt exceeds the OS argument-size limit for 'kimi -p' (128 KiB per argument on Linux): shorten the prompt, or use file inputs so comanda references them by path instead of inlining contents: %w", err)
				}
				return "", fmt.Errorf("kimi command failed: %w (stderr: %s)", err, stderrStr)
			}
			k.debugf("Command completed successfully in %v", elapsed)
			return stdout.String(), nil
		case <-ticker.C:
			elapsed := time.Since(startTime).Round(time.Second)
			k.debugf("Kimi still running... %v elapsed (stdout: %d bytes, stderr: %d bytes)", elapsed, stdout.Len(), stderr.Len())
		case <-timeout:
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			return "", fmt.Errorf("kimi command timed out after %v", 30*time.Minute)
		}
	}
}

// SetVerbose enables or disables verbose mode
func (k *KimiCodeProvider) SetVerbose(verbose bool) {
	k.verbose = verbose
}

// ValidateModel checks if the model is valid for Kimi Code
func (k *KimiCodeProvider) ValidateModel(modelName string) bool {
	return k.SupportsModel(modelName)
}

// IsKimiCodeAvailable checks if the kimi binary is available on the system
func IsKimiCodeAvailable() bool {
	provider := NewKimiCodeProvider()
	_, err := provider.findKimiBinary()
	if err != nil {
		config.DebugLog("[Provider] Kimi Code binary not found: %v", err)
		return false
	}
	config.DebugLog("[Provider] Kimi Code binary is available")
	return true
}
