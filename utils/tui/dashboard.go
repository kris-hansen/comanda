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

// DashboardModel is the bubbletea model for the live dashboard
type DashboardModel struct {
	workflowName  string
	currentStep   string
	currentLoop   string
	loopIteration int
	maxIterations int
	elapsedTime   time.Duration
	tokensUsed    int
	tokensAvail   int
	contextPct    float64
	cpuPercent    float64
	memoryMB      float64
	activities    []Activity
	outputLines   []string
	status        string // "running", "paused", "complete", "error"
	errorMsg      string

	spinner     spinner.Model
	width       int
	height      int
	theme       *Theme
	keymap      dashboardKeyMap
	startTime   time.Time
	eventChan   chan ProgressEvent
	reporter    *ProgressReporter
	quitting    bool
	verbose     bool
	maxActivity int
	maxOutput   int
}

type dashboardKeyMap struct {
	Quit    key.Binding
	Verbose key.Binding
}

func defaultDashboardKeyMap() dashboardKeyMap {
	return dashboardKeyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Verbose: key.NewBinding(
			key.WithKeys("v"),
			key.WithHelp("v", "verbose"),
		),
	}
}

// TickMsg is sent on each tick for time/resource updates
type TickMsg time.Time

// EventMsg wraps a progress event
type EventMsg struct {
	Event ProgressEvent
}

// NewDashboardModel creates a new dashboard model
func NewDashboardModel(workflowName string, reporter *ProgressReporter) *DashboardModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED"))

	eventChan := reporter.Subscribe()

	return &DashboardModel{
		workflowName: workflowName,
		status:       "running",
		spinner:      s,
		theme:        DefaultTheme(),
		keymap:       defaultDashboardKeyMap(),
		startTime:    time.Now(),
		eventChan:    eventChan,
		reporter:     reporter,
		maxActivity:  8,
		maxOutput:    5,
	}
}

// Init initializes the model
func (m *DashboardModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.tickCmd(),
		m.waitForEvent(),
	)
}

func (m *DashboardModel) tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

