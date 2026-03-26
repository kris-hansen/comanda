package tui

import (
	"testing"
	"time"
)

func TestNewTokenEstimator(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	tests := []struct {
		model         string
		expectedLimit int
	}{
		{"claude-code", 200000},
		{"gpt-4o", 128000},
		{"gemini-2.0-flash", 1000000},
		{"unknown-model", 128000}, // default
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			est := NewTokenEstimator(tt.model, reporter)
			if est == nil {
				t.Fatal("NewTokenEstimator returned nil")
			}
			_, _, limit := est.GetEstimates()
			if limit != tt.expectedLimit {
				t.Errorf("Expected context limit %d for %s, got %d", tt.expectedLimit, tt.model, limit)
			}
		})
	}
}

func TestTokenEstimatorAddInput(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	est := NewTokenEstimator("claude-code", reporter)

	// Add some input text
	est.AddInput("Hello, this is a test input.")
	est.AddInput("More input text here.")

	inputTokens, _, _ := est.GetEstimates()
	if inputTokens == 0 {
		t.Error("Expected non-zero input tokens after AddInput")
	}
}

func TestTokenEstimatorAddOutput(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	est := NewTokenEstimator("claude-code", reporter)

	// Add some output text
	est.AddOutput("This is the model response.")
	est.AddOutput("More response content.")

	_, outputTokens, _ := est.GetEstimates()
	if outputTokens == 0 {
		t.Error("Expected non-zero output tokens after AddOutput")
	}
}

func TestTokenEstimatorReset(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	est := NewTokenEstimator("claude-code", reporter)

	est.AddInput("Some input")
	est.AddOutput("Some output")
	est.Reset()

	inputTokens, outputTokens, _ := est.GetEstimates()
	if inputTokens != 0 || outputTokens != 0 {
		t.Errorf("Expected zero tokens after reset, got input=%d output=%d", inputTokens, outputTokens)
	}
}

func TestTokenEstimatorSetModel(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	est := NewTokenEstimator("claude-code", reporter)

	// Change model
	est.SetModel("gpt-4o")

	_, _, limit := est.GetEstimates()
	if limit != 128000 {
		t.Errorf("Expected context limit 128000 after SetModel, got %d", limit)
	}
}

func TestTokenEstimatorGetContextPercent(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	est := NewTokenEstimator("claude-code", reporter) // 200k context

	// Add enough text to reach ~1% (200k * 0.01 = 2000 tokens = ~7000 chars)
	largeText := make([]byte, 7000)
	for i := range largeText {
		largeText[i] = 'a'
	}
	est.AddOutput(string(largeText))

	pct := est.GetContextPercent()
	if pct < 0.5 || pct > 2.0 {
		t.Errorf("Expected context percent around 1%%, got %.2f%%", pct)
	}
}

func TestTokenEstimatorEmitUpdate(t *testing.T) {
	reporter := NewProgressReporter()
	defer reporter.Close()

	ch := reporter.Subscribe()
	est := NewTokenEstimator("claude-code", reporter)

	est.AddOutput("Some output text for testing")
	est.EmitUpdate()

	select {
	case event := <-ch:
		if event.Type != "tokens" {
			t.Errorf("Expected 'tokens' event, got %q", event.Type)
		}
		if event.TokensAvail != 200000 {
			t.Errorf("Expected TokensAvail 200000, got %d", event.TokensAvail)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Expected tokens event but none received")
	}
}

func TestModelContextWindowValues(t *testing.T) {
	// Verify key models have expected context windows
	tests := []struct {
		model string
		want  int
	}{
		{"claude-code", 200000},
		{"gpt-4", 128000},
		{"gpt-4.1", 1000000},
		{"gemini-2.5-pro", 1000000},
		{"gemini-1.5-pro", 2000000},
		{"o3-mini", 200000},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got, ok := ModelContextWindow[tt.model]
			if !ok {
				t.Errorf("Model %s not found in ModelContextWindow", tt.model)
				return
			}
			if got != tt.want {
				t.Errorf("ModelContextWindow[%s] = %d, want %d", tt.model, got, tt.want)
			}
		})
	}
}
