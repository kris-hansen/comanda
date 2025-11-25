package processor

import (
	"strings"
	"testing"
)

func TestNewToolExecutor(t *testing.T) {
	// Test with nil config (should use defaults)
	executor := NewToolExecutor(nil, false, nil)
	if executor == nil {
		t.Fatal("expected non-nil executor")
	}

	// Should have default timeout
	if executor.config.Timeout != 30 {
		t.Errorf("expected default timeout of 30, got %d", executor.config.Timeout)
	}

	// Test with custom config
	customConfig := &ToolConfig{
		Allowlist: []string{"custom-tool"},
		Denylist:  []string{"forbidden-tool"},
		Timeout:   60,
	}
	executor2 := NewToolExecutor(customConfig, false, nil)
	if executor2.config.Timeout != 60 {
		t.Errorf("expected timeout of 60, got %d", executor2.config.Timeout)
	}
}

func TestToolExecutorIsAllowed(t *testing.T) {
	tests := []struct {
		name      string
		config    *ToolConfig
		command   string
		allowed   bool
		errSubstr string
	}{
		{
			name:    "default allowlist - ls allowed",
			config:  nil,
			command: "ls -la",
			allowed: true,
		},
		{
			name:    "default allowlist - cat allowed",
			config:  nil,
			command: "cat file.txt",
			allowed: true,
		},
		{
			name:      "default denylist - rm denied",
			config:    nil,
			command:   "rm -rf /",
			allowed:   false,
			errSubstr: "denylist",
		},
		{
			name:      "default denylist - sudo denied",
			config:    nil,
			command:   "sudo ls",
			allowed:   false,
			errSubstr: "denylist",
		},
		{
			name:      "default denylist - bash denied",
			config:    nil,
			command:   "bash -c 'echo test'",
			allowed:   false,
			errSubstr: "denylist",
		},
		{
			name:    "custom allowlist - custom tool allowed",
			config:  &ToolConfig{Allowlist: []string{"my-tool"}},
			command: "my-tool --arg",
			allowed: true,
		},
		{
			name:      "custom allowlist - ls not in custom allowlist",
			config:    &ToolConfig{Allowlist: []string{"my-tool"}},
			command:   "ls -la",
			allowed:   false,
			errSubstr: "not in the allowlist",
		},
		{
			name:    "bd tool allowed by default",
			config:  nil,
			command: "bd list",
			allowed: true,
		},
		{
			name:    "jq allowed by default",
			config:  nil,
			command: "jq '.data'",
			allowed: true,
		},
		{
			name:    "grep allowed by default",
			config:  nil,
			command: "grep -i pattern",
			allowed: true,
		},
		{
			name:    "STDIN prefix handled correctly",
			config:  nil,
			command: "STDIN|grep pattern",
			allowed: true,
		},
		{
			name:    "STDOUT prefix handled correctly",
			config:  nil,
			command: "STDOUT|jq '.'",
			allowed: true,
		},
		{
			name:      "STDIN prefix with denied command",
			config:    nil,
			command:   "STDIN|rm file",
			allowed:   false,
			errSubstr: "denylist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := NewToolExecutor(tt.config, false, nil)
			allowed, reason := executor.IsAllowed(tt.command)

			if allowed != tt.allowed {
				t.Errorf("expected allowed=%v, got allowed=%v (reason: %s)", tt.allowed, allowed, reason)
			}

			if !allowed && tt.errSubstr != "" {
				if !strings.Contains(reason, tt.errSubstr) {
					t.Errorf("expected reason to contain %q, got %q", tt.errSubstr, reason)
				}
			}
		})
	}
}

