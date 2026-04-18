package tuner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

const streamMuxHLSPackager = "hlspkg"

type ffmpegHLSPackagerSession struct {
	id           string
	reuseKey     string
	channelID    string
	channelName  string
	dir          string
	playlistPath string
	segmentGlobs []string
	cancel       context.CancelFunc
	cmd          *exec.Cmd
	createdAt    time.Time
	lastAccess   time.Time
	tunerHeld    bool

	mu      sync.Mutex
	waitErr error
	exited  bool
}

func (s *ffmpegHLSPackagerSession) touch(now time.Time) {
	s.mu.Lock()
	s.lastAccess = now
	s.mu.Unlock()
}

func (s *ffmpegHLSPackagerSession) markExit(err error) {
	s.mu.Lock()
	s.waitErr = err
	s.exited = true
	s.mu.Unlock()
}

func (s *ffmpegHLSPackagerSession) snapshot() (createdAt, lastAccess time.Time, exited bool, waitErr error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createdAt, s.lastAccess, s.exited, s.waitErr
}

func hlsPackagerReuseKey(channelID string, profile resolvedStreamProfile) string {
	return strings.Join([]string{
		strings.TrimSpace(channelID),
		strings.TrimSpace(profile.Name),
		strings.TrimSpace(profile.BaseProfile),
		strconv.FormatBool(profile.ForceTranscode),
		normalizeStreamOutputMuxName(profile.OutputMux),
	}, "\x1f")
}

func hlsPackagerStartupTimeout() time.Duration {
	ms := getenvInt("IPTV_TUNERR_FFMPEG_HLS_PACKAGER_STARTUP_TIMEOUT_MS", 8000)
	if ms < 100 {
		ms = 100
	}
	return time.Duration(ms) * time.Millisecond
}

func hlsPackagerFileWaitTimeout() time.Duration {
	ms := getenvInt("IPTV_TUNERR_FFMPEG_HLS_PACKAGER_FILE_WAIT_TIMEOUT_MS", 4000)
	if ms < 100 {
		ms = 100
	}
	return time.Duration(ms) * time.Millisecond
}

func hlsPackagerPollInterval() time.Duration {
	ms := getenvInt("IPTV_TUNERR_FFMPEG_HLS_PACKAGER_POLL_MS", 100)
	if ms < 20 {
		ms = 20
	}
	return time.Duration(ms) * time.Millisecond
}

func hlsPackagerIdleTimeout() time.Duration {
	sec := getenvInt("IPTV_TUNERR_FFMPEG_HLS_PACKAGER_IDLE_SEC", 45)
	if sec < 5 {
		sec = 5
	}
	return time.Duration(sec) * time.Second
}

func hlsPackagerMaxAge() time.Duration {
	sec := getenvInt("IPTV_TUNERR_FFMPEG_HLS_PACKAGER_MAX_AGE_SEC", 300)
	if sec < 15 {
		sec = 15
	}
	return time.Duration(sec) * time.Second
}

func hlsPackagerJanitorInterval() time.Duration {
	sec := getenvInt("IPTV_TUNERR_FFMPEG_HLS_PACKAGER_JANITOR_SEC", 15)
	if sec < 5 {
		sec = 5
	}
	return time.Duration(sec) * time.Second
}

func hlsPackagerListSize() int {
	n := getenvInt("IPTV_TUNERR_FFMPEG_HLS_PACKAGER_LIST_SIZE", 6)
	if n < 2 {
		n = 2
	}
	return n
}

func hlsPackagerSegmentSeconds() int {
	n := getenvInt("IPTV_TUNERR_FFMPEG_HLS_PACKAGER_SEGMENT_SECONDS", 2)
	if n < 1 {
		n = 1
	}
	return n
}

func hlsPackagerBaseDir() string {
	return strings.TrimSpace(os.Getenv("IPTV_TUNERR_FFMPEG_HLS_PACKAGER_DIR"))
}

func gatewayHLSPackagerProxyURL(channelID, sessionID, file string) string {
	rel := "/stream/" + url.PathEscape(channelID) + "?mux=" + url.QueryEscape(streamMuxHLSPackager) + "&sid=" + url.QueryEscape(sessionID) + "&file=" + url.QueryEscape(file)
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("IPTV_TUNERR_STREAM_PUBLIC_BASE_URL")), "/")
	if base == "" {
		return rel
	}
	return base + rel
}

