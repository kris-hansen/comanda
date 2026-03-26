package tui

import (
	"sync"
)

// ModelContextWindow holds context window sizes for different models
var ModelContextWindow = map[string]int{
	// Claude models
	"claude-code":                200000,
	"claude-sonnet-4-20250514":   200000,
	"claude-opus-4-20250514":     200000,
	"claude-3-5-sonnet-20241022": 200000,
	"claude-3-5-haiku-20241022":  200000,
	"claude-3-opus-20240229":     200000,
	"claude-3-sonnet-20240229":   200000,
	"claude-3-haiku-20240307":    200000,

	// OpenAI models
	"gpt-4":        128000,
	"gpt-4-turbo":  128000,
	"gpt-4o":       128000,
	"gpt-4o-mini":  128000,
	"gpt-4.1":      1000000,
	"gpt-4.1-mini": 1000000,
	"gpt-4.1-nano": 1000000,
	"o1":           200000,
	"o1-mini":      128000,
	"o1-preview":   128000,
	"o3":           200000,
	"o3-mini":      200000,
	"o4-mini":      200000,

	// Google models
	"gemini-2.0-flash":      1000000,
	"gemini-2.0-flash-lite": 1000000,
	"gemini-2.5-pro":        1000000,
	"gemini-2.5-flash":      1000000,
	"gemini-1.5-pro":        2000000,
	"gemini-1.5-flash":      1000000,

	// Defaults
	"default": 128000,
}

// TokenEstimator tracks estimated token usage during workflow execution
type TokenEstimator struct {
	mu           sync.Mutex
	inputChars   int
	outputChars  int
	model        string
	contextLimit int
	reporter     *ProgressReporter
}

// NewTokenEstimator creates a token estimator for a given model
func NewTokenEstimator(model string, reporter *ProgressReporter) *TokenEstimator {
	contextLimit := ModelContextWindow["default"]

	// Try to find exact match
	if limit, ok := ModelContextWindow[model]; ok {
		contextLimit = limit
	} else {
		// Try prefix matching for versioned models
		for prefix, limit := range ModelContextWindow {
			if len(model) > len(prefix) && model[:len(prefix)] == prefix {
				contextLimit = limit
				break
			}
		}
	}

	return &TokenEstimator{
		model:        model,
		contextLimit: contextLimit,
		reporter:     reporter,
	}
}

// AddInput records input characters
// Does not emit events - call EmitUpdate() explicitly when ready
func (t *TokenEstimator) AddInput(text string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.inputChars += len(text)
}

// AddOutput records output characters (can be called incrementally for streaming)
// Does not emit events - call EmitUpdate() explicitly when ready
func (t *TokenEstimator) AddOutput(text string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.outputChars += len(text)
}

// Reset clears the token counts (e.g., between steps)
func (t *TokenEstimator) Reset() {
	t.mu.Lock()
	t.inputChars = 0
	t.outputChars = 0
	t.mu.Unlock()

	t.EmitUpdate()
}

// SetModel updates the model and context limit
func (t *TokenEstimator) SetModel(model string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.model = model
	if limit, ok := ModelContextWindow[model]; ok {
		t.contextLimit = limit
	}
}

// GetEstimates returns estimated token counts
// Uses ~4 characters per token as rough estimate for English text
func (t *TokenEstimator) GetEstimates() (inputTokens, outputTokens, contextLimit int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Rough estimate: 4 chars per token for English, 2-3 for code
	// Using 3.5 as middle ground
	inputTokens = t.inputChars * 10 / 35
	outputTokens = t.outputChars * 10 / 35
	contextLimit = t.contextLimit

	return inputTokens, outputTokens, contextLimit
}

// GetContextPercent returns the estimated context window usage percentage
func (t *TokenEstimator) GetContextPercent() float64 {
	inputTokens, outputTokens, contextLimit := t.GetEstimates()
	if contextLimit == 0 {
		return 0
	}
	totalUsed := inputTokens + outputTokens
	return float64(totalUsed) / float64(contextLimit) * 100
}

// EmitUpdate sends the current token estimate to the reporter
func (t *TokenEstimator) EmitUpdate() {
	if t.reporter == nil {
		return
	}

	inputTokens, outputTokens, contextLimit := t.GetEstimates()
	totalUsed := inputTokens + outputTokens

	t.reporter.TokenUpdate(totalUsed, contextLimit)
}
