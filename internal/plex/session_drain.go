package plex

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

// SessionRow describes an active Plex Live TV session.
type SessionRow struct {
	Title          string
	LiveKey        string
	SessionKey     string
	PlayerAddr     string
	PlayerProduct  string
	PlayerPlatform string
	PlayerDevice   string
	MachineID      string
	State          string
	TranscodeID    string
	SessionID      string
}

// SessionDrainConfig holds options for session drain/watch operations.
type SessionDrainConfig struct {
	PlexURL         string
	Token           string
	MachineID       string // filter: only this machineIdentifier
	PlayerIP        string // filter: only this player IP
	AllLive         bool   // drain all live sessions (default when no filter)
	DryRun          bool
	Poll            time.Duration
	Wait            time.Duration
	WatchMode       bool
	WatchFor        time.Duration // 0 = run forever
	SSE             bool          // subscribe to Plex SSE notifications in watch mode
	IdleAfter       time.Duration // stop when no activity for this long (needs LogCmd)
	RenewLeaseAfter time.Duration // renewable heartbeat lease
	LeaseAfter      time.Duration // hard backstop
	LogLookback     time.Duration // how far back to look in Plex logs per poll
	LogCmd          string        // shell command to fetch recent logs; {since} = seconds
}

// plexXMLSessions is an internal XML model for /status/sessions.
type plexXMLSessions struct {
	Videos []struct {
		XMLName    xml.Name `xml:"Video"`
		Key        string   `xml:"key,attr"`
		Title      string   `xml:"title,attr"`
		SessionKey string   `xml:"sessionKey,attr"`
		Player     *struct {
			Address           string `xml:"address,attr"`
			Product           string `xml:"product,attr"`
			Platform          string `xml:"platform,attr"`
			Device            string `xml:"device,attr"`
			MachineIdentifier string `xml:"machineIdentifier,attr"`
			State             string `xml:"state,attr"`
		} `xml:"Player"`
		TranscodeSession *struct {
			Key string `xml:"key,attr"`
		} `xml:"TranscodeSession"`
		Session *struct {
			ID string `xml:"id,attr"`
		} `xml:"Session"`
	} `xml:"Video"`
}

func plexReq(method, baseURL, token, path string, body io.Reader) (*http.Response, error) {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	u := strings.TrimRight(baseURL, "/") + path + sep + "X-Plex-Token=" + url.QueryEscape(token)
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/xml")
	return http.DefaultClient.Do(req)
}

