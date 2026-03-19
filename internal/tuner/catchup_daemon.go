package tuner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/snapetech/iptvtunerr/internal/httpclient"
)

type CatchupRecorderDaemonConfig struct {
	StreamBaseURL         string
	OutDir                string
	PublishDir            string
	StateFile             string
	PollInterval          time.Duration
	LeadTime              time.Duration
	MaxConcurrency        int
	MaxRecordDuration     time.Duration
	RetainCompleted       int
	RetainFailed          int
	LaneRetainCompleted   map[string]int
	LaneRetainFailed      map[string]int
	LaneBudgetBytes       map[string]int64
	IncludeLanes          []string
	ExcludeLanes          []string
	IncludeChannels       []string
	ExcludeChannels       []string
	AllowRetryInterrupted bool
	OnPublished           func(CatchupRecordedPublishedItem) error
	Once                  bool
	Now                   func() time.Time
}

type CatchupRecorderState struct {
	UpdatedAt  string                    `json:"updated_at"`
	RootDir    string                    `json:"root_dir"`
	StateFile  string                    `json:"state_file,omitempty"`
	Active     []CatchupRecorderItem     `json:"active"`
	Completed  []CatchupRecorderItem     `json:"completed"`
	Failed     []CatchupRecorderItem     `json:"failed"`
	Statistics CatchupRecorderStatistics `json:"statistics"`
}

type CatchupRecorderStatistics struct {
	ActiveCount    int `json:"active_count"`
	CompletedCount int `json:"completed_count"`
	FailedCount    int `json:"failed_count"`
}

type CatchupRecorderItem struct {
	CapsuleID        string `json:"capsule_id"`
	RecordKey        string `json:"record_key,omitempty"`
	DNAID            string `json:"dna_id,omitempty"`
	ChannelID        string `json:"channel_id"`
	GuideNumber      string `json:"guide_number,omitempty"`
	ChannelName      string `json:"channel_name,omitempty"`
	Title            string `json:"title"`
	Lane             string `json:"lane"`
	State            string `json:"state"`
	ReplayMode       string `json:"replay_mode,omitempty"`
	ReplayURL        string `json:"replay_url,omitempty"`
	Start            string `json:"start,omitempty"`
	Stop             string `json:"stop,omitempty"`
	ExpiresAt        string `json:"expires_at,omitempty"`
	SourceURL        string `json:"source_url,omitempty"`
	OutputPath       string `json:"output_path,omitempty"`
	PublishedDir     string `json:"published_dir,omitempty"`
	PublishedPath    string `json:"published_path,omitempty"`
	NFOPath          string `json:"nfo_path,omitempty"`
	Status           string `json:"status"`
	EligibleAt       string `json:"eligible_at,omitempty"`
	ScheduledFor     string `json:"scheduled_for,omitempty"`
	StartedAt        string `json:"started_at,omitempty"`
	StoppedAt        string `json:"stopped_at,omitempty"`
	RecoveredAt      string `json:"recovered_at,omitempty"`
	DurationMins     int    `json:"duration_mins,omitempty"`
	BytesRecorded    int64  `json:"bytes_recorded,omitempty"`
	PartialRecording bool   `json:"partial_recording,omitempty"`
	Attempt          int    `json:"attempt,omitempty"`
	RecoveryReason   string `json:"recovery_reason,omitempty"`
	Error            string `json:"error,omitempty"`
}

type CatchupRecorderPreviewFunc func(time.Time) (CatchupCapsulePreview, error)