func (m *DashboardModel) waitForEvent() tea.Cmd {
	return func() tea.Msg {
		event, ok := <-m.eventChan
		if !ok {
			return nil
		}
		return EventMsg{Event: event}
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
		m.elapsedTime = time.Since(m.startTime)
		// Update resource usage
		if m.reporter != nil {
			m.cpuPercent, m.memoryMB = m.reporter.GetResourceUsage()
		}
		cmds = append(cmds, m.tickCmd())

	case EventMsg:
		m.handleEvent(msg.Event)
		if m.status != "complete" && m.status != "error" {
			cmds = append(cmds, m.waitForEvent())
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *DashboardModel) handleEvent(event ProgressEvent) {
	switch event.Type {
	case "step_start":
		m.currentStep = event.StepName
		m.addActivity("info", fmt.Sprintf("Starting: %s", event.StepName))

	case "step_end":
		if event.Error != nil {
			m.addActivity("error", fmt.Sprintf("Failed: %s - %v", event.StepName, event.Error))
		} else {
			m.addActivity("info", fmt.Sprintf("Completed: %s", event.StepName))
		}

	case "loop_iter":
		m.currentLoop = event.LoopName
		m.loopIteration = event.Iteration
		m.maxIterations = event.MaxIter
		m.addActivity("info", fmt.Sprintf("Loop %s iteration %d/%d", event.LoopName, event.Iteration, event.MaxIter))

	case "tool_call":
		m.addActivity("tool", event.Message)

	case "output":
		m.addOutput(event.Message)

	case "tokens":
		m.tokensUsed = event.TokensUsed
		m.tokensAvail = event.TokensAvail
		if m.tokensAvail > 0 {
			m.contextPct = float64(m.tokensUsed) / float64(m.tokensAvail) * 100
		}

	case "complete":
		if event.Error != nil {
			m.status = "error"
			m.errorMsg = event.Error.Error()
			m.addActivity("error", fmt.Sprintf("Workflow failed: %v", event.Error))
		} else {
			m.status = "complete"
			m.addActivity("info", "Workflow completed successfully")
		}
	}
}

func (m *DashboardModel) addActivity(actType, message string) {
	m.activities = append(m.activities, Activity{
		Timestamp: time.Now(),
		Type:      actType,
		Message:   message,
	})

	// Trim to max
	if len(m.activities) > m.maxActivity*2 {
		m.activities = m.activities[len(m.activities)-m.maxActivity:]
	}
}

func (m *DashboardModel) addOutput(line string) {
	lines := strings.Split(line, "\n")
	m.outputLines = append(m.outputLines, lines...)

	// Trim to max
	if len(m.outputLines) > m.maxOutput*2 {
		m.outputLines = m.outputLines[len(m.outputLines)-m.maxOutput:]
	}
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
	if len(m.outputLines) > 0 || m.verbose {
		sections = append(sections, m.renderOutput())
	}

	// Footer
	sections = append(sections, m.renderFooter())

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (m *DashboardModel) renderHeader() string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(m.theme.Primary).
		Background(m.theme.BgPrimary).
		Padding(0, 2).
		Width(m.width - 2).
		Align(lipgloss.Center)

	var statusIcon string
	var statusStyle lipgloss.Style
	switch m.status {
	case "running":
		statusIcon = m.spinner.View()
		statusStyle = lipgloss.NewStyle().Foreground(m.theme.Primary)
	case "complete":
		statusIcon = "✓"
		statusStyle = lipgloss.NewStyle().Foreground(m.theme.Success)
	case "error":
		statusIcon = "✗"
		statusStyle = lipgloss.NewStyle().Foreground(m.theme.Error)
	}

	elapsed := formatDuration(m.elapsedTime)
	elapsedStyle := lipgloss.NewStyle().Foreground(m.theme.Muted)

	header := fmt.Sprintf("COMANDA  %s %s  %s",
		statusStyle.Render(statusIcon),
		statusStyle.Render(m.status),
		elapsedStyle.Render("elapsed: "+elapsed),
	)

	return titleStyle.Render(header)
}

func (m *DashboardModel) renderProgress() string {
	boxStyle := m.theme.BoxNormal.Width(m.width - 4)

	var content strings.Builder

	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Secondary)
	content.WriteString(nameStyle.Render("Workflow: "))
	content.WriteString(m.workflowName)
	content.WriteString("\n")

	if m.currentLoop != "" {
		loopStyle := lipgloss.NewStyle().Foreground(m.theme.Accent)
		content.WriteString(loopStyle.Render(fmt.Sprintf("Loop: %s", m.currentLoop)))

		if m.maxIterations > 0 {
			progress := float64(m.loopIteration) / float64(m.maxIterations)
			progressBar := m.theme.ProgressBar(progress, 20)
			percent := int(progress * 100)

			content.WriteString(fmt.Sprintf("  [%d/%d]  %s  %d%%",
				m.loopIteration,
				m.maxIterations,
				progressBar,
				percent,
			))
		}
		content.WriteString("\n")
	}

	if m.currentStep != "" {
		stepStyle := lipgloss.NewStyle().Foreground(m.theme.Muted)
		content.WriteString(stepStyle.Render("Step: "))
		content.WriteString(m.currentStep)
	}

	return boxStyle.Render(content.String())
}

func (m *DashboardModel) renderResources() string {
	boxStyle := m.theme.BoxNormal.Width(m.width - 4)

	leftWidth := (m.width - 8) / 2
	rightWidth := (m.width - 8) / 2

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Secondary)

	// Left column: System resources
	var left strings.Builder
	left.WriteString(labelStyle.Render("RESOURCES"))
	left.WriteString("\n")

	cpuBar := m.theme.ProgressBar(m.cpuPercent/100, 12)
	left.WriteString(fmt.Sprintf("CPU:  %s %3.0f%%\n", cpuBar, m.cpuPercent))

	memPercent := m.memoryMB / 8192
	if memPercent > 1 {
		memPercent = 1
	}
	memBar := m.theme.ProgressBar(memPercent, 12)
	left.WriteString(fmt.Sprintf("MEM:  %s %.1fGB", memBar, m.memoryMB/1024))

	leftBox := lipgloss.NewStyle().Width(leftWidth).Render(left.String())

	// Right column: Context window (estimated)
	var right strings.Builder
	right.WriteString(labelStyle.Render("CONTEXT WINDOW"))

	// Show (est.) indicator since we're estimating tokens
	estStyle := lipgloss.NewStyle().Foreground(m.theme.Muted).Italic(true)
	right.WriteString(estStyle.Render(" (est.)"))
	right.WriteString("\n")

	ctxBar := m.theme.ProgressBar(m.contextPct/100, 16)
	right.WriteString(fmt.Sprintf("%s %.1f%%\n", ctxBar, m.contextPct))

	if m.tokensAvail > 0 {
		valueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E5E7EB"))
		right.WriteString(valueStyle.Render(fmt.Sprintf("~%dk / %dk tokens",
			m.tokensUsed/1000,
			m.tokensAvail/1000,
		)))
	} else {
		// Show placeholder when no context info yet
		mutedStyle := lipgloss.NewStyle().Foreground(m.theme.Muted).Italic(true)
		right.WriteString(mutedStyle.Render("awaiting model info..."))
	}

	rightBox := lipgloss.NewStyle().Width(rightWidth).Render(right.String())

	combined := lipgloss.JoinHorizontal(lipgloss.Top, leftBox, rightBox)

	return boxStyle.Render(combined)
}

