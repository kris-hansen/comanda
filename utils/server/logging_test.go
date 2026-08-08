package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMaskAuthHeader(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"bearer token", "Bearer abc123secret", "Bearer ********"},
		{"bearer with trailing space only", "Bearer ", "********"},
		{"bare bearer, no space", "Bearer", "********"},
		{"shorter than prefix", "Bear", "********"},
		{"basic scheme", "Basic dXNlcjpwYXNz", "********"},
		{"empty", "", "********"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maskAuthHeader(tt.input); got != tt.want {
				t.Errorf("maskAuthHeader(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// Regression test: a short/malformed Authorization header must not panic
// the request logger (previously: slice bounds out of range [7:6]).
func TestLogRequestDoesNotPanicOnShortAuthHeader(t *testing.T) {
	handler := logRequest(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for _, header := range []string{"Bearer", "Bearer ", "Bear", "Basic xyz"} {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.Header.Set("Authorization", header)
		rec := httptest.NewRecorder()

		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("logRequest panicked for Authorization: %q: %v", header, r)
				}
			}()
			handler(rec, req)
		}()
	}
}
