package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/skills"
)

// Options configures the MCP server.
type Options struct {
	Name      string // Server name reported to MCP clients
	Version   string // Server version reported to MCP clients
	Verbose   bool   // Enable debug logging (to stderr)
	EnvConfig *config.EnvConfig
	Workflows []WorkflowDef   // Workflows to expose as tools
	Skills    []*skills.Skill // Skills to expose as prompts (nil/empty disables)
}

// NewServer builds an MCP server exposing each workflow as a tool and each
// skill as a prompt.
func NewServer(opts Options) (*mcpsdk.Server, error) {
	name := opts.Name
	if name == "" {
		name = "comanda"
	}
	s := mcpsdk.NewServer(&mcpsdk.Implementation{Name: name, Version: opts.Version}, nil)

	runner := NewRunner(opts.EnvConfig, opts.Verbose)
	for _, def := range opts.Workflows {
		registerTool(s, runner, def)
	}
	for _, skill := range opts.Skills {
		registerPrompt(s, skill, opts.Verbose)
	}
	return s, nil
}

// registerTool exposes a single workflow as an MCP tool.
func registerTool(s *mcpsdk.Server, runner *Runner, def WorkflowDef) {
	tool := &mcpsdk.Tool{
		Name:        def.Name,
		Description: def.Description,
		InputSchema: toolInputSchema(def),
	}
	s.AddTool(tool, func(ctx context.Context, req *mcpsdk.CallToolRequest) (*mcpsdk.CallToolResult, error) {
		return runToolCall(ctx, runner, def, req), nil
	})
}

// toolInputSchema builds a JSON Schema object with one optional string
// property per workflow variable plus the reserved "input" property.
func toolInputSchema(def WorkflowDef) map[string]any {
	properties := make(map[string]any, len(def.Vars)+1)
	for _, v := range def.Vars {
		if v == "input" {
			// "input" is always the reserved STDIN-style argument; a workflow
			// variable named "input" cannot be set separately.
			continue
		}
		properties[v] = map[string]any{
			"type":        "string",
			"description": fmt.Sprintf("Value for the {{ %s }} workflow variable", v),
		}
	}
	properties["input"] = map[string]any{
		"type":        "string",
		"description": "Input text passed to the workflow as STDIN-style input",
	}
	return map[string]any{
		"type":       "object",
		"properties": properties,
	}
}

// runToolCall executes a workflow for one tools/call request. Workflow and
// argument errors are reported as tool-error results (IsError) rather than
// JSON-RPC protocol errors.
func runToolCall(ctx context.Context, runner *Runner, def WorkflowDef, req *mcpsdk.CallToolRequest) *mcpsdk.CallToolResult {
	args := make(map[string]string)
	if req.Params != nil && len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return toolError(fmt.Errorf("invalid arguments for tool %q: %w", def.Name, err))
		}
	}

	output, err := runner.Run(ctx, def, args)
	if err != nil {
		return toolError(fmt.Errorf("workflow %q failed: %w", def.Name, err))
	}
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: output}},
	}
}

// toolError wraps err as a tool-error result.
func toolError(err error) *mcpsdk.CallToolResult {
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: err.Error()}},
		IsError: true,
	}
}

// ServeStdio serves the MCP server over stdin/stdout until the client
// disconnects or ctx is canceled. Nothing except MCP protocol frames may be
// written to stdout while this runs; cmd sets log output to stderr.
func ServeStdio(ctx context.Context, s *mcpsdk.Server) error {
	log.Printf("[INFO][MCP] Serving MCP over stdio\n")
	return s.Run(ctx, &mcpsdk.StdioTransport{})
}

// ServeHTTP serves the MCP server over the streamable HTTP transport. It is
// intended for localhost use only; no authentication is applied.
func ServeHTTP(ctx context.Context, s *mcpsdk.Server, addr string) error {
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return s }, nil)
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	log.Printf("[INFO][MCP] Serving MCP over HTTP on %s (localhost-trusted, no authentication)\n", addr)
	err := httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
