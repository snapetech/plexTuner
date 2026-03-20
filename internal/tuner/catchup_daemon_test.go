package tuner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunCatchupRecorderDaemon_OnceRecordsInProgressCapsule(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ts-data-daemon"))
	}))
	defer srv.Close()

	now := time.Now().UTC()
	dir := t.TempDir()
	state, err := RunCatchupRecorderDaemon(context.Background(), CatchupRecorderDaemonConfig{
		StreamBaseURL:  srv.URL,
		OutDir:         dir,
		PollInterval:   10 * time.Millisecond,
		MaxConcurrency: 2,
		Once:           true,
		Now:            func() time.Time { return now },
	}, func(time.Time) (CatchupCapsulePreview, error) {
		return CatchupCapsulePreview{
			Capsules: []CatchupCapsule{
				{
					CapsuleID:   "dna:test:202603191800:live-game",
					DNAID:       "dna:test",
					ChannelID:   "101",
					GuideNumber: "101",
					ChannelName: "SportsNet",
					Title:       "Live Game",
					Lane:        "sports",
					State:       "in_progress",
					Start:       now.Add(-5 * time.Minute).Format(time.RFC3339),
					Stop:        now.Add(5 * time.Minute).Format(time.RFC3339),
				},
			},
		}, nil
	}, srv.Client())
	if err != nil {
		t.Fatalf("RunCatchupRecorderDaemon: %v", err)
	}
	if got := state.Statistics.CompletedCount; got != 1 {
		t.Fatalf("completed=%d want 1", got)
	}
	if len(state.Completed) != 1 {
		t.Fatalf("completed len=%d want 1", len(state.Completed))
	}
	data, err := os.ReadFile(state.Completed[0].OutputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != "ts-data-daemon" {
		t.Fatalf("data=%q want ts-data-daemon", string(data))
	}
	stateData, err := os.ReadFile(filepath.Join(dir, "recorder-state.json"))
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	var parsed CatchupRecorderState
	if err := json.Unmarshal(stateData, &parsed); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	if parsed.Statistics.CompletedCount != 1 {
		t.Fatalf("state completed=%d want 1", parsed.Statistics.CompletedCount)
	}
}

func TestRunCatchupRecorderDaemon_PublishesCompletedRecording(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ts-data-daemon"))
	}))
	defer srv.Close()

	now := time.Now().UTC()
	dir := t.TempDir()
	publishDir := filepath.Join(dir, "published")
	state, err := RunCatchupRecorderDaemon(context.Background(), CatchupRecorderDaemonConfig{
		StreamBaseURL:  srv.URL,
		OutDir:         filepath.Join(dir, "recordings"),
		PublishDir:     publishDir,
		PollInterval:   10 * time.Millisecond,
		MaxConcurrency: 1,
		Once:           true,
		Now:            func() time.Time { return now },
	}, func(time.Time) (CatchupCapsulePreview, error) {
		return CatchupCapsulePreview{
			Capsules: []CatchupCapsule{{
				CapsuleID:   "dna:test:publish",
				ChannelID:   "101",
				ChannelName: "SportsNet",
				Title:       "Live Game",
				Lane:        "sports",
				State:       "in_progress",
				Start:       now.Add(-time.Minute).Format(time.RFC3339),
				Stop:        now.Add(time.Minute).Format(time.RFC3339),
			}},
		}, nil
	}, srv.Client())
	if err != nil {
		t.Fatalf("RunCatchupRecorderDaemon: %v", err)
	}
	if len(state.Completed) != 1 {
		t.Fatalf("completed len=%d want 1", len(state.Completed))
	}
	item := state.Completed[0]
	if item.PublishedPath == "" || item.NFOPath == "" {
		t.Fatalf("expected published paths on completed item: %+v", item)
	}
	if _, err := os.Stat(item.PublishedPath); err != nil {
		t.Fatalf("stat published path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(publishDir, "recorded-publish-manifest.json")); err != nil {
		t.Fatalf("stat publish manifest: %v", err)
	}
}

