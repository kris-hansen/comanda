package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/kris-hansen/comanda/utils/config"
	"gopkg.in/yaml.v3"
)

func checkAuth(serverConfig *config.ServerConfig, w http.ResponseWriter, r *http.Request) bool {
	if !serverConfig.Enabled {
		config.VerboseLog("Authentication disabled")
		config.DebugLog("Auth check skipped: server auth is disabled")
		return true
	}

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		config.VerboseLog("Missing Authorization header")
		config.DebugLog("Auth failed: no Authorization header present in request")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "Authorization header required",
		})
		return false
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		config.VerboseLog("Invalid authorization header format")
		config.DebugLog("Auth failed: malformed Authorization header: %s", maskToken(authHeader))
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "Invalid authorization header format",
		})
		return false
	}

	if parts[1] != serverConfig.BearerToken {
		config.VerboseLog("Invalid bearer token")
		config.DebugLog("Auth failed: invalid bearer token provided")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   "Invalid bearer token",
		})
		return false
	}

	config.VerboseLog("Authentication successful")
	config.DebugLog("Auth successful: valid bearer token")
	return true
}

// containsStdin checks if a string contains STDIN, handling variable assignments
func containsStdin(input string) bool {
	// Split on "as $" to handle variable assignments
	parts := strings.Split(input, " as $")
	// Check the first part (before any variable assignment)
	return strings.EqualFold(strings.TrimSpace(parts[0]), "STDIN")
}

// hasStdinInput checks if any step in the YAML uses STDIN as input
func hasStdinInput(yamlContent []byte) bool {
	config.VerboseLog("Checking YAML for STDIN input requirement")

	// First try parsing with a Node to preserve comments and formatting
	var node yaml.Node
	if err := yaml.Unmarshal(yamlContent, &node); err != nil {
		config.DebugLog("YAML node parse error: %v", err)
		// If node parsing fails, try direct string parsing
		return hasStdinInputFallback(string(yamlContent))
	}

	// Parse YAML into a map to preserve step order
	var rawConfig map[string]interface{}
	if err := yaml.Unmarshal(yamlContent, &rawConfig); err != nil {
		config.DebugLog("YAML map parse error: %v", err)
		// If map parsing fails, try direct string parsing
		return hasStdinInputFallback(string(yamlContent))
	}

	// Check each step's input field
	for stepName, stepConfig := range rawConfig {
		config.DebugLog("Checking step: %s", stepName)

		// Convert step to map
		stepMap, ok := stepConfig.(map[string]interface{})
		if !ok {
			continue
		}

		// Get input field
		input, exists := stepMap["input"]
		if !exists {
			continue
		}

		// Check input based on type
		switch v := input.(type) {
		case string:
			if containsStdin(v) {
				config.DebugLog("Found STDIN input in string format")
				return true
			}
		case []interface{}:
			// Check all array elements for STDIN
			for _, item := range v {
				if str, ok := item.(string); ok && containsStdin(str) {
					config.DebugLog("Found STDIN input in array format")
					return true
				}
			}
		case map[string]interface{}:
			// Handle map type inputs (like database configs)
			config.DebugLog("Found map input type")
		}
	}

	config.DebugLog("No STDIN input found in YAML")
	return false
}

// hasStdinInputFallback is a more lenient fallback parser that looks for STDIN in the raw content
func hasStdinInputFallback(content string) bool {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		// Remove comments
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = line[:idx]
		}

		// Look for input: STDIN pattern
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "input:") {
			value := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(line), "input:"))
			if strings.HasPrefix(value, "stdin") {
				config.DebugLog("Found STDIN input using fallback parser")
				return true
			}
		}
	}
	return false
}
