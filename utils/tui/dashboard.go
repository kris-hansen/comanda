package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Activity represents a recent activity in the dashboard
type Activity struct {
	Timestamp time.Time
	Type      string // "tool", "output", "error", "info"
	Message   string
}

// DashboardState holds the current state of the workflow
type DashboardState struct {
	WorkflowName    string
	CurrentStep     string
	CurrentLoop     string
	LoopIteration   int
	MaxIterations   int
	ElapsedTime     time.Duration
	TokensUsed      int
	TokensAvailable int
	ContextPercent  float64
	CPUPercent      float64
	MemoryMB        float64
	Activities      []Activity
	OutputLines     []string
	Status          string // "running", "paused", "complete", "error"
	Error           string
}

// DashboardModel is the bubbletea model for the live dashboard
type DashboardModel struct {
	state       DashboardState
	spinner     spinner.Model
	width       int
	height      int
	theme       *Theme
	keymap      dashboardKeyMap
	startTime   time.Time
	updateChan  chan DashboardState
	quitting    bool
	paused      bool
	verbose     bool
	maxActivity int
	maxOutput   int
}

type dashboardKeyMap struct {
	Quit    key.Binding
	Pause   key.Binding
	Verbose key.Binding
	Save    key.Binding
}

func defaultDashboardKeyMap() dashboardKeyMap {
	return dashboardKeyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Pause: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "pause"),
		),
		Verbose: key.NewBinding(
			key.WithKeys("v"),
			key.WithHelp("v", "verbose"),
		),
		Save: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "snapshot"),
		),
	}
}

// StateUpdateMsg is sent when the state is updated
type StateUpdateMsg struct {
	State DashboardState
}

// TickMsg is sent on each tick for time updates
type TickMsg time.Time

// NewDashboardModel creates a new dashboard model
func NewDashboardModel(workflowName string) *DashboardModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED"))

	return &DashboardModel{
		state: DashboardState{
			WorkflowName: workflowName,
			Status:       "running",
		},
		spinner:     s,
		theme:       DefaultTheme(),
		keymap:      defaultDashboardKeyMap(),
		startTime:   time.Now(),
		updateChan:  make(chan DashboardState, 100),
		maxActivity: 8,
		maxOutput:   5,
	}
}

// Init initializes the model
func (m *DashboardModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.tickCmd(),
		m.waitForUpdate(),
	)
}

func (m *DashboardModel) tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

func (m *DashboardModel) waitForUpdate() tea.Cmd {
	return func() tea.Msg {
		state := <-m.updateChan
		return StateUpdateMsg{State: state}
	}
}

// SendUpdate sends a state update to the dashboard
func (m *DashboardModel) SendUpdate(state DashboardState) {
	select {
	case m.updateChan <- state:
	default:
		// Channel full, skip update
	}
}

// Update handles messages
func (m *DashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keymap.Quit):
			m.quitting = true
			return m, tea.Quit
		case key.Matches(msg, m.keymap.Pause):
			m.paused = !m.paused
		case key.Matches(msg, m.keymap.Verbose):
			m.verbose = !m.verbose
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case TickMsg:
		m.state.ElapsedTime = time.Since(m.startTime)
		cmds = append(cmds, m.tickCmd())

	case StateUpdateMsg:
		m.state = msg.State
		m.state.ElapsedTime = time.Since(m.startTime)
		cmds = append(cmds, m.waitForUpdate())
	}

	return m, tea.Batch(cmds...)
}

