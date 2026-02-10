package processor

import (
	"fmt"
	"os"

	"github.com/kris-hansen/comanda/utils/codebaseindex"
	"github.com/kris-hansen/comanda/utils/fileutil"
)

// processCodebaseIndexStep handles the codebase-index step type
func (p *Processor) processCodebaseIndexStep(step Step, isParallel bool, parallelID string) (string, error) {
	p.debugf("Processing codebase-index step: %s", step.Name)

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
	}

	config.Verbose = p.verbose

	return config
}
