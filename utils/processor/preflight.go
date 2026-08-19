package processor

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kris-hansen/comanda/utils/database"
	"gopkg.in/yaml.v3"
)

// Preflight verifies that a workflow can be processed without executing any
// model actions, shell tools, quality gates, or workflow writes. It uses the
// same model, path, tool-policy, database-connection, and output-permission
// checks that processing relies on. Output permissions are proven with a
// temporary probe file in the nearest existing parent directory.
func (p *Processor) Preflight() error {
	if p.config == nil {
		return fmt.Errorf("workflow configuration is missing")
	}

	hasLoops := len(p.config.Loops) > 0
	hasSteps := len(p.config.Steps) > 0 || len(p.config.ParallelSteps) > 0 || len(p.config.AgenticLoops) > 0
	if !hasLoops && !hasSteps {
		return fmt.Errorf("no steps or loops defined in DSL configuration")
	}
	if err := p.preflightWorktrees(); err != nil {
		return err
	}

	for _, step := range p.config.Steps {
		if err := p.preflightStep(step); err != nil {
			return err
		}
	}
	for groupName, steps := range p.config.ParallelSteps {
		for _, step := range steps {
			if err := p.preflightStep(step); err != nil {
				return fmt.Errorf("parallel group %q: %w", groupName, err)
			}
		}
	}
	if len(p.config.Steps) > 0 || len(p.config.ParallelSteps) > 0 {
		if err := p.validateDependencies(); err != nil {
			return fmt.Errorf("dependency validation failed: %w", err)
		}
	}
	for name, loop := range p.config.AgenticLoops {
		if err := p.preflightLoop(name, loop); err != nil {
			return err
		}
	}
	for name, loop := range p.config.Loops {
		if err := p.preflightLoop(name, loop); err != nil {
			return err
		}
	}
	return nil
}

func (p *Processor) preflightStep(step Step) error {
	if err := p.validateStepConfig(step.Name, step.Config); err != nil {
		return fmt.Errorf("step %q: %w", step.Name, err)
	}

	if step.Config.Process != nil {
		return p.preflightWorkflowFile(step.Name, step.Config.Process.WorkflowFile)
	}
	if step.Config.Generate != nil {
		models := p.NormalizeStringSlice(step.Config.Generate.Model)
		if len(models) == 0 && p.envConfig != nil && p.envConfig.DefaultGenerationModel != "" {
			models = []string{p.envConfig.DefaultGenerationModel}
		}
		if len(models) == 0 {
			return fmt.Errorf("step %q: no generate model configured", step.Name)
		}
		if err := p.validateModel(models, []string{InputNA}); err != nil {
			return fmt.Errorf("step %q: generate model validation failed: %w", step.Name, err)
		}
		if err := p.configureProviders(); err != nil {
			return fmt.Errorf("step %q: provider configuration failed: %w", step.Name, err)
		}
		return p.preflightOutput(step.Name, step.Config.Generate.Output, step.Config.ToolConfig)
	}
	if step.Config.Type == "codebase-index" || step.Config.CodebaseIndex != nil {
		return p.preflightCodebaseIndex(step)
	}
	if step.Config.Type == "qmd-search" || step.Config.QmdSearch != nil {
		if step.Config.QmdSearch == nil || strings.TrimSpace(step.Config.QmdSearch.Query) == "" {
			return fmt.Errorf("step %q: qmd_search.query is required", step.Name)
		}
		if _, err := exec.LookPath("qmd"); err != nil {
			return fmt.Errorf("step %q: qmd not found in PATH: %w", step.Name, err)
		}
		return nil
	}
	if step.Config.AgenticLoop != nil {
		return p.preflightLoop(step.Name, step.Config.AgenticLoop)
	}
	if step.Config.Type == "openai-responses" || step.Config.Skill != "" {
		return nil
	}
	if inputMap, ok := step.Config.Input.(map[string]interface{}); ok {
		if dbName, hasDatabase := inputMap["database"].(string); hasDatabase {
			sql, _ := inputMap["sql"].(string)
			h := database.NewHandler(p.envConfig)
			defer h.Close()
			if err := h.ValidateOperation(sql, database.ReadOperation); err != nil {
				return fmt.Errorf("step %q database input: %w", step.Name, err)
			}
			if err := h.TestConnection(dbName); err != nil {
				return fmt.Errorf("step %q database input: %w", step.Name, err)
			}
		}
		if rawURL, hasURL := inputMap["url"].(string); hasURL {
			if err := preflightURL(rawURL); err != nil {
				return fmt.Errorf("step %q scrape URL %q: %w", step.Name, rawURL, err)
			}
		}
	}

	inputs := p.NormalizeStringSlice(step.Config.Input)
	p.substituteCLIVariablesInSlice(inputs)
	if err := p.preflightInputs(step.Name, inputs, step.Config.ToolConfig); err != nil {
		return err
	}

	models := p.NormalizeStringSlice(step.Config.Model)
	if err := p.validateModel(models, inputs); err != nil {
		return fmt.Errorf("step %q: model validation failed: %w", step.Name, err)
	}
	if len(models) != 1 || models[0] != "NA" {
		if err := p.configureProviders(); err != nil {
			return fmt.Errorf("step %q: provider configuration failed: %w", step.Name, err)
		}
	}
	return p.preflightOutput(step.Name, step.Config.Output, step.Config.ToolConfig)
}

