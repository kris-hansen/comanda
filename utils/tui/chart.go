package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ChartNode represents a node in the workflow chart
type ChartNode struct {
	Name        string
	Model       string
	Type        string // "step", "parallel", "loop", "condition"
	IsValid     bool
	Children    []*ChartNode
	Input       string
	Output      string
	Action      string
	LoopConfig  *LoopConfig
	IsDeferred  bool
	Description string
}

// LoopConfig contains loop-specific configuration
type LoopConfig struct {
	MaxIterations int
	ExitCondition string
	ContextWindow int
}

// ChartModel is the bubbletea model for the chart view
type ChartModel struct {
	nodes        []*ChartNode
	cursor       int
	selected     *ChartNode
	viewport     viewport.Model
	ready        bool
	width        int
	height       int
	theme        *Theme
	workflowName string
	stats        ChartStats
	showDetails  bool
	keymap       chartKeyMap
}

// ChartStats holds workflow statistics
type ChartStats struct {
	TotalSteps    int
	ParallelSteps int
	LoopCount     int
	ValidSteps    int
	DeferredSteps int
	Models        map[string]int
}

type chartKeyMap struct {
	Up       key.Binding
	Down     key.Binding
	Enter    key.Binding
	Quit     key.Binding
	Help     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
}

func defaultChartKeyMap() chartKeyMap {
	return chartKeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter", " "),
			key.WithHelp("enter", "details"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "esc", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup", "ctrl+u"),
			key.WithHelp("pgup", "page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown", "ctrl+d"),
			key.WithHelp("pgdn", "page down"),
		),
	}
}

// NewChartModel creates a new chart model
func NewChartModel(workflowName string, nodes []*ChartNode, stats ChartStats) ChartModel {
	theme := DefaultTheme()

	return ChartModel{
		workflowName: workflowName,
		nodes:        nodes,
		theme:        theme,
		stats:        stats,
		keymap:       defaultChartKeyMap(),
	}
}

// Init initializes the model
func (m ChartModel) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m ChartModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keymap.Quit):
			return m, tea.Quit
		case key.Matches(msg, m.keymap.Up):
			if m.cursor > 0 {
				m.cursor--
			}
		case key.Matches(msg, m.keymap.Down):
			if m.cursor < len(m.nodes)-1 {
				m.cursor++
			}
		case key.Matches(msg, m.keymap.Enter):
			m.showDetails = !m.showDetails
			if m.cursor < len(m.nodes) {
				m.selected = m.nodes[m.cursor]
			}
		case key.Matches(msg, m.keymap.PageUp):
			m.viewport, cmd = m.viewport.Update(msg)
			cmds = append(cmds, cmd)
		case key.Matches(msg, m.keymap.PageDown):
			m.viewport, cmd = m.viewport.Update(msg)
			cmds = append(cmds, cmd)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-10)
			m.viewport.YPosition = 5
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - 10
		}
	}

	// Update viewport content
	m.viewport.SetContent(m.renderChart())
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

// View renders the model
func (m ChartModel) View() string {
	if !m.ready {
		return "Initializing..."
	}

	var b strings.Builder

	// Header
	header := m.renderHeader()
	b.WriteString(header)
	b.WriteString("\n")

	// Main content
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	// Footer with stats and help
	footer := m.renderFooter()
	b.WriteString(footer)

	// Details panel (if showing)
	if m.showDetails && m.selected != nil {
		details := m.renderDetails()
		b.WriteString("\n")
		b.WriteString(details)
	}

	return b.String()
}

func (m ChartModel) renderHeader() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.Primary).
		Background(m.theme.BgPrimary).
		Padding(0, 2).
		MarginBottom(1)

	subtitleStyle := lipgloss.NewStyle().
		Foreground(m.theme.Secondary)

	title := titleStyle.Render("COMANDA")
	subtitle := subtitleStyle.Render(m.workflowName)

	return lipgloss.JoinHorizontal(lipgloss.Center, title, "  ", subtitle)
}

