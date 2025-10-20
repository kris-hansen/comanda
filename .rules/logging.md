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
- **ALWAYS implement error handling fallbacks** for file-based logging operations
- **ALWAYS add newline characters** (`\n`) to `log.Printf` format strings for consistency
- Provide informative error messages when log file operations fail

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
    
    // Optional: Configure log file output with error handling
    if logFile := os.Getenv("COMANDA_LOG_FILE"); logFile != "" {
        if file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666); err == nil {
            log.SetOutput(file)
            log.Printf("[INFO] Logging session started at %s\n", time.Now().Format(time.RFC3339))
        } else {
            // Fallback: warn user but continue with stdout logging
            log.Printf("[WARN] Failed to open log file '%s': %v. Continuing with stdout logging.\n", logFile, err)
        }
    }
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

// WRONG: Missing error handling for file operations
if logFile := os.Getenv("LOG_FILE"); logFile != "" {
    file, _ := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
    log.SetOutput(file) // Silent failure if file can't be opened
}

// WRONG: Missing newline in log.Printf
log.Printf("[DEBUG] Message without newline", args...) // Inconsistent formatting
```

## File-Based Logging (Implementation)
File-based logging is available for debugging sessions:
- Set the `COMANDA_LOG_FILE` environment variable to enable file logging
- Example: `export COMANDA_LOG_FILE=".logs/debug-$(date +%Y%m%d-%H%M%S).log"`
- Log files preserve debugging information after session ends
- Session start times are automatically logged when file logging is enabled

### 6. Debug Message Context
- **ALWAYS include sufficient context** in debug messages for effective troubleshooting
- Include relevant identifiers (model names, file paths, step names) in debug logs
- Use consistent format: `[COMPONENT] context: message`
- Example: `p.debugf("[%s] Writing response to file: %s", modelName, outputPath)`

### 7. Error Handling in Logging Operations
- **NEVER let logging failures crash the application**
- Provide fallback mechanisms for file logging failures
- Log warnings when fallback mechanisms are activated
- Ensure application continues functioning even if logging fails

## Enforcement
- All logging code should follow these standards
- Code reviews should verify compliance with these guidelines
- Automated linting should check for `fmt.Printf` usage in debug contexts
- **Critical**: All model providers must use `log.Printf` instead of `fmt.Printf`

## Related Components
- `utils/processor/dsl.go` - Primary debug logging implementation
- `cmd/root.go` - Log configuration and initialization
- Any component using concurrent/parallel processing - Must ensure thread safety