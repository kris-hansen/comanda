package models

import (
	"os"
	"strings"
	"testing"
)

func TestClaudeCodeProviderName(t *testing.T) {
	provider := NewClaudeCodeProvider()
	if provider.Name() != "claude-code" {
		t.Errorf("Expected provider name 'claude-code', got '%s'", provider.Name())
	}
}

func TestClaudeCodeSupportsModel(t *testing.T) {
	provider := NewClaudeCodeProvider()

	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{"base claude-code", "claude-code", true},
		{"claude-code-opus", "claude-code-opus", true},
		{"claude-code-sonnet", "claude-code-sonnet", true},
		{"claude-code-haiku", "claude-code-haiku", true},
		{"uppercase", "CLAUDE-CODE", true},
		{"mixed case", "Claude-Code", true},
		{"claude-code with custom model", "claude-code-custom", true},
		{"regular claude model", "claude-4-sonnet", false},
		{"openai model", "gpt-4o", false},
		{"empty string", "", false},
		{"partial match", "claude", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.SupportsModel(tt.model)
			if result != tt.expected {
				t.Errorf("SupportsModel(%q) = %v, want %v", tt.model, result, tt.expected)
			}
		})
	}
}

func TestClaudeCodeBuildArgs(t *testing.T) {
	provider := NewClaudeCodeProvider()

	tests := []struct {
		name     string
		model    string
		prompt   string
		contains []string
	}{
		{
			name:     "base model",
			model:    "claude-code",
			prompt:   "hello",
			contains: []string{"--print", "-p", "hello"},
		},
		{
			name:     "opus variant",
			model:    "claude-code-opus",
			prompt:   "test",
			contains: []string{"--print", "--model", "claude-opus-4-5-20251101", "-p", "test"},
		},
		{
			name:     "sonnet variant",
			model:    "claude-code-sonnet",
			prompt:   "test",
			contains: []string{"--print", "--model", "claude-sonnet-4-5-20250929", "-p", "test"},
		},
		{
			name:     "haiku variant",
			model:    "claude-code-haiku",
			prompt:   "test",
			contains: []string{"--print", "--model", "claude-haiku-4-5-20251001", "-p", "test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := provider.buildArgs(tt.model, tt.prompt, "")
			argsStr := ""
			for _, arg := range args {
				argsStr += arg + " "
			}
			for _, expected := range tt.contains {
				found := false
				for _, arg := range args {
					if arg == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("buildArgs(%q, %q) missing expected arg %q, got: %v", tt.model, tt.prompt, expected, args)
				}
			}
		})
	}
}

func TestClaudeCodeSetVerbose(t *testing.T) {
	provider := NewClaudeCodeProvider()

	// Test setting verbose
	provider.SetVerbose(true)
	if !provider.verbose {
		t.Error("Expected verbose to be true")
	}

	provider.SetVerbose(false)
	if provider.verbose {
		t.Error("Expected verbose to be false")
	}
}

func TestClaudeCodeValidateModel(t *testing.T) {
	provider := NewClaudeCodeProvider()

	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{"claude-code", "claude-code", true},
		{"claude-code-opus", "claude-code-opus", true},
		{"claude-sonnet (not supported)", "claude-sonnet", false},
		{"gpt-4 (not supported)", "gpt-4", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.ValidateModel(tt.model)
			if result != tt.expected {
				t.Errorf("ValidateModel(%q) = %v, want %v", tt.model, result, tt.expected)
			}
		})
	}
}

func TestIsClaudeCodeAvailable(t *testing.T) {
	// This test just ensures the function doesn't panic
	// The actual result depends on whether claude is installed
	available := IsClaudeCodeAvailable()
	t.Logf("Claude Code binary available: %v", available)
}

func TestClaudeCodeGetModelFlag(t *testing.T) {
	provider := NewClaudeCodeProvider()

	tests := []struct {
		name     string
		model    string
		expected string
	}{
		{"base claude-code", "claude-code", ""},
		{"opus", "claude-code-opus", "claude-opus-4-5-20251101"},
		{"sonnet", "claude-code-sonnet", "claude-sonnet-4-5-20250929"},
		{"haiku", "claude-code-haiku", "claude-haiku-4-5-20251001"},
		{"custom full model name", "claude-code-custom-model-123", "custom-model-123"},
		{"single word variant", "claude-code-test", ""},
		{"not claude-code model", "gpt-4", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := provider.getModelFlag(tt.model)
			if result != tt.expected {
				t.Errorf("getModelFlag(%q) = %q, want %q", tt.model, result, tt.expected)
			}
		})
	}
}

