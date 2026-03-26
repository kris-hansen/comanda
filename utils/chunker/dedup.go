package chunker

import (
	"os"
	"strings"
	"unicode"

	"github.com/cespare/xxhash/v2"
)

// DeduplicateChunks removes near-duplicate chunks from a ChunkResult.
// It hashes each chunk's normalized text content using xxhash and removes
// chunks whose hash has already been seen, keeping the first occurrence.
// Returns a new ChunkResult with duplicates filtered out.
func DeduplicateChunks(result *ChunkResult) (*ChunkResult, error) {
	if result == nil || len(result.ChunkPaths) <= 1 {
		return result, nil
	}

	seen := make(map[uint64]bool)
	var filteredPaths []string
	removedCount := 0

	for _, path := range result.ChunkPaths {
		hash, err := computeChunkHash(path)
		if err != nil {
			// If we can't read a chunk, keep it rather than silently dropping
			filteredPaths = append(filteredPaths, path)
			continue
		}

		if seen[hash] {
			removedCount++
			continue
		}

		seen[hash] = true
		filteredPaths = append(filteredPaths, path)
	}

	return &ChunkResult{
		ChunkPaths:    filteredPaths,
		TempDir:       result.TempDir,
		TotalChunks:   len(filteredPaths),
		RemovedChunks: removedCount,
	}, nil
}

// computeChunkHash reads a chunk file, normalizes its text, and returns
// an xxhash digest.
func computeChunkHash(path string) (uint64, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	normalized := normalizeForHash(string(content))
	return xxhash.Sum64String(normalized), nil
}

// normalizeForHash collapses whitespace and lowercases text so that
// chunks differing only in formatting are treated as duplicates.
func normalizeForHash(text string) string {
	// Collapse all whitespace runs to a single space
	var b strings.Builder
	b.Grow(len(text))
	inSpace := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			if !inSpace {
				b.WriteByte(' ')
				inSpace = true
			}
		} else {
			b.WriteRune(unicode.ToLower(r))
			inSpace = false
		}
	}
	return strings.TrimSpace(b.String())
}
