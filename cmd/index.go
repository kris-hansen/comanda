package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kris-hansen/comanda/utils/codebaseindex"
	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/fileutil"
	"github.com/spf13/cobra"
)

var (
	// Capture flags
	indexName     string
	indexOutput   string
	indexFormat   string
	indexGlobal   bool
	indexEncrypt  bool
	indexForce    bool

	// Update flags
	updateFull bool

	// Diff flags
	diffSince string
	diffJSON  bool

	// Show flags
	showSummary bool
	showRaw     bool

	// List flags
	listJSON  bool
	listPaths bool

	// Remove flags
	removeDelete bool
)

var indexCmd = &cobra.Command{
	Use:   "index",
	Short: "Manage codebase indexes for AI context",
	Long: `Manage codebase indexes that provide code context awareness for AI workflows.

Indexes are registered in your comanda config and can be referenced in workflows
using the codebase_index.use field for multi-codebase analysis.

Examples:
  comanda index capture                    # Index current directory
  comanda index capture ~/project -n proj  # Index with custom name
  comanda index list                       # Show all registered indexes
  comanda index show proj                  # Display index content
  comanda index diff proj                  # Show changes since last index
  comanda index update proj                # Incrementally update index`,
}

var captureCmd = &cobra.Command{
	Use:   "capture [path]",
	Short: "Generate and register a code index",
	Long: `Generate a code index for a repository and register it in config.

The index captures code structure, symbols, and organization to provide
rich context for AI tools and workflows.

Examples:
  comanda index capture                     # Index current directory
  comanda index capture ~/clawd/comanda     # Index specific path
  comanda index capture -n myproject        # Custom name
  comanda index capture -f summary          # Compact format
  comanda index capture -g                  # Store in ~/.comanda/indexes/
  comanda index capture -e                  # Encrypt output`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCapture,
}

var updateCmd = &cobra.Command{
	Use:   "update [name]",
	Short: "Incrementally update an existing index",
	Long: `Update an existing index by re-indexing only changed files.

If no name is provided, updates the index for the current directory.

Examples:
  comanda index update           # Update index for current dir
  comanda index update comanda   # Update named index
  comanda index update --full    # Force full regeneration`,
	Args: cobra.MaximumNArgs(1),
	RunE: runUpdate,
}

var diffCmd = &cobra.Command{
	Use:   "diff [name]",
	Short: "Show changes since last index",
	Long: `Compare the current state of a codebase against its stored index.

Shows files that have been added, modified, or deleted since the last
index was generated.

Examples:
  comanda index diff              # Diff current directory
  comanda index diff comanda      # Diff named index
  comanda index diff --json       # Output as JSON`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDiff,
}

var showCmd = &cobra.Command{
	Use:   "show [name]",
	Short: "Display a stored index",
	Long: `Display the contents of a stored index.

If no name is provided, shows the index for the current directory.

Examples:
  comanda index show              # Show index for current dir
  comanda index show comanda      # Show named index
  comanda index show --summary    # Show just summary section
  comanda index show --raw        # Output raw markdown`,
	Args: cobra.MaximumNArgs(1),
	RunE: runShow,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all registered indexes",
	Long: `List all codebase indexes registered in your comanda config.

Examples:
  comanda index list              # Show all indexes
  comanda index list --json       # Output as JSON
  comanda index list --paths      # Show full paths`,
	RunE: runList,
}

var removeCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an index from the registry",
	Long: `Remove a codebase index from the registry.

By default, only removes the registry entry. Use --delete to also
remove the index file.

Examples:
  comanda index remove old-project           # Remove from registry
  comanda index remove old-project --delete  # Also delete file`,
	Args: cobra.ExactArgs(1),
	RunE: runRemove,
}

