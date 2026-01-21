package codebaseindex

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Adapter defines the interface for language-specific indexing adapters
type Adapter interface {
	// Name returns the adapter identifier (e.g., "go", "python")
	Name() string

	// DetectionFiles returns files that indicate this language is used
	// (e.g., "go.mod", "requirements.txt")
	DetectionFiles() []string

	// FileExtensions returns file extensions this adapter handles
	// (e.g., ".go", ".py")
	FileExtensions() []string

	// IgnoreDirs returns directories to ignore for this language
	// (e.g., "vendor", "node_modules")
	IgnoreDirs() []string

	// IgnoreGlobs returns glob patterns to ignore
	// (e.g., "*.generated.go", "*.min.js")
	IgnoreGlobs() []string

	// EntrypointPatterns returns file patterns that indicate entrypoints
	// (e.g., "main.go", "index.ts")
	EntrypointPatterns() []string

	// ConfigPatterns returns file patterns that indicate config files
	// (e.g., "go.mod", "package.json")
	ConfigPatterns() []string

	// ExtractSymbols extracts symbols from file content
	// Only first maxBytes of content is provided for performance
	ExtractSymbols(path string, content []byte) (*SymbolInfo, error)

	// ScoreFile returns a score modifier for file prioritization
	// Higher scores = more important files
	ScoreFile(path string, depth int, isEntrypoint, isConfig bool) int
}

// Registry manages available adapters and language detection
type Registry struct {
	adapters map[string]Adapter
	mu       sync.RWMutex
}

// NewRegistry creates a new adapter registry with default adapters
func NewRegistry() *Registry {
	r := &Registry{
		adapters: make(map[string]Adapter),
	}
	// Register default adapters
	r.Register(&GoAdapter{})
	r.Register(&PythonAdapter{})
	r.Register(&TypeScriptAdapter{})
	r.Register(&FlutterAdapter{})
	return r
}

// Register adds an adapter to the registry
func (r *Registry) Register(a Adapter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.adapters[a.Name()] = a
}

// Get retrieves an adapter by name
func (r *Registry) Get(name string) (Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[name]
	return a, ok
}

// All returns all registered adapters
func (r *Registry) All() []Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Adapter, 0, len(r.adapters))
	for _, a := range r.adapters {
		result = append(result, a)
	}
	return result
}

// Detect identifies which adapters apply to the given repository
func (r *Registry) Detect(repoPath string) []Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var detected []Adapter
	for _, adapter := range r.adapters {
		if r.detectAdapter(repoPath, adapter) {
			detected = append(detected, adapter)
		}
	}
	return detected
}

// detectAdapter checks if a specific adapter applies to the repo
func (r *Registry) detectAdapter(repoPath string, adapter Adapter) bool {
	for _, detectionFile := range adapter.DetectionFiles() {
		// Check root level first
		if fileExists(filepath.Join(repoPath, detectionFile)) {
			return true
		}

		// Check all direct subdirectories (monorepo support)
		entries, err := os.ReadDir(repoPath)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
					if fileExists(filepath.Join(repoPath, entry.Name(), detectionFile)) {
						return true
					}
				}
			}
		}
	}
	return false
}

// GetByNames retrieves adapters by name, returning only those found
func (r *Registry) GetByNames(names []string) []Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Adapter
	for _, name := range names {
		if a, ok := r.adapters[name]; ok {
			result = append(result, a)
		}
	}
	return result
}

// CombinedIgnoreDirs returns all ignore dirs from the given adapters
func CombinedIgnoreDirs(adapters []Adapter) []string {
	seen := make(map[string]bool)
	var result []string
	for _, a := range adapters {
		for _, dir := range a.IgnoreDirs() {
			if !seen[dir] {
				seen[dir] = true
				result = append(result, dir)
			}
		}
	}
	return result
}

// CombinedIgnoreGlobs returns all ignore globs from the given adapters
func CombinedIgnoreGlobs(adapters []Adapter) []string {
	seen := make(map[string]bool)
	var result []string
	for _, a := range adapters {
		for _, glob := range a.IgnoreGlobs() {
			if !seen[glob] {
				seen[glob] = true
				result = append(result, glob)
			}
		}
	}
	return result
}

