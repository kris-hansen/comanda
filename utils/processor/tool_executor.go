package processor

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ToolConfig holds configuration for shell tool execution in workflows
type ToolConfig struct {
	// Allowlist of command names that are explicitly allowed (e.g., "ls", "bd", "jq")
	// If empty, all non-denied commands are allowed (use with caution)
	Allowlist []string `yaml:"allowlist"`

	// Denylist of command names that are explicitly denied
	// These take precedence over the allowlist
	Denylist []string `yaml:"denylist"`

	// Timeout for tool execution in seconds (default: 30)
	Timeout int `yaml:"timeout"`

	// WorkingDir for command execution (optional, defaults to current directory)
	WorkingDir string `yaml:"working_dir"`
}

// DefaultDenylist contains dangerous commands that should never be executed
// These commands can cause system damage, security issues, or data loss
var DefaultDenylist = []string{
	// Dangerous file operations
	"rm",
	"rmdir",
	"mv",
	"dd",
	"shred",
	"mkfs",
	"fdisk",
	"parted",

	// Privilege escalation
	"sudo",
	"su",
	"doas",
	"pkexec",

	// System modification
	"chmod",
	"chown",
	"chgrp",
	"chattr",
	"setfacl",

	// Network attacks
	"nc",
	"netcat",
	"ncat",
	"nmap",
	"masscan",
	"hping3",

	// Process manipulation
	"kill",
	"killall",
	"pkill",

	// Shell spawning (prevent escapes)
	"bash",
	"sh",
	"zsh",
	"fish",
	"csh",
	"tcsh",
	"ksh",
	"dash",

	// Package managers (can install malware)
	"apt",
	"apt-get",
	"yum",
	"dnf",
	"pacman",
	"brew",
	"pip",
	"npm",
	"yarn",
	"gem",
	"cargo",
	"go",

	// Dangerous utilities
	"wget",
	"curl",
	"ssh",
	"scp",
	"sftp",
	"rsync",
	"ftp",
	"telnet",
	"rsh",

	// System information that could aid attacks
	"passwd",
	"shadow",

	// Compression with overwrite capabilities
	"tar",
	"zip",
	"unzip",
	"gzip",
	"gunzip",

	// Disk utilities
	"mount",
	"umount",
	"losetup",

	// Cron/scheduling (persistence)
	"crontab",
	"at",

	// Init systems
	"systemctl",
	"service",
	"init",
	"reboot",
	"shutdown",
	"halt",
	"poweroff",
}

// DefaultAllowlist contains safe commands commonly used in workflows
var DefaultAllowlist = []string{
	// Safe read-only commands
	"ls",
	"cat",
	"head",
	"tail",
	"wc",
	"sort",
	"uniq",
	"grep",
	"awk",
	"sed",
	"cut",
	"tr",
	"diff",
	"comm",
	"join",
	"paste",
	"column",
	"fold",
	"fmt",
	"nl",
	"pr",
	"tee",
	"xargs",

	// JSON/YAML processing
	"jq",
	"yq",

	// Date/time
	"date",
	"cal",

	// Text processing
	"echo",
	"printf",
	"tac",
	"rev",

	// File info (read-only)
	"file",
	"stat",
	"du",
	"df",
	"which",
	"whereis",
	"type",
	"basename",
	"dirname",
	"realpath",
	"readlink",

	// Environment (read-only)
	"env",
	"printenv",
	"pwd",
	"id",
	"whoami",
	"hostname",
	"uname",

	// Beads workflow tool
	"bd",

	// Safe data transformation
	"base64",
	"md5sum",
	"sha256sum",
	"sha1sum",
	"xxd",
	"od",
	"hexdump",

	// Find (read-only)
	"find",
	"locate",
	"updatedb",

	// Process info (read-only)
	"ps",
	"top",
	"htop",
	"pgrep",

	// Network info (read-only)
	"ping",
	"host",
	"dig",
	"nslookup",
	"ifconfig",
	"ip",
	"netstat",
	"ss",
}

// ToolExecutor handles safe execution of shell tools
type ToolExecutor struct {
	config    *ToolConfig
	allowlist map[string]bool
	denylist  map[string]bool
	mu        sync.RWMutex
	verbose   bool
	debugFunc func(format string, args ...interface{})
}

