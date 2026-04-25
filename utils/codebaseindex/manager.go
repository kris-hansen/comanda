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

	// Check if root path exists
	if _, err := os.Stat(m.config.Root); os.IsNotExist(err) {
		return nil, fmt.Errorf("repository path does not exist: %s", m.config.Root)
	}

	// Step 1: Detect or use specified adapters
	m.adapters = m.detectAdapters()
	if len(m.adapters) == 0 {
		return nil, fmt.Errorf("no language adapters detected for repository at %s (supported: Go, Python, TypeScript, Flutter, Java)", m.config.Root)
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

	// Step 6b: Save metadata for future diffing
	result := &Result{
		Content:     content,
		OutputPath:  outputPath,
		ContentHash: contentHash,
		Updated:     true,
		Format:      m.config.OutputFormat,
		GeneratedAt: time.Now(),
		RepoName:    m.config.RepoFileSlug,
		Languages:   languages,
		FileCount:   len(scanResult.Candidates),
	}
	if err := m.SaveMetadata(result, scanResult.Candidates); err != nil {
		m.logf("Warning: failed to save metadata: %v", err)
	}

	// Step 7: Register with qmd (if configured)
	if m.config.Qmd != nil && m.config.Qmd.Collection != "" {
		if err := m.registerWithQmd(outputPath); err != nil {
			m.logf("Warning: qmd registration failed: %v", err)
		}
	}

	duration := time.Since(startTime)
	m.logf("Index generation completed in %v", duration)

	// Update result with duration
	result.Duration = duration

	return result, nil
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

// GenerateIncremental performs an incremental update based on file changes
func (m *Manager) GenerateIncremental(storedIndexPath string) (*Result, bool, error) {
	startTime := time.Now()

	m.logf("Starting incremental index update for: %s", m.config.Root)

	// Load stored metadata
	metadataPath := storedIndexPath + ".meta.json"
	storedMeta, err := loadMetadata(metadataPath)
	if err != nil {
		m.logf("No metadata file found at %s, falling back to full regeneration", metadataPath)
		result, err := m.Generate()
		return result, false, err
	}

	// Detect adapters
	m.adapters = m.detectAdapters()
	if len(m.adapters) == 0 {
		return nil, false, fmt.Errorf("no language adapters detected")
	}

	// Compute diff
	diffResult, err := m.Diff(storedIndexPath)
	if err != nil {
		m.logf("Diff computation failed, falling back to full regeneration: %v", err)
		result, err := m.Generate()
		return result, false, err
	}

	totalChanges := len(diffResult.Added) + len(diffResult.Modified) + len(diffResult.Deleted)
	m.logf("Diff: %d added, %d modified, %d deleted, %d unchanged",
		len(diffResult.Added), len(diffResult.Modified), len(diffResult.Deleted), diffResult.Unchanged)

	// If no changes, return early
	if totalChanges == 0 {
		m.logf("No changes detected, index is up to date")
		return &Result{
			OutputPath:  storedIndexPath,
			ContentHash: storedMeta.ContentHash,
			Updated:     false,
			Format:      m.config.OutputFormat,
			GeneratedAt: storedMeta.GeneratedAt,
			RepoName:    storedMeta.RepoName,
			Languages:   storedMeta.Languages,
			FileCount:   storedMeta.FileCount,
			Duration:    time.Since(startTime),
		}, true, nil
	}

	// If more than 50% of files changed, do a full regeneration (more efficient)
	changeRatio := float64(totalChanges) / float64(storedMeta.FileCount+len(diffResult.Added))
	if changeRatio > 0.5 {
		m.logf("More than 50%% of files changed (%.1f%%), performing full regeneration", changeRatio*100)
		result, err := m.Generate()
		return result, false, err
	}

	// Build set of unchanged file paths for quick lookup
	unchangedPaths := make(map[string]bool)
	changedPaths := make(map[string]bool)

	for _, p := range diffResult.Added {
		changedPaths[p] = true
	}
	for _, p := range diffResult.Modified {
		changedPaths[p] = true
	}
	for _, p := range diffResult.Deleted {
		changedPaths[p] = true
	}

	for _, f := range storedMeta.Files {
		if !changedPaths[f.Path] {
			unchangedPaths[f.Path] = true
		}
	}

	m.logf("Processing %d changed files incrementally...", len(diffResult.Added)+len(diffResult.Modified))

	// Scan only the changed/added files
	changedFiles := make([]*FileEntry, 0, len(diffResult.Added)+len(diffResult.Modified))
	for _, path := range append(diffResult.Added, diffResult.Modified...) {
		fullPath := filepath.Join(m.config.Root, path)
		entry := m.processFile(fullPath)
		if entry != nil {
			changedFiles = append(changedFiles, entry)
		}
	}

	// Extract symbols for changed files only
	m.logf("Extracting symbols for %d changed files...", len(changedFiles))
	if err := m.extractSymbols(changedFiles); err != nil {
		return nil, false, fmt.Errorf("symbol extraction failed: %w", err)
	}

	// Now we need to scan unchanged files (for synthesis) but skip symbol extraction
	// This is the key optimization: we don't re-extract symbols for unchanged files
	unchangedFiles := make([]*FileEntry, 0, len(unchangedPaths))
	for path := range unchangedPaths {
		fullPath := filepath.Join(m.config.Root, path)
		entry := m.processFile(fullPath)
		if entry != nil {
			// Mark as unchanged so synthesis can use cached data if available
			unchangedFiles = append(unchangedFiles, entry)
		}
	}

	// Combine all candidates
	allCandidates := append(changedFiles, unchangedFiles...)

	// Build scan result for synthesis
	scanResult := &ScanResult{
		Candidates: allCandidates,
		Files:      allCandidates,
		TotalFiles: len(allCandidates),
	}

	// Rebuild directory tree
	scanResult.DirTree = m.buildDirTree(allCandidates)

	// Synthesize markdown
	m.logf("Synthesizing index...")
	content, err := m.synthesize(scanResult)
	if err != nil {
		return nil, false, fmt.Errorf("synthesis failed: %w", err)
	}

	// Compute hash
	contentHash := m.computeHash([]byte(content))

	// Write output
	outputPath, err := m.writeOutput(content)
	if err != nil {
		return nil, false, fmt.Errorf("failed to write output: %w", err)
	}
	m.logf("Index written to: %s", outputPath)

	languages := make([]string, len(m.adapters))
	for i, a := range m.adapters {
		languages[i] = a.Name()
	}

	result := &Result{
		Content:     content,
		OutputPath:  outputPath,
		ContentHash: contentHash,
		Updated:     true,
		Format:      m.config.OutputFormat,
		GeneratedAt: time.Now(),
		RepoName:    m.config.RepoFileSlug,
		Languages:   languages,
		FileCount:   len(allCandidates),
	}

	// Save metadata for future diffing
	if err := m.SaveMetadata(result, allCandidates); err != nil {
		m.logf("Warning: failed to save metadata: %v", err)
	}

	duration := time.Since(startTime)
	result.Duration = duration
	m.logf("Incremental index update completed in %v", duration)

	return result, true, nil
}

// buildDirTree constructs a directory tree from file entries
func (m *Manager) buildDirTree(files []*FileEntry) *DirNode {
	root := &DirNode{
		Name:     filepath.Base(m.config.Root),
		Path:     m.config.Root,
		Children: make([]*DirNode, 0),
	}

	// Build tree structure
	dirNodes := make(map[string]*DirNode)
	dirNodes["."] = root

	for _, f := range files {
		dir := filepath.Dir(f.Path)
		if dir == "." {
			root.Files = append(root.Files, filepath.Base(f.Path))
			continue
		}

		// Ensure all parent directories exist in tree
		parts := strings.Split(dir, string(filepath.Separator))
		currentPath := ""
		parent := root

		for i, part := range parts {
			if currentPath == "" {
				currentPath = part
			} else {
				currentPath = filepath.Join(currentPath, part)
			}

			if node, exists := dirNodes[currentPath]; exists {
				parent = node
			} else {
				newNode := &DirNode{
					Name:     part,
					Path:     currentPath,
					Children: make([]*DirNode, 0),
					Depth:    i + 1,
				}
				parent.Children = append(parent.Children, newNode)
				dirNodes[currentPath] = newNode
				parent = newNode
			}
		}

		// Add file to its directory
		parent.Files = append(parent.Files, filepath.Base(f.Path))
	}

	return root
}