func (p *Processor) preflightLoop(name string, loop *AgenticLoopConfig) error {
	if loop == nil {
		return fmt.Errorf("loop %q: configuration is missing", name)
	}
	if loop.Stateful && loop.Name == "" {
		return fmt.Errorf("loop %q: stateful loops require a name", name)
	}
	if len(loop.AllowedPaths) > 0 && p.envConfig != nil && !p.envConfig.IsAgenticToolsAllowed() {
		return fmt.Errorf("loop %q: agentic tool use is disabled in global config", name)
	}
	for _, path := range loop.AllowedPaths {
		if err := preflightReadableDirectory(path); err != nil {
			return fmt.Errorf("loop %q allowed_path %q: %w", name, path, err)
		}
	}
	for _, step := range loop.Steps {
		if err := p.preflightStep(step); err != nil {
			return fmt.Errorf("loop %q: %w", name, err)
		}
	}
	return nil
}

func (p *Processor) preflightWorkflowFile(stepName, path string) error {
	if path == "" {
		return fmt.Errorf("step %q: process.workflow_file is required", stepName)
	}
	resolved, err := p.preflightResolvePath(path)
	if err != nil {
		return fmt.Errorf("step %q: resolve sub-workflow: %w", stepName, err)
	}
	if err := preflightReadableFile(resolved); err != nil {
		return fmt.Errorf("step %q: sub-workflow %q: %w", stepName, resolved, err)
	}
	contents, err := os.ReadFile(resolved)
	if err != nil {
		return fmt.Errorf("step %q: read sub-workflow %q: %w", stepName, resolved, err)
	}
	structure := ValidateWorkflowStructure(string(contents))
	if !structure.Valid {
		return fmt.Errorf("step %q: sub-workflow %q is invalid: %s", stepName, resolved, structure.ErrorSummary())
	}
	var child DSLConfig
	if err := yaml.Unmarshal(contents, &child); err != nil {
		return fmt.Errorf("step %q: parse sub-workflow %q: %w", stepName, resolved, err)
	}
	childProcessor := NewProcessor(&child, p.envConfig, p.serverConfig, p.verbose, p.runtimeDir, p.cliVariables)
	childProcessor.SetWorkflowFile(resolved)
	return childProcessor.Preflight()
}

func (p *Processor) preflightCodebaseIndex(step Step) error {
	ci := step.Config.CodebaseIndex
	if ci == nil {
		ci = &CodebaseIndexConfig{}
	}
	if ci.Use != nil {
		return nil // Registry loading is validated when the index is read during processing.
	}
	root := ci.Root
	if root == "" {
		root = "."
	}
	resolved, err := p.preflightResolvePath(root)
	if err != nil {
		return fmt.Errorf("step %q: resolve codebase root: %w", step.Name, err)
	}
	if err := preflightReadableDirectory(resolved); err != nil {
		return fmt.Errorf("step %q: codebase root %q: %w", step.Name, resolved, err)
	}
	if ci.Output != nil && ci.Output.Path != "" {
		return p.preflightOutput(step.Name, ci.Output.Path, step.Config.ToolConfig)
	}
	return nil
}