func (m *DashboardModel) renderActivity() string {
	boxStyle := m.theme.BoxNormal.Width(m.width - 4)

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Secondary)

	var content strings.Builder
	content.WriteString(labelStyle.Render("RECENT ACTIVITY"))
	content.WriteString("\n")

	if len(m.activities) == 0 {
		mutedStyle := lipgloss.NewStyle().Foreground(m.theme.Muted).Italic(true)
		content.WriteString(mutedStyle.Render("  Waiting for activity..."))
	} else {
		start := 0
		if len(m.activities) > m.maxActivity {
			start = len(m.activities) - m.maxActivity
		}

		for _, activity := range m.activities[start:] {
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
	boxStyle := m.theme.BoxNormal.Width(m.width - 4)

	labelStyle := lipgloss.NewStyle().Bold(true).Foreground(m.theme.Secondary)
	mutedStyle := lipgloss.NewStyle().Foreground(m.theme.Muted)

	var content strings.Builder
	content.WriteString(labelStyle.Render("OUTPUT"))
	content.WriteString(mutedStyle.Render(fmt.Sprintf(" (last %d lines)", m.maxOutput)))
	content.WriteString("\n")

	if len(m.outputLines) == 0 {
		content.WriteString(mutedStyle.Italic(true).Render("  No output yet..."))
	} else {
		start := 0
		if len(m.outputLines) > m.maxOutput {
			start = len(m.outputLines) - m.maxOutput
		}

		for _, line := range m.outputLines[start:] {
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

	keys := []string{"q quit", "v verbose"}
	help := strings.Join(keys, "  •  ")

	return helpStyle.Render(help)
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	min := d / time.Minute
	d -= min * time.Minute
	s := d / time.Second

	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, min, s)
	}
	return fmt.Sprintf("%02d:%02d", min, s)
}

// RunDashboard starts the dashboard TUI and returns the program for external control
func RunDashboard(workflowName string, reporter *ProgressReporter) (*DashboardModel, *tea.Program) {
	m := NewDashboardModel(workflowName, reporter)
	p := tea.NewProgram(m, tea.WithAltScreen())
	return m, p
}
