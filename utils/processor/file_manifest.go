package processor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileManifest represents a token-aware file listing for agentic loops
type FileManifest struct {
	Root        string
	TotalFiles  int
	SafeFiles   []ManifestEntry // < 10k tokens
	LargeFiles  []ManifestEntry // 10k-25k tokens
	Oversized   []ManifestEntry // > 25k tokens
	TotalTokens int
}

// ManifestEntry represents a single file in the manifest
type ManifestEntry struct {
	Path            string
	Size            int64
	EstimatedTokens int
}

// Token thresholds (matching claude-code limits)
const (
	tokenSafeThreshold  = 10000
	tokenLargeThreshold = 25000
)

// GenerateFileManifest scans paths and generates a token-aware manifest
func GenerateFileManifest(paths []string) (*FileManifest, error) {
	manifest := &FileManifest{}

	for _, root := range paths {
		// Expand path
		if strings.HasPrefix(root, "~") {
			home, _ := os.UserHomeDir()
			root = filepath.Join(home, root[1:])
		}

		info, err := os.Stat(root)
		if err != nil {
			continue
		}

		if info.IsDir() {
			manifest.Root = root
			if err := scanDir(root, manifest); err != nil {
				return nil, err
			}
		} else {
			addFileToManifest(root, info, manifest)
		}
	}

	// Sort each category by token count (descending)
	sort.Slice(manifest.Oversized, func(i, j int) bool {
		return manifest.Oversized[i].EstimatedTokens > manifest.Oversized[j].EstimatedTokens
	})
	sort.Slice(manifest.LargeFiles, func(i, j int) bool {
		return manifest.LargeFiles[i].EstimatedTokens > manifest.LargeFiles[j].EstimatedTokens
	})

	return manifest, nil
}

// scanDir recursively scans a directory for files
func scanDir(root string, manifest *FileManifest) error {
	return filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip hidden directories and common non-code dirs
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip hidden files and non-text files
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		addFileToManifest(path, info, manifest)
		return nil
	})
}

// addFileToManifest categorizes a file by token count
func addFileToManifest(path string, info os.FileInfo, manifest *FileManifest) {
	// Skip binary/non-text extensions
	ext := strings.ToLower(filepath.Ext(path))
	binaryExts := map[string]bool{
		".exe": true, ".bin": true, ".so": true, ".dylib": true,
		".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".ico": true,
		".zip": true, ".tar": true, ".gz": true, ".rar": true,
		".pdf": true, ".doc": true, ".docx": true,
		".mp3": true, ".mp4": true, ".avi": true, ".mov": true,
		".woff": true, ".woff2": true, ".ttf": true, ".eot": true,
	}
	if binaryExts[ext] {
		return
	}

	manifest.TotalFiles++
	tokens := int(info.Size() / 4) // Rough estimate
	manifest.TotalTokens += tokens

	entry := ManifestEntry{
		Path:            path,
		Size:            info.Size(),
		EstimatedTokens: tokens,
	}

	if tokens >= tokenLargeThreshold {
		manifest.Oversized = append(manifest.Oversized, entry)
	} else if tokens >= tokenSafeThreshold {
		manifest.LargeFiles = append(manifest.LargeFiles, entry)
	} else {
		manifest.SafeFiles = append(manifest.SafeFiles, entry)
	}
}

// String returns a human-readable manifest for injection into prompts
func (m *FileManifest) String() string {
	var sb strings.Builder

	sb.WriteString("## 📁 File Manifest (Token Budget)\n\n")
	sb.WriteString(fmt.Sprintf("**Total:** %d files, ~%dk tokens estimated\n\n", m.TotalFiles, m.TotalTokens/1000))

	if len(m.Oversized) > 0 {
		sb.WriteString("### ❌ Oversized (>25k tokens) — DO NOT read directly\n")
		sb.WriteString("Use `grep` to search or read with `offset`/`limit` parameters.\n\n")
		for _, f := range m.Oversized {
			relPath := f.Path
			if m.Root != "" {
				relPath, _ = filepath.Rel(m.Root, f.Path)
			}
			sb.WriteString(fmt.Sprintf("- `%s` (~%dk tokens, %d bytes)\n", relPath, f.EstimatedTokens/1000, f.Size))
		}
		sb.WriteString("\n")
	}

	if len(m.LargeFiles) > 0 {
		sb.WriteString("### ⚠️ Large (10k-25k tokens) — Read with care\n")
		sb.WriteString("Consider if you need the full file or just specific sections.\n\n")
		for _, f := range m.LargeFiles {
			relPath := f.Path
			if m.Root != "" {
				relPath, _ = filepath.Rel(m.Root, f.Path)
			}
			sb.WriteString(fmt.Sprintf("- `%s` (~%dk tokens)\n", relPath, f.EstimatedTokens/1000))
		}
		sb.WriteString("\n")
	}

	if len(m.Oversized) > 0 || len(m.LargeFiles) > 0 {
		sb.WriteString("### ✅ Safe files\n")
		sb.WriteString(fmt.Sprintf("%d files under 10k tokens — safe to read fully.\n\n", len(m.SafeFiles)))
	}

	return sb.String()
}