func rewritePackagedHLSPlaylist(body []byte, channelID, sessionID string) []byte {
	lines := strings.Split(strings.ReplaceAll(string(body), "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" {
			out = append(out, line)
			continue
		}
		if strings.HasPrefix(trim, "#") {
			if strings.Contains(line, "URI=\"") {
				line = hlsQuotedURIAttr.ReplaceAllStringFunc(line, func(m string) string {
					sub := hlsQuotedURIAttr.FindStringSubmatch(m)
					if len(sub) != 4 {
						return m
					}
					ref := strings.TrimSpace(sub[2])
					if ref == "" || safeurl.IsHTTPOrHTTPS(ref) || strings.HasPrefix(ref, "/") {
						return m
					}
					return sub[1] + gatewayHLSPackagerProxyURL(channelID, sessionID, ref) + sub[3]
				})
			}
			if strings.Contains(line, "URI='") {
				line = hlsQuotedURIAttrSingle.ReplaceAllStringFunc(line, func(m string) string {
					sub := hlsQuotedURIAttrSingle.FindStringSubmatch(m)
					if len(sub) != 4 {
						return m
					}
					ref := strings.TrimSpace(sub[2])
					if ref == "" || safeurl.IsHTTPOrHTTPS(ref) || strings.HasPrefix(ref, "/") {
						return m
					}
					return sub[1] + gatewayHLSPackagerProxyURL(channelID, sessionID, ref) + sub[3]
				})
			}
			out = append(out, line)
			continue
		}
		if safeurl.IsHTTPOrHTTPS(trim) || strings.HasPrefix(trim, "/") {
			out = append(out, line)
			continue
		}
		out = append(out, gatewayHLSPackagerProxyURL(channelID, sessionID, trim))
	}
	return []byte(strings.Join(out, "\n"))
}

func (g *Gateway) startHLSPackagerJanitor() {
	if g == nil {
		return
	}
	g.hlsPackagerJanitorOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(hlsPackagerJanitorInterval())
			defer ticker.Stop()
			for range ticker.C {
				g.cleanupExpiredHLSPackagerSessions()
			}
		}()
	})
}

func (g *Gateway) cleanupExpiredHLSPackagerSessions() {
	if g == nil {
		return
	}
	now := time.Now()
	idle := hlsPackagerIdleTimeout()
	maxAge := hlsPackagerMaxAge()
	var expired []*ffmpegHLSPackagerSession
	g.mu.Lock()
	for id, sess := range g.hlsPackagerSessions {
		if sess == nil {
			delete(g.hlsPackagerSessions, id)
			continue
		}
		createdAt, lastAccess, exited, _ := sess.snapshot()
		if now.Sub(lastAccess) < idle && now.Sub(createdAt) < maxAge && !(exited && now.Sub(lastAccess) >= 5*time.Second) {
			continue
		}
		g.removeHLSPackagerSessionLocked(id, sess)
		expired = append(expired, sess)
	}
	g.mu.Unlock()
	for _, sess := range expired {
		g.stopHLSPackagerSession(sess, "expired")
	}
}

func (g *Gateway) stopHLSPackagerSession(sess *ffmpegHLSPackagerSession, reason string) {
	if sess == nil {
		return
	}
	if sess.cancel != nil {
		sess.cancel()
	}
	if sess.cmd != nil && sess.cmd.Process != nil {
		_ = sess.cmd.Process.Kill()
	}
	if sess.dir != "" {
		if err := os.RemoveAll(sess.dir); err != nil {
			log.Printf("gateway: channel=%q id=%s hls-packager cleanup reason=%s dir=%q err=%v",
				sess.channelName, sess.channelID, reason, sess.dir, err)
		}
	}
}

func (g *Gateway) removeHLSPackagerSessionLocked(sessionID string, sess *ffmpegHLSPackagerSession) {
	if g.hlsPackagerSessions != nil && sessionID != "" {
		delete(g.hlsPackagerSessions, sessionID)
	}
	if sess != nil && sess.reuseKey != "" && g.hlsPackagerSessionsByKey != nil {
		if current := g.hlsPackagerSessionsByKey[sess.reuseKey]; current == sess {
			delete(g.hlsPackagerSessionsByKey, sess.reuseKey)
		}
	}
	if sess != nil && sess.tunerHeld && g.hlsPackagerInUse > 0 {
		g.hlsPackagerInUse--
		sess.tunerHeld = false
	}
}

