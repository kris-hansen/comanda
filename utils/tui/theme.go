// Package tui provides terminal UI components for comanda using bubbletea and lipgloss
package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Theme defines the color scheme and styles for the TUI
type Theme struct {
	// Base colors
	Primary   lipgloss.Color
	Secondary lipgloss.Color
	Accent    lipgloss.Color
	Muted     lipgloss.Color
	Success   lipgloss.Color
	Warning   lipgloss.Color
	Error     lipgloss.Color

	// Background colors
	BgPrimary   lipgloss.Color
	BgSecondary lipgloss.Color

	// Model colors (for distinguishing different AI providers)
	ModelColors map[string]lipgloss.Color

	// Pre-built styles
	Title       lipgloss.Style
	Subtitle    lipgloss.Style
	Body        lipgloss.Style
	MutedText   lipgloss.Style
	SuccessText lipgloss.Style
	WarningText lipgloss.Style
	ErrorText   lipgloss.Style

	// Box styles
	BoxNormal    lipgloss.Style
	BoxHighlight lipgloss.Style
	BoxError     lipgloss.Style
	BoxSuccess   lipgloss.Style

	// Progress bar colors
	ProgressFull  lipgloss.Color
	ProgressEmpty lipgloss.Color
}

// DefaultTheme returns the default comanda theme
func DefaultTheme() *Theme {
	t := &Theme{
		// Base colors - vibrant but professional
		Primary:   lipgloss.Color("#7C3AED"), // Purple
		Secondary: lipgloss.Color("#06B6D4"), // Cyan
		Accent:    lipgloss.Color("#F59E0B"), // Amber
		Muted:     lipgloss.Color("#6B7280"), // Gray
		Success:   lipgloss.Color("#10B981"), // Emerald
		Warning:   lipgloss.Color("#F59E0B"), // Amber
		Error:     lipgloss.Color("#EF4444"), // Red

		// Backgrounds
		BgPrimary:   lipgloss.Color("#1F2937"), // Dark gray
		BgSecondary: lipgloss.Color("#374151"), // Lighter gray

		// Model-specific colors
		ModelColors: map[string]lipgloss.Color{
			"claude":      lipgloss.Color("#D97706"), // Orange (Anthropic)
			"claude-code": lipgloss.Color("#D97706"), // Orange
			"gpt":         lipgloss.Color("#10B981"), // Green (OpenAI)
			"gpt-4":       lipgloss.Color("#10B981"),
			"gpt-4o":      lipgloss.Color("#10B981"),
			"o1":          lipgloss.Color("#10B981"),
			"gemini":      lipgloss.Color("#4285F4"), // Blue (Google)
			"ollama":      lipgloss.Color("#9333EA"), // Purple (local)
			"codex":       lipgloss.Color("#10B981"), // Green
			"default":     lipgloss.Color("#6B7280"), // Gray
		},

		// Progress
		ProgressFull:  lipgloss.Color("#7C3AED"),
		ProgressEmpty: lipgloss.Color("#374151"),
	}

	// Build styles
	t.Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(t.Primary).
		MarginBottom(1)

	t.Subtitle = lipgloss.NewStyle().
		Foreground(t.Secondary)

	t.Body = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#E5E7EB"))

	t.MutedText = lipgloss.NewStyle().
		Foreground(t.Muted)

	t.SuccessText = lipgloss.NewStyle().
		Foreground(t.Success)

	t.WarningText = lipgloss.NewStyle().
		Foreground(t.Warning)

	t.ErrorText = lipgloss.NewStyle().
		Foreground(t.Error)

	// Box styles
	t.BoxNormal = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Muted).
		Padding(0, 1)

	t.BoxHighlight = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Primary).
		Padding(0, 1)

	t.BoxError = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Error).
		Padding(0, 1)

	t.BoxSuccess = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(t.Success).
		Padding(0, 1)

	return t
}

// ModelColor returns the color for a given model name
func (t *Theme) ModelColor(model string) lipgloss.Color {
	// Try exact match first
	if color, ok := t.ModelColors[model]; ok {
		return color
	}

	// Try prefix matching
	for prefix, color := range t.ModelColors {
		if len(model) >= len(prefix) && model[:len(prefix)] == prefix {
			return color
		}
	}

	return t.ModelColors["default"]
}

// ModelStyle returns a style configured for a given model
func (t *Theme) ModelStyle(model string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(t.ModelColor(model))
}

// ProgressBar creates a progress bar string
func (t *Theme) ProgressBar(percent float64, width int) string {
	filled := int(percent * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}

	empty := width - filled

	filledStyle := lipgloss.NewStyle().Foreground(t.ProgressFull)
	emptyStyle := lipgloss.NewStyle().Foreground(t.ProgressEmpty)

	bar := ""
	for i := 0; i < filled; i++ {
		bar += filledStyle.Render("█")
	}
	for i := 0; i < empty; i++ {
		bar += emptyStyle.Render("░")
	}

	return bar
}

// StatusIcon returns an icon and style for a status
func (t *Theme) StatusIcon(status string) (string, lipgloss.Style) {
	switch status {
	case "success", "done", "complete":
		return "✓", t.SuccessText
	case "error", "failed":
		return "✗", t.ErrorText
	case "warning":
		return "⚠", t.WarningText
	case "running", "active":
		return "●", lipgloss.NewStyle().Foreground(t.Primary)
	case "pending", "waiting":
		return "○", t.MutedText
	default:
		return "·", t.MutedText
	}
}
