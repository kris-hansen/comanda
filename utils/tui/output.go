package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Output provides styled output for non-TUI mode
type Output struct {
	theme   *Theme
	width   int
	enabled bool
}

// NewOutput creates a new output helper
func NewOutput(width int) *Output {
	return &Output{
		theme:   DefaultTheme(),
		width:   width,
		enabled: true,
	}
}

// SetEnabled enables or disables styled output
func (o *Output) SetEnabled(enabled bool) {
	o.enabled = enabled
}

// Header prints a styled header box
func (o *Output) Header(title string) string {
	if !o.enabled {
		return fmt.Sprintf("=== %s ===", title)
	}

	style := lipgloss.NewStyle().
		Bold(true).
		Foreground(o.theme.Primary).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(o.theme.Primary).
		Padding(0, 4).
		Align(lipgloss.Center)

	return style.Render(title)
}

// SubHeader prints a styled subheader
func (o *Output) SubHeader(title string) string {
	if !o.enabled {
		return fmt.Sprintf("--- %s ---", title)
	}

	style := lipgloss.NewStyle().
		Bold(true).
		Foreground(o.theme.Secondary).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(o.theme.Muted).
		Width(o.width - 4)

	return style.Render(title)
}

// Section prints a styled section box
func (o *Output) Section(title, content string) string {
	if !o.enabled {
		return fmt.Sprintf("[%s]\n%s", title, content)
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(o.theme.Secondary)

	boxStyle := o.theme.BoxNormal.
		Width(o.width - 4)

	inner := titleStyle.Render(title) + "\n" + content

	return boxStyle.Render(inner)
}

// Step prints a styled step indicator
func (o *Output) Step(name, model, status string) string {
	if !o.enabled {
		return fmt.Sprintf("[%s] %s (%s)", status, name, model)
	}

	icon, iconStyle := o.theme.StatusIcon(status)
	modelStyle := o.theme.ModelStyle(model)
	nameStyle := lipgloss.NewStyle().Bold(true)

	return fmt.Sprintf("%s %s  %s",
		iconStyle.Render(icon),
		nameStyle.Render(name),
		modelStyle.Render(model),
	)
}

// Progress prints a styled progress line
func (o *Output) Progress(current, total int, label string) string {
	if !o.enabled {
		return fmt.Sprintf("[%d/%d] %s", current, total, label)
	}

	percent := float64(current) / float64(total)
	bar := o.theme.ProgressBar(percent, 20)

	labelStyle := lipgloss.NewStyle().Foreground(o.theme.Secondary)
	countStyle := lipgloss.NewStyle().Foreground(o.theme.Muted)

	return fmt.Sprintf("%s %s %s",
		labelStyle.Render(label),
		bar,
		countStyle.Render(fmt.Sprintf("[%d/%d]", current, total)),
	)
}

// Success prints a success message
func (o *Output) Success(message string) string {
	if !o.enabled {
		return fmt.Sprintf("✓ %s", message)
	}

	icon, style := o.theme.StatusIcon("success")
	return style.Render(icon + " " + message)
}

// Error prints an error message
func (o *Output) Error(message string) string {
	if !o.enabled {
		return fmt.Sprintf("✗ %s", message)
	}

	icon, style := o.theme.StatusIcon("error")
	return style.Render(icon + " " + message)
}

// Warning prints a warning message
func (o *Output) Warning(message string) string {
	if !o.enabled {
		return fmt.Sprintf("⚠ %s", message)
	}

	icon, style := o.theme.StatusIcon("warning")
	return style.Render(icon + " " + message)
}

// Info prints an info message
func (o *Output) Info(message string) string {
	if !o.enabled {
		return fmt.Sprintf("• %s", message)
	}

	style := lipgloss.NewStyle().Foreground(o.theme.Muted)
	return style.Render("• " + message)
}

// Muted prints muted text
func (o *Output) Muted(message string) string {
	if !o.enabled {
		return message
	}

	return o.theme.MutedText.Render(message)
}

// Bold prints bold text
func (o *Output) Bold(message string) string {
	if !o.enabled {
		return message
	}

	return lipgloss.NewStyle().Bold(true).Render(message)
}

// ModelTag prints a colored model tag
func (o *Output) ModelTag(model string) string {
	if !o.enabled {
		return fmt.Sprintf("[%s]", model)
	}

	style := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(o.theme.ModelColor(model)).
		Padding(0, 1).
		Bold(true)

	return style.Render(model)
}

// Table prints a simple styled table
func (o *Output) Table(headers []string, rows [][]string) string {
	if !o.enabled {
		var b strings.Builder
		b.WriteString(strings.Join(headers, "\t") + "\n")
		for _, row := range rows {
			b.WriteString(strings.Join(row, "\t") + "\n")
		}
		return b.String()
	}

	// Calculate column widths
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < len(widths) && len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}

	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(o.theme.Secondary).
		BorderStyle(lipgloss.NormalBorder()).
		BorderBottom(true).
		BorderForeground(o.theme.Muted)

	cellStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#E5E7EB"))

	var b strings.Builder

	// Header row
	var headerCells []string
	for i, h := range headers {
		cell := headerStyle.Width(widths[i] + 2).Render(h)
		headerCells = append(headerCells, cell)
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, headerCells...))
	b.WriteString("\n")

	// Data rows
	for _, row := range rows {
		var cells []string
		for i, cell := range row {
			if i < len(widths) {
				c := cellStyle.Width(widths[i] + 2).Render(cell)
				cells = append(cells, c)
			}
		}
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, cells...))
		b.WriteString("\n")
	}

	return b.String()
}

// StatsBox prints a statistics box
func (o *Output) StatsBox(stats map[string]interface{}) string {
	if !o.enabled {
		var b strings.Builder
		for k, v := range stats {
			b.WriteString(fmt.Sprintf("%s: %v\n", k, v))
		}
		return b.String()
	}

	boxStyle := o.theme.BoxNormal.Width(o.width - 4)
	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(o.theme.Secondary)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB"))

	var content strings.Builder
	content.WriteString(labelStyle.Render("STATISTICS"))
	content.WriteString("\n")

	for k, v := range stats {
		content.WriteString(fmt.Sprintf("  %s: %s\n",
			o.theme.MutedText.Render(k),
			valueStyle.Render(fmt.Sprintf("%v", v)),
		))
	}

	return boxStyle.Render(content.String())
}

// Divider prints a horizontal divider
func (o *Output) Divider() string {
	if !o.enabled {
		return strings.Repeat("-", o.width)
	}

	style := lipgloss.NewStyle().Foreground(o.theme.Muted)
	return style.Render(strings.Repeat("─", o.width-4))
}

// Box wraps content in a styled box
func (o *Output) Box(content string) string {
	if !o.enabled {
		return content
	}

	return o.theme.BoxNormal.Width(o.width - 4).Render(content)
}

// HighlightBox wraps content in a highlighted box
func (o *Output) HighlightBox(content string) string {
	if !o.enabled {
		return content
	}

	return o.theme.BoxHighlight.Width(o.width - 4).Render(content)
}
