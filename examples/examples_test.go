package examples

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	processor "github.com/kris-hansen/comanda/utils/processor"
	"gopkg.in/yaml.v3"
)

func TestExampleWorkflowsValidateAndParse(t *testing.T) {
	files := discoverExampleYAMLFiles(t)
	if len(files) == 0 {
		t.Fatal("no YAML files found in examples directory")
	}

	for _, filename := range files {
		t.Run(filename, func(t *testing.T) {
			content, err := os.ReadFile(filename)
			if err != nil {
				t.Fatalf("failed to read %s: %v", filename, err)
			}

			var root yaml.Node
			if err := yaml.Unmarshal(content, &root); err != nil {
				t.Fatalf("failed to parse YAML in %s: %v", filename, err)
			}

			validationResult := processor.ValidateWorkflowStructure(string(content))
			if !validationResult.Valid {
				t.Fatalf("workflow validation failed for %s:\n%s", filename, validationResult.ErrorSummary())
			}

			var config processor.DSLConfig
			if err := yaml.Unmarshal(content, &config); err != nil {
				t.Fatalf("failed to parse workflow into DSLConfig for %s: %v", filename, err)
			}
		})
	}
}

func TestExampleReferencedInputsExist(t *testing.T) {
	files := discoverExampleYAMLFiles(t)

	for _, filename := range files {
		t.Run(filename, func(t *testing.T) {
			content, err := os.ReadFile(filename)
			if err != nil {
				t.Fatalf("failed to read %s: %v", filename, err)
			}

			var root yaml.Node
			if err := yaml.Unmarshal(content, &root); err != nil {
				t.Fatalf("failed to parse YAML in %s: %v", filename, err)
			}

			if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
				t.Fatalf("file %s: expected document node at root", filename)
			}

			outputFiles := make(map[string]bool)
			collectOutputFiles(root.Content[0], outputFiles)
			validateReferencedPaths(t, filename, root.Content[0], outputFiles)
		})
	}
}

func discoverExampleYAMLFiles(t *testing.T) []string {
	t.Helper()

	var files []string
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".yaml") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to walk examples directory: %v", err)
	}

	sort.Strings(files)
	return files
}

func collectOutputFiles(node *yaml.Node, outputFiles map[string]bool) {
	if node == nil {
		return
	}

	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			collectOutputFiles(child, outputFiles)
		}
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i].Value
			value := node.Content[i+1]

			switch key {
			case "output":
				collectScalarOutputs(value, outputFiles)
			case "capture_outputs":
				collectScalarOutputs(value, outputFiles)
			}

			collectOutputFiles(value, outputFiles)
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			collectOutputFiles(child, outputFiles)
		}
	}
}

func collectScalarOutputs(node *yaml.Node, outputFiles map[string]bool) {
	switch node.Kind {
	case yaml.ScalarNode:
		outputFiles[node.Value] = true
	case yaml.SequenceNode:
		for _, item := range node.Content {
			if item.Kind == yaml.ScalarNode {
				outputFiles[item.Value] = true
			}
		}
	}
}

func validateReferencedPaths(t *testing.T, filename string, node *yaml.Node, outputFiles map[string]bool) {
	t.Helper()

	if node == nil {
		return
	}

	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			validateReferencedPaths(t, filename, child, outputFiles)
		}
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i].Value
			value := node.Content[i+1]

			switch key {
			case "input":
				validateInputValue(t, filename, value, outputFiles)
			case "workflow_file":
				if value.Kind == yaml.ScalarNode {
					validateInputPath(t, filename, value.Value, outputFiles)
				}
			}

			validateReferencedPaths(t, filename, value, outputFiles)
		}
	case yaml.SequenceNode:
		for _, child := range node.Content {
			validateReferencedPaths(t, filename, child, outputFiles)
		}
	}
}

func validateInputValue(t *testing.T, filename string, node *yaml.Node, outputFiles map[string]bool) {
	t.Helper()

	switch node.Kind {
	case yaml.ScalarNode:
		validateInputPath(t, filename, node.Value, outputFiles)
	case yaml.SequenceNode:
		for _, item := range node.Content {
			if item.Kind == yaml.ScalarNode {
				validateInputPath(t, filename, item.Value, outputFiles)
			}
		}
	}
}

func validateInputPath(t *testing.T, filename, input string, outputFiles map[string]bool) {
	t.Helper()

	if isSpecialInput(input) || outputFiles[input] || !looksLikePathReference(input) {
		return
	}

	if strings.HasPrefix(input, "filenames:") {
		files := strings.TrimPrefix(input, "filenames:")
		for _, file := range strings.Split(files, ",") {
			file = strings.TrimSpace(file)
			if file != "" {
				if outputFiles[file] || !looksLikePathReference(file) {
					continue
				}
				validateSingleInputPath(t, filename, file)
			}
		}
		return
	}

	validateSingleInputPath(t, filename, input)
}

func validateSingleInputPath(t *testing.T, filename, input string) {
	t.Helper()

	yamlDir := filepath.Dir(filename)
	paths := []string{
		input,
		filepath.Join(yamlDir, input),
	}

	if strings.HasPrefix(input, "examples/") {
		paths = append(paths, strings.TrimPrefix(input, "examples/"))
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return
		}
	}

	t.Errorf("%s references non-existent input file: %s", filename, input)
}

func isSpecialInput(input string) bool {
	specialInputs := []string{
		"STDIN",
		"screenshot",
		"NA",
	}

	for _, special := range specialInputs {
		if input == special || strings.HasPrefix(input, "STDIN as $") {
			return true
		}
	}

	if strings.HasPrefix(input, "$") {
		return true
	}

	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") {
		return true
	}

	return strings.ContainsAny(input, "*?[]") || strings.HasPrefix(input, "tool: ")
}

func looksLikePathReference(input string) bool {
	if strings.HasPrefix(input, "./") {
		return false
	}

	if strings.ContainsAny(input, "\n\r\t") {
		return false
	}

	if strings.Contains(input, "${") || strings.Contains(input, "{{") {
		return false
	}

	if strings.Contains(input, " ") {
		return false
	}

	if filepath.Ext(input) != "" {
		return true
	}

	return strings.Contains(input, "/") || strings.HasPrefix(input, ".")
}