func init() {
	rootCmd.AddCommand(indexCmd)
	indexCmd.AddCommand(captureCmd)
	indexCmd.AddCommand(updateCmd)
	indexCmd.AddCommand(diffCmd)
	indexCmd.AddCommand(showCmd)
	indexCmd.AddCommand(listCmd)
	indexCmd.AddCommand(removeCmd)

	// Capture flags
	captureCmd.Flags().StringVarP(&indexName, "name", "n", "", "Index name (default: auto-derived from repo)")
	captureCmd.Flags().StringVarP(&indexOutput, "output", "o", "", "Output file path (default: .comanda/index.md)")
	captureCmd.Flags().StringVarP(&indexFormat, "format", "f", "structured", "Output format: summary, structured, full")
	captureCmd.Flags().BoolVarP(&indexGlobal, "global", "g", false, "Store in ~/.comanda/indexes/")
	captureCmd.Flags().BoolVarP(&indexEncrypt, "encrypt", "e", false, "Encrypt the output")
	captureCmd.Flags().BoolVar(&indexForce, "force", false, "Overwrite existing index")

	// Update flags
	updateCmd.Flags().BoolVar(&updateFull, "full", false, "Force full regeneration")

	// Diff flags
	diffCmd.Flags().StringVar(&diffSince, "since", "", "Show changes since date (YYYY-MM-DD)")
	diffCmd.Flags().BoolVar(&diffJSON, "json", false, "Output as JSON")

	// Show flags
	showCmd.Flags().BoolVar(&showSummary, "summary", false, "Show just summary section")
	showCmd.Flags().BoolVar(&showRaw, "raw", false, "Output raw markdown")

	// List flags
	listCmd.Flags().BoolVar(&listJSON, "json", false, "Output as JSON")
	listCmd.Flags().BoolVar(&listPaths, "paths", bool(false), "Show full paths")

	// Remove flags
	removeCmd.Flags().BoolVar(&removeDelete, "delete", false, "Also delete the index file")
}

func runCapture(cmd *cobra.Command, args []string) error {
	// Determine root path
	rootPath := "."
	if len(args) > 0 {
		rootPath = args[0]
	}

	// Expand path
	expandedPath, err := fileutil.ExpandPath(rootPath)
	if err != nil {
		return fmt.Errorf("failed to expand path: %w", err)
	}

	// Resolve to absolute
	absPath, err := filepath.Abs(expandedPath)
	if err != nil {
		return fmt.Errorf("failed to resolve path: %w", err)
	}

	// Verify path exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", absPath)
	}

	// Build config
	cfg := codebaseindex.DefaultConfig()
	cfg.Root = absPath
	cfg.Verbose = verbose

	// Set format
	switch indexFormat {
	case "summary":
		cfg.OutputFormat = codebaseindex.FormatSummary
	case "structured":
		cfg.OutputFormat = codebaseindex.FormatStructured
	case "full":
		cfg.OutputFormat = codebaseindex.FormatFull
	default:
		return fmt.Errorf("invalid format: %s (must be summary, structured, or full)", indexFormat)
	}

	// Determine output path
	if indexOutput != "" {
		expandedOutput, err := fileutil.ExpandPath(indexOutput)
		if err != nil {
			return fmt.Errorf("failed to expand output path: %w", err)
		}
		cfg.OutputPath = expandedOutput
	} else if indexGlobal {
		comandaDir, err := config.GetComandaDir()
		if err != nil {
			return fmt.Errorf("failed to get comanda dir: %w", err)
		}
		indexesDir := filepath.Join(comandaDir, "indexes")
		if err := os.MkdirAll(indexesDir, 0755); err != nil {
			return fmt.Errorf("failed to create indexes dir: %w", err)
		}
		// Will be set after we know the repo slug
	}

	// Set encryption
	cfg.Encrypt = indexEncrypt
	if indexEncrypt {
		cfg.EncryptionKey = os.Getenv("COMANDA_INDEX_KEY")
		if cfg.EncryptionKey == "" && envConfig != nil {
			cfg.EncryptionKey = envConfig.IndexEncryptionKey
		}
		if cfg.EncryptionKey == "" {
			return fmt.Errorf("encryption requested but no key configured (set COMANDA_INDEX_KEY or run 'comanda configure')")
		}
	}

	// Create manager
	manager, err := codebaseindex.NewManager(cfg, verbose)
	if err != nil {
		return fmt.Errorf("failed to create index manager: %w", err)
	}

	// Get derived slugs for naming
	managerCfg := manager.GetConfig()

	// Determine index name
	name := indexName
	if name == "" {
		name = strings.ToLower(managerCfg.RepoFileSlug)
	}

	// Set global output path if needed (now that we have the slug)
	if indexGlobal && indexOutput == "" {
		comandaDir, _ := config.GetComandaDir()
		cfg.OutputPath = filepath.Join(comandaDir, "indexes", name+".md")
	}

	// Check if index already exists
	if envConfig != nil && envConfig.Indexes != nil {
		if existing, ok := envConfig.Indexes[name]; ok && !indexForce {
			return fmt.Errorf("index '%s' already exists (use --force to overwrite)\n  Path: %s\n  Last indexed: %s",
				name, existing.Path, existing.LastIndexed)
		}
	}

	// Generate the index
	log.Printf("Indexing %s...\n", absPath)
	result, err := manager.Generate()
	if err != nil {
		return fmt.Errorf("index generation failed: %w", err)
	}

	// Register in config
	if err := registerIndex(name, absPath, result, managerCfg); err != nil {
		log.Printf("Warning: failed to register index: %v\n", err)
		log.Printf("Index was generated at: %s\n", result.OutputPath)
	} else {
		log.Printf("Index registered as '%s'\n", name)
	}

	// Print summary
	log.Printf("\nIndex generated successfully:")
	log.Printf("  Name:       %s\n", name)
	log.Printf("  Languages:  %v\n", result.Languages)
	log.Printf("  Files:      %d\n", result.FileCount)
	log.Printf("  Output:     %s\n", result.OutputPath)
	log.Printf("  Duration:   %v\n", result.Duration)
	log.Printf("\nUse in workflows with: codebase_index.use: %s\n", name)

	return nil
}

