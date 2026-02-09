// Package filescan provides shared file scanning and token estimation utilities
package filescan

import (
	"os"
	"path/filepath"
	"strings"
)

// Token thresholds (matching claude-code limits)
const (
	TokenThresholdSafe    = 10000  // Files under this are safe to read fully
	TokenThresholdLarge   = 25000  // Files under this can be read with care
	TokenThresholdMax     = 25000  // Claude-code's max file read limit
	BytesPerToken         = 4      // Rough estimate for text files
)

// FileInfo holds basic file information with token estimation
type FileInfo struct {
	Path            string
	RelPath         string // Relative to scan root
	Size            int64
	EstimatedTokens int
	IsDir           bool
}

// TokenCategory returns the token budget category for a file
func (f *FileInfo) TokenCategory() string {
	if f.EstimatedTokens < TokenThresholdSafe {
		return "safe"
	} else if f.EstimatedTokens < TokenThresholdLarge {
		return "large"
	}
	return "oversized"
}

// ScanResult holds the results of a directory scan
type ScanResult struct {
	Root        string
	Files       []FileInfo
	TotalFiles  int
	TotalTokens int
	SafeCount   int
	LargeCount  int
	OversizedCount int
}

// ScanOptions configures the scanner behavior
type ScanOptions struct {
	// IgnoreDirs is a set of directory names to skip
	IgnoreDirs map[string]bool
	
	// IgnoreHidden skips files and dirs starting with "."
	IgnoreHidden bool
	
	// IgnoreBinary skips common binary file extensions
	IgnoreBinary bool
	
	// MaxDepth limits recursion depth (0 = unlimited)
	MaxDepth int
}

// DefaultOptions returns sensible defaults for code scanning
func DefaultOptions() ScanOptions {
	return ScanOptions{
		IgnoreDirs: map[string]bool{
			"node_modules": true,
			"vendor":       true,
			"__pycache__":  true,
			".git":         true,
			".svn":         true,
			".hg":          true,
			"dist":         true,
			"build":        true,
			"target":       true,
		},
		IgnoreHidden: true,
		IgnoreBinary: true,
		MaxDepth:     0,
	}
}

// BinaryExtensions is the set of extensions to skip when IgnoreBinary is true
var BinaryExtensions = map[string]bool{
	".exe": true, ".bin": true, ".so": true, ".dylib": true, ".dll": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true, ".webp": true,
	".zip": true, ".tar": true, ".gz": true, ".rar": true, ".7z": true,
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".mp3": true, ".mp4": true, ".avi": true, ".mov": true, ".mkv": true,
	".woff": true, ".woff2": true, ".ttf": true, ".eot": true, ".otf": true,
	".pyc": true, ".pyo": true, ".class": true, ".o": true, ".a": true,
	".sqlite": true, ".db": true,
}

// Scan walks a directory and collects file information with token estimates
func Scan(root string, opts ScanOptions) (*ScanResult, error) {
	// Expand ~ in path
	if strings.HasPrefix(root, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		root = filepath.Join(home, root[1:])
	}

	// Make path absolute
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	result := &ScanResult{
		Root:  absRoot,
		Files: make([]FileInfo, 0, 100),
	}

	err = scanDir(absRoot, absRoot, 0, opts, result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// ScanPaths scans multiple paths and combines results
func ScanPaths(paths []string, opts ScanOptions) (*ScanResult, error) {
	combined := &ScanResult{
		Files: make([]FileInfo, 0, 100),
	}

	for _, path := range paths {
		// Expand ~ in path
		if strings.HasPrefix(path, "~") {
			home, _ := os.UserHomeDir()
			path = filepath.Join(home, path[1:])
		}

		info, err := os.Stat(path)
		if err != nil {
			continue // Skip paths that don't exist
		}

		if info.IsDir() {
			if combined.Root == "" {
				combined.Root = path
			}
			result, err := Scan(path, opts)
			if err != nil {
				continue
			}
			// Merge results
			combined.Files = append(combined.Files, result.Files...)
			combined.TotalFiles += result.TotalFiles
			combined.TotalTokens += result.TotalTokens
			combined.SafeCount += result.SafeCount
			combined.LargeCount += result.LargeCount
			combined.OversizedCount += result.OversizedCount
		} else {
			// Single file
			addFile(path, path, info, combined)
		}
	}

	return combined, nil
}

func scanDir(root, dir string, depth int, opts ScanOptions, result *ScanResult) error {
	if opts.MaxDepth > 0 && depth > opts.MaxDepth {
		return nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil // Skip unreadable directories
	}

	for _, entry := range entries {
		name := entry.Name()
		path := filepath.Join(dir, name)

		// Skip hidden files/dirs
		if opts.IgnoreHidden && strings.HasPrefix(name, ".") {
			continue
		}

		if entry.IsDir() {
			// Skip ignored directories
			if opts.IgnoreDirs[name] {
				continue
			}
			if err := scanDir(root, path, depth+1, opts, result); err != nil {
				continue // Skip on error
			}
		} else {
			// Skip binary files
			if opts.IgnoreBinary {
				ext := strings.ToLower(filepath.Ext(name))
				if BinaryExtensions[ext] {
					continue
				}
			}

			info, err := entry.Info()
			if err != nil {
				continue
			}
			addFile(root, path, info, result)
		}
	}

	return nil
}

func addFile(root, path string, info os.FileInfo, result *ScanResult) {
	relPath, _ := filepath.Rel(root, path)
	tokens := int(info.Size() / BytesPerToken)

	file := FileInfo{
		Path:            path,
		RelPath:         relPath,
		Size:            info.Size(),
		EstimatedTokens: tokens,
	}

	result.Files = append(result.Files, file)
	result.TotalFiles++
	result.TotalTokens += tokens

	switch file.TokenCategory() {
	case "safe":
		result.SafeCount++
	case "large":
		result.LargeCount++
	case "oversized":
		result.OversizedCount++
	}
}

// FilterByCategory returns files matching the given category
func (r *ScanResult) FilterByCategory(category string) []FileInfo {
	var filtered []FileInfo
	for _, f := range r.Files {
		if f.TokenCategory() == category {
			filtered = append(filtered, f)
		}
	}
	return filtered
}

// HasLargeFiles returns true if there are any large or oversized files
func (r *ScanResult) HasLargeFiles() bool {
	return r.LargeCount > 0 || r.OversizedCount > 0
}