func RunCatchupRecorderDaemon(ctx context.Context, cfg CatchupRecorderDaemonConfig, previewFn CatchupRecorderPreviewFunc, client *http.Client) (CatchupRecorderState, error) {
	cfg = normalizeCatchupRecorderDaemonConfig(cfg)
	if strings.TrimSpace(cfg.OutDir) == "" {
		return CatchupRecorderState{}, fmt.Errorf("output directory required")
	}
	if strings.TrimSpace(cfg.StreamBaseURL) == "" {
		return CatchupRecorderState{}, fmt.Errorf("stream base url required")
	}
	if previewFn == nil {
		return CatchupRecorderState{}, fmt.Errorf("preview function required")
	}
	if client == nil {
		client = httpclient.ForStreaming()
	}
	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return CatchupRecorderState{}, err
	}
	if strings.TrimSpace(cfg.PublishDir) != "" {
		if err := os.MkdirAll(strings.TrimSpace(cfg.PublishDir), 0o755); err != nil {
			return CatchupRecorderState{}, err
		}
	}
	stateFile := strings.TrimSpace(cfg.StateFile)
	if stateFile == "" {
		stateFile = filepath.Join(cfg.OutDir, "recorder-state.json")
	}
	mgr, err := newCatchupRecorderManager(cfg, stateFile, client)
	if err != nil {
		return CatchupRecorderState{}, err
	}
	defer mgr.wait()

	scan := func() error {
		now := cfg.Now()
		preview, err := previewFn(now)
		if err != nil {
			return err
		}
		mgr.schedule(preview, now)
		return nil
	}

	if err := scan(); err != nil {
		return mgr.snapshot(), err
	}
	if cfg.Once {
		mgr.wait()
		return mgr.snapshot(), nil
	}

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			mgr.wait()
			return mgr.snapshot(), nil
		case <-ticker.C:
			if err := scan(); err != nil {
				return mgr.snapshot(), err
			}
		}
	}
}

type catchupRecorderManager struct {
	cfg           CatchupRecorderDaemonConfig
	stateFile     string
	client        *http.Client
	semaphore     chan struct{}
	now           func() time.Time
	mu            sync.Mutex
	state         CatchupRecorderState
	activeSet     map[string]bool
	activeKeys    map[string]bool
	completed     map[string]bool
	completedKeys map[string]bool
	failed        map[string]bool
	failedKeys    map[string]bool
	retryAttempts map[string]int
	wg            sync.WaitGroup
}

func newCatchupRecorderManager(cfg CatchupRecorderDaemonConfig, stateFile string, client *http.Client) (*catchupRecorderManager, error) {
	m := &catchupRecorderManager{
		cfg:           cfg,
		stateFile:     stateFile,
		client:        client,
		semaphore:     make(chan struct{}, cfg.MaxConcurrency),
		now:           cfg.Now,
		activeSet:     map[string]bool{},
		activeKeys:    map[string]bool{},
		completed:     map[string]bool{},
		completedKeys: map[string]bool{},
		failed:        map[string]bool{},
		failedKeys:    map[string]bool{},
		retryAttempts: map[string]int{},
		state: CatchupRecorderState{
			RootDir:   cfg.OutDir,
			StateFile: stateFile,
			Active:    []CatchupRecorderItem{},
			Completed: []CatchupRecorderItem{},
			Failed:    []CatchupRecorderItem{},
		},
	}
	if err := m.load(); err != nil {
		return nil, err
	}
	return m, nil
}

func normalizeCatchupRecorderDaemonConfig(cfg CatchupRecorderDaemonConfig) CatchupRecorderDaemonConfig {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 30 * time.Second
	}
	if cfg.LeadTime < 0 {
		cfg.LeadTime = 0
	}
	if cfg.MaxConcurrency <= 0 {
		cfg.MaxConcurrency = 2
	}
	if cfg.RetainCompleted <= 0 {
		cfg.RetainCompleted = 200
	}
	if cfg.RetainFailed <= 0 {
		cfg.RetainFailed = 100
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if !cfg.AllowRetryInterrupted {
		cfg.AllowRetryInterrupted = true
	}
	cfg.LaneRetainCompleted = normalizeLaneIntLimits(cfg.LaneRetainCompleted)
	cfg.LaneRetainFailed = normalizeLaneIntLimits(cfg.LaneRetainFailed)
	cfg.LaneBudgetBytes = normalizeLaneByteLimits(cfg.LaneBudgetBytes)
	return cfg
}

