package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// GetMemoryPath returns the memory file path by checking in order:
// 1. COMANDA_MEMORY environment variable
// 2. memory_file setting in .env config
// 3. COMANDA.md in current directory
// 4. COMANDA.md in parent directories (up to 5 levels)
// 5. ~/.comanda/COMANDA.md (user-level default)
// Returns empty string if no memory file is found
func GetMemoryPath(envConfig *EnvConfig) string {
	// 1. Check COMANDA_MEMORY environment variable
	if memPath := os.Getenv("COMANDA_MEMORY"); memPath != "" {
		DebugLog("Using memory file from COMANDA_MEMORY env var: %s", memPath)
		if fileExists(memPath) {
			return memPath
		}
		VerboseLog("Memory file specified in COMANDA_MEMORY does not exist: %s", memPath)
	}

	// 2. Check memory_file setting in .env config
	if envConfig != nil && envConfig.MemoryFile != "" {
		memPath := envConfig.MemoryFile
		// Make absolute if relative
		if !filepath.IsAbs(memPath) {
			envPath := GetEnvPath()
			envDir := filepath.Dir(envPath)
			memPath = filepath.Join(envDir, memPath)
		}
		DebugLog("Using memory file from .env config: %s", memPath)
		if fileExists(memPath) {
			return memPath
		}
		VerboseLog("Memory file specified in .env does not exist: %s", memPath)
	}

	// 3. Check COMANDA.md in current directory
	currentDir, err := os.Getwd()
	if err == nil {
		memPath := filepath.Join(currentDir, "COMANDA.md")
		if fileExists(memPath) {
			DebugLog("Found memory file in current directory: %s", memPath)
			return memPath
		}
	}

	// 4. Check COMANDA.md in parent directories (up to 5 levels)
	if err == nil {
		dir := currentDir
		for i := 0; i < 5; i++ {
			parentDir := filepath.Dir(dir)
			if parentDir == dir {
				// Reached root
				break
			}
			dir = parentDir
			memPath := filepath.Join(dir, "COMANDA.md")
			if fileExists(memPath) {
				DebugLog("Found memory file in parent directory: %s", memPath)
				return memPath
			}
		}
	}

	// 5. Check ~/.comanda/COMANDA.md (user-level default)
	homeDir, err := os.UserHomeDir()
	if err == nil {
		memPath := filepath.Join(homeDir, ".comanda", "COMANDA.md")
		if fileExists(memPath) {
			DebugLog("Found memory file in user directory: %s", memPath)
			return memPath
		}
	}

	DebugLog("No memory file found")
	return ""
}

// fileExists checks if a file exists at the given path
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// InitializeUserMemoryFile creates the default user-level memory file if it doesn't exist
func InitializeUserMemoryFile() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	comandaDir := filepath.Join(homeDir, ".comanda")
	memPath := filepath.Join(comandaDir, "COMANDA.md")

	// Check if file already exists
	if fileExists(memPath) {
		return memPath, nil
	}

	// Create .comanda directory if it doesn't exist
	if err := os.MkdirAll(comandaDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create .comanda directory: %w", err)
	}

	// Create initial memory file with template
	template := `# Project Memory

This file serves as persistent memory for your comanda workflows.
Steps can read from this file (using memory: true) and write to it (using output: MEMORY).

## Project Context

<!-- Add general project information here -->

## Current Status

<!-- Steps can update this section with: output: MEMORY:current_status -->

## Key Learnings

<!-- Document important insights and decisions -->

## Notes

<!-- General notes and observations -->
`

	if err := os.WriteFile(memPath, []byte(template), 0644); err != nil {
		return "", fmt.Errorf("failed to create memory file: %w", err)
	}

	return memPath, nil
}
