package codebaseindex

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
	"github.com/kris-hansen/comanda/utils/filescan"
	gitignore "github.com/sabhiram/go-gitignore"
)

const (
	// maxCandidates is the maximum number of files to select for indexing
	maxCandidates = 100

	// maxHashReadSize is the maximum bytes to read for hashing (1MB)
	maxHashReadSize = 1024 * 1024

	// maxSymbolReadSize is the maximum bytes to read for symbol extraction (32KB)
	maxSymbolReadSize = 32 * 1024
)

// scanRepository walks the repository and collects file information
func (m *Manager) scanRepository() (*ScanResult, error) {
	startTime := time.Now()

	// Build combined ignore rules
	ignoreDirs := m.buildIgnoreDirs()
	ignoreGlobs := m.buildIgnoreGlobs()
	gitIgnore := m.loadGitignore()

	// Get valid extensions for our adapters
	validExtensions := CombinedExtensions(m.adapters)
	extMap := make(map[string]bool)
	for _, ext := range validExtensions {
		extMap[ext] = true
	}

	// Also track config files which may not match extension rules
	configPatterns := m.buildConfigPatterns()

	result := &ScanResult{
		Files:     make([]*FileEntry, 0, 1000),
		DirTree:   &DirNode{Name: filepath.Base(m.config.Root), Path: m.config.Root},
		TotalDirs: 1,
	}

	// File collection channel for parallel processing
	fileChan := make(chan string, 100)
	resultChan := make(chan *FileEntry, 100)
	var wg sync.WaitGroup

	// Start worker pool
	numWorkers := runtime.NumCPU()
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range fileChan {
				entry := m.processFile(path)
				if entry != nil {
					resultChan <- entry
				}
			}
		}()
	}

	// Collector goroutine
	var collectorWg sync.WaitGroup
	collectorWg.Add(1)
	go func() {
		defer collectorWg.Done()
		for entry := range resultChan {
			result.Files = append(result.Files, entry)
		}
	}()

	// Walk the repository using os.ReadDir for performance
	err := m.walkDir(m.config.Root, 0, result.DirTree, ignoreDirs, ignoreGlobs, gitIgnore, extMap, configPatterns, fileChan, result)
	close(fileChan)

	// Wait for workers to finish
	wg.Wait()
	close(resultChan)

	// Wait for collector to finish
	collectorWg.Wait()

	if err != nil {
		return nil, err
	}

	result.TotalFiles = len(result.Files)
	result.ProcessedTime = time.Since(startTime)

	// Select candidates based on scoring
	result.Candidates = m.selectCandidates(result.Files)

	return result, nil
}

// walkDir recursively walks a directory using os.ReadDir (faster than filepath.Walk)
func (m *Manager) walkDir(
	dir string,
	depth int,
	node *DirNode,
	ignoreDirs map[string]bool,
	ignoreGlobs []*gitignore.GitIgnore,
	gitIgnoreRules *gitignore.GitIgnore,
	validExts map[string]bool,
	configPatterns []string,
	fileChan chan<- string,
	result *ScanResult,
) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(dir, name)
		relPath, _ := filepath.Rel(m.config.Root, path)

		if entry.IsDir() {
			// Early pruning for directories
			if m.shouldIgnoreDir(name, relPath, ignoreDirs, gitIgnoreRules) {
				result.IgnoredDirs++
				continue
			}

			result.TotalDirs++
			childNode := &DirNode{
				Name:  name,
				Path:  path,
				Depth: depth + 1,
			}
			node.Children = append(node.Children, childNode)

			// Recurse into subdirectory
			if err := m.walkDir(path, depth+1, childNode, ignoreDirs, ignoreGlobs, gitIgnoreRules, validExts, configPatterns, fileChan, result); err != nil {
				// Log but continue on errors
				m.logf("Warning: error walking %s: %v", path, err)
			}
		} else {
			// Check if file should be processed
			ext := filepath.Ext(name)
			isValidExt := validExts[ext]
			isConfig := m.matchesConfigPattern(name, configPatterns)

			if !isValidExt && !isConfig {
				continue
			}

			// Check gitignore
			if gitIgnoreRules != nil && gitIgnoreRules.MatchesPath(relPath) {
				result.IgnoredFiles++
				continue
			}

			// Check glob ignores
			if m.matchesIgnoreGlob(relPath, ignoreGlobs) {
				result.IgnoredFiles++
				continue
			}

			// Add to directory node
			node.Files = append(node.Files, name)

			// Send to worker pool for processing
			fileChan <- path
		}
	}

	return nil
}

// processFile processes a single file and returns a FileEntry
func (m *Manager) processFile(path string) *FileEntry {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}

	relPath, _ := filepath.Rel(m.config.Root, path)
	depth := strings.Count(relPath, string(os.PathSeparator))

	entry := &FileEntry{
		Path:            relPath,
		Size:            info.Size(),
		ModTime:         info.ModTime(),
		Depth:           depth,
		EstimatedTokens: int(info.Size() / filescan.BytesPerToken),
	}

	// Determine language
	ext := filepath.Ext(path)
	for _, adapter := range m.adapters {
		for _, adapterExt := range adapter.FileExtensions() {
			if ext == adapterExt {
				entry.Language = adapter.Name()
				break
			}
		}
		if entry.Language != "" {
			break
		}
	}

	// Check if entrypoint or config
	name := filepath.Base(path)
	entry.IsEntrypoint = m.isEntrypoint(name, relPath)
	entry.IsConfig = m.isConfigFile(name)

	// Compute hash (use xxhash by default for speed)
	entry.Hash = m.computeFileHash(path)

	// Score the file
	entry.Score = m.scoreFile(entry)

	return entry
}

