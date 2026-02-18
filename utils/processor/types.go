package processor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// resolvePathAtParseTime expands ~ and resolves relative paths to absolute
// This is called during YAML parsing to ensure paths are resolved relative to
// where comanda is invoked, not where claude-code might run later
func resolvePathAtParseTime(path string) string {
	// Expand ~ to home directory
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	} else if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			path = home
		}
	}

	// Resolve to absolute path (handles "." and relative paths)
	if absPath, err := filepath.Abs(path); err == nil {
		return absPath
	}
	return path
}

// AgenticLoopConfig represents the configuration for an agentic loop
type AgenticLoopConfig struct {
	MaxIterations  int      `yaml:"max_iterations"`          // Maximum iterations before stopping (default: 10)
	TimeoutSeconds int      `yaml:"timeout_seconds"`         // Total timeout in seconds (default: 0 = no timeout)
	ExitCondition  string   `yaml:"exit_condition"`          // Exit condition: llm_decides, pattern_match
	ExitPattern    string   `yaml:"exit_pattern"`            // Regex pattern for pattern_match exit condition
	ContextWindow  int      `yaml:"context_window"`          // Number of past iterations to include in context (default: 5)
	Steps          []Step   `yaml:"steps,omitempty"`         // Sub-steps to execute within each iteration
	AllowedPaths   []string `yaml:"allowed_paths,omitempty"` // Directories for agentic tool access
	Tools          []string `yaml:"tools,omitempty"`         // Optional tool whitelist (Read, Write, Edit, Bash, etc.)

	// State persistence & quality gates
	Name               string              `yaml:"name,omitempty"`                // Loop name (required for stateful loops)
	Stateful           bool                `yaml:"stateful,omitempty"`            // Enable state persistence
	CheckpointInterval int                 `yaml:"checkpoint_interval,omitempty"` // Save state every N iterations (default: 5)
	QualityGates       []QualityGateConfig `yaml:"quality_gates,omitempty"`       // Quality gates to run after each iteration

	// Multi-loop orchestration
	DependsOn   []string `yaml:"depends_on,omitempty"`   // Wait for these loops to complete
	InputState  string   `yaml:"input_state,omitempty"`  // Variable to read from dependent loop
	OutputState string   `yaml:"output_state,omitempty"` // Variable to export for dependent loops
}

// LoopContext holds runtime state for an agentic loop
type LoopContext struct {
	Iteration      int             // Current iteration number (1-based)
	PreviousOutput string          // Output from previous iteration
	History        []LoopIteration // History of all iterations
	StartTime      time.Time       // When the loop started
}

// LoopIteration represents a single iteration's state
type LoopIteration struct {
	Index     int       // Iteration index
	Output    string    // Output from this iteration
	Timestamp time.Time // When this iteration completed
}

// ChunkConfig represents the configuration for chunking a large file
type ChunkConfig struct {
	By        string `yaml:"by"`         // How to split the file: "lines", "bytes", or "tokens"
	Size      int    `yaml:"size"`       // Chunk size (e.g., 10000 lines)
	Overlap   int    `yaml:"overlap"`    // Lines/bytes to overlap between chunks for context
	MaxChunks int    `yaml:"max_chunks"` // Limit total chunks to prevent overload
}

// ToolListConfig allows specifying tool allowlist/denylist at the step level
type ToolListConfig struct {
	Allowlist []string `yaml:"allowlist"` // Commands explicitly allowed
	Denylist  []string `yaml:"denylist"`  // Commands explicitly denied (takes precedence)
	Timeout   int      `yaml:"timeout"`   // Timeout in seconds for tool execution
}