func TestRunCatchupRecorderDaemon_OnPublishedHookErrorMarksCompletedItem(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ts-data-daemon"))
	}))
	defer srv.Close()

	now := time.Now().UTC()
	dir := t.TempDir()
	publishDir := filepath.Join(dir, "published")
	hookCalls := 0
	state, err := RunCatchupRecorderDaemon(context.Background(), CatchupRecorderDaemonConfig{
		StreamBaseURL:  srv.URL,
		OutDir:         filepath.Join(dir, "recordings"),
		PublishDir:     publishDir,
		PollInterval:   10 * time.Millisecond,
		MaxConcurrency: 1,
		OnPublished: func(item CatchupRecordedPublishedItem) error {
			hookCalls++
			if item.MediaPath == "" {
				t.Fatalf("expected published media path in hook: %+v", item)
			}
			return os.ErrPermission
		},
		Once: true,
		Now:  func() time.Time { return now },
	}, func(time.Time) (CatchupCapsulePreview, error) {
		return CatchupCapsulePreview{
			Capsules: []CatchupCapsule{{
				CapsuleID:   "dna:test:hook",
				ChannelID:   "101",
				ChannelName: "SportsNet",
				Title:       "Live Game",
				Lane:        "sports",
				State:       "in_progress",
				Start:       now.Add(-time.Minute).Format(time.RFC3339),
				Stop:        now.Add(time.Minute).Format(time.RFC3339),
			}},
		}, nil
	}, srv.Client())
	if err != nil {
		t.Fatalf("RunCatchupRecorderDaemon: %v", err)
	}
	if hookCalls != 1 {
		t.Fatalf("hookCalls=%d want 1", hookCalls)
	}
	if len(state.Completed) != 1 {
		t.Fatalf("completed len=%d want 1", len(state.Completed))
	}
	if state.Completed[0].Error == "" {
		t.Fatalf("expected completed item to retain hook error: %+v", state.Completed[0])
	}
}

func TestPruneCatchupRecorderCompletedMaxAge(t *testing.T) {
	now := time.Now().UTC()
	items := []CatchupRecorderItem{
		{CapsuleID: "old", StoppedAt: now.Add(-3 * time.Hour).Format(time.RFC3339), Lane: "sports"},
		{CapsuleID: "new", StoppedAt: now.Add(-time.Hour).Format(time.RFC3339), Lane: "sports"},
	}
	out := pruneCatchupRecorderCompletedMaxAge(items, 2*time.Hour, nil, now)
	if len(out) != 1 || out[0].CapsuleID != "new" {
		t.Fatalf("got %+v", out)
	}
	laneItems := []CatchupRecorderItem{
		{CapsuleID: "old", StoppedAt: now.Add(-3 * time.Hour).Format(time.RFC3339), Lane: "sports"},
		{CapsuleID: "new", StoppedAt: now.Add(-time.Hour).Format(time.RFC3339), Lane: "sports"},
	}
	out2 := pruneCatchupRecorderCompletedMaxAge(laneItems, 0, map[string]time.Duration{"sports": 2 * time.Hour}, now)
	if len(out2) != 1 || out2[0].CapsuleID != "new" {
		t.Fatalf("lane got %+v", out2)
	}
}

