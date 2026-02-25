package codebaseindex

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// IndexMetadata stores metadata about an index for diffing
type IndexMetadata struct {
	GeneratedAt time.Time         `json:"generated_at"`
	RepoName    string            `json:"repo_name"`
	ContentHash string            `json:"content_hash"`
	FileCount   int               `json:"file_count"`
	Files       []FileMetadata    `json:"files"`
	Languages   []string          `json:"languages"`
}

// FileMetadata stores per-file metadata for diffing
type FileMetadata struct {
	Path    string `json:"path"`
	Hash    string `json:"hash"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time"` // Unix timestamp
}

// DiffResult contains the results of comparing current state to stored index
type DiffResult struct {
	Added      []string
	Modified   []string
	Deleted    []string
	Unchanged  int
	StoredAt   time.Time
	StoredHash string
}

// Diff compares the current state of the repository against a stored index
func (m *Manager) Diff(storedIndexPath string) (*DiffResult, error) {
	m.logf("Computing diff for: %s", m.config.Root)

	// Try to load metadata file first
	metadataPath := storedIndexPath + ".meta.json"
	storedMeta, err := loadMetadata(metadataPath)
	if err != nil {
		m.logf("No metadata file found, falling back to index parsing")
		// Fall back to parsing the index file itself
		storedMeta, err = m.parseIndexForFiles(storedIndexPath)
		if err != nil {
			return nil, err
		}
	}

	// Scan current state
	m.adapters = m.detectAdapters()
	scanResult, err := m.scanRepository()
	if err != nil {
		return nil, err
	}

	// Build map of current files
	currentFiles := make(map[string]*FileEntry)
	for _, f := range scanResult.Candidates {
		currentFiles[f.Path] = f
	}

	// Build map of stored files
	storedFiles := make(map[string]FileMetadata)
	for _, f := range storedMeta.Files {
		storedFiles[f.Path] = f
	}

	result := &DiffResult{
		StoredAt:   storedMeta.GeneratedAt,
		StoredHash: storedMeta.ContentHash,
	}

	// Find added and modified
	for path, current := range currentFiles {
		if stored, exists := storedFiles[path]; exists {
			// Check if modified (by hash or size)
			if stored.Hash != current.Hash || stored.Size != current.Size {
				result.Modified = append(result.Modified, path)
			} else {
				result.Unchanged++
			}
		} else {
			result.Added = append(result.Added, path)
		}
	}

	// Find deleted
	for path := range storedFiles {
		if _, exists := currentFiles[path]; !exists {
			result.Deleted = append(result.Deleted, path)
		}
	}

	return result, nil
}

// SaveMetadata saves index metadata for future diffing
func (m *Manager) SaveMetadata(result *Result, candidates []*FileEntry) error {
	meta := IndexMetadata{
		GeneratedAt: result.GeneratedAt,
		RepoName:    result.RepoName,
		ContentHash: result.ContentHash,
		FileCount:   result.FileCount,
		Languages:   result.Languages,
		Files:       make([]FileMetadata, len(candidates)),
	}

	for i, f := range candidates {
		meta.Files[i] = FileMetadata{
			Path:    f.Path,
			Hash:    f.Hash,
			Size:    f.Size,
			ModTime: f.ModTime.Unix(),
		}
	}

	metaPath := result.OutputPath + ".meta.json"
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metaPath, data, 0644)
}

// loadMetadata loads stored metadata from a JSON file
func loadMetadata(path string) (*IndexMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var meta IndexMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

// parseIndexForFiles extracts file list from an index markdown file
// This is a fallback when no metadata file exists
func (m *Manager) parseIndexForFiles(indexPath string) (*IndexMetadata, error) {
	f, err := os.Open(indexPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	meta := &IndexMetadata{
		Files: []FileMetadata{},
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		
		// Look for file entries (lines starting with "- `" or "- ")
		if strings.HasPrefix(line, "- `") {
			// Extract path from markdown link or code block
			path := strings.TrimPrefix(line, "- `")
			if idx := strings.Index(path, "`"); idx > 0 {
				path = path[:idx]
			}
			if path != "" && !strings.HasPrefix(path, "#") {
				meta.Files = append(meta.Files, FileMetadata{
					Path: path,
				})
			}
		} else if strings.HasPrefix(line, "- ") && strings.Contains(line, "/") {
			// Plain file path
			path := strings.TrimPrefix(line, "- ")
			path = strings.TrimSpace(path)
			if filepath.Ext(path) != "" {
				meta.Files = append(meta.Files, FileMetadata{
					Path: path,
				})
			}
		}
	}

	// Get file info for stored index
	if info, err := os.Stat(indexPath); err == nil {
		meta.GeneratedAt = info.ModTime()
	}

	return meta, scanner.Err()
}

// ComputeChangeSet builds a ChangeSet from diff result for incremental updates
func (m *Manager) ComputeChangeSet(diff *DiffResult) *ChangeSet {
	cs := &ChangeSet{
		Deleted: diff.Deleted,
	}

	// For added/modified, we need to get the full FileEntry
	// This would require re-scanning those specific files
	// For now, return paths only (full implementation would populate FileEntry)

	return cs
}
