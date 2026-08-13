package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestDashboardActivityDetailsPreserveFullError(t *testing.T) {
	model := NewDashboardModel("test", NewProgressReporter())
	model.width = 52
	model.height = 20

	fullError := "workflow failed because the agent action exceeded the configured context window; inspect the source manifest before retrying"
	model.addActivity("error", fullError)

	lines := model.activityDetailLines()
	if len(lines) < 2 {
		t.Fatalf("activity detail lines = %d, want wrapped error output", len(lines))
	}
	details := strings.Join(activityTexts(lines), "\n")
	if !strings.Contains(details, "source") || !strings.Contains(details, "retrying") {
		t.Fatal("detailed activity view did not retain the end of the error message")
	}
}

func TestDashboardCompactActivityStaysWithinPreviewHeight(t *testing.T) {
	model := NewDashboardModel("test", NewProgressReporter())
	model.width = 80
	model.addActivity("output", "first line\nsecond line\nthird line\nfourth line")

	view := model.renderActivity()
	// Rounded borders plus the label and single preview occupy five rows.
	if got, want := lipgloss.Height(view), 5; got != want {
		t.Fatalf("compact activity height = %d, want %d", got, want)
	}
	if !strings.Contains(view, "first line second line") {
		t.Fatalf("compact activity did not retain a one-line preview: %q", view)
	}

	lines := model.activityDetailLines()
	details := strings.Join(activityTexts(lines), "\n")
	if !strings.Contains(details, "fourth line") {
		t.Fatal("detailed activity view did not retain the complete message")
	}
}

func TestDashboardActivityHistoryIsBounded(t *testing.T) {
	model := NewDashboardModel("test", NewProgressReporter())
	for i := 0; i < 501; i++ {
		model.addActivity("info", "activity")
	}
	if got := len(model.activities); got != 500 {
		t.Fatalf("activity history = %d, want 500", got)
	}
}

func TestDashboardCtrlRTogglesActivityDetails(t *testing.T) {
	model := NewDashboardModel("test", NewProgressReporter())
	model.outputExpanded = true

	model.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if !model.activityExpanded {
		t.Fatal("Ctrl+R did not open activity details")
	}
	if model.outputExpanded {
		t.Fatal("opening activity details did not close expanded output")
	}

	model.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if model.activityExpanded {
		t.Fatal("Ctrl+R did not close activity details")
	}
}

func activityTexts(lines []activityDisplayLine) []string {
	texts := make([]string, len(lines))
	for i, line := range lines {
		texts[i] = line.Text
	}
	return texts
}
