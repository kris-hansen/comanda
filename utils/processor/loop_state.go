package processor

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// LoopState represents the persistent state of an agentic loop
type LoopState struct {
	LoopName           string              `json:"loop_name"`
	Iteration          int                 `json:"iteration"`
	MaxIterations      int                 `json:"max_iterations"`
	StartTime          time.Time           `json:"start_time"`
	LastUpdateTime     time.Time           `json:"last_update_time"`
	PreviousOutput     string              `json:"previous_output"`
	History            []LoopIteration     `json:"history"`
	Variables          map[string]string   `json:"variables"`
	Status             string              `json:"status"` // running, paused, completed, failed
	ExitCondition      string              `json:"exit_condition"`
	ExitPattern        string              `json:"exit_pattern,omitempty"`
	WorkflowFile       string              `json:"workflow_file"`
	WorkflowChecksum   string              `json:"workflow_checksum"` // Detect workflow changes
	QualityGateResults []QualityGateResult `json:"quality_gate_results,omitempty"`
}

// LoopStateManager handles persistence of loop states
type LoopStateManager struct {
	stateDir string
}

// NewLoopStateManager creates a new state manager
func NewLoopStateManager(stateDir string) *LoopStateManager {
	return &LoopStateManager{
		stateDir: stateDir,
	}
}

// ensureStateDir creates the state directory if it doesn't exist
func (m *LoopStateManager) ensureStateDir() error {
	if err := os.MkdirAll(m.stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}
	return nil
}

// stateFilePath returns the path to a loop's state file
func (m *LoopStateManager) stateFilePath(loopName string) string {
	return filepath.Join(m.stateDir, fmt.Sprintf("%s.json", loopName))
}

// backupFilePath returns the path to a numbered backup file
func (m *LoopStateManager) backupFilePath(loopName string, backupNum int) string {
	return filepath.Join(m.stateDir, fmt.Sprintf("%s.json.%d", loopName, backupNum))
}

// rotateBackups rotates backup files, keeping the last 3 versions
func (m *LoopStateManager) rotateBackups(loopName string) error {
	stateFile := m.stateFilePath(loopName)

	// Check if the current state file exists
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		return nil // No file to backup
	}

	// Rotate backups: .json.2 -> .json.3, .json.1 -> .json.2, .json -> .json.1
	for i := 2; i >= 1; i-- {
		oldPath := m.backupFilePath(loopName, i)
		newPath := m.backupFilePath(loopName, i+1)

		if _, err := os.Stat(oldPath); err == nil {
			if err := os.Rename(oldPath, newPath); err != nil {
				return fmt.Errorf("failed to rotate backup %d: %w", i, err)
			}
		}
	}

	// Move current file to .json.1
	backup1 := m.backupFilePath(loopName, 1)
	if err := copyFile(stateFile, backup1); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	return nil
}

// SaveState persists a loop state to disk with backup rotation
func (m *LoopStateManager) SaveState(state *LoopState) error {
	if err := m.ensureStateDir(); err != nil {
		return err
	}

	// Update last update time
	state.LastUpdateTime = time.Now()

	// Rotate backups before saving
	if err := m.rotateBackups(state.LoopName); err != nil {
		return fmt.Errorf("failed to rotate backups: %w", err)
	}

	// Marshal state to JSON
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Write to file
	stateFile := m.stateFilePath(state.LoopName)
	if err := os.WriteFile(stateFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// LoadState loads a loop state from disk
func (m *LoopStateManager) LoadState(loopName string) (*LoopState, error) {
	stateFile := m.stateFilePath(loopName)

	// Read the file
	data, err := os.ReadFile(stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no saved state found for loop '%s'", loopName)
		}
		return nil, fmt.Errorf("failed to read state file: %w", err)
	}

	// Unmarshal state
	var state LoopState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state (file may be corrupted): %w", err)
	}

	return &state, nil
}