func (m *catchupRecorderManager) load() error {
	data, err := os.ReadFile(m.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return m.persistLocked()
		}
		return err
	}
	var state CatchupRecorderState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	m.state = state
	m.state.RootDir = m.cfg.OutDir
	m.state.StateFile = m.stateFile
	for i := range m.state.Active {
		item := m.state.Active[i]
		item.Status = "interrupted"
		item.RecoveryReason = firstNonEmptyString(item.RecoveryReason, "daemon_restart")
		item.RecoveredAt = m.now().UTC().Format(time.RFC3339)
		item.Error = firstNonEmptyString(item.Error, "interrupted before completion")
		item.StoppedAt = m.now().UTC().Format(time.RFC3339)
		if size := catchupRecorderItemSize(item); size > 0 {
			item.BytesRecorded = size
			item.PartialRecording = true
		}
		m.state.Failed = append([]CatchupRecorderItem{item}, m.state.Failed...)
		m.failed[item.CapsuleID] = true
		if key := catchupRecorderItemKey(item); key != "" {
			m.failedKeys[key] = true
		}
	}
	m.state.Active = nil
	for _, item := range m.state.Completed {
		m.completed[item.CapsuleID] = true
		if key := catchupRecorderItemKey(item); key != "" {
			m.completedKeys[key] = true
		}
	}
	for _, item := range m.state.Failed {
		m.failed[item.CapsuleID] = true
		if key := catchupRecorderItemKey(item); key != "" {
			m.failedKeys[key] = true
		}
	}
	m.trimLocked()
	return m.persistLocked()
}

func (m *catchupRecorderManager) schedule(preview CatchupCapsulePreview, now time.Time) {
	for _, capsule := range preview.Capsules {
		if !m.eligibleCapsule(capsule, now) {
			continue
		}
		item := catchupRecorderItemFromCapsule(capsule, now)
		key := catchupRecorderItemKey(item)
		m.mu.Lock()
		if m.cfg.AllowRetryInterrupted {
			m.consumeRetryableFailureLocked(item, now)
		}
		if m.activeSet[item.CapsuleID] || m.completed[item.CapsuleID] || m.failed[item.CapsuleID] ||
			(key != "" && (m.activeKeys[key] || m.completedKeys[key] || m.failedKeys[key])) {
			m.mu.Unlock()
			continue
		}
		m.activeSet[item.CapsuleID] = true
		if key != "" {
			m.activeKeys[key] = true
		}
		m.state.Active = append([]CatchupRecorderItem{item}, m.state.Active...)
		_ = m.persistLocked()
		m.mu.Unlock()
		m.wg.Add(1)
		go func(c CatchupCapsule) {
			defer m.wg.Done()
			m.runCapsule(c)
		}(capsule)
	}
}

func (m *catchupRecorderManager) eligibleCapsule(c CatchupCapsule, now time.Time) bool {
	if !laneAllowed(c.Lane, m.cfg.IncludeLanes, m.cfg.ExcludeLanes) {
		return false
	}
	if !channelAllowed(c, m.cfg.IncludeChannels, m.cfg.ExcludeChannels) {
		return false
	}
	state := strings.ToLower(strings.TrimSpace(c.State))
	if state != "in_progress" && state != "starting_soon" {
		return false
	}
	start, err := time.Parse(time.RFC3339, c.Start)
	if err != nil {
		return false
	}
	if state == "starting_soon" && start.After(now.Add(m.cfg.LeadTime)) {
		return false
	}
	return true
}

