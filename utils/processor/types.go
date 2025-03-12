package processor

// StepConfig represents the configuration for a single step
type StepConfig struct {
	Input      interface{} `yaml:"input"`       // Can be string or map[string]interface{}
	Model      interface{} `yaml:"model"`       // Can be string or []string
	Action     interface{} `yaml:"action"`      // Can be string or []string
	Output     interface{} `yaml:"output"`      // Can be string or []string
	NextAction interface{} `yaml:"next-action"` // Can be string or []string
	BatchMode  string      `yaml:"batch_mode"`  // How to process multiple files: "combined" (default) or "individual"
	SkipErrors bool        `yaml:"skip_errors"` // Whether to continue processing if some files fail
}

// Step represents a named step in the DSL
type Step struct {
	Name   string
	Config StepConfig
}

// DSLConfig represents the structure of the DSL configuration
type DSLConfig struct {
	Steps         []Step
	ParallelSteps map[string][]Step // Steps that can be executed in parallel
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
