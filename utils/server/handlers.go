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

	"github.com/kris-hansen/comanda/utils/config"
	"github.com/kris-hansen/comanda/utils/fileutil"
	"github.com/kris-hansen/comanda/utils/processor"
	"gopkg.in/yaml.v3"
)

// No default runtime directory - use data directory by default

func handleProcess(w http.ResponseWriter, r *http.Request, serverConfig *config.ServerConfig, envConfig *config.EnvConfig) {
	// Determine if streaming is requested
	streaming := r.URL.Query().Get("streaming") == "true" || r.Header.Get("Accept") == "text/event-stream"

	// Set appropriate headers based on streaming mode
	if streaming {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
	} else {
		w.Header().Set("Content-Type", "application/json")
	}

	// Verify this is a POST request first
	if r.Method != http.MethodPost {
		config.VerboseLog("Method not allowed: requires POST")
		config.DebugLog("Method not allowed: got %s, need POST", r.Method)
		w.WriteHeader(http.StatusMethodNotAllowed)
		if streaming {
			flusher, ok := w.(http.Flusher)
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(ProcessResponse{
					Success: false,
					Error:   "Streaming is not supported",
				})
				return
			}
			sw := &sseWriter{w: w, f: flusher}
			sw.SendError(fmt.Errorf("YAML processing is only available via POST requests. Please use POST with your YAML content."))
		} else {
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   "YAML processing is only available via POST requests. Please use POST with your YAML content.",
			})
		}
		return
	}

	// Get filename from query parameters
	filename := r.URL.Query().Get("filename")
	if filename == "" {
		config.VerboseLog("Missing filename parameter")
		config.DebugLog("Process request failed: no filename provided")
		w.WriteHeader(http.StatusBadRequest)
		if streaming {
			flusher, ok := w.(http.Flusher)
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(ProcessResponse{
					Success: false,
					Error:   "Streaming is not supported",
				})
				return
			}
			sw := &sseWriter{w: w, f: flusher}
			sw.SendError(fmt.Errorf("filename parameter is required"))
		} else {
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   "filename parameter is required",
			})
		}
		return
	}

	config.VerboseLog("Processing file: %s", filename)
	config.DebugLog("Starting process request for file: %s", filename)

	// Clean the path to remove any . or .. components
	cleanPath := filepath.Clean(filename)

	// Check if the path contains directory separators
	if !strings.Contains(cleanPath, string(filepath.Separator)) {
		// No directory specified, assume it's in the root of DataDir
		cleanPath = filepath.Join(serverConfig.DataDir, cleanPath)
		config.DebugLog("Using file in data directory: %s", cleanPath)
	} else {
		// Path contains separators, prepend DataDir
		cleanPath = filepath.Join(serverConfig.DataDir, cleanPath)
		config.DebugLog("Using path with directories: %s", cleanPath)
	}

	config.DebugLog("Adjusted filename path: %s", cleanPath)

	// Get the relative path between the data directory and the target file
	relPath, err := filepath.Rel(serverConfig.DataDir, cleanPath)
	if err != nil {
		config.VerboseLog("Error validating file path: %v", err)
		config.DebugLog("Path validation error: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		if streaming {
			flusher, ok := w.(http.Flusher)
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(ProcessResponse{
					Success: false,
					Error:   "Streaming is not supported",
				})
				return
			}
			sw := &sseWriter{w: w, f: flusher}
			sw.SendError(fmt.Errorf("Invalid file path"))
		} else {
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   "Invalid file path",
			})
		}
		return
	}

	// Check if the relative path tries to escape the data directory
	if strings.HasPrefix(relPath, "..") || strings.Contains(relPath, "/../") {
		config.VerboseLog("Attempted directory traversal detected")
		config.DebugLog("Security violation: attempted path traversal")
		w.WriteHeader(http.StatusForbidden)
		if streaming {
			flusher, ok := w.(http.Flusher)
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(ProcessResponse{
					Success: false,
					Error:   "Streaming is not supported",
				})
				return
			}
			sw := &sseWriter{w: w, f: flusher}
			sw.SendError(fmt.Errorf("Invalid file path: attempted directory traversal"))
		} else {
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   "Invalid file path: attempted directory traversal",
			})
		}
		return
	}

	// Verify the final path exists and is within the data directory
	finalPath := filepath.Clean(cleanPath)
	if !strings.HasPrefix(finalPath, filepath.Clean(serverConfig.DataDir)) {
		config.VerboseLog("File path escapes data directory")
		config.DebugLog("Security violation: path escapes data directory")
		w.WriteHeader(http.StatusForbidden)
		if streaming {
			flusher, ok := w.(http.Flusher)
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(ProcessResponse{
					Success: false,
					Error:   "Streaming is not supported",
				})
				return
			}
			sw := &sseWriter{w: w, f: flusher}
			sw.SendError(fmt.Errorf("Invalid file path: access denied"))
		} else {
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   "Invalid file path: access denied",
			})
		}
		return
	}

	// Read and validate YAML file
	yamlContent, err := fileutil.SafeReadFile(finalPath)
	if err != nil {
		config.VerboseLog("Error reading file: %v", err)
		config.DebugLog("File read error: path=%s error=%v", finalPath, err)
		if streaming {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			flusher, ok := w.(http.Flusher)
			if !ok {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(ProcessResponse{
					Success: false,
					Error:   "Streaming is not supported",
				})
				return
			}
			sw := &sseWriter{w: w, f: flusher}
			sw.SendError(fmt.Errorf("Error reading YAML file: %v", err))
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   fmt.Sprintf("Error reading YAML file: %v", err),
			})
		}
		return
	}

	// Log YAML content details before parsing
	config.DebugLog("Processing YAML content: length=%d bytes", len(yamlContent))

	// First unmarshal into a map to preserve step names (same as CLI)
	var rawConfig map[string]processor.StepConfig
	if err := yaml.Unmarshal(yamlContent, &rawConfig); err != nil {
		config.VerboseLog("Error parsing YAML: %v", err)
		config.DebugLog("YAML parse error: content_preview='%s' error=%v", truncateString(string(yamlContent), 200), err)
		w.WriteHeader(http.StatusBadRequest)
		if streaming {
			flusher, ok := w.(http.Flusher)
			if !ok {
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(ProcessResponse{
					Success: false,
					Error:   "Streaming is not supported",
				})
				return
			}
			sw := &sseWriter{w: w, f: flusher}
			sw.SendError(fmt.Errorf("Error parsing YAML file: %v", err))
		} else {
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   fmt.Sprintf("Error parsing YAML file: %v", err),
			})
		}
		return
	}

	// Convert map to ordered Steps slice (same as CLI)
	var dslConfig processor.DSLConfig
	config.DebugLog("Converting YAML to DSL config: step_count=%d", len(rawConfig))
	for name, stepConfig := range rawConfig {
		config.DebugLog("Processing step: name=%s model=%v action=%v", name, stepConfig.Model, stepConfig.Action)
		dslConfig.Steps = append(dslConfig.Steps, processor.Step{
			Name:   name,
			Config: stepConfig,
		})
	}

	// Get runtime directory from query parameter or calculate from path
	runtimeDir := r.URL.Query().Get("runtimeDir")
	if runtimeDir == "" {
		// Calculate from path if not provided
		runtimeDir = filepath.Dir(relPath) // Use relPath which is relative to DataDir
		if runtimeDir == "." {
			runtimeDir = "" // No subdirectory
		}
	}
	config.DebugLog("Runtime directory context:")
	config.DebugLog("- From query param: %v", r.URL.Query().Get("runtimeDir") != "")
	config.DebugLog("- DataDir: %s", serverConfig.DataDir)
	config.DebugLog("- RelPath: %s", relPath)
	config.DebugLog("- RuntimeDir: %s", runtimeDir)
	config.DebugLog("- CleanPath: %s", cleanPath)

	// Create and configure processor with runtime directory
	config.DebugLog("Creating processor instance with validation enabled")
	proc := processor.NewProcessor(&dslConfig, envConfig, serverConfig, true, runtimeDir)
	config.DebugLog("Processor created successfully with config: steps=%d, runtimeDir=%s", len(dslConfig.Steps), runtimeDir)

	// Handle POST input with detailed logging
	var stdinInput string

	config.DebugLog("Processing POST request input: content_type=%s", r.Header.Get("Content-Type"))

	// First check query parameter
	stdinInput = r.URL.Query().Get("input")

	// If not in query, check JSON body
	if stdinInput == "" && r.Body != nil {
		var jsonBody struct {
			Input     string `json:"input"`
			Streaming bool   `json:"streaming"`
		}
		if err := json.NewDecoder(r.Body).Decode(&jsonBody); err == nil {
			stdinInput = jsonBody.Input
			streaming = jsonBody.Streaming
		}
		config.DebugLog("Extracted input from JSON body")
	}

	// Always initialize the processor with input (empty string if none provided)
	if stdinInput != "" {
		config.VerboseLog("Processing STDIN input")
		config.DebugLog("Input length: %d bytes", len(stdinInput))
	} else {
		config.VerboseLog("No input provided - proceeding with empty input")
		config.DebugLog("Processing without input")
		stdinInput = ""
	}
	// Set the input (empty or not) as the processor's last output
	proc.SetLastOutput(stdinInput)

	// Check Accept header for streaming
	if r.Header.Get("Accept") == "text/event-stream" {
		streaming = true
	}

	if streaming {
		config.DebugLog("Initializing SSE streaming mode")

		// Set up SSE streaming
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		config.DebugLog("SSE headers set")

		// Flusher capability is already checked in middleware
		flusher, ok := w.(http.Flusher)
		if !ok {
			config.DebugLog("Streaming requested but flusher not available, type: %T", w)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(ProcessResponse{
				Success: false,
				Error:   "Streaming is not supported by this server configuration",
			})
			return
		}
		config.DebugLog("Setting up streaming with confirmed flusher support")
		flusher.Flush() // Ensure headers are sent immediately

		var sw *sseWriter
		var progressChan chan processor.ProgressUpdate

		// Create SSE writer with confirmed flusher support
		sw = &sseWriter{w: w, f: flusher}
		config.DebugLog("SSE writer created")

		// Create progress channel and writer
		progressChan = make(chan processor.ProgressUpdate)
		progressWriter := processor.NewChannelProgressWriter(progressChan)
		config.DebugLog("Progress channel created: buffer=%d, capacity=%d", len(progressChan), cap(progressChan))

		// Set up processor with progress writer
		proc.SetProgressWriter(progressWriter)
		config.DebugLog("Progress writer configured on processor")

		// Create context with timeout
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()

		// Add panic recovery for processor initialization
		defer func() {
			if r := recover(); r != nil {
				config.DebugLog("Panic during processor setup: %v", r)
				if sw != nil {
					sw.SendError(fmt.Errorf("internal server error: processor initialization failed"))
				}
				return
			}
		}()

		// Send initial progress message
		if sw != nil {
			sw.SendProgress("Starting workflow processing")
		}

		// Run the processor in a goroutine with error context
		processDone := make(chan error)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					config.DebugLog("Panic in processor goroutine: %v", r)
					processDone <- fmt.Errorf("processor panic: %v", r)
				}
			}()

			config.DebugLog("Starting processor goroutine for streaming")
			if err := proc.Process(); err != nil {
				config.DebugLog("Processor error in streaming mode: %v", err)
				processDone <- fmt.Errorf("processing error: %w", err)
			} else {
				processDone <- nil
			}
		}()

		// Start heartbeat ticker
		heartbeat := time.NewTicker(15 * time.Second)
		defer heartbeat.Stop()

		// Handle events
		config.DebugLog("Starting event processing loop")
		for {
			select {
			case <-ctx.Done():
				config.DebugLog("Context timeout reached after 5 minutes")
				if sw != nil {
					sw.SendError(fmt.Errorf("processing timed out after 5 minutes"))
				}
				return
			case <-r.Context().Done():
				config.DebugLog("Client connection closed: %v", r.Context().Err())
				return
			case err := <-processDone:
				if err != nil {
					errMsg := fmt.Sprintf("Processing failed: %v", err)
					config.DebugLog("Streaming error: %s", errMsg)
					if sw != nil {
						sw.SendError(fmt.Errorf("%s", errMsg))
					}
				} else {
					config.DebugLog("Processing completed successfully")
					if sw != nil {
						sw.SendProgress("Workflow processing completed successfully")
					}
				}
				return
			case update := <-progressChan:
				config.DebugLog("Received progress update: type=%v message='%s'", update.Type, update.Message)
				if sw == nil {
					config.DebugLog("Warning: SSE writer is nil while receiving progress update")
					continue
				}
				switch update.Type {
				case processor.ProgressStep:
					// For initial and completion messages, send as plain text
					if update.Message == "Starting workflow processing" || update.Message == "Workflow processing completed successfully" {
						sw.SendProgress(update.Message)
					} else {
						// For other messages, create structured progress data
						progressData := map[string]interface{}{
							"message": update.Message,
						}
						if update.Step != nil {
							progressData["step"] = map[string]string{
								"name":   update.Step.Name,
								"model":  update.Step.Model,
								"action": update.Step.Action,
							}
						}
						sw.SendProgress(progressData)
					}
				case processor.ProgressOutput:
					config.DebugLog("Received output event: %s", update.Stdout)
					sw.SendOutput(update.Stdout)
				case processor.ProgressComplete:
					config.DebugLog("Received completion event: %s", update.Message)
					sw.SendComplete(update.Message)
				case processor.ProgressError:
					config.DebugLog("Received error event: %v", update.Error)
					sw.SendError(update.Error)
				default:
					config.DebugLog("Warning: Unknown progress update type: %v", update.Type)
				}
			case <-heartbeat.C:
				config.DebugLog("Sending heartbeat")
				if sw != nil {
					sw.SendHeartbeat()
				}
			}
		}
	}

	// Non-streaming behavior (existing implementation)
	var buf bytes.Buffer
	pipeReader, pipeWriter, _ := os.Pipe()
	filterWriter := &filteringWriter{
		output: pipeWriter,
		debug:  os.Stdout,
	}

	originalLogOutput := log.Writer()
	log.SetOutput(filterWriter)

	config.DebugLog("Starting workflow processing")

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
		config.VerboseLog("Error processing workflow: %v", err)
		config.DebugLog("Workflow processing error: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(ProcessResponse{
			Success: false,
			Error:   fmt.Sprintf("Error processing workflow file: %v", err),
			Output:  finalOutput,
		})
		return
	}

	config.VerboseLog("Successfully processed file: %s", filename)
	config.DebugLog("Workflow processing complete. Output length: %d bytes", len(finalOutput))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ProcessResponse{
		Success: true,
		Message: fmt.Sprintf("Successfully processed %s", filename),
		Output:  finalOutput,
	})
}
