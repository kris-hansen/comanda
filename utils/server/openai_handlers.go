package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/kris-hansen/comanda/utils/fileutil"
	"github.com/kris-hansen/comanda/utils/processor"
	"gopkg.in/yaml.v3"
)

// generateCompletionID generates a unique completion ID
func generateCompletionID() string {
	return fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
}

// estimateTokens provides a rough token estimate (4 chars per token average)
func estimateTokens(text string) int {
	return len(text) / 4
}

// sendOpenAIError sends an error response in OpenAI format
func sendOpenAIError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(OpenAIError{
		Error: OpenAIErrorDetail{
			Message: message,
			Type:    errType,
		},
	})
}

// extractInputFromMessages extracts the last user message as primary input
func extractInputFromMessages(messages []ChatMessage) string {
	// Find the last user message
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}
	return ""
}

// buildMemoryContext builds a memory context string from the message history
func buildMemoryContext(messages []ChatMessage) string {
	var parts []string

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			parts = append(parts, fmt.Sprintf("System: %s", msg.Content))
		case "user":
			parts = append(parts, fmt.Sprintf("User: %s", msg.Content))
		case "assistant":
			parts = append(parts, fmt.Sprintf("Assistant: %s", msg.Content))
		}
	}

	return strings.Join(parts, "\n\n")
}

// resolveWorkflowPath converts model name to workflow file path
func (s *Server) resolveWorkflowPath(modelName string) (string, error) {
	// Add .yaml extension if not present
	if !strings.HasSuffix(modelName, ".yaml") && !strings.HasSuffix(modelName, ".yml") {
		modelName = modelName + ".yaml"
	}

	// Build full path within DataDir
	fullPath := filepath.Join(s.config.DataDir, modelName)

	// Validate path is within DataDir (security check)
	absDataDir, err := filepath.Abs(s.config.DataDir)
	if err != nil {
		return "", fmt.Errorf("invalid data directory")
	}

	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("invalid path")
	}

	if !strings.HasPrefix(absPath, absDataDir) {
		return "", fmt.Errorf("invalid model name: path escape attempt")
	}

	// Check if file exists
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		return "", fmt.Errorf("workflow not found: %s", modelName)
	}

	return fullPath, nil
}

// handleListModels handles GET /v1/models - lists available workflows as models
func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendOpenAIError(w, http.StatusMethodNotAllowed, "invalid_request_error", "Method not allowed")
		return
	}

	var models []ModelInfo

	// Walk the data directory to find all YAML files
	_ = filepath.Walk(s.config.DataDir, func(path string, info os.FileInfo, _ error) error {
		if info == nil {
			return nil // Skip files we can't access
		}

		if info.IsDir() {
			return nil
		}

		// Only include .yaml and .yml files
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}

		// Get relative path from DataDir
		relPath, err := filepath.Rel(s.config.DataDir, path)
		if err != nil {
			return nil //nolint:nilerr // intentionally skip files with path errors
		}

		// Remove extension for model ID
		modelID := strings.TrimSuffix(relPath, ext)

		models = append(models, ModelInfo{
			ID:      modelID,
			Object:  "model",
			Created: info.ModTime().Unix(),
			OwnedBy: "comanda",
		})

		return nil
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ModelListResponse{
		Object: "list",
		Data:   models,
	})
}

// handleChatCompletions handles POST /v1/chat/completions
func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		sendOpenAIError(w, http.StatusMethodNotAllowed, "invalid_request_error", "Method not allowed")
		return
	}

	// Parse request body
	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "Invalid JSON: "+err.Error())
		return
	}

	// Validate required fields
	if req.Model == "" {
		sendOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}

	if len(req.Messages) == 0 {
		sendOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "messages is required")
		return
	}

	// Resolve workflow path
	workflowPath, err := s.resolveWorkflowPath(req.Model)
	if err != nil {
		sendOpenAIError(w, http.StatusNotFound, "model_not_found", err.Error())
		return
	}

	// Extract input and build memory context
	input := extractInputFromMessages(req.Messages)
	memoryContext := buildMemoryContext(req.Messages)

	if req.Stream {
		s.handleStreamingChatCompletion(w, r, req, workflowPath, input, memoryContext)
	} else {
		s.handleNonStreamingChatCompletion(w, r, req, workflowPath, input, memoryContext)
	}
}

