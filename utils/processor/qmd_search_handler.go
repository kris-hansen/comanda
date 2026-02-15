package processor

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
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

	// Determine search mode
	mode := config.Mode
	if mode == "" {
		mode = "search" // Default to BM25 (fastest)
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

	p.debugf("Executing: qmd %s", strings.Join(args, " "))

	// Execute qmd
	cmd := exec.Command(qmdPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("qmd search failed: %s: %w", stderr.String(), err)
	}

	result := strings.TrimSpace(stdout.String())
	if result == "" {
		result = "No results found."
	}

	p.debugf("qmd search returned %d bytes", len(result))

	return result, nil
}
