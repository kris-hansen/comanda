package codebaseindex

import (
	"strings"
	"unicode"

	"github.com/cespare/xxhash/v2"
)

// CompressAggregatedContent deduplicates sections across multiple index contents.
// It splits each content by ## markdown headers, hashes each section after
// whitespace normalization, and removes duplicate sections (keeping the first
// occurrence). This is useful when aggregating multiple codebase indexes that
// share common boilerplate sections like "Repository Layout" or "Key Entry Points".
func CompressAggregatedContent(contents []string) string {
	if len(contents) == 0 {
		return ""
	}
	if len(contents) == 1 {
		return contents[0]
	}

	seen := make(map[uint64]bool)
	var result []string

	for i, content := range contents {
		sections := splitSections(content)
		var kept []string

		for _, section := range sections {
			hash := sectionHash(section)
			if seen[hash] {
				continue
			}
			seen[hash] = true
			kept = append(kept, section)
		}

		if len(kept) > 0 {
			compressed := strings.Join(kept, "\n\n")
			result = append(result, compressed)
		} else if i == 0 {
			// Always keep at least the first index even if empty after dedup
			result = append(result, content)
		}
	}

	return strings.Join(result, "\n\n---\n\n")
}

// splitSections splits markdown content by ## headers. Each section includes
// its header line and all content until the next ## header.
func splitSections(content string) []string {
	lines := strings.Split(content, "\n")
	var sections []string
	var current []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") && len(current) > 0 {
			sections = append(sections, strings.Join(current, "\n"))
			current = nil
		}
		current = append(current, line)
	}

	if len(current) > 0 {
		sections = append(sections, strings.Join(current, "\n"))
	}

	return sections
}

// sectionHash computes an xxhash of a section's normalized text content.
// Normalization collapses whitespace and lowercases to catch sections that
// differ only in formatting.
func sectionHash(section string) uint64 {
	normalized := normalizeSectionText(section)
	return xxhash.Sum64String(normalized)
}

// normalizeSectionText collapses whitespace and lowercases for dedup comparison.
func normalizeSectionText(text string) string {
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
