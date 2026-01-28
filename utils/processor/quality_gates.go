package processor

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// QualityGate is the interface that all quality gates must implement
type QualityGate interface {
	Name() string
	Check(ctx context.Context, workDir string) (*QualityGateResult, error)
}

// CommandGate executes a shell command and checks the exit code
type CommandGate struct {
	name    string
	command string
	timeout int
}

// NewCommandGate creates a new command-based quality gate
func NewCommandGate(name, command string, timeout int) *CommandGate {
	if timeout <= 0 {
		timeout = 60 // Default 60 seconds
	}
	return &CommandGate{
		name:    name,
		command: command,
		timeout: timeout,
	}
}

func (g *CommandGate) Name() string {
	return g.name
}

func (g *CommandGate) Check(ctx context.Context, workDir string) (*QualityGateResult, error) {
	start := time.Now()

	// Create timeout context
	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(g.timeout)*time.Second)
	defer cancel()

	// Execute command
	cmd := exec.CommandContext(cmdCtx, "bash", "-c", g.command)
	cmd.Dir = workDir

	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	result := &QualityGateResult{
		GateName: g.name,
		Duration: duration,
		Attempts: 1,
	}

	if err != nil {
		result.Passed = false
		result.Message = fmt.Sprintf("Command failed: %v", err)
		result.Details = map[string]interface{}{
			"output":    string(output),
			"command":   g.command,
			"exit_code": getExitCode(err),
		}
		return result, nil
	}

	result.Passed = true
	result.Message = "Command succeeded"
	result.Details = map[string]interface{}{
		"output":  string(output),
		"command": g.command,
	}

	return result, nil
}

// SyntaxGate checks for common syntax errors in various languages
type SyntaxGate struct {
	name    string
	workDir string
	timeout int
}

// NewSyntaxGate creates a new syntax checking gate
func NewSyntaxGate(name string, timeout int) *SyntaxGate {
	if timeout <= 0 {
		timeout = 30
	}
	return &SyntaxGate{
		name:    name,
		timeout: timeout,
	}
}

func (g *SyntaxGate) Name() string {
	return g.name
}

func (g *SyntaxGate) Check(ctx context.Context, workDir string) (*QualityGateResult, error) {
	start := time.Now()

	result := &QualityGateResult{
		GateName: g.name,
		Duration: 0,
		Attempts: 1,
		Details:  make(map[string]interface{}),
	}

	// Detect language-specific syntax checkers
	checks := []struct {
		pattern string
		command string
		label   string
	}{
		{"*.go", "go vet ./...", "Go"},
		{"*.py", "python -m py_compile **/*.py", "Python"},
		{"*.js", "node --check **/*.js", "JavaScript"},
		{"*.ts", "tsc --noEmit", "TypeScript"},
		{"*.java", "javac -Xlint:all **/*.java", "Java"},
		{"*.rb", "ruby -c **/*.rb", "Ruby"},
	}

	passed := true
	messages := []string{}

	for _, check := range checks {
		// Check if files matching pattern exist
		matches, err := filepath.Glob(filepath.Join(workDir, check.pattern))
		if err != nil || len(matches) == 0 {
			continue
		}

		// Run syntax check
		cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(g.timeout)*time.Second)
		cmd := exec.CommandContext(cmdCtx, "bash", "-c", check.command)
		cmd.Dir = workDir
		output, err := cmd.CombinedOutput()
		cancel()

		if err != nil {
			passed = false
			messages = append(messages, fmt.Sprintf("%s syntax error: %s", check.label, string(output)))
			result.Details[strings.ToLower(check.label)] = string(output)
		} else {
			messages = append(messages, fmt.Sprintf("%s syntax OK", check.label))
		}
	}

	result.Duration = time.Since(start)
	result.Passed = passed

	if passed {
		result.Message = strings.Join(messages, "; ")
	} else {
		result.Message = "Syntax errors detected: " + strings.Join(messages, "; ")
	}

	return result, nil
}

// SecurityGate scans for common security issues
type SecurityGate struct {
	name    string
	timeout int
}

