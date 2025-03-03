package processor

import (
	"fmt"
	"os"
	"path/filepath"
)

// handleOutput processes the model's response according to the output configuration
func (p *Processor) handleOutput(modelName string, response string, outputs []string, metrics *PerformanceMetrics) error {
	p.debugf("Handling %d output(s)", len(outputs))
	for _, output := range outputs {
		p.debugf("Processing output: %s", output)
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
				fmt.Printf("\nResponse from %s:\n%s\n", modelName, response)

				// Print performance metrics if available
				if metrics != nil {
					fmt.Printf("\nPerformance Metrics:\n"+
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
			p.debugf("Response written to STDOUT")
		} else {
			// Determine the output path based on server mode
			outputPath := output
			if p.serverConfig != nil && p.serverConfig.Enabled {
				// In server mode, make the path relative to DataDir
				p.debugf("Server mode enabled, using DataDir: %s", p.serverConfig.DataDir)
				outputPath = filepath.Join(p.serverConfig.DataDir, output)
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
			p.debugf("Writing response to file: %s", outputPath)
			if err := os.WriteFile(outputPath, []byte(response), 0644); err != nil {
				return fmt.Errorf("failed to write response to file %s: %w", outputPath, err)
			}
			p.debugf("Response successfully written to file: %s", outputPath)
		}
	}
	return nil
}
