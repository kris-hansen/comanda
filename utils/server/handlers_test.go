package server

import (
	"testing"

	"github.com/kris-hansen/comanda/utils/processor"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestYAMLParsingParity(t *testing.T) {
	// Sample YAML that uses STDIN input (similar to stdin-example.yaml)
	yamlContent := []byte(`
analyze_text:
  input: STDIN
  model: gpt-4o
  action: "Analyze the following text and provide key insights:"
  output: STDOUT

summarize:
  input: STDIN
  model: gpt-4o-mini
  action: "Summarize the analysis in 3 bullet points:"
  output: STDOUT
`)

	// CLI-style parsing (from cmd/process.go)
	var cliRawConfig map[string]processor.StepConfig
	err := yaml.Unmarshal(yamlContent, &cliRawConfig)
	assert.NoError(t, err, "CLI parsing should not error")

	var cliConfig processor.DSLConfig
	for name, config := range cliRawConfig {
		cliConfig.Steps = append(cliConfig.Steps, processor.Step{
			Name:   name,
			Config: config,
		})
	}

	// Server-style parsing (from utils/server/handlers.go)
	var serverRawConfig map[string]processor.StepConfig
	err = yaml.Unmarshal(yamlContent, &serverRawConfig)
	assert.NoError(t, err, "Server parsing should not error")

	var serverConfig processor.DSLConfig
	for name, config := range serverRawConfig {
		serverConfig.Steps = append(serverConfig.Steps, processor.Step{
			Name:   name,
			Config: config,
		})
	}

	// Verify both methods produce identical results
	assert.Equal(t, len(cliConfig.Steps), len(serverConfig.Steps),
		"CLI and server should parse the same number of steps")

	// Create maps for easier comparison since order isn't guaranteed
	cliSteps := make(map[string]processor.Step)
	serverSteps := make(map[string]processor.Step)

	for _, step := range cliConfig.Steps {
		cliSteps[step.Name] = step
	}
	for _, step := range serverConfig.Steps {
		serverSteps[step.Name] = step
	}

	// Compare steps by name
	for name, cliStep := range cliSteps {
		serverStep, exists := serverSteps[name]
		assert.True(t, exists, "Step %s should exist in both configs", name)

		// Compare StepConfig fields
		assert.Equal(t, cliStep.Config.Input, serverStep.Config.Input,
			"Input should match for step %s", name)
		assert.Equal(t, cliStep.Config.Model, serverStep.Config.Model,
			"Model should match for step %s", name)
		assert.Equal(t, cliStep.Config.Action, serverStep.Config.Action,
			"Action should match for step %s", name)
		assert.Equal(t, cliStep.Config.Output, serverStep.Config.Output,
			"Output should match for step %s", name)
		assert.Equal(t, cliStep.Config.NextAction, serverStep.Config.NextAction,
			"NextAction should match for step %s", name)
	}

	// Verify both configs can be processed
	cliProc := processor.NewProcessor(&cliConfig, nil, true)
	assert.NotNil(t, cliProc, "CLI processor should be created successfully")

	serverProc := processor.NewProcessor(&serverConfig, nil, true)
	assert.NotNil(t, serverProc, "Server processor should be created successfully")
}

// Test that direct DSLConfig parsing fails for our YAML format
func TestDirectDSLConfigParsing(t *testing.T) {
	yamlContent := []byte(`
analyze_text:
  input: STDIN
  model: gpt-4o
  action: "Test action"
  output: STDOUT
`)

	// Try parsing directly into DSLConfig (the old way that caused the bug)
	var dslConfig processor.DSLConfig
	err := yaml.Unmarshal(yamlContent, &dslConfig)

	// This should result in a DSLConfig with no steps
	assert.NoError(t, err, "Parsing should not error")
	assert.Empty(t, dslConfig.Steps,
		"Direct parsing into DSLConfig should result in no steps due to YAML structure mismatch")
}
