package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleListModels(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "comanda-openai-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test workflow files
	testWorkflows := []struct {
		path    string
		content string
	}{
		{
			path: "simple.yaml",
			content: `step:
  input: STDIN
  model: gpt-4o
  action: "Test"
  output: STDOUT`,
		},
		{
			path: "nested/workflow.yaml",
			content: `step:
  input: STDIN
  model: gpt-4o
  action: "Nested test"
  output: STDOUT`,
		},
		{
			path: "another.yml",
			content: `step:
  input: STDIN
  model: gpt-4o
  action: "YML extension"
  output: STDOUT`,
		},
		{
			path:    "notaworkflow.txt",
			content: "This is not a workflow",
		},
	}

	for _, wf := range testWorkflows {
		fullPath := filepath.Join(tempDir, wf.path)
		err := os.MkdirAll(filepath.Dir(fullPath), 0755)
		require.NoError(t, err)
		err = os.WriteFile(fullPath, []byte(wf.content), 0644)
		require.NoError(t, err)
	}

	// Create server instance with OpenAI compat enabled
	server := &Server{
		config: &config.ServerConfig{
			DataDir:     tempDir,
			BearerToken: "test-token",
			Enabled:     true,
			OpenAICompat: config.OpenAICompatConfig{
				Enabled: true,
				Prefix:  "/v1",
			},
		},
		envConfig: &config.EnvConfig{},
	}

	tests := []struct {
		name           string
		method         string
		expectedStatus int
		checkModels    bool
		expectedModels []string
	}{
		{
			name:           "GET request returns models list",
			method:         http.MethodGet,
			expectedStatus: http.StatusOK,
			checkModels:    true,
			expectedModels: []string{"simple", "nested/workflow", "another"},
		},
		{
			name:           "POST method not allowed",
			method:         http.MethodPost,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:           "PUT method not allowed",
			method:         http.MethodPut,
			expectedStatus: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/v1/models", nil)
			req.Header.Set("Authorization", "Bearer test-token")

			w := httptest.NewRecorder()
			server.handleListModels(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.checkModels {
				var response ModelListResponse
				err := json.NewDecoder(w.Body).Decode(&response)
				require.NoError(t, err)

				assert.Equal(t, "list", response.Object)
				assert.Len(t, response.Data, len(tt.expectedModels))

				// Check that all expected models are present
				modelIDs := make([]string, len(response.Data))
				for i, model := range response.Data {
					modelIDs[i] = model.ID
					assert.Equal(t, "model", model.Object)
					assert.Equal(t, "comanda", model.OwnedBy)
				}

				for _, expected := range tt.expectedModels {
					assert.Contains(t, modelIDs, expected)
				}
			}
		})
	}
}

func TestHandleChatCompletions(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "comanda-openai-chat-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a test workflow
	workflowContent := `step:
  input: STDIN
  model: gpt-4o
  action: "Process this input"
  output: STDOUT`

	err = os.WriteFile(filepath.Join(tempDir, "test-workflow.yaml"), []byte(workflowContent), 0644)
	require.NoError(t, err)

	// Create server instance
	server := &Server{
		config: &config.ServerConfig{
			DataDir:     tempDir,
			BearerToken: "test-token",
			Enabled:     true,
			OpenAICompat: config.OpenAICompatConfig{
				Enabled: true,
				Prefix:  "/v1",
			},
		},
		envConfig: &config.EnvConfig{
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
		},
	}

	tests := []struct {
		name           string
		method         string
		request        *ChatCompletionRequest
		expectedStatus int
		expectedError  string
	}{
		{
			name:   "Valid non-streaming request",
			method: http.MethodPost,
			request: &ChatCompletionRequest{
				Model: "test-workflow",
				Messages: []ChatMessage{
					{Role: "user", Content: "Hello, world!"},
				},
				Stream: false,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "GET method not allowed",
			method:         http.MethodGet,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name:   "Missing model",
			method: http.MethodPost,
			request: &ChatCompletionRequest{
				Messages: []ChatMessage{
					{Role: "user", Content: "Hello"},
				},
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "model is required",
		},
		{
			name:   "Missing messages",
			method: http.MethodPost,
			request: &ChatCompletionRequest{
				Model: "test-workflow",
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "messages is required",
		},
		{
			name:   "Workflow not found",
			method: http.MethodPost,
			request: &ChatCompletionRequest{
				Model: "nonexistent-workflow",
				Messages: []ChatMessage{
					{Role: "user", Content: "Hello"},
				},
			},
			expectedStatus: http.StatusNotFound,
			expectedError:  "workflow not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body []byte
			if tt.request != nil {
				body, _ = json.Marshal(tt.request)
			}

			req := httptest.NewRequest(tt.method, "/v1/chat/completions", bytes.NewBuffer(body))
			req.Header.Set("Authorization", "Bearer test-token")
			req.Header.Set("Content-Type", "application/json")

			w := httptest.NewRecorder()
			server.handleChatCompletions(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedError != "" {
				var errResp OpenAIError
				err := json.NewDecoder(w.Body).Decode(&errResp)
				require.NoError(t, err)
				assert.Contains(t, errResp.Error.Message, tt.expectedError)
			} else if tt.expectedStatus == http.StatusOK {
				var response ChatCompletionResponse
				err := json.NewDecoder(w.Body).Decode(&response)
				require.NoError(t, err)

				assert.NotEmpty(t, response.ID)
				assert.Equal(t, "chat.completion", response.Object)
				assert.Equal(t, tt.request.Model, response.Model)
				assert.Len(t, response.Choices, 1)
				assert.Equal(t, "assistant", response.Choices[0].Message.Role)
				assert.NotNil(t, response.Choices[0].FinishReason)
				assert.Equal(t, "stop", *response.Choices[0].FinishReason)
			}
		})
	}
}

func TestExtractInputFromMessages(t *testing.T) {
	tests := []struct {
		name     string
		messages []ChatMessage
		expected string
	}{
		{
			name: "Single user message",
			messages: []ChatMessage{
				{Role: "user", Content: "Hello"},
			},
			expected: "Hello",
		},
		{
			name: "Multiple messages - last user message",
			messages: []ChatMessage{
				{Role: "system", Content: "You are helpful"},
				{Role: "user", Content: "First question"},
				{Role: "assistant", Content: "First answer"},
				{Role: "user", Content: "Second question"},
			},
			expected: "Second question",
		},
		{
			name: "System message only",
			messages: []ChatMessage{
				{Role: "system", Content: "You are helpful"},
			},
			expected: "",
		},
		{
			name:     "Empty messages",
			messages: []ChatMessage{},
			expected: "",
		},
		{
			name: "Assistant message last",
			messages: []ChatMessage{
				{Role: "user", Content: "Question"},
				{Role: "assistant", Content: "Answer"},
			},
			expected: "Question",
		},
		{
			name: "Array content format (multi-modal)",
			messages: []ChatMessage{
				{Role: "user", Content: []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Hello from array",
					},
				}},
			},
			expected: "Hello from array",
		},
		{
			name: "Multiple text parts in array",
			messages: []ChatMessage{
				{Role: "user", Content: []interface{}{
					map[string]interface{}{
						"type": "text",
						"text": "Part 1",
					},
					map[string]interface{}{
						"type": "text",
						"text": "Part 2",
					},
				}},
			},
			expected: "Part 1\nPart 2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractInputFromMessages(tt.messages)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildMemoryContext(t *testing.T) {
	tests := []struct {
		name     string
		messages []ChatMessage
		contains []string
	}{
		{
			name: "Full conversation",
			messages: []ChatMessage{
				{Role: "system", Content: "You are helpful"},
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there"},
			},
			contains: []string{
				"System: You are helpful",
				"User: Hello",
				"Assistant: Hi there",
			},
		},
		{
			name:     "Empty messages",
			messages: []ChatMessage{},
			contains: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildMemoryContext(tt.messages)

			for _, expected := range tt.contains {
				assert.Contains(t, result, expected)
			}

			if len(tt.messages) == 0 {
				assert.Empty(t, result)
			}
		})
	}
}

func TestResolveWorkflowPath(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "comanda-resolve-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test files
	err = os.WriteFile(filepath.Join(tempDir, "workflow.yaml"), []byte("test"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tempDir, "workflow.yml"), []byte("test"), 0644)
	require.NoError(t, err)
	err = os.MkdirAll(filepath.Join(tempDir, "nested"), 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tempDir, "nested", "deep.yaml"), []byte("test"), 0644)
	require.NoError(t, err)

	server := &Server{
		config: &config.ServerConfig{
			DataDir: tempDir,
		},
	}

	tests := []struct {
		name          string
		modelName     string
		expectError   bool
		expectedError string
	}{
		{
			name:      "Simple workflow name",
			modelName: "workflow",
		},
		{
			name:      "Workflow with .yaml extension",
			modelName: "workflow.yaml",
		},
		{
			name:      "Nested workflow",
			modelName: "nested/deep",
		},
		{
			name:          "Nonexistent workflow",
			modelName:     "nonexistent",
			expectError:   true,
			expectedError: "workflow not found",
		},
		{
			name:          "Path traversal attempt",
			modelName:     "../etc/passwd",
			expectError:   true,
			expectedError: "path escape",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := server.resolveWorkflowPath(tt.modelName)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.True(t, strings.HasPrefix(path, tempDir))
			}
		})
	}
}

