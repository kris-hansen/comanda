package codebaseindex

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Manager orchestrates the codebase indexing process
type Manager struct {
	config   *Config
	registry *Registry
	adapters []Adapter
	verbose  bool
}

// NewManager creates a new index manager with the given configuration
func NewManager(config *Config, verbose bool) (*Manager, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Resolve root path to absolute
	absRoot, err := filepath.Abs(config.Root)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve root path: %w", err)
	}
	config.Root = absRoot

	// Derive repo slugs
	config.RepoFileSlug, config.RepoVarSlug = deriveRepoSlugs(absRoot)

	m := &Manager{
		config:   config,
		registry: NewRegistry(),
		verbose:  verbose,
	}

	return m, nil
}

// Generate creates the codebase index
func (m *Manager) Generate() (*Result, error) {
	startTime := time.Now()

	m.logf("Starting codebase index generation for: %s", m.config.Root)

	// Step 1: Detect or use specified adapters
	m.adapters = m.detectAdapters()
	if len(m.adapters) == 0 {
		return nil, fmt.Errorf("no language adapters detected for repository")
	}

	languages := make([]string, len(m.adapters))
	for i, a := range m.adapters {
		languages[i] = a.Name()
	}
	m.logf("Using adapters: %v", languages)

	// Step 2: Scan repository
	m.logf("Scanning repository...")
	scanResult, err := m.scanRepository()
	if err != nil {
		return nil, fmt.Errorf("scan failed: %w", err)
	}
	m.logf("Found %d files, selected %d candidates", scanResult.TotalFiles, len(scanResult.Candidates))

	// Step 3: Extract symbols from candidates
	m.logf("Extracting symbols...")
	if err := m.extractSymbols(scanResult.Candidates); err != nil {
		return nil, fmt.Errorf("symbol extraction failed: %w", err)
	}

	// Step 4: Synthesize markdown
	m.logf("Synthesizing index...")
	content, err := m.synthesize(scanResult)
	if err != nil {
		return nil, fmt.Errorf("synthesis failed: %w", err)
	}

	// Step 5: Compute hash
	contentHash := m.computeHash([]byte(content))

	// Step 6: Store output
	outputPath, err := m.writeOutput(content)
	if err != nil {
		return nil, fmt.Errorf("failed to write output: %w", err)
	}
	m.logf("Index written to: %s", outputPath)

	duration := time.Since(startTime)
	m.logf("Index generation completed in %v", duration)

	return &Result{
		Content:     content,
		OutputPath:  outputPath,
		ContentHash: contentHash,
		Updated:     true, // TODO: support incremental mode
		GeneratedAt: time.Now(),
		RepoName:    m.config.RepoFileSlug,
		Languages:   languages,
		FileCount:   len(scanResult.Candidates),
		Duration:    duration,
	}, nil
}

// detectAdapters determines which language adapters to use
func (m *Manager) detectAdapters() []Adapter {
	// If adapters are specified via overrides, use those
	if len(m.config.AdapterOverrides) > 0 {
		var names []string
		for name := range m.config.AdapterOverrides {
			names = append(names, name)
		}
		adapters := m.registry.GetByNames(names)
		if len(adapters) > 0 {
			return adapters
		}
	}

	// Auto-detect from repository
	return m.registry.Detect(m.config.Root)
}

// GetConfig returns the current configuration
func (m *Manager) GetConfig() *Config {
	return m.config
}

// deriveRepoSlugs derives the file and variable slugs from the repo path
func deriveRepoSlugs(repoPath string) (fileSlug, varSlug string) {
	// Try to get repo name from git
	repoName := getGitRepoName(repoPath)
	if repoName == "" {
		// Fall back to directory name
		repoName = filepath.Base(repoPath)
	}

	// Normalize: replace non-alphanumeric with underscore
	re := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	normalized := re.ReplaceAllString(repoName, "_")

	// Collapse repeated underscores
	re = regexp.MustCompile(`_+`)
	normalized = re.ReplaceAllString(normalized, "_")

	// Trim leading/trailing underscores
	normalized = strings.Trim(normalized, "_")

	fileSlug = strings.ToLower(normalized)
	varSlug = strings.ToUpper(normalized)

	return fileSlug, varSlug
}

// getGitRepoName gets the repository name from git
func getGitRepoName(repoPath string) string {
	// Check if .git exists
	gitPath := filepath.Join(repoPath, ".git")
	if _, err := os.Stat(gitPath); os.IsNotExist(err) {
		return ""
	}

	// Get the repo root from git
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	topLevel := strings.TrimSpace(string(output))
	return filepath.Base(topLevel)
}

// logf logs a message if verbose mode is enabled
func (m *Manager) logf(format string, args ...interface{}) {
	if m.verbose {
		fmt.Printf("[codebase-index] "+format+"\n", args...)
	}
}
