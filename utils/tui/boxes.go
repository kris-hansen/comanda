// Package tui provides terminal UI components using lipgloss
package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Box styles for consistent rendering across the codebase
var (
	// RoundedBox uses rounded corners (╭╮╰╯)
	RoundedBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))

	// NormalBox uses standard corners (┌┐└┘)
	NormalBox = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("240"))

	// DoubleBox uses double-line borders (╔╗╚╝)
	DoubleBox = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("240"))

	// HeaderBox is a bold double-line box for headers
	HeaderBox = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("99")).
			Bold(true)

	// Separator creates a horizontal line
	SeparatorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))
)

// RenderBox renders text in a rounded box with auto-width
func RenderBox(text string, width int) string {
	// Use padding for visual balance, let lipgloss calculate width
	return RoundedBox.Padding(0, 4).Align(lipgloss.Center).Render(text)
}

// RenderDoubleBox renders text in a double-line box with auto-width
func RenderDoubleBox(text string, width int) string {
	return DoubleBox.Padding(0, 4).Align(lipgloss.Center).Render(text)
}

// RenderHeaderBox renders text in a header-style box with auto-width
func RenderHeaderBox(text string, width int) string {
	return HeaderBox.Padding(0, 4).Align(lipgloss.Center).Render(text)
}

// RenderMultiLineBox renders multiple lines in a box with auto-width
func RenderMultiLineBox(lines []string, width int, useDouble bool) string {
	content := strings.Join(lines, "\n")
	style := NormalBox
	if useDouble {
		style = DoubleBox
	}
	return style.Padding(0, 2).Render(content)
}

// Separator returns a horizontal separator line of specified width
func Separator(width int) string {
	return SeparatorStyle.Render(strings.Repeat("─", width))
}

// DoubleSeparator returns a double-line separator
func DoubleSeparator(width int) string {
	return SeparatorStyle.Render(strings.Repeat("═", width))
}

// VerticalConnector returns a vertical connector (│) centered in width
func VerticalConnector(width int) string {
	padding := (width - 1) / 2
	return strings.Repeat(" ", padding) + "│"
}

// ArrowDown returns a down arrow (▼) centered in width
func ArrowDown(width int) string {
	padding := (width - 1) / 2
	return strings.Repeat(" ", padding) + "▼"
}
