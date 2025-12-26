package cmd

import "testing"

func TestParseVarsFlags(t *testing.T) {
	tests := []struct {
		name      string
		flags     []string
		stdinData string
		expected  map[string]string
	}{
		{
			name:      "empty flags",
			flags:     []string{},
			stdinData: "",
			expected:  map[string]string{},
		},
		{
			name:      "single variable",
			flags:     []string{"filename=/path/to/file.txt"},
			stdinData: "",
			expected:  map[string]string{"filename": "/path/to/file.txt"},
		},
		{
			name:      "multiple variables",
			flags:     []string{"key1=value1", "key2=value2"},
			stdinData: "",
			expected:  map[string]string{"key1": "value1", "key2": "value2"},
		},
		{
			name:      "STDIN variable",
			flags:     []string{"data=STDIN"},
			stdinData: "hello world",
			expected:  map[string]string{"data": "hello world"},
		},
		{
			name:      "mixed STDIN and regular variables",
			flags:     []string{"filename=/path/to/file.txt", "content=STDIN"},
			stdinData: "stdin content here",
			expected:  map[string]string{"filename": "/path/to/file.txt", "content": "stdin content here"},
		},
		{
			name:      "value with equals sign",
			flags:     []string{"query=SELECT * FROM users WHERE id=1"},
			stdinData: "",
			expected:  map[string]string{"query": "SELECT * FROM users WHERE id=1"},
		},
		{
			name:      "invalid format without equals",
			flags:     []string{"invalid"},
			stdinData: "",
			expected:  map[string]string{},
		},
		{
			name:      "empty value",
			flags:     []string{"empty="},
			stdinData: "",
			expected:  map[string]string{"empty": ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseVarsFlags(tt.flags, tt.stdinData)

			if len(result) != len(tt.expected) {
				t.Errorf("parseVarsFlags() returned %d items, want %d", len(result), len(tt.expected))
			}

			for key, expectedValue := range tt.expected {
				if actualValue, ok := result[key]; !ok {
					t.Errorf("parseVarsFlags() missing key %q", key)
				} else if actualValue != expectedValue {
					t.Errorf("parseVarsFlags()[%q] = %q, want %q", key, actualValue, expectedValue)
				}
			}
		})
	}
}
