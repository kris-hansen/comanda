package processor

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// qmd search timeouts based on mode
const (
	qmdSearchTimeout  = 30 * time.Second  // BM25 is fast
	qmdVsearchTimeout = 2 * time.Minute   // Vector search can be slow (model loading)
	qmdQueryTimeout   = 5 * time.Minute   // Hybrid + reranking is slowest
)

// processQmdSearchStep handles the qmd-search step type
func (p *Processor) processQmdSearchStep(step Step, isParallel bool, parallelID string) (string, error) {
	p.debugf("Processing qmd-search step: %s", step.Name)

	config := step.Config.QmdSearch
	if config == nil {
		return "", fmt.Errorf("qmd_search configuration is required")
	}

	// Check if qmd is available
	qmdPath, err := exec.LookPath("qmd")
	if err != nil {
		return "", fmt.Errorf("qmd not found in PATH: %w (install with: bun install -g @tobilu/qmd)", err)
	}

	// Expand variables in query
	query := p.substituteVariables(config.Query)
	if query == "" {
		return "", fmt.Errorf("qmd search query cannot be empty")
	}

	// Determine search mode and timeout
	mode := config.Mode
	if mode == "" {
		mode = "search" // Default to BM25 (fastest)
	}

	// Set timeout based on mode
	var timeout time.Duration
	switch mode {
	case "vsearch":
		timeout = qmdVsearchTimeout
	case "query":
		timeout = qmdQueryTimeout
	default:
		timeout = qmdSearchTimeout
	}

	// Build command arguments
	args := []string{mode, query}

	// Add collection filter
	if config.Collection != "" {
		args = append(args, "-c", config.Collection)
	}

	// Add limit
	limit := config.Limit
	if limit <= 0 {
		limit = 5
	}
	args = append(args, "-n", strconv.Itoa(limit))

	// Add min score
	if config.MinScore > 0 {
		args = append(args, "--min-score", fmt.Sprintf("%.2f", config.MinScore))
	}

	// Add format options
	switch config.Format {
	case "json":
		args = append(args, "--json")
	case "files":
		args = append(args, "--files")
	}

	// Add full flag
	if config.Full {
		args = append(args, "--full")
	}

	p.debugf("Executing: qmd %s (timeout: %v)", strings.Join(args, " "), timeout)

	// Execute qmd with timeout
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, qmdPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("qmd %s timed out after %v", mode, timeout)
		}
		return "", fmt.Errorf("qmd search failed: %s: %w", stderr.String(), err)
	}

	result := strings.TrimSpace(stdout.String())
	if result == "" {
		result = "No results found."
	}

	p.debugf("qmd search returned %d bytes", len(result))

	return result, nil
}
