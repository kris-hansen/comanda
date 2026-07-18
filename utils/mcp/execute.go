package mcp

import (
	"context"
	"io"
	"log"
	"os"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/processor"
)

// Runner executes workflows on behalf of MCP tool calls.
type Runner struct {
	envConfig *config.EnvConfig
	verbose   bool
}

// NewRunner creates a Runner that executes workflows with the given
// environment configuration.
func NewRunner(envConfig *config.EnvConfig, verbose bool) *Runner {
	return &Runner{envConfig: envConfig, verbose: verbose}
}

// Run executes the workflow defined by def with the given tool arguments and
// returns the workflow's final output. All arguments except the reserved
// "input" argument are passed as CLI variables ({{ var }} substitution);
// "input" is fed to the workflow as STDIN-style input via SetLastOutput.
func (r *Runner) Run(ctx context.Context, def WorkflowDef, args map[string]string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}

	yamlFile, err := os.ReadFile(def.Path)
	if err != nil {
		return "", err
	}

	var dslConfig processor.DSLConfig
	if err := yaml.Unmarshal(yamlFile, &dslConfig); err != nil {
		return "", err
	}

	cliVars := make(map[string]string, len(args))
	for key, value := range args {
		if key == "input" {
			continue
		}
		cliVars[key] = value
	}

	// No --runtime-dir flag in MCP mode: replicate the resolveProcessRuntimeDir
	// default from cmd/process.go ("" means relative paths resolve from cwd).
	runtimeDir := ""

	proc := processor.NewProcessor(&dslConfig, r.envConfig, &config.ServerConfig{Enabled: false}, r.verbose, runtimeDir, cliVars)
	proc.SetWorkflowFile(def.Path)
	// Spinner and progress display write directly to the console; disable them
	// so they cannot interleave with MCP protocol frames.
	proc.DisableSpinner()
	proc.DisableProgressDisplay()

	if input := args["input"]; input != "" {
		proc.SetLastOutput(input)
	}

	if r.verbose {
		log.Printf("[DEBUG][MCP] Running workflow %s (%s) with %d var(s)\n", def.Name, def.Path, len(cliVars))
	}

	// Redirect os.Stdout to stderr while the processor runs so that any
	// fmt.Print* output from workflow steps cannot corrupt the stdio transport.
	if err := runWithStdoutGuard(proc.Process); err != nil {
		return "", err
	}

	return proc.LastOutput(), nil
}

// stdoutGuard serializes stdout redirection across concurrent tool calls.
var stdoutGuard sync.Mutex

// runWithStdoutGuard executes fn while the os.Stdout variable points at a pipe
// whose contents are copied to os.Stderr, then restores it. This keeps the
// stdio MCP transport channel clean: the SDK captured the original os.Stdout
// when the transport connected, so protocol frames are unaffected, while Go
// code that reads os.Stdout dynamically (fmt.Print* in the processor) is
// redirected. If the pipe cannot be created, fn runs without redirection.
func runWithStdoutGuard(fn func() error) error {
	stdoutGuard.Lock()
	defer stdoutGuard.Unlock()

	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		log.Printf("[WARN][MCP] Failed to create stdout guard pipe, running without redirection: %v\n", err)
		return fn()
	}

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(os.Stderr, reader)
		close(done)
	}()

	os.Stdout = writer
	defer func() {
		os.Stdout = original
		_ = writer.Close()
		<-done
		_ = reader.Close()
	}()

	return fn()
}
