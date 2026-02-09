package filescan

import (
	"fmt"
	"sort"
	"strings"
)

// Manifest generates a human-readable token budget manifest
func (r *ScanResult) Manifest() string {
	var sb strings.Builder

	sb.WriteString("## 📁 File Manifest (Token Budget)\n\n")
	sb.WriteString(fmt.Sprintf("**Total:** %d files, ~%dk tokens estimated\n\n", r.TotalFiles, r.TotalTokens/1000))

	// Get categorized files
	oversized := r.FilterByCategory("oversized")
	large := r.FilterByCategory("large")

	// Sort by token count descending
	sort.Slice(oversized, func(i, j int) bool {
		return oversized[i].EstimatedTokens > oversized[j].EstimatedTokens
	})
	sort.Slice(large, func(i, j int) bool {
		return large[i].EstimatedTokens > large[j].EstimatedTokens
	})

	if len(oversized) > 0 {
		sb.WriteString("### ❌ Oversized (>25k tokens) — DO NOT read directly\n")
		sb.WriteString("Use `grep` to search or read with `offset`/`limit` parameters.\n\n")
		for _, f := range oversized {
			sb.WriteString(fmt.Sprintf("- `%s` (~%dk tokens, %d bytes)\n", f.RelPath, f.EstimatedTokens/1000, f.Size))
		}
		sb.WriteString("\n")
	}

	if len(large) > 0 {
		sb.WriteString("### ⚠️ Large (10k-25k tokens) — Read with care\n")
		sb.WriteString("Consider if you need the full file or just specific sections.\n\n")
		for _, f := range large {
			sb.WriteString(fmt.Sprintf("- `%s` (~%dk tokens)\n", f.RelPath, f.EstimatedTokens/1000))
		}
		sb.WriteString("\n")
	}

	if len(oversized) > 0 || len(large) > 0 {
		sb.WriteString("### ✅ Safe files\n")
		sb.WriteString(fmt.Sprintf("%d files under 10k tokens — safe to read fully.\n\n", r.SafeCount))
	}

	return sb.String()
}

// MarkdownSection generates a markdown section for embedding in codebase index
func (r *ScanResult) MarkdownSection() string {
	oversized := r.FilterByCategory("oversized")
	large := r.FilterByCategory("large")

	if len(oversized) == 0 && len(large) == 0 {
		return ""
	}

	var sb strings.Builder

	sb.WriteString("## ⚠️ Token Budget\n\n")
	sb.WriteString("Some files exceed recommended token limits for single reads.\n\n")

	if len(oversized) > 0 {
		sb.WriteString("### ❌ Oversized (>25k tokens) — Use grep or chunked reads\n\n")
		for _, f := range oversized {
			sb.WriteString(fmt.Sprintf("- `%s` (~%dk tokens)\n", f.RelPath, f.EstimatedTokens/1000))
		}
		sb.WriteString("\n")
	}

	if len(large) > 0 {
		sb.WriteString("### ⚡ Large (10k-25k tokens) — Read with care\n\n")
		for _, f := range large {
			sb.WriteString(fmt.Sprintf("- `%s` (~%dk tokens)\n", f.RelPath, f.EstimatedTokens/1000))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("**Tip:** For oversized files, use `grep` to find relevant sections, or read with `offset` and `limit` parameters.\n\n")

	return sb.String()
}
