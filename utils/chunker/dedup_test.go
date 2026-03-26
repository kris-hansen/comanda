package chunker

import (
	"os"
	"path/filepath"
	"testing"
)

func writeChunkFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test chunk: %v", err)
	}
	return path
}

func TestDeduplicateExactDuplicates(t *testing.T) {
	dir := t.TempDir()
	p1 := writeChunkFile(t, dir, "chunk_0.txt", "hello world this is a test")
	p2 := writeChunkFile(t, dir, "chunk_1.txt", "hello world this is a test")
	p3 := writeChunkFile(t, dir, "chunk_2.txt", "different content entirely")

	result := &ChunkResult{
		ChunkPaths:  []string{p1, p2, p3},
		TempDir:     dir,
		TotalChunks: 3,
	}

	deduped, err := DeduplicateChunks(result)
	if err != nil {
		t.Fatalf("DeduplicateChunks failed: %v", err)
	}

	if deduped.TotalChunks != 2 {
		t.Errorf("expected 2 chunks after dedup, got %d", deduped.TotalChunks)
	}
	if deduped.RemovedChunks != 1 {
		t.Errorf("expected 1 removed chunk, got %d", deduped.RemovedChunks)
	}
	// First occurrence should be kept
	if deduped.ChunkPaths[0] != p1 {
		t.Errorf("expected first chunk to be %s, got %s", p1, deduped.ChunkPaths[0])
	}
}

func TestDeduplicateWhitespaceNormalization(t *testing.T) {
	dir := t.TempDir()
	p1 := writeChunkFile(t, dir, "chunk_0.txt", "Hello   World\n\nTest")
	p2 := writeChunkFile(t, dir, "chunk_1.txt", "hello world\ntest")

	result := &ChunkResult{
		ChunkPaths:  []string{p1, p2},
		TempDir:     dir,
		TotalChunks: 2,
	}

	deduped, err := DeduplicateChunks(result)
	if err != nil {
		t.Fatalf("DeduplicateChunks failed: %v", err)
	}

	if deduped.TotalChunks != 1 {
		t.Errorf("expected 1 chunk after dedup (whitespace normalization), got %d", deduped.TotalChunks)
	}
}

func TestDeduplicateNoFalsePositives(t *testing.T) {
	dir := t.TempDir()
	p1 := writeChunkFile(t, dir, "chunk_0.txt", "first chunk content")
	p2 := writeChunkFile(t, dir, "chunk_1.txt", "second chunk content")
	p3 := writeChunkFile(t, dir, "chunk_2.txt", "third chunk content")

	result := &ChunkResult{
		ChunkPaths:  []string{p1, p2, p3},
		TempDir:     dir,
		TotalChunks: 3,
	}

	deduped, err := DeduplicateChunks(result)
	if err != nil {
		t.Fatalf("DeduplicateChunks failed: %v", err)
	}

	if deduped.TotalChunks != 3 {
		t.Errorf("expected all 3 unique chunks preserved, got %d", deduped.TotalChunks)
	}
	if deduped.RemovedChunks != 0 {
		t.Errorf("expected 0 removed chunks, got %d", deduped.RemovedChunks)
	}
}

func TestDeduplicateEmptyInput(t *testing.T) {
	t.Run("nil_result", func(t *testing.T) {
		deduped, err := DeduplicateChunks(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if deduped != nil {
			t.Error("expected nil result for nil input")
		}
	})

	t.Run("single_chunk", func(t *testing.T) {
		dir := t.TempDir()
		p1 := writeChunkFile(t, dir, "chunk_0.txt", "only chunk")

		result := &ChunkResult{
			ChunkPaths:  []string{p1},
			TempDir:     dir,
			TotalChunks: 1,
		}

		deduped, err := DeduplicateChunks(result)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if deduped.TotalChunks != 1 {
			t.Errorf("expected 1 chunk, got %d", deduped.TotalChunks)
		}
	})
}

func TestNormalizeForHash(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello world"},
		{"  spaces  everywhere  ", "spaces everywhere"},
		{"tabs\t\there", "tabs here"},
		{"newlines\n\nhere", "newlines here"},
		{"UPPER lower MiXeD", "upper lower mixed"},
		{"", ""},
	}

	for _, tt := range tests {
		got := normalizeForHash(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeForHash(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