func (m ChartModel) renderChart() string {
	var b strings.Builder

	for i, node := range m.nodes {
		line := m.renderNode(node, i == m.cursor, 0)
		b.WriteString(line)
		b.WriteString("\n")

		// Render connector if not last
		if i < len(m.nodes)-1 {
			connector := m.renderConnector(node, m.nodes[i+1])
			b.WriteString(connector)
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m ChartModel) renderNode(node *ChartNode, selected bool, depth int) string {
	indent := strings.Repeat("  ", depth)

	// Determine style based on node type and selection
	var boxStyle lipgloss.Style
	if selected {
		boxStyle = m.theme.BoxHighlight.
			BorderForeground(m.theme.Primary).
			Bold(true)
	} else if !node.IsValid {
		boxStyle = m.theme.BoxError
	} else {
		boxStyle = m.theme.BoxNormal
	}

	// Model color
	modelStyle := m.theme.ModelStyle(node.Model)

	// Status icon
	status := "success"
	if !node.IsValid {
		status = "error"
	}
	icon, iconStyle := m.theme.StatusIcon(status)

	// Type indicator
	var typeIndicator string
	switch node.Type {
	case "loop":
		typeIndicator = lipgloss.NewStyle().Foreground(m.theme.Accent).Render("↻")
	case "parallel":
		typeIndicator = lipgloss.NewStyle().Foreground(m.theme.Secondary).Render("⫸")
	case "condition":
		typeIndicator = lipgloss.NewStyle().Foreground(m.theme.Warning).Render("?")
	default:
		typeIndicator = lipgloss.NewStyle().Foreground(m.theme.Muted).Render("→")
	}

	// Build content
	nameStyle := lipgloss.NewStyle().Bold(true)
	if selected {
		nameStyle = nameStyle.Foreground(m.theme.Primary)
	}

	content := fmt.Sprintf("%s %s %s  %s",
		iconStyle.Render(icon),
		typeIndicator,
		nameStyle.Render(node.Name),
		modelStyle.Render(node.Model),
	)

	box := boxStyle.Render(content)

	return indent + box
}

func (m ChartModel) renderConnector(from, to *ChartNode) string {
	connectorStyle := lipgloss.NewStyle().Foreground(m.theme.Muted)

	if from.Type == "parallel" || to.Type == "parallel" {
		return connectorStyle.Render("    ├──┬──┤")
	}

	return connectorStyle.Render("    │")
}

func (m ChartModel) renderFooter() string {
	// Stats bar
	statsStyle := lipgloss.NewStyle().
		Foreground(m.theme.Muted).
		MarginTop(1)

	stats := fmt.Sprintf("Steps: %d  Parallel: %d  Loops: %d  Valid: %d/%d",
		m.stats.TotalSteps,
		m.stats.ParallelSteps,
		m.stats.LoopCount,
		m.stats.ValidSteps,
		m.stats.TotalSteps,
	)

	// Model breakdown
	var modelParts []string
	for model, count := range m.stats.Models {
		style := m.theme.ModelStyle(model)
		modelParts = append(modelParts, style.Render(fmt.Sprintf("%s:%d", model, count)))
	}
	modelStats := strings.Join(modelParts, "  ")

	// Help
	helpStyle := lipgloss.NewStyle().
		Foreground(m.theme.Muted).
		MarginTop(1)

	help := helpStyle.Render("↑↓ navigate • enter details • q quit")

	return lipgloss.JoinVertical(lipgloss.Left,
		statsStyle.Render(stats),
		modelStats,
		help,
	)
}

func (m ChartModel) renderDetails() string {
	if m.selected == nil {
		return ""
	}

	node := m.selected

	detailStyle := m.theme.BoxHighlight.
		Width(m.width-4).
		Padding(1, 2)

	labelStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.Secondary)

	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#E5E7EB"))

	var details strings.Builder
	details.WriteString(lipgloss.NewStyle().Bold(true).Foreground(m.theme.Primary).Render(node.Name))
	details.WriteString("\n\n")

	details.WriteString(labelStyle.Render("Model: "))
	details.WriteString(m.theme.ModelStyle(node.Model).Render(node.Model))
	details.WriteString("\n")

	details.WriteString(labelStyle.Render("Type: "))
	details.WriteString(valueStyle.Render(node.Type))
	details.WriteString("\n")

	if node.Input != "" {
		details.WriteString(labelStyle.Render("Input: "))
		details.WriteString(valueStyle.Render(node.Input))
		details.WriteString("\n")
	}

	if node.Output != "" {
		details.WriteString(labelStyle.Render("Output: "))
		details.WriteString(valueStyle.Render(node.Output))
		details.WriteString("\n")
	}

	if node.Action != "" {
		details.WriteString(labelStyle.Render("Action: "))
		// Truncate long actions
		action := node.Action
		if len(action) > 100 {
			action = action[:97] + "..."
		}
		details.WriteString(valueStyle.Render(action))
		details.WriteString("\n")
	}

	if node.LoopConfig != nil {
		details.WriteString("\n")
		details.WriteString(labelStyle.Render("Loop Config:"))
		details.WriteString("\n")
		details.WriteString(fmt.Sprintf("  Max Iterations: %d\n", node.LoopConfig.MaxIterations))
		if node.LoopConfig.ExitCondition != "" {
			details.WriteString(fmt.Sprintf("  Exit Condition: %s\n", node.LoopConfig.ExitCondition))
		}
		if node.LoopConfig.ContextWindow > 0 {
			details.WriteString(fmt.Sprintf("  Context Window: %d\n", node.LoopConfig.ContextWindow))
		}
	}

	return detailStyle.Render(details.String())
}

// RunChart starts the chart TUI
func RunChart(workflowName string, nodes []*ChartNode, stats ChartStats) error {
	m := NewChartModel(workflowName, nodes, stats)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