func registerIndex(name, repoPath string, result *codebaseindex.Result, cfg *codebaseindex.Config) error {
	// Load current config
	comandaDir, err := config.GetComandaDir()
	if err != nil {
		return err
	}
	configPath := filepath.Join(comandaDir, "config.yaml")

	// Load existing config
	envCfg, err := config.LoadEnvConfig(configPath)
	if err != nil {
		// If config doesn't exist, create a new one
		envCfg = &config.EnvConfig{}
	}

	// Initialize indexes map if needed
	if envCfg.Indexes == nil {
		envCfg.Indexes = make(map[string]*config.IndexEntry)
	}

	// Get file size
	var sizeBytes int64
	if info, err := os.Stat(result.OutputPath); err == nil {
		sizeBytes = info.Size()
	}

	// Create/update entry
	envCfg.Indexes[name] = &config.IndexEntry{
		Path:        repoPath,
		IndexPath:   result.OutputPath,
		LastIndexed: time.Now().Format(time.RFC3339),
		ContentHash: result.ContentHash,
		Format:      string(result.Format),
		FileCount:   result.FileCount,
		SizeBytes:   sizeBytes,
		VarPrefix:   cfg.RepoVarSlug,
		Encrypted:   cfg.Encrypt,
		Languages:   strings.Join(result.Languages, ", "),
	}

	// Write config back
	return config.SaveEnvConfig(configPath, envCfg)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	// Determine which index to update
	name := ""
	if len(args) > 0 {
		name = args[0]
	}

	// Find index entry
	entry, name, err := findIndex(name)
	if err != nil {
		return err
	}

	log.Printf("Updating index '%s'...\n", name)

	// Build config from entry
	cfg := codebaseindex.DefaultConfig()
	cfg.Root = entry.Path
	cfg.OutputPath = entry.IndexPath
	cfg.Verbose = verbose

	// Set format from entry
	switch entry.Format {
	case "summary":
		cfg.OutputFormat = codebaseindex.FormatSummary
	case "structured":
		cfg.OutputFormat = codebaseindex.FormatStructured
	case "full":
		cfg.OutputFormat = codebaseindex.FormatFull
	}

	cfg.Encrypt = entry.Encrypted
	if entry.Encrypted && envConfig != nil {
		cfg.EncryptionKey = envConfig.IndexEncryptionKey
	}

	// TODO: Implement true incremental mode
	// For now, regenerate fully
	if !updateFull {
		log.Printf("Note: Incremental update not yet implemented, performing full regeneration\n")
	}

	// Create manager and regenerate
	manager, err := codebaseindex.NewManager(cfg, verbose)
	if err != nil {
		return fmt.Errorf("failed to create index manager: %w", err)
	}

	result, err := manager.Generate()
	if err != nil {
		return fmt.Errorf("index update failed: %w", err)
	}

	// Update registry
	if err := registerIndex(name, entry.Path, result, manager.GetConfig()); err != nil {
		log.Printf("Warning: failed to update registry: %v\n", err)
	}

	log.Printf("\nIndex updated:")
	log.Printf("  Files:    %d\n", result.FileCount)
	log.Printf("  Duration: %v\n", result.Duration)

	return nil
}

