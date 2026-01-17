package processor

import "time"

// AgenticLoopConfig represents the configuration for an agentic loop
type AgenticLoopConfig struct {
	MaxIterations  int    `yaml:"max_iterations"`  // Maximum iterations before stopping (default: 10)
	TimeoutSeconds int    `yaml:"timeout_seconds"` // Total timeout in seconds (default: 300)
	ExitCondition  string `yaml:"exit_condition"`  // Exit condition: llm_decides, pattern_match
	ExitPattern    string `yaml:"exit_pattern"`    // Regex pattern for pattern_match exit condition
	ContextWindow  int    `yaml:"context_window"`  // Number of past iterations to include in context (default: 5)
	Steps          []Step `yaml:"steps,omitempty"` // Sub-steps to execute within each iteration
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
	Generate    *GenerateStepConfig `yaml:"generate,omitempty"`     // Configuration for generating a workflow
	Process     *ProcessStepConfig  `yaml:"process,omitempty"`      // Configuration for processing a sub-workflow
	AgenticLoop *AgenticLoopConfig  `yaml:"agentic_loop,omitempty"` // Inline agentic loop configuration
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
	AgenticLoops  map[string]*AgenticLoopConfig // Block-style agentic loops
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
