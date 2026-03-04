package tui

import (
	"strings"
	"testing"
)

func TestNewOutput(t *testing.T) {
	output := NewOutput(80)
	if output == nil {
		t.Fatal("NewOutput returned nil")
	}
	if output.width != 80 {
		t.Errorf("Expected width 80, got %d", output.width)
	}
	if !output.enabled {
		t.Error("Output should be enabled by default")
	}
}

func TestOutputSetEnabled(t *testing.T) {
	output := NewOutput(80)

	output.SetEnabled(false)
	if output.enabled {
		t.Error("Output should be disabled")
	}

	output.SetEnabled(true)
	if !output.enabled {
		t.Error("Output should be enabled")
	}
}

func TestOutputHeader(t *testing.T) {
	output := NewOutput(80)

	// Enabled mode
	header := output.Header("Test Header")
	if header == "" {
		t.Error("Header returned empty string")
	}
	if !strings.Contains(header, "Test Header") {
		t.Error("Header should contain title text")
	}

	// Disabled mode
	output.SetEnabled(false)
	header = output.Header("Test Header")
	if !strings.Contains(header, "Test Header") {
		t.Error("Disabled header should still contain title")
	}
	if !strings.Contains(header, "===") {
		t.Error("Disabled header should use === markers")
	}
}

func TestOutputSubHeader(t *testing.T) {
	output := NewOutput(80)

	subHeader := output.SubHeader("Sub Header")
	if subHeader == "" {
		t.Error("SubHeader returned empty string")
	}
	if !strings.Contains(subHeader, "Sub Header") {
		t.Error("SubHeader should contain title text")
	}
}

func TestOutputSection(t *testing.T) {
	output := NewOutput(80)

	section := output.Section("Section Title", "Section content here")
	if section == "" {
		t.Error("Section returned empty string")
	}
	if !strings.Contains(section, "Section Title") {
		t.Error("Section should contain title")
	}
	if !strings.Contains(section, "Section content") {
		t.Error("Section should contain content")
	}
}

func TestOutputStep(t *testing.T) {
	output := NewOutput(80)

	tests := []struct {
		name   string
		model  string
		status string
	}{
		{"analyze", "claude-sonnet", "success"},
		{"process", "gpt-4o", "running"},
		{"validate", "gemini", "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := output.Step(tt.name, tt.model, tt.status)
			if step == "" {
				t.Error("Step returned empty string")
			}
			if !strings.Contains(step, tt.name) {
				t.Errorf("Step should contain name %q", tt.name)
			}
			if !strings.Contains(step, tt.model) {
				t.Errorf("Step should contain model %q", tt.model)
			}
		})
	}
}

func TestOutputProgress(t *testing.T) {
	output := NewOutput(80)

	progress := output.Progress(5, 10, "Processing")
	if progress == "" {
		t.Error("Progress returned empty string")
	}
	if !strings.Contains(progress, "Processing") {
		t.Error("Progress should contain label")
	}
	if !strings.Contains(progress, "5") || !strings.Contains(progress, "10") {
		t.Error("Progress should contain counts")
	}
}

func TestOutputSuccess(t *testing.T) {
	output := NewOutput(80)

	msg := output.Success("Operation completed")
	if msg == "" {
		t.Error("Success returned empty string")
	}
	if !strings.Contains(msg, "Operation completed") {
		t.Error("Success should contain message")
	}
	if !strings.Contains(msg, "✓") {
		t.Error("Success should contain checkmark")
	}
}

func TestOutputError(t *testing.T) {
	output := NewOutput(80)

	msg := output.Error("Something failed")
	if msg == "" {
		t.Error("Error returned empty string")
	}
	if !strings.Contains(msg, "Something failed") {
		t.Error("Error should contain message")
	}
	if !strings.Contains(msg, "✗") {
		t.Error("Error should contain X mark")
	}
}

func TestOutputWarning(t *testing.T) {
	output := NewOutput(80)

	msg := output.Warning("Caution required")
	if msg == "" {
		t.Error("Warning returned empty string")
	}
	if !strings.Contains(msg, "Caution required") {
		t.Error("Warning should contain message")
	}
	if !strings.Contains(msg, "⚠") {
		t.Error("Warning should contain warning symbol")
	}
}

