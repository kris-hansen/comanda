package models

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractKimiCodeResult(t *testing.T) {
	// Realistic stream from `kimi -p ... --output-format stream-json` with a tool
	// call round: assistant tool_calls event, tool result, final assistant text,
	// trailing session metadata.
	stream := `{"role":"assistant","tool_calls":[{"type":"function","id":"tool_123","function":{"name":"Read","arguments":"{\"path\":\"/tmp/x.txt\"}"}}]}
{"role":"tool","tool_call_id":"tool_123","content":"file contents"}
{"role":"assistant","content":"The file says hello."}
{"role":"meta","type":"session.resume_hint","session_id":"session_abc","content":"To resume this session: kimi -r session_abc"}`

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "extracts final assistant message after tool calls",
			in:   stream,
			want: "The file says hello.",
		},
		{
			name: "extracts simple assistant reply",
			in:   `{"role":"assistant","content":"hello world"}`,
			want: "hello world",
		},
		{
			name: "ignores meta events",
			in: `{"role":"assistant","content":"real answer"}
{"role":"meta","type":"session.resume_hint","content":"To resume this session: kimi -r session_x"}`,
			want: "real answer",
		},
		{
			name: "passes through plain text",
			in:   "just a plain reply",
			want: "just a plain reply",
		},
		{
			name: "passes through empty string",
			in:   "",
			want: "",
		},
		{
			name: "skips malformed lines",
			in: `{"role":"assistant","content":
{"role":"assistant","content":"good"}`,
			want: "good",
		},
		{
			name: "skips assistant events without content (tool calls only)",
			in:   `{"role":"assistant","tool_calls":[{"type":"function","id":"t1","function":{"name":"Bash","arguments":"{}"}}]}`,
			want: `{"role":"assistant","tool_calls":[{"type":"function","id":"t1","function":{"name":"Bash","arguments":"{}"}}]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractKimiCodeResult(tt.in)
			if got != tt.want {
				t.Errorf("extractKimiCodeResult(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestKimiCodeProviderName(t *testing.T) {
	provider := NewKimiCodeProvider()
	if provider.Name() != "kimi-code" {
		t.Errorf("Expected provider name 'kimi-code', got '%s'", provider.Name())
	}
}

func TestKimiCodeSupportsModel(t *testing.T) {
	provider := NewKimiCodeProvider()

	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{"base kimi-code", "kimi-code", true},
		{"kimi-code with alias", "kimi-code-fast", true},
		{"uppercase", "KIMI-CODE", true},
		{"mixed case", "Kimi-Code", true},
		{"kimi-code with custom alias", "kimi-code-my-alias", true},
		{"moonshot api model", "moonshot-v1-8k", false},
		{"openai model", "gpt-4o", false},
		{"empty string", "", false},
		{"partial match", "kimi", false},
		{"no dash suffix", "kimi-codefast", false},
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

func TestKimiCodeGetModelFlag(t *testing.T) {
	provider := NewKimiCodeProvider()

	tests := []struct {
		name     string
		model    string
		expected string
	}{
		{"base kimi-code", "kimi-code", ""},
		{"alias variant", "kimi-code-fast", "fast"},
		{"multi-word alias", "kimi-code-my-alias", "my-alias"},
		{"alias case preserved", "kimi-code-MyAlias", "MyAlias"},
		{"uppercase prefix, alias case preserved", "KIMI-CODE-Fast", "Fast"},
		{"not a kimi-code model", "gpt-4", ""},
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

func TestKimiCodeBuildArgs(t *testing.T) {
	provider := NewKimiCodeProvider()

	tests := []struct {
		name        string
		model       string
		contains    []string
		notContains []string
	}{
		{
			name:        "base model",
			model:       "kimi-code",
			contains:    []string{"--prompt", "test prompt", "--output-format", "stream-json"},
			notContains: []string{"--model"},
		},
		{
			name:        "alias variant",
			model:       "kimi-code-fast",
			contains:    []string{"--prompt", "test prompt", "--output-format", "stream-json", "--model", "fast"},
			notContains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := provider.buildArgs(tt.model, "test prompt")
			for _, expected := range tt.contains {
				found := false
				for _, arg := range args {
					if arg == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("buildArgs(%q) missing expected arg %q, got: %v", tt.model, expected, args)
				}
			}
			for _, notExpected := range tt.notContains {
				for _, arg := range args {
					if arg == notExpected {
						t.Errorf("buildArgs(%q) should not contain %q, got: %v", tt.model, notExpected, args)
					}
				}
			}
		})
	}
}

// TestKimiCodeSendPromptArgvTransport verifies the prompt travels as the argv value
// after --prompt (kimi has no stdin prompt mode) and that even a large prompt
// survives, using a fake kimi binary that echoes the prompt back in a stream-json
// assistant event.
func TestKimiCodeSendPromptArgvTransport(t *testing.T) {
	dir := t.TempDir()
	binaryPath := filepath.Join(dir, "fake-kimi")
	script := "#!/bin/sh\n" +
		"[ \"$1\" = \"--prompt\" ] || exit 2\n" +
		"printf '%s' \"{\\\"role\\\":\\\"assistant\\\",\\\"content\\\":\\\"$2\\\"}\"\n"
	if err := os.WriteFile(binaryPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake kimi: %v", err)
	}

	provider := NewKimiCodeProvider()
	provider.binaryPath = binaryPath
	// 100 KB is large enough to prove argv transport while staying under Linux's
	// 128 KiB per-argument limit (MAX_ARG_STRLEN), which the test must respect
	// to run cross-platform.
	prompt := strings.Repeat("x", 100_000)

	got, err := provider.SendPrompt("kimi-code", prompt)
	if err != nil {
		t.Fatalf("SendPrompt() error: %v", err)
	}
	if got != prompt {
		t.Fatalf("SendPrompt() returned %d bytes, want %d", len(got), len(prompt))
	}
}

// TestKimiCodeSendPromptWithFileReferencesPath verifies that file context is passed
// to kimi as a path to read (keeping contents off argv), not inlined into the
// prompt, and that kimi runs in the file's directory.
func TestKimiCodeSendPromptWithFileReferencesPath(t *testing.T) {
	dir := t.TempDir()
	capturePath := filepath.Join(dir, "capture.txt")

	binaryPath := filepath.Join(dir, "fake-kimi")
	script := "#!/bin/sh\n" +
		"printf '%s\\n' \"$2\" > \"$FAKE_KIMI_CAPTURE\"\n" +
		"pwd >> \"$FAKE_KIMI_CAPTURE\"\n" +
		"printf '%s' '{\"role\":\"assistant\",\"content\":\"ok\"}'\n"
	if err := os.WriteFile(binaryPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake kimi: %v", err)
	}
	t.Setenv("FAKE_KIMI_CAPTURE", capturePath)

	inputFile := filepath.Join(dir, "input.go")
	if err := os.WriteFile(inputFile, []byte("secret-content-xyz"), 0o644); err != nil {
		t.Fatalf("write input file: %v", err)
	}

	provider := NewKimiCodeProvider()
	provider.binaryPath = binaryPath

	got, err := provider.SendPromptWithFile("kimi-code", "summarize it", FileInput{Path: inputFile})
	if err != nil {
		t.Fatalf("SendPromptWithFile() error: %v", err)
	}
	if got != "ok" {
		t.Fatalf("SendPromptWithFile() = %q, want %q", got, "ok")
	}

	capture, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("read capture: %v", err)
	}
	if !strings.Contains(string(capture), inputFile) {
		t.Errorf("prompt should reference the file by path %q, got: %q", inputFile, string(capture))
	}
	if strings.Contains(string(capture), "secret-content-xyz") {
		t.Errorf("prompt should not inline the file contents, got: %q", string(capture))
	}
	if !strings.Contains(string(capture), dir) {
		t.Errorf("kimi should run in the file's directory %q, capture: %q", dir, string(capture))
	}
}

func TestKimiCodeVerifyKimiBinary(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name        string
		script      string
		expectError bool
	}{
		{
			name:        "kimi-code bare semver",
			script:      "#!/bin/sh\necho '0.27.0'\n",
			expectError: false,
		},
		{
			name:        "legacy kimi-cli click-style version",
			script:      "#!/bin/sh\necho 'kimi, version 0.5.1'\n",
			expectError: true,
		},
		{
			name:        "version flag unsupported",
			script:      "#!/bin/sh\nexit 1\n",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			binaryPath := filepath.Join(dir, strings.ReplaceAll(tt.name, " ", "-"))
			if err := os.WriteFile(binaryPath, []byte(tt.script), 0o755); err != nil {
				t.Fatalf("write fake binary: %v", err)
			}
			err := verifyKimiBinary(binaryPath)
			if tt.expectError && err == nil {
				t.Errorf("verifyKimiBinary() expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("verifyKimiBinary() unexpected error: %v", err)
			}
			if tt.expectError && err != nil && strings.Contains(tt.name, "legacy") && !strings.Contains(err.Error(), "legacy") {
				t.Errorf("verifyKimiBinary() error should mention the legacy kimi-cli, got: %v", err)
			}
		})
	}
}

func TestKimiCodeSetVerbose(t *testing.T) {
	provider := NewKimiCodeProvider()

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

func TestKimiCodeValidateModel(t *testing.T) {
	provider := NewKimiCodeProvider()

	tests := []struct {
		name     string
		model    string
		expected bool
	}{
		{"kimi-code", "kimi-code", true},
		{"kimi-code-fast", "kimi-code-fast", true},
		{"moonshot-v1-8k (not supported)", "moonshot-v1-8k", false},
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

func TestIsKimiCodeAvailable(t *testing.T) {
	// This test just ensures the function doesn't panic
	// The actual result depends on whether kimi is installed
	available := IsKimiCodeAvailable()
	t.Logf("Kimi Code binary available: %v", available)
}
