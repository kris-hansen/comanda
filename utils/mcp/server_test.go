package mcp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/skills"
)

const testSkill = `---
name: test-skill
description: A skill for testing
arguments:
  - topic
argument-hint: The topic to write about
---

Write a haiku about ${topic}.
`

// connectTestServer builds an MCP server over the in-memory transport and
// returns a connected client session.
func connectTestServer(t *testing.T, opts Options) *mcpsdk.ClientSession {
	t.Helper()

	server, err := NewServer(opts)
	if err != nil {
		t.Fatalf("NewServer() error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	serverTransport, clientTransport := mcpsdk.NewInMemoryTransports()
	go func() {
		_ = server.Run(ctx, serverTransport)
	}()

	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "v0.0.1"}, nil)
	cs, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client Connect() error: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })
	return cs
}

func TestServerToolsListAndCall(t *testing.T) {
	dir := t.TempDir()
	path := writeWorkflow(t, dir, "echo.yaml", naEchoWorkflow)
	defs, err := DiscoverWorkflows(nil, []string{path})
	if err != nil {
		t.Fatalf("DiscoverWorkflows() error: %v", err)
	}

	cs := connectTestServer(t, Options{
		Name:      "comanda-test",
		Version:   "v0.0.1",
		EnvConfig: &config.EnvConfig{},
		Workflows: defs,
	})
	ctx := context.Background()

	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error: %v", err)
	}
	if len(tools.Tools) != 1 {
		t.Fatalf("ListTools() returned %d tools, want 1", len(tools.Tools))
	}
	tool := tools.Tools[0]
	if tool.Name != "echo" {
		t.Errorf("tool name = %q, want %q", tool.Name, "echo")
	}
	if tool.Description != "Echo input back" {
		t.Errorf("tool description = %q, want %q", tool.Description, "Echo input back")
	}

	result, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "echo",
		Arguments: map[string]any{"input": "hello over mcp"},
	})
	if err != nil {
		t.Fatalf("CallTool() error: %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool() returned tool error: %+v", result.Content)
	}
	if len(result.Content) == 0 {
		t.Fatal("CallTool() returned no content")
	}
	text, ok := result.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("CallTool() content type = %T, want *TextContent", result.Content[0])
	}
	if !strings.Contains(text.Text, "hello over mcp") {
		t.Errorf("CallTool() output = %q, want it to contain %q", text.Text, "hello over mcp")
	}
}

func TestServerToolCallError(t *testing.T) {
	// A def pointing at a file that no longer exists exercises the tool-error
	// path (IsError result rather than a JSON-RPC failure).
	broken := WorkflowDef{
		Name:        "broken",
		Path:        filepath.Join(t.TempDir(), "gone.yaml"),
		Description: "Broken workflow",
	}

	cs := connectTestServer(t, Options{
		Name:      "comanda-test",
		Version:   "v0.0.1",
		EnvConfig: &config.EnvConfig{},
		Workflows: []WorkflowDef{broken},
	})

	result, err := cs.CallTool(context.Background(), &mcpsdk.CallToolParams{Name: "broken"})
	if err != nil {
		t.Fatalf("CallTool() protocol error (expected tool error instead): %v", err)
	}
	if !result.IsError {
		t.Errorf("CallTool() IsError = false, want true")
	}
}

func TestServerPrompts(t *testing.T) {
	skill, err := skills.ParseSkill(testSkill)
	if err != nil {
		t.Fatalf("ParseSkill() error: %v", err)
	}

	cs := connectTestServer(t, Options{
		Name:      "comanda-test",
		Version:   "v0.0.1",
		EnvConfig: &config.EnvConfig{},
		Skills:    []*skills.Skill{skill},
	})
	ctx := context.Background()

	prompts, err := cs.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatalf("ListPrompts() error: %v", err)
	}
	if len(prompts.Prompts) != 1 {
		t.Fatalf("ListPrompts() returned %d prompts, want 1", len(prompts.Prompts))
	}
	prompt := prompts.Prompts[0]
	if prompt.Name != "test-skill" {
		t.Errorf("prompt name = %q, want %q", prompt.Name, "test-skill")
	}
	if prompt.Description != "A skill for testing" {
		t.Errorf("prompt description = %q, want %q", prompt.Description, "A skill for testing")
	}
	if len(prompt.Arguments) != 1 || prompt.Arguments[0].Name != "topic" {
		t.Fatalf("prompt arguments = %+v, want one 'topic' argument", prompt.Arguments)
	}
	if prompt.Arguments[0].Description != "The topic to write about" {
		t.Errorf("argument description = %q, want argument hint", prompt.Arguments[0].Description)
	}

	result, err := cs.GetPrompt(ctx, &mcpsdk.GetPromptParams{
		Name:      "test-skill",
		Arguments: map[string]string{"topic": "moss"},
	})
	if err != nil {
		t.Fatalf("GetPrompt() error: %v", err)
	}
	if len(result.Messages) != 1 {
		t.Fatalf("GetPrompt() returned %d messages, want 1", len(result.Messages))
	}
	msg := result.Messages[0]
	if msg.Role != "user" {
		t.Errorf("message role = %q, want %q", msg.Role, "user")
	}
	text, ok := msg.Content.(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("message content type = %T, want *TextContent", msg.Content)
	}
	if !strings.Contains(text.Text, "Write a haiku about moss.") {
		t.Errorf("prompt body = %q, want it to contain %q", text.Text, "Write a haiku about moss.")
	}
}
