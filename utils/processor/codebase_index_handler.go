package processor

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kris-hansen/comanda/utils/codebaseindex"
	"github.com/kris-hansen/comanda/utils/fileutil"
)

// processCodebaseIndexStep handles the codebase-index step type
func (p *Processor) processCodebaseIndexStep(step Step, isParallel bool, parallelID string) (string, error) {
	p.debugf("Processing codebase-index step: %s", step.Name)

	ci := step.Config.CodebaseIndex
	if ci == nil {
		ci = &CodebaseIndexConfig{}
	}

	// Check if we're loading from registry
	if ci.Use != nil {
		return p.processCodebaseIndexFromRegistry(step, ci)
	}

	// Otherwise, generate fresh index
	return p.processCodebaseIndexGenerate(step)
}

// processCodebaseIndexFromRegistry loads indexes from the registry
func (p *Processor) processCodebaseIndexFromRegistry(step Step, ci *CodebaseIndexConfig) (string, error) {
	// Parse 'use' field - can be string or []string
	var names []string
	switch v := ci.Use.(type) {
	case string:
		names = []string{v}
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				names = append(names, s)
			}
		}
	case []string:
		names = v
	default:
		return "", fmt.Errorf("invalid 'use' field type: expected string or []string")
	}

	if len(names) == 0 {
		return "", fmt.Errorf("'use' field is empty")
	}

	// Check for registered indexes in envConfig
	if p.envConfig == nil || p.envConfig.Indexes == nil {
		return "", fmt.Errorf("no indexes registered (run 'comanda index capture' first)")
	}

	var allContent []string
	var loadedIndexes []string

	for _, name := range names {
		entry, ok := p.envConfig.Indexes[name]
		if !ok {
			return "", fmt.Errorf("index '%s' not found in registry", name)
		}

		// Check max_age if specified
		if ci.MaxAge != "" {
			maxAge, err := time.ParseDuration(ci.MaxAge)
			if err != nil {
				p.debugf("Warning: invalid max_age '%s': %v", ci.MaxAge, err)
			} else {
				if t, err := time.Parse(time.RFC3339, entry.LastIndexed); err == nil {
					age := time.Since(t)
					if age > maxAge {
						p.debugf("Warning: index '%s' is stale (age: %v, max: %v)", name, age, maxAge)
					}
				}
			}
		}

		// Load index content
		content, err := os.ReadFile(entry.IndexPath)
		if err != nil {
			return "", fmt.Errorf("failed to read index '%s' from %s: %w", name, entry.IndexPath, err)
		}

		// Handle encrypted indexes
		if entry.Encrypted {
			key := os.Getenv("COMANDA_INDEX_KEY")
			if key == "" && p.envConfig != nil {
				key = p.envConfig.IndexEncryptionKey
			}
			if key == "" {
				return "", fmt.Errorf("index '%s' is encrypted but no decryption key provided", name)
			}
			decrypted, err := codebaseindex.Decrypt(content, key)
			if err != nil {
				return "", fmt.Errorf("failed to decrypt index '%s': %w", name, err)
			}
			content = decrypted
		}

		contentStr := string(content)

		// Export as individual variable
		p.variables[entry.VarPrefix+"_INDEX"] = contentStr
		p.variables[entry.VarPrefix+"_INDEX_PATH"] = entry.IndexPath

		allContent = append(allContent, contentStr)
		loadedIndexes = append(loadedIndexes, name)

		p.debugf("Loaded index '%s' from registry (%d bytes)", name, len(content))
	}

	// If aggregate mode, also create combined variable
	if ci.Aggregate && len(allContent) > 1 {
		aggregated := strings.Join(allContent, "\n\n---\n\n")
		p.variables["AGGREGATED_INDEX"] = aggregated
		p.debugf("Created AGGREGATED_INDEX from %d indexes", len(allContent))
	}

	return fmt.Sprintf("Loaded %d index(es) from registry: %v", len(loadedIndexes), loadedIndexes), nil
}