// StepConfig represents the configuration for a single step
type StepConfig struct {
	Type       string          `yaml:"type"`            // Step type (default is standard LLM step)
	Input      interface{}     `yaml:"input"`           // Can be string, map, or "tool: command"
	Model      interface{}     `yaml:"model"`           // Can be string or []string
	Action     interface{}     `yaml:"action"`          // Can be string or []string
	Output     interface{}     `yaml:"output"`          // Can be string, []string, or "tool: command" / "STDOUT|command"
	NextAction interface{}     `yaml:"next-action"`     // Can be string or []string
	BatchMode  string          `yaml:"batch_mode"`      // How to process multiple files: "combined" (default) or "individual"
	SkipErrors bool            `yaml:"skip_errors"`     // Whether to continue processing if some files fail
	Chunk      *ChunkConfig    `yaml:"chunk,omitempty"` // Configuration for chunking large files
	Memory     bool            `yaml:"memory"`          // Whether to include memory context in this step
	ToolConfig *ToolListConfig `yaml:"tool,omitempty"`  // Tool execution configuration for this step

	// OpenAI Responses API specific fields
	Instructions       string                   `yaml:"instructions"`         // System message
	Tools              []map[string]interface{} `yaml:"tools"`                // Tools configuration
	PreviousResponseID string                   `yaml:"previous_response_id"` // For conversation state
	MaxOutputTokens    int                      `yaml:"max_output_tokens"`    // Token limit
	Temperature        float64                  `yaml:"temperature"`          // Temperature setting
	TopP               float64                  `yaml:"top_p"`                // Top-p sampling
	Stream             bool                     `yaml:"stream"`               // Whether to stream the response
	ResponseFormat     map[string]interface{}   `yaml:"response_format"`      // Format specification (e.g., JSON)

	// Meta-processing fields
	Generate      *GenerateStepConfig  `yaml:"generate,omitempty"`       // Configuration for generating a workflow
	Process       *ProcessStepConfig   `yaml:"process,omitempty"`        // Configuration for processing a sub-workflow
	AgenticLoop   *AgenticLoopConfig   `yaml:"agentic_loop,omitempty"`   // Inline agentic loop configuration
	CodebaseIndex *CodebaseIndexConfig `yaml:"codebase_index,omitempty"` // Codebase indexing configuration
	QmdSearch     *QmdSearchConfig     `yaml:"qmd_search,omitempty"`     // qmd search configuration
}

// Step represents a named step in the DSL
type Step struct {
	Name   string
	Config StepConfig
}

// DSLConfig represents the structure of the DSL configuration
type DSLConfig struct {
	Steps         []Step
	ParallelSteps map[string][]Step             // Steps that can be executed in parallel
	Defer         map[string]StepConfig         `yaml:"defer,omitempty"`
	AgenticLoops  map[string]*AgenticLoopConfig // Block-style agentic loops (legacy)

	// Multi-loop orchestration
	Loops        map[string]*AgenticLoopConfig `yaml:"loops,omitempty"`         // Named loops for orchestration
	ExecuteLoops []string                      `yaml:"execute_loops,omitempty"` // Simple execution order
	Workflow     map[string]*WorkflowNode      `yaml:"workflow,omitempty"`      // Complex workflow definition
}

// StepDependency represents a dependency between steps
type StepDependency struct {
	Name      string
	DependsOn []string
}

// NormalizeOptions represents options for string slice normalization
type NormalizeOptions struct {
	AllowEmpty bool // Whether to allow empty strings in the result
}

// PerformanceMetrics tracks timing information for processing steps
type PerformanceMetrics struct {
	InputProcessingTime  int64 // Time in milliseconds to process inputs
	ModelProcessingTime  int64 // Time in milliseconds for model processing
	ActionProcessingTime int64 // Time in milliseconds for action processing
	OutputProcessingTime int64 // Time in milliseconds for output processing
	TotalProcessingTime  int64 // Total time in milliseconds for the step
}

// CodebaseIndexConfig represents the configuration for codebase-index step
type CodebaseIndexConfig struct {
	Root        string                      `yaml:"root"`                    // Repository path (defaults to current directory)
	Output      *CodebaseIndexOutputConfig  `yaml:"output,omitempty"`        // Output configuration
	Expose      *CodebaseIndexExposeConfig  `yaml:"expose,omitempty"`        // Variable/memory exposure configuration
	Adapters    map[string]*AdapterOverride `yaml:"adapters,omitempty"`      // Per-adapter overrides
	MaxOutputKB int                         `yaml:"max_output_kb,omitempty"` // Maximum output size in KB
	Qmd         *QmdIntegrationConfig       `yaml:"qmd,omitempty"`           // qmd integration configuration
}

