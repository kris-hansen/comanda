package server

import (
	"testing"

	"github.com/kris-hansen/comanda/utils/processor"
	"gopkg.in/yaml.v3"
	"github.com/stretchr/testify/assert"
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

	// Compare each step in detail
	for i := 0; i < len(cliConfig.Steps); i++ {
		cliStep := cliConfig.Steps[i]
		serverStep := serverConfig.Steps[i]

		assert.Equal(t, cliStep.Name, serverStep.Name, 
			"Step names should match for step %d", i)
		
		// Compare StepConfig fields
		assert.Equal(t, cliStep.Config.Input, serverStep.Config.Input, 
			"Input should match for step %s", cliStep.Name)
		assert.Equal(t, cliStep.Config.Model, serverStep.Config.Model, 
			"Model should match for step %s", cliStep.Name)
		assert.Equal(t, cliStep.Config.Action, serverStep.Config.Action, 
			"Action should match for step %s", cliStep.Name)
		assert.Equal(t, cliStep.Config.Output, serverStep.Config.Output, 
			"Output should match for step %s", cliStep.Name)
		assert.Equal(t, cliStep.Config.NextAction, serverStep.Config.NextAction, 
			"NextAction should match for step %s", cliStep.Name)
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
