package processor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePathAtParseTime(t *testing.T) {
	homeDir, _ := os.UserHomeDir()

	// Check if cwd is available - some CI environments have issues with this
	cwd, cwdErr := os.Getwd()
	cwdAvailable := cwdErr == nil && cwd != ""

	t.Run("dot resolves to absolute path", func(t *testing.T) {
		if !cwdAvailable {
			t.Skip("skipping: cwd not available in this environment")
		}
		result := resolvePathAtParseTime(".")
		if !filepath.IsAbs(result) {
			t.Errorf("resolvePathAtParseTime(\".\") = %q, expected absolute path", result)
		}
	})

	t.Run("tilde resolves to home", func(t *testing.T) {
		result := resolvePathAtParseTime("~")
		if result != homeDir {
			t.Errorf("resolvePathAtParseTime(\"~\") = %q, want %q", result, homeDir)
		}
	})

	t.Run("tilde path resolves", func(t *testing.T) {
		result := resolvePathAtParseTime("~/test/path")
		expected := filepath.Join(homeDir, "test/path")
		if result != expected {
			t.Errorf("resolvePathAtParseTime(\"~/test/path\") = %q, want %q", result, expected)
		}
	})

	t.Run("absolute path unchanged", func(t *testing.T) {
		result := resolvePathAtParseTime("/absolute/path")
		if result != "/absolute/path" {
			t.Errorf("resolvePathAtParseTime(\"/absolute/path\") = %q, want \"/absolute/path\"", result)
		}
	})

	t.Run("relative path becomes absolute", func(t *testing.T) {
		if !cwdAvailable {
			t.Skip("skipping: cwd not available in this environment")
		}
		result := resolvePathAtParseTime("relative/path")
		if !filepath.IsAbs(result) {
			t.Errorf("resolvePathAtParseTime(\"relative/path\") = %q, expected absolute path", result)
		}
		if filepath.Base(filepath.Dir(result)) != "relative" || filepath.Base(result) != "path" {
			t.Errorf("resolvePathAtParseTime(\"relative/path\") = %q, expected path ending in relative/path", result)
		}
	})
}
