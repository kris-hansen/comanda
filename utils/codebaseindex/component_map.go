package codebaseindex

import (
	"fmt"
	"strings"
)

func (m *Manager) writeComponentMap(sb *strings.Builder, scan *ScanResult) {
	if scan == nil || len(scan.Components) == 0 {
		return
	}

	sb.WriteString("## Component Map")
	if scan.IsMonorepo {
		sb.WriteString(" (Monorepo Detected)")
	}
	sb.WriteString("\n\n")
	if scan.IsMonorepo {
		sb.WriteString("The repository appears to contain multiple logical components. Treat these as separate change surfaces before editing: identify which component owns the behavior, then inspect that component's manifest, entrypoints, tests, and neighboring files.\n\n")
	} else {
		sb.WriteString("Logical component boundaries inferred from manifests, entrypoints, and framework imports.\n\n")
	}

	for _, c := range scan.Components {
		sb.WriteString("### ")
		sb.WriteString(c.Name)
		sb.WriteString("\n\n")
		sb.WriteString(fmt.Sprintf("- **Root:** `%s`\n", c.Root))
		sb.WriteString(fmt.Sprintf("- **Role:** %s", c.Kind))
		if c.Language != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", c.Language))
		}
		sb.WriteString("\n")
		if c.FileCount > 0 {
			sb.WriteString(fmt.Sprintf("- **Indexed files in component:** %d\n", c.FileCount))
		}
		if len(c.Frameworks) > 0 {
			sb.WriteString("- **Framework/library signals:** ")
			sb.WriteString(strings.Join(c.Frameworks, ", "))
			sb.WriteString("\n")
		}
		if len(c.ConfigFiles) > 0 {
			sb.WriteString("- **Config/manifest evidence:** ")
			writeInlineCodeList(sb, c.ConfigFiles)
			sb.WriteString("\n")
		}
		if len(c.EntryPoints) > 0 {
			sb.WriteString("- **Entry points:** ")
			writeInlineCodeList(sb, c.EntryPoints)
			sb.WriteString("\n")
		}
		if len(c.KeyDirs) > 0 {
			sb.WriteString("- **Key dirs:** ")
			writeInlineCodeList(sb, c.KeyDirs)
			sb.WriteString("\n")
		}
		sb.WriteString("- **Agent guidance:** ")
		sb.WriteString(componentGuidance(c))
		sb.WriteString("\n\n")
	}
}

func componentGuidance(c *CodebaseComponent) string {
	switch c.Kind {
	case "frontend":
		return "For UI changes, start in this component's route/page/component directories and verify state/API contracts before touching backend code."
	case "backend":
		return "For API or service changes, trace from entrypoints/handlers into service/storage layers and update tests or contracts in this component."
	case "cli":
		return "For user-facing command changes, update command wiring/flags first, then delegate behavior to reusable packages and test the command path."
	case "mobile":
		return "For app changes, inspect the app entrypoint, feature screens/widgets, and platform config together; generated files should usually not be edited directly."
	case "shared-library":
		return "Treat this as shared surface area: check all importing components and keep changes backward-compatible unless callers are updated together."
	case "infrastructure":
		return "Treat changes as operationally sensitive; inspect deploy/config dependencies and prefer small, reversible patches."
	default:
		return "Inspect the manifest/config and neighboring directories before editing; the role is not confidently inferred from first-pass evidence."
	}
}

func writeInlineCodeList(sb *strings.Builder, items []string) {
	for i, item := range items {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("`")
		sb.WriteString(item)
		sb.WriteString("`")
	}
}