func runDiff(cmd *cobra.Command, args []string) error {
	// Determine which index to diff
	name := ""
	if len(args) > 0 {
		name = args[0]
	}

	// Find index entry
	entry, name, err := findIndex(name)
	if err != nil {
		return err
	}

	log.Printf("Comparing '%s' against stored index...\n", name)

	// Build config
	cfg := codebaseindex.DefaultConfig()
	cfg.Root = entry.Path
	cfg.Verbose = verbose

	// Create manager
	manager, err := codebaseindex.NewManager(cfg, verbose)
	if err != nil {
		return fmt.Errorf("failed to create index manager: %w", err)
	}

	// Compute diff
	diffResult, err := manager.Diff(entry.IndexPath)
	if err != nil {
		return fmt.Errorf("failed to compute diff: %w", err)
	}

	// Format output
	if diffJSON {
		// TODO: Implement JSON output
		return fmt.Errorf("JSON output not yet implemented")
	}

	fmt.Printf("\nIndex: %s (last updated: %s)\n", name, entry.LastIndexed)
	fmt.Println()

	if len(diffResult.Added) > 0 {
		fmt.Printf("Added (%d):\n", len(diffResult.Added))
		for _, path := range diffResult.Added {
			fmt.Printf("  + %s\n", path)
		}
		fmt.Println()
	}

	if len(diffResult.Modified) > 0 {
		fmt.Printf("Modified (%d):\n", len(diffResult.Modified))
		for _, path := range diffResult.Modified {
			fmt.Printf("  ~ %s\n", path)
		}
		fmt.Println()
	}

	if len(diffResult.Deleted) > 0 {
		fmt.Printf("Deleted (%d):\n", len(diffResult.Deleted))
		for _, path := range diffResult.Deleted {
			fmt.Printf("  - %s\n", path)
		}
		fmt.Println()
	}

	total := len(diffResult.Added) + len(diffResult.Modified) + len(diffResult.Deleted)
	if total == 0 {
		fmt.Println("No changes detected.")
	} else {
		fmt.Printf("Summary: %d added, %d modified, %d deleted (%d unchanged)\n",
			len(diffResult.Added), len(diffResult.Modified), len(diffResult.Deleted), diffResult.Unchanged)
	}

	return nil
}

func runShow(cmd *cobra.Command, args []string) error {
	// Determine which index to show
	name := ""
	if len(args) > 0 {
		name = args[0]
	}

	// Find index entry
	entry, name, err := findIndex(name)
	if err != nil {
		return err
	}

	// Read index file
	content, err := os.ReadFile(entry.IndexPath)
	if err != nil {
		return fmt.Errorf("failed to read index file: %w", err)
	}

	// Handle encrypted content
	if entry.Encrypted {
		// TODO: Implement decryption
		return fmt.Errorf("encrypted indexes not yet supported in show command")
	}

	if showRaw {
		fmt.Print(string(content))
		return nil
	}

	// Print with header
	fmt.Printf("# Index: %s\n", name)
	fmt.Printf("# Path: %s\n", entry.Path)
	fmt.Printf("# Last indexed: %s\n", entry.LastIndexed)
	fmt.Printf("# Format: %s | Files: %d | Size: %d bytes\n", entry.Format, entry.FileCount, entry.SizeBytes)
	fmt.Println("---")

	if showSummary {
		// Extract just the summary section
		lines := strings.Split(string(content), "\n")
		inSummary := false
		for _, line := range lines {
			if strings.HasPrefix(line, "## Summary") || strings.HasPrefix(line, "# Summary") {
				inSummary = true
			} else if inSummary && strings.HasPrefix(line, "## ") {
				break
			}
			if inSummary {
				fmt.Println(line)
			}
		}
		if !inSummary {
			// No summary section, print first 50 lines
			for i, line := range lines {
				if i >= 50 {
					fmt.Println("...")
					break
				}
				fmt.Println(line)
			}
		}
	} else {
		fmt.Print(string(content))
	}

	return nil
}