// View renders the dashboard
func (m *DashboardModel) View() string {
	if m.quitting {
		return ""
	}

	if m.width == 0 {
		return "Loading..."
	}

	var sections []string

	// Header
	sections = append(sections, m.renderHeader())

	// Progress section
	sections = append(sections, m.renderProgress())

	// Resources section
	sections = append(sections, m.renderResources())

	// Activity section
	sections = append(sections, m.renderActivity())

	// Output section
	if len(m.state.OutputLines) > 0 || m.verbose {
		sections = append(sections, m.renderOutput())
	}

	// Footer
	sections = append(sections, m.renderFooter())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m *DashboardModel) renderHeader() string {
	// Main title box
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.Primary).
		Background(m.theme.BgPrimary).
		Padding(0, 2).
		Width(m.width - 2).
		Align(lipgloss.Center)

	// Status indicator
	var statusIcon string
	var statusStyle lipgloss.Style
	switch m.state.Status {
	case "running":
		statusIcon = m.spinner.View()
		statusStyle = lipgloss.NewStyle().Foreground(m.theme.Primary)
	case "paused":
		statusIcon = "⏸"
		statusStyle = lipgloss.NewStyle().Foreground(m.theme.Warning)
	case "complete":
		statusIcon = "✓"
		statusStyle = lipgloss.NewStyle().Foreground(m.theme.Success)
	case "error":
		statusIcon = "✗"
		statusStyle = lipgloss.NewStyle().Foreground(m.theme.Error)
	}

	elapsed := formatDuration(m.state.ElapsedTime)
	elapsedStyle := lipgloss.NewStyle().Foreground(m.theme.Muted)

	header := fmt.Sprintf("COMANDA  %s %s  %s",
		statusStyle.Render(statusIcon),
		statusStyle.Render(m.state.Status),
		elapsedStyle.Render("elapsed: "+elapsed),
	)

	return titleStyle.Render(header)
}

func (m *DashboardModel) renderProgress() string {
	boxStyle := m.theme.BoxNormal.Copy().Width(m.width - 4)

	var content strings.Builder

	// Workflow name
	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Secondary)
	content.WriteString(nameStyle.Render("Workflow: "))
	content.WriteString(m.state.WorkflowName)
	content.WriteString("\n")

	// Current step/loop
	if m.state.CurrentLoop != "" {
		loopStyle := lipgloss.NewStyle().Foreground(m.theme.Accent)
		content.WriteString(loopStyle.Render(fmt.Sprintf("Loop: %s", m.state.CurrentLoop)))

		if m.state.MaxIterations > 0 {
			// Progress bar for loop
			progress := float64(m.state.LoopIteration) / float64(m.state.MaxIterations)
			progressBar := m.theme.ProgressBar(progress, 20)
			percent := int(progress * 100)

			content.WriteString(fmt.Sprintf("  [%d/%d]  %s  %d%%",
				m.state.LoopIteration,
				m.state.MaxIterations,
				progressBar,
				percent,
			))
		}
		content.WriteString("\n")
	}

	if m.state.CurrentStep != "" {
		stepStyle := lipgloss.NewStyle().Foreground(m.theme.Muted)
		content.WriteString(stepStyle.Render("Step: "))
		content.WriteString(m.state.CurrentStep)
	}

	return boxStyle.Render(content.String())
}

func (m *DashboardModel) renderResources() string {
	boxStyle := m.theme.BoxNormal.Copy().Width(m.width - 4)

	// Split into two columns
	leftWidth := (m.width - 8) / 2
	rightWidth := (m.width - 8) / 2

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Secondary)
	valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB"))

	// Left column: System resources
	var left strings.Builder
	left.WriteString(labelStyle.Render("RESOURCES"))
	left.WriteString("\n")

	// CPU
	cpuBar := m.theme.ProgressBar(m.state.CPUPercent/100, 12)
	left.WriteString(fmt.Sprintf("CPU:  %s %3.0f%%\n", cpuBar, m.state.CPUPercent))

	// Memory
	memPercent := m.state.MemoryMB / 8192 // Assume 8GB baseline
	if memPercent > 1 {
		memPercent = 1
	}
	memBar := m.theme.ProgressBar(memPercent, 12)
	left.WriteString(fmt.Sprintf("MEM:  %s %.1fGB", memBar, m.state.MemoryMB/1024))

	leftBox := lipgloss.NewStyle().Width(leftWidth).Render(left.String())

	// Right column: Context window
	var right strings.Builder
	right.WriteString(labelStyle.Render("CONTEXT WINDOW"))
	right.WriteString("\n")

	ctxBar := m.theme.ProgressBar(m.state.ContextPercent/100, 16)
	right.WriteString(fmt.Sprintf("%s %.0f%%\n", ctxBar, m.state.ContextPercent))

	if m.state.TokensAvailable > 0 {
		right.WriteString(valueStyle.Render(fmt.Sprintf("%dk / %dk tokens",
			m.state.TokensUsed/1000,
			m.state.TokensAvailable/1000,
		)))
	}

	rightBox := lipgloss.NewStyle().Width(rightWidth).Render(right.String())

	combined := lipgloss.JoinHorizontal(lipgloss.Top, leftBox, rightBox)

	return boxStyle.Render(combined)
}

