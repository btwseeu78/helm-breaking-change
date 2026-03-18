package reporter

import (
	"fmt"
	"strings"

	"check-breaking-change/internal/models"
)

// FormatStdout generates a plain-text summary for stdout output.
func FormatStdout(report models.Report) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Chart: %s\n", report.ChartName))
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

