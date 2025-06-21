package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/models"
	"github.com/kris-hansen/comanda/utils/processor"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

// MockProvider implements the models.Provider interface for testing
type MockProvider struct {
	name       string
	configured bool
	verbose    bool
	apiKey     string
}

func NewMockProvider(name string) *MockProvider {
	return &MockProvider{
		name: name,
	}
}

func (m *MockProvider) Name() string {
	return m.name
}

func (m *MockProvider) SupportsModel(modelName string) bool {
	return modelName == "gpt-4o"
}

func (m *MockProvider) Configure(apiKey string) error {
	if apiKey == "" {
		return fmt.Errorf("API key is required")
	}
	m.configured = true
	m.apiKey = apiKey
	return nil
}

func (m *MockProvider) SendPrompt(model, prompt string) (string, error) {
	if !m.configured {
		return "", fmt.Errorf("provider not configured")
	}
	if !m.SupportsModel(model) {
		return "", fmt.Errorf("unsupported model: %s", model)
	}
	// Mock successful text processing
	return fmt.Sprintf("mock response for prompt: %s", prompt), nil
}

func (m *MockProvider) SendPromptWithFile(model, prompt string, file models.FileInput) (string, error) {
	if !m.configured {
		return "", fmt.Errorf("provider not configured")
	}
	if !m.SupportsModel(model) {
		return "", fmt.Errorf("unsupported model: %s", model)
	}
	// Mock successful file processing
	return fmt.Sprintf("mock response for file: %s with prompt: %s", file.Path, prompt), nil
}

func (m *MockProvider) SetVerbose(verbose bool) {
	m.verbose = verbose
}

func init() {
	// Override the default provider detection for testing
	originalDetectProvider := models.DetectProvider
	models.DetectProvider = func(modelName string) models.Provider {
		if modelName == "gpt-4o" {
			provider := NewMockProvider("openai")
			provider.Configure("test-key")
			return provider
		}
		return originalDetectProvider(modelName)
	}
}

// sseRecorder is a custom ResponseRecorder that captures SSE events
type sseRecorder struct {
	*httptest.ResponseRecorder
	events []string
}

func newSSERecorder() *sseRecorder {
	return &sseRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		events:           make([]string, 0),
	}
}

func (r *sseRecorder) parseEvents() {
	body := r.Body.String()
	scanner := bufio.NewScanner(strings.NewReader(body))
	var currentEvent strings.Builder
	var currentEventType string

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			// Empty line marks end of event
			if currentEvent.Len() > 0 && currentEventType != "" {
				// For progress events, we want to capture the step message
				// For complete events, we want to capture the completion message
				// For error events, we want to capture the error message
				if currentEventType == "progress" || currentEventType == "complete" || currentEventType == "error" {
					data := strings.TrimSpace(currentEvent.String())
					// Handle both JSON and plain text formats
					if data == "Starting workflow processing" || data == "Workflow processing completed successfully" || strings.HasPrefix(data, "Error reading YAML file") || strings.HasPrefix(data, "Error parsing YAML") {
						r.events = append(r.events, data)
					} else {
						// Try to parse as JSON
						var jsonData map[string]interface{}
						if err := json.Unmarshal([]byte(data), &jsonData); err == nil {
							// If it's JSON and has an error field, use that
							if errMsg, ok := jsonData["error"].(string); ok {
								r.events = append(r.events, errMsg)
							} else if msg, ok := jsonData["message"].(string); ok {
								// For progress events, extract the message field
								if msg == "Starting workflow processing" || msg == "Workflow processing completed successfully" || strings.HasPrefix(msg, "Error reading YAML file") || strings.HasPrefix(msg, "Error parsing YAML") {
									r.events = append(r.events, msg)
								}
							}
						}
					}
				}
			}
			currentEvent.Reset()
			currentEventType = ""
		} else if strings.HasPrefix(line, "event: ") {
			currentEventType = strings.TrimSpace(strings.TrimPrefix(line, "event: "))
		} else if strings.HasPrefix(line, "data: ") && currentEventType != "" {
			data := strings.TrimPrefix(line, "data: ")
			currentEvent.WriteString(data)
		}
	}

	// Add final event if exists
	if currentEvent.Len() > 0 && currentEventType != "" {
		if currentEventType == "progress" || currentEventType == "complete" || currentEventType == "error" {
			data := strings.TrimSpace(currentEvent.String())
			// Handle both JSON and plain text formats
			if data == "Starting workflow processing" || data == "Workflow processing completed successfully" || strings.HasPrefix(data, "Error reading YAML file") || strings.HasPrefix(data, "Error parsing YAML") {
				r.events = append(r.events, data)
			} else {
				// Try to parse as JSON
				var jsonData map[string]interface{}
				if err := json.Unmarshal([]byte(data), &jsonData); err == nil {
					// If it's JSON and has an error field, use that
					if errMsg, ok := jsonData["error"].(string); ok {
						r.events = append(r.events, errMsg)
					} else if msg, ok := jsonData["message"].(string); ok {
						// For progress events, extract the message field
						if msg == "Starting workflow processing" || msg == "Workflow processing completed successfully" || strings.HasPrefix(msg, "Error reading YAML file") || strings.HasPrefix(msg, "Error parsing YAML") {
							r.events = append(r.events, msg)
						}
					}
				}
			}
		}
	}
}

