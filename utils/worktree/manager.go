// Package worktree provides Git worktree management for parallel Claude Code execution.
// It enables running multiple Claude Code sessions in isolated worktrees, each with
// their own branch and working directory, while sharing repository history.
package worktree

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Worktree represents a single Git worktree
type Worktree struct {
	Name   string // User-friendly name (e.g., "feature-auth")
	Branch string // Git branch name (e.g., "worktree-feature-auth")
	Path   string // Absolute path to worktree directory
}

// Manager handles creation and cleanup of Git worktrees
type Manager struct {
	RepoPath  string               // Path to the main repository
	BaseDir   string               // Directory for worktrees (default: .comanda-worktrees)
	Worktrees map[string]*Worktree // Active worktrees by name
	verbose   bool
	mu        sync.RWMutex
}

// NewManager creates a new worktree manager
func NewManager(repoPath string, verbose bool) (*Manager, error) {
	// Resolve to absolute path
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve repo path: %w", err)
	}

	// Verify it's a git repository
	gitDir := filepath.Join(absPath, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("not a git repository: %s", absPath)
	}

	return &Manager{
		RepoPath:  absPath,
		BaseDir:   ".comanda-worktrees",
		Worktrees: make(map[string]*Worktree),
		verbose:   verbose,
	}, nil
}

// SetBaseDir changes the base directory for worktrees
func (m *Manager) SetBaseDir(dir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.BaseDir = dir
}

// debugf prints debug output if verbose mode is enabled
func (m *Manager) debugf(format string, args ...interface{}) {
	if m.verbose {
		fmt.Printf("[DEBUG][Worktree] "+format+"\n", args...)
	}
}

// Create creates a new worktree from an existing branch
func (m *Manager) Create(name, branch string) (*Worktree, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if worktree already exists
	if wt, exists := m.Worktrees[name]; exists {
		return wt, nil
	}

	// Create worktree directory path
	wtPath := filepath.Join(m.RepoPath, m.BaseDir, name)

	// Ensure base directory exists
	baseDir := filepath.Join(m.RepoPath, m.BaseDir)
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create worktree base directory: %w", err)
	}

	// Create the worktree
	m.debugf("Creating worktree %s from branch %s at %s", name, branch, wtPath)
	cmd := exec.Command("git", "worktree", "add", wtPath, branch)
	cmd.Dir = m.RepoPath

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to create worktree: %w (%s)", err, stderr.String())
	}

	wt := &Worktree{
		Name:   name,
		Branch: branch,
		Path:   wtPath,
	}
	m.Worktrees[name] = wt

	m.debugf("Created worktree: %s -> %s", name, wtPath)
	return wt, nil
}

// CreateNewBranch creates a worktree with a new branch from a base branch
func (m *Manager) CreateNewBranch(name, baseBranch string) (*Worktree, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if worktree already exists
	if wt, exists := m.Worktrees[name]; exists {
		return wt, nil
	}

	// Create worktree directory path
	wtPath := filepath.Join(m.RepoPath, m.BaseDir, name)
	branchName := "worktree-" + name

	// Ensure base directory exists
	baseDir := filepath.Join(m.RepoPath, m.BaseDir)
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create worktree base directory: %w", err)
	}

	// If no base branch specified, use current HEAD
	if baseBranch == "" {
		baseBranch = "HEAD"
	}

	// Create the worktree with a new branch
	m.debugf("Creating worktree %s with new branch %s from %s", name, branchName, baseBranch)
	cmd := exec.Command("git", "worktree", "add", "-b", branchName, wtPath, baseBranch)
	cmd.Dir = m.RepoPath

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to create worktree: %w (%s)", err, stderr.String())
	}

	wt := &Worktree{
		Name:   name,
		Branch: branchName,
		Path:   wtPath,
	}
	m.Worktrees[name] = wt

	m.debugf("Created worktree with new branch: %s -> %s (branch: %s)", name, wtPath, branchName)
	return wt, nil
}

// Get returns a worktree by name
func (m *Manager) Get(name string) *Worktree {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Worktrees[name]
}