// NewSecurityGate creates a new security scanning gate
func NewSecurityGate(name string, timeout int) *SecurityGate {
	if timeout <= 0 {
		timeout = 60
	}
	return &SecurityGate{
		name:    name,
		timeout: timeout,
	}
}

func (g *SecurityGate) Name() string {
	return g.name
}

func (g *SecurityGate) Check(ctx context.Context, workDir string) (*QualityGateResult, error) {
	start := time.Now()

	result := &QualityGateResult{
		GateName: g.name,
		Duration: 0,
		Attempts: 1,
		Details:  make(map[string]interface{}),
	}

	// Common patterns for security issues
	securityPatterns := []struct {
		pattern     string
		description string
	}{
		{`(?i)(password|passwd|pwd)\s*=\s*["'][^"']+["']`, "Hardcoded password"},
		{`(?i)(api_key|apikey|access_key)\s*=\s*["'][^"']+["']`, "Hardcoded API key"},
		{`(?i)(secret|token)\s*=\s*["'][^"']+["']`, "Hardcoded secret/token"},
		{`(?i)-----BEGIN\s+(?:RSA\s+)?PRIVATE\s+KEY-----`, "Private key in code"},
		{`eval\s*\(`, "Use of eval() function"},
		{`exec\s*\(`, "Use of exec() function"},
		{`(?i)select\s+.*\s+from\s+.*\s+where.*\+`, "Possible SQL injection"},
	}

	issues := []string{}
	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(g.timeout)*time.Second)
	defer cancel()

	for _, sp := range securityPatterns {
		// Use grep to search for patterns
		cmd := exec.CommandContext(cmdCtx, "grep", "-r", "-E", "-n", sp.pattern, workDir)
		output, err := cmd.CombinedOutput()

		if err == nil && len(output) > 0 {
			// Found potential security issue
			issues = append(issues, fmt.Sprintf("%s: %s", sp.description, strings.TrimSpace(string(output))))
		}
	}

	result.Duration = time.Since(start)

	if len(issues) > 0 {
		result.Passed = false
		result.Message = fmt.Sprintf("Found %d potential security issues", len(issues))
		result.Details["issues"] = issues
	} else {
		result.Passed = true
		result.Message = "No security issues detected"
	}

	return result, nil
}

// TestGate runs tests and checks for failures
type TestGate struct {
	name    string
	command string
	timeout int
}

// NewTestGate creates a new test execution gate
func NewTestGate(name, command string, timeout int) *TestGate {
	if timeout <= 0 {
		timeout = 300 // Default 5 minutes for tests
	}
	return &TestGate{
		name:    name,
		command: command,
		timeout: timeout,
	}
}

func (g *TestGate) Name() string {
	return g.name
}

func (g *TestGate) Check(ctx context.Context, workDir string) (*QualityGateResult, error) {
	start := time.Now()

	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(g.timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "bash", "-c", g.command)
	cmd.Dir = workDir

	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	result := &QualityGateResult{
		GateName: g.name,
		Duration: duration,
		Attempts: 1,
	}

	if err != nil {
		result.Passed = false
		result.Message = "Tests failed"
		result.Details = map[string]interface{}{
			"output":    string(output),
			"command":   g.command,
			"exit_code": getExitCode(err),
		}

		// Try to extract test summary from common test runners
		if summary := extractTestSummary(string(output)); summary != "" {
			result.Details["summary"] = summary
		}

		return result, nil
	}

	result.Passed = true
	result.Message = "All tests passed"
	result.Details = map[string]interface{}{
		"output":  string(output),
		"command": g.command,
	}

	if summary := extractTestSummary(string(output)); summary != "" {
		result.Details["summary"] = summary
	}

	return result, nil
}

