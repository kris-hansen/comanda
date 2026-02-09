package processor

import (
	"bufio"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DebugWatcher monitors a claude-code debug file for context usage and other metrics
type DebugWatcher struct {
	path       string
	streamLog  *StreamLogger
	stop       chan struct{}
	wg         sync.WaitGroup
	lastOffset int64
}

// NewDebugWatcher creates a watcher for the specified debug file
func NewDebugWatcher(debugFilePath string, streamLog *StreamLogger) *DebugWatcher {
	return &DebugWatcher{
		path:      debugFilePath,
		streamLog: streamLog,
		stop:      make(chan struct{}),
	}
}

// Start begins watching the debug file
func (w *DebugWatcher) Start() {
	w.wg.Add(1)
	go w.watch()
}

// Stop stops the watcher
func (w *DebugWatcher) Stop() {
	close(w.stop)
	w.wg.Wait()
}

// watch polls the debug file for new content
func (w *DebugWatcher) watch() {
	defer w.wg.Done()

	// Patterns to look for
	// Example: "Auto tool search enabled: 50216 tokens (threshold: 20000, 10% of context)"
	tokenPattern := regexp.MustCompile(`(\d+)\s+tokens?\s*\(threshold:\s*(\d+),\s*([\d.]+)%\s*of\s*context\)`)

	// Context window pattern from API responses
	// Example: "context window: 180000 tokens"
	windowPattern := regexp.MustCompile(`context\s*window[:\s]+(\d+)\s*tokens?`)

	// Tool use patterns
	toolStartPattern := regexp.MustCompile(`\[Tool:\s*(\w+)\]|Running\s+tool[:\s]+(\w+)|Executing[:\s]+(\w+)`)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var lastContextPct float64
	var effectiveWindow int = 180000 // Default assumption

	for {
		select {
		case <-w.stop:
			return
		case <-ticker.C:
			w.parseNewLines(tokenPattern, windowPattern, toolStartPattern, &lastContextPct, &effectiveWindow)
		}
	}
}

// parseNewLines reads new lines from the debug file and extracts metrics
func (w *DebugWatcher) parseNewLines(
	tokenPattern, windowPattern, toolStartPattern *regexp.Regexp,
	lastContextPct *float64,
	effectiveWindow *int,
) {
	file, err := os.Open(w.path)
	if err != nil {
		return // File might not exist yet
	}
	defer file.Close()

	// Seek to last position
	if w.lastOffset > 0 {
		file.Seek(w.lastOffset, 0)
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Check for context window info
		if matches := windowPattern.FindStringSubmatch(line); len(matches) > 1 {
			if window, err := strconv.Atoi(matches[1]); err == nil {
				*effectiveWindow = window
			}
		}

		// Check for token usage
		if matches := tokenPattern.FindStringSubmatch(line); len(matches) > 3 {
			usedTokens, _ := strconv.Atoi(matches[1])
			thresholdTokens, _ := strconv.Atoi(matches[2])
			percentage, _ := strconv.ParseFloat(matches[3], 64)

			// Only log if percentage changed significantly (avoid spam)
			if percentage-*lastContextPct >= 5 || (percentage >= 80 && percentage-*lastContextPct >= 2) {
				*lastContextPct = percentage
				w.streamLog.LogContextUsage(usedTokens, thresholdTokens, *effectiveWindow, percentage)
			}
		}

		// Check for tool use
		if matches := toolStartPattern.FindStringSubmatch(line); len(matches) > 0 {
			toolName := ""
			for i := 1; i < len(matches); i++ {
				if matches[i] != "" {
					toolName = matches[i]
					break
				}
			}
			if toolName != "" && w.streamLog != nil {
				w.streamLog.Log("🔧 Tool: %s", toolName)
			}
		}

		// Check for API streaming start
		if strings.Contains(line, "Stream started") {
			w.streamLog.Log("📡 API stream started")
		}

		// Check for errors
		if strings.Contains(line, "ERROR") || strings.Contains(line, "Error:") {
			// Extract just the error part, not the whole line
			if idx := strings.Index(line, "ERROR"); idx >= 0 {
				w.streamLog.Log("⚠️  %s", line[idx:])
			} else if idx := strings.Index(line, "Error:"); idx >= 0 {
				w.streamLog.Log("⚠️  %s", line[idx:])
			}
		}
	}

	// Update offset for next read
	w.lastOffset, _ = file.Seek(0, 1)
}
