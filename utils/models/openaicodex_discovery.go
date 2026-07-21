package models

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const openAICodexModelAlias = "openai-codex"

var (
	openAICodexModelsOnce sync.Once
	openAICodexModels     []string
)

// GetOpenAICodexModels returns the models currently available to the locally
// authenticated Codex CLI. The CLI's app-server is the authoritative source:
// Codex availability varies by account and changes independently of Comanda.
//
// The generic openai-codex alias is always retained. If discovery is not
// available (for example, with an older Codex CLI), callers still get that
// alias instead of a stale, fabricated catalog.
func GetOpenAICodexModels() []string {
	openAICodexModelsOnce.Do(func() {
		models, err := discoverOpenAICodexModels(context.Background())
		if err != nil {
			openAICodexModels = []string{openAICodexModelAlias}
			return
		}
		openAICodexModels = normalizeOpenAICodexModels(models)
	})

	return append([]string(nil), openAICodexModels...)
}

type codexRPCMessage struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type codexModelListResult struct {
	Data []struct {
		Model string `json:"model"`
	} `json:"data"`
}

// discoverOpenAICodexModels uses the documented Codex app-server protocol.
// model/list must follow initialize and initialized, and the initialize result
// must be read before the next request is sent.
func discoverOpenAICodexModels(parent context.Context) ([]string, error) {
	provider := NewOpenAICodexProvider()
	binaryPath, err := provider.findCodexBinary()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "app-server", "--stdio")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open Codex app-server stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open Codex app-server stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start Codex app-server: %w", err)
	}

	encoder := json.NewEncoder(stdin)
	decoder := json.NewDecoder(stdout)
	if err := encoder.Encode(map[string]any{
		"id":     1,
		"method": "initialize",
		"params": map[string]any{"clientInfo": map[string]string{"name": "comanda", "version": "model-discovery"}},
	}); err != nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		return nil, fmt.Errorf("initialize Codex app-server: %w", err)
	}
	if _, err := waitForCodexResponse(decoder, 1); err != nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		return nil, err
	}
	if err := encoder.Encode(map[string]any{"method": "initialized"}); err != nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		return nil, fmt.Errorf("notify Codex app-server initialized: %w", err)
	}
	if err := encoder.Encode(map[string]any{
		"id":     2,
		"method": "model/list",
		"params": map[string]any{"limit": 100, "includeHidden": false},
	}); err != nil {
		_ = stdin.Close()
		_ = cmd.Wait()
		return nil, fmt.Errorf("request Codex models: %w", err)
	}

	response, err := waitForCodexResponse(decoder, 2)
	_ = stdin.Close()
	waitErr := cmd.Wait()
	if err != nil {
		return nil, err
	}
	if waitErr != nil {
		return nil, fmt.Errorf("Codex app-server exited: %w (%s)", waitErr, strings.TrimSpace(stderr.String()))
	}

	var result codexModelListResult
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return nil, fmt.Errorf("decode Codex model list: %w", err)
	}
	models := make([]string, 0, len(result.Data))
	for _, item := range result.Data {
		if item.Model != "" {
			models = append(models, item.Model)
		}
	}
	if len(models) == 0 {
		return nil, fmt.Errorf("Codex app-server returned no models")
	}
	return models, nil
}

func waitForCodexResponse(decoder *json.Decoder, id int) (codexRPCMessage, error) {
	for {
		var message codexRPCMessage
		if err := decoder.Decode(&message); err != nil {
			return codexRPCMessage{}, fmt.Errorf("read Codex app-server response: %w", err)
		}
		var responseID int
		if len(message.ID) == 0 || json.Unmarshal(message.ID, &responseID) != nil || responseID != id {
			continue // Notification or response to another request.
		}
		if message.Error != nil {
			return codexRPCMessage{}, fmt.Errorf("Codex app-server error: %s", message.Error.Message)
		}
		return message, nil
	}
}

func normalizeOpenAICodexModels(discovered []string) []string {
	models := []string{openAICodexModelAlias}
	seen := map[string]bool{openAICodexModelAlias: true}
	for _, model := range discovered {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		wrapped := openAICodexModelAlias + "-" + model
		if !seen[wrapped] {
			models = append(models, wrapped)
			seen[wrapped] = true
		}
	}
	return models
}