func TestYAMLParsingParity(t *testing.T) {
	// Sample YAML that uses STDIN input (similar to stdin-example.yaml)
	yamlContent := []byte(`
analyze_text:
  input: STDIN
  model: gpt-4o
  action: "Analyze the following text and provide key insights:"
  output: STDOUT

summarize:
  input: STDIN
  model: gpt-4o-mini
  action: "Summarize the analysis in 3 bullet points:"
  output: STDOUT
`)

	// CLI-style parsing (from cmd/process.go)
	var cliRawConfig map[string]processor.StepConfig
	err := yaml.Unmarshal(yamlContent, &cliRawConfig)
	assert.NoError(t, err, "CLI parsing should not error")

	var cliConfig processor.DSLConfig
	for name, config := range cliRawConfig {
		cliConfig.Steps = append(cliConfig.Steps, processor.Step{
			Name:   name,
			Config: config,
		})
	}

	// Server-style parsing (from utils/server/handlers.go)
	var serverRawConfig map[string]processor.StepConfig
	err = yaml.Unmarshal(yamlContent, &serverRawConfig)
	assert.NoError(t, err, "Server parsing should not error")

	var serverConfig processor.DSLConfig
	for name, config := range serverRawConfig {
		serverConfig.Steps = append(serverConfig.Steps, processor.Step{
			Name:   name,
			Config: config,
		})
	}

	// Verify both methods produce identical results
	assert.Equal(t, len(cliConfig.Steps), len(serverConfig.Steps),
		"CLI and server should parse the same number of steps")

	// Create maps for easier comparison since order isn't guaranteed
	cliSteps := make(map[string]processor.Step)
	serverSteps := make(map[string]processor.Step)

	for _, step := range cliConfig.Steps {
		cliSteps[step.Name] = step
	}
	for _, step := range serverConfig.Steps {
		serverSteps[step.Name] = step
	}

	// Compare steps by name
	for name, cliStep := range cliSteps {
		serverStep, exists := serverSteps[name]
		assert.True(t, exists, "Step %s should exist in both configs", name)

		// Compare StepConfig fields
		assert.Equal(t, cliStep.Config.Input, serverStep.Config.Input,
			"Input should match for step %s", name)
		assert.Equal(t, cliStep.Config.Model, serverStep.Config.Model,
			"Model should match for step %s", name)
		assert.Equal(t, cliStep.Config.Action, serverStep.Config.Action,
			"Action should match for step %s", name)
		assert.Equal(t, cliStep.Config.Output, serverStep.Config.Output,
			"Output should match for step %s", name)
		assert.Equal(t, cliStep.Config.NextAction, serverStep.Config.NextAction,
			"NextAction should match for step %s", name)
	}

	// Create test server config
	testServerConfig := &config.ServerConfig{
		BearerToken: "test-token",
		Enabled:     true,
	}

	// Create test environment config with OpenAI provider
	testEnvConfig := &config.EnvConfig{
		Providers: map[string]*config.Provider{
			"openai": {
				APIKey: "test-key",
				Models: []config.Model{
					{
						Name:  "gpt-4o",
						Modes: []config.ModelMode{config.TextMode},
					},
				},
			},
		},
	}

	// Verify both configs can be processed
	cliProc := processor.NewProcessor(&cliConfig, testEnvConfig, testServerConfig, true, "")
	assert.NotNil(t, cliProc, "CLI processor should be created successfully")

	serverProc := processor.NewProcessor(&serverConfig, testEnvConfig, testServerConfig, true, "")
	assert.NotNil(t, serverProc, "Server processor should be created successfully")
}

// Test that direct DSLConfig parsing works for our YAML format
func TestDirectDSLConfigParsing(t *testing.T) {
	yamlContent := []byte(`
analyze_text:
  input: STDIN
  model: gpt-4o
  action: "Test action"
  output: STDOUT
`)

	// Try parsing directly into DSLConfig
	var dslConfig processor.DSLConfig
	err := yaml.Unmarshal(yamlContent, &dslConfig)

	// This should result in a DSLConfig with one step
	assert.NoError(t, err, "Parsing should not error")
	assert.Len(t, dslConfig.Steps, 1, "Direct parsing into DSLConfig should result in one step")
	assert.Equal(t, "analyze_text", dslConfig.Steps[0].Name, "Step name should be 'analyze_text'")
	assert.Equal(t, "STDIN", dslConfig.Steps[0].Config.Input, "Step input should be 'STDIN'")
	assert.Equal(t, "gpt-4o", dslConfig.Steps[0].Config.Model, "Step model should be 'gpt-4o'")
	assert.Equal(t, "Test action", dslConfig.Steps[0].Config.Action, "Step action should be 'Test action'")
	assert.Equal(t, "STDOUT", dslConfig.Steps[0].Config.Output, "Step output should be 'STDOUT'")
}