func TestToolExecutorExtractBaseCommand(t *testing.T) {
	executor := NewToolExecutor(nil, false, nil)

	tests := []struct {
		input    string
		expected string
	}{
		{"ls -la", "ls"},
		{"cat file.txt", "cat"},
		{"/usr/bin/ls -la", "ls"},
		{"STDIN|grep pattern", "grep"},
		{"STDOUT|jq '.'", "jq"},
		{"  ls  ", "ls"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := executor.extractBaseCommand(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestToolExecutorExecute(t *testing.T) {
	executor := NewToolExecutor(nil, false, nil)

	// Test simple echo command
	stdout, stderr, err := executor.Execute("echo hello", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(stdout) != "hello" {
		t.Errorf("expected 'hello', got %q", stdout)
	}
	// Note: stderr may contain shell initialization warnings in some environments
	// so we only log it rather than fail the test
	if stderr != "" {
		t.Logf("stderr (may be expected in some environments): %q", stderr)
	}

	// Test command with STDIN
	stdout, _, err = executor.Execute("STDIN|grep test", "line1\ntest line\nline3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(stdout) != "test line" {
		t.Errorf("expected 'test line', got %q", stdout)
	}

	// Test denied command
	_, _, err = executor.Execute("rm -rf /", "")
	if err == nil {
		t.Error("expected error for denied command")
	}
	if !strings.Contains(err.Error(), "denied") {
		t.Errorf("expected denial error, got: %v", err)
	}
}

func TestParseToolInput(t *testing.T) {
	tests := []struct {
		input     string
		command   string
		usesStdin bool
		hasError  bool
	}{
		{"tool: ls -la", "ls -la", false, false},
		{"tool: STDIN|grep pattern", "STDIN|grep pattern", true, false},
		{"tool:echo test", "echo test", false, false},
		{"tool:   spaced   ", "spaced", false, false},
		{"not a tool", "", false, true},
		{"tool:", "", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			command, usesStdin, err := ParseToolInput(tt.input)

			if tt.hasError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if command != tt.command {
				t.Errorf("expected command %q, got %q", tt.command, command)
			}

			if usesStdin != tt.usesStdin {
				t.Errorf("expected usesStdin=%v, got %v", tt.usesStdin, usesStdin)
			}
		})
	}
}

func TestParseToolOutput(t *testing.T) {
	tests := []struct {
		output      string
		command     string
		pipesStdout bool
		hasError    bool
	}{
		{"tool: jq '.'", "jq '.'", true, false},
		{"STDOUT|grep pattern", "grep pattern", true, false},
		{"tool:awk '{print $1}'", "awk '{print $1}'", true, false},
		{"not a tool output", "", false, true},
		{"tool:", "", false, true},
		{"STDOUT|", "", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.output, func(t *testing.T) {
			command, pipesStdout, err := ParseToolOutput(tt.output)

			if tt.hasError {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if command != tt.command {
				t.Errorf("expected command %q, got %q", tt.command, command)
			}

			if pipesStdout != tt.pipesStdout {
				t.Errorf("expected pipesStdout=%v, got %v", tt.pipesStdout, pipesStdout)
			}
		})
	}
}

func TestIsToolInput(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"tool: ls", true},
		{"tool:ls", true},
		{"  tool: ls", true},
		{"STDIN", false},
		{"file.txt", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := IsToolInput(tt.input)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsToolOutput(t *testing.T) {
	tests := []struct {
		output   string
		expected bool
	}{
		{"tool: jq", true},
		{"STDOUT|grep", true},
		{"  tool: jq", true},
		{"STDOUT", false},
		{"file.txt", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.output, func(t *testing.T) {
			result := IsToolOutput(tt.output)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestDefaultDenylist(t *testing.T) {
	// Ensure critical dangerous commands are in the denylist
	dangerousCommands := []string{
		"rm", "sudo", "bash", "sh", "chmod", "curl", "wget",
	}

	for _, cmd := range dangerousCommands {
		found := false
		for _, denied := range DefaultDenylist {
			if denied == cmd {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q to be in DefaultDenylist", cmd)
		}
	}
}

func TestDefaultAllowlist(t *testing.T) {
	// Ensure safe commands are in the allowlist
	safeCommands := []string{
		"ls", "cat", "grep", "jq", "bd", "echo", "date",
	}

	for _, cmd := range safeCommands {
		found := false
		for _, allowed := range DefaultAllowlist {
			if allowed == cmd {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q to be in DefaultAllowlist", cmd)
		}
	}
}

func TestSecurityWarning(t *testing.T) {
	warning := SecurityWarning()
	if warning == "" {
		t.Error("expected non-empty security warning")
	}
	if !strings.Contains(warning, "SECURITY WARNING") {
		t.Error("expected warning to contain 'SECURITY WARNING'")
	}
}