func (g *Gateway) registerHLSPackagerSession(sess *ffmpegHLSPackagerSession) {
	if g == nil || sess == nil {
		return
	}
	g.startHLSPackagerJanitor()
	g.mu.Lock()
	if g.hlsPackagerSessions == nil {
		g.hlsPackagerSessions = make(map[string]*ffmpegHLSPackagerSession)
	}
	if g.hlsPackagerSessionsByKey == nil {
		g.hlsPackagerSessionsByKey = make(map[string]*ffmpegHLSPackagerSession)
	}
	g.hlsPackagerSessions[sess.id] = sess
	if sess.reuseKey != "" {
		g.hlsPackagerSessionsByKey[sess.reuseKey] = sess
	}
	g.hlsPackagerInUse++
	sess.tunerHeld = true
	g.mu.Unlock()
}

func (g *Gateway) unregisterHLSPackagerSession(sessionID, reason string) {
	if g == nil || sessionID == "" {
		return
	}
	var sess *ffmpegHLSPackagerSession
	g.mu.Lock()
	if g.hlsPackagerSessions != nil {
		sess = g.hlsPackagerSessions[sessionID]
	}
	if sess != nil {
		g.removeHLSPackagerSessionLocked(sessionID, sess)
	}
	g.mu.Unlock()
	g.stopHLSPackagerSession(sess, reason)
}

func (g *Gateway) lookupHLSPackagerSession(sessionID string) *ffmpegHLSPackagerSession {
	if g == nil || sessionID == "" {
		return nil
	}
	g.cleanupExpiredHLSPackagerSessions()
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.hlsPackagerSessions == nil {
		return nil
	}
	return g.hlsPackagerSessions[sessionID]
}

func (g *Gateway) lookupReusableHLSPackagerSession(reuseKey string) *ffmpegHLSPackagerSession {
	if g == nil || reuseKey == "" {
		return nil
	}
	g.cleanupExpiredHLSPackagerSessions()
	var stale *ffmpegHLSPackagerSession
	g.mu.Lock()
	if g.hlsPackagerSessionsByKey != nil {
		if sess := g.hlsPackagerSessionsByKey[reuseKey]; sess != nil {
			_, _, exited, _ := sess.snapshot()
			if exited {
				stale = sess
				g.removeHLSPackagerSessionLocked(sess.id, sess)
			} else {
				g.mu.Unlock()
				return sess
			}
		}
	}
	g.mu.Unlock()
	if stale != nil {
		g.stopHLSPackagerSession(stale, "exited")
	}
	return nil
}

func newHLSPackagerSessionID(channelID string) string {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		channelID = "stream"
	}
	now := strconv.FormatInt(time.Now().UnixNano(), 36)
	return channelID + "-" + now
}

func createHLSPackagerDir() (string, error) {
	base := hlsPackagerBaseDir()
	if base != "" {
		if err := os.MkdirAll(base, 0755); err != nil {
			return "", err
		}
	}
	return os.MkdirTemp(base, "iptvtunerr-hlspkg-*")
}

func buildFFmpegHLSPackageOutputArgs(transcode bool, profile string, playlistPath string) []string {
	if playlistPath == "" {
		return nil
	}
	segPattern := filepath.Join(filepath.Dir(playlistPath), "seg-%06d.ts")
	args := buildFFmpegStreamCodecArgs(transcode, profile, streamMuxMPEGTS)
	flags := []string{"delete_segments", "independent_segments", "omit_endlist", "temp_file"}
	args = append(args,
		"-flush_packets", "1",
		"-max_interleave_delta", "0",
		"-hls_time", strconv.Itoa(hlsPackagerSegmentSeconds()),
		"-hls_list_size", strconv.Itoa(hlsPackagerListSize()),
		"-hls_allow_cache", "0",
		"-hls_flags", strings.Join(flags, "+"),
		"-hls_segment_filename", segPattern,
		"-f", "hls",
		playlistPath,
	)
	return args
}

