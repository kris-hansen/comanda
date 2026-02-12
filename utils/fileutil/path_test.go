package fileutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandPath(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home dir: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get cwd: %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
		wantErr  bool
	}{
		{
			name:     "empty path",
			input:    "",
			expected: "",
			wantErr:  false,
		},
		{
			name:     "tilde only",
			input:    "~",
			expected: homeDir,
			wantErr:  false,
		},
		{
			name:     "tilde with subpath",
			input:    "~/Documents",
			expected: filepath.Join(homeDir, "Documents"),
			wantErr:  false,
		},
		{
			name:     "tilde with nested path",
			input:    "~/projects/comanda/src",
			expected: filepath.Join(homeDir, "projects/comanda/src"),
			wantErr:  false,
		},
		{
			name:     "absolute path unchanged",
			input:    "/usr/local/bin",
			expected: "/usr/local/bin",
			wantErr:  false,
		},
		{
			name:     "relative path resolved to absolute",
			input:    "./src/../lib",
			expected: filepath.Join(cwd, "lib"),
			wantErr:  false,
		},
		{
			name:     "dot path resolved to cwd",
			input:    ".",
			expected: cwd,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExpandPath(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ExpandPath() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("ExpandPath() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestExpandPathWithEnvVar(t *testing.T) {
	// Set a test environment variable
	testPath := "/test/path"
	os.Setenv("TEST_COMANDA_PATH", testPath)
	defer os.Unsetenv("TEST_COMANDA_PATH")

	got, err := ExpandPath("$TEST_COMANDA_PATH/subdir")
	if err != nil {
		t.Fatalf("ExpandPath() error = %v", err)
	}

	expected := filepath.Join(testPath, "subdir")
	if got != expected {
		t.Errorf("ExpandPath() = %v, want %v", got, expected)
	}
}

func TestExpandPaths(t *testing.T) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Failed to get home dir: %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get cwd: %v", err)
	}

	input := []string{"~/foo", "~/bar", "."}
	expected := []string{
		filepath.Join(homeDir, "foo"),
		filepath.Join(homeDir, "bar"),
		cwd,
	}

	got, err := ExpandPaths(input)
	if err != nil {
		t.Fatalf("ExpandPaths() error = %v", err)
	}

	if len(got) != len(expected) {
		t.Fatalf("ExpandPaths() returned %d paths, want %d", len(got), len(expected))
	}

	for i := range got {
		if got[i] != expected[i] {
			t.Errorf("ExpandPaths()[%d] = %v, want %v", i, got[i], expected[i])
		}
	}
}