func (p *Processor) preflightInputs(stepName string, inputs []string, toolConfig *ToolListConfig) error {
	for _, inputPath := range inputs {
		inputPath = strings.TrimSpace(inputPath)
		if inputPath == "" || inputPath == InputNA || inputPath == InputSTDIN || inputPath == "screenshot" {
			continue
		}
		if IsToolInput(inputPath) {
			if err := p.preflightTool(inputPath, toolConfig, true); err != nil {
				return fmt.Errorf("step %q input: %w", stepName, err)
			}
			continue
		}
		if p.isURL(inputPath) {
			if err := preflightURL(inputPath); err != nil {
				return fmt.Errorf("step %q input URL %q: %w", stepName, inputPath, err)
			}
			continue
		}
		if strings.ContainsAny(inputPath, "*?[") {
			resolved, err := p.preflightResolvePath(inputPath)
			if err != nil {
				return fmt.Errorf("step %q input %q: %w", stepName, inputPath, err)
			}
			matches, err := filepath.Glob(resolved)
			if err != nil || len(matches) == 0 {
				return fmt.Errorf("step %q input glob %q did not match any files", stepName, inputPath)
			}
			for _, match := range matches {
				if err := p.validator.ValidatePath(match); err != nil {
					return fmt.Errorf("step %q input %q: %w", stepName, match, err)
				}
				if err := p.validator.ValidateFileExtension(match); err != nil {
					return fmt.Errorf("step %q input %q: %w", stepName, match, err)
				}
				if err := preflightReadableFile(match); err != nil {
					return fmt.Errorf("step %q input %q: %w", stepName, match, err)
				}
			}
			continue
		}
		resolved, err := p.preflightResolvePath(inputPath)
		if err != nil {
			return fmt.Errorf("step %q input %q: %w", stepName, inputPath, err)
		}
		if _, err := os.Stat(resolved); os.IsNotExist(err) && p.isOutputInOtherSteps(inputPath) {
			continue
		}
		if err := p.validator.ValidatePath(resolved); err != nil {
			return fmt.Errorf("step %q input %q: %w", stepName, resolved, err)
		}
		if err := p.validator.ValidateFileExtension(resolved); err != nil {
			return fmt.Errorf("step %q input %q: %w", stepName, resolved, err)
		}
		if err := preflightReadableFile(resolved); err != nil {
			return fmt.Errorf("step %q input %q: %w", stepName, resolved, err)
		}
	}
	return nil
}

func (p *Processor) preflightOutput(stepName string, output interface{}, toolConfig *ToolListConfig) error {
	if outputMap, ok := output.(map[string]interface{}); ok {
		if dbName, ok := outputMap["database"].(string); ok {
			sql, _ := outputMap["sql"].(string)
			h := database.NewHandler(p.envConfig)
			defer h.Close()
			if err := h.ValidateOperation(sql, database.WriteOperation); err != nil {
				return fmt.Errorf("step %q database output: %w", stepName, err)
			}
			if err := h.TestConnection(dbName); err != nil {
				return fmt.Errorf("step %q database output: %w", stepName, err)
			}
			return nil
		}
	}
	for _, value := range p.NormalizeStringSlice(output) {
		value = p.SubstituteCLIVariables(value)
		if value == "" || value == OutputSTDOUT || strings.HasPrefix(value, "MEMORY") {
			continue
		}
		if IsToolOutput(value) {
			if err := p.preflightTool(value, toolConfig, false); err != nil {
				return fmt.Errorf("step %q output: %w", stepName, err)
			}
			continue
		}
		path, err := p.preflightResolveOutputPath(value)
		if err != nil {
			return fmt.Errorf("step %q output %q: %w", stepName, value, err)
		}
		if err := preflightWritablePath(path); err != nil {
			return fmt.Errorf("step %q output %q: %w", stepName, path, err)
		}
	}
	return nil
}