func TestRunCatchupRecorderDaemon_PrunesExpiredCompleted(t *testing.T) {
	now := time.Now().UTC()
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "recorder-state.json")
	expiredTS := filepath.Join(dir, "old.ts")
	if err := os.WriteFile(expiredTS, []byte("old"), 0o600); err != nil {
		t.Fatalf("write old ts: %v", err)
	}
	initial := CatchupRecorderState{
		UpdatedAt: now.Add(-time.Hour).Format(time.RFC3339),
		RootDir:   dir,
		Completed: []CatchupRecorderItem{{
			CapsuleID:  "expired",
			Title:      "Expired",
			Status:     "completed",
			OutputPath: expiredTS,
			ExpiresAt:  now.Add(-time.Minute).Format(time.RFC3339),
		}},
	}
	data, _ := json.MarshalIndent(initial, "", "  ")
	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
		t.Fatalf("write state file: %v", err)
	}
	state, err := RunCatchupRecorderDaemon(context.Background(), CatchupRecorderDaemonConfig{
		StreamBaseURL: "http://127.0.0.1:59999",
		OutDir:        dir,
		StateFile:     stateFile,
		Once:          true,
		Now:           func() time.Time { return now },
	}, func(time.Time) (CatchupCapsulePreview, error) {
		return CatchupCapsulePreview{}, nil
	}, nil)
	if err != nil {
		t.Fatalf("RunCatchupRecorderDaemon: %v", err)
	}
	if state.Statistics.CompletedCount != 0 {
		t.Fatalf("completed=%d want 0", state.Statistics.CompletedCount)
	}
	if _, err := os.Stat(expiredTS); !os.IsNotExist(err) {
		t.Fatalf("expected expired TS to be pruned, stat err=%v", err)
	}
}

func TestRunCatchupRecorderDaemon_LaneFilterSkipsNonIncluded(t *testing.T) {
	now := time.Now().UTC()
	dir := t.TempDir()
	state, err := RunCatchupRecorderDaemon(context.Background(), CatchupRecorderDaemonConfig{
		StreamBaseURL:  "http://127.0.0.1:59999",
		OutDir:         dir,
		MaxConcurrency: 1,
		IncludeLanes:   []string{"sports"},
		Once:           true,
		Now:            func() time.Time { return now },
	}, func(time.Time) (CatchupCapsulePreview, error) {
		return CatchupCapsulePreview{
			Capsules: []CatchupCapsule{
				{
					CapsuleID: "capsule-general",
					ChannelID: "201",
					Title:     "Talk Show",
					Lane:      "general",
					State:     "in_progress",
					Start:     now.Add(-time.Minute).Format(time.RFC3339),
					Stop:      now.Add(time.Minute).Format(time.RFC3339),
				},
			},
		}, nil
	}, nil)
	if err != nil {
		t.Fatalf("RunCatchupRecorderDaemon: %v", err)
	}
	if state.Statistics.CompletedCount != 0 || state.Statistics.FailedCount != 0 || state.Statistics.ActiveCount != 0 {
		t.Fatalf("unexpected stats: %+v", state.Statistics)
	}
}

func TestRunCatchupRecorderDaemon_ChannelFilterMatchesGuideNumberAndDNAID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ts-data-daemon"))
	}))
	defer srv.Close()

	now := time.Now().UTC()
	dir := t.TempDir()
	state, err := RunCatchupRecorderDaemon(context.Background(), CatchupRecorderDaemonConfig{
		StreamBaseURL:   srv.URL,
		OutDir:          dir,
		MaxConcurrency:  1,
		IncludeChannels: []string{"101", "dna:target"},
		ExcludeChannels: []string{"Skip Me"},
		Once:            true,
		Now:             func() time.Time { return now },
	}, func(time.Time) (CatchupCapsulePreview, error) {
		return CatchupCapsulePreview{
			Capsules: []CatchupCapsule{
				{
					CapsuleID:   "capsule-allow-guide",
					ChannelID:   "201",
					GuideNumber: "101",
					ChannelName: "SportsNet",
					Title:       "Allowed by guide number",
					Lane:        "sports",
					State:       "in_progress",
					Start:       now.Add(-time.Minute).Format(time.RFC3339),
					Stop:        now.Add(time.Minute).Format(time.RFC3339),
				},
				{
					CapsuleID:   "capsule-allow-dna",
					DNAID:       "dna:target",
					ChannelID:   "202",
					GuideNumber: "202",
					ChannelName: "Target",
					Title:       "Allowed by DNA",
					Lane:        "general",
					State:       "in_progress",
					Start:       now.Add(-time.Minute).Format(time.RFC3339),
					Stop:        now.Add(time.Minute).Format(time.RFC3339),
				},
				{
					CapsuleID:   "capsule-block-name",
					DNAID:       "dna:target",
					ChannelID:   "203",
					GuideNumber: "203",
					ChannelName: "Skip Me",
					Title:       "Blocked by name",
					Lane:        "general",
					State:       "in_progress",
					Start:       now.Add(-time.Minute).Format(time.RFC3339),
					Stop:        now.Add(time.Minute).Format(time.RFC3339),
				},
			},
		}, nil
	}, srv.Client())
	if err != nil {
		t.Fatalf("RunCatchupRecorderDaemon: %v", err)
	}
	if got := state.Statistics.CompletedCount; got != 2 {
		t.Fatalf("completed=%d want 2", got)
	}
}

