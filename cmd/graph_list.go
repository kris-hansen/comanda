package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/kris-hansen/comanda/utils/semanticmemory"
	"github.com/spf13/cobra"
)

var graphListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available knowledge graphs",
	Long: `List built graphs in the project-local databases of registered indexes.
Use --db to list graphs in a custom database, or --namespace to filter by name.
Indexes with no graph are omitted.`,
	Args: cobra.NoArgs,
	RunE: runGraphList,
}

type graphListEntry struct {
	semanticmemory.GraphSummary
	Database string `json:"database"`
}

func runGraphList(cmd *cobra.Command, _ []string) error {
	entries, err := listGraphs(cmd.Context(), graphNamespace, graphDBPath)
	if err != nil {
		return err
	}
	asJSON, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if asJSON {
		return json.NewEncoder(out).Encode(entries)
	}
	if len(entries) == 0 {
		_, err := fmt.Fprintln(out, "No graphs found. Use 'comanda graph build [index-name]' to build one.")
		return err
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tNODES\tEDGES\tDATABASE")
	for _, entry := range entries {
		fmt.Fprintf(w, "%s\t%d\t%d\t%s\n", entry.Namespace, entry.Nodes, entry.Edges, entry.Database)
	}
	return w.Flush()
}

func listGraphs(ctx context.Context, namespace, dbPath string) ([]graphListEntry, error) {
	entries := make([]graphListEntry, 0)
	read := func(path, filter string, allowMissing bool) error {
		// Listing must not create databases for indexes without a graph.
		if _, err := os.Stat(path); err != nil {
			if allowMissing && os.IsNotExist(err) {
				return nil
			}
			return fmt.Errorf("inspect graph database %q: %w", path, err)
		}
		store, err := semanticmemory.Open(path)
		if err != nil {
			return fmt.Errorf("open graph database %q: %w", path, err)
		}
		defer store.Close()
		summaries, err := store.GraphSummaries(ctx)
		if err != nil {
			return fmt.Errorf("list graphs in %q: %w", path, err)
		}
		for _, summary := range summaries {
			if filter == "" || summary.Namespace == filter {
				entries = append(entries, graphListEntry{GraphSummary: summary, Database: path})
			}
		}
		return nil
	}
	if dbPath != "" {
		if err := read(dbPath, namespace, false); err != nil {
			return nil, err
		}
	} else if envConfig != nil {
		var names []string
		for name := range envConfig.Indexes {
			if namespace == "" || name == namespace {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			entry := envConfig.Indexes[name]
			if entry == nil || entry.Path == "" {
				continue
			}
			if err := read(semanticmemory.DefaultPath(entry.Path, name), name, true); err != nil {
				return nil, err
			}
		}
	}
	return entries, nil
}
