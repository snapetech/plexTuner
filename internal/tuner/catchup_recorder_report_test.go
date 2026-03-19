package tuner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCatchupRecorderReport(t *testing.T) {
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "recorder-state.json")
	state := CatchupRecorderState{
		UpdatedAt: "2026-03-19T18:00:00Z",
		RootDir:   dir,
		Statistics: CatchupRecorderStatistics{
			ActiveCount:    1,
			CompletedCount: 2,
			FailedCount:    1,
		},
		Active: []CatchupRecorderItem{{
			CapsuleID: "active-1",
			Lane:      "sports",
			Title:     "Live Game",
		}},
		Completed: []CatchupRecorderItem{
			{CapsuleID: "done-1", Lane: "sports", Title: "Sports Done", PublishedPath: filepath.Join(dir, "sports", "done.ts")},
			{CapsuleID: "done-2", Lane: "general", Title: "General Done"},
		},
		Failed: []CatchupRecorderItem{{
			CapsuleID:      "fail-1",
			Lane:           "general",
			Title:          "General Fail",
			Status:         "interrupted",
			RecoveryReason: "daemon_restart",
		}},
	}
	data, _ := json.MarshalIndent(state, "", "  ")
	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
		t.Fatalf("write state: %v", err)
	}
	report, err := LoadCatchupRecorderReport(stateFile, 1)
	if err != nil {
		t.Fatalf("LoadCatchupRecorderReport: %v", err)
	}
	if report.StateFile != stateFile {
		t.Fatalf("state_file=%q want %q", report.StateFile, stateFile)
	}
	if report.PublishedCount != 1 {
		t.Fatalf("published_count=%d want 1", report.PublishedCount)
	}
	if report.InterruptedCount != 1 {
		t.Fatalf("interrupted_count=%d want 1", report.InterruptedCount)
	}
	if len(report.Active) != 1 || len(report.Completed) != 1 || len(report.Failed) != 1 {
		t.Fatalf("unexpected truncated lengths: active=%d completed=%d failed=%d", len(report.Active), len(report.Completed), len(report.Failed))
	}
	if len(report.Lanes) != 2 {
		t.Fatalf("lanes=%d want 2", len(report.Lanes))
	}
	if report.Lanes[0].Lane != "general" || report.Lanes[1].Lane != "sports" {
		t.Fatalf("lane order=%+v", report.Lanes)
	}
}
