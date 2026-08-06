package retry

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

func TestIsTransient(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"rate limit", errors.New("HTTP 429 rate limit"), true},
		{"overloaded", errors.New("API Error: overloaded"), true},
		{"stalled stream", errors.New("API Error: Response stalled mid-stream"), true},
		{"broken pipe", errors.New("write: broken pipe"), true},
		{"deadline", fmt.Errorf("request failed: %w", context.DeadlineExceeded), true},
		{"server error", errors.New("status code: 503"), true},
		{"bad request", errors.New("status code: 400 invalid model"), false},
		{"authentication", errors.New("authentication failed"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsTransient(tt.err); got != tt.want {
				t.Fatalf("IsTransient(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
