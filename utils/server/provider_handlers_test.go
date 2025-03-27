package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
)

func TestHandleDeleteProvider(t *testing.T) {
	// Create test configuration
	testConfig := &config.EnvConfig{
		Providers: map[string]*config.Provider{
			"openai": {
				APIKey: "test-key",
				Models: []config.Model{
					{Name: "gpt-4", Modes: []config.ModelMode{config.TextMode}},
				},
			},
			"anthropic": {
				APIKey: "test-key",
				Models: []config.Model{
					{Name: "claude-2", Modes: []config.ModelMode{config.TextMode}},
				},
			},
		},
	}

	// Create server instance
	server := &Server{
		mux: http.NewServeMux(),
		config: &config.ServerConfig{
			BearerToken: "test-token",
			Enabled:     true,
		},
		envConfig: testConfig,
	}

	// Register routes
	server.routes()

	// Test cases
	tests := []struct {
		name           string
		providerName   string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "Delete existing provider",
			providerName:   "openai",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Delete nonexistent provider",
			providerName:   "nonexistent",
			expectedStatus: http.StatusOK, // Deleting nonexistent provider is not an error
		},
		{
			name:           "Empty provider name",
			providerName:   "",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Provider name is required in the path", // Updated expected error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request
			req := httptest.NewRequest("DELETE", "/providers/"+tt.providerName, nil)
			req.Header.Set("Authorization", "Bearer test-token")
			rec := httptest.NewRecorder()

			// Handle request using the server's mux
			server.mux.ServeHTTP(rec, req)

			// Check status code
			if rec.Code != tt.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatus, rec.Code)
			}

			// Parse response based on expected status
			if tt.expectedStatus >= 400 {
				var response ErrorResponse
				if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode error response: %v. Body: %s", err, rec.Body.String())
				}
				if response.Error != tt.expectedError {
					t.Errorf("Expected error '%s', got '%s'", tt.expectedError, response.Error)
				}
				if response.Success {
					t.Error("Expected success to be false in error response")
				}
			} else {
				var response SuccessResponse
				if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode success response: %v. Body: %s", err, rec.Body.String())
				}
				// For successful deletion
				expectedMessage := "Provider " + tt.providerName + " removed successfully"
				if response.Message != expectedMessage {
					t.Errorf("Expected message '%s', got '%s'", expectedMessage, response.Message)
				}
				if !response.Success {
					t.Error("Expected success to be true in success response") // This check was correct
				}
				// Verify provider was actually removed
				if tt.providerName != "" && testConfig.Providers[tt.providerName] != nil {
					t.Error("Provider was not removed from configuration")
				}
			}
		})
	}

	// Test unauthorized access
	t.Run("Unauthorized access", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/providers/openai", nil)
		rec := httptest.NewRecorder()

		server.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rec.Code)
		}
	})

	// Test invalid token
	t.Run("Invalid token", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/providers/openai", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		rec := httptest.NewRecorder()

		server.mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rec.Code)
		}
	})
}

func TestProviderRouteHandling(t *testing.T) {
	// Create test configuration
	testConfig := &config.EnvConfig{
		Providers: map[string]*config.Provider{
			"openai": {
				APIKey: "test-key",
				Models: []config.Model{
					{Name: "gpt-4", Modes: []config.ModelMode{config.TextMode}},
				},
			},
		},
	}

	// Create server instance
	server := &Server{
		mux: http.NewServeMux(),
		config: &config.ServerConfig{
			BearerToken: "test-token",
			Enabled:     true,
		},
		envConfig: testConfig,
	}

	// Register routes
	server.routes()

	// Test cases for provider route handling
	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "GET providers list",
			method:         "GET",
			path:           "/providers",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "DELETE provider with path parameter",
			method:         "DELETE",
			path:           "/providers/openai",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid method on root",
			method:         "PATCH",
			path:           "/providers",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:  "Method not allowed",
		},
		{
			name:           "Invalid method on provider path",
			method:         "GET",
			path:           "/providers/openai",
			expectedStatus: http.StatusMethodNotAllowed,
			expectedError:  "Method not allowed for this path", // Updated expected error
		},
		{
			name:           "Empty provider name",
			method:         "DELETE",
			path:           "/providers/",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Provider name is required in the path", // Updated expected error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Header.Set("Authorization", "Bearer test-token")
			rec := httptest.NewRecorder()

			// Handle request using the server's mux
			server.mux.ServeHTTP(rec, req)

			// Check status code
			if rec.Code != tt.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatus, rec.Code)
			} else if tt.expectedStatus >= 400 { // Check error responses for other tests
				var response ErrorResponse
				if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
					t.Fatalf("Failed to decode error response: %v. Body: %s", err, rec.Body.String())
				}
				if response.Error != tt.expectedError {
					t.Errorf("Expected error '%s', got '%s'", tt.expectedError, response.Error)
				}
				if response.Success {
					t.Error("Expected success to be false in error response")
				}
			}
			// No need to check success response body for these tests yet
		})
	}
}
