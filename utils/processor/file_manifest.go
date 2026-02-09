package processor

import (
	"github.com/kris-hansen/comanda/utils/filescan"
)

// GenerateFileManifest scans paths and generates a token-aware manifest
// This is a convenience wrapper around filescan.ScanPaths
func GenerateFileManifest(paths []string) (*filescan.ScanResult, error) {
	opts := filescan.DefaultOptions()
	return filescan.ScanPaths(paths, opts)
}