// QmdIntegrationConfig configures qmd integration for codebase indexing
type QmdIntegrationConfig struct {
	Collection string `yaml:"collection"`        // Collection name to register with qmd
	Embed      bool   `yaml:"embed,omitempty"`   // Run qmd embed after registration
	Context    string `yaml:"context,omitempty"` // Context description for the collection
	Mask       string `yaml:"mask,omitempty"`    // File mask for indexing (default: index file)
}

// QmdSearchConfig configures qmd search step
type QmdSearchConfig struct {
	Query      string  `yaml:"query"`                // Search query (supports variable substitution)
	Collection string  `yaml:"collection,omitempty"` // Restrict to a specific collection
	Mode       string  `yaml:"mode,omitempty"`       // Search mode: search (BM25), vsearch (vector), query (hybrid)
	Limit      int     `yaml:"limit,omitempty"`      // Number of results (default: 5)
	MinScore   float64 `yaml:"min_score,omitempty"`  // Minimum score threshold (0.0-1.0)
	Format     string  `yaml:"format,omitempty"`     // Output format: text (default), json, files
	Full       bool    `yaml:"full,omitempty"`       // Return full document content
}

// CodebaseIndexOutputConfig configures index output
type CodebaseIndexOutputConfig struct {
	Path    string `yaml:"path,omitempty"`    // Custom output path
	Format  string `yaml:"format,omitempty"`  // Output format: summary, structured, full (default: structured)
	Store   string `yaml:"store,omitempty"`   // Where to store: repo, config, both
	Encrypt bool   `yaml:"encrypt,omitempty"` // Whether to encrypt the output
}

// CodebaseIndexExposeConfig configures how index is exposed
type CodebaseIndexExposeConfig struct {
	WorkflowVariable bool                       `yaml:"workflow_variable,omitempty"` // Expose as workflow variable
	Memory           *CodebaseIndexMemoryConfig `yaml:"memory,omitempty"`            // Memory integration
}

// CodebaseIndexMemoryConfig configures memory integration
type CodebaseIndexMemoryConfig struct {
	Enabled bool   `yaml:"enabled,omitempty"` // Enable memory integration
	Key     string `yaml:"key,omitempty"`     // Memory key name
}

// AdapterOverride allows customization of adapter behavior
type AdapterOverride struct {
	IgnoreDirs      []string `yaml:"ignore_dirs,omitempty"`
	IgnoreGlobs     []string `yaml:"ignore_globs,omitempty"`
	PriorityFiles   []string `yaml:"priority_files,omitempty"`
	ReplaceDefaults bool     `yaml:"replace_defaults,omitempty"`
}

// QualityGateConfig represents the configuration for a quality gate
type QualityGateConfig struct {
	Name    string       `yaml:"name"`              // Gate name
	Command string       `yaml:"command,omitempty"` // Shell command to execute
	Type    string       `yaml:"type,omitempty"`    // Built-in type: syntax, security, test
	OnFail  string       `yaml:"on_fail"`           // Action on failure: retry, skip, abort
	Timeout int          `yaml:"timeout,omitempty"` // Timeout in seconds
	Retry   *RetryConfig `yaml:"retry,omitempty"`   // Retry configuration
}

// RetryConfig configures retry behavior for quality gates
type RetryConfig struct {
	MaxAttempts  int    `yaml:"max_attempts"`  // Maximum retry attempts (default: 3)
	BackoffType  string `yaml:"backoff_type"`  // Backoff strategy: linear, exponential
	InitialDelay int    `yaml:"initial_delay"` // Initial delay in seconds
}

// QualityGateResult represents the result of running a quality gate
type QualityGateResult struct {
	GateName string                 `json:"gate_name"`
	Passed   bool                   `json:"passed"`
	Message  string                 `json:"message"`
	Details  map[string]interface{} `json:"details,omitempty"`
	Duration time.Duration          `json:"duration"`
	Attempts int                    `json:"attempts"` // Number of attempts made
}