func laneAllowed(lane string, include, exclude []string) bool {
	lane = strings.ToLower(strings.TrimSpace(lane))
	if len(include) > 0 {
		ok := false
		for _, v := range include {
			if strings.ToLower(strings.TrimSpace(v)) == lane {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	for _, v := range exclude {
		if strings.ToLower(strings.TrimSpace(v)) == lane {
			return false
		}
	}
	return true
}

func catchupRecorderItemFromCapsule(c CatchupCapsule, now time.Time) CatchupRecorderItem {
	return CatchupRecorderItem{
		CapsuleID:    c.CapsuleID,
		RecordKey:    catchupCapsuleRecordKey(c),
		DNAID:        c.DNAID,
		ChannelID:    c.ChannelID,
		GuideNumber:  c.GuideNumber,
		ChannelName:  c.ChannelName,
		Title:        c.Title,
		Lane:         firstNonEmptyString(c.Lane, "general"),
		State:        c.State,
		ReplayMode:   c.ReplayMode,
		ReplayURL:    c.ReplayURL,
		Start:        c.Start,
		Stop:         c.Stop,
		ExpiresAt:    c.ExpiresAt,
		Status:       "scheduled",
		EligibleAt:   now.UTC().Format(time.RFC3339),
		ScheduledFor: strings.TrimSpace(c.Start),
		DurationMins: c.DurationMins,
	}
}

func (m *catchupRecorderManager) runCapsule(c CatchupCapsule) {
	now := m.now()
	startAt, _ := time.Parse(time.RFC3339, c.Start)
	stopAt, _ := time.Parse(time.RFC3339, c.Stop)
	if startAt.After(now) {
		time.Sleep(time.Until(startAt))
	}
	m.semaphore <- struct{}{}
	defer func() { <-m.semaphore }()

	item := catchupRecorderItemFromCapsule(c, m.now())
	item.Status = "recording"
	item.StartedAt = m.now().UTC().Format(time.RFC3339)
	item.Attempt = m.nextAttemptForItem(item)
	if item.Attempt < 1 {
		item.Attempt = 1
	}
	sourceURL, _ := ResolveCatchupRecordSourceURL(c, m.cfg.StreamBaseURL)
	item.SourceURL = sourceURL
	item.OutputPath = filepath.Join(m.cfg.OutDir, firstNonEmptyString(c.Lane, "general"), sanitizeCatchupName(c.CapsuleID)+".ts")
	m.updateActive(item)

	ctx := context.Background()
	if !stopAt.IsZero() || m.cfg.MaxRecordDuration > 0 {
		deadline := stopAt
		if deadline.IsZero() {
			deadline = m.now().Add(m.cfg.MaxRecordDuration)
		}
		if m.cfg.MaxRecordDuration > 0 {
			capDeadline := m.now().Add(m.cfg.MaxRecordDuration)
			if deadline.IsZero() || capDeadline.Before(deadline) {
				deadline = capDeadline
			}
		}
		var cancel context.CancelFunc
		ctx, cancel = context.WithDeadline(context.Background(), deadline)
		defer cancel()
	}
	recorded, err := RecordCatchupCapsule(ctx, c, m.cfg.StreamBaseURL, m.cfg.OutDir, m.client)
	if err != nil {
		item.Status = "failed"
		item.Error = err.Error()
		item.StoppedAt = m.now().UTC().Format(time.RFC3339)
		m.finish(item, false)
		return
	}
	item.Status = "completed"
	item.OutputPath = recorded.OutputPath
	item.SourceURL = recorded.SourceURL
	item.BytesRecorded = recorded.Bytes
	item.StoppedAt = m.now().UTC().Format(time.RFC3339)
	if strings.TrimSpace(m.cfg.PublishDir) != "" {
		pub, pubErr := PublishRecordedCatchupItem(m.cfg.PublishDir, c, recorded)
		if pubErr != nil {
			item.Status = "failed"
			item.Error = pubErr.Error()
			item.StoppedAt = m.now().UTC().Format(time.RFC3339)
			m.finish(item, false)
			return
		}
		item.PublishedDir = pub.Directory
		item.PublishedPath = pub.MediaPath
		item.NFOPath = pub.NFOPath
		if m.cfg.OnPublished != nil {
			if hookErr := m.cfg.OnPublished(pub); hookErr != nil {
				item.Error = firstNonEmptyString(item.Error, hookErr.Error())
			}
		}
	}
	m.finish(item, true)
}

func (m *catchupRecorderManager) updateActive(item CatchupRecorderItem) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.state.Active {
		if m.state.Active[i].CapsuleID == item.CapsuleID {
			m.state.Active[i] = item
			_ = m.persistLocked()
			return
		}
	}
	m.state.Active = append([]CatchupRecorderItem{item}, m.state.Active...)
	_ = m.persistLocked()
}

func (m *catchupRecorderManager) finish(item CatchupRecorderItem, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	active := m.state.Active[:0]
	for _, existing := range m.state.Active {
		if existing.CapsuleID != item.CapsuleID {
			active = append(active, existing)
		}
	}
	m.state.Active = active
	delete(m.activeSet, item.CapsuleID)
	if key := catchupRecorderItemKey(item); key != "" {
		delete(m.activeKeys, key)
	}
	if success {
		m.state.Completed = append([]CatchupRecorderItem{item}, m.state.Completed...)
		m.completed[item.CapsuleID] = true
		if key := catchupRecorderItemKey(item); key != "" {
			m.completedKeys[key] = true
		}
		if strings.TrimSpace(m.cfg.PublishDir) != "" {
			_ = m.persistRecordedPublishManifestLocked()
		}
	} else {
		m.state.Failed = append([]CatchupRecorderItem{item}, m.state.Failed...)
		m.failed[item.CapsuleID] = true
		if key := catchupRecorderItemKey(item); key != "" {
			m.failedKeys[key] = true
		}
	}
	m.trimLocked()
	_ = m.persistLocked()
}

func (m *catchupRecorderManager) trimLocked() {
	now := m.now()
	filterExpired := func(items []CatchupRecorderItem) []CatchupRecorderItem {
		out := items[:0]
		for _, item := range items {
			if expiry := strings.TrimSpace(item.ExpiresAt); expiry != "" {
				if t, err := time.Parse(time.RFC3339, expiry); err == nil && !t.After(now) {
					_ = removeCatchupRecorderItemFiles(item)
					continue
				}
			}
			out = append(out, item)
		}
		return out
	}
	m.state.Completed = filterExpired(m.state.Completed)
	m.state.Completed = pruneCatchupRecorderLaneRetention(m.state.Completed, m.cfg.LaneRetainCompleted, nil)
	m.state.Completed = pruneCatchupRecorderLaneRetention(m.state.Completed, nil, m.cfg.LaneBudgetBytes)
	if max := m.cfg.RetainCompleted; max > 0 && len(m.state.Completed) > max {
		for _, item := range m.state.Completed[max:] {
			_ = removeCatchupRecorderItemFiles(item)
		}
		m.state.Completed = m.state.Completed[:max]
	}
	m.state.Failed = filterExpired(m.state.Failed)
	m.state.Failed = pruneCatchupRecorderLaneRetention(m.state.Failed, m.cfg.LaneRetainFailed, nil)
	if max := m.cfg.RetainFailed; max > 0 && len(m.state.Failed) > max {
		for _, item := range m.state.Failed[max:] {
			_ = removeCatchupRecorderItemFiles(item)
		}
		m.state.Failed = m.state.Failed[:max]
	}
	m.rebuildIndexesLocked()
	m.state.UpdatedAt = m.now().UTC().Format(time.RFC3339)
	m.state.Statistics = CatchupRecorderStatistics{
		ActiveCount:    len(m.state.Active),
		CompletedCount: len(m.state.Completed),
		FailedCount:    len(m.state.Failed),
	}
	sort.SliceStable(m.state.Active, func(i, j int) bool { return m.state.Active[i].EligibleAt > m.state.Active[j].EligibleAt })
}

func (m *catchupRecorderManager) persistLocked() error {
	m.trimLocked()
	data, err := json.MarshalIndent(m.state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.stateFile, data, 0o600)
}

func (m *catchupRecorderManager) persistRecordedPublishManifestLocked() error {
	items := make([]CatchupRecordedPublishedItem, 0, len(m.state.Completed))
	for _, item := range m.state.Completed {
		if strings.TrimSpace(item.PublishedDir) == "" || strings.TrimSpace(item.PublishedPath) == "" {
			continue
		}
		items = append(items, CatchupRecordedPublishedItem{
			CapsuleID: item.CapsuleID,
			Lane:      item.Lane,
			Title:     item.Title,
			Directory: item.PublishedDir,
			MediaPath: item.PublishedPath,
			NFOPath:   item.NFOPath,
			SourceTS:  item.OutputPath,
		})
	}
	return SaveRecordedCatchupPublishManifest(m.cfg.PublishDir, items)
}

func (m *catchupRecorderManager) snapshot() CatchupRecorderState {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := m.state
	out.Active = append([]CatchupRecorderItem(nil), m.state.Active...)
	out.Completed = append([]CatchupRecorderItem(nil), m.state.Completed...)
	out.Failed = append([]CatchupRecorderItem(nil), m.state.Failed...)
	return out
}

func (m *catchupRecorderManager) consumeRetryableFailureLocked(item CatchupRecorderItem, now time.Time) {
	bestAttempt := 0
	retryable := func(existing CatchupRecorderItem) bool {
		if strings.TrimSpace(existing.RecoveryReason) != "daemon_restart" && strings.ToLower(strings.TrimSpace(existing.Status)) != "interrupted" {
			return false
		}
		if stop := strings.TrimSpace(existing.Stop); stop != "" {
			if t, err := time.Parse(time.RFC3339, stop); err == nil && !t.After(now) {
				return false
			}
		}
		if existing.CapsuleID == item.CapsuleID {
			return true
		}
		key := catchupRecorderItemKey(existing)
		return key != "" && key == catchupRecorderItemKey(item)
	}
	filtered := m.state.Failed[:0]
	changed := false
	for _, existing := range m.state.Failed {
		if retryable(existing) {
			changed = true
			if existing.Attempt > bestAttempt {
				bestAttempt = existing.Attempt
			}
			continue
		}
		filtered = append(filtered, existing)
	}
	if changed {
		m.state.Failed = filtered
		m.rememberRetryAttempt(item, bestAttempt)
		m.rebuildIndexesLocked()
	}
}

func (m *catchupRecorderManager) rebuildIndexesLocked() {
	m.activeSet = map[string]bool{}
	m.activeKeys = map[string]bool{}
	m.completed = map[string]bool{}
	m.completedKeys = map[string]bool{}
	m.failed = map[string]bool{}
	m.failedKeys = map[string]bool{}
	for _, item := range m.state.Active {
		m.activeSet[item.CapsuleID] = true
		if key := catchupRecorderItemKey(item); key != "" {
			m.activeKeys[key] = true
		}
	}
	for _, item := range m.state.Completed {
		m.completed[item.CapsuleID] = true
		if key := catchupRecorderItemKey(item); key != "" {
			m.completedKeys[key] = true
		}
	}
	for _, item := range m.state.Failed {
		m.failed[item.CapsuleID] = true
		if key := catchupRecorderItemKey(item); key != "" {
			m.failedKeys[key] = true
		}
	}
}

func (m *catchupRecorderManager) wait() {
	m.wg.Wait()
}

func removeCatchupRecorderItemFiles(item CatchupRecorderItem) error {
	if p := strings.TrimSpace(item.OutputPath); p != "" {
		_ = os.Remove(p)
	}
	if p := strings.TrimSpace(item.NFOPath); p != "" {
		_ = os.Remove(p)
	}
	if p := strings.TrimSpace(item.PublishedPath); p != "" {
		_ = os.Remove(p)
	}
	if d := strings.TrimSpace(item.PublishedDir); d != "" {
		_ = os.Remove(d)
		_ = os.RemoveAll(d)
	}
	return nil
}

func channelAllowed(c CatchupCapsule, include, exclude []string) bool {
	match := func(filters []string) bool {
		if len(filters) == 0 {
			return false
		}
		values := []string{
			strings.ToLower(strings.TrimSpace(c.ChannelID)),
			strings.ToLower(strings.TrimSpace(c.GuideNumber)),
			strings.ToLower(strings.TrimSpace(c.DNAID)),
			strings.ToLower(strings.TrimSpace(c.ChannelName)),
		}
		for _, raw := range filters {
			filter := strings.ToLower(strings.TrimSpace(raw))
			if filter == "" {
				continue
			}
			for _, v := range values {
				if v != "" && v == filter {
					return true
				}
			}
		}
		return false
	}
	if len(include) > 0 && !match(include) {
		return false
	}
	if match(exclude) {
		return false
	}
	return true
}

func catchupCapsuleRecordKey(c CatchupCapsule) string {
	return catchupCuratedKey(c)
}

func catchupRecorderItemKey(item CatchupRecorderItem) string {
	if key := strings.TrimSpace(item.RecordKey); key != "" {
		return key
	}
	base := strings.TrimSpace(item.DNAID)
	if base == "" {
		base = strings.TrimSpace(item.ChannelID)
	}
	if base == "" || strings.TrimSpace(item.Start) == "" {
		return ""
	}
	return strings.ToLower(base + "|" + strings.TrimSpace(item.Start) + "|" + normalizeCatchupTitle(item.Title))
}

func catchupRecorderAttemptFromState(state CatchupRecorderState, item CatchupRecorderItem) int {
	best := 0
	key := catchupRecorderItemKey(item)
	check := func(items []CatchupRecorderItem) {
		for _, existing := range items {
			same := existing.CapsuleID == item.CapsuleID
			if !same && key != "" && catchupRecorderItemKey(existing) == key {
				same = true
			}
			if same && existing.Attempt > best {
				best = existing.Attempt
			}
		}
	}
	check(state.Active)
	check(state.Completed)
	check(state.Failed)
	return best
}

func (m *catchupRecorderManager) rememberRetryAttempt(item CatchupRecorderItem, attempt int) {
	if attempt <= 0 {
		return
	}
	if item.CapsuleID != "" && m.retryAttempts[item.CapsuleID] < attempt {
		m.retryAttempts[item.CapsuleID] = attempt
	}
	if key := catchupRecorderItemKey(item); key != "" && m.retryAttempts[key] < attempt {
		m.retryAttempts[key] = attempt
	}
}

func (m *catchupRecorderManager) nextAttemptForItem(item CatchupRecorderItem) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	best := catchupRecorderAttemptFromState(m.state, item)
	if v := m.retryAttempts[item.CapsuleID]; v > best {
		best = v
	}
	if key := catchupRecorderItemKey(item); key != "" {
		if v := m.retryAttempts[key]; v > best {
			best = v
		}
		delete(m.retryAttempts, key)
	}
	delete(m.retryAttempts, item.CapsuleID)
	return best + 1
}

func normalizeLaneIntLimits(in map[string]int) map[string]int {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]int, len(in))
	for lane, v := range in {
		lane = strings.ToLower(strings.TrimSpace(lane))
		if lane == "" || v <= 0 {
			continue
		}
		out[lane] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeLaneByteLimits(in map[string]int64) map[string]int64 {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]int64, len(in))
	for lane, v := range in {
		lane = strings.ToLower(strings.TrimSpace(lane))
		if lane == "" || v <= 0 {
			continue
		}
		out[lane] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func pruneCatchupRecorderLaneRetention(items []CatchupRecorderItem, laneCounts map[string]int, laneBudgets map[string]int64) []CatchupRecorderItem {
	if len(items) == 0 {
		return items
	}
	if len(laneCounts) == 0 && len(laneBudgets) == 0 {
		return items
	}
	seenCounts := map[string]int{}
	seenBytes := map[string]int64{}
	out := items[:0]
	for _, item := range items {
		lane := firstNonEmptyString(item.Lane, "general")
		laneKey := strings.ToLower(strings.TrimSpace(lane))
		keep := true
		if max := laneCounts[laneKey]; max > 0 && seenCounts[laneKey] >= max {
			keep = false
		}
		size := catchupRecorderItemSize(item)
		if keep {
			if budget := laneBudgets[laneKey]; budget > 0 && seenBytes[laneKey]+size > budget {
				keep = false
			}
		}
		if !keep {
			_ = removeCatchupRecorderItemFiles(item)
			continue
		}
		seenCounts[laneKey]++
		seenBytes[laneKey] += size
		out = append(out, item)
	}
	return out
}

func catchupRecorderItemSize(item CatchupRecorderItem) int64 {
	if item.BytesRecorded > 0 {
		return item.BytesRecorded
	}
	for _, path := range []string{strings.TrimSpace(item.PublishedPath), strings.TrimSpace(item.OutputPath)} {
		if path == "" {
			continue
		}
		if st, err := os.Stat(path); err == nil && st.Size() > 0 {
			return st.Size()
		}
	}
	return 0
}