func runList(cmd *cobra.Command, args []string) error {
	if envConfig == nil || envConfig.Indexes == nil || len(envConfig.Indexes) == 0 {
		log.Println("No indexes registered.")
		log.Println("Use 'comanda index capture' to create one.")
		return nil
	}

	if listJSON {
		// TODO: Implement JSON output
		return fmt.Errorf("JSON output not yet implemented")
	}

	// Print header
	if listPaths {
		fmt.Printf("%-15s %-45s %-20s %-12s %s\n", "NAME", "PATH", "LAST INDEXED", "FORMAT", "FILES")
		fmt.Println(strings.Repeat("-", 110))
	} else {
		fmt.Printf("%-15s %-30s %-20s %-12s %s\n", "NAME", "PATH", "LAST INDEXED", "FORMAT", "FILES")
		fmt.Println(strings.Repeat("-", 90))
	}

	// Print entries
	for name, entry := range envConfig.Indexes {
		path := entry.Path
		if !listPaths {
			// Shorten path
			if home, err := os.UserHomeDir(); err == nil {
				path = strings.Replace(path, home, "~", 1)
			}
			if len(path) > 28 {
				path = "..." + path[len(path)-25:]
			}
		}

		// Parse and format timestamp
		lastIndexed := entry.LastIndexed
		if t, err := time.Parse(time.RFC3339, entry.LastIndexed); err == nil {
			lastIndexed = t.Format("2006-01-02 15:04")
		}

		if listPaths {
			fmt.Printf("%-15s %-45s %-20s %-12s %d\n", name, path, lastIndexed, entry.Format, entry.FileCount)
		} else {
			fmt.Printf("%-15s %-30s %-20s %-12s %d\n", name, path, lastIndexed, entry.Format, entry.FileCount)
		}
	}

	return nil
}

func runRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	if envConfig == nil || envConfig.Indexes == nil {
		return fmt.Errorf("no indexes registered")
	}

	entry, ok := envConfig.Indexes[name]
	if !ok {
		return fmt.Errorf("index '%s' not found", name)
	}

	// Delete file if requested
	if removeDelete {
		if err := os.Remove(entry.IndexPath); err != nil && !os.IsNotExist(err) {
			log.Printf("Warning: failed to delete index file: %v\n", err)
		} else {
			log.Printf("Deleted: %s\n", entry.IndexPath)
		}
	}

	// Remove from config
	comandaDir, err := config.GetComandaDir()
	if err != nil {
		return err
	}
	configPath := filepath.Join(comandaDir, "config.yaml")

	// Load config
	envCfg, err := config.LoadEnvConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Remove entry
	delete(envCfg.Indexes, name)

	// Write back
	if err := config.SaveEnvConfig(configPath, envCfg); err != nil {
		return fmt.Errorf("failed to update config: %w", err)
	}

	log.Printf("Removed index '%s' from registry\n", name)
	return nil
}

// findIndex finds an index by name, or by current directory if name is empty
func findIndex(name string) (*config.IndexEntry, string, error) {
	if envConfig == nil || envConfig.Indexes == nil || len(envConfig.Indexes) == 0 {
		return nil, "", fmt.Errorf("no indexes registered (use 'comanda index capture' first)")
	}

	if name != "" {
		entry, ok := envConfig.Indexes[name]
		if !ok {
			return nil, "", fmt.Errorf("index '%s' not found", name)
		}
		return entry, name, nil
	}

	// Find by current directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil, "", fmt.Errorf("failed to get current directory: %w", err)
	}

	for n, entry := range envConfig.Indexes {
		if entry.Path == cwd {
			return entry, n, nil
		}
	}

	return nil, "", fmt.Errorf("no index found for current directory (use 'comanda index capture' or specify name)")
}

// Config helpers - use config.LoadEnvConfig and config.SaveEnvConfig directly
