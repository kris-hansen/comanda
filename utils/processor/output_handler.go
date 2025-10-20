package processor

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// handleOutput processes the model's response according to the output configuration
func (p *Processor) handleOutput(modelName string, response string, outputs []string, metrics *PerformanceMetrics) error {
	p.debugf("[%s] Handling %d output(s)", modelName, len(outputs))
	for _, output := range outputs {
		p.debugf("[%s] Processing output: %s", modelName, output)

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

		if output == "STDOUT" {
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
			if p.serverConfig != nil {
				if p.runtimeDir != "" {
					// When runtime directory is set, treat all output paths as relative to it
					p.debugf("Using runtime directory: %s, output path: %s", p.runtimeDir, output)
					outputPath = filepath.Join(p.serverConfig.DataDir, p.runtimeDir, output)
				} else {
					// No runtime directory, use DataDir directly
					outputPath = filepath.Join(p.serverConfig.DataDir, output)
				}
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

			// Write to file
				p.debugf("[%s] Writing response to file: %s", modelName, outputPath)
			p.debugf("[%s] Response length: %d characters", modelName, len(response))
			p.debugf("[%s] First 100 characters: %s", modelName, response[:min(100, len(response))])

			if err := os.WriteFile(outputPath, []byte(response), 0644); err != nil {
				errMsg := fmt.Sprintf("failed to write response to file %s: %v", outputPath, err)
				p.debugf(errMsg)
				return fmt.Errorf(errMsg)
			}
			p.debugf("[%s] Response successfully written to file: %s", modelName, outputPath)

			// Print a simple confirmation to the console
			log.Printf("\nResponse written to file: %s\n", outputPath)
		}
	}
	return nil
}