// List returns all active worktrees
func (m *Manager) List() []*Worktree {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Worktree, 0, len(m.Worktrees))
	for _, wt := range m.Worktrees {
		result = append(result, wt)
	}
	return result
}

// Remove removes a worktree and optionally its branch
func (m *Manager) Remove(name string, removeBranch bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	wt, exists := m.Worktrees[name]
	if !exists {
		return nil // Already removed
	}

	m.debugf("Removing worktree: %s", name)

	// Remove the worktree
	cmd := exec.Command("git", "worktree", "remove", wt.Path, "--force")
	cmd.Dir = m.RepoPath

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Try to remove directory manually if git worktree remove fails
		if rmErr := os.RemoveAll(wt.Path); rmErr != nil {
			return fmt.Errorf("failed to remove worktree: %w (%s)", err, stderr.String())
		}
		// Prune worktrees to clean up
		pruneCmd := exec.Command("git", "worktree", "prune")
		pruneCmd.Dir = m.RepoPath
		_ = pruneCmd.Run()
	}

	// Optionally remove the branch
	if removeBranch && strings.HasPrefix(wt.Branch, "worktree-") {
		m.debugf("Removing branch: %s", wt.Branch)
		branchCmd := exec.Command("git", "branch", "-D", wt.Branch)
		branchCmd.Dir = m.RepoPath
		_ = branchCmd.Run() // Ignore errors - branch might not exist or might be checked out elsewhere
	}

	delete(m.Worktrees, name)
	return nil
}

// CleanupAll removes all managed worktrees
func (m *Manager) CleanupAll(removeBranches bool) error {
	m.mu.Lock()
	names := make([]string, 0, len(m.Worktrees))
	for name := range m.Worktrees {
		names = append(names, name)
	}
	m.mu.Unlock()

	var lastErr error
	for _, name := range names {
		if err := m.Remove(name, removeBranches); err != nil {
			lastErr = err
			m.debugf("Error removing worktree %s: %v", name, err)
		}
	}

	return lastErr
}

// HasChanges checks if a worktree has uncommitted changes
func (m *Manager) HasChanges(name string) (bool, error) {
	m.mu.RLock()
	wt, exists := m.Worktrees[name]
	m.mu.RUnlock()

	if !exists {
		return false, fmt.Errorf("worktree not found: %s", name)
	}

	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = wt.Path

	output, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("failed to check worktree status: %w", err)
	}

	return len(bytes.TrimSpace(output)) > 0, nil
}

// GetDiff returns the diff for a worktree (staged and unstaged changes)
func (m *Manager) GetDiff(name string) (string, error) {
	m.mu.RLock()
	wt, exists := m.Worktrees[name]
	m.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("worktree not found: %s", name)
	}

	// Get both staged and unstaged changes
	cmd := exec.Command("git", "diff", "HEAD")
	cmd.Dir = wt.Path

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get diff: %w", err)
	}

	return string(output), nil
}

// DiscoverExisting discovers worktrees that already exist in the base directory
func (m *Manager) DiscoverExisting() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	baseDir := filepath.Join(m.RepoPath, m.BaseDir)
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return nil // No base directory, nothing to discover
	}

	// List git worktrees
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = m.RepoPath

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	// Parse worktree list
	lines := strings.Split(string(output), "\n")
	var currentPath, currentBranch string

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "worktree ") {
			currentPath = strings.TrimPrefix(line, "worktree ")
		} else if strings.HasPrefix(line, "branch ") {
			currentBranch = strings.TrimPrefix(line, "branch refs/heads/")
		} else if line == "" && currentPath != "" {
			// Check if this worktree is in our base directory
			if strings.HasPrefix(currentPath, baseDir) {
				name := filepath.Base(currentPath)
				if _, exists := m.Worktrees[name]; !exists {
					m.Worktrees[name] = &Worktree{
						Name:   name,
						Branch: currentBranch,
						Path:   currentPath,
					}
					m.debugf("Discovered existing worktree: %s", name)
				}
			}
			currentPath = ""
			currentBranch = ""
		}
	}

	return nil
}