func TestOutputInfo(t *testing.T) {
	output := NewOutput(80)

	msg := output.Info("Information here")
	if msg == "" {
		t.Error("Info returned empty string")
	}
	if !strings.Contains(msg, "Information here") {
		t.Error("Info should contain message")
	}
}

func TestOutputMuted(t *testing.T) {
	output := NewOutput(80)

	msg := output.Muted("Subtle text")
	if msg == "" {
		t.Error("Muted returned empty string")
	}
	if !strings.Contains(msg, "Subtle text") {
		t.Error("Muted should contain message")
	}
}

func TestOutputBold(t *testing.T) {
	output := NewOutput(80)

	msg := output.Bold("Important")
	if msg == "" {
		t.Error("Bold returned empty string")
	}
	if !strings.Contains(msg, "Important") {
		t.Error("Bold should contain message")
	}
}

func TestOutputModelTag(t *testing.T) {
	output := NewOutput(80)

	models := []string{"claude", "gpt-4o", "gemini", "ollama"}

	for _, model := range models {
		t.Run(model, func(t *testing.T) {
			tag := output.ModelTag(model)
			if tag == "" {
				t.Error("ModelTag returned empty string")
			}
			if !strings.Contains(tag, model) {
				t.Errorf("ModelTag should contain model name %q", model)
			}
		})
	}
}

func TestOutputTable(t *testing.T) {
	output := NewOutput(80)

	headers := []string{"Name", "Status", "Count"}
	rows := [][]string{
		{"Step 1", "Done", "10"},
		{"Step 2", "Running", "5"},
	}

	table := output.Table(headers, rows)
	if table == "" {
		t.Error("Table returned empty string")
	}
	if !strings.Contains(table, "Name") {
		t.Error("Table should contain headers")
	}
	if !strings.Contains(table, "Step 1") {
		t.Error("Table should contain row data")
	}
}

func TestOutputStatsBox(t *testing.T) {
	output := NewOutput(80)

	stats := map[string]interface{}{
		"Total":   100,
		"Success": 95,
		"Failed":  5,
	}

	box := output.StatsBox(stats)
	if box == "" {
		t.Error("StatsBox returned empty string")
	}
	if !strings.Contains(box, "STATISTICS") {
		t.Error("StatsBox should contain STATISTICS header")
	}
	if !strings.Contains(box, "Total") {
		t.Error("StatsBox should contain stat keys")
	}
}

func TestOutputDivider(t *testing.T) {
	output := NewOutput(80)

	divider := output.Divider()
	if divider == "" {
		t.Error("Divider returned empty string")
	}
	if !strings.Contains(divider, "─") {
		t.Error("Divider should contain horizontal line character")
	}
}

func TestOutputBox(t *testing.T) {
	output := NewOutput(80)

	box := output.Box("Content inside box")
	if box == "" {
		t.Error("Box returned empty string")
	}
	if !strings.Contains(box, "Content inside box") {
		t.Error("Box should contain content")
	}
}

func TestOutputHighlightBox(t *testing.T) {
	output := NewOutput(80)

	box := output.HighlightBox("Important content")
	if box == "" {
		t.Error("HighlightBox returned empty string")
	}
	if !strings.Contains(box, "Important content") {
		t.Error("HighlightBox should contain content")
	}
}

func TestOutputDisabledMode(t *testing.T) {
	output := NewOutput(80)
	output.SetEnabled(false)

	// All methods should still work but return plain text
	tests := []struct {
		name   string
		result string
	}{
		{"Header", output.Header("Test")},
		{"SubHeader", output.SubHeader("Test")},
		{"Section", output.Section("Title", "Content")},
		{"Step", output.Step("name", "model", "status")},
		{"Progress", output.Progress(1, 2, "label")},
		{"Success", output.Success("msg")},
		{"Error", output.Error("msg")},
		{"Warning", output.Warning("msg")},
		{"Info", output.Info("msg")},
		{"Muted", output.Muted("msg")},
		{"Bold", output.Bold("msg")},
		{"ModelTag", output.ModelTag("claude")},
		{"Divider", output.Divider()},
		{"Box", output.Box("content")},
		{"HighlightBox", output.HighlightBox("content")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.result == "" {
				t.Errorf("%s returned empty string in disabled mode", tt.name)
			}
		})
	}
}