// MergeToolConfigs merges a global tool config with a step-level config.
// Step-level config takes precedence: if step specifies an allowlist, it's used.
// Global allowlist is additive to the defaults.
// Denylists are always merged (additive).
// Step timeout overrides global timeout if specified.
func MergeToolConfigs(globalConfig *ToolConfig, stepConfig *ToolConfig) *ToolConfig {
	result := &ToolConfig{}

	// Start with global config values if present
	if globalConfig != nil {
		result.Allowlist = append(result.Allowlist, globalConfig.Allowlist...)
		result.Denylist = append(result.Denylist, globalConfig.Denylist...)
		result.Timeout = globalConfig.Timeout
	}

	// Step config overrides/extends
	if stepConfig != nil {
		// If step specifies an allowlist, it takes precedence (replaces global)
		if len(stepConfig.Allowlist) > 0 {
			result.Allowlist = stepConfig.Allowlist
		}
		// Denylist is always additive
		result.Denylist = append(result.Denylist, stepConfig.Denylist...)
		// Step timeout takes precedence if specified
		if stepConfig.Timeout > 0 {
			result.Timeout = stepConfig.Timeout
		}
	}

	return result
}

// NewToolExecutor creates a new tool executor with the given configuration
func NewToolExecutor(config *ToolConfig, verbose bool, debugFunc func(format string, args ...interface{})) *ToolExecutor {
	if config == nil {
		config = &ToolConfig{}
	}

	// Set default timeout if not specified
	if config.Timeout <= 0 {
		config.Timeout = 30
	}

	te := &ToolExecutor{
		config:    config,
		allowlist: make(map[string]bool),
		denylist:  make(map[string]bool),
		verbose:   verbose,
		debugFunc: debugFunc,
	}

	// Build denylist (always include defaults)
	for _, cmd := range DefaultDenylist {
		te.denylist[cmd] = true
	}
	// Add user-specified denylist
	for _, cmd := range config.Denylist {
		te.denylist[cmd] = true
	}

	// Build allowlist
	if len(config.Allowlist) > 0 {
		// Use only user-specified allowlist
		for _, cmd := range config.Allowlist {
			te.allowlist[cmd] = true
		}
	} else {
		// Use default allowlist
		for _, cmd := range DefaultAllowlist {
			te.allowlist[cmd] = true
		}
	}

	return te
}

// IsAllowed checks if a command is allowed to be executed
func (te *ToolExecutor) IsAllowed(command string) (bool, string) {
	te.mu.RLock()
	defer te.mu.RUnlock()

	// Extract the base command (first word before any arguments or pipes)
	baseCmd := te.extractBaseCommand(command)

	// Check denylist first (takes precedence)
	if te.denylist[baseCmd] {
		log.Printf("[SECURITY] Blocked command '%s': command is in the denylist and cannot be executed", baseCmd)
		return false, fmt.Sprintf("command '%s' is in the denylist and cannot be executed", baseCmd)
	}

	// Check allowlist
	if !te.allowlist[baseCmd] {
		return false, fmt.Sprintf("command '%s' is not in the allowlist; add it to your workflow's tool_allowlist to enable", baseCmd)
	}

	return true, ""
}

// extractBaseCommand extracts the base command name from a full command string
func (te *ToolExecutor) extractBaseCommand(command string) string {
	// Trim whitespace
	command = strings.TrimSpace(command)

	// Handle STDIN| prefix (e.g., "STDIN|grep -i 'friend'")
	if strings.HasPrefix(command, "STDIN|") {
		command = strings.TrimPrefix(command, "STDIN|")
		command = strings.TrimSpace(command)
	}

	// Handle STDOUT| prefix for output tools
	if strings.HasPrefix(command, "STDOUT|") {
		command = strings.TrimPrefix(command, "STDOUT|")
		command = strings.TrimSpace(command)
	}

	// Get the first word (the command itself)
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return ""
	}

	baseCmd := parts[0]

	// Handle path-based commands (e.g., /usr/bin/ls -> ls)
	if strings.Contains(baseCmd, "/") {
		pathParts := strings.Split(baseCmd, "/")
		baseCmd = pathParts[len(pathParts)-1]
	}

	return baseCmd
}

