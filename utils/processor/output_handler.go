package processor

import (
	"fmt"
	"os"
	"path/filepath"
)

// handleOutput processes the model's response according to the output configuration
func (p *Processor) handleOutput(modelName string, response string, outputs []string) error {
	p.debugf("Handling %d output(s)", len(outputs))
	for _, output := range outputs {
		p.debugf("Processing output: %s", output)
		if output == "STDOUT" {
			fmt.Printf("\nResponse from %s:\n%s\n", modelName, response)
			p.debugf("Response written to STDOUT")
		} else {
			// Create directory if it doesn't exist
			dir := filepath.Dir(output)
			if dir != "." {
				p.debugf("Creating directory if it doesn't exist: %s", dir)
				if err := os.MkdirAll(dir, 0755); err != nil {
					return fmt.Errorf("failed to create directory %s: %w", dir, err)
				}
			}

			// Write to file
			p.debugf("Writing response to file: %s", output)
			if err := os.WriteFile(output, []byte(response), 0644); err != nil {
				return fmt.Errorf("failed to write response to file %s: %w", output, err)
			}
			p.debugf("Response successfully written to file: %s", output)
		}
	}
	return nil
}
