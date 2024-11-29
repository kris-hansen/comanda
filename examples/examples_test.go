package examples

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestYAMLFiles(t *testing.T) {
	// Directories to check for YAML files
	dirs := []string{
		".",
		"model-examples",
		"file-processing",
		"web-scraping",
		"document-processing",
		"image-processing",
	}

	yamlCount := 0
	for _, dir := range dirs {
		files, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("Failed to read directory %s: %v", dir, err)
		}

		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".yaml") {
				yamlCount++
				filePath := filepath.Join(dir, file.Name())
				t.Run(filePath, func(t *testing.T) {
					validateYAMLFile(t, filePath)
				})
			}
		}
	}

	if yamlCount == 0 {
		t.Error("No YAML files found in examples directory")
	}
}

func validateYAMLFile(t *testing.T, filename string) {
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Errorf("Failed to read file %s: %v", filename, err)
		return
	}

	var root yaml.Node
	err = yaml.Unmarshal(content, &root)
	if err != nil {
		t.Errorf("Failed to parse YAML in %s: %v", filename, err)
		return
	}

	// Root should be a document
	if root.Kind != yaml.DocumentNode {
		t.Errorf("File %s: expected document node at root", filename)
		return
	}

	// Document should contain a mapping
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		t.Errorf("File %s: expected mapping node as document content", filename)
		return
	}

	// First pass: collect all output files
	outputFiles := make(map[string]bool)
	for i := 0; i < len(doc.Content); i += 2 {
		value := doc.Content[i+1]
		if value.Kind == yaml.MappingNode {
			collectOutputFiles(value, outputFiles)
		}
	}

	// Validate each step in the document
	foundSteps := false
	for i := 0; i < len(doc.Content); i += 2 {
		key := doc.Content[i]
		value := doc.Content[i+1]

		// Consider any mapping with the required fields as a step
		if value.Kind == yaml.MappingNode {
			foundSteps = true
			validateStep(t, filename, key.Value, value, outputFiles)
		}
	}

	if !foundSteps {
		t.Errorf("File %s contains no valid steps", filename)
	}
}

func collectOutputFiles(stepNode *yaml.Node, outputFiles map[string]bool) {
	for i := 0; i < len(stepNode.Content); i += 2 {
		key := stepNode.Content[i].Value
		value := stepNode.Content[i+1]

		if key == "output" {
			if value.Kind == yaml.ScalarNode {
				outputFiles[value.Value] = true
			} else if value.Kind == yaml.SequenceNode {
				for _, item := range value.Content {
					outputFiles[item.Value] = true
				}
			}
		}
	}
}

func validateStep(t *testing.T, filename, stepName string, stepNode *yaml.Node, outputFiles map[string]bool) {
	if stepNode.Kind != yaml.MappingNode {
		t.Errorf("Step %s in %s: expected mapping node", stepName, filename)
		return
	}

	requiredFields := map[string]bool{
		"input":  false,
		"model":  false,
		"action": false,
		"output": false,
	}

	for i := 0; i < len(stepNode.Content); i += 2 {
		key := stepNode.Content[i].Value
		value := stepNode.Content[i+1]

		if _, required := requiredFields[key]; required {
			requiredFields[key] = true

			// Value can be either a scalar or sequence
			if value.Kind != yaml.ScalarNode && value.Kind != yaml.SequenceNode {
				t.Errorf("Step %s in %s: field '%s' must be either a string or array", stepName, filename, key)
				continue
			}

			// For sequence nodes, ensure they're not empty
			if value.Kind == yaml.SequenceNode && len(value.Content) == 0 {
				t.Errorf("Step %s in %s: field '%s' array is empty", stepName, filename, key)
				continue
			}

			// For scalar nodes, ensure they're not empty
			if value.Kind == yaml.ScalarNode && value.Value == "" {
				t.Errorf("Step %s in %s: field '%s' is empty", stepName, filename, key)
				continue
			}

			// Validate input files exist if they're local files
			if key == "input" {
				if value.Kind == yaml.ScalarNode {
					validateInputPath(t, filename, stepName, value.Value, outputFiles)
				} else {
					for _, item := range value.Content {
						validateInputPath(t, filename, stepName, item.Value, outputFiles)
					}
				}
			}
		}
	}

	// Check if any required fields are missing
	for field, found := range requiredFields {
		if !found {
			t.Errorf("Step %s in %s: missing required field '%s'", stepName, filename, field)
		}
	}
}

func validateInputPath(t *testing.T, filename, stepName, input string, outputFiles map[string]bool) {
	// Skip special input types
	if isSpecialInput(input) {
		return
	}

	// Handle filenames tag format
	if strings.HasPrefix(input, "filenames:") {
		files := strings.TrimPrefix(input, "filenames:")
		// Split by comma and validate each file
		for _, file := range strings.Split(files, ",") {
			file = strings.TrimSpace(file)
			if file != "" {
				validateSingleInputPath(t, filename, stepName, file, outputFiles)
			}
		}
		return
	}

	validateSingleInputPath(t, filename, stepName, input, outputFiles)
}

func validateSingleInputPath(t *testing.T, filename, stepName, input string, outputFiles map[string]bool) {
	// If it's an output from another step, no need to check the file
	if outputFiles[input] {
		return
	}

	// Get the directory of the YAML file
	yamlDir := filepath.Dir(filename)

	// Possible paths to check
	paths := []string{
		input,                         // As is
		filepath.Join(yamlDir, input), // Relative to YAML file
	}

	// If input starts with "examples/", also check without the prefix
	if strings.HasPrefix(input, "examples/") {
		paths = append(paths, strings.TrimPrefix(input, "examples/"))
	}

	// Check if file exists in any of the possible paths
	fileExists := false
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			fileExists = true
			break
		}
	}

	if !fileExists {
		t.Errorf("Step %s in %s references non-existent input file: %s", stepName, filename, input)
	}
}

func isSpecialInput(input string) bool {
	// List of special input types that don't require file validation
	specialInputs := []string{
		"STDIN",
		"screenshot",
		"NA",
	}

	// Check if input is a special type
	for _, special := range specialInputs {
		if input == special || strings.HasPrefix(input, "STDIN as $") {
			return true
		}
	}

	// Check if input is a URL
	return strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://")
}
