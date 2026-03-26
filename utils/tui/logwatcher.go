package tui

import (
	"bufio"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// LogWatcher monitors a stream log file and emits progress events
type LogWatcher struct {
	path           string
	reporter       *ProgressReporter
	tokenEstimator *TokenEstimator
	stop           chan struct{}
	done           chan struct{}
	totalChars     int // Track total characters for token estimation
}

// Patterns for parsing stream log
var (
	iterationPattern = regexp.MustCompile(`ITERATION (\d+)/(\d+) - (.+)`)
	contextPattern   = regexp.MustCompile(`CONTEXT: .+ (\d+\.?\d*)% \((\d+)k/(\d+)k`)
	toolPattern      = regexp.MustCompile(`→ (Read|Write|Edit|Bash|execute|calling tool:) (.+)`)
	stepPattern      = regexp.MustCompile(`(Starting|Processing|Completed|Executing) (step|loop): (.+)`)
	errorPattern     = regexp.MustCompile(`(?i)(error|failed|exception): (.+)`)
	modelPattern     = regexp.MustCompile(`(?i)(model|using):\s*(claude-code|claude-[^\s,]+|gpt-[^\s,]+|gemini-[^\s,]+|o[134]-[^\s,]+)`)
)

// NewLogWatcher creates a new log watcher
func NewLogWatcher(path string, reporter *ProgressReporter) *LogWatcher {
	// Create token estimator with default model (will update when model is detected)
	estimator := NewTokenEstimator("claude-code", reporter)

	return &LogWatcher{
		path:           path,
		reporter:       reporter,
		tokenEstimator: estimator,
		stop:           make(chan struct{}),
		done:           make(chan struct{}),
	}
}

// SetModel updates the model for token estimation
func (w *LogWatcher) SetModel(model string) {
	if w.tokenEstimator != nil {
		w.tokenEstimator.SetModel(model)
	}
}

// Start begins watching the log file
func (w *LogWatcher) Start() {
	go w.watch()
}

// Stop stops watching
func (w *LogWatcher) Stop() {
	close(w.stop)
	<-w.done
}

func (w *LogWatcher) watch() {
	defer close(w.done)

	// Wait for file to exist
	var file *os.File
	var err error
	for i := 0; i < 50; i++ { // Wait up to 5 seconds
		file, err = os.Open(w.path)
		if err == nil {
			break
		}
		select {
		case <-w.stop:
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
	if file == nil {
		return
	}
	defer file.Close()

	// Start from end of file
	_, _ = file.Seek(0, 2)

	reader := bufio.NewReader(file)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-w.stop:
			return
		case <-ticker.C:
			for {
				line, err := reader.ReadString('\n')
				if err != nil {
					break
				}
				w.parseLine(strings.TrimSpace(line))
			}
		}
	}
}

func (w *LogWatcher) parseLine(line string) {
	if line == "" {
		return
	}

	// Track all output for token estimation (silently - don't emit event yet)
	w.totalChars += len(line)

	// Remove timestamp prefix [HH:MM:SS]
	cleanLine := line
	if len(line) > 10 && line[0] == '[' {
		if idx := strings.Index(line, "]"); idx > 0 && idx < 15 {
			cleanLine = strings.TrimSpace(line[idx+1:])
		}
	}

	// Check for model mentions to update context window (no event emitted)
	if matches := modelPattern.FindStringSubmatch(cleanLine); matches != nil {
		model := matches[2]
		if w.tokenEstimator != nil {
			w.tokenEstimator.SetModel(model)
		}
	}

	// Check for iteration
	if matches := iterationPattern.FindStringSubmatch(cleanLine); matches != nil {
		current, _ := strconv.Atoi(matches[1])
		total, _ := strconv.Atoi(matches[2])
		loopName := matches[3]
		w.reporter.LoopIteration(loopName, current, total)
		return
	}

	// Check for context usage (if the underlying tool outputs it)
	// This takes precedence over our estimation
	if matches := contextPattern.FindStringSubmatch(cleanLine); matches != nil {
		used, _ := strconv.Atoi(matches[2])
		avail, _ := strconv.Atoi(matches[3])
		w.reporter.TokenUpdate(used*1000, avail*1000)
		return
	}

	// Check for tool calls
	if matches := toolPattern.FindStringSubmatch(cleanLine); matches != nil {
		toolName := matches[1]
		details := matches[2]
		w.reporter.ToolCall(toolName + ": " + details)
		return
	}

	// Check for step info
	if matches := stepPattern.FindStringSubmatch(cleanLine); matches != nil {
		action := matches[1]
		stepName := matches[3]
		if action == "Starting" || action == "Executing" {
			w.reporter.StepStart(stepName)
		} else if action == "Completed" {
			w.reporter.StepEnd(stepName, nil)
		}
		return
	}

	// Check for errors
	if matches := errorPattern.FindStringSubmatch(cleanLine); matches != nil {
		w.reporter.Emit(ProgressEvent{
			Type:    "error",
			Message: matches[2],
		})
		return
	}

	// Generic output for other lines (if they seem meaningful)
	if len(cleanLine) > 5 && !strings.HasPrefix(cleanLine, "─") && !strings.HasPrefix(cleanLine, "═") {
		// Skip decorative lines
		if !strings.HasPrefix(cleanLine, "  ") {
			w.reporter.Output(cleanLine)
		}
	}

	// Track chars for token estimation (silent - added to running total)
	if w.tokenEstimator != nil {
		w.tokenEstimator.AddOutput(line)
		// Emit token update every ~2000 chars to keep UI updated without flooding
		if w.totalChars%2000 < len(line) {
			w.tokenEstimator.EmitUpdate()
		}
	}
}
