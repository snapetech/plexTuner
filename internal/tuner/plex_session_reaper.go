package tuner

import (
	"bufio"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/plextuner/plex-tuner/internal/httpclient"
)

type plexSessionReaperConfig struct {
	Enabled        bool
	PMSURL         string
	Token          string
	PollInterval   time.Duration
	IdleTimeout    time.Duration
	RenewLease     time.Duration
	HardLease      time.Duration
	SSE            bool
	ScopeMachineID string
	ScopePlayerIP  string
	LogPrefix      string
}

type plexLiveSessionRow struct {
	LiveKey        string
	Title          string
	PlayerAddr     string
	PlayerProduct  string
	PlayerPlatform string
	PlayerDevice   string
	PlayerState    string
	MachineID      string
	TranscodeID    string
	SessionID      string
	TranscodeTS    string
	MaxOffsetAvail float64
	MinOffsetAvail float64
}

type plexSessionReaper struct {
	cfg    plexSessionReaperConfig
	client *http.Client
}

type plexSessionReaperState struct {
	firstSeen       time.Time
	lastActivity    time.Time
	lastRenewLease  time.Time
	lastMaxOffset   float64
	lastTransTS     string
	lastStopAttempt time.Time
}

func loadPlexSessionReaperConfigFromEnv() plexSessionReaperConfig {
	cfg := plexSessionReaperConfig{
		Enabled:        envBool("PLEX_TUNER_PLEX_SESSION_REAPER", false),
		PMSURL:         strings.TrimSpace(os.Getenv("PLEX_TUNER_PMS_URL")),
		Token:          strings.TrimSpace(os.Getenv("PLEX_TUNER_PMS_TOKEN")),
		PollInterval:   envDurationSeconds("PLEX_TUNER_PLEX_SESSION_REAPER_POLL_S", 2*time.Second),
		IdleTimeout:    envDurationSeconds("PLEX_TUNER_PLEX_SESSION_REAPER_IDLE_S", 15*time.Second),
		RenewLease:     envDurationSeconds("PLEX_TUNER_PLEX_SESSION_REAPER_RENEW_LEASE_S", 20*time.Second),
		HardLease:      envDurationSeconds("PLEX_TUNER_PLEX_SESSION_REAPER_HARD_LEASE_S", 30*time.Minute),
		SSE:            envBool("PLEX_TUNER_PLEX_SESSION_REAPER_SSE", true),
		ScopeMachineID: strings.TrimSpace(os.Getenv("PLEX_TUNER_PLEX_SESSION_REAPER_MACHINE_ID")),
		ScopePlayerIP:  strings.TrimSpace(os.Getenv("PLEX_TUNER_PLEX_SESSION_REAPER_PLAYER_IP")),
		LogPrefix:      "plex-reaper:",
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 2 * time.Second
	}
	return cfg
}

