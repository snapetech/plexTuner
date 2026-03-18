package tuner

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/httpclient"
)

type GhostHunterConfig struct {
	PMSURL         string
	Token          string
	PollInterval   time.Duration
	ObserveWindow  time.Duration
	IdleTimeout    time.Duration
	RenewLease     time.Duration
	HardLease      time.Duration
	ScopeMachineID string
	ScopePlayerIP  string
}

type GhostHunterSession struct {
	LiveKey        string   `json:"live_key"`
	Title          string   `json:"title"`
	PlayerAddr     string   `json:"player_addr"`
	PlayerProduct  string   `json:"player_product"`
	PlayerPlatform string   `json:"player_platform"`
	PlayerState    string   `json:"player_state"`
	MachineID      string   `json:"machine_id"`
	TranscodeID    string   `json:"transcode_id"`
	SessionID      string   `json:"session_id"`
	MaxOffsetAvail float64  `json:"max_offset_available"`
	MinOffsetAvail float64  `json:"min_offset_available"`
	SessionAge     string   `json:"session_age"`
	IdleAge        string   `json:"idle_age"`
	RenewLeaseAge  string   `json:"renew_lease_age"`
	Status         string   `json:"status"`
	Reasons        []string `json:"reasons,omitempty"`
}

type GhostHunterReport struct {
	GeneratedAt         string               `json:"generated_at"`
	ObservedFor         string               `json:"observed_for"`
	SessionCount        int                  `json:"session_count"`
	StaleCount          int                  `json:"stale_count"`
	CanStop             bool                 `json:"can_stop"`
	HiddenGrabSuspected bool                 `json:"hidden_grab_suspected"`
	RecommendedAction   string               `json:"recommended_action,omitempty"`
	RecoveryCommand     string               `json:"recovery_command,omitempty"`
	Runbook             string               `json:"runbook,omitempty"`
	Notes               []string             `json:"notes,omitempty"`
	Thresholds          map[string]string    `json:"thresholds"`
	Sessions            []GhostHunterSession `json:"sessions"`
}

func NewGhostHunterConfigFromEnv() GhostHunterConfig {
	reaper := loadPlexSessionReaperConfigFromEnv()
	return GhostHunterConfig{
		PMSURL:         reaper.PMSURL,
		Token:          reaper.Token,
		PollInterval:   reaper.PollInterval,
		ObserveWindow:  4 * time.Second,
		IdleTimeout:    reaper.IdleTimeout,
		RenewLease:     reaper.RenewLease,
		HardLease:      reaper.HardLease,
		ScopeMachineID: reaper.ScopeMachineID,
		ScopePlayerIP:  reaper.ScopePlayerIP,
	}
}

func RunGhostHunter(ctx context.Context, cfg GhostHunterConfig, stop bool, client *http.Client) (GhostHunterReport, error) {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Second
	}
	if client == nil {
		client = httpclient.ForStreaming()
	}
	reaper := &plexSessionReaper{
		cfg: plexSessionReaperConfig{
			PMSURL:         strings.TrimSpace(cfg.PMSURL),
			Token:          strings.TrimSpace(cfg.Token),
			PollInterval:   cfg.PollInterval,
			IdleTimeout:    cfg.IdleTimeout,
			RenewLease:     cfg.RenewLease,
			HardLease:      cfg.HardLease,
			ScopeMachineID: strings.TrimSpace(cfg.ScopeMachineID),
			ScopePlayerIP:  strings.TrimSpace(cfg.ScopePlayerIP),
			LogPrefix:      "ghost-hunter:",
		},
		client: client,
	}
	return reaper.observeAndOptionallyStop(ctx, cfg.ObserveWindow, stop)
}

