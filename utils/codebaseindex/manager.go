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

	for i, plugin := range config.ParserPlugins {
		adapter, err := NewParserPluginAdapter(plugin, config.Root)
		if err != nil {
			return nil, err
		}
		if _, exists := m.registry.Get(adapter.Name()); exists {
			return nil, fmt.Errorf("parser plugin %q conflicts with an existing adapter", adapter.Name())
		}
		m.registry.Register(adapter)
		// Keep later adapter selection consistent with normalized plugin names
		// and extensions (for example, "templatex" becomes ".templatex").
		config.ParserPlugins[i] = adapter.config
	}

	return m, nil
}

// Scan runs the deterministic front half of index generation — adapter
// detection, repository scan, symbol extraction, and component analysis —
// without synthesizing markdown. It returns the populated ScanResult and the
// detected language names. Knowledge-graph building and other consumers use
// this to get structured data without producing an index file.
func (m *Manager) Scan() (*ScanResult, []string, error) {
	// Check if root path exists
	if _, err := os.Stat(m.config.Root); os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("repository path does not exist: %s", m.config.Root)
	}

	// Step 1: Detect or use specified adapters
	m.reportProgress(ProgressEvent{Phase: "Detecting languages", Current: m.config.Root})
	m.adapters = m.detectAdapters()
	if len(m.adapters) == 0 {
		return nil, nil, fmt.Errorf("no language adapters detected for repository at %s (supported: Go, Python, TypeScript, Flutter, Java)", m.config.Root)
	}

	languages := make([]string, len(m.adapters))
	for i, a := range m.adapters {
		languages[i] = a.Name()
	}
	m.logf("Using adapters: %v", languages)

	// Step 2: Scan repository
	m.logf("Scanning repository...")
	m.reportProgress(ProgressEvent{Phase: "Scanning source files", Current: m.config.Root})
	scanResult, err := m.scanRepository()
	if err != nil {
		return nil, nil, fmt.Errorf("scan failed: %w", err)
	}
	m.logf("Found %d files, selected %d candidates", scanResult.TotalFiles, len(scanResult.Candidates))
	m.reportProgress(ProgressEvent{
		Phase:     "Selecting graph candidates",
		Current:   fmt.Sprintf("%d source files", scanResult.TotalFiles),
		Completed: len(scanResult.Candidates),
		Total:     len(scanResult.Candidates),
	})

	// Step 3: Extract symbols from candidates
	m.logf("Extracting symbols...")
	m.reportProgress(ProgressEvent{Phase: "Extracting symbols", Total: len(scanResult.Files)})
	scanResult.GraphFiles = scanResult.Files
	if err := m.extractSymbols(scanResult.GraphFiles); err != nil {
		return nil, nil, fmt.Errorf("symbol extraction failed: %w", err)
	}

	// Step 3b: Infer macro components after symbol extraction so monorepos retain
	// frontend/backend/package boundaries in the generated index.
	m.reportProgress(ProgressEvent{Phase: "Mapping repository components"})
	m.analyzeComponents(scanResult)
	m.resolvePackagePaths(scanResult.GraphFiles)

	return scanResult, languages, nil
}

func (m *Manager) reportProgress(event ProgressEvent) {
	if m.config.Progress != nil {
		m.config.Progress(event)
	}
}

// Generate creates the codebase index
func (m *Manager) Generate() (*Result, error) {
	startTime := time.Now()

	m.logf("Starting codebase index generation for: %s", m.config.Root)

	scanResult, languages, err := m.Scan()
	if err != nil {
		return nil, err
	}

	// Step 4: Synthesize markdown
	m.logf("Synthesizing index...")
	content, err := m.synthesize(scanResult)
	if err != nil {
		return nil, fmt.Errorf("synthesis failed: %w", err)
	}

	// Optional Step 4b: second-pass AI macro analysis. The first pass above stays
	// deterministic and fast; this pass asks the configured generation model to
	// turn the scan/index into deeper repo-specific guidance.
	if m.config.EnhanceIndex {
		m.logf("Enhancing index with model: %s", m.config.EnhancementModel)
		content, err = m.enhanceIndex(scanResult, content)
		if err != nil {
			return nil, fmt.Errorf("AI index enhancement failed: %w", err)
		}
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
	if err := m.SaveMetadata(result, scanResult.GraphFiles); err != nil {
		return nil, fmt.Errorf("failed to save index metadata: %w", err)
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
		seen := make(map[string]bool)
		var names []string
		for name := range m.config.AdapterOverrides {
			if !seen[name] {
				names = append(names, name)
				seen[name] = true
			}
		}
		// Parser plugins are an explicit opt-in. Keep them active even when a
		// workflow also narrows the built-in adapter set with overrides.
		for _, plugin := range m.config.ParserPlugins {
			if !seen[plugin.Name] {
				names = append(names, plugin.Name)
				seen[plugin.Name] = true
			}
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
	if storedMeta.Version != symbolCacheVersion || storedMeta.Root != m.config.Root {
		m.logf("Upgrading index structure metadata")
		result, err := m.Generate()
		return result, false, err
	}
	m.config.SymbolCache = LoadIndexSymbolCache(storedIndexPath, m.config.Root)

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
	if totalChanges == 0 && !m.config.EnhanceIndex && len(m.config.ParserPlugins) == 0 &&
		storedMeta.MaxFiles == m.config.MaxFiles && storedMeta.MaxFilesPerDir == m.config.MaxFilesPerDir &&
		storedMeta.Format == m.config.OutputFormat {
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

	// Regenerate both views from the same scan, reusing content-validated symbols
	// for unchanged files. This also refreshes components and directory structure.
	result, err := m.Generate()
	return result, true, err
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
