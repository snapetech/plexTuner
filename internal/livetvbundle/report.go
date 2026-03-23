package livetvbundle

import (
	"fmt"
	"strings"
)

// FormatMigrationAuditSummary renders a compact human-readable migration audit report.
func FormatMigrationAuditSummary(result MigrationAuditResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Migration rollout audit\n")
	fmt.Fprintf(&b, "overall_status: %s\n", defaultIfEmpty(result.Status, "unknown"))
	fmt.Fprintf(&b, "ready_to_apply: %t\n", result.ReadyToApply)
	fmt.Fprintf(&b, "targets: %d\n", len(result.Results))
	fmt.Fprintf(&b, "conflicts: %d\n", result.ConflictCount)
	for _, target := range result.Results {
		fmt.Fprintf(&b, "\n[%s] %s\n", target.Target, defaultIfEmpty(target.TargetHost, "(no host)"))
		fmt.Fprintf(&b, "status: %s\n", defaultIfEmpty(target.Status, "unknown"))
		fmt.Fprintf(&b, "ready_to_apply: %t\n", target.ReadyToApply)
		if target.StatusReason != "" {
			fmt.Fprintf(&b, "reason: %s\n", target.StatusReason)
		}
		fmt.Fprintf(&b, "live_tv_indexed_channels: %d\n", target.LiveTV.IndexedChannelCount)
		fmt.Fprintf(&b, "live_tv_conflicts: %d\n", target.LiveTV.ConflictCount)
		if target.Library != nil {
			fmt.Fprintf(&b, "library_mode: %s\n", defaultIfEmpty(target.LibraryMode, "included"))
			fmt.Fprintf(&b, "library_conflicts: %d\n", target.Library.ConflictCount)
			if len(target.MissingLibraries) > 0 {
				fmt.Fprintf(&b, "missing_libraries: %s\n", strings.Join(target.MissingLibraries, ", "))
			}
			if len(target.LaggingLibraries) > 0 {
				fmt.Fprintf(&b, "count_lagging_libraries: %s\n", strings.Join(target.LaggingLibraries, ", "))
			}
			if len(target.TitleLaggingLibraries) > 0 {
				fmt.Fprintf(&b, "title_lagging_libraries: %s\n", strings.Join(target.TitleLaggingLibraries, ", "))
				for _, line := range summarizeTitleLagDetails(target) {
					fmt.Fprintf(&b, "%s\n", line)
				}
			}
			if len(target.EmptyLibraries) > 0 {
				fmt.Fprintf(&b, "empty_libraries: %s\n", strings.Join(target.EmptyLibraries, ", "))
			}
			if target.LibraryScan != nil {
				fmt.Fprintf(&b, "library_scan: state=%s running=%t progress=%.0f%%\n",
					defaultIfEmpty(target.LibraryScan.State, "unknown"),
					target.LibraryScan.Running,
					target.LibraryScan.ProgressPercent,
				)
			}
		} else if target.LibraryMode != "" {
			fmt.Fprintf(&b, "library_mode: %s\n", target.LibraryMode)
		}
	}
	return b.String()
}

func defaultIfEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func summarizeTitleLagDetails(target MigrationTargetAudit) []string {
	if target.Library == nil {
		return nil
	}
	const maxLibraries = 3
	const maxTitles = 5
	lines := make([]string, 0, maxLibraries+1)
	totalLagging := 0
	for _, library := range target.Library.Libraries {
		if library.TitleParityStatus != "sample_missing" || len(library.MissingTitles) == 0 {
			continue
		}
		totalLagging++
		if len(lines) >= maxLibraries {
			continue
		}
		shown := library.MissingTitles
		suffix := ""
		if len(shown) > maxTitles {
			shown = shown[:maxTitles]
			suffix = fmt.Sprintf(" (+%d more)", len(library.MissingTitles)-len(shown))
		}
		lines = append(lines, fmt.Sprintf("title_missing[%s]: %s%s", library.Name, strings.Join(shown, ", "), suffix))
	}
	if totalLagging > len(lines) {
		lines = append(lines, fmt.Sprintf("title_missing_more_libraries: %d", totalLagging-len(lines)))
	}
	return lines
}