func (g *Gateway) startFFmpegPackagedHLS(
	r *http.Request,
	ffmpegPath string,
	playlistURL string,
	channelName string,
	channelID string,
	profile resolvedStreamProfile,
) (*ffmpegHLSPackagerSession, error) {
	dir, err := createHLSPackagerDir()
	if err != nil {
		return nil, err
	}
	playlistPath := filepath.Join(dir, "index.m3u8")
	ctx, cancel := context.WithCancel(r.Context())
	ffmpegPlaylistURL, ffmpegInputHost, ffmpegInputIP := canonicalizeFFmpegInputURL(r.Context(), playlistURL, g.DisableFFmpegDNS)
	hlsAnalyzeDurationUs := getenvInt("IPTV_TUNERR_FFMPEG_HLS_ANALYZEDURATION_US", 5000000)
	hlsProbeSize := getenvInt("IPTV_TUNERR_FFMPEG_HLS_PROBESIZE", 5000000)
	hlsRWTimeoutUs := getenvInt("IPTV_TUNERR_FFMPEG_HLS_RW_TIMEOUT_US", 60000000)
	hlsLiveStartIndex := ffmpegHLSLiveStartIndex()
	hlsReconnect := getenvBool("IPTV_TUNERR_FFMPEG_HLS_RECONNECT", false)
	hlsHTTPPersistent := ffmpegHLSHTTPPersistentEnabled()
	hlsMultipleRequests := getenvBool("IPTV_TUNERR_FFMPEG_HLS_MULTIPLE_REQUESTS", true)
	if g.shouldAutoEnableHLSReconnect() {
		hlsReconnect = true
	}
	hlsLogLevel := strings.TrimSpace(os.Getenv("IPTV_TUNERR_FFMPEG_HLS_LOGLEVEL"))
	if hlsLogLevel == "" {
		hlsLogLevel = "error"
	}
	args := []string{
		"-nostdin",
		"-hide_banner",
		"-loglevel", hlsLogLevel,
		"-fflags", "+discardcorrupt+genpts",
		"-analyzeduration", strconv.Itoa(hlsAnalyzeDurationUs),
		"-probesize", strconv.Itoa(hlsProbeSize),
		"-rw_timeout", strconv.Itoa(hlsRWTimeoutUs),
		"-user_agent", g.effectiveUpstreamUserAgent(r),
	}
	if hlsHTTPPersistent {
		args = append(args, "-http_persistent", "1")
	}
	if hlsMultipleRequests {
		args = append(args, "-multiple_requests", "1")
	}
	if referer := g.effectiveUpstreamReferer(r); referer != "" {
		args = append(args, "-referer", referer)
	}
	if cookies := g.ffmpegCookiesOptionForURL(playlistURL); cookies != "" {
		args = append(args, "-cookies", cookies)
	}
	if hlsReconnect {
		args = append(args,
			"-reconnect", "1",
			"-reconnect_streamed", "1",
			"-reconnect_at_eof", "1",
			"-reconnect_on_network_error", "1",
			"-reconnect_delay_max", "2",
		)
	}
	if hlsLiveStartIndex != 0 {
		args = append(args, "-live_start_index", strconv.Itoa(hlsLiveStartIndex))
	}
	if headers := g.ffmpegInputHeaderBlock(r, playlistURL, ffmpegInputHost); headers != "" {
		args = append(args, "-headers", headers)
	}
	args = append(args, "-i", ffmpegPlaylistURL)
	args = append(args, buildFFmpegHLSPackageOutputArgs(profile.ForceTranscode, profile.BaseProfile, playlistPath)...)
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		cancel()
		_ = os.RemoveAll(dir)
		return nil, err
	}
	sess := &ffmpegHLSPackagerSession{
		id:           newHLSPackagerSessionID(channelID),
		reuseKey:     hlsPackagerReuseKey(channelID, profile),
		channelID:    channelID,
		channelName:  channelName,
		dir:          dir,
		playlistPath: playlistPath,
		cancel:       cancel,
		cmd:          cmd,
		createdAt:    time.Now(),
		lastAccess:   time.Now(),
		segmentGlobs: []string{filepath.Join(dir, "seg-*.ts"), filepath.Join(dir, "seg-*.tmp")},
	}
	go func() {
		err := cmd.Wait()
		sess.markExit(err)
		msg := strings.TrimSpace(stderr.String())
		if msg == "" && err == nil {
			log.Printf("gateway: channel=%q id=%s hls-packager exited cleanly", channelName, channelID)
			return
		}
		if len(msg) > 500 {
			msg = msg[:500] + "..."
		}
		log.Printf("gateway: channel=%q id=%s hls-packager exited err=%v stderr=%q", channelName, channelID, err, msg)
	}()
	if ffmpegInputHost != "" && ffmpegInputIP != "" {
		log.Printf("gateway: channel=%q id=%s hls-packager input-host-resolved %q=>%q", channelName, channelID, ffmpegInputHost, ffmpegInputIP)
	}
	return sess, nil
}