func TestHandleProcessStreaming(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "comanda-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create test YAML files
	testYAML := `
step_one:
  model: gpt-4o
  input: STDIN
  action: "Analyze this text"
  output: STDOUT
`
	stdinYAML := `
step_one:
  model: gpt-4o
  input: STDIN
  action: "Analyze this text"
  output: STDOUT
`

	if err := os.WriteFile(fmt.Sprintf("%s/test.yaml", tempDir), []byte(testYAML), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fmt.Sprintf("%s/stdin.yaml", tempDir), []byte(stdinYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Create test environment config with OpenAI provider
	envConfig := &config.EnvConfig{
		Providers: map[string]*config.Provider{
			"openai": {
				APIKey: "test-key",
				Models: []config.Model{
					{
						Name:  "gpt-4o",
						Modes: []config.ModelMode{config.TextMode},
					},
				},
			},
		},
	}

	// Create server instance
	server := &Server{
		config: &config.ServerConfig{
			DataDir:     tempDir,
			BearerToken: "test-token",
			Enabled:     true,
		},
		envConfig: envConfig,
	}

	tests := []struct {
		name           string
		filename       string
		input          string
		streaming      bool
		method         string
		expectedEvents []string
		expectedError  string
	}{
		{
			name:      "Valid POST request with streaming",
			filename:  "stdin.yaml",
			input:     "test input",
			streaming: true,
			method:    http.MethodPost,
			expectedEvents: []string{
				"Starting workflow processing",
				"Workflow processing completed successfully",
			},
		},
		{
			name:          "Error case - file not found",
			filename:      "nonexistent.yaml",
			streaming:     true,
			method:        http.MethodPost,
			input:         "test input",
			expectedError: "Error reading YAML file",
		},
		{
			name:          "Error case - GET method not allowed",
			filename:      "stdin.yaml",
			streaming:     true,
			method:        http.MethodGet,
			expectedError: "YAML processing is only available via POST requests. Please use POST with your YAML content.",
		},
		{
			name:          "Error case - PUT method not allowed",
			filename:      "stdin.yaml",
			streaming:     true,
			method:        http.MethodPut,
			expectedError: "YAML processing is only available via POST requests. Please use POST with your YAML content.",
		},
		{
			name:      "Valid POST request without input",
			filename:  "stdin.yaml",
			streaming: true,
			method:    http.MethodPost,
			expectedEvents: []string{
				"Starting workflow processing",
				"Workflow processing completed successfully",
			},
		},
		{
			name:          "Error case - DELETE method not allowed",
			filename:      "stdin.yaml",
			streaming:     true,
			method:        http.MethodDelete,
			expectedError: "YAML processing is only available via POST requests. Please use POST with your YAML content.",
		},
		{
			name:          "Error case - PATCH method not allowed",
			filename:      "stdin.yaml",
			streaming:     true,
			method:        http.MethodPatch,
			expectedError: "YAML processing is only available via POST requests. Please use POST with your YAML content.",
		},
		{
			name:          "Error case - HEAD method not allowed",
			filename:      "stdin.yaml",
			streaming:     true,
			method:        http.MethodHead,
			expectedError: "YAML processing is only available via POST requests. Please use POST with your YAML content.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			if tt.method == http.MethodPost {
				reqBody := struct {
					Input     string `json:"input"`
					Streaming bool   `json:"streaming"`
				}{
					Input:     tt.input,
					Streaming: tt.streaming,
				}
				body, _ = json.Marshal(reqBody)
			}

			// Create request
			url := fmt.Sprintf("/process?filename=%s&streaming=true", tt.filename)
			req := httptest.NewRequest(tt.method, url, bytes.NewBuffer(body))
			req.Header.Set("Authorization", "Bearer test-token")
			req.Header.Set("Accept", "text/event-stream")
			if tt.method == http.MethodPost {
				req.Header.Set("Content-Type", "application/json")
			}

			// Use custom recorder for streaming tests
			w := newSSERecorder()

			// Call handler
			handleProcess(w, req, server.config, server.envConfig)

			if tt.streaming {
				// Parse and verify events first (before checking headers)
				w.parseEvents()

				// Verify SSE headers
				assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
				assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
				assert.Equal(t, "keep-alive", w.Header().Get("Connection"))

				if tt.expectedError != "" {
					if len(w.events) > 0 {
						assert.Contains(t, w.events[len(w.events)-1], tt.expectedError)
					} else {
						// If no events, check the response body for error
						var response ProcessResponse
						err := json.NewDecoder(w.Body).Decode(&response)
						assert.NoError(t, err)
						assert.Contains(t, response.Error, tt.expectedError)
					}
				} else {
					assert.Equal(t, len(tt.expectedEvents), len(w.events))
					for i, expectedEvent := range tt.expectedEvents {
						assert.Contains(t, w.events[i], expectedEvent)
					}
				}
			} else {
				// Verify regular response
				var response ProcessResponse
				err := json.NewDecoder(w.Body).Decode(&response)
				assert.NoError(t, err)

				if tt.expectedError != "" {
					assert.False(t, response.Success)
					assert.Contains(t, response.Error, tt.expectedError)
				} else {
					assert.True(t, response.Success)
				}
			}
		})
	}
}

