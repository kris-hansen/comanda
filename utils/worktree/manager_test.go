package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNewManager(t *testing.T) {
	// Create a temporary git repo for testing
	tmpDir, err := os.MkdirTemp("", "worktree-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git user for commits
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = tmpDir
	_ = cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = tmpDir
	_ = cmd.Run()

	// Create initial commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	_ = cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	// Test manager creation
	manager, err := NewManager(tmpDir, true)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	if manager.RepoPath != tmpDir {
		t.Errorf("Expected RepoPath %s, got %s", tmpDir, manager.RepoPath)
	}

	if manager.BaseDir != ".comanda-worktrees" {
		t.Errorf("Expected BaseDir .comanda-worktrees, got %s", manager.BaseDir)
	}
}

func TestCreateNewBranch(t *testing.T) {
	// Create a temporary git repo for testing
	tmpDir, err := os.MkdirTemp("", "worktree-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git user
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = tmpDir
	_ = cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = tmpDir
	_ = cmd.Run()

	// Create initial commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	_ = cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	// Create manager
	manager, err := NewManager(tmpDir, false)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Test creating a worktree with new branch
	wt, err := manager.CreateNewBranch("test-feature", "")
	if err != nil {
		t.Fatalf("Failed to create worktree: %v", err)
	}

	if wt.Name != "test-feature" {
		t.Errorf("Expected name 'test-feature', got %s", wt.Name)
	}

	if wt.Branch != "worktree-test-feature" {
		t.Errorf("Expected branch 'worktree-test-feature', got %s", wt.Branch)
	}

	expectedPath := filepath.Join(tmpDir, ".comanda-worktrees", "test-feature")
	if wt.Path != expectedPath {
		t.Errorf("Expected path %s, got %s", expectedPath, wt.Path)
	}

	// Verify worktree directory exists
	if _, err := os.Stat(wt.Path); os.IsNotExist(err) {
		t.Error("Worktree directory was not created")
	}

	// Cleanup
	if err := manager.Remove("test-feature", true); err != nil {
		t.Errorf("Failed to remove worktree: %v", err)
	}
}

func TestListWorktrees(t *testing.T) {
	// Create a temporary git repo for testing
	tmpDir, err := os.MkdirTemp("", "worktree-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Configure git user
	cmd = exec.Command("git", "config", "user.email", "test@test.com")
	cmd.Dir = tmpDir
	_ = cmd.Run()
	cmd = exec.Command("git", "config", "user.name", "Test")
	cmd.Dir = tmpDir
	_ = cmd.Run()

	// Create initial commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	_ = cmd.Run()
	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = tmpDir
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create initial commit: %v", err)
	}

	// Create manager
	manager, err := NewManager(tmpDir, false)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Create multiple worktrees
	_, err = manager.CreateNewBranch("feature-a", "")
	if err != nil {
		t.Fatalf("Failed to create worktree a: %v", err)
	}
	_, err = manager.CreateNewBranch("feature-b", "")
	if err != nil {
		t.Fatalf("Failed to create worktree b: %v", err)
	}

	// List worktrees
	worktrees := manager.List()
	if len(worktrees) != 2 {
		t.Errorf("Expected 2 worktrees, got %d", len(worktrees))
	}

	// Cleanup
	manager.CleanupAll(true)
}
