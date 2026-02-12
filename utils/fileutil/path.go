package fileutil

import (
	"os"
	"path/filepath"
	"strings"
)

// ExpandPath expands ~ and ~user to the user's home directory.
// It also expands environment variables and resolves relative paths to absolute.
// This ensures paths like "." and "./subdir" become absolute paths based on cwd.
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

	// Resolve to absolute path (handles ".", "..", and relative paths)
	// This ensures paths are resolved relative to where comanda is invoked
	absPath, err := filepath.Abs(path)
	if err != nil {
		// filepath.Abs rarely fails, but if it does, return the error
		return "", err
	}
	return absPath, nil
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