// WorkflowNode represents a node in a multi-loop workflow
type WorkflowNode struct {
	Type      string `yaml:"type"`                // Node type: loop, step, parallel
	Loop      string `yaml:"loop,omitempty"`      // Loop name to execute
	Role      string `yaml:"role,omitempty"`      // Role: creator, checker, finalizer
	Validates string `yaml:"validates,omitempty"` // Loop name this node validates
	OnFail    string `yaml:"on_fail,omitempty"`   // Action on validation failure: rerun_creator, abort, manual
}

// LoopOutput represents the output of a completed loop
type LoopOutput struct {
	LoopName     string              `json:"loop_name"`
	Status       string              `json:"status"`        // completed, failed
	Result       string              `json:"result"`        // Final output
	Variables    map[string]string   `json:"variables"`     // Variables to pass to dependent loops
	QualityGates []QualityGateResult `json:"quality_gates"` // Quality gate results
	StartTime    time.Time           `json:"start_time"`
	EndTime      time.Time           `json:"end_time"`
}

// UnmarshalYAML implements custom unmarshaling for AgenticLoopConfig
// to support both map and list syntax for steps
func (c *AgenticLoopConfig) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("expected mapping node for AgenticLoopConfig")
	}

	// Find and extract the steps node for special handling
	var stepsNode *yaml.Node
	var stepsKeyIdx int = -1

	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		if keyNode.Value == "steps" {
			stepsNode = node.Content[i+1]
			stepsKeyIdx = i
			break
		}
	}

	// Create a copy of the node without the steps field for clean decoding
	// This avoids the decode error when steps contains map syntax
	cleanNode := &yaml.Node{
		Kind:    yaml.MappingNode,
		Tag:     node.Tag,
		Content: make([]*yaml.Node, 0, len(node.Content)),
	}
	for i := 0; i < len(node.Content); i += 2 {
		if i != stepsKeyIdx {
			cleanNode.Content = append(cleanNode.Content, node.Content[i], node.Content[i+1])
		}
	}

	// Decode non-steps fields using alias type to avoid recursion
	type Alias AgenticLoopConfig
	aux := (*Alias)(c)
	if err := cleanNode.Decode(aux); err != nil {
		return fmt.Errorf("failed to decode loop config: %w", err)
	}

	// Resolve allowed_paths to absolute paths at parse time
	// This ensures paths like "." and "~" are resolved relative to where comanda is run
	if len(c.AllowedPaths) > 0 {
		resolvedPaths := make([]string, len(c.AllowedPaths))
		for i, p := range c.AllowedPaths {
			resolvedPaths[i] = resolvePathAtParseTime(p)
		}
		c.AllowedPaths = resolvedPaths
	}

	// Now handle steps specially
	if stepsNode != nil {
		if stepsNode.Kind == yaml.MappingNode {
			// Map syntax: steps: { stepname: {config...} }
			for i := 0; i < len(stepsNode.Content); i += 2 {
				stepKeyNode := stepsNode.Content[i]
				stepValueNode := stepsNode.Content[i+1]
				stepName := stepKeyNode.Value

				var stepConfig StepConfig
				if err := stepValueNode.Decode(&stepConfig); err != nil {
					return fmt.Errorf("failed to decode step '%s': %w", stepName, err)
				}
				c.Steps = append(c.Steps, Step{Name: stepName, Config: stepConfig})
			}
		} else if stepsNode.Kind == yaml.SequenceNode {
			// List syntax: steps: [- stepname: {config...}]
			for _, itemNode := range stepsNode.Content {
				if itemNode.Kind != yaml.MappingNode {
					return fmt.Errorf("expected mapping in steps list")
				}
				// Each item should be a single-key map: stepname: config
				if len(itemNode.Content) >= 2 {
					stepName := itemNode.Content[0].Value
					var stepConfig StepConfig
					if err := itemNode.Content[1].Decode(&stepConfig); err != nil {
						return fmt.Errorf("failed to decode step '%s': %w", stepName, err)
					}
					c.Steps = append(c.Steps, Step{Name: stepName, Config: stepConfig})
				}
			}
		}
	}

	return nil
}
