package fileutil

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath expands ~ and ~user to the user's home directory.
// It also cleans the path and expands environment variables.
func ExpandPath(path string) (string, error) {
	if path == "" {
		return path, nil
	}

	// Expand environment variables first (e.g., $HOME)
	path = os.ExpandEnv(path)

	// Handle tilde expansion
	if strings.HasPrefix(path, "~") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}

		if path == "~" {
			return homeDir, nil
		}

		if strings.HasPrefix(path, "~/") {
			return filepath.Join(homeDir, path[2:]), nil
		}

		// ~user syntax is not supported, return as-is
		// (would require looking up other users' home dirs)
	}

	return filepath.Clean(path), nil
}

// ExpandPaths expands a slice of paths using ExpandPath.
func ExpandPaths(paths []string) ([]string, error) {
	expanded := make([]string, len(paths))
	for i, p := range paths {
		exp, err := ExpandPath(p)
		if err != nil {
			return nil, err
		}
		expanded[i] = exp
	}
	return expanded, nil
}
