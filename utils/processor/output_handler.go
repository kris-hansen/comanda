package processor

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Output mode constants
const (
	OutputModeOverwrite   = "overwrite"   // Default: overwrite existing file
	OutputModeAppend      = "append"      // Append to existing file
	OutputModeIncremental = "incremental" // Load file as context, then append new content
)

// loadIncrementalContext loads existing file content for incremental output mode.
// Returns the file content if found, or empty string if file doesn't exist.
// This context should be prepended to the step's input.
func (p *Processor) loadIncrementalContext(stepConfig *StepConfig) string {
	if stepConfig == nil || stepConfig.OutputMode != OutputModeIncremental {
		return ""
	}

	outputs := p.NormalizeStringSlice(stepConfig.Output)
	if len(outputs) == 0 {
		return ""
	}

	// Get the first output file (for incremental mode, typically one output file)
	outputPath := outputs[0]
	if outputPath == "" || outputPath == OutputSTDOUT || outputPath == "STDOUT" {
		return ""
	}

	// Resolve the path
	if p.serverConfig != nil && p.serverConfig.Enabled {
		if p.runtimeDir != "" {
			outputPath = filepath.Join(p.serverConfig.DataDir, p.runtimeDir, outputPath)
		} else {
			outputPath = filepath.Join(p.serverConfig.DataDir, outputPath)
		}
	} else if p.runtimeDir != "" && !filepath.IsAbs(outputPath) {
		outputPath = filepath.Join(p.runtimeDir, outputPath)
	}

	// Read existing content
	content, err := os.ReadFile(outputPath)
	if err != nil {
		if os.IsNotExist(err) {
			p.debugf("Incremental mode: output file does not exist yet: %s", outputPath)
			return ""
		}
		p.debugf("Incremental mode: error reading output file %s: %v", outputPath, err)
		return ""
	}

	if len(content) == 0 {
		return ""
	}

	p.debugf("Incremental mode: loaded %d bytes from %s", len(content), outputPath)

	// Format as context block
	return fmt.Sprintf("[EXISTING DOCUMENT - Continue from where this ends]\n%s\n[END EXISTING DOCUMENT]\n\n", string(content))
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// handleOutput processes the model's response according to the output configuration
func (p *Processor) handleOutput(modelName string, response string, outputs []string, metrics *PerformanceMetrics) error {
	return p.handleOutputWithOptions(modelName, response, outputs, metrics, nil, "")
}

// handleOutputWithToolConfig processes the model's response with optional tool configuration
func (p *Processor) handleOutputWithToolConfig(modelName string, response string, outputs []string, metrics *PerformanceMetrics, toolConfig *ToolListConfig) error {
	return p.handleOutputWithOptions(modelName, response, outputs, metrics, toolConfig, "")
}

// handleOutputWithMode processes the model's response with a specific output mode
func (p *Processor) handleOutputWithMode(modelName string, response string, outputs []string, metrics *PerformanceMetrics, toolConfig *ToolListConfig, outputMode string) error {
	return p.handleOutputWithOptions(modelName, response, outputs, metrics, toolConfig, outputMode)
}

// handleOutputForStep processes output using the step's configuration
func (p *Processor) handleOutputForStep(modelName string, response string, outputs []string, metrics *PerformanceMetrics, stepConfig *StepConfig) error {
	var toolConfig *ToolListConfig
	var outputMode string
	if stepConfig != nil {
		toolConfig = stepConfig.ToolConfig
		outputMode = stepConfig.OutputMode
	}
	return p.handleOutputWithOptions(modelName, response, outputs, metrics, toolConfig, outputMode)
}

// handleOutputWithOptions processes the model's response with full options
func (p *Processor) handleOutputWithOptions(modelName string, response string, outputs []string, metrics *PerformanceMetrics, toolConfig *ToolListConfig, outputMode string) error {
	p.debugf("[%s] Handling %d output(s)", modelName, len(outputs))

	// Apply CLI variable substitution to outputs
	for i, output := range outputs {
		outputs[i] = p.SubstituteCLIVariables(output)
	}

	for _, output := range outputs {
		p.debugf("[%s] Processing output: %s", modelName, output)

		// Check for tool output (e.g., "tool: jq '.data'" or "STDOUT|grep 'pattern'")
		if IsToolOutput(output) {
			p.debugf("[%s] Processing tool output: %s", modelName, output)

			// Parse the tool command
			command, _, err := ParseToolOutput(output)
			if err != nil {
				return fmt.Errorf("failed to parse tool output: %w", err)
			}

			// Create tool executor with merged global + step-level configuration
			stepToolConfig := &ToolConfig{}
			if toolConfig != nil {
				stepToolConfig.Allowlist = toolConfig.Allowlist
				stepToolConfig.Denylist = toolConfig.Denylist
				stepToolConfig.Timeout = toolConfig.Timeout
			}
			executorConfig := MergeToolConfigs(p.getGlobalToolConfig(), stepToolConfig)
			executor := NewToolExecutor(executorConfig, p.verbose, p.debugf)

			// Execute the tool with the response as stdin
			stdout, stderr, err := executor.Execute("STDIN|"+command, response)
			if err != nil {
				if stderr != "" {
					return fmt.Errorf("tool output execution failed: %w\nstderr: %s", err, stderr)
				}
				return fmt.Errorf("tool output execution failed: %w", err)
			}

			if stderr != "" {
				p.debugf("[%s] Tool stderr: %s", modelName, stderr)
			}

			// Output the tool's stdout
			response = stdout
			p.debugf("[%s] Tool output (%d bytes)", modelName, len(response))

			// Now send this through STDOUT handling
			if p.progress != nil {
				if err := p.progress.WriteProgress(ProgressUpdate{
					Type:               ProgressOutput,
					Stdout:             response,
					PerformanceMetrics: metrics,
				}); err != nil {
					p.debugf("[%s] Error sending tool output event: %v", modelName, err)
					return err
				}
			} else {
				log.Printf("\nTool Output from %s:\n%s\n", modelName, response)
			}
			continue
		}

		// Handle MEMORY output
		if output == "MEMORY" || strings.HasPrefix(output, "MEMORY:") {
			if p.memory == nil || !p.memory.HasMemory() {
				p.debugf("Warning: MEMORY output requested but no memory file configured")
				continue
			}

			// Check if this is a sectioned memory write
			if strings.HasPrefix(output, "MEMORY:") {
				sectionName := strings.TrimPrefix(output, "MEMORY:")
				p.debugf("Writing to memory section: %s", sectionName)
				if err := p.memory.WriteMemorySection(sectionName, response); err != nil {
					return fmt.Errorf("failed to write to memory section %s: %w", sectionName, err)
				}
				p.debugf("Successfully wrote to memory section: %s", sectionName)
			} else {
				// Append to memory file
				p.debugf("Appending to memory file")
				if err := p.memory.AppendMemory(response); err != nil {
					return fmt.Errorf("failed to append to memory: %w", err)
				}
				p.debugf("Successfully appended to memory file")
			}
			continue
		}

		if output == OutputSTDOUT {
			if p.progress != nil {
				// Send through progress channel for streaming
				p.debugf("Sending output event with content: %s", response)

				// Format performance metrics for display
				var perfInfo string
				if metrics != nil {
					perfInfo = fmt.Sprintf("\n\nPerformance Metrics:\n"+
						"- Input processing: %d ms\n"+
						"- Model processing: %d ms\n"+
						"- Action processing: %d ms\n"+
						"- Output processing: (in progress)\n"+
						"- Total processing: (in progress)\n",
						metrics.InputProcessingTime,
						metrics.ModelProcessingTime,
						metrics.ActionProcessingTime)
				}

				// Add performance metrics to the output
				outputWithMetrics := response
				if metrics != nil {
					outputWithMetrics = response + perfInfo
				}

				if err := p.progress.WriteProgress(ProgressUpdate{
					Type:               ProgressOutput,
					Stdout:             outputWithMetrics,
					PerformanceMetrics: metrics,
				}); err != nil {
					p.debugf("Error sending output event: %v", err)
					return err
				}
				p.debugf("Output event sent successfully")
			} else {
				// Fallback to direct console output
				log.Printf("\nResponse from %s:\n%s\n", modelName, response)

				// Print performance metrics if available
				if metrics != nil {
					log.Printf("\nPerformance Metrics:\n"+
						"- Input processing: %d ms\n"+
						"- Model processing: %d ms\n"+
						"- Action processing: %d ms\n"+
						"- Output processing: (in progress)\n"+
						"- Total processing: (in progress)\n",
						metrics.InputProcessingTime,
						metrics.ModelProcessingTime,
						metrics.ActionProcessingTime)
				}
			}
			p.debugf("[%s] Response written to STDOUT", modelName)
		} else {
			// Determine the output path based on server mode and runtime directory
			outputPath := output
			if p.serverConfig != nil && p.serverConfig.Enabled {
				if p.runtimeDir != "" {
					// When runtime directory is set, treat all output paths as relative to it
					p.debugf("Using runtime directory: %s, output path: %s", p.runtimeDir, output)
					outputPath = filepath.Join(p.serverConfig.DataDir, p.runtimeDir, output)
				} else {
					// No runtime directory, use DataDir directly
					outputPath = filepath.Join(p.serverConfig.DataDir, output)
				}
				p.debugf("Resolved output path: %s", outputPath)
			} else if p.runtimeDir != "" && !filepath.IsAbs(output) {
				p.debugf("CLI mode with runtime directory: using path '%s' in %s", output, p.runtimeDir)
				outputPath = filepath.Join(p.runtimeDir, output)
				p.debugf("Resolved output path: %s", outputPath)
			}

			// Create directory if it doesn't exist
			dir := filepath.Dir(outputPath)
			if dir != "." {
				p.debugf("Creating directory if it doesn't exist: %s", dir)
				if err := os.MkdirAll(dir, 0755); err != nil {
					return fmt.Errorf("failed to create directory %s: %w", dir, err)
				}
			}

			// Handle output mode
			p.debugf("[%s] Writing response to file: %s (mode: %s)", modelName, outputPath, outputMode)
			p.debugf("[%s] Response length: %d characters", modelName, len(response))
			if len(response) > 0 {
				p.debugf("[%s] First 100 characters: %s", modelName, response[:min(100, len(response))])
			}

			var writeErr error
			switch outputMode {
			case OutputModeAppend:
				// Append to existing file
				f, err := os.OpenFile(outputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					writeErr = fmt.Errorf("failed to open file for append %s: %w", outputPath, err)
				} else {
					// Add newline separator if file exists and has content
					if info, _ := os.Stat(outputPath); info != nil && info.Size() > 0 {
						_, _ = f.WriteString("\n")
					}
					_, err = f.WriteString(response)
					f.Close()
					if err != nil {
						writeErr = fmt.Errorf("failed to append to file %s: %w", outputPath, err)
					}
				}
				p.debugf("[%s] Response appended to file: %s", modelName, outputPath)

			case OutputModeIncremental:
				// Append to existing file (incremental mode - load happens at input processing)
				f, err := os.OpenFile(outputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					writeErr = fmt.Errorf("failed to open file for incremental write %s: %w", outputPath, err)
				} else {
					// Add section separator
					if info, _ := os.Stat(outputPath); info != nil && info.Size() > 0 {
						_, _ = f.WriteString("\n\n---\n\n")
					}
					_, err = f.WriteString(response)
					f.Close()
					if err != nil {
						writeErr = fmt.Errorf("failed to write incrementally to file %s: %w", outputPath, err)
					}
				}
				p.debugf("[%s] Response written incrementally to file: %s", modelName, outputPath)

			default: // OutputModeOverwrite or empty
				// Default: overwrite existing file
				writeErr = os.WriteFile(outputPath, []byte(response), 0644)
				if writeErr != nil {
					writeErr = fmt.Errorf("failed to write response to file %s: %w", outputPath, writeErr)
				}
				p.debugf("[%s] Response written to file (overwrite): %s", modelName, outputPath)
			}

			if writeErr != nil {
				errMsg := writeErr.Error()
				p.debugf("%s", errMsg)
				return writeErr
			}

			// Print a simple confirmation to the console
			log.Printf("\nResponse written to file: %s\n", outputPath)
		}
	}
	return nil
}
