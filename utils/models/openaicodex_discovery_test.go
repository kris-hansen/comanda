package models

import "testing"

func TestNormalizeOpenAICodexModels(t *testing.T) {
	got := normalizeOpenAICodexModels([]string{"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-sol", "", " gpt-5.4 "})
	want := []string{
		"openai-codex",
		"openai-codex-gpt-5.6-sol",
		"openai-codex-gpt-5.6-terra",
		"openai-codex-gpt-5.4",
	}
	if len(got) != len(want) {
		t.Fatalf("normalizeOpenAICodexModels() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalizeOpenAICodexModels()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