func TestRunCatchupRecorderDaemon_SuppressesDuplicateRecordKeyAcrossCapsules(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ts-data-daemon"))
	}))
	defer srv.Close()

	now := time.Now().UTC()
	dir := t.TempDir()
	state, err := RunCatchupRecorderDaemon(context.Background(), CatchupRecorderDaemonConfig{
		StreamBaseURL:  srv.URL,
		OutDir:         dir,
		MaxConcurrency: 2,
		Once:           true,
		Now:            func() time.Time { return now },
	}, func(time.Time) (CatchupCapsulePreview, error) {
		start := now.Add(-time.Minute).Format(time.RFC3339)
		stop := now.Add(time.Minute).Format(time.RFC3339)
		return CatchupCapsulePreview{
			Capsules: []CatchupCapsule{
				{
					CapsuleID:   "capsule-primary",
					DNAID:       "dna:shared",
					ChannelID:   "301",
					GuideNumber: "301",
					ChannelName: "Primary",
					Title:       "Shared Show",
					Lane:        "general",
					State:       "in_progress",
					Start:       start,
					Stop:        stop,
				},
				{
					CapsuleID:   "capsule-backup",
					DNAID:       "dna:shared",
					ChannelID:   "302",
					GuideNumber: "302",
					ChannelName: "Backup",
					Title:       "Shared Show",
					Lane:        "general",
					State:       "in_progress",
					Start:       start,
					Stop:        stop,
				},
			},
		}, nil
	}, srv.Client())
	if err != nil {
		t.Fatalf("RunCatchupRecorderDaemon: %v", err)
	}
	if got := state.Statistics.CompletedCount; got != 1 {
		t.Fatalf("completed=%d want 1", got)
	}
	if state.Completed[0].RecordKey == "" {
		t.Fatalf("expected record key on completed item: %+v", state.Completed[0])
	}
}