func (r *plexSessionReaper) observeAndOptionallyStop(ctx context.Context, observe time.Duration, stop bool) (GhostHunterReport, error) {
	if r == nil {
		return GhostHunterReport{}, nil
	}
	if r.client == nil {
		r.client = httpclient.ForStreaming()
	}
	now := time.Now()
	report := GhostHunterReport{
		GeneratedAt: now.UTC().Format(time.RFC3339),
		CanStop:     stop,
		Thresholds: map[string]string{
			"idle_timeout": strings.TrimSpace(r.cfg.IdleTimeout.String()),
			"renew_lease":  strings.TrimSpace(r.cfg.RenewLease.String()),
			"hard_lease":   strings.TrimSpace(r.cfg.HardLease.String()),
			"poll":         strings.TrimSpace(r.cfg.PollInterval.String()),
		},
	}
	if strings.TrimSpace(r.cfg.PMSURL) == "" || strings.TrimSpace(r.cfg.Token) == "" {
		report.Notes = append(report.Notes, "missing PMS URL or token")
		return report, nil
	}
	if observe < 0 {
		observe = 0
	}
	states := map[string]*plexSessionReaperState{}
	rowsByKey := map[string]plexLiveSessionRow{}
	start := time.Now()
	for {
		rows, err := r.listLiveSessions(ctx)
		if err != nil {
			return report, err
		}
		now = time.Now()
		activeKeys := make(map[string]struct{}, len(rows))
		for _, row := range rows {
			key := row.TranscodeID
			if key == "" {
				key = row.LiveKey
			}
			if key == "" {
				continue
			}
			activeKeys[key] = struct{}{}
			rowsByKey[key] = row
			st := states[key]
			if st == nil {
				st = &plexSessionReaperState{
					firstSeen:      now,
					lastActivity:   now,
					lastRenewLease: now,
					lastMaxOffset:  row.MaxOffsetAvail,
					lastTransTS:    row.TranscodeTS,
				}
				states[key] = st
				continue
			}
			sawActivity := false
			if row.MaxOffsetAvail > st.lastMaxOffset+0.25 {
				sawActivity = true
			}
			if row.TranscodeTS != "" && row.TranscodeTS != st.lastTransTS {
				sawActivity = true
			}
			if sawActivity {
				st.lastActivity = now
				st.lastRenewLease = now
			}
			if row.MaxOffsetAvail > st.lastMaxOffset {
				st.lastMaxOffset = row.MaxOffsetAvail
			}
			if row.TranscodeTS != "" {
				st.lastTransTS = row.TranscodeTS
			}
		}
		for key := range states {
			if _, ok := activeKeys[key]; !ok {
				delete(states, key)
				delete(rowsByKey, key)
			}
		}
		if observe == 0 || time.Since(start) >= observe {
			report.ObservedFor = time.Since(start).Round(time.Millisecond).String()
			break
		}
		select {
		case <-ctx.Done():
			return report, ctx.Err()
		case <-time.After(r.cfg.PollInterval):
		}
	}
	report.Sessions = make([]GhostHunterSession, 0, len(rowsByKey))
	for key, row := range rowsByKey {
		st := states[key]
		if st == nil {
			continue
		}
		sessionAge := now.Sub(st.firstSeen)
		idleAge := now.Sub(st.lastActivity)
		renewLeaseAge := now.Sub(st.lastRenewLease)
		status := "active"
		var reasons []string
		if r.cfg.IdleTimeout > 0 && idleAge >= r.cfg.IdleTimeout {
			status = "stale"
			reasons = append(reasons, "idle>="+r.cfg.IdleTimeout.String())
		}
		if r.cfg.RenewLease > 0 && renewLeaseAge >= r.cfg.RenewLease {
			status = "stale"
			reasons = append(reasons, "renew_lease>="+r.cfg.RenewLease.String())
		}
		if r.cfg.HardLease > 0 && sessionAge >= r.cfg.HardLease {
			status = "stale"
			reasons = append(reasons, "hard_lease>="+r.cfg.HardLease.String())
		}
		if strings.TrimSpace(strings.ToLower(row.PlayerState)) != "playing" && row.PlayerState != "" {
			reasons = append(reasons, "player_state="+strings.ToLower(strings.TrimSpace(row.PlayerState)))
		}
		item := GhostHunterSession{
			LiveKey:        row.LiveKey,
			Title:          row.Title,
			PlayerAddr:     row.PlayerAddr,
			PlayerProduct:  row.PlayerProduct,
			PlayerPlatform: row.PlayerPlatform,
			PlayerState:    row.PlayerState,
			MachineID:      row.MachineID,
			TranscodeID:    row.TranscodeID,
			SessionID:      row.SessionID,
			MaxOffsetAvail: row.MaxOffsetAvail,
			MinOffsetAvail: row.MinOffsetAvail,
			SessionAge:     sessionAge.Round(time.Millisecond).String(),
			IdleAge:        idleAge.Round(time.Millisecond).String(),
			RenewLeaseAge:  renewLeaseAge.Round(time.Millisecond).String(),
			Status:         status,
			Reasons:        reasons,
		}
		if status == "stale" {
			report.StaleCount++
			if stop && row.TranscodeID != "" {
				code, err := r.stopTranscode(ctx, row.TranscodeID)
				if err != nil {
					item.Reasons = append(item.Reasons, "stop_err="+err.Error())
				} else {
					item.Reasons = append(item.Reasons, "stop_status="+http.StatusText(code))
				}
			}
		}
		report.Sessions = append(report.Sessions, item)
	}
	report.SessionCount = len(report.Sessions)
	if observe == 0 {
		report.Notes = append(report.Notes, "single-snapshot mode: stale classification is limited without observing offset/timestamp movement over time")
	}
	if report.SessionCount == 0 {
		report.HiddenGrabSuspected = true
		report.RecommendedAction = "If channel tunes are still blocked and IptvTunerr sees no /stream requests, run the guarded hidden-grab recovery helper."
		report.RecoveryCommand = "./scripts/plex-hidden-grab-recover.sh --dry-run"
		report.Runbook = "docs/runbooks/plex-hidden-live-grab-recovery.md"
		report.Notes = append(report.Notes, "no visible live sessions found; hidden Plex grabs can still require external recovery if tunes remain blocked")
	}
	return report, nil
}