func envBool(name string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

func envDurationSeconds(name string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f < 0 {
		return def
	}
	return time.Duration(f * float64(time.Second))
}

func maybeStartPlexSessionReaper(ctx context.Context, client *http.Client) {
	cfg := loadPlexSessionReaperConfigFromEnv()
	if !cfg.Enabled {
		return
	}
	if cfg.PMSURL == "" || cfg.Token == "" {
		log.Printf("%s disabled (missing PLEX_TUNER_PMS_URL or PLEX_TUNER_PMS_TOKEN)", cfg.LogPrefix)
		return
	}
	if cfg.IdleTimeout <= 0 && cfg.RenewLease <= 0 && cfg.HardLease <= 0 {
		log.Printf("%s disabled (no thresholds configured)", cfg.LogPrefix)
		return
	}
	if client == nil {
		client = httpclient.ForStreaming()
	}
	r := &plexSessionReaper{cfg: cfg, client: client}
	go r.run(ctx)
}

func (r *plexSessionReaper) run(ctx context.Context) {
	log.Printf("%s enabled pms_url=%q sse=%t poll=%s idle=%s renew_lease=%s hard_lease=%s scope_machine=%q scope_ip=%q",
		r.cfg.LogPrefix, r.cfg.PMSURL, r.cfg.SSE, r.cfg.PollInterval, r.cfg.IdleTimeout, r.cfg.RenewLease, r.cfg.HardLease, r.cfg.ScopeMachineID, r.cfg.ScopePlayerIP)
	wakeCh := make(chan struct{}, 1)
	if r.cfg.SSE {
		go r.runSSE(ctx, wakeCh)
	}
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	states := map[string]*plexSessionReaperState{}

	for {
		if err := r.scanAndReap(ctx, states); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("%s scan err=%v", r.cfg.LogPrefix, err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-wakeCh:
		}
	}
}

func (r *plexSessionReaper) runSSE(ctx context.Context, wakeCh chan<- struct{}) {
	for {
		if ctx.Err() != nil {
			return
		}
		err := r.consumeSSE(ctx, wakeCh)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("%s sse reconnect err=%v", r.cfg.LogPrefix, err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(1500 * time.Millisecond):
		}
	}
}

func (r *plexSessionReaper) consumeSSE(ctx context.Context, wakeCh chan<- struct{}) error {
	base := strings.TrimRight(r.cfg.PMSURL, "/")
	u := base + "/:/eventsource/notifications?X-Plex-Token=" + url.QueryEscape(r.cfg.Token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	resp, err := r.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sse status=%d", resp.StatusCode)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 4096), 256*1024)
	var eventName string
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" {
			if eventName != "" && eventName != "ping" {
				// Only player-facing activity renews timers; other events still wake scans.
				select {
				case wakeCh <- struct{}{}:
				default:
				}
			}
			eventName = ""
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return io.EOF
}

func (r *plexSessionReaper) scanAndReap(ctx context.Context, states map[string]*plexSessionReaperState) error {
	rows, err := r.listLiveSessions(ctx)
	if err != nil {
		return err
	}
	now := time.Now()

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

		sessionAge := now.Sub(st.firstSeen)
		idleAge := now.Sub(st.lastActivity)
		renewLeaseAge := now.Sub(st.lastRenewLease)

		idleReady := r.cfg.IdleTimeout > 0 && idleAge >= r.cfg.IdleTimeout
		renewReady := r.cfg.RenewLease > 0 && renewLeaseAge >= r.cfg.RenewLease
		hardReady := r.cfg.HardLease > 0 && sessionAge >= r.cfg.HardLease

		if idleReady || renewReady || hardReady {
			if !st.lastStopAttempt.IsZero() && now.Sub(st.lastStopAttempt) < 10*time.Second {
				continue
			}
			st.lastStopAttempt = now
			var why []string
			if idleReady {
				why = append(why, "idle>="+r.cfg.IdleTimeout.String())
			}
			if renewReady {
				why = append(why, "renew_lease>="+r.cfg.RenewLease.String())
			}
			if hardReady {
				why = append(why, "hard_lease>="+r.cfg.HardLease.String())
			}
			log.Printf("%s stop transcode=%s live=%s ip=%s client=%s/%s state=%s title=%q why=%s idle=%s renew=%s age=%s maxOffset=%.2f",
				r.cfg.LogPrefix, row.TranscodeID, row.LiveKey, row.PlayerAddr, row.PlayerProduct, row.PlayerPlatform, row.PlayerState, row.Title,
				strings.Join(why, ","), idleAge.Round(time.Millisecond), renewLeaseAge.Round(time.Millisecond), sessionAge.Round(time.Millisecond), row.MaxOffsetAvail)
			if row.TranscodeID != "" {
				code, stopErr := r.stopTranscode(ctx, row.TranscodeID)
				if stopErr != nil {
					log.Printf("%s stop transcode=%s err=%v", r.cfg.LogPrefix, row.TranscodeID, stopErr)
				} else {
					log.Printf("%s stop transcode=%s status=%d", r.cfg.LogPrefix, row.TranscodeID, code)
				}
			}
		}
	}

	for k := range states {
		if _, ok := activeKeys[k]; !ok {
			delete(states, k)
		}
	}
	return nil
}

func (r *plexSessionReaper) listLiveSessions(ctx context.Context) ([]plexLiveSessionRow, error) {
	base := strings.TrimRight(r.cfg.PMSURL, "/")
	u := base + "/status/sessions?X-Plex-Token=" + url.QueryEscape(r.cfg.Token)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/xml")
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("/status/sessions status=%d", resp.StatusCode)
	}
	return parsePlexLiveSessionRows(resp.Body, r.cfg.ScopeMachineID, r.cfg.ScopePlayerIP)
}

func (r *plexSessionReaper) stopTranscode(ctx context.Context, transcodeID string) (int, error) {
	base := strings.TrimRight(r.cfg.PMSURL, "/")
	u := base + "/video/:/transcode/universal/stop?session=" + url.QueryEscape(transcodeID) + "&X-Plex-Token=" + url.QueryEscape(r.cfg.Token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, strings.NewReader(""))
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	resp, err := r.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func parsePlexLiveSessionRows(rd io.Reader, scopeMachineID, scopePlayerIP string) ([]plexLiveSessionRow, error) {
	dec := xml.NewDecoder(rd)
	var out []plexLiveSessionRow
	var inVideo bool
	var row plexLiveSessionRow
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "Video":
				key := xmlStartAttr(t, "key")
				if !strings.HasPrefix(strings.TrimSpace(key), "/livetv/sessions/") {
					inVideo = false
					continue
				}
				inVideo = true
				row = plexLiveSessionRow{
					LiveKey: strings.TrimSpace(key),
					Title:   xmlStartAttr(t, "title"),
				}
			case "Player":
				if !inVideo {
					continue
				}
				row.PlayerAddr = xmlStartAttr(t, "address")
				row.PlayerProduct = xmlStartAttr(t, "product")
				row.PlayerPlatform = xmlStartAttr(t, "platform")
				row.PlayerDevice = xmlStartAttr(t, "device")
				row.MachineID = xmlStartAttr(t, "machineIdentifier")
				row.PlayerState = xmlStartAttr(t, "state")
			case "Session":
				if !inVideo {
					continue
				}
				row.SessionID = xmlStartAttr(t, "id")
			case "TranscodeSession":
				if !inVideo {
					continue
				}
				key := xmlStartAttr(t, "key")
				if strings.Contains(key, "/transcode/sessions/") {
					row.TranscodeID = key[strings.LastIndex(key, "/")+1:]
				}
				row.TranscodeTS = xmlStartAttr(t, "timeStamp")
				row.MaxOffsetAvail = parseFloat(xmlStartAttr(t, "maxOffsetAvailable"))
				row.MinOffsetAvail = parseFloat(xmlStartAttr(t, "minOffsetAvailable"))
			}
		case xml.EndElement:
			if t.Name.Local == "Video" && inVideo {
				inVideo = false
				if scopeMachineID != "" && row.MachineID != scopeMachineID {
					continue
				}
				if scopePlayerIP != "" && row.PlayerAddr != scopePlayerIP {
					continue
				}
				out = append(out, row)
			}
		}
	}
	return out, nil
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}
