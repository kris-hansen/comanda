package cmd

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/processor"
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
		config.DebugLog("Auth failed: malformed Authorization header: %s", authHeader)
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

// hasStdinInput checks if the first step in the YAML uses STDIN as input
func hasStdinInput(yamlContent []byte) bool {
	config.VerboseLog("Checking YAML for STDIN input requirement")

	// Parse YAML into the same structure used by handleProcess
	var rawConfig map[string]processor.StepConfig
	if err := yaml.Unmarshal(yamlContent, &rawConfig); err != nil {
		config.DebugLog("YAML parse error: %v", err)
		return false
	}

	// Find the first step alphabetically
	var firstStepName string
	var firstStepConfig processor.StepConfig
	for name, config := range rawConfig {
		if firstStepName == "" || name < firstStepName {
			firstStepName = name
			firstStepConfig = config
		}
	}

	if firstStepName == "" {
		config.DebugLog("No steps found in YAML content")
		return false
	}

	config.DebugLog("First step name: %s", firstStepName)

	// Handle input field based on its type
	switch input := firstStepConfig.Input.(type) {
	case string:
		isStdin := strings.EqualFold(input, "STDIN")
		config.DebugLog("Found string input: %s, isStdin=%v", input, isStdin)
		return isStdin
	case []interface{}:
		if len(input) > 0 {
			if str, ok := input[0].(string); ok {
				isStdin := strings.EqualFold(str, "STDIN")
				config.DebugLog("Found array input: %v, isStdin=%v", input, isStdin)
				return isStdin
			}
		}
	case map[string]interface{}:
		config.DebugLog("Found map input: %v", input)
		return false
	default:
		config.DebugLog("Input field has unexpected type: %T", input)
	}

	return false
}