func TestRunCatchupRecorderDaemon_PrunesPerLaneRetentionAndAllowsFutureRerecord(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ts-data-daemon"))
	}))
	defer srv.Close()

	now := time.Now().UTC()
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "recorder-state.json")
	oldPath := filepath.Join(dir, "sports", "old.ts")
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(oldPath, []byte("old"), 0o600); err != nil {
		t.Fatalf("write old ts: %v", err)
	}
	initial := CatchupRecorderState{
		UpdatedAt: now.Add(-time.Hour).Format(time.RFC3339),
		RootDir:   dir,
		Completed: []CatchupRecorderItem{
			{
				CapsuleID:  "newest",
				RecordKey:  "dna:shared|" + now.Add(-2*time.Minute).Format(time.RFC3339) + "|shared-show",
				DNAID:      "dna:shared",
				ChannelID:  "101",
				Title:      "Shared Show",
				Lane:       "sports",
				Start:      now.Add(-2 * time.Minute).Format(time.RFC3339),
				OutputPath: filepath.Join(dir, "sports", "newest.ts"),
			},
			{
				CapsuleID:  "oldest",
				RecordKey:  "dna:shared|" + now.Add(-10*time.Minute).Format(time.RFC3339) + "|shared-show",
				DNAID:      "dna:shared",
				ChannelID:  "101",
				Title:      "Shared Show",
				Lane:       "sports",
				Start:      now.Add(-10 * time.Minute).Format(time.RFC3339),
				OutputPath: oldPath,
			},
		},
	}
	data, _ := json.MarshalIndent(initial, "", "  ")
	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
		t.Fatalf("write state file: %v", err)
	}
	state, err := RunCatchupRecorderDaemon(context.Background(), CatchupRecorderDaemonConfig{
		StreamBaseURL:       srv.URL,
		OutDir:              dir,
		StateFile:           stateFile,
		RetainCompleted:     10,
		LaneRetainCompleted: map[string]int{"sports": 1},
		Once:                true,
		Now:                 func() time.Time { return now },
	}, func(time.Time) (CatchupCapsulePreview, error) {
		return CatchupCapsulePreview{
			Capsules: []CatchupCapsule{{
				CapsuleID:   "fresh",
				DNAID:       "dna:shared",
				ChannelID:   "101",
				GuideNumber: "101",
				ChannelName: "SportsNet",
				Title:       "Shared Show",
				Lane:        "sports",
				State:       "in_progress",
				Start:       now.Add(-time.Minute).Format(time.RFC3339),
				Stop:        now.Add(time.Minute).Format(time.RFC3339),
			}},
		}, nil
	}, srv.Client())
	if err != nil {
		t.Fatalf("RunCatchupRecorderDaemon: %v", err)
	}
	if got := state.Statistics.CompletedCount; got != 1 {
		t.Fatalf("completed=%d want 1", got)
	}
	if state.Completed[0].CapsuleID != "fresh" {
		t.Fatalf("completed item=%q want fresh", state.Completed[0].CapsuleID)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected oldest path to be pruned, stat err=%v", err)
	}
}

func TestRunCatchupRecorderDaemon_PrunesPerLaneBudgetBytes(t *testing.T) {
	now := time.Now().UTC()
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "recorder-state.json")
	keepPath := filepath.Join(dir, "movies", "keep.ts")
	dropPath := filepath.Join(dir, "movies", "drop.ts")
	if err := os.MkdirAll(filepath.Dir(keepPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(keepPath, []byte("12345"), 0o600); err != nil {
		t.Fatalf("write keep: %v", err)
	}
	if err := os.WriteFile(dropPath, []byte("67890"), 0o600); err != nil {
		t.Fatalf("write drop: %v", err)
	}
	initial := CatchupRecorderState{
		UpdatedAt: now.Add(-time.Hour).Format(time.RFC3339),
		RootDir:   dir,
		Completed: []CatchupRecorderItem{
			{CapsuleID: "keep", Lane: "movies", Title: "Keep", OutputPath: keepPath, BytesRecorded: 5},
			{CapsuleID: "drop", Lane: "movies", Title: "Drop", OutputPath: dropPath, BytesRecorded: 5},
		},
	}
	data, _ := json.MarshalIndent(initial, "", "  ")
	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
		t.Fatalf("write state file: %v", err)
	}
	state, err := RunCatchupRecorderDaemon(context.Background(), CatchupRecorderDaemonConfig{
		StreamBaseURL:   "http://127.0.0.1:59999",
		OutDir:          dir,
		StateFile:       stateFile,
		LaneBudgetBytes: map[string]int64{"movies": 8},
		Once:            true,
		Now:             func() time.Time { return now },
	}, func(time.Time) (CatchupCapsulePreview, error) {
		return CatchupCapsulePreview{}, nil
	}, nil)
	if err != nil {
		t.Fatalf("RunCatchupRecorderDaemon: %v", err)
	}
	if got := state.Statistics.CompletedCount; got != 1 {
		t.Fatalf("completed=%d want 1", got)
	}
	if state.Completed[0].CapsuleID != "keep" {
		t.Fatalf("kept=%q want keep", state.Completed[0].CapsuleID)
	}
	if _, err := os.Stat(dropPath); !os.IsNotExist(err) {
		t.Fatalf("expected drop path to be pruned, stat err=%v", err)
	}
}

