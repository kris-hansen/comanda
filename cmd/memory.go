package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kris-hansen/comanda/utils/semanticmemory"
	"github.com/spf13/cobra"
)

var (
	memoryDBPath    string
	memoryNamespace string
	memoryType      string
	memoryPriority  int
	memorySourceRef string
	memoryLimit     int
	memoryTypes     string
	memoryJSON      bool
)

var memoryCmd = &cobra.Command{
	Use:   "memory",
	Short: "Inspect and manage durable workflow memory",
	Long: `Manage project-local durable memory used by workflow steps with a semantic memory mapping.

By default, data is stored in .comanda/memory/<namespace>.db under the current directory.
The database is local SQLite with an FTS5 index; it can be inspected or backed up normally.`,
}

var memoryAddCmd = &cobra.Command{
	Use:   "add <fact>",
	Short: "Add a durable fact to memory",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		store, err := openMemoryStore()
		if err != nil {
			return err
		}
		defer store.Close()
		record, err := store.Upsert(context.Background(), semanticmemory.Record{
			Namespace: memoryNamespace,
			Type:      memoryType,
			Priority:  memoryPriority,
			Content:   args[0],
			SourceRef: memorySourceRef,
		})
		if err != nil {
			return err
		}
		return printMemory(record)
	},
}

var memorySearchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search durable memory with local full-text retrieval",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		store, err := openMemoryStore()
		if err != nil {
			return err
		}
		defer store.Close()
		records, err := store.Search(context.Background(), args[0], semanticmemory.SearchOptions{
			Namespace: memoryNamespace,
			Types:     splitMemoryTypes(memoryTypes),
			Limit:     memoryLimit,
		})
		if err != nil {
			return err
		}
		if memoryJSON {
			return json.NewEncoder(os.Stdout).Encode(records)
		}
		for _, record := range records {
			fmt.Printf("%s  [%s, priority %d]\n%s\nsource: %s\n\n", record.ID, record.Type, record.Priority, record.Content, displaySource(record))
		}
		return nil
	},
}

var memoryShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show a durable memory and its provenance",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		store, err := openMemoryStore()
		if err != nil {
			return err
		}
		defer store.Close()
		record, err := store.Get(context.Background(), args[0])
		if err != nil {
			return err
		}
		return printMemory(record)
	},
}

func init() {
	rootCmd.AddCommand(memoryCmd)
	memoryCmd.AddCommand(memoryAddCmd, memorySearchCmd, memoryShowCmd)
	memoryCmd.PersistentFlags().StringVar(&memoryDBPath, "db", "", "Path to a memory SQLite database (default: project-local)")
	memoryCmd.PersistentFlags().StringVarP(&memoryNamespace, "namespace", "n", "default", "Memory namespace")
	memoryCmd.PersistentFlags().BoolVar(&memoryJSON, "json", false, "Output JSON")
	memoryAddCmd.Flags().StringVarP(&memoryType, "type", "t", "finding", "Memory type (for example: decision, constraint, finding, failure)")
	memoryAddCmd.Flags().IntVarP(&memoryPriority, "priority", "p", 50, "Memory priority (0-100)")
	memoryAddCmd.Flags().StringVar(&memorySourceRef, "source", "manual", "Source reference for provenance")
	memorySearchCmd.Flags().IntVarP(&memoryLimit, "limit", "l", 5, "Maximum memories to return")
	memorySearchCmd.Flags().StringVar(&memoryTypes, "types", "", "Comma-separated memory types to include")
}

func openMemoryStore() (*semanticmemory.Store, error) {
	path := memoryDBPath
	if path == "" {
		root, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("resolve current directory: %w", err)
		}
		path = semanticmemory.DefaultPath(root, memoryNamespace)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create project memory directory: %w", err)
		}
	} else if !filepath.IsAbs(path) {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve memory database path: %w", err)
		}
		path = absolute
	}
	return semanticmemory.Open(path)
}

func splitMemoryTypes(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func printMemory(record semanticmemory.Record) error {
	if memoryJSON {
		return json.NewEncoder(os.Stdout).Encode(record)
	}
	fmt.Printf("ID: %s\nNamespace: %s\nType: %s\nPriority: %d\nConfidence: %.2f\nContent: %s\nSource: %s\nCreated: %s\nUpdated: %s\n",
		record.ID, record.Namespace, record.Type, record.Priority, record.Confidence, record.Content,
		displaySource(record), record.CreatedAt.Format("2006-01-02 15:04:05Z07:00"), record.UpdatedAt.Format("2006-01-02 15:04:05Z07:00"))
	return nil
}

func displaySource(record semanticmemory.Record) string {
	parts := []string{record.SourceRef, record.SourceRunID, record.SourceStep}
	var nonEmpty []string
	for _, part := range parts {
		if part != "" {
			nonEmpty = append(nonEmpty, part)
		}
	}
	if len(nonEmpty) == 0 {
		return "manual"
	}
	return strings.Join(nonEmpty, " / ")
}
