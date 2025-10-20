package retry

import (
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/kris-hansen/comanda/utils/config"
)

// RetryConfig holds configuration for retry operations
type RetryConfig struct {
	MaxRetries  int           // Maximum number of retry attempts
	InitialWait time.Duration // Initial wait time before first retry
	MaxWait     time.Duration // Maximum wait time between retries
	Factor      float64       // Exponential backoff factor
}

// DefaultRetryConfig provides sensible defaults for retry operations
var DefaultRetryConfig = RetryConfig{
	MaxRetries:  5,
	InitialWait: 1 * time.Second,
	MaxWait:     60 * time.Second,
	Factor:      2.0,
}

// WithRetry executes the given function with retry logic
// It will retry the function if it returns an error that matches the shouldRetry function
func WithRetry(operation func() (interface{}, error), shouldRetry func(error) bool, config RetryConfig) (interface{}, error) {
	var result interface{}
	var err error
	var wait = config.InitialWait

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Execute the operation
		result, err = operation()

		// If no error or error doesn't match retry criteria, return immediately
		if err == nil || !shouldRetry(err) {
			return result, err
		}

		// If this was the last attempt, return the error
		if attempt == config.MaxRetries {
			return nil, fmt.Errorf("operation failed after %d retries: %w", config.MaxRetries, err)
		}

		// Calculate wait time for next retry with exponential backoff
		retryWait := time.Duration(math.Min(float64(wait), float64(config.MaxWait)))

		// Extract retry time from error message if available
		if retryTime := extractRetryTime(err.Error()); retryTime > 0 {
			retryWait = retryTime
		}

		// Log the retry attempt - detailed in debug mode, brief otherwise
		config.DebugLog("Received retryable error: %v. Retrying in %v (attempt %d/%d)",
			err, retryWait, attempt+1, config.MaxRetries)

		// Also print a brief message in non-debug mode
		log.Printf("Rate limit detected, retrying in %v (attempt %d/%d)...\n",
			retryWait, attempt+1, config.MaxRetries)

		// Wait before next retry
		time.Sleep(retryWait)

		// Increase wait time for next iteration
		wait = time.Duration(float64(wait) * config.Factor)
	}

	// This should never be reached due to the return in the loop
	return nil, fmt.Errorf("unexpected error in retry logic")
}

// Is429Error checks if the error is a rate limit (429) error
func Is429Error(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()
	return strings.Contains(errMsg, "429") ||
		strings.Contains(errMsg, "rate limit") ||
		strings.Contains(errMsg, "quota exceeded") ||
		strings.Contains(errMsg, "too many requests")
}

// extractRetryTime attempts to extract a retry time from an error message
// Returns 0 if no retry time could be extracted
func extractRetryTime(errMsg string) time.Duration {
	// Look for patterns like "retry in 18s" or "retry after 30 seconds"
	retryPatterns := []string{
		"retry in ",
		"retry after ",
		"try again in ",
		"try again after ",
	}

	for _, pattern := range retryPatterns {
		if idx := strings.Index(strings.ToLower(errMsg), pattern); idx >= 0 {
			// Extract the time part
			timeStr := errMsg[idx+len(pattern):]

			// Try to parse seconds
			var seconds int
			if _, err := fmt.Sscanf(timeStr, "%ds", &seconds); err == nil {
				return time.Duration(seconds) * time.Second
			}

			// Try to parse "X seconds"
			if _, err := fmt.Sscanf(timeStr, "%d seconds", &seconds); err == nil {
				return time.Duration(seconds) * time.Second
			}

			// Add more parsing patterns as needed
		}
	}

	return 0
}

// DebugLog logs debug information if verbose mode is enabled
func (c RetryConfig) DebugLog(format string, args ...interface{}) {
	config.DebugLog("[Retry] "+format, args...)
}

// Log prints a message regardless of debug mode
func (c RetryConfig) Log(format string, args ...interface{}) {
	log.Printf(format+"\n", args...)
}