// DeleteState removes a loop's state file and backups
func (m *LoopStateManager) DeleteState(loopName string) error {
	// Delete main state file
	stateFile := m.stateFilePath(loopName)
	if err := os.Remove(stateFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete state file: %w", err)
	}

	// Delete backups
	for i := 1; i <= 3; i++ {
		backupFile := m.backupFilePath(loopName, i)
		if err := os.Remove(backupFile); err != nil && !os.IsNotExist(err) {
			// Don't fail if backup doesn't exist
			continue
		}
	}

	return nil
}

// ListStates returns all saved loop states
func (m *LoopStateManager) ListStates() ([]*LoopState, error) {
	if err := m.ensureStateDir(); err != nil {
		return nil, err
	}

	// Read directory
	entries, err := os.ReadDir(m.stateDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read state directory: %w", err)
	}

	var states []*LoopState
	for _, entry := range entries {
		// Skip non-JSON files and backup files
		if entry.IsDir() || !isMainStateFile(entry.Name()) {
			continue
		}

		// Extract loop name from filename (remove .json extension)
		loopName := entry.Name()[:len(entry.Name())-5]

		// Load the state
		state, err := m.LoadState(loopName)
		if err != nil {
			// Skip corrupted states
			continue
		}

		states = append(states, state)
	}

	// Sort by last update time (most recent first)
	sort.Slice(states, func(i, j int) bool {
		return states[i].LastUpdateTime.After(states[j].LastUpdateTime)
	})

	return states, nil
}

// ComputeWorkflowChecksum computes a SHA256 checksum of a workflow file
func ComputeWorkflowChecksum(workflowPath string) (string, error) {
	file, err := os.Open(workflowPath)
	if err != nil {
		return "", fmt.Errorf("failed to open workflow file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to compute checksum: %w", err)
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// ValidateWorkflowChecksum verifies that a workflow file hasn't changed
func ValidateWorkflowChecksum(workflowPath string, expectedChecksum string) error {
	actualChecksum, err := ComputeWorkflowChecksum(workflowPath)
	if err != nil {
		return err
	}

	if actualChecksum != expectedChecksum {
		return fmt.Errorf("workflow file has been modified since loop started (checksum mismatch)")
	}

	return nil
}

// loopStateFromContext converts a LoopContext to a LoopState for persistence
func loopStateFromContext(ctx *LoopContext, name string, config *AgenticLoopConfig, workflowFile string, variables map[string]string) *LoopState {
	// Compute workflow checksum
	checksum := ""
	if workflowFile != "" {
		if cs, err := ComputeWorkflowChecksum(workflowFile); err == nil {
			checksum = cs
		}
	}

	return &LoopState{
		LoopName:         name,
		Iteration:        ctx.Iteration,
		MaxIterations:    config.MaxIterations,
		StartTime:        ctx.StartTime,
		LastUpdateTime:   time.Now(),
		PreviousOutput:   ctx.PreviousOutput,
		History:          ctx.History,
		Variables:        variables,
		Status:           "running",
		ExitCondition:    config.ExitCondition,
		ExitPattern:      config.ExitPattern,
		WorkflowFile:     workflowFile,
		WorkflowChecksum: checksum,
	}
}

// stateToLoopContext converts a LoopState back to a LoopContext for resuming
func stateToLoopContext(state *LoopState) *LoopContext {
	return &LoopContext{
		Iteration:      state.Iteration,
		PreviousOutput: state.PreviousOutput,
		History:        state.History,
		StartTime:      state.StartTime,
	}
}

// isMainStateFile checks if a filename is a main state file (not a backup)
func isMainStateFile(filename string) bool {
	return filepath.Ext(filename) == ".json" &&
		len(filename) > 5 &&
		filename[len(filename)-6] != '.'
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	if _, err := io.Copy(destFile, sourceFile); err != nil {
		return err
	}

	return destFile.Sync()
}