// Execute runs a shell command and returns its output
func (te *ToolExecutor) Execute(command string, stdin string) (stdout string, stderr string, err error) {
	// Check if command is allowed
	allowed, reason := te.IsAllowed(command)
	if !allowed {
		return "", "", fmt.Errorf("tool execution denied: %s", reason)
	}

	if te.debugFunc != nil {
		te.debugFunc("Executing tool command: %s", command)
	}

	// Parse the command to handle STDIN| prefix
	actualCommand := command
	useStdin := false

	if strings.HasPrefix(command, "STDIN|") {
		actualCommand = strings.TrimPrefix(command, "STDIN|")
		actualCommand = strings.TrimSpace(actualCommand)
		useStdin = true
	}

	// Create command with shell
	ctx := exec.Command("sh", "-c", actualCommand)

	// Set working directory if specified
	if te.config.WorkingDir != "" {
		ctx.Dir = te.config.WorkingDir
	}

	// Set up stdin if needed
	if useStdin && stdin != "" {
		ctx.Stdin = strings.NewReader(stdin)
	}

	// Capture stdout and stderr
	var stdoutBuf, stderrBuf bytes.Buffer
	ctx.Stdout = &stdoutBuf
	ctx.Stderr = &stderrBuf

	// Create a channel for the result
	done := make(chan error, 1)

	go func() {
		done <- ctx.Run()
	}()

	// Wait for completion or timeout
	timeout := time.Duration(te.config.Timeout) * time.Second
	select {
	case err := <-done:
		stdout = stdoutBuf.String()
		stderr = stderrBuf.String()
		if err != nil {
			return stdout, stderr, fmt.Errorf("command execution failed: %w (stderr: %s)", err, stderr)
		}
		return stdout, stderr, nil
	case <-time.After(timeout):
		// Kill the process
		if ctx.Process != nil {
			_ = ctx.Process.Kill() // Ignore error - process may have already exited
		}
		return "", "", fmt.Errorf("command execution timed out after %d seconds", te.config.Timeout)
	}
}

// ParseToolInput parses a tool input specification and returns the command and any STDIN handling
// Formats supported:
// - "tool: ls -la" - simple command
// - "tool: STDIN|grep -i 'pattern'" - pipe STDIN to command
func ParseToolInput(input string) (command string, usesStdin bool, err error) {
	input = strings.TrimSpace(input)

	// Check for "tool:" prefix
	if !strings.HasPrefix(input, "tool:") {
		return "", false, fmt.Errorf("invalid tool input format: must start with 'tool:'")
	}

	// Extract the command part
	command = strings.TrimPrefix(input, "tool:")
	command = strings.TrimSpace(command)

	if command == "" {
		return "", false, fmt.Errorf("empty tool command")
	}

	// Check if it uses STDIN
	usesStdin = strings.HasPrefix(command, "STDIN|")

	return command, usesStdin, nil
}

// ParseToolOutput parses a tool output specification
// Formats supported:
// - "tool: jq '.data'" - pipe output through command
// - "STDOUT|grep 'pattern'" - pipe STDOUT through command
func ParseToolOutput(output string) (command string, pipesStdout bool, err error) {
	output = strings.TrimSpace(output)

	// Check for "tool:" prefix
	if strings.HasPrefix(output, "tool:") {
		command = strings.TrimPrefix(output, "tool:")
		command = strings.TrimSpace(command)
		if command == "" {
			return "", false, fmt.Errorf("empty tool output command")
		}
		return command, true, nil
	}

	// Check for "STDOUT|" prefix
	if strings.HasPrefix(output, "STDOUT|") {
		command = strings.TrimPrefix(output, "STDOUT|")
		command = strings.TrimSpace(command)
		if command == "" {
			return "", false, fmt.Errorf("empty tool output command")
		}
		return command, true, nil
	}

	return "", false, fmt.Errorf("invalid tool output format: must start with 'tool:' or 'STDOUT|'")
}

// IsToolInput checks if an input string is a tool input specification
func IsToolInput(input string) bool {
	input = strings.TrimSpace(input)
	return strings.HasPrefix(input, "tool:")
}

// IsToolOutput checks if an output string is a tool output specification
func IsToolOutput(output string) bool {
	output = strings.TrimSpace(output)
	return strings.HasPrefix(output, "tool:") || strings.HasPrefix(output, "STDOUT|")
}

// SecurityWarning returns a warning message about tool use
func SecurityWarning() string {
	return `
================================================================================
                            SECURITY WARNING
================================================================================
Tool use in workflows allows execution of shell commands. While comanda enforces
allowlist/denylist controls, this feature carries inherent risks:

1. Commands run with YOUR user permissions
2. Malicious workflows could attempt to exfiltrate data
3. Even "safe" commands can be dangerous with certain arguments

RECOMMENDATIONS:
- Only run workflows from trusted sources
- Review workflow YAML before execution
- Use restrictive allowlists for sensitive environments
- Never add dangerous commands to your allowlist

For production environments, consider running workflows in sandboxed containers.
================================================================================
`
}
