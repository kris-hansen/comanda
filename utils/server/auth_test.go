package server

import (
	"testing"
)

func TestHasStdinInput(t *testing.T) {
	tests := []struct {
		name     string
		yamlStr  string
		wantPost bool
	}{
		{
			name: "direct stdin input",
			yamlStr: `analyze_text:
  input: STDIN
  model: gpt-4
  action: "analyze this"
  output: STDOUT`,
			wantPost: true,
		},
		{
			name: "array with stdin first",
			yamlStr: `analyze_text:
  input: 
    - STDIN
    - file.txt
  model: gpt-4
  action: "analyze this"
  output: STDOUT`,
			wantPost: true,
		},
		{
			name: "array with stdin not first",
			yamlStr: `analyze_text:
  input: 
    - file.txt
    - STDIN
  model: gpt-4
  action: "analyze this"
  output: STDOUT`,
			wantPost: true,
		},
		{
			name: "no stdin input",
			yamlStr: `analyze_text:
  input: file.txt
  model: gpt-4
  action: "analyze this"
  output: STDOUT`,
			wantPost: false,
		},
		{
			name: "map input type",
			yamlStr: `analyze_text:
  input:
    database:
      type: postgres
      query: "SELECT * FROM users"
  model: gpt-4
  action: "analyze this"
  output: STDOUT`,
			wantPost: false,
		},
		{
			name: "multiple steps with stdin in first step",
			yamlStr: `step1:
  input: STDIN
  model: gpt-4
  action: "first action"
  output: STDOUT
step2:
  input: file.txt
  model: gpt-4
  action: "second action"
  output: STDOUT`,
			wantPost: true,
		},
		{
			name: "multiple steps with stdin in later step",
			yamlStr: `step1:
  input: file.txt
  model: gpt-4
  action: "first action"
  output: STDOUT
step2:
  input: STDIN
  model: gpt-4
  action: "second action"
  output: STDOUT`,
			wantPost: true,
		},
		{
			name: "case insensitive stdin",
			yamlStr: `analyze_text:
  input: stdin
  model: gpt-4
  action: "analyze this"
  output: STDOUT`,
			wantPost: true,
		},
		{
			name: "stdin with variable assignment",
			yamlStr: `analyze_text:
  input: "STDIN as $var"
  model: gpt-4
  action: "analyze this"
  output: STDOUT`,
			wantPost: true,
		},
		{
			name: "complex yaml with multi-line action",
			yamlStr: `analyze_text:
  input: STDIN
  model: gpt-4
  action: |
    As a code review expert, analyze the following code and provide:
    1) Potential security vulnerabilities
    2) Performance optimization opportunities
    3) Code quality improvements
    4) A risk assessment score from 0-100
    
    Here is the code to analyze:
  output: STDOUT`,
			wantPost: true,
		},
		{
			name: "exact match of production yaml",
			yamlStr: `analyze_text:
  input: STDIN  # This makes the YAML eligible for POST requests
  model: gpt-4o
  action: "As a cybersecurity IAM expert, assess the following role and service account assignments and provide the following: 1) an overview of any risks associated with the current settings 2) anything which stands out as non standard or unusual 3) an overall risk rating score out of 100. Here are the roles in JSON format:"
  output: STDOUT`,
			wantPost: true,
		},
		{
			name: "yaml with comments before input",
			yamlStr: `# This is a test YAML file
# It uses STDIN for input
analyze_text:
  input: STDIN  # This makes it POST-only
  model: gpt-4
  action: "analyze this"
  output: STDOUT`,
			wantPost: true,
		},
		{
			name: "yaml with unusual formatting",
			yamlStr: `
# Unusual formatting test
analyze_text:
    input:    STDIN   # Lots of spaces
    model:    gpt-4
    action:   "test"
    output:   STDOUT`,
			wantPost: true,
		},
		{
			name: "yaml with special characters in comments",
			yamlStr: `analyze_text:
  input: STDIN  # Special chars: @#$%^&*()
  model: gpt-4  # More special chars: !@#$
  action: "test"
  output: STDOUT`,
			wantPost: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasStdinInput([]byte(tt.yamlStr))
			if got != tt.wantPost {
				t.Errorf("hasStdinInput() = %v, want %v\nYAML:\n%s", got, tt.wantPost, tt.yamlStr)
			}
		})
	}
}