func (g *Gateway) serveFFmpegPackagedHLSPlaylist(w http.ResponseWriter, channelID string, sess *ffmpegHLSPackagerSession, shared bool) error {
	if sess == nil {
		return errors.New("missing packaged hls session")
	}
	sess.touch(time.Now())
	if err := waitForReadableFile(sess.playlistPath, hlsPackagerStartupTimeout()); err != nil {
		return err
	}
	body, err := os.ReadFile(sess.playlistPath)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-store")
	if shared {
		w.Header().Set("X-IptvTunerr-Shared-Upstream", "ffmpeg_hls_packager")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(rewritePackagedHLSPlaylist(body, channelID, sess.id))
	return nil
}

func waitForReadableFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		info, err := os.Stat(path)
		if err == nil && info.Size() > 0 {
			return nil
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if time.Now().After(deadline) {
			if err != nil {
				return err
			}
			return fmt.Errorf("timeout waiting for %s", path)
		}
		time.Sleep(hlsPackagerPollInterval())
	}
}

func packagedHLSFilePath(sess *ffmpegHLSPackagerSession, file string) (string, error) {
	if sess == nil {
		return "", errors.New("missing session")
	}
	name := strings.TrimSpace(file)
	if name == "" {
		name = "index.m3u8"
	}
	clean := strings.TrimPrefix(filepath.Clean("/"+name), "/")
	if clean == "." || clean == "" {
		return "", errors.New("invalid packaged file")
	}
	full := filepath.Join(sess.dir, filepath.FromSlash(clean))
	rel, err := filepath.Rel(sess.dir, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", errors.New("invalid packaged file path")
	}
	return full, nil
}

func (g *Gateway) serveFFmpegPackagedHLSInitial(
	w http.ResponseWriter,
	r *http.Request,
	channelName string,
	channelID string,
	playlistURL string,
	profile resolvedStreamProfile,
) error {
	ffmpegPath, err := resolveFFmpegPath()
	if err != nil {
		return err
	}
	sess, err := g.startFFmpegPackagedHLS(r, ffmpegPath, playlistURL, channelName, channelID, profile)
	if err != nil {
		return err
	}
	g.registerHLSPackagerSession(sess)
	if err := g.serveFFmpegPackagedHLSPlaylist(w, channelID, sess, false); err != nil {
		g.unregisterHLSPackagerSession(sess.id, "startup_failed")
		return err
	}
	return nil
}

func (g *Gateway) maybeServeFFmpegPackagedHLSTarget(w http.ResponseWriter, r *http.Request, channelID string) bool {
	if strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mux"))) != streamMuxHLSPackager {
		return false
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeMethodNotAllowed(w, http.MethodGet, http.MethodHead)
		return true
	}
	sessionID := strings.TrimSpace(r.URL.Query().Get("sid"))
	if sessionID == "" {
		http.Error(w, "missing packaged hls session", http.StatusBadRequest)
		return true
	}
	sess := g.lookupHLSPackagerSession(sessionID)
	if sess == nil || sess.channelID != channelID {
		http.NotFound(w, r)
		return true
	}
	sess.touch(time.Now())
	filePath, err := packagedHLSFilePath(sess, r.URL.Query().Get("file"))
	if err != nil {
		http.Error(w, "invalid packaged hls file", http.StatusBadRequest)
		return true
	}
	if strings.HasSuffix(strings.ToLower(filePath), ".m3u8") {
		if err := waitForReadableFile(filePath, hlsPackagerFileWaitTimeout()); err != nil {
			http.Error(w, "packaged playlist unavailable", http.StatusBadGateway)
			return true
		}
		body, err := os.ReadFile(filePath)
		if err != nil {
			http.Error(w, "packaged playlist unavailable", http.StatusBadGateway)
			return true
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(rewritePackagedHLSPlaylist(body, channelID, sessionID))
		return true
	}
	if err := waitForReadableFile(filePath, hlsPackagerFileWaitTimeout()); err != nil {
		http.Error(w, "packaged segment unavailable", http.StatusBadGateway)
		return true
	}
	w.Header().Set("Cache-Control", "no-store")
	if strings.HasSuffix(strings.ToLower(filePath), ".ts") {
		w.Header().Set("Content-Type", "video/mp2t")
	}
	http.ServeFile(w, r, filePath)
	return true
}