func TestClaudeCodeBuildArgsAgentic(t *testing.T) {
	provider := NewClaudeCodeProvider()

	tests := []struct {
		name            string
		model           string
		prompt          string
		allowedPaths    []string
		tools           []string
		contains        []string
		notContains     []string
		checkAbsPaths   bool // Check that paths are resolved to absolute
		expectedPathCnt int  // Expected number of --add-dir flags
	}{
		{
			name:            "basic agentic call",
			model:           "claude-code-sonnet",
			prompt:          "explore",
			allowedPaths:    []string{"./"},
			tools:           nil,
			contains:        []string{"--add-dir", "--dangerously-skip-permissions", "--model", "claude-sonnet-4-5-20250929", "-p", "explore"},
			notContains:     []string{"--print"},
			checkAbsPaths:   true,
			expectedPathCnt: 1,
		},
		{
			name:            "with tool restrictions",
			model:           "claude-code",
			prompt:          "test",
			allowedPaths:    []string{"/tmp"},
			tools:           []string{"Read", "Bash"},
			contains:        []string{"--add-dir", "/tmp", "--dangerously-skip-permissions", "--tools", "Read,Bash", "-p", "test"},
			notContains:     []string{"--print", "--model"},
			checkAbsPaths:   false, // /tmp is already absolute
			expectedPathCnt: 1,
		},
		{
			name:            "multiple paths",
			model:           "claude-code-opus",
			prompt:          "analyze",
			allowedPaths:    []string{"./src", "./tests"},
			tools:           nil,
			contains:        []string{"--add-dir", "--dangerously-skip-permissions", "-p", "analyze"},
			notContains:     []string{"--print"},
			checkAbsPaths:   true,
			expectedPathCnt: 2,
		},
		{
			name:            "no allowed paths skips permission mode",
			model:           "claude-code",
			prompt:          "query",
			allowedPaths:    []string{},
			tools:           nil,
			contains:        []string{"-p", "query"},
			notContains:     []string{"--print", "--add-dir", "--dangerously-skip-permissions"},
			checkAbsPaths:   false,
			expectedPathCnt: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := provider.buildArgsAgentic(tt.model, tt.prompt, tt.allowedPaths, tt.tools)

			for _, expected := range tt.contains {
				found := false
				for _, arg := range args {
					if arg == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("buildArgsAgentic() missing expected arg %q, got: %v", expected, args)
				}
			}

			for _, notExpected := range tt.notContains {
				for _, arg := range args {
					if arg == notExpected {
						t.Errorf("buildArgsAgentic() should not contain %q, got: %v", notExpected, args)
					}
				}
			}

			// Check that paths are resolved to absolute paths
			if tt.checkAbsPaths {
				addDirCount := 0
				for i, arg := range args {
					if arg == "--add-dir" && i+1 < len(args) {
						addDirCount++
						path := args[i+1]
						if !strings.HasPrefix(path, "/") {
							t.Errorf("buildArgsAgentic() path should be absolute, got: %q", path)
						}
					}
				}
				if addDirCount != tt.expectedPathCnt {
					t.Errorf("buildArgsAgentic() expected %d --add-dir flags, got %d", tt.expectedPathCnt, addDirCount)
				}
			}
		})
	}
}

func TestClaudeCodeSendPromptAgenticPathValidation(t *testing.T) {
	provider := NewClaudeCodeProvider()

	tests := []struct {
		name          string
		allowedPaths  []string
		expectError   bool
		errorContains string
	}{
		{
			name:         "valid path",
			allowedPaths: []string{"."},
			expectError:  false,
		},
		{
			name:          "invalid path",
			allowedPaths:  []string{"/nonexistent/path/that/does/not/exist"},
			expectError:   true,
			errorContains: "allowed_path does not exist",
		},
		{
			name:          "mixed valid and invalid",
			allowedPaths:  []string{".", "/nonexistent/path"},
			expectError:   true,
			errorContains: "allowed_path does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := provider.SendPromptAgentic("claude-code", "test", tt.allowedPaths, nil, "")
			if tt.expectError {
				if err == nil {
					t.Errorf("SendPromptAgentic() expected error, got nil")
				} else if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("SendPromptAgentic() error = %v, want error containing %q", err, tt.errorContains)
				}
			}
			// Note: valid paths may still fail if claude binary not found, that's ok
		})
	}
}

func TestResolvePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantAbs  bool
		wantHome bool // Should start with home directory
	}{
		{
			name:     "tilde expansion",
			input:    "~/test",
			wantAbs:  true,
			wantHome: true,
		},
		{
			name:     "tilde only",
			input:    "~",
			wantAbs:  true,
			wantHome: true,
		},
		{
			name:     "relative path",
			input:    "./src",
			wantAbs:  true,
			wantHome: false,
		},
		{
			name:     "dot only",
			input:    ".",
			wantAbs:  true,
			wantHome: false,
		},
		{
			name:     "absolute path",
			input:    "/tmp/test",
			wantAbs:  true,
			wantHome: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolvePath(tt.input)

			if tt.wantAbs && !strings.HasPrefix(result, "/") {
				t.Errorf("resolvePath(%q) = %q, want absolute path", tt.input, result)
			}

			if tt.wantHome {
				home, _ := os.UserHomeDir()
				if !strings.HasPrefix(result, home) {
					t.Errorf("resolvePath(%q) = %q, want path starting with %q", tt.input, result, home)
				}
			}
		})
	}
}
