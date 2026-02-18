package codebaseindex

import (
	"time"

	"github.com/kris-hansen/comanda/utils/filescan"
)

// HashAlgorithm specifies the hashing algorithm to use
type HashAlgorithm string

const (
	HashXXHash  HashAlgorithm = "xxhash"
	HashSHA256  HashAlgorithm = "sha256"
	DefaultHash HashAlgorithm = HashXXHash
)

// StoreLocation specifies where to store the index
type StoreLocation string

const (
	StoreRepo   StoreLocation = "repo"
	StoreConfig StoreLocation = "config"
	StoreBoth   StoreLocation = "both"
)

// OutputFormat specifies the format/verbosity of the generated index
type OutputFormat string

const (
	// FormatSummary generates a compact overview (1-2KB)
	// Contains: repo purpose, main areas, key entry points
	// Best for: quick context, agent system prompts
	FormatSummary OutputFormat = "summary"

	// FormatStructured generates a categorized index (10-50KB)
	// Contains: files grouped by domain/purpose, semantic sections
	// Best for: agentic loops that need to understand codebase areas
	FormatStructured OutputFormat = "structured"

	// FormatFull generates the complete index (50-200KB)
	// Contains: all files, symbols, detailed tree structure
	// Best for: comprehensive analysis, initial exploration
	FormatFull OutputFormat = "full"

	// DefaultFormat is the default output format
	DefaultFormat OutputFormat = FormatStructured
)

// Config represents the parsed configuration for codebase indexing
type Config struct {
	// Root path to scan (defaults to current directory)
	Root string

	// Output configuration
	OutputPath    string
	OutputFormat  OutputFormat // summary, structured, or full
	Store         StoreLocation
	Encrypt       bool
	EncryptionKey string

	// Expose configuration
	ExposeVariable bool
	MemoryEnabled  bool
	MemoryKey      string

	// Adapter overrides per language
	AdapterOverrides map[string]*AdapterOverride

	// Processing options
	MaxOutputKB   int
	HashAlgorithm HashAlgorithm
	Incremental   bool
	Verbose       bool

	// qmd integration (optional)
	Qmd *QmdConfig

	// Derived values (computed at runtime)
	RepoFileSlug string // lowercase slug for filenames
	RepoVarSlug  string // uppercase slug for variables
}

// AdapterOverride allows customization of adapter behavior
type AdapterOverride struct {
	IgnoreDirs      []string
	IgnoreGlobs     []string
	PriorityFiles   []string
	ReplaceDefaults bool
}

// Result represents the output of index generation
type Result struct {
	// The generated markdown content (primary format)
	Content string

	// Additional format outputs (generated on demand)
	Summary    string // Always generated - compact overview for agents
	Structured string // Categorized index if format != summary
	Full       string // Complete index if format == full

	// Path where the index was written
	OutputPath string

	// Hash of the plaintext content
	ContentHash string

	// Whether the index was updated (for incremental mode)
	Updated bool

	// Format used for Content
	Format OutputFormat

	// Metadata
	GeneratedAt time.Time
	RepoName    string
	Languages   []string
	FileCount   int
	Duration    time.Duration

	// Categorization (populated during structured/full generation)
	Categories map[string][]string // category -> file paths
}

// ScanResult holds the results of repository scanning
type ScanResult struct {
	// All files found (before candidate selection)
	Files []*FileEntry

	// Selected candidate files for indexing
	Candidates []*FileEntry

	// Directory structure summary
	DirTree *DirNode

	// Statistics
	TotalFiles    int
	TotalDirs     int
	IgnoredFiles  int
	IgnoredDirs   int
	TotalBytes    int64
	ProcessedTime time.Duration
}

// FileEntry represents a single file in the repository
type FileEntry struct {
	// Relative path from repo root
	Path string

	// File metadata
	Size    int64
	ModTime time.Time
	Hash    string // xxhash or sha256 depending on config

	// Token estimation (Size / 4 as rough approximation)
	EstimatedTokens int

	// Scoring
	Score        int
	Depth        int
	IsEntrypoint bool
	IsConfig     bool
	IsGenerated  bool

	// Language association
	Language string

	// Extracted symbols (populated during extraction phase)
	Symbols *SymbolInfo
}

// TokenBudgetCategory returns the token budget category for a file
// Uses shared thresholds from filescan package
func (f *FileEntry) TokenBudgetCategory() string {
	if f.EstimatedTokens < filescan.TokenThresholdSafe {
		return "safe"
	} else if f.EstimatedTokens < filescan.TokenThresholdLarge {
		return "large"
	}
	return "oversized"
}

// DirNode represents a directory in the tree structure
type DirNode struct {
	Name     string
	Path     string
	Children []*DirNode
	Files    []string // File names only (not full entries)
	Depth    int
}

// SymbolInfo holds extracted symbols from a file
type SymbolInfo struct {
	// Package/module declaration
	Package string

	// Imports/dependencies
	Imports []string

	// Functions and methods
	Functions []FunctionInfo

	// Types (structs, classes, interfaces)
	Types []TypeInfo

	// Constants and variables
	Constants []string
	Variables []string

	// Framework/library indicators
	Frameworks []string

	// Risk indicators (auth, crypto, db, concurrency)
	RiskTags []string
}

// FunctionInfo describes a function or method
type FunctionInfo struct {
	Name       string
	Signature  string
	IsExported bool
	IsMethod   bool
	Receiver   string // For methods
	Comments   string
}

// TypeInfo describes a type definition
type TypeInfo struct {
	Name       string
	Kind       string // struct, interface, class, enum, etc.
	IsExported bool
	Fields     []string
	Methods    []string
	Comments   string
}

// ChangeSet represents files that changed since last index
type ChangeSet struct {
	Added    []*FileEntry
	Modified []*FileEntry
	Deleted  []string
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Root:           ".",
		OutputFormat:   DefaultFormat,
		Store:          StoreRepo,
		Encrypt:        false,
		ExposeVariable: true,
		MemoryEnabled:  false,
		MaxOutputKB:    100,
		HashAlgorithm:  DefaultHash,
		Incremental:    false,
		Verbose:        false,
	}
}