// processCodebaseIndexGenerate generates a fresh index
func (p *Processor) processCodebaseIndexGenerate(step Step) (string, error) {
	// Build configuration from step config
	config := p.buildCodebaseIndexConfig(step.Config)

	// Create manager
	manager, err := codebaseindex.NewManager(config, p.verbose)
	if err != nil {
		return "", fmt.Errorf("failed to create codebase index manager: %w", err)
	}

	// Generate the index
	result, err := manager.Generate()
	if err != nil {
		return "", fmt.Errorf("codebase index generation failed: %w", err)
	}

	// Export workflow variables
	cfg := manager.GetConfig()
	varPrefix := cfg.RepoVarSlug

	// Main index content
	p.variables[varPrefix+"_INDEX"] = result.Content

	// Index path
	p.variables[varPrefix+"_INDEX_PATH"] = result.OutputPath

	// Content hash
	p.variables[varPrefix+"_INDEX_SHA"] = result.ContentHash

	// Updated flag
	if result.Updated {
		p.variables[varPrefix+"_INDEX_UPDATED"] = "true"
	} else {
		p.variables[varPrefix+"_INDEX_UPDATED"] = "false"
	}

	p.debugf("Codebase index generated: %d files indexed, output at %s", result.FileCount, result.OutputPath)
	p.debugf("Exported variables: %s_INDEX, %s_INDEX_PATH, %s_INDEX_SHA, %s_INDEX_UPDATED", varPrefix, varPrefix, varPrefix, varPrefix)

	// Return summary message
	return fmt.Sprintf("Codebase index generated successfully.\nLanguages: %v\nFiles indexed: %d\nOutput: %s\nDuration: %v",
		result.Languages, result.FileCount, result.OutputPath, result.Duration), nil
}

// buildCodebaseIndexConfig converts processor config to codebaseindex.Config
func (p *Processor) buildCodebaseIndexConfig(stepConfig StepConfig) *codebaseindex.Config {
	config := codebaseindex.DefaultConfig()

	// Use step-level config if available
	if stepConfig.CodebaseIndex != nil {
		ci := stepConfig.CodebaseIndex

		if ci.Root != "" {
			// Expand ~ in root path
			expandedRoot, err := fileutil.ExpandPath(ci.Root)
			if err != nil {
				p.debugf("Warning: failed to expand root path %s: %v", ci.Root, err)
				config.Root = ci.Root // Fall back to unexpanded path
			} else {
				config.Root = expandedRoot
			}
		}

		if ci.Output != nil {
			if ci.Output.Path != "" {
				// Expand ~ in output path
				expandedOutput, err := fileutil.ExpandPath(ci.Output.Path)
				if err != nil {
					p.debugf("Warning: failed to expand output path %s: %v", ci.Output.Path, err)
					config.OutputPath = ci.Output.Path // Fall back to unexpanded path
				} else {
					config.OutputPath = expandedOutput
				}
			}
			if ci.Output.Format != "" {
				switch ci.Output.Format {
				case "summary":
					config.OutputFormat = codebaseindex.FormatSummary
				case "structured":
					config.OutputFormat = codebaseindex.FormatStructured
				case "full":
					config.OutputFormat = codebaseindex.FormatFull
				default:
					config.OutputFormat = codebaseindex.FormatStructured
				}
			}
			if ci.Output.Store != "" {
				switch ci.Output.Store {
				case "repo":
					config.Store = codebaseindex.StoreRepo
				case "config":
					config.Store = codebaseindex.StoreConfig
				case "both":
					config.Store = codebaseindex.StoreBoth
				}
			}
			config.Encrypt = ci.Output.Encrypt
			// Get encryption key: env var takes precedence, then config
			if ci.Output.Encrypt {
				config.EncryptionKey = os.Getenv("COMANDA_INDEX_KEY")
				if config.EncryptionKey == "" && p.envConfig != nil {
					config.EncryptionKey = p.envConfig.IndexEncryptionKey
				}
			}
		}

		if ci.Expose != nil {
			config.ExposeVariable = ci.Expose.WorkflowVariable
			if ci.Expose.Memory != nil {
				config.MemoryEnabled = ci.Expose.Memory.Enabled
				config.MemoryKey = ci.Expose.Memory.Key
			}
		}

		if ci.MaxOutputKB > 0 {
			config.MaxOutputKB = ci.MaxOutputKB
		}

		// Convert adapter overrides
		if len(ci.Adapters) > 0 {
			config.AdapterOverrides = make(map[string]*codebaseindex.AdapterOverride)
			for name, override := range ci.Adapters {
				config.AdapterOverrides[name] = &codebaseindex.AdapterOverride{
					IgnoreDirs:      override.IgnoreDirs,
					IgnoreGlobs:     override.IgnoreGlobs,
					PriorityFiles:   override.PriorityFiles,
					ReplaceDefaults: override.ReplaceDefaults,
				}
			}
		}

		// Convert qmd integration config
		if ci.Qmd != nil {
			config.Qmd = &codebaseindex.QmdConfig{
				Collection: ci.Qmd.Collection,
				Embed:      ci.Qmd.Embed,
				Context:    ci.Qmd.Context,
				Mask:       ci.Qmd.Mask,
			}
		}
	}

	config.Verbose = p.verbose

	return config
}