// RunQualityGates executes a list of quality gates with retry logic
func RunQualityGates(configs []QualityGateConfig, workDir string) ([]QualityGateResult, error) {
	var results []QualityGateResult
	ctx := context.Background()

	for _, config := range configs {
		// Create the appropriate gate
		gate, err := createGate(config)
		if err != nil {
			return results, fmt.Errorf("failed to create gate '%s': %w", config.Name, err)
		}

		// Run gate with retry logic
		result, err := runGateWithRetry(ctx, gate, workDir, config)
		if err != nil {
			return results, fmt.Errorf("failed to run gate '%s': %w", config.Name, err)
		}

		results = append(results, *result)

		// Handle failure based on on_fail policy
		if !result.Passed {
			switch config.OnFail {
			case "abort":
				return results, fmt.Errorf("quality gate '%s' failed (on_fail: abort)", config.Name)
			case "skip":
				// Continue to next gate
				continue
			case "retry":
				// Already retried, continue
				continue
			default:
				// Default to abort
				return results, fmt.Errorf("quality gate '%s' failed", config.Name)
			}
		}
	}

	return results, nil
}

// createGate creates a QualityGate from config
func createGate(config QualityGateConfig) (QualityGate, error) {
	switch config.Type {
	case "syntax":
		return NewSyntaxGate(config.Name, config.Timeout), nil
	case "security":
		return NewSecurityGate(config.Name, config.Timeout), nil
	case "test":
		if config.Command == "" {
			return nil, fmt.Errorf("test gate requires a command")
		}
		return NewTestGate(config.Name, config.Command, config.Timeout), nil
	case "":
		// Default to command gate if command is provided
		if config.Command == "" {
			return nil, fmt.Errorf("gate requires either a type or command")
		}
		return NewCommandGate(config.Name, config.Command, config.Timeout), nil
	default:
		return nil, fmt.Errorf("unknown gate type: %s", config.Type)
	}
}

// runGateWithRetry executes a gate with retry logic
func runGateWithRetry(ctx context.Context, gate QualityGate, workDir string, config QualityGateConfig) (*QualityGateResult, error) {
	maxAttempts := 1
	backoffType := "linear"
	initialDelay := 1

	if config.Retry != nil {
		if config.Retry.MaxAttempts > 0 {
			maxAttempts = config.Retry.MaxAttempts
		}
		if config.Retry.BackoffType != "" {
			backoffType = config.Retry.BackoffType
		}
		if config.Retry.InitialDelay > 0 {
			initialDelay = config.Retry.InitialDelay
		}
	}

	var lastResult *QualityGateResult
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result, err := gate.Check(ctx, workDir)
		if err != nil {
			lastErr = err
			lastResult = result
		} else {
			lastResult = result
			lastResult.Attempts = attempt

			if result.Passed {
				return result, nil
			}
		}

		// If not the last attempt and gate failed, wait before retry
		if attempt < maxAttempts && (lastErr != nil || !lastResult.Passed) {
			delay := calculateBackoff(attempt, initialDelay, backoffType)
			time.Sleep(time.Duration(delay) * time.Second)
		}
	}

	// All attempts exhausted
	if lastResult != nil {
		lastResult.Attempts = maxAttempts
		return lastResult, lastErr
	}

	return nil, fmt.Errorf("quality gate failed after %d attempts: %w", maxAttempts, lastErr)
}

// calculateBackoff calculates the delay based on backoff strategy
func calculateBackoff(attempt, initialDelay int, backoffType string) int {
	switch backoffType {
	case "exponential":
		// delay = initialDelay * 2^(attempt-1)
		delay := initialDelay
		for i := 1; i < attempt; i++ {
			delay *= 2
		}
		return delay
	case "linear":
		fallthrough
	default:
		// delay = initialDelay * attempt
		return initialDelay * attempt
	}
}

// getExitCode extracts exit code from error
func getExitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// extractTestSummary tries to extract test summary from common test runners
func extractTestSummary(output string) string {
	patterns := []string{
		`(\d+)\s+passed.*(\d+)\s+failed`,         // Jest, pytest
		`(\d+)\s+tests?,\s+(\d+)\s+failures?`,    // JUnit, Go
		`PASS:\s+(\d+).*FAIL:\s+(\d+)`,           // Go test
		`(\d+)\s+examples?,\s+(\d+)\s+failures?`, // RSpec
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if match := re.FindStringSubmatch(output); match != nil {
			return match[0]
		}
	}

	return ""
}