func TestRunCatchupRecorderDaemon_LoadMarksInterruptedPartialRecording(t *testing.T) {
	now := time.Now().UTC()
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "recorder-state.json")
	partialPath := filepath.Join(dir, "sports", "active-1.partial.ts")
	if err := os.MkdirAll(filepath.Dir(partialPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(partialPath, []byte("partial"), 0o600); err != nil {
		t.Fatalf("write partial: %v", err)
	}
	initial := CatchupRecorderState{
		UpdatedAt: now.Add(-time.Hour).Format(time.RFC3339),
		RootDir:   dir,
		Active: []CatchupRecorderItem{{
			CapsuleID:  "active-1",
			DNAID:      "dna:shared",
			ChannelID:  "101",
			Title:      "Shared Show",
			Lane:       "sports",
			Start:      now.Add(-2 * time.Minute).Format(time.RFC3339),
			Stop:       now.Add(2 * time.Minute).Format(time.RFC3339),
			OutputPath: partialPath,
			Attempt:    1,
		}},
	}
	data, _ := json.MarshalIndent(initial, "", "  ")
	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
		t.Fatalf("write state file: %v", err)
	}
	state, err := RunCatchupRecorderDaemon(context.Background(), CatchupRecorderDaemonConfig{
		StreamBaseURL: "http://127.0.0.1:59999",
		OutDir:        dir,
		StateFile:     stateFile,
		Once:          true,
		Now:           func() time.Time { return now },
	}, func(time.Time) (CatchupCapsulePreview, error) {
		return CatchupCapsulePreview{}, nil
	}, nil)
	if err != nil {
		t.Fatalf("RunCatchupRecorderDaemon: %v", err)
	}
	if got := state.Statistics.FailedCount; got != 1 {
		t.Fatalf("failed=%d want 1", got)
	}
	item := state.Failed[0]
	if item.Status != "interrupted" {
		t.Fatalf("status=%q want interrupted", item.Status)
	}
	if item.RecoveryReason != "daemon_restart" {
		t.Fatalf("recovery_reason=%q want daemon_restart", item.RecoveryReason)
	}
	if !item.PartialRecording || item.BytesRecorded == 0 {
		t.Fatalf("expected partial recording metadata: %+v", item)
	}
}

func TestRunCatchupRecorderDaemon_RetriesInterruptedProgrammeWithinWindow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ts-data-daemon"))
	}))
	defer srv.Close()

	now := time.Now().UTC()
	dir := t.TempDir()
	stateFile := filepath.Join(dir, "recorder-state.json")
	initial := CatchupRecorderState{
		UpdatedAt: now.Add(-time.Hour).Format(time.RFC3339),
		RootDir:   dir,
		Active: []CatchupRecorderItem{{
			CapsuleID: "capsule-old",
			DNAID:     "dna:shared",
			ChannelID: "101",
			Title:     "Shared Show",
			Lane:      "sports",
			Start:     now.Add(-2 * time.Minute).Format(time.RFC3339),
			Stop:      now.Add(2 * time.Minute).Format(time.RFC3339),
			Attempt:   1,
		}},
	}
	data, _ := json.MarshalIndent(initial, "", "  ")
	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
		t.Fatalf("write state file: %v", err)
	}
	state, err := RunCatchupRecorderDaemon(context.Background(), CatchupRecorderDaemonConfig{
		StreamBaseURL: srv.URL,
		OutDir:        dir,
		StateFile:     stateFile,
		Once:          true,
		Now:           func() time.Time { return now },
	}, func(time.Time) (CatchupCapsulePreview, error) {
		return CatchupCapsulePreview{
			Capsules: []CatchupCapsule{{
				CapsuleID:   "capsule-new",
				DNAID:       "dna:shared",
				ChannelID:   "101",
				GuideNumber: "101",
				ChannelName: "SportsNet",
				Title:       "Shared Show",
				Lane:        "sports",
				State:       "in_progress",
				Start:       now.Add(-2 * time.Minute).Format(time.RFC3339),
				Stop:        now.Add(2 * time.Minute).Format(time.RFC3339),
			}},
		}, nil
	}, srv.Client())
	if err != nil {
		t.Fatalf("RunCatchupRecorderDaemon: %v", err)
	}
	if got := state.Statistics.CompletedCount; got != 1 {
		t.Fatalf("completed=%d want 1", got)
	}
	if state.Completed[0].Attempt != 2 {
		t.Fatalf("attempt=%d want 2", state.Completed[0].Attempt)
	}
	if got := state.Statistics.FailedCount; got != 0 {
		t.Fatalf("failed=%d want 0 after retryable interruption consumed", got)
	}
}