// ListLiveSessions fetches /status/sessions and returns live TV sessions.
func ListLiveSessions(plexURL, token string) ([]SessionRow, error) {
	resp, err := plexReq("GET", plexURL, token, "/status/sessions", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)

	var mc struct {
		XMLName xml.Name `xml:"MediaContainer"`
		plexXMLSessions
	}
	if err := xml.Unmarshal(data, &mc); err != nil {
		return nil, fmt.Errorf("parse /status/sessions: %w", err)
	}

	var rows []SessionRow
	for _, v := range mc.Videos {
		if !strings.HasPrefix(v.Key, "/livetv/sessions/") {
			continue
		}
		row := SessionRow{
			Title:      v.Title,
			LiveKey:    strings.TrimSpace(v.Key),
			SessionKey: v.SessionKey,
		}
		if v.Player != nil {
			row.PlayerAddr = v.Player.Address
			row.PlayerProduct = v.Player.Product
			row.PlayerPlatform = v.Player.Platform
			row.PlayerDevice = v.Player.Device
			row.MachineID = v.Player.MachineIdentifier
			row.State = v.Player.State
		}
		if v.TranscodeSession != nil {
			if idx := strings.LastIndex(v.TranscodeSession.Key, "/"); idx >= 0 {
				if strings.Contains(v.TranscodeSession.Key, "/transcode/sessions/") {
					row.TranscodeID = v.TranscodeSession.Key[idx+1:]
				}
			}
		}
		if v.Session != nil {
			row.SessionID = v.Session.ID
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// StopTranscode stops a Plex transcode session by ID.
func StopTranscode(plexURL, token, transcodeID string) (int, error) {
	path := "/video/:/transcode/universal/stop?session=" + url.QueryEscape(transcodeID)
	resp, err := plexReq("PUT", plexURL, token, path, strings.NewReader(""))
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

var _reqRe = regexp.MustCompile(`\[(\d+\.\d+\.\d+\.\d+):\d+[^\]]*\]\s+\S+\s+(/\S+)`)

// logHasActivity checks if the PMS log text contains any client-facing request
// matching the given session row.
func logHasActivity(row SessionRow, logText string) bool {
	liveUUID := ""
	if idx := strings.LastIndex(row.LiveKey, "/"); idx >= 0 {
		liveUUID = row.LiveKey[idx+1:]
	}
	livePathFrag := ""
	if liveUUID != "" {
		livePathFrag = "/livetv/sessions/" + liveUUID + "/"
	}
	sc := bufio.NewScanner(strings.NewReader(logText))
	for sc.Scan() {
		line := sc.Text()
		m := _reqRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ip, path := m[1], m[2]
		if livePathFrag != "" && strings.Contains(path, livePathFrag) {
			return true
		}
		if row.TranscodeID != "" && strings.Contains(path, "/transcode/universal/session/"+row.TranscodeID+"/") {
			return true
		}
		if row.PlayerAddr != "" && ip == row.PlayerAddr {
			if strings.HasPrefix(path, "/:/timeline") || strings.HasSuffix(path, "/start.mpd") {
				return true
			}
		}
	}
	return false
}

// SessionWatcher watches live sessions and stops them when idle/lease conditions are met.
type SessionWatcher struct {
	cfg                SessionDrainConfig
	idleLastAct        map[string]time.Time
	firstSeen          map[string]time.Time
	renewLeaseLastSeen map[string]time.Time
	mu                 sync.Mutex
	sseLastEvent       time.Time
}

// NewSessionWatcher creates a watcher ready to run.
func NewSessionWatcher(cfg SessionDrainConfig) *SessionWatcher {
	return &SessionWatcher{
		cfg:                cfg,
		idleLastAct:        map[string]time.Time{},
		firstSeen:          map[string]time.Time{},
		renewLeaseLastSeen: map[string]time.Time{},
	}
}

func (w *SessionWatcher) matchRows(all []SessionRow) []SessionRow {
	var out []SessionRow
	for _, r := range all {
		if w.cfg.MachineID != "" && r.MachineID != w.cfg.MachineID {
			continue
		}
		if w.cfg.PlayerIP != "" && r.PlayerAddr != w.cfg.PlayerIP {
			continue
		}
		out = append(out, r)
	}
	return out
}

func sessionKey(r SessionRow) string {
	if r.TranscodeID != "" {
		return r.TranscodeID
	}
	return r.LiveKey
}

// Drain performs a one-shot drain: stop matching sessions and wait for them to clear.
// Writer receives human-readable status lines.
func (w *SessionWatcher) Drain(logFn func(string)) error {
	all, err := ListLiveSessions(w.cfg.PlexURL, w.cfg.Token)
	if err != nil {
		return err
	}
	rows := w.matchRows(all)
	logFn(fmt.Sprintf("LIVE_MATCHED %d", len(rows)))
	for _, r := range rows {
		logFn(fmt.Sprintf("LIVE machine=%s ip=%s client=%s/%s/%s state=%s transcode=%s title=%s",
			r.MachineID, r.PlayerAddr, r.PlayerProduct, r.PlayerPlatform, r.PlayerDevice,
			r.State, r.TranscodeID, r.Title))
	}
	if w.cfg.DryRun {
		return nil
	}

	seen := map[string]bool{}
	for _, r := range rows {
		tid := r.TranscodeID
		if tid == "" || seen[tid] {
			continue
		}
		seen[tid] = true
		code, err := StopTranscode(w.cfg.PlexURL, w.cfg.Token, tid)
		if err != nil {
			logFn(fmt.Sprintf("STOP transcode=%s err=%v", tid, err))
		} else {
			logFn(fmt.Sprintf("STOP transcode=%s status=%d", tid, code))
		}
	}

	deadline := time.Now().Add(w.cfg.Wait)
	for time.Now().Before(deadline) {
		remain, _ := ListLiveSessions(w.cfg.PlexURL, w.cfg.Token)
		if len(w.matchRows(remain)) == 0 {
			logFn("DRAIN OK remaining=0")
			return nil
		}
		time.Sleep(w.cfg.Poll)
	}
	remain, _ := ListLiveSessions(w.cfg.PlexURL, w.cfg.Token)
	left := w.matchRows(remain)
	logFn(fmt.Sprintf("DRAIN TIMEOUT remaining=%d", len(left)))
	for _, r := range left {
		logFn(fmt.Sprintf("REMAIN machine=%s ip=%s transcode=%s", r.MachineID, r.PlayerAddr, r.TranscodeID))
	}
	return fmt.Errorf("drain timeout: %d sessions remain", len(left))
}

// Watch continuously monitors and stops sessions when idle/lease conditions fire.
// Runs until ctx is cancelled or watchFor elapses. Writer receives status lines.
func (w *SessionWatcher) Watch(stop <-chan struct{}, logFn func(string)) {
	useSSE := w.cfg.SSE && (w.cfg.IdleAfter > 0 || w.cfg.RenewLeaseAfter > 0)
	kick := make(chan struct{}, 1)
	if useSSE {
		go w.runSSE(stop, kick)
		logFn("WATCH sse=on")
	} else {
		logFn("WATCH sse=off")
	}

	started := time.Now()
	for {
		select {
		case <-stop:
			return
		default:
		}

		all, err := ListLiveSessions(w.cfg.PlexURL, w.cfg.Token)
		if err != nil {
			logFn(fmt.Sprintf("WATCH err=%v", err))
		} else {
			rows := w.matchRows(all)
			w.annotate(rows, logFn)
			w.reap(rows, logFn)
		}

		if w.cfg.WatchFor > 0 && time.Since(started) >= w.cfg.WatchFor {
			logFn("WATCH_DONE runtime")
			return
		}

		select {
		case <-stop:
			return
		case <-kick:
		case <-time.After(w.cfg.Poll):
		}
	}
}

func (w *SessionWatcher) annotate(rows []SessionRow, logFn func(string)) {
	now := time.Now()
	active := map[string]bool{}
	for _, r := range rows {
		k := sessionKey(r)
		if k == "" {
			continue
		}
		active[k] = true
		w.mu.Lock()
		if _, ok := w.idleLastAct[k]; !ok {
			w.idleLastAct[k] = now
		}
		if _, ok := w.firstSeen[k]; !ok {
			w.firstSeen[k] = now
		}
		if _, ok := w.renewLeaseLastSeen[k]; !ok {
			w.renewLeaseLastSeen[k] = now
		}
		w.mu.Unlock()
	}
	// Prune stale state.
	w.mu.Lock()
	for k := range w.idleLastAct {
		if !active[k] {
			delete(w.idleLastAct, k)
			delete(w.firstSeen, k)
			delete(w.renewLeaseLastSeen, k)
		}
	}
	w.mu.Unlock()
	for _, r := range rows {
		idleAge := w.idleAge(r)
		sessAge := w.sessionAge(r)
		renewAge := w.renewLeaseAge(r)
		logFn(fmt.Sprintf(
			"WATCH machine=%s ip=%s transcode=%s state=%s idle_age=%.1fs session_age=%.1fs renew_lease_age=%.1fs title=%s",
			r.MachineID, r.PlayerAddr, r.TranscodeID, r.State,
			idleAge.Seconds(), sessAge.Seconds(), renewAge.Seconds(), r.Title))
	}
}

func (w *SessionWatcher) idleAge(r SessionRow) time.Duration {
	k := sessionKey(r)
	w.mu.Lock()
	t, ok := w.idleLastAct[k]
	w.mu.Unlock()
	if !ok {
		return 0
	}
	return time.Since(t)
}

func (w *SessionWatcher) sessionAge(r SessionRow) time.Duration {
	k := sessionKey(r)
	w.mu.Lock()
	t, ok := w.firstSeen[k]
	w.mu.Unlock()
	if !ok {
		return 0
	}
	return time.Since(t)
}

func (w *SessionWatcher) renewLeaseAge(r SessionRow) time.Duration {
	k := sessionKey(r)
	w.mu.Lock()
	t, ok := w.renewLeaseLastSeen[k]
	w.mu.Unlock()
	if !ok {
		return 0
	}
	return time.Since(t)
}

func (w *SessionWatcher) markActivity(r SessionRow) {
	k := sessionKey(r)
	now := time.Now()
	w.mu.Lock()
	w.idleLastAct[k] = now
	w.renewLeaseLastSeen[k] = now
	w.mu.Unlock()
}

func (w *SessionWatcher) markSSEActivity() {
	w.mu.Lock()
	w.sseLastEvent = time.Now()
	w.mu.Unlock()
}

func (w *SessionWatcher) reap(rows []SessionRow, logFn func(string)) {
	seen := map[string]bool{}
	for _, r := range rows {
		idleReady := w.cfg.IdleAfter > 0 && w.idleAge(r) >= w.cfg.IdleAfter
		renewReady := w.cfg.RenewLeaseAfter > 0 && w.renewLeaseAge(r) >= w.cfg.RenewLeaseAfter
		leaseReady := w.cfg.LeaseAfter > 0 && w.sessionAge(r) >= w.cfg.LeaseAfter
		if !idleReady && !renewReady && !leaseReady {
			continue
		}
		tid := r.TranscodeID
		if tid == "" || seen[tid] {
			continue
		}
		seen[tid] = true
		var why []string
		if idleReady {
			why = append(why, fmt.Sprintf("idle>=%.0fs", w.cfg.IdleAfter.Seconds()))
		}
		if renewReady {
			why = append(why, fmt.Sprintf("renew_lease>=%.0fs", w.cfg.RenewLeaseAfter.Seconds()))
		}
		if leaseReady {
			why = append(why, fmt.Sprintf("lease>=%.0fs", w.cfg.LeaseAfter.Seconds()))
		}
		if w.cfg.DryRun {
			logFn(fmt.Sprintf("WATCH_STOP_DRY transcode=%s why=%s", tid, strings.Join(why, ",")))
			continue
		}
		code, err := StopTranscode(w.cfg.PlexURL, w.cfg.Token, tid)
		if err != nil {
			logFn(fmt.Sprintf("WATCH_STOP transcode=%s err=%v why=%s", tid, err, strings.Join(why, ",")))
		} else {
			logFn(fmt.Sprintf("WATCH_STOP transcode=%s status=%d why=%s", tid, code, strings.Join(why, ",")))
		}
	}
}

func (w *SessionWatcher) runSSE(stop <-chan struct{}, kick chan<- struct{}) {
	u := strings.TrimRight(w.cfg.PlexURL, "/") +
		"/:/eventsource/notifications?X-Plex-Token=" + url.QueryEscape(w.cfg.Token)
	playbackEvents := map[string]bool{"activity": true, "playing": true, "timeline": true}
	for {
		select {
		case <-stop:
			return
		default:
		}
		func() {
			resp, err := http.Get(u) //nolint:noctx
			if err != nil {
				time.Sleep(time.Second)
				return
			}
			defer resp.Body.Close()
			sc := bufio.NewScanner(resp.Body)
			var eventName string
			for sc.Scan() {
				select {
				case <-stop:
					return
				default:
				}
				line := sc.Text()
				if line == "" {
					if eventName != "" && eventName != "ping" {
						if playbackEvents[eventName] {
							w.markSSEActivity()
						}
						select {
						case kick <- struct{}{}:
						default:
						}
					}
					eventName = ""
					continue
				}
				if strings.HasPrefix(line, "event:") {
					eventName = strings.TrimSpace(line[6:])
				}
			}
		}()
		time.Sleep(time.Second)
	}
}
