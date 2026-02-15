package codebaseindex

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// QmdConfig holds configuration for qmd integration
type QmdConfig struct {
	// Collection name to register with qmd
	Collection string `yaml:"collection"`

	// Whether to run qmd embed after registration
	Embed bool `yaml:"embed"`

	// Context description for the collection
	Context string `yaml:"context"`

	// File mask for indexing (default: "**/*.md")
	Mask string `yaml:"mask"`
}

// qmd command timeouts
const (
	qmdStatusTimeout     = 10 * time.Second
	qmdUpdateTimeout     = 60 * time.Second
	qmdCollectionTimeout = 60 * time.Second
	qmdContextTimeout    = 10 * time.Second
	qmdEmbedTimeout      = 10 * time.Minute // Embedding can take a long time
)

// registerWithQmd registers the generated index with qmd as a collection
func (m *Manager) registerWithQmd(indexPath string) error {
	if m.config.Qmd == nil || m.config.Qmd.Collection == "" {
		return nil
	}

	qmdPath, err := exec.LookPath("qmd")
	if err != nil {
		return fmt.Errorf("qmd not found in PATH: %w", err)
	}

	collectionName := m.config.Qmd.Collection
	indexDir := filepath.Dir(indexPath)

	// Determine mask - default to the specific index file if it's markdown
	mask := m.config.Qmd.Mask
	if mask == "" {
		if strings.HasSuffix(indexPath, ".md") {
			// Index just this file
			mask = filepath.Base(indexPath)
		} else {
			mask = "**/*.md"
		}
	}

	m.logf("Registering with qmd: collection=%s path=%s mask=%s", collectionName, indexDir, mask)

	// Check if collection already exists
	ctx, cancel := context.WithTimeout(context.Background(), qmdStatusTimeout)
	defer cancel()
	checkCmd := exec.CommandContext(ctx, qmdPath, "status", "--json")
	if output, err := checkCmd.Output(); err == nil {
		if strings.Contains(string(output), fmt.Sprintf(`"name":"%s"`, collectionName)) {
			// Collection exists, update it
			m.logf("Collection '%s' exists, updating...", collectionName)
			updateCtx, updateCancel := context.WithTimeout(context.Background(), qmdUpdateTimeout)
			defer updateCancel()
			updateCmd := exec.CommandContext(updateCtx, qmdPath, "update", "-c", collectionName)
			if output, err := updateCmd.CombinedOutput(); err != nil {
				m.logf("Warning: qmd update failed: %s", string(output))
			}
		} else {
			// Create new collection
			if err := m.createQmdCollection(qmdPath, collectionName, indexDir, mask); err != nil {
				return err
			}
		}
	} else {
		// Status failed, try creating anyway
		if err := m.createQmdCollection(qmdPath, collectionName, indexDir, mask); err != nil {
			return err
		}
	}

	// Add context if provided
	if m.config.Qmd.Context != "" {
		ctxTimeout, ctxCancel := context.WithTimeout(context.Background(), qmdContextTimeout)
		defer ctxCancel()
		contextCmd := exec.CommandContext(ctxTimeout, qmdPath, "context", "add",
			fmt.Sprintf("qmd://%s", collectionName),
			m.config.Qmd.Context)
		if output, err := contextCmd.CombinedOutput(); err != nil {
			m.logf("Warning: failed to add context: %s", string(output))
		}
	}

	// Run embed if requested
	if m.config.Qmd.Embed {
		m.logf("Running qmd embed (this may take a while)...")
		embedCtx, embedCancel := context.WithTimeout(context.Background(), qmdEmbedTimeout)
		defer embedCancel()
		embedCmd := exec.CommandContext(embedCtx, qmdPath, "embed", "-c", collectionName)
		if output, err := embedCmd.CombinedOutput(); err != nil {
			if embedCtx.Err() == context.DeadlineExceeded {
				m.logf("Warning: qmd embed timed out after %v", qmdEmbedTimeout)
			} else {
				m.logf("Warning: qmd embed failed: %s", string(output))
			}
		} else {
			m.logf("qmd embed completed")
		}
	}

	m.logf("Successfully registered with qmd as collection '%s'", collectionName)
	return nil
}

// createQmdCollection creates a new qmd collection
func (m *Manager) createQmdCollection(qmdPath, name, path, mask string) error {
	args := []string{"collection", "add", path, "--name", name}
	if mask != "" {
		args = append(args, "--mask", mask)
	}

	ctx, cancel := context.WithTimeout(context.Background(), qmdCollectionTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, qmdPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("qmd collection add timed out after %v", qmdCollectionTimeout)
		}
		return fmt.Errorf("failed to create qmd collection: %s: %w", string(output), err)
	}
	return nil
}