func TestRunCatchupRecorderDaemon_RetriesTransientHTTP(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("okdata"))
	}))
	defer srv.Close()

	now := time.Now().UTC()
	dir := t.TempDir()
	state, err := RunCatchupRecorderDaemon(context.Background(), CatchupRecorderDaemonConfig{
		StreamBaseURL:      srv.URL,
		OutDir:             dir,
		PollInterval:       10 * time.Millisecond,
		MaxConcurrency:     1,
		RecordMaxAttempts:  5,
		RecordRetryInitial: 5 * time.Millisecond,
		RecordRetryMax:     100 * time.Millisecond,
		Once:               true,
		Now:                func() time.Time { return now },
	}, func(time.Time) (CatchupCapsulePreview, error) {
		return CatchupCapsulePreview{
			Capsules: []CatchupCapsule{{
				CapsuleID: "dna:retry:http",
				ChannelID: "101",
				Title:     "Show",
				Lane:      "sports",
				State:     "in_progress",
				Start:     now.Add(-time.Minute).Format(time.RFC3339),
				Stop:      now.Add(time.Minute).Format(time.RFC3339),
			}},
		}, nil
	}, srv.Client())
	if err != nil {
		t.Fatalf("RunCatchupRecorderDaemon: %v", err)
	}
	if state.Statistics.CompletedCount != 1 {
		t.Fatalf("completed=%d want 1", state.Statistics.CompletedCount)
	}
	if n.Load() != 3 {
		t.Fatalf("requests=%d want 3", n.Load())
	}
}

func TestRunCatchupRecorderDaemon_LaneStorageStats(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("abcdef"))
	}))
	defer srv.Close()

	now := time.Now().UTC()
	dir := t.TempDir()
	state, err := RunCatchupRecorderDaemon(context.Background(), CatchupRecorderDaemonConfig{
		StreamBaseURL:   srv.URL,
		OutDir:          dir,
		PollInterval:    10 * time.Millisecond,
		MaxConcurrency:  1,
		LaneBudgetBytes: map[string]int64{"sports": 1000},
		Once:            true,
		Now:             func() time.Time { return now },
	}, func(time.Time) (CatchupCapsulePreview, error) {
		return CatchupCapsulePreview{
			Capsules: []CatchupCapsule{{
				CapsuleID: "dna:lane:stat",
				ChannelID: "101",
				Title:     "Show",
				Lane:      "sports",
				State:     "in_progress",
				Start:     now.Add(-time.Minute).Format(time.RFC3339),
				Stop:      now.Add(time.Minute).Format(time.RFC3339),
			}},
		}, nil
	}, srv.Client())
	if err != nil {
		t.Fatalf("RunCatchupRecorderDaemon: %v", err)
	}
	ls := state.Statistics.LaneStorage
	if ls == nil {
		t.Fatal("expected lane storage stats")
	}
	st, ok := ls["sports"]
	if !ok {
		t.Fatalf("missing sports lane: %+v", ls)
	}
	if st.UsedBytes != 6 || st.BudgetBytes != 1000 || st.HeadroomBytes != 994 {
		t.Fatalf("unexpected storage: %+v", st)
	}
}