// CombinedExtensions returns all file extensions from the given adapters
func CombinedExtensions(adapters []Adapter) []string {
	seen := make(map[string]bool)
	var result []string
	for _, a := range adapters {
		for _, ext := range a.FileExtensions() {
			if !seen[ext] {
				seen[ext] = true
				result = append(result, ext)
			}
		}
	}
	return result
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Placeholder adapters - will be implemented in adapters/ subdirectory

// GoAdapter handles Go codebases
type GoAdapter struct{}

func (a *GoAdapter) Name() string             { return "go" }
func (a *GoAdapter) DetectionFiles() []string { return []string{"go.mod", "go.sum"} }
func (a *GoAdapter) FileExtensions() []string { return []string{".go"} }
func (a *GoAdapter) IgnoreDirs() []string     { return []string{"vendor", "testdata"} }
func (a *GoAdapter) IgnoreGlobs() []string {
	return []string{"*_test.go", "*.generated.go", "mock_*.go"}
}
func (a *GoAdapter) EntrypointPatterns() []string { return []string{"main.go", "cmd/*/main.go"} }
func (a *GoAdapter) ConfigPatterns() []string     { return []string{"go.mod", "go.sum", "Makefile"} }
func (a *GoAdapter) ScoreFile(path string, depth int, isEntrypoint, isConfig bool) int {
	score := 0
	if isEntrypoint {
		score += 40
	}
	if isConfig {
		score += 30
	}
	if depth <= 2 {
		score += 60
	}
	return score
}
func (a *GoAdapter) ExtractSymbols(path string, content []byte) (*SymbolInfo, error) {
	// Implemented in adapters/go.go
	return extractGoSymbols(path, content)
}

// PythonAdapter handles Python codebases
type PythonAdapter struct{}

func (a *PythonAdapter) Name() string { return "python" }
func (a *PythonAdapter) DetectionFiles() []string {
	return []string{"pyproject.toml", "requirements.txt", "setup.py", "Pipfile"}
}
func (a *PythonAdapter) FileExtensions() []string { return []string{".py"} }
func (a *PythonAdapter) IgnoreDirs() []string {
	return []string{"__pycache__", ".venv", "venv", ".tox", ".eggs", "*.egg-info"}
}
func (a *PythonAdapter) IgnoreGlobs() []string { return []string{"*.pyc", "*_pb2.py", "*_pb2_grpc.py"} }
func (a *PythonAdapter) EntrypointPatterns() []string {
	return []string{"main.py", "app.py", "__main__.py", "manage.py"}
}
func (a *PythonAdapter) ConfigPatterns() []string {
	return []string{"pyproject.toml", "setup.py", "setup.cfg", "requirements.txt"}
}
func (a *PythonAdapter) ScoreFile(path string, depth int, isEntrypoint, isConfig bool) int {
	score := 0
	if isEntrypoint {
		score += 40
	}
	if isConfig {
		score += 30
	}
	if depth <= 2 {
		score += 60
	}
	return score
}
func (a *PythonAdapter) ExtractSymbols(path string, content []byte) (*SymbolInfo, error) {
	return extractPythonSymbols(path, content)
}

// TypeScriptAdapter handles TypeScript/JavaScript codebases
type TypeScriptAdapter struct{}

func (a *TypeScriptAdapter) Name() string { return "typescript" }
func (a *TypeScriptAdapter) DetectionFiles() []string {
	return []string{"tsconfig.json", "package.json"}
}
func (a *TypeScriptAdapter) FileExtensions() []string { return []string{".ts", ".tsx", ".js", ".jsx"} }
func (a *TypeScriptAdapter) IgnoreDirs() []string {
	return []string{"node_modules", "dist", "build", ".next", "coverage"}
}
func (a *TypeScriptAdapter) IgnoreGlobs() []string {
	return []string{"*.min.js", "*.bundle.js", "*.d.ts", "*.map"}
}
func (a *TypeScriptAdapter) EntrypointPatterns() []string {
	return []string{"index.ts", "index.js", "main.ts", "app.ts", "server.ts"}
}
func (a *TypeScriptAdapter) ConfigPatterns() []string {
	return []string{"package.json", "tsconfig.json", "webpack.config.js", "vite.config.ts"}
}
func (a *TypeScriptAdapter) ScoreFile(path string, depth int, isEntrypoint, isConfig bool) int {
	score := 0
	if isEntrypoint {
		score += 40
	}
	if isConfig {
		score += 30
	}
	if depth <= 2 {
		score += 60
	}
	return score
}
func (a *TypeScriptAdapter) ExtractSymbols(path string, content []byte) (*SymbolInfo, error) {
	return extractTypeScriptSymbols(path, content)
}

// FlutterAdapter handles Flutter/Dart codebases
type FlutterAdapter struct{}

func (a *FlutterAdapter) Name() string                 { return "flutter" }
func (a *FlutterAdapter) DetectionFiles() []string     { return []string{"pubspec.yaml"} }
func (a *FlutterAdapter) FileExtensions() []string     { return []string{".dart"} }
func (a *FlutterAdapter) IgnoreDirs() []string         { return []string{".dart_tool", "build", ".pub-cache"} }
func (a *FlutterAdapter) IgnoreGlobs() []string        { return []string{"*.g.dart", "*.freezed.dart"} }
func (a *FlutterAdapter) EntrypointPatterns() []string { return []string{"main.dart", "lib/main.dart"} }
func (a *FlutterAdapter) ConfigPatterns() []string {
	return []string{"pubspec.yaml", "analysis_options.yaml"}
}
func (a *FlutterAdapter) ScoreFile(path string, depth int, isEntrypoint, isConfig bool) int {
	score := 0
	if isEntrypoint {
		score += 40
	}
	if isConfig {
		score += 30
	}
	if depth <= 2 {
		score += 60
	}
	return score
}
func (a *FlutterAdapter) ExtractSymbols(path string, content []byte) (*SymbolInfo, error) {
	return extractFlutterSymbols(path, content)
}
