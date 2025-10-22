# COMANDA Logging Standards

## Overview
This document defines the logging standards and guidelines for the COMANDA project to ensure consistent, maintainable, and debuggable code.

## Logging Principles

### 1. Use Standard Library Logging
- **ALWAYS** use `log.Printf()`, `log.Println()`, etc. instead of `fmt.Printf()` for **ALL** output (debug, user-facing, error messages)
- This provides consistency and allows for easier redirection of log outputs
- **NO EXCEPTIONS**: All console output should go through the logging system for unified control

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

### Correct Log Configuration with Resource Management
```go
// Global variable for log file cleanup
var logFile *os.File

// Configure logging for verbose mode
if verbose {
    log.SetFlags(0) // Remove timestamps for cleaner debug output
    
    // Optional: Configure log file output with proper resource management
    if logFileName := os.Getenv("COMANDA_LOG_FILE"); logFileName != "" {
        if file, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666); err == nil {
            logFile = file // Store for cleanup
            log.SetOutput(file)
            log.Printf("[INFO] Logging session started at %s\n", time.Now().Format(time.RFC3339))
            
            // CRITICAL: Ensure proper cleanup on application exit
            defer func() {
                if logFile != nil {
                    log.Printf("[INFO] Logging session ended at %s\n", time.Now().Format(time.RFC3339))
                    logFile.Sync() // Flush all buffered data
                    logFile.Close()
                }
            }()
        } else {
            // Fallback: warn user but continue with stdout logging
            log.Printf("[WARN] Failed to open log file '%s': %v. Continuing with stdout logging.\n", logFileName, err)
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

// WRONG: Missing log file cleanup
if logFile != "" {
    file, _ := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
    log.SetOutput(file)
    // File never closed - resource leak!
}

// WRONG: Using fmt.Printf for user messages
fmt.Printf("Response written to file: %s\n", outputPath) // Should use log.Printf
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
- **ALWAYS ensure log files are properly closed** when application exits
- Use `defer` statements to guarantee file cleanup and flushing

### 8. User-Facing vs Debug Messages
- **Use `log.Printf` for ALL output messages** to maintain consistency
- Avoid mixing `fmt.Printf` and `log.Printf` for different message types
- User-facing messages should go through the same logging system as debug messages
- This enables consistent redirection and formatting across all output types

### 9. Resource Management Requirements
- **MANDATORY**: All log file handles must be properly closed when application exits
- Use global variables to store file handles for proper lifecycle management
- **ALWAYS** call `file.Sync()` before `file.Close()` to ensure data is flushed
- Use `defer` statements in initialization functions to guarantee cleanup
- Log session start and end times for audit trails

## Enforcement and Verification

### Code Review Checklist
- [ ] **NO `fmt.Printf` statements** anywhere in the codebase (except in `.rules/logging.md` examples)
- [ ] All debug functions use **thread-safe mutex protection** in concurrent contexts
- [ ] Log file operations include **proper error handling and fallbacks**
- [ ] File handles are **stored globally and properly closed** with defer statements
- [ ] All log messages include **appropriate context** (model names, file paths, etc.)
- [ ] **Newline characters** (`\n`) are consistently used in `log.Printf` format strings
- [ ] **All model providers** use consistent `log.Printf` instead of `fmt.Printf`

### Automated Verification
```bash
# Verify no fmt.Printf debug statements remain
grep -r "fmt\.Printf.*\[DEBUG" . --exclude-dir=.git --exclude="*.md" || echo "✅ No fmt.Printf debug statements found"

# Verify all model providers use log.Printf
grep -r "log\.Printf.*\[DEBUG" utils/models/ || echo "❌ Model providers not using log.Printf"

# Run tests to ensure functionality
make test || echo "❌ Tests failing after logging changes"
```

### Standards Compliance
- **Critical**: All model providers must use `log.Printf` instead of `fmt.Printf`
- **Mandatory**: Thread safety must be implemented for all concurrent debug logging
- **Required**: Resource cleanup must be guaranteed for all file operations

## Related Components
- `utils/processor/dsl.go` - Primary debug logging implementation
- `cmd/root.go` - Log configuration and initialization
- Any component using concurrent/parallel processing - Must ensure thread safety