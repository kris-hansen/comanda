package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	comandamcp "github.com/kris-hansen/comanda/utils/mcp"
	"github.com/kris-hansen/comanda/utils/skills"
)

var (
	mcpDirs     []string
	mcpFiles    []string
	mcpHTTPAddr string
	mcpNoSkills bool
	mcpName     string
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run an MCP server exposing workflows as tools and skills as prompts",
	Long: `Start a Model Context Protocol (MCP) server so MCP-native agents
(Claude Code, Kimi Code, Codex, Cursor) can run comanda workflows as tools
and use comanda skills as prompts.

Workflows are discovered from ~/.comanda/workflows/ and .comanda/workflows/
when those directories exist; use --dir and --workflow to add more. Each
discovered workflow file becomes one MCP tool; each skill becomes one MCP
prompt. The server speaks MCP over stdio by default; use --http to serve
the streamable HTTP transport instead (localhost-trusted, no authentication).`,
	Example: `  # Serve workflows from the default directories over stdio
  comanda mcp

  # Serve specific workflow files
  comanda mcp --workflow summarize.yaml --workflow review.yaml

  # Add a directory of workflows and disable skills
  comanda mcp --dir ./workflows --no-skills

  # Serve over HTTP on localhost port 8080
  comanda mcp --http :8080`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// In stdio mode stdout carries MCP protocol frames only; all logging
		// and stray workflow output must go to stderr.
		log.SetOutput(os.Stderr)

		dirs := append(comandamcp.DefaultWorkflowDirs(), mcpDirs...)
		defs, err := comandamcp.DiscoverWorkflows(dirs, mcpFiles)
		if err != nil {
			return err
		}

		var allSkills []*skills.Skill
		if !mcpNoSkills {
			skillIdx := skills.NewIndex()
			if err := skillIdx.Load(); err != nil {
				log.Printf("[WARN] Failed to load skills: %v\n", err)
			} else {
				allSkills = skillIdx.All()
			}
		}

		if len(defs) == 0 && len(allSkills) == 0 {
			return fmt.Errorf("no workflows or skills discovered; create %s or point at workflows with --dir/--workflow", "~/.comanda/workflows/")
		}

		server, err := comandamcp.NewServer(comandamcp.Options{
			Name:      mcpName,
			Version:   getVersion(),
			Verbose:   verbose,
			EnvConfig: envConfig,
			Workflows: defs,
			Skills:    allSkills,
		})
		if err != nil {
			return err
		}

		log.Printf("[INFO] MCP server %q starting: %d workflow tool(s), %d skill prompt(s)\n", mcpName, len(defs), len(allSkills))
		if verbose {
			for _, def := range defs {
				log.Printf("[DEBUG][MCP] Tool %s -> %s (vars: %v)\n", def.Name, def.Path, def.Vars)
			}
			for _, skill := range allSkills {
				log.Printf("[DEBUG][MCP] Prompt %s (%s)\n", skill.DisplayName(), skill.FilePath)
			}
		}

		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()

		if mcpHTTPAddr != "" {
			return comandamcp.ServeHTTP(ctx, server, mcpHTTPAddr)
		}
		return comandamcp.ServeStdio(ctx, server)
	},
}

func init() {
	mcpCmd.Flags().StringArrayVar(&mcpDirs, "dir", []string{}, "Directory to scan for workflow .yaml/.yml files (repeatable; adds to default directories)")
	mcpCmd.Flags().StringArrayVar(&mcpFiles, "workflow", []string{}, "Workflow file to expose as a tool (repeatable)")
	mcpCmd.Flags().StringVar(&mcpHTTPAddr, "http", "", "Serve MCP over streamable HTTP on this address (e.g. :8080) instead of stdio")
	mcpCmd.Flags().BoolVar(&mcpNoSkills, "no-skills", false, "Do not expose skills as MCP prompts")
	mcpCmd.Flags().StringVar(&mcpName, "name", "comanda", "MCP server name reported to clients")
	rootCmd.AddCommand(mcpCmd)
}
