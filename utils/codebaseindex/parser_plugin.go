package codebaseindex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const defaultParserPluginTimeout = 5 * time.Second

// ParserPluginRequest is sent to a parser plugin on standard input. Content is
// intentionally capped by the indexer's normal symbol-read limit.
type ParserPluginRequest struct {
	Version  int    `json:"version"`
	Root     string `json:"root"`
	Path     string `json:"path"`
	Content  string `json:"content"`
	MaxBytes int64  `json:"max_bytes"`
}

// ParserPluginResponse is the single JSON object a parser plugin must write to
// standard output. Error lets a plugin return a per-file diagnostic without
// relying on stderr parsing.
type ParserPluginResponse struct {
	Symbols *SymbolInfo `json:"symbols"`
	Error   string      `json:"error,omitempty"`
}

// LoadParserPluginManifests reads plugin declarations from local YAML files.
// The manifests contain only launch configuration; parser source code remains
// wherever the caller keeps it and is never copied into an index.
func LoadParserPluginManifests(paths []string) ([]ParserPluginConfig, error) {
	plugins := make([]ParserPluginConfig, 0, len(paths))
	for _, path := range paths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil, fmt.Errorf("resolve parser plugin manifest %q: %w", path, err)
		}
		data, err := os.ReadFile(absPath)
		if err != nil {
			return nil, fmt.Errorf("read parser plugin manifest %q: %w", absPath, err)
		}

		var plugin ParserPluginConfig
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(&plugin); err != nil {
			return nil, fmt.Errorf("parse parser plugin manifest %q: %w", absPath, err)
		}
		if err := validateParserPluginConfig(&plugin); err != nil {
			return nil, fmt.Errorf("invalid parser plugin manifest %q: %w", absPath, err)
		}
		plugins = append(plugins, plugin)
	}
	return plugins, nil
}

// ParserPluginAdapter adapts a local executable to the normal indexing adapter
// interface. Commands are always invoked directly, never via a shell.
type ParserPluginAdapter struct {
	config ParserPluginConfig
	root   string
}

// NewParserPluginAdapter validates and initializes one local parser plugin.
func NewParserPluginAdapter(config ParserPluginConfig, root string) (*ParserPluginAdapter, error) {
	if err := validateParserPluginConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid parser plugin %q: %w", config.Name, err)
	}
	if _, err := exec.LookPath(config.Command); err != nil {
		return nil, fmt.Errorf("parser plugin %q command %q is not executable: %w", config.Name, config.Command, err)
	}
	return &ParserPluginAdapter{config: config, root: root}, nil
}

func validateParserPluginConfig(config *ParserPluginConfig) error {
	config.Name = strings.TrimSpace(config.Name)
	config.Command = strings.TrimSpace(config.Command)
	if config.Name == "" {
		return fmt.Errorf("name is required")
	}
	if strings.ContainsAny(config.Name, `/\\`) {
		return fmt.Errorf("name must not contain path separators")
	}
	if config.Command == "" {
		return fmt.Errorf("command is required")
	}
	if len(config.Extensions) == 0 {
		return fmt.Errorf("at least one extension is required")
	}
	for i, extension := range config.Extensions {
		extension = strings.TrimSpace(extension)
		if extension == "" {
			return fmt.Errorf("extensions must not contain empty values")
		}
		if !strings.HasPrefix(extension, ".") {
			extension = "." + extension
		}
		config.Extensions[i] = strings.ToLower(extension)
	}
	if config.TimeoutMS < 0 {
		return fmt.Errorf("timeout_ms must not be negative")
	}
	return nil
}

func (a *ParserPluginAdapter) Name() string { return a.config.Name }
func (a *ParserPluginAdapter) DetectionFiles() []string {
	return append([]string(nil), a.config.DetectionFiles...)
}
func (a *ParserPluginAdapter) FileExtensions() []string {
	return append([]string(nil), a.config.Extensions...)
}
func (a *ParserPluginAdapter) IgnoreDirs() []string {
	return append([]string(nil), a.config.IgnoreDirs...)
}
func (a *ParserPluginAdapter) IgnoreGlobs() []string {
	return append([]string(nil), a.config.IgnoreGlobs...)
}
func (a *ParserPluginAdapter) EntrypointPatterns() []string {
	return append([]string(nil), a.config.EntrypointPatterns...)
}
func (a *ParserPluginAdapter) ConfigPatterns() []string {
	return append([]string(nil), a.config.ConfigPatterns...)
}
func (a *ParserPluginAdapter) DisableSymbolCache() bool { return true }
func (a *ParserPluginAdapter) FailOnExtractionError() bool {
	return true
}

func (a *ParserPluginAdapter) ScoreFile(_ string, _ int, isEntrypoint, isConfig bool) int {
	score := a.config.Priority
	if isEntrypoint {
		score += 40
	}
	if isConfig {
		score += 30
	}
	return score
}

// ExtractSymbols asks the plugin to extract a SymbolInfo value for one file.
func (a *ParserPluginAdapter) ExtractSymbols(path string, content []byte) (*SymbolInfo, error) {
	request, err := json.Marshal(ParserPluginRequest{
		Version:  1,
		Root:     a.root,
		Path:     path,
		Content:  string(content),
		MaxBytes: int64(len(content)),
	})
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	timeout := defaultParserPluginTimeout
	if a.config.TimeoutMS > 0 {
		timeout = time.Duration(a.config.TimeoutMS) * time.Millisecond
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, a.config.Command, a.config.Args...)
	cmd.Stdin = bytes.NewReader(request)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("parser plugin %q timed out after %s", a.Name(), timeout)
		}
		message := strings.TrimSpace(stderr.String())
		if message != "" {
			return nil, fmt.Errorf("parser plugin %q failed: %w: %s", a.Name(), err, message)
		}
		return nil, fmt.Errorf("parser plugin %q failed: %w", a.Name(), err)
	}

	var response ParserPluginResponse
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return nil, fmt.Errorf("parser plugin %q returned invalid JSON: %w", a.Name(), err)
	}
	if response.Error != "" {
		return nil, fmt.Errorf("parser plugin %q: %s", a.Name(), response.Error)
	}
	if response.Symbols == nil {
		return nil, fmt.Errorf("parser plugin %q response is missing symbols", a.Name())
	}
	return response.Symbols, nil
}
