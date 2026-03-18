package reporter

import (
	"fmt"
	"strings"

	"check-breaking-change/internal/models"
)

// FormatMarkdown generates a markdown-formatted report of all breaking changes.
func FormatMarkdown(report models.Report) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("# Breaking Subchart Changes — `%s`\n\n", report.ChartName))
	sb.WriteString(fmt.Sprintf("**Target Branch:** `%s`\n\n", report.TargetBranch))
	sb.WriteString("---\n\n")

	for _, scr := range report.SubchartReports {
		breakingResults := filterBreaking(scr.Results)
		infoResults := filterNonBreaking(scr.Results)

		if len(breakingResults) == 0 && len(infoResults) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("## Subchart: `%s` (`%s` → `%s`)\n\n", scr.SubchartName, scr.OldVersion, scr.NewVersion))

		if len(breakingResults) > 0 {
			sb.WriteString("### ⛔ Breaking Changes\n\n")
			sb.WriteString("| Key Path | Change Type | Details |\n")
			sb.WriteString("|----------|-------------|----------|\n")
			for _, r := range breakingResults {
				sb.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", r.KeyPath, changeTypeLabel(r.Type), r.Detail))
			}
			sb.WriteString("\n")

			// Detailed diff for each breaking change
			sb.WriteString("<details>\n<summary>Detailed diff</summary>\n\n")
			for _, r := range breakingResults {
				sb.WriteString(fmt.Sprintf("#### `%s`\n\n", r.KeyPath))
				sb.WriteString("```diff\n")
				sb.WriteString(fmt.Sprintf("- Old: %s\n", formatValue(r.OldValue)))
				sb.WriteString(fmt.Sprintf("+ New: %s\n", formatValue(r.NewValue)))
				sb.WriteString("```\n\n")
			}
			sb.WriteString("</details>\n\n")
		}

		if len(infoResults) > 0 {
			sb.WriteString("### ℹ️ Informational Changes (Non-Breaking)\n\n")
			sb.WriteString("| Key Path | Change Type | Details |\n")
			sb.WriteString("|----------|-------------|----------|\n")
			for _, r := range infoResults {
				sb.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", r.KeyPath, changeTypeLabel(r.Type), r.Detail))
			}
			sb.WriteString("\n")
		}

		sb.WriteString("---\n\n")
	}

	return sb.String()
}

// FormatStdout generates a plain-text summary for dry-run / stdout output.
func FormatStdout(report models.Report) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Chart: %s (target: %s)\n", report.ChartName, report.TargetBranch))
	sb.WriteString(strings.Repeat("=", 60) + "\n\n")

	for _, scr := range report.SubchartReports {
		breakingResults := filterBreaking(scr.Results)
		infoResults := filterNonBreaking(scr.Results)

		if len(breakingResults) == 0 {
			sb.WriteString(fmt.Sprintf("✅ %s (%s → %s): No breaking changes\n", scr.SubchartName, scr.OldVersion, scr.NewVersion))
		} else {
			sb.WriteString(fmt.Sprintf("⛔ %s (%s → %s): %d breaking change(s)\n", scr.SubchartName, scr.OldVersion, scr.NewVersion, len(breakingResults)))
			for _, r := range breakingResults {
				sb.WriteString(fmt.Sprintf("   - [%s] %s: %s\n", changeTypeLabel(r.Type), r.KeyPath, r.Detail))
			}
		}

		// Show informational results (e.g. missing overrides, value changes)
		if len(infoResults) > 0 {
			sb.WriteString(fmt.Sprintf("   ℹ️  %d informational note(s):\n", len(infoResults)))
			for _, r := range infoResults {
				sb.WriteString(fmt.Sprintf("   - [%s] %s: %s\n", changeTypeLabel(r.Type), r.KeyPath, r.Detail))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func filterBreaking(results []models.DiffResult) []models.DiffResult {
	var out []models.DiffResult
	for _, r := range results {
		if r.Breaking {
			out = append(out, r)
		}
	}
	return out
}

func filterNonBreaking(results []models.DiffResult) []models.DiffResult {
	var out []models.DiffResult
	for _, r := range results {
		if !r.Breaking {
			out = append(out, r)
		}
	}
	return out
}

func changeTypeLabel(ct models.ChangeType) string {
	switch ct {
	case models.ChangeStructural:
		return "Structural"
	case models.ChangeKeyRemoved:
		return "Key Removed"
	case models.ChangeKeyAdded:
		return "Key Added"
	case models.ChangeValueOnly:
		return "Value Change"
	case models.ChangeSafeTypeConv:
		return "Type Conversion"
	case models.ChangeParentTypeMismatch:
		return "Parent Type Mismatch"
	case models.ChangeMissingOverride:
		return "Missing Override"
	case models.ChangeKeyOrphanOverride:
		return "Orphan Override"
	default:
		return "Unknown"
	}
}

func formatValue(v interface{}) string {
	if v == nil {
		return "<removed>"
	}
	if m, ok := v.(map[string]interface{}); ok {
		var parts []string
		for k, val := range m {
			parts = append(parts, fmt.Sprintf("%s: %v", k, val))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return fmt.Sprintf("%v", v)
}