func TestGenerateCompletionID(t *testing.T) {
	id1 := generateCompletionID()

	assert.True(t, strings.HasPrefix(id1, "chatcmpl-"))
	// Check that the ID contains a numeric timestamp after the prefix
	assert.Greater(t, len(id1), len("chatcmpl-"))
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		text     string
		expected int
	}{
		{"", 0},
		{"test", 1},
		{"hello world", 2},
		{"This is a longer sentence with more words.", 10},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("text_len_%d", len(tt.text)), func(t *testing.T) {
			result := estimateTokens(tt.text)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestChatCompletionStreaming(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "comanda-openai-stream-test-*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create a test workflow
	workflowContent := `step:
  input: STDIN
  model: gpt-4o
  action: "Process this input"
  output: STDOUT`

	err = os.WriteFile(filepath.Join(tempDir, "test-workflow.yaml"), []byte(workflowContent), 0644)
	require.NoError(t, err)

	// Create server instance
	server := &Server{
		config: &config.ServerConfig{
			DataDir:     tempDir,
			BearerToken: "test-token",
			Enabled:     true,
			OpenAICompat: config.OpenAICompatConfig{
				Enabled: true,
				Prefix:  "/v1",
			},
		},
		envConfig: &config.EnvConfig{
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
		},
	}

	request := &ChatCompletionRequest{
		Model: "test-workflow",
		Messages: []ChatMessage{
			{Role: "user", Content: "Hello, world!"},
		},
		Stream: true,
	}

	body, _ := json.Marshal(request)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBuffer(body))
	req.Header.Set("Authorization", "Bearer test-token")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	w := httptest.NewRecorder()
	server.handleChatCompletions(w, req)

	// Check SSE headers
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
	assert.Equal(t, "keep-alive", w.Header().Get("Connection"))

	// Check that the response contains SSE data
	responseBody := w.Body.String()
	assert.Contains(t, responseBody, "data: ")
	assert.Contains(t, responseBody, "[DONE]")
}

func TestOpenAIErrorResponse(t *testing.T) {
	w := httptest.NewRecorder()
	sendOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "Test error message")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var errResp OpenAIError
	err := json.NewDecoder(w.Body).Decode(&errResp)
	require.NoError(t, err)

	assert.Equal(t, "Test error message", errResp.Error.Message)
	assert.Equal(t, "invalid_request_error", errResp.Error.Type)
}
