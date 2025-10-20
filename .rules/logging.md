# COMANDA Logging Standards

## Overview
This document defines the logging standards and guidelines for the COMANDA project to ensure consistent, maintainable, and debuggable code.

## Logging Principles

### 1. Use Standard Library Logging
- **ALWAYS** use `log.Printf()`, `log.Println()`, etc. instead of `fmt.Printf()` for any logging output
- This provides consistency and allows for easier redirection of log outputs
- Exception: User-facing output that is not debug/logging should use `fmt.Printf()`

### 2. Debug Logging Guidelines
- All debug logging must be thread-safe when used in concurrent contexts
- Use mutex protection for debug functions that may be called from goroutines
- Debug messages should be prefixed with appropriate context tags (e.g., `[DEBUG][DSL]`)

### 3. Log Levels and Formatting
- **Debug logs**: Use `[DEBUG][COMPONENT]` prefix format
- **Info logs**: Use `[INFO]` prefix for general information
- **Error logs**: Use `[ERROR]` prefix for error conditions
- **Warning logs**: Use `[WARN]` prefix for warning conditions

### 4. Log Output Configuration
- When verbose mode is enabled, configure log formatting for cleaner output
- Use `log.SetFlags(0)` to remove timestamps for debug output unless timestamps are specifically needed
- Consider file-based logging for debugging sessions to preserve logs after execution

### 5. Thread Safety Requirements
- Any logging function that may be called from multiple goroutines MUST be thread-safe
- Use `sync.Mutex` to protect shared logging state
- Always use `defer` for mutex unlocking to ensure cleanup even if panics occur

## Implementation Examples

### Correct Debug Logging
```go
// Thread-safe debug function
func (p *Processor) debugf(format string, args ...interface{}) {
    if p.verbose {
        p.mu.Lock()
        defer p.mu.Unlock()
        log.Printf("[DEBUG][DSL] "+format+"\n", args...)
    }
}
```

### Correct Log Configuration
```go
// Configure logging for verbose mode
if verbose {
    log.SetFlags(0) // Remove timestamps for cleaner debug output
    // Optional: Configure log file output for persistent debugging
}
```

### Incorrect Examples (DO NOT USE)
```go
// WRONG: Using fmt.Printf for logging
fmt.Printf("[DEBUG] Something happened\n")

// WRONG: Non-thread-safe debug logging in concurrent code
func debugf(format string, args ...interface{}) {
    fmt.Printf("[DEBUG] "+format+"\n", args...) // Race condition risk
}
```

## File-Based Logging (Implementation)
File-based logging is available for debugging sessions:
- Set the `COMANDA_LOG_FILE` environment variable to enable file logging
- Example: `export COMANDA_LOG_FILE=".logs/debug-$(date +%Y%m%d-%H%M%S).log"`
- Log files preserve debugging information after session ends
- Session start times are automatically logged when file logging is enabled

## Enforcement
- All logging code should follow these standards
- Code reviews should verify compliance with these guidelines
- Automated linting should check for `fmt.Printf` usage in debug contexts

## Related Components
- `utils/processor/dsl.go` - Primary debug logging implementation
- `cmd/root.go` - Log configuration and initialization
- Any component using concurrent/parallel processing - Must ensure thread safety