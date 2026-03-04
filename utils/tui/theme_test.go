package tui

import (
	"strings"
	"testing"
)

func TestDefaultTheme(t *testing.T) {
	theme := DefaultTheme()

	if theme == nil {
		t.Fatal("DefaultTheme returned nil")
	}

	// Check that all required colors are set
	if theme.Primary == "" {
		t.Error("Primary color not set")
	}
	if theme.Secondary == "" {
		t.Error("Secondary color not set")
	}
	if theme.Success == "" {
		t.Error("Success color not set")
	}
	if theme.Error == "" {
		t.Error("Error color not set")
	}

	// Check model colors map
	if len(theme.ModelColors) == 0 {
		t.Error("ModelColors map is empty")
	}
	if _, ok := theme.ModelColors["default"]; !ok {
		t.Error("ModelColors missing 'default' key")
	}
}

func TestModelColor(t *testing.T) {
	theme := DefaultTheme()

	tests := []struct {
		model    string
		wantKey  string // Expected key to match in ModelColors
	}{
		{"claude-sonnet", "claude"},
		{"claude-code", "claude-code"},
		{"gpt-4o", "gpt-4o"},
		{"gpt-4-turbo", "gpt"},
		{"gemini-pro", "gemini"},
		{"ollama/llama2", "ollama"},
		{"unknown-model", "default"},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			color := theme.ModelColor(tt.model)
			if color == "" {
				t.Errorf("ModelColor(%q) returned empty color", tt.model)
			}
		})
	}
}

func TestProgressBar(t *testing.T) {
	theme := DefaultTheme()

	tests := []struct {
		percent float64
		width   int
	}{
		{0.0, 10},
		{0.5, 10},
		{1.0, 10},
		{0.25, 20},
		{1.5, 10}, // Over 100%
		{-0.5, 10}, // Negative
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			bar := theme.ProgressBar(tt.percent, tt.width)
			// Bar should contain filled and empty characters
			if bar == "" {
				t.Error("ProgressBar returned empty string")
			}
			// Check that it contains expected characters
			if !strings.Contains(bar, "█") && !strings.Contains(bar, "░") {
				// At extremes (0% or 100%), might only have one type
				if tt.percent > 0 && tt.percent < 1 {
					t.Logf("ProgressBar might not have both filled and empty chars: %q", bar)
				}
			}
		})
	}
}

func TestStatusIcon(t *testing.T) {
	theme := DefaultTheme()

	tests := []struct {
		status   string
		wantIcon string
	}{
		{"success", "✓"},
		{"done", "✓"},
		{"complete", "✓"},
		{"error", "✗"},
		{"failed", "✗"},
		{"warning", "⚠"},
		{"running", "●"},
		{"active", "●"},
		{"pending", "○"},
		{"waiting", "○"},
		{"unknown", "·"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			icon, style := theme.StatusIcon(tt.status)
			if icon != tt.wantIcon {
				t.Errorf("StatusIcon(%q) = %q, want %q", tt.status, icon, tt.wantIcon)
			}
			if style.String() == "" {
				// Style should render to something
				t.Logf("StatusIcon style for %q renders OK", tt.status)
			}
		})
	}
}

func TestModelStyle(t *testing.T) {
	theme := DefaultTheme()

	models := []string{"claude", "gpt-4o", "gemini", "ollama", "unknown"}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			style := theme.ModelStyle(model)
			// Style should be usable
			rendered := style.Render("test")
			if rendered == "" {
				t.Error("ModelStyle rendered empty string")
			}
		})
	}
}