// selectCandidates selects the top files for indexing based on score
func (m *Manager) selectCandidates(files []*FileEntry) []*FileEntry {
	// Sort by score descending
	sort.Slice(files, func(i, j int) bool {
		return files[i].Score > files[j].Score
	})

	// Select top N
	limit := maxCandidates
	if len(files) < limit {
		limit = len(files)
	}

	return files[:limit]
}

// scoreFile calculates a score for file prioritization
func (m *Manager) scoreFile(entry *FileEntry) int {
	score := 0

	// Adapter-specific scoring
	for _, adapter := range m.adapters {
		if adapter.Name() == entry.Language {
			score += adapter.ScoreFile(entry.Path, entry.Depth, entry.IsEntrypoint, entry.IsConfig)
			break
		}
	}

	// Universal scoring factors
	if entry.IsEntrypoint {
		score += 40
	}
	if entry.IsConfig {
		score += 30
	}
	if entry.Depth <= 2 {
		score += 60 - (entry.Depth * 20) // depth 0 = +60, depth 1 = +40, depth 2 = +20
	}

	// Penalize generated files
	if entry.IsGenerated {
		score -= 100
	}

	return score
}

// buildIgnoreDirs builds a set of directories to ignore
func (m *Manager) buildIgnoreDirs() map[string]bool {
	dirs := make(map[string]bool)

	// Always ignore these
	dirs[".git"] = true
	dirs[".svn"] = true
	dirs[".hg"] = true
	dirs[".idea"] = true
	dirs[".vscode"] = true

	// Add adapter-specific ignores
	for _, dir := range CombinedIgnoreDirs(m.adapters) {
		dirs[dir] = true
	}

	// Add user overrides
	for _, override := range m.config.AdapterOverrides {
		for _, dir := range override.IgnoreDirs {
			dirs[dir] = true
		}
	}

	return dirs
}

// buildIgnoreGlobs compiles glob patterns for ignoring files
func (m *Manager) buildIgnoreGlobs() []*gitignore.GitIgnore {
	patterns := CombinedIgnoreGlobs(m.adapters)

	// Add user overrides
	for _, override := range m.config.AdapterOverrides {
		patterns = append(patterns, override.IgnoreGlobs...)
	}

	var globs []*gitignore.GitIgnore
	for _, pattern := range patterns {
		gi := gitignore.CompileIgnoreLines(pattern)
		globs = append(globs, gi)
	}

	return globs
}

// buildConfigPatterns collects config file patterns from all adapters
func (m *Manager) buildConfigPatterns() []string {
	seen := make(map[string]bool)
	var patterns []string

	for _, adapter := range m.adapters {
		for _, pattern := range adapter.ConfigPatterns() {
			if !seen[pattern] {
				seen[pattern] = true
				patterns = append(patterns, pattern)
			}
		}
	}

	return patterns
}

// loadGitignore loads .gitignore rules from the repository
func (m *Manager) loadGitignore() *gitignore.GitIgnore {
	gitignorePath := filepath.Join(m.config.Root, ".gitignore")
	gi, err := gitignore.CompileIgnoreFile(gitignorePath)
	if err != nil {
		return nil
	}
	return gi
}

// shouldIgnoreDir checks if a directory should be ignored
func (m *Manager) shouldIgnoreDir(name, relPath string, ignoreDirs map[string]bool, gitIgnore *gitignore.GitIgnore) bool {
	// Check direct name match
	if ignoreDirs[name] {
		return true
	}

	// Check gitignore
	if gitIgnore != nil && gitIgnore.MatchesPath(relPath) {
		return true
	}

	return false
}

// matchesIgnoreGlob checks if a path matches any ignore glob
func (m *Manager) matchesIgnoreGlob(path string, globs []*gitignore.GitIgnore) bool {
	for _, gi := range globs {
		if gi.MatchesPath(path) {
			return true
		}
	}
	return false
}

// matchesConfigPattern checks if a filename matches any config pattern
func (m *Manager) matchesConfigPattern(name string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
	}
	return false
}

// isEntrypoint checks if a file is an entrypoint
func (m *Manager) isEntrypoint(name, relPath string) bool {
	for _, adapter := range m.adapters {
		for _, pattern := range adapter.EntrypointPatterns() {
			if matched, _ := filepath.Match(pattern, name); matched {
				return true
			}
			if matched, _ := filepath.Match(pattern, relPath); matched {
				return true
			}
		}
	}
	return false
}

// isConfigFile checks if a file is a config file
func (m *Manager) isConfigFile(name string) bool {
	for _, adapter := range m.adapters {
		for _, pattern := range adapter.ConfigPatterns() {
			if matched, _ := filepath.Match(pattern, name); matched {
				return true
			}
		}
	}
	return false
}

// computeFileHash computes the hash of a file
func (m *Manager) computeFileHash(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	// Limit read size for performance
	limitReader := io.LimitReader(f, maxHashReadSize)

	if m.config.HashAlgorithm == HashSHA256 {
		h := sha256.New()
		if _, err := io.Copy(h, limitReader); err != nil {
			return ""
		}
		return hex.EncodeToString(h.Sum(nil))
	}

	// Default: xxhash
	h := xxhash.New()
	if _, err := io.Copy(h, limitReader); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// computeHash computes a hash of content (for the final index)
func (m *Manager) computeHash(content []byte) string {
	if m.config.HashAlgorithm == HashSHA256 {
		h := sha256.Sum256(content)
		return hex.EncodeToString(h[:])
	}

	// Default: xxhash
	h := xxhash.Sum64(content)
	return hex.EncodeToString([]byte{
		byte(h >> 56), byte(h >> 48), byte(h >> 40), byte(h >> 32),
		byte(h >> 24), byte(h >> 16), byte(h >> 8), byte(h),
	})
}