// handleNonStreamingChatCompletion handles non-streaming chat completion
func (s *Server) handleNonStreamingChatCompletion(w http.ResponseWriter, r *http.Request, req ChatCompletionRequest, workflowPath, input, memoryContext string) {
	// Load and parse workflow
	yamlContent, err := fileutil.SafeReadFile(workflowPath)
	if err != nil {
		sendOpenAIError(w, http.StatusInternalServerError, "server_error", "Failed to read workflow: "+err.Error())
		return
	}

	var rawConfig map[string]processor.StepConfig
	if err := yaml.Unmarshal(yamlContent, &rawConfig); err != nil {
		sendOpenAIError(w, http.StatusInternalServerError, "server_error", "Failed to parse workflow: "+err.Error())
		return
	}

	var dslConfig processor.DSLConfig
	for name, stepConfig := range rawConfig {
		dslConfig.Steps = append(dslConfig.Steps, processor.Step{
			Name:   name,
			Config: stepConfig,
		})
	}

	// Create processor
	runtimeDir := filepath.Dir(workflowPath)
	relPath, _ := filepath.Rel(s.config.DataDir, runtimeDir)
	if relPath == "." {
		relPath = ""
	}

	proc := processor.NewProcessor(&dslConfig, s.envConfig, s.config, true, relPath)
	proc.SetLastOutput(input)

	// Set memory context if we have one
	if memoryContext != "" {
		proc.SetMemoryContext(memoryContext)
	}

	// Capture output
	var buf bytes.Buffer
	pipeReader, pipeWriter, _ := os.Pipe()
	filterWriter := &filteringWriter{
		output: pipeWriter,
		debug:  os.Stdout,
	}

	originalLogOutput := log.Writer()
	log.SetOutput(filterWriter)

	err = proc.Process()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		io.Copy(&buf, pipeReader)
	}()

	log.SetOutput(originalLogOutput)
	pipeWriter.Close()
	wg.Wait()

	finalOutput := proc.LastOutput()

	if err != nil {
		sendOpenAIError(w, http.StatusInternalServerError, "server_error", "Workflow execution failed: "+err.Error())
		return
	}

	// Build response
	completionID := generateCompletionID()
	finishReason := "stop"

	response := ChatCompletionResponse{
		ID:      completionID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []ChatCompletionChoice{
			{
				Index: 0,
				Message: &ChatMessage{
					Role:    "assistant",
					Content: finalOutput,
				},
				FinishReason: &finishReason,
			},
		},
		Usage: UsageInfo{
			PromptTokens:     estimateTokens(input),
			CompletionTokens: estimateTokens(finalOutput),
			TotalTokens:      estimateTokens(input) + estimateTokens(finalOutput),
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleStreamingChatCompletion handles streaming chat completion
func (s *Server) handleStreamingChatCompletion(w http.ResponseWriter, r *http.Request, req ChatCompletionRequest, workflowPath, input, memoryContext string) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		sendOpenAIError(w, http.StatusInternalServerError, "server_error", "Streaming not supported")
		return
	}

	completionID := generateCompletionID()
	created := time.Now().Unix()

	// Send initial chunk with role
	sendStreamChunk(w, flusher, completionID, created, req.Model, &ChatDelta{Role: "assistant"}, nil)

	// Load and parse workflow
	yamlContent, err := fileutil.SafeReadFile(workflowPath)
	if err != nil {
		sendStreamError(w, flusher, "Failed to read workflow: "+err.Error())
		return
	}

	var rawConfig map[string]processor.StepConfig
	if err := yaml.Unmarshal(yamlContent, &rawConfig); err != nil {
		sendStreamError(w, flusher, "Failed to parse workflow: "+err.Error())
		return
	}

	var dslConfig processor.DSLConfig
	for name, stepConfig := range rawConfig {
		dslConfig.Steps = append(dslConfig.Steps, processor.Step{
			Name:   name,
			Config: stepConfig,
		})
	}

	// Create processor
	runtimeDir := filepath.Dir(workflowPath)
	relPath, _ := filepath.Rel(s.config.DataDir, runtimeDir)
	if relPath == "." {
		relPath = ""
	}

	proc := processor.NewProcessor(&dslConfig, s.envConfig, s.config, true, relPath)
	proc.SetLastOutput(input)

	// Set memory context if we have one
	if memoryContext != "" {
		proc.SetMemoryContext(memoryContext)
	}

	// Create progress channel for streaming updates
	progressChan := make(chan processor.ProgressUpdate)
	progressWriter := processor.NewChannelProgressWriter(progressChan)
	proc.SetProgressWriter(progressWriter)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	// Run processor in goroutine
	processDone := make(chan error)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				processDone <- fmt.Errorf("processor panic: %v", r)
			}
		}()
		if err := proc.Process(); err != nil {
			processDone <- err
		} else {
			processDone <- nil
		}
	}()

	// Handle events
	for {
		select {
		case <-ctx.Done():
			sendStreamError(w, flusher, "Request timed out")
			return
		case <-r.Context().Done():
			return
		case err := <-processDone:
			if err != nil {
				sendStreamError(w, flusher, err.Error())
				return
			}
			// Send final output
			finalOutput := proc.LastOutput()
			if finalOutput != "" {
				sendStreamChunk(w, flusher, completionID, created, req.Model, &ChatDelta{Content: finalOutput}, nil)
			}
			// Send finish reason
			finishReason := "stop"
			sendStreamChunk(w, flusher, completionID, created, req.Model, &ChatDelta{}, &finishReason)
			// Send [DONE]
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			return
		case update := <-progressChan:
			// Stream output updates
			if update.Type == processor.ProgressOutput && update.Stdout != "" {
				sendStreamChunk(w, flusher, completionID, created, req.Model, &ChatDelta{Content: update.Stdout}, nil)
			}
		}
	}
}

// sendStreamChunk sends a single SSE chunk in OpenAI format
func sendStreamChunk(w http.ResponseWriter, flusher http.Flusher, id string, created int64, model string, delta *ChatDelta, finishReason *string) {
	chunk := struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		Model   string `json:"model"`
		Choices []struct {
			Index        int        `json:"index"`
			Delta        *ChatDelta `json:"delta"`
			FinishReason *string    `json:"finish_reason"`
		} `json:"choices"`
	}{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []struct {
			Index        int        `json:"index"`
			Delta        *ChatDelta `json:"delta"`
			FinishReason *string    `json:"finish_reason"`
		}{
			{
				Index:        0,
				Delta:        delta,
				FinishReason: finishReason,
			},
		},
	}

	data, _ := json.Marshal(chunk)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}

// sendStreamError sends an error in streaming format
func sendStreamError(w http.ResponseWriter, flusher http.Flusher, message string) {
	errResp := OpenAIError{
		Error: OpenAIErrorDetail{
			Message: message,
			Type:    "server_error",
		},
	}
	data, _ := json.Marshal(errResp)
	fmt.Fprintf(w, "data: %s\n\n", data)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}