func TestHandleYAMLProcessStreaming(t *testing.T) {
	// Create test environment config with OpenAI provider
	envConfig := &config.EnvConfig{
		Providers: map[string]*config.Provider{
			"openai": {
				APIKey: "test-key",
				Models: []config.Model{
					{
						Name:  "gpt-4o",
						Modes: []config.ModelMode{config.TextMode},
					},
				},
			},
		},
	}

	// Create server instance
	server := &Server{
		config: &config.ServerConfig{
			BearerToken: "test-token",
			Enabled:     true,
		},
		envConfig: envConfig,
	}

	tests := []struct {
		name           string
		yamlContent    string
		streaming      bool
		input          string
		expectedEvents []string
		expectedError  string
	}{
		{
			name: "Simple streaming process with input",
			yamlContent: `
step_one:
  model: gpt-4o
  input: STDIN
  action: "Analyze this text"
  output: STDOUT`,
			streaming: true,
			input:     "test input",
			expectedEvents: []string{
				"Starting workflow processing",
				"Workflow processing completed successfully",
			},
		},
		{
			name: "Simple streaming process without input",
			yamlContent: `
step_one:
  model: gpt-4o
  input: STDIN
  action: "Analyze this text"
  output: STDOUT`,
			streaming: true,
			input:     "",
			expectedEvents: []string{
				"Starting workflow processing",
				"Workflow processing completed successfully",
			},
		},
		{
			name:          "Error case - invalid YAML",
			yamlContent:   `invalid: yaml: content`,
			streaming:     true,
			expectedError: "Error parsing YAML",
		},
		{
			name: "Non-streaming process",
			yamlContent: `
step_one:
  model: gpt-4o
  input: STDIN
  action: "Analyze this text"
  output: STDOUT`,
			streaming: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create request body
			reqBody := struct {
				Content   string `json:"content"`
				Streaming bool   `json:"streaming"`
				Input     string `json:"input"`
			}{
				Content:   tt.yamlContent,
				Streaming: tt.streaming,
				Input:     tt.input,
			}
			body, _ := json.Marshal(reqBody)

			// Create request
			req := httptest.NewRequest(http.MethodPost, "/yaml/process", bytes.NewBuffer(body))
			req.Header.Set("Authorization", "Bearer test-token")
			req.Header.Set("Content-Type", "application/json")
			if tt.streaming {
				req.Header.Set("Accept", "text/event-stream")
			}

			// Use custom recorder for streaming tests
			w := newSSERecorder()

			// Call handler
			server.handleYAMLProcess(w, req)

			if tt.streaming {
				// Parse and verify events first (before checking headers)
				w.parseEvents()

				// Verify SSE headers
				assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
				assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
				assert.Equal(t, "keep-alive", w.Header().Get("Connection"))

				if tt.expectedError != "" {
					if len(w.events) > 0 {
						assert.Contains(t, w.events[len(w.events)-1], tt.expectedError)
					} else {
						// If no events, check the response body for error
						var response ProcessResponse
						err := json.NewDecoder(w.Body).Decode(&response)
						assert.NoError(t, err)
						assert.Contains(t, response.Error, tt.expectedError)
					}
				} else {
					assert.Equal(t, len(tt.expectedEvents), len(w.events))
					for i, expectedEvent := range tt.expectedEvents {
						assert.Contains(t, w.events[i], expectedEvent)
					}
				}
			} else {
				// Verify regular response
				var response ProcessResponse
				err := json.NewDecoder(w.Body).Decode(&response)
				assert.NoError(t, err)

				if tt.expectedError != "" {
					assert.False(t, response.Success)
					assert.Contains(t, response.Error, tt.expectedError)
				} else {
					assert.True(t, response.Success)
				}
			}
		})
	}
}
