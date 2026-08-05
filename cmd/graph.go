package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/kris-hansen/comanda/utils/codebaseindex"
	"github.com/kris-hansen/comanda/utils/knowledgegraph"
	"github.com/kris-hansen/comanda/utils/semanticmemory"
	"github.com/spf13/cobra"
)

var (
	graphNamespace    string
	graphDBPath       string
	graphEnhance      bool
	graphEnhanceModel string
	graphJSON         bool
	graphExportOutput string
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Build and query knowledge graphs from codebase indexes",
	Long: `Build a knowledge graph from a registered codebase index and store it in
the semantic memory store, then query it instead of grepping files.

The graph is extracted deterministically from the index scan (components,
packages, files, symbols, imports). Edges carry confidence tags: EXTRACTED
means explicit in the source, INFERRED means resolved by name matching or an
optional AI enhancement pass (--enhance).

Graph data lives in the project's semantic memory database
(.comanda/memory/<index-name>.db), so 'comanda memory search' and workflow
memory recall (types: [graph_node]) see graph concepts too.

Examples:
  comanda graph build myproject              # Build graph from a registered index
  comanda graph build myproject --enhance    # Add AI-inferred concept nodes/edges
  comanda graph explain Store                # Show a node and its connections
  comanda graph path main Store              # Shortest connection between nodes
  comanda graph query "what uses the store?" # Scoped subgraph for a question
  comanda graph stats                        # Counts and hub nodes
  comanda graph export -o graph.json         # graphify-style JSON export`,
}

var graphBuildCmd = &cobra.Command{
	Use:   "build <index-name>",
	Short: "Build (or rebuild) the knowledge graph for a registered index",
	Args:  cobra.ExactArgs(1),
	RunE:  runGraphBuild,
}

var graphUpdateCmd = &cobra.Command{
	Use:   "update <index-name>",
	Short: "Rebuild the knowledge graph from a fresh scan of the index",
	Long: `Rebuild the knowledge graph for a registered index.

The graph is rebuilt from a fresh deterministic scan and replaces the stored
graph for the namespace (stale nodes from deleted files are removed).`,
	Args: cobra.ExactArgs(1),
	RunE: runGraphBuild,
}

var graphExplainCmd = &cobra.Command{
	Use:   "explain <node>",
	Short: "Show a node and its connections",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		q, closeStore, err := openGraphQuerier()
		if err != nil {
			return err
		}
		defer closeStore()
		ctx := context.Background()
		if graphJSON {
			node, err := q.Resolve(ctx, args[0])
			if err != nil {
				return err
			}
			sub, err := q.ScopedExport(ctx, []semanticmemory.GraphNode{*node}, 1)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(sub)
		}
		out, err := q.Explain(ctx, args[0])
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil
	},
}

var graphPathCmd = &cobra.Command{
	Use:   "path <A> <B>",
	Short: "Show the shortest connection between two nodes",
	Args:  cobra.ExactArgs(2),
	RunE: func(_ *cobra.Command, args []string) error {
		q, closeStore, err := openGraphQuerier()
		if err != nil {
			return err
		}
		defer closeStore()
		ctx := context.Background()
		if graphJSON {
			hops, err := q.PathHops(ctx, args[0], args[1])
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(map[string]any{
				"from": args[0], "to": args[1], "hops": hops,
			})
		}
		out, err := q.Path(ctx, args[0], args[1])
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil
	},
}

var graphQueryCmd = &cobra.Command{
	Use:   "query <question>",
	Short: "Answer a question with a scoped subgraph",
	Args:  cobra.ExactArgs(1),
	RunE: func(_ *cobra.Command, args []string) error {
		q, closeStore, err := openGraphQuerier()
		if err != nil {
			return err
		}
		defer closeStore()
		ctx := context.Background()
		if graphJSON {
			seeds, err := q.FindNodes(ctx, args[0], 5)
			if err != nil {
				return err
			}
			sub, err := q.ScopedExport(ctx, seeds, 1)
			if err != nil {
				return err
			}
			return json.NewEncoder(os.Stdout).Encode(sub)
		}
		out, err := q.Query(ctx, args[0])
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil
	},
}

var graphStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show graph counts and hub nodes",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		q, closeStore, err := openGraphQuerier()
		if err != nil {
			return err
		}
		defer closeStore()
		out, err := q.Stats(context.Background())
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil
	},
}

var graphExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export the graph as graphify-style JSON",
	Args:  cobra.NoArgs,
	RunE: func(_ *cobra.Command, _ []string) error {
		q, closeStore, err := openGraphQuerier()
		if err != nil {
			return err
		}
		defer closeStore()
		exported, err := q.Export(context.Background())
		if err != nil {
			return err
		}
		data, err := json.MarshalIndent(exported, "", "  ")
		if err != nil {
			return err
		}
		if graphExportOutput != "" {
			if err := os.WriteFile(graphExportOutput, data, 0644); err != nil {
				return fmt.Errorf("failed to write export: %w", err)
			}
			log.Printf("Graph exported to %s (%d nodes, %d edges)\n", graphExportOutput, len(exported.Nodes), len(exported.Edges))
			return nil
		}
		fmt.Println(string(data))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(graphCmd)
	graphCmd.AddCommand(graphBuildCmd, graphUpdateCmd, graphExplainCmd, graphPathCmd, graphQueryCmd, graphStatsCmd, graphExportCmd)

	graphCmd.PersistentFlags().StringVarP(&graphNamespace, "namespace", "n", "", "Graph namespace (default: index registered for current directory)")
	graphCmd.PersistentFlags().StringVar(&graphDBPath, "db", "", "Path to the graph/memory SQLite database (default: project-local)")

	graphBuildCmd.Flags().BoolVar(&graphEnhance, "enhance", false, "Add AI-inferred concept nodes and edges using default_generation_model")
	graphBuildCmd.Flags().StringVar(&graphEnhanceModel, "enhance-model", "", "Model for --enhance (default: configured default_generation_model)")
	graphUpdateCmd.Flags().BoolVar(&graphEnhance, "enhance", false, "Add AI-inferred concept nodes and edges using default_generation_model")
	graphUpdateCmd.Flags().StringVar(&graphEnhanceModel, "enhance-model", "", "Model for --enhance (default: configured default_generation_model)")

	graphExplainCmd.Flags().BoolVar(&graphJSON, "json", false, "Output JSON subgraph")
	graphPathCmd.Flags().BoolVar(&graphJSON, "json", false, "Output JSON hop list")
	graphQueryCmd.Flags().BoolVar(&graphJSON, "json", false, "Output JSON subgraph")

	graphExportCmd.Flags().StringVarP(&graphExportOutput, "output", "o", "", "Write export to a file instead of stdout")
}

// buildKnowledgeGraph scans the repo behind a registered index and rebuilds
// its knowledge graph. Shared by index capture/update (--graph) and the
// graph build/update commands.
func buildKnowledgeGraph(indexName, repoPath string, enhance bool, enhanceModel string) error {
	entryEncrypted := false
	if envConfig != nil && envConfig.Indexes != nil {
		if entry, ok := envConfig.Indexes[indexName]; ok {
			entryEncrypted = entry.Encrypted
		}
	}
	if entryEncrypted {
		log.Printf("Warning: skipping graph build for encrypted index '%s'\n", indexName)
		return nil
	}

	cfg := codebaseindex.DefaultConfig()
	cfg.Root = repoPath
	cfg.Verbose = verbose
	cfg.MaxFiles = indexMaxFiles
	cfg.MaxFilesPerDir = indexMaxFilesPerDir

	manager, err := codebaseindex.NewManager(cfg, verbose)
	if err != nil {
		return fmt.Errorf("failed to create index manager: %w", err)
	}

	log.Printf("Scanning %s for graph build...\n", repoPath)
	scan, _, err := manager.Scan()
	if err != nil {
		return fmt.Errorf("graph scan failed: %w", err)
	}

	graph := knowledgegraph.Build(scan, indexName)

	if enhance {
		modelName, enhanceFunc, err := buildIndexEnhancer(enhanceModel)
		if err != nil {
			return err
		}
		log.Printf("Enhancing graph with model: %s\n", modelName)
		if err := knowledgegraph.Enhance(graph, enhanceFunc); err != nil {
			log.Printf("Warning: graph enhancement skipped: %v\n", err)
		}
	}

	dbPath := graphDBPath
	if dbPath == "" {
		dbPath = semanticmemory.DefaultPath(repoPath, indexName)
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return fmt.Errorf("create graph database directory: %w", err)
	}
	store, err := semanticmemory.Open(dbPath)
	if err != nil {
		return err
	}
	defer store.Close()

	if err := knowledgegraph.Rebuild(context.Background(), store, graph); err != nil {
		return fmt.Errorf("failed to store graph: %w", err)
	}

	log.Printf("Knowledge graph built: %d nodes, %d edges (namespace %s, db %s)\n",
		len(graph.Nodes), len(graph.Edges), indexName, dbPath)
	return nil
}

func runGraphBuild(_ *cobra.Command, args []string) error {
	entry, name, err := findIndex(args[0])
	if err != nil {
		return err
	}
	return buildKnowledgeGraph(name, entry.Path, graphEnhance, graphEnhanceModel)
}

// openGraphQuerier resolves the namespace (flag, or the index registered for
// the current directory) and opens a querier on the right database.
func openGraphQuerier() (*knowledgegraph.Querier, func() error, error) {
	namespace := graphNamespace
	dbPath := graphDBPath

	if dbPath == "" {
		root := ""
		if namespace == "" {
			// Default: the index registered for the current directory.
			entry, name, err := findIndex("")
			if err != nil {
				return nil, nil, fmt.Errorf("no namespace given and %v", err)
			}
			namespace = name
			root = entry.Path
		} else if envConfig != nil && envConfig.Indexes != nil {
			if entry, ok := envConfig.Indexes[namespace]; ok {
				root = entry.Path
			}
		}
		if root == "" {
			var err error
			root, err = os.Getwd()
			if err != nil {
				return nil, nil, fmt.Errorf("resolve current directory: %w", err)
			}
		}
		dbPath = semanticmemory.DefaultPath(root, namespace)
	}

	store, err := semanticmemory.Open(dbPath)
	if err != nil {
		return nil, nil, err
	}
	return knowledgegraph.NewQuerier(store, namespace), store.Close, nil
}
