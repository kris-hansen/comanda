package processor

import (
	"fmt"
	"strings"

	"github.com/kris-hansen/comanda/utils/worktree"
)

// WorktreeHandler manages worktrees for a processor
type WorktreeHandler struct {
	manager    *worktree.Manager
	config     *WorktreeConfig
	verbose    bool
	repoPath   string
	worktrees  map[string]*worktree.Worktree
	cleanupSet bool
}

// NewWorktreeHandler creates a new worktree handler
func NewWorktreeHandler(config *WorktreeConfig, repoPath string, verbose bool) (*WorktreeHandler, error) {
	if config == nil {
		return nil, nil // No worktree config, nothing to do
	}

	// Default repo path to current directory
	if repoPath == "" {
		repoPath = "."
	}
	if config.Repo != "" {
		repoPath = config.Repo
	}

	manager, err := worktree.NewManager(repoPath, verbose)
	if err != nil {
		return nil, fmt.Errorf("failed to create worktree manager: %w", err)
	}

	// Set base directory if specified
	if config.BaseDir != "" {
		manager.SetBaseDir(config.BaseDir)
	}

	handler := &WorktreeHandler{
		manager:    manager,
		config:     config,
		verbose:    verbose,
		repoPath:   repoPath,
		worktrees:  make(map[string]*worktree.Worktree),
		cleanupSet: config.Cleanup,
	}

	return handler, nil
}

// debugf prints debug output if verbose mode is enabled
func (h *WorktreeHandler) debugf(format string, args ...interface{}) {
	if h.verbose {
		fmt.Printf("[DEBUG][WorktreeHandler] "+format+"\n", args...)
	}
}

// Setup creates all worktrees defined in the config
func (h *WorktreeHandler) Setup() error {
	if h == nil || h.config == nil {
		return nil
	}

	h.debugf("Setting up %d worktrees", len(h.config.Trees))

	for _, spec := range h.config.Trees {
		if spec.Name == "" {
			return fmt.Errorf("worktree spec missing required 'name' field")
		}

		var wt *worktree.Worktree
		var err error

		if spec.NewBranch {
			// Create a new branch
			wt, err = h.manager.CreateNewBranch(spec.Name, spec.BaseBranch)
		} else if spec.Branch != "" {
			// Use existing branch
			wt, err = h.manager.Create(spec.Name, spec.Branch)
		} else {
			// Default: create new branch from HEAD
			wt, err = h.manager.CreateNewBranch(spec.Name, "HEAD")
		}

		if err != nil {
			return fmt.Errorf("failed to create worktree %s: %w", spec.Name, err)
		}

		h.worktrees[spec.Name] = wt
		h.debugf("Created worktree: %s -> %s", spec.Name, wt.Path)
	}

	return nil
}

// GetWorktreePath returns the path for a named worktree
func (h *WorktreeHandler) GetWorktreePath(name string) (string, error) {
	if h == nil {
		return "", fmt.Errorf("worktree handler not initialized")
	}

	wt, exists := h.worktrees[name]
	if !exists {
		return "", fmt.Errorf("worktree not found: %s", name)
	}

	return wt.Path, nil
}

// GetWorktree returns the worktree object by name
func (h *WorktreeHandler) GetWorktree(name string) *worktree.Worktree {
	if h == nil {
		return nil
	}
	return h.worktrees[name]
}

// ListWorktrees returns all worktrees
func (h *WorktreeHandler) ListWorktrees() []*worktree.Worktree {
	if h == nil {
		return nil
	}

	result := make([]*worktree.Worktree, 0, len(h.worktrees))
	for _, wt := range h.worktrees {
		result = append(result, wt)
	}
	return result
}

// ListWorktreeNames returns all worktree names
func (h *WorktreeHandler) ListWorktreeNames() []string {
	if h == nil {
		return nil
	}

	names := make([]string, 0, len(h.worktrees))
	for name := range h.worktrees {
		names = append(names, name)
	}
	return names
}

// HasWorktrees returns true if any worktrees are configured
func (h *WorktreeHandler) HasWorktrees() bool {
	return h != nil && len(h.worktrees) > 0
}

// GetWorkDir returns the appropriate working directory for a step
// If worktreeName is specified and exists, returns the worktree path
// Otherwise returns the default workDir
func (h *WorktreeHandler) GetWorkDir(worktreeName, defaultWorkDir string) string {
	if h == nil || worktreeName == "" {
		return defaultWorkDir
	}

	if path, err := h.GetWorktreePath(worktreeName); err == nil {
		return path
	}

	return defaultWorkDir
}

// Cleanup removes all worktrees if cleanup is enabled
func (h *WorktreeHandler) Cleanup() error {
	if h == nil || !h.cleanupSet {
		return nil
	}

	h.debugf("Cleaning up worktrees")
	return h.manager.CleanupAll(true) // Remove branches too
}

// GetDiffs returns diffs for all worktrees
func (h *WorktreeHandler) GetDiffs() (map[string]string, error) {
	if h == nil {
		return nil, nil
	}

	diffs := make(map[string]string)
	for name := range h.worktrees {
		diff, err := h.manager.GetDiff(name)
		if err != nil {
			return nil, fmt.Errorf("failed to get diff for worktree %s: %w", name, err)
		}
		if diff != "" {
			diffs[name] = diff
		}
	}

	return diffs, nil
}

// ExpandWorktreeVariables expands ${worktrees.name.path} and ${worktrees.name.branch} in a string
func (h *WorktreeHandler) ExpandWorktreeVariables(input string) string {
	if h == nil || !strings.Contains(input, "${worktrees.") {
		return input
	}

	result := input
	for name, wt := range h.worktrees {
		// Replace ${worktrees.name.path}
		pathVar := fmt.Sprintf("${worktrees.%s.path}", name)
		result = strings.ReplaceAll(result, pathVar, wt.Path)

		// Replace ${worktrees.name.branch}
		branchVar := fmt.Sprintf("${worktrees.%s.branch}", name)
		result = strings.ReplaceAll(result, branchVar, wt.Branch)

		// Replace ${worktrees.name.name}
		nameVar := fmt.Sprintf("${worktrees.%s.name}", name)
		result = strings.ReplaceAll(result, nameVar, wt.Name)
	}

	return result
}
