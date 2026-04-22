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

	"github.com/kris-hansen/comanda/utils/fileutil"
	"github.com/kris-hansen/comanda/utils/retry"
)

const (
	llamaCPPPrefixA = "llama.cpp:"
	llamaCPPPrefixB = "llamacpp:"
)

// LlamaCPPProvider handles local llama.cpp CLI execution against GGUF models.
type LlamaCPPProvider struct {
	verbose    bool
	binaryPath string
	mu         sync.Mutex
}

// NewLlamaCPPProvider creates a new llama.cpp provider instance.
func NewLlamaCPPProvider() *LlamaCPPProvider {
	return &LlamaCPPProvider{}
}

// Name returns the provider name.
func (l *LlamaCPPProvider) Name() string {
	return "llama.cpp"
}

// debugf prints debug information if verbose mode is enabled.
func (l *LlamaCPPProvider) debugf(format string, args ...interface{}) {
	if l.verbose {
		l.mu.Lock()
		defer l.mu.Unlock()
		log.Printf("[DEBUG][llama.cpp] "+format+"\n", args...)
	}
}

// SupportsModel accepts GGUF file paths directly, with optional llama.cpp:/llamacpp: prefixes.
func (l *LlamaCPPProvider) SupportsModel(modelName string) bool {
	normalized := normalizeLlamaCPPModel(modelName)
	return strings.HasSuffix(strings.ToLower(normalized), ".gguf")
}

// Configure resolves the llama.cpp CLI binary.
func (l *LlamaCPPProvider) Configure(apiKey string) error {
	if apiKey != "" && apiKey != "LOCAL" {
		return fmt.Errorf("invalid API key for llama.cpp: must be 'LOCAL' to indicate local execution")
	}

	binaryPath, err := findLlamaCPPBinary()
	if err != nil {
		return err
	}
	l.binaryPath = binaryPath
	l.debugf("Found llama.cpp binary at: %s", binaryPath)
	return nil
}

// SendPrompt executes llama.cpp against the requested GGUF model.
func (l *LlamaCPPProvider) SendPrompt(modelName string, prompt string) (string, error) {
	if l.binaryPath == "" {
		if err := l.Configure("LOCAL"); err != nil {
			return "", err
		}
	}

	modelPath, err := l.resolveModelPath(modelName)
	if err != nil {
		return "", err
	}

	args := l.buildArgs(modelPath, prompt)
	l.debugf("Executing: %s %v", l.binaryPath, args)

	result, err := retry.WithRetry(
		func() (interface{}, error) {
			return l.executeCommand(args, "")
		},
		func(err error) bool {
			return strings.Contains(err.Error(), "timeout")
		},
		retry.DefaultRetryConfig,
	)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(result.(string)), nil
}

// SendPromptWithFile executes llama.cpp with file context embedded into the prompt.
func (l *LlamaCPPProvider) SendPromptWithFile(modelName string, prompt string, file FileInput) (string, error) {
	fileData, err := fileutil.SafeReadFile(file.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	combinedPrompt := fmt.Sprintf("File: %s\n\n```\n%s\n```\n\nTask: %s", file.Path, string(fileData), prompt)
	return l.SendPrompt(modelName, combinedPrompt)
}

// SetVerbose enables or disables verbose mode.
func (l *LlamaCPPProvider) SetVerbose(verbose bool) {
	l.verbose = verbose
}

func (l *LlamaCPPProvider) buildArgs(modelPath, prompt string) []string {
	args := []string{
		"-m", modelPath,
		"--no-display-prompt",
		"--no-show-timings",
		"-n", "1024",
		"-st",
		"-p", prompt,
	}

	if extra := strings.TrimSpace(os.Getenv("LLAMA_CPP_ARGS")); extra != "" {
		args = append(args, strings.Fields(extra)...)
	}

	return args
}

func (l *LlamaCPPProvider) executeCommand(args []string, workDir string) (string, error) {
	cmd := exec.Command(l.binaryPath, args...)
	if workDir != "" {
		cmd.Dir = workDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	done := make(chan error, 1)
	go func() {
		done <- cmd.Run()
	}()

	select {
	case err := <-done:
		if err != nil {
			return "", fmt.Errorf("llama.cpp execution failed: %w\nstderr: %s", err, strings.TrimSpace(stderr.String()))
		}
	case <-time.After(2 * time.Minute):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return "", fmt.Errorf("llama.cpp execution timeout")
	}

	response := strings.TrimSpace(stdout.String())
	if response == "" && stderr.Len() > 0 {
		return "", fmt.Errorf("llama.cpp returned no output\nstderr: %s", strings.TrimSpace(stderr.String()))
	}

	return response, nil
}

func (l *LlamaCPPProvider) resolveModelPath(modelName string) (string, error) {
	modelPath := normalizeLlamaCPPModel(modelName)
	if modelPath == "" {
		return "", fmt.Errorf("llama.cpp model path is empty")
	}

	absPath, err := filepath.Abs(modelPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve GGUF path %q: %w", modelPath, err)
	}

	if !strings.HasSuffix(strings.ToLower(absPath), ".gguf") {
		return "", fmt.Errorf("llama.cpp requires a .gguf model file, got %q", modelName)
	}

	if _, err := os.Stat(absPath); err != nil {
		return "", fmt.Errorf("GGUF model file not found: %s", absPath)
	}

	return absPath, nil
}

func normalizeLlamaCPPModel(modelName string) string {
	trimmed := strings.TrimSpace(modelName)
	lower := strings.ToLower(trimmed)
	switch {
	case strings.HasPrefix(lower, llamaCPPPrefixA):
		return strings.TrimSpace(trimmed[len(llamaCPPPrefixA):])
	case strings.HasPrefix(lower, llamaCPPPrefixB):
		return strings.TrimSpace(trimmed[len(llamaCPPPrefixB):])
	default:
		return trimmed
	}
}

func findLlamaCPPBinary() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("LLAMA_CPP_BINARY")); configured != "" {
		if _, err := os.Stat(configured); err == nil {
			return configured, nil
		}
	}

	candidates := []string{
		"llama-cli",
		"llama",
	}
	for _, candidate := range candidates {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("llama.cpp binary not found and home directory unavailable: %w", err)
	}

	commonPaths := []string{
		filepath.Join(homeDir, ".local", "bin", "llama-cli"),
		filepath.Join(homeDir, "llama.cpp", "build", "bin", "llama-cli"),
		filepath.Join(homeDir, "src", "llama.cpp", "build", "bin", "llama-cli"),
		"/opt/homebrew/bin/llama-cli",
		"/usr/local/bin/llama-cli",
		"/usr/bin/llama-cli",
	}

	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("llama.cpp binary not found; set LLAMA_CPP_BINARY or install llama-cli")
}

// IsLlamaCPPAvailable reports whether the llama.cpp CLI is available locally.
func IsLlamaCPPAvailable() bool {
	_, err := findLlamaCPPBinary()
	return err == nil
}