func (m *DashboardModel) renderActivity() string {
	boxStyle := m.theme.BoxNormal.Copy().Width(m.width - 4)

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Secondary)

	var content strings.Builder
	content.WriteString(labelStyle.Render("RECENT ACTIVITY"))
	content.WriteString("\n")

	if len(m.state.Activities) == 0 {
		mutedStyle := lipgloss.NewStyle().Foreground(m.theme.Muted).Italic(true)
		content.WriteString(mutedStyle.Render("  Waiting for activity..."))
	} else {
		// Show last N activities
		start := 0
		if len(m.state.Activities) > m.maxActivity {
			start = len(m.state.Activities) - m.maxActivity
		}

		for _, activity := range m.state.Activities[start:] {
			var icon string
			var style lipgloss.Style

			switch activity.Type {
			case "tool":
				icon = "→"
				style = lipgloss.NewStyle().Foreground(m.theme.Secondary)
			case "output":
				icon = "◦"
				style = lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB"))
			case "error":
				icon = "✗"
				style = lipgloss.NewStyle().Foreground(m.theme.Error)
			case "info":
				icon = "•"
				style = lipgloss.NewStyle().Foreground(m.theme.Muted)
			default:
				icon = "·"
				style = lipgloss.NewStyle().Foreground(m.theme.Muted)
			}

			// Truncate long messages
			msg := activity.Message
			maxLen := m.width - 10
			if len(msg) > maxLen {
				msg = msg[:maxLen-3] + "..."
			}

			content.WriteString(fmt.Sprintf("  %s %s\n", icon, style.Render(msg)))
		}
	}

	return boxStyle.Render(content.String())
}

func (m *DashboardModel) renderOutput() string {
	boxStyle := m.theme.BoxNormal.Copy().Width(m.width - 4)

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Secondary)
	mutedStyle := lipgloss.NewStyle().Foreground(m.theme.Muted)

	var content strings.Builder
	content.WriteString(labelStyle.Render("OUTPUT"))
	content.WriteString(mutedStyle.Render(fmt.Sprintf(" (last %d lines)", m.maxOutput)))
	content.WriteString("\n")

	if len(m.state.OutputLines) == 0 {
		content.WriteString(mutedStyle.Italic(true).Render("  No output yet..."))
	} else {
		start := 0
		if len(m.state.OutputLines) > m.maxOutput {
			start = len(m.state.OutputLines) - m.maxOutput
		}

		for _, line := range m.state.OutputLines[start:] {
			// Truncate long lines
			if len(line) > m.width-8 {
				line = line[:m.width-11] + "..."
			}
			content.WriteString("  " + line + "\n")
		}
	}

	return boxStyle.Render(content.String())
}

func (m *DashboardModel) renderFooter() string {
	helpStyle := lipgloss.NewStyle().
		Foreground(m.theme.Muted).
		Width(m.width - 4).
		Align(lipgloss.Center).
		MarginTop(1)

	keys := []string{"q quit", "p pause", "v verbose", "s snapshot"}
	help := strings.Join(keys, "  •  ")

	return helpStyle.Render(help)
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// RunDashboard starts the dashboard TUI
func RunDashboard(workflowName string) (*DashboardModel, *tea.Program) {
	m := NewDashboardModel(workflowName)
	p := tea.NewProgram(m, tea.WithAltScreen())

	return m, p
}