func (p *Processor) preflightTool(value string, toolConfig *ToolListConfig, input bool) error {
	var command string
	var err error
	if input {
		command, _, err = ParseToolInput(value)
	} else {
		command, _, err = ParseToolOutput(value)
	}
	if err != nil {
		return err
	}
	stepConfig := &ToolConfig{}
	if toolConfig != nil {
		stepConfig.Allowlist = toolConfig.Allowlist
		stepConfig.Denylist = toolConfig.Denylist
		stepConfig.Timeout = toolConfig.Timeout
	}
	executor := NewToolExecutor(MergeToolConfigs(p.getGlobalToolConfig(), stepConfig), p.verbose, p.debugf)
	if allowed, reason := executor.IsAllowed(command); !allowed {
		return fmt.Errorf("tool execution denied: %s", reason)
	}
	if _, err := exec.LookPath(executor.extractBaseCommand(command)); err != nil {
		return fmt.Errorf("tool command is not available: %w", err)
	}
	return nil
}

func (p *Processor) preflightResolvePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	if p.serverConfig != nil && p.serverConfig.Enabled {
		if p.sourceRoot != "" && !p.isOutputInOtherSteps(path) {
			projectPath, err := ResolveProjectPath(p.sourceRoot, path)
			if err != nil {
				return "", err
			}
			if _, err := os.Stat(projectPath); err == nil {
				return projectPath, nil
			} else if !os.IsNotExist(err) {
				return "", err
			}
		}
		if p.runtimeDir != "" {
			return filepath.Join(p.serverConfig.DataDir, p.runtimeDir, path), nil
		}
		return filepath.Join(p.serverConfig.DataDir, path), nil
	}
	if p.sourceRoot != "" {
		return ResolveProjectPath(p.sourceRoot, path)
	}
	if p.runtimeDir != "" {
		return filepath.Join(p.runtimeDir, path), nil
	}
	return path, nil
}

func (p *Processor) preflightResolveOutputPath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return path, nil
	}
	if p.serverConfig != nil && p.serverConfig.Enabled {
		return filepath.Join(p.serverConfig.DataDir, p.runtimeDir, path), nil
	}
	if p.runtimeDir != "" {
		return filepath.Join(p.runtimeDir, path), nil
	}
	return path, nil
}

func (p *Processor) preflightWorktrees() error {
	if p.config.Worktrees == nil || len(p.config.Worktrees.Trees) == 0 {
		return nil
	}
	repo := p.config.Worktrees.Repo
	if repo == "" {
		repo = "."
	}
	if err := preflightReadableDirectory(repo); err != nil {
		return fmt.Errorf("worktree repository %q: %w", repo, err)
	}
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		return fmt.Errorf("worktree repository %q is not a Git repository", repo)
	}
	seen := make(map[string]struct{})
	for _, tree := range p.config.Worktrees.Trees {
		if tree.Name == "" {
			return fmt.Errorf("worktree name is required")
		}
		if _, exists := seen[tree.Name]; exists {
			return fmt.Errorf("duplicate worktree name %q", tree.Name)
		}
		seen[tree.Name] = struct{}{}
	}
	return nil
}

func preflightReadableFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("is a directory, not a file")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	return f.Close()
}

func preflightReadableDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("is not a directory")
	}
	_, err = os.ReadDir(path)
	return err
}

func preflightWritablePath(path string) error {
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		f, openErr := os.OpenFile(path, os.O_WRONLY, 0)
		if openErr != nil {
			return openErr
		}
		return f.Close()
	}
	parent := filepath.Dir(path)
	for {
		if _, err := os.Stat(parent); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return err
		}
		next := filepath.Dir(parent)
		if next == parent {
			return fmt.Errorf("no existing parent directory")
		}
		parent = next
	}
	probe, err := os.CreateTemp(parent, ".comanda-validate-*")
	if err != nil {
		return err
	}
	name := probe.Name()
	if closeErr := probe.Close(); closeErr != nil {
		_ = os.Remove(name)
		return closeErr
	}
	return os.Remove(name)
}

func preflightURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("invalid URL")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(raw)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status code %d", resp.StatusCode)
	}
	return nil
}
