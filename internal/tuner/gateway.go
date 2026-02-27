package tuner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"

	"github.com/plextuner/plex-tuner/internal/catalog"
	"github.com/plextuner/plex-tuner/internal/httpclient"
	"github.com/plextuner/plex-tuner/internal/safeurl"
)

// Gateway proxies live stream requests to provider URLs with optional auth.
// Limit concurrent streams to TunerCount (tuner semantics).
type Gateway struct {
	Channels            []catalog.LiveChannel
	ProviderUser        string
	ProviderPass        string
	TunerCount          int
	StreamBufferBytes   int    // 0 = no buffer, -1 = auto
	StreamTranscodeMode string // "off" | "on" | "auto"
	TranscodeOverrides  map[string]bool
	DefaultProfile      string
	ProfileOverrides    map[string]string
	Client              *http.Client
	PlexPMSURL          string
	PlexPMSToken        string
	PlexClientAdapt     bool
	mu                  sync.Mutex
	inUse               int
	reqSeq              uint64
}

// ActiveStreams returns the number of streams currently being served.
// Used by background workers (e.g. SDT prober) to yield when users are watching.
func (g *Gateway) ActiveStreams() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.inUse
}

type gatewayReqIDKey struct{}

func gatewayReqIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if v, ok := ctx.Value(gatewayReqIDKey{}).(string); ok {
		return v
	}
	return ""
}

func gatewayReqIDField(ctx context.Context) string {
	if id := gatewayReqIDFromContext(ctx); id != "" {
		return " req=" + id
	}
	return ""
}

func getenvInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getenvFloat(key string, def float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func getenvBool(key string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return def
	}
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func mpegTSFlagsWithOptionalInitialDiscontinuity() string {
	flags := []string{"resend_headers", "pat_pmt_at_frames"}
	if getenvBool("PLEX_TUNER_MPEGTS_INITIAL_DISCONTINUITY", true) {
		flags = append(flags, "initial_discontinuity")
	}
	return "+" + strings.Join(flags, "+")
}

func isClientDisconnectWriteError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) ||
		errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "connection reset by peer") ||
		strings.Contains(msg, "use of closed network connection")
}

type streamDebugOptions struct {
	HTTPHeaders bool
	TeeBytes    int64
	TeeDir      string
}

func getenvInt64(key string, def int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func streamDebugOptionsFromEnv() streamDebugOptions {
	teeBytes := getenvInt64("PLEX_TUNER_DEBUG_TEE_BYTES", 0)
	teeDir := strings.TrimSpace(os.Getenv("PLEX_TUNER_DEBUG_TEE_DIR"))
	if teeDir == "" {
		teeDir = "/tmp/plextuner-debug-tee"
	}
	return streamDebugOptions{
		HTTPHeaders: getenvBool("PLEX_TUNER_DEBUG_HTTP_HEADERS", false),
		TeeBytes:    teeBytes,
		TeeDir:      teeDir,
	}
}

func (o streamDebugOptions) enabled() bool {
	return o.HTTPHeaders || o.TeeBytes > 0
}

func sanitizeFileToken(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "na"
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return "na"
	}
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

func debugHeaderLines(h http.Header) []string {
	if h == nil {
		return nil
	}
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := make([]string, 0, len(keys))
	for _, k := range keys {
		vv := h.Values(k)
		if len(vv) == 0 {
			lines = append(lines, k+":")
			continue
		}
		show := make([]string, len(vv))
		for i := range vv {
			show[i] = vv[i]
		}
		switch strings.ToLower(k) {
		case "authorization", "cookie":
			for i := range show {
				show[i] = "<redacted>"
			}
		}
		lines = append(lines, k+": "+strings.Join(show, ", "))
	}
	return lines
}

type cappedBodyTee struct {
	reqID       string
	channelName string
	channelID   string
	path        string
	file        *os.File
	remain      int64
	written     int64
	openErr     error
	loggedErr   bool
}

func newCappedBodyTee(dir string, maxBytes int64, reqID, channelName, channelID string) *cappedBodyTee {
	if maxBytes <= 0 {
		return nil
	}
	t := &cappedBodyTee{
		reqID:       reqID,
		channelName: channelName,
		channelID:   channelID,
		remain:      maxBytes,
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.openErr = err
		log.Printf("gateway: req=%s channel=%q id=%s debug-tee mkdir failed dir=%q err=%v", reqID, channelName, channelID, dir, err)
		return t
	}
	name := fmt.Sprintf("%s-%s-%s-%s.ts",
		time.Now().UTC().Format("20060102T150405.000Z"),
		sanitizeFileToken(reqID),
		sanitizeFileToken(channelID),
		sanitizeFileToken(channelName),
	)
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.openErr = err
		log.Printf("gateway: req=%s channel=%q id=%s debug-tee open failed path=%q err=%v", reqID, channelName, channelID, path, err)
		return t
	}
	t.file = f
	t.path = path
	log.Printf("gateway: req=%s channel=%q id=%s debug-tee start path=%q max_bytes=%d", reqID, channelName, channelID, path, maxBytes)
	return t
}

func (t *cappedBodyTee) Write(p []byte) {
	if t == nil || t.file == nil || t.remain <= 0 || len(p) == 0 {
		return
	}
	if int64(len(p)) > t.remain {
		p = p[:t.remain]
	}
	n, err := t.file.Write(p)
	if n > 0 {
		t.written += int64(n)
		t.remain -= int64(n)
	}
	if err != nil && !t.loggedErr {
		t.loggedErr = true
		log.Printf("gateway: req=%s channel=%q id=%s debug-tee write err=%v", t.reqID, t.channelName, t.channelID, err)
	}
}

func (t *cappedBodyTee) Close() {
	if t == nil || t.file == nil {
		return
	}
	if err := t.file.Close(); err != nil {
		log.Printf("gateway: req=%s channel=%q id=%s debug-tee close err=%v path=%q", t.reqID, t.channelName, t.channelID, err, t.path)
		return
	}
	log.Printf("gateway: req=%s channel=%q id=%s debug-tee done path=%q bytes=%d", t.reqID, t.channelName, t.channelID, t.path, t.written)
}

type streamDebugResponseWriter struct {
	http.ResponseWriter
	reqID        string
	channelName  string
	channelID    string
	start        time.Time
	logHeaders   bool
	headerLogged bool
	firstByte    bool
	status       int
	tee          *cappedBodyTee
}

type responseStartedReporter interface {
	ResponseStarted() bool
}

func newStreamDebugResponseWriter(
	w http.ResponseWriter,
	reqID string,
	channelName string,
	channelID string,
	start time.Time,
	opts streamDebugOptions,
) *streamDebugResponseWriter {
	var tee *cappedBodyTee
	if opts.TeeBytes > 0 {
		tee = newCappedBodyTee(opts.TeeDir, opts.TeeBytes, reqID, channelName, channelID)
	}
	return &streamDebugResponseWriter{
		ResponseWriter: w,
		reqID:          reqID,
		channelName:    channelName,
		channelID:      channelID,
		start:          start,
		logHeaders:     opts.HTTPHeaders,
		tee:            tee,
	}
}

func (w *streamDebugResponseWriter) logResponseHeaders(implicit bool) {
	if w.headerLogged {
		return
	}
	w.headerLogged = true
	status := w.status
	if status == 0 {
		status = http.StatusOK
	}
	log.Printf("gateway: req=%s channel=%q id=%s debug-http response-headers status=%d implicit=%t startup=%s",
		w.reqID, w.channelName, w.channelID, status, implicit, time.Since(w.start).Round(time.Millisecond))
	if !w.logHeaders {
		return
	}
	for _, line := range debugHeaderLines(w.ResponseWriter.Header()) {
		log.Printf("gateway: req=%s channel=%q id=%s debug-http > %s", w.reqID, w.channelName, w.channelID, line)
	}
}

func (w *streamDebugResponseWriter) WriteHeader(code int) {
	w.status = code
	w.logResponseHeaders(false)
	w.ResponseWriter.WriteHeader(code)
}

func (w *streamDebugResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if !w.headerLogged {
		w.logResponseHeaders(true)
	}
	n, err := w.ResponseWriter.Write(p)
	if w.tee != nil && n > 0 {
		w.tee.Write(p[:n])
	}
	if n > 0 && !w.firstByte {
		w.firstByte = true
		log.Printf("gateway: req=%s channel=%q id=%s debug-http first-byte-sent startup=%s bytes=%d",
			w.reqID, w.channelName, w.channelID, time.Since(w.start).Round(time.Millisecond), n)
	}
	return n, err
}

func (w *streamDebugResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *streamDebugResponseWriter) Close() {
	if w.tee != nil {
		w.tee.Close()
	}
}

func (w *streamDebugResponseWriter) ResponseStarted() bool {
	return w.status != 0 || w.firstByte || w.headerLogged
}

func responseAlreadyStarted(w http.ResponseWriter) bool {
	if w == nil {
		return false
	}
	if s, ok := w.(responseStartedReporter); ok {
		return s.ResponseStarted()
	}
	return false
}

type tsDiscontinuitySpliceWriter struct {
	dst        io.Writer
	reqField   string
	channel    string
	channelID  string
	seenPIDs   map[uint16]struct{}
	buf        []byte
	emitted    int64
	shimPkts   int
	rawPackets int
	active     bool
	maxPIDs    int
}

func newTSDiscontinuitySpliceWriter(ctx context.Context, dst io.Writer, channelName, channelID string) *tsDiscontinuitySpliceWriter {
	return &tsDiscontinuitySpliceWriter{
		dst:       dst,
		reqField:  gatewayReqIDField(ctx),
		channel:   channelName,
		channelID: channelID,
		seenPIDs:  make(map[uint16]struct{}, 8),
		active:    true,
		maxPIDs:   16,
	}
}

func makeTSDiscontinuityPacket(pid uint16, cc byte) [188]byte {
	var pkt [188]byte
	pkt[0] = 0x47
	pkt[1] = byte((pid >> 8) & 0x1F)
	pkt[2] = byte(pid & 0xFF)
	// adaptation field only; reuse incoming CC so following payload packet with same CC remains legal
	pkt[3] = 0x20 | (cc & 0x0F)
	pkt[4] = 183
	pkt[5] = 0x80 // discontinuity_indicator
	for i := 6; i < len(pkt); i++ {
		pkt[i] = 0xFF
	}
	return pkt
}

func (w *tsDiscontinuitySpliceWriter) writePacket(pkt []byte) error {
	if len(pkt) != 188 {
		_, err := w.dst.Write(pkt)
		if err == nil {
			w.emitted += int64(len(pkt))
		}
		return err
	}
	if w.active {
		if pkt[0] != 0x47 {
			w.active = false
			log.Printf("gateway:%s channel=%q id=%s hls-relay splice-discontinuity disable reason=lost-sync head=%x",
				w.reqField, w.channel, w.channelID, pkt[:min(len(pkt), 8)])
		} else {
			pid := uint16(pkt[1]&0x1F)<<8 | uint16(pkt[2])
			if pid != 0x1FFF {
				if _, ok := w.seenPIDs[pid]; !ok && len(w.seenPIDs) < w.maxPIDs {
					shim := makeTSDiscontinuityPacket(pid, pkt[3]&0x0F)
					if _, err := w.dst.Write(shim[:]); err != nil {
						return err
					}
					w.emitted += int64(len(shim))
					w.shimPkts++
					w.seenPIDs[pid] = struct{}{}
				}
			}
			if len(w.seenPIDs) >= w.maxPIDs {
				w.active = false
			}
		}
	}
	_, err := w.dst.Write(pkt)
	if err == nil {
		w.emitted += int64(len(pkt))
		w.rawPackets++
	}
	return err
}

func (w *tsDiscontinuitySpliceWriter) Write(p []byte) (int, error) {
	if w == nil || w.dst == nil || len(p) == 0 {
		return len(p), nil
	}
	w.buf = append(w.buf, p...)
	for len(w.buf) >= 188 {
		if err := w.writePacket(w.buf[:188]); err != nil {
			return 0, err
		}
		w.buf = w.buf[188:]
	}
	return len(p), nil
}

func (w *tsDiscontinuitySpliceWriter) FlushRemainder() error {
	if w == nil {
		return nil
	}
	if len(w.buf) > 0 {
		if _, err := w.dst.Write(w.buf); err != nil {
			return err
		}
		w.emitted += int64(len(w.buf))
		w.buf = nil
	}
	log.Printf("gateway:%s channel=%q id=%s hls-relay splice-discontinuity shims=%d unique_pids=%d raw_packets=%d",
		w.reqField, w.channel, w.channelID, w.shimPkts, len(w.seenPIDs), w.rawPackets)
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func resolveFFmpegPath() (string, error) {
	if v := strings.TrimSpace(os.Getenv("PLEX_TUNER_FFMPEG_PATH")); v != "" {
		return exec.LookPath(v)
	}
	return exec.LookPath("ffmpeg")
}

// canonicalizeFFmpegInputURL resolves the input host in Go and rewrites the URL
// to a numeric host for ffmpeg. This avoids resolver differences where Go can
// resolve a host (for example a k8s short service hostname) but the bundled
// ffmpeg binary cannot.
func canonicalizeFFmpegInputURL(ctx context.Context, raw string) (rewritten string, fromHost string, toHost string) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.Host == "" {
		return raw, "", ""
	}
	host := u.Hostname()
	if host == "" {
		return raw, "", ""
	}
	if ip := net.ParseIP(host); ip != nil {
		return raw, "", ""
	}
	lookupCtx := ctx
	if lookupCtx == nil {
		lookupCtx = context.Background()
	}
	lookupCtx, cancel := context.WithTimeout(lookupCtx, 2*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupHost(lookupCtx, host)
	if err != nil || len(ips) == 0 {
		return raw, "", ""
	}
	ip := strings.TrimSpace(ips[0])
	if ip == "" || ip == host {
		return raw, "", ""
	}
	if p := u.Port(); p != "" {
		u.Host = net.JoinHostPort(ip, p)
	} else {
		u.Host = ip
	}
	return u.String(), host, ip
}

type startSignalState struct {
	TSLikePackets int
	HasIDR        bool
	HasAAC        bool
	AlignedOffset int
}

func containsH264IDRAnnexB(buf []byte) bool {
	if len(buf) < 4 {
		return false
	}
	for i := 0; i < len(buf)-3; i++ {
		// 3-byte Annex B start code: 00 00 01 <nal>
		if i+4 <= len(buf) && buf[i] == 0x00 && buf[i+1] == 0x00 && buf[i+2] == 0x01 {
			if (buf[i+3] & 0x1f) == 5 {
				return true
			}
			continue
		}
		// 4-byte Annex B start code: 00 00 00 01 <nal>
		if i+5 <= len(buf) && buf[i] == 0x00 && buf[i+1] == 0x00 && buf[i+2] == 0x00 && buf[i+3] == 0x01 {
			if (buf[i+4] & 0x1f) == 5 {
				return true
			}
		}
	}
	return false
}

func looksLikeGoodTSStart(buf []byte) startSignalState {
	const pkt = 188
	st := startSignalState{}
	st.AlignedOffset = -1
	var idrCarry []byte
	// Quick TS sanity and payload scan for H264 IDR + AAC/ADTS.
	for off := 0; off+pkt <= len(buf); {
		if buf[off] != 0x47 {
			// Resync locally.
			n := bytes.IndexByte(buf[off+1:], 0x47)
			if n < 0 {
				break
			}
			off += n + 1
			continue
		}
		st.TSLikePackets++
		if st.AlignedOffset < 0 {
			// Prefer a packet boundary that looks stable for a few packets.
			ok := 0
			for k := off; k < len(buf) && ok < 4; k += pkt {
				if k >= len(buf) || buf[k] != 0x47 {
					break
				}
				ok++
			}
			if ok >= 3 {
				st.AlignedOffset = off
			}
		}
		p := buf[off : off+pkt]
		afc := (p[3] >> 4) & 0x3
		i := 4
		if afc == 0 || afc == 2 { // reserved or adaptation-only
			off += pkt
			continue
		}
		if afc == 3 {
			if i >= len(p) {
				off += pkt
				continue
			}
			alen := int(p[i])
			i++
			i += alen
		}
		if i >= len(p) {
			off += pkt
			continue
		}
		payload := p[i:]
		// H264 Annex B IDR (NAL type 5)
		if !st.HasIDR {
			if containsH264IDRAnnexB(payload) {
				st.HasIDR = true
			} else if len(idrCarry) > 0 {
				joined := make([]byte, 0, len(idrCarry)+len(payload))
				joined = append(joined, idrCarry...)
				joined = append(joined, payload...)
				if containsH264IDRAnnexB(joined) {
					st.HasIDR = true
				}
			}
		}
		// AAC ADTS syncword
		if !st.HasAAC {
			for j := 0; j+1 < len(payload); j++ {
				if payload[j] == 0xFF && (payload[j+1]&0xF0) == 0xF0 {
					st.HasAAC = true
					break
				}
			}
		}
		if len(payload) > 0 {
			if len(payload) >= 4 {
				idrCarry = append(idrCarry[:0], payload[len(payload)-4:]...)
			} else {
				keep := len(idrCarry) + len(payload)
				if keep > 4 {
					drop := keep - 4
					if drop < len(idrCarry) {
						idrCarry = idrCarry[drop:]
					} else {
						idrCarry = idrCarry[:0]
					}
				}
				idrCarry = append(idrCarry, payload...)
				if len(idrCarry) > 4 {
					idrCarry = idrCarry[len(idrCarry)-4:]
				}
			}
		}
		if st.HasIDR && st.HasAAC && st.TSLikePackets >= 8 {
			return st
		}
		off += pkt
	}
	return st
}

type plexForwardedHints struct {
	SessionIdentifier string
	ClientIdentifier  string
	Product           string
	Platform          string
	Device            string
	Raw               map[string]string
}

type plexResolvedClient struct {
	SessionIdentifier string
	ClientIdentifier  string
	Product           string
	Platform          string
	Title             string
}

func (h plexForwardedHints) empty() bool {
	return h.SessionIdentifier == "" && h.ClientIdentifier == "" && h.Product == "" && h.Platform == "" && h.Device == ""
}

func (h plexForwardedHints) summary() string {
	parts := []string{}
	if h.SessionIdentifier != "" {
		parts = append(parts, `sid="`+h.SessionIdentifier+`"`)
	}
	if h.ClientIdentifier != "" {
		parts = append(parts, `cid="`+h.ClientIdentifier+`"`)
	}
	if h.Product != "" {
		parts = append(parts, `product="`+h.Product+`"`)
	}
	if h.Platform != "" {
		parts = append(parts, `platform="`+h.Platform+`"`)
	}
	if h.Device != "" {
		parts = append(parts, `device="`+h.Device+`"`)
	}
	if len(h.Raw) > 0 {
		parts = append(parts, `raw=`+strconv.Itoa(len(h.Raw)))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " ")
}

func plexRequestHints(r *http.Request) plexForwardedHints {
	get := func(keys ...string) string {
		for _, k := range keys {
			if v := strings.TrimSpace(r.Header.Get(k)); v != "" {
				return v
			}
			if v := strings.TrimSpace(r.URL.Query().Get(k)); v != "" {
				return v
			}
			// Also allow lowercase query/header names.
			lk := strings.ToLower(k)
			if v := strings.TrimSpace(r.Header.Get(lk)); v != "" {
				return v
			}
			if v := strings.TrimSpace(r.URL.Query().Get(lk)); v != "" {
				return v
			}
		}
		return ""
	}
	raw := map[string]string{}
	for k, vals := range r.Header {
		kl := strings.ToLower(k)
		if !strings.HasPrefix(kl, "x-plex-") {
			continue
		}
		if len(vals) > 0 && strings.TrimSpace(vals[0]) != "" {
			raw[k] = strings.TrimSpace(vals[0])
		}
	}
	for k, vals := range r.URL.Query() {
		kl := strings.ToLower(k)
		if !strings.Contains(kl, "plex") && !strings.Contains(kl, "session") && !strings.Contains(kl, "client") {
			continue
		}
		if len(vals) > 0 && strings.TrimSpace(vals[0]) != "" {
			raw["q:"+k] = strings.TrimSpace(vals[0])
		}
	}
	return plexForwardedHints{
		SessionIdentifier: get("X-Plex-Session-Identifier", "session", "sessionId", "session_id"),
		ClientIdentifier:  get("X-Plex-Client-Identifier", "X-Plex-Target-Client-Identifier", "clientIdentifier", "client_id"),
		Product:           get("X-Plex-Product"),
		Platform:          get("X-Plex-Platform", "X-Plex-Client-Platform"),
		Device:            get("X-Plex-Device", "X-Plex-Device-Name"),
		Raw:               raw,
	}
}

func xmlStartAttr(start xml.StartElement, name string) string {
	for _, a := range start.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func (g *Gateway) resolvePlexClient(ctx context.Context, hints plexForwardedHints) (*plexResolvedClient, error) {
	if g == nil || !g.PlexClientAdapt {
		return nil, nil
	}
	if strings.TrimSpace(g.PlexPMSURL) == "" || strings.TrimSpace(g.PlexPMSToken) == "" {
		return nil, nil
	}
	if hints.SessionIdentifier == "" && hints.ClientIdentifier == "" {
		return nil, nil
	}
	base := strings.TrimRight(strings.TrimSpace(g.PlexPMSURL), "/")
	u := base + "/status/sessions?X-Plex-Token=" + url.QueryEscape(strings.TrimSpace(g.PlexPMSToken))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	client := g.Client
	if client == nil {
		client = httpclient.ForStreaming()
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("pms /status/sessions status=" + strconv.Itoa(resp.StatusCode))
	}
	dec := xml.NewDecoder(resp.Body)
	type candidate struct {
		title    string
		player   plexResolvedClient
		session  string
		clientID string
	}
	var stack []string
	var cur *candidate
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
			stack = append(stack, t.Name.Local)
			switch t.Name.Local {
			case "Video", "Track", "Photo", "Metadata":
				if cur == nil {
					cur = &candidate{
						title: xmlStartAttr(t, "title"),
					}
				}
			case "Player":
				if cur != nil {
					cur.player.ClientIdentifier = xmlStartAttr(t, "machineIdentifier")
					cur.player.Product = xmlStartAttr(t, "product")
					cur.player.Platform = xmlStartAttr(t, "platform")
					if cur.player.Platform == "" {
						cur.player.Platform = xmlStartAttr(t, "platformTitle")
					}
				}
			case "Session":
				if cur != nil {
					cur.session = xmlStartAttr(t, "id")
				}
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			if cur != nil && (t.Name.Local == "Video" || t.Name.Local == "Track" || t.Name.Local == "Photo" || t.Name.Local == "Metadata") {
				matchSID := hints.SessionIdentifier != "" && cur.session != "" && cur.session == hints.SessionIdentifier
				matchCID := hints.ClientIdentifier != "" && cur.player.ClientIdentifier != "" && cur.player.ClientIdentifier == hints.ClientIdentifier
				if matchSID || matchCID {
					out := cur.player
					out.SessionIdentifier = cur.session
					out.Title = cur.title
					if out.ClientIdentifier == "" {
						out.ClientIdentifier = hints.ClientIdentifier
					}
					if out.SessionIdentifier == "" {
						out.SessionIdentifier = hints.SessionIdentifier
					}
					return &out, nil
				}
				cur = nil
			}
		}
	}
	return nil, nil
}

func looksLikePlexWeb(s string) bool {
	v := strings.ToLower(strings.TrimSpace(s))
	return strings.Contains(v, "plex web") || strings.Contains(v, "web") || strings.Contains(v, "browser") || strings.Contains(v, "firefox") || strings.Contains(v, "chrome") || strings.Contains(v, "safari")
}

func (g *Gateway) requestAdaptation(ctx context.Context, r *http.Request, channel *catalog.LiveChannel, channelID string) (bool, bool, string, string) {
	hints := plexRequestHints(r)
	log.Printf("gateway: channel=%q id=%s plex-hints %s", channel.GuideName, channelID, hints.summary())
	// Explicit override always wins and is deterministic.
	explicitProfile := normalizeProfileName(r.URL.Query().Get("profile"))
	if strings.TrimSpace(r.URL.Query().Get("profile")) != "" {
		switch explicitProfile {
		case profilePlexSafe, profileAACCFR, profileVideoOnly, profileLowBitrate, profileDashFast:
			return true, true, explicitProfile, "query-profile"
		default:
			return true, false, explicitProfile, "query-profile"
		}
	}
	if !g.PlexClientAdapt {
		return false, false, "", "adapt-disabled"
	}
	info, err := g.resolvePlexClient(ctx, hints)
	if err != nil {
		log.Printf("gateway: channel=%q id=%s plex-client-resolve err=%v", channel.GuideName, channelID, err)
		return true, true, profilePlexSafe, "resolve-error-websafe"
	}
	if info == nil {
		return true, true, profilePlexSafe, "unknown-client-websafe"
	}
	log.Printf("gateway: channel=%q id=%s plex-client-resolved sid=%q cid=%q product=%q platform=%q title=%q",
		channel.GuideName, channelID, info.SessionIdentifier, info.ClientIdentifier, info.Product, info.Platform, info.Title)
	if looksLikePlexWeb(info.Product) || looksLikePlexWeb(info.Platform) {
		return true, true, profilePlexSafe, "resolved-web-client"
	}
	return true, false, "", "resolved-nonweb-client"
}

// Adaptive buffer tuning: grow when client is slow (backpressure), shrink when client keeps up.
const (
	adaptiveBufferMin       = 64 << 10 // 64 KiB
	adaptiveBufferMax       = 2 << 20  // 2 MiB
	adaptiveBufferInitial   = 1 << 20  // 1 MiB
	adaptiveSlowFlushMs     = 100
	adaptiveFastFlushMs     = 20
	adaptiveFastCountShrink = 3
)

type adaptiveWriter struct {
	w            io.Writer
	buf          bytes.Buffer
	targetSize   int
	minSize      int
	maxSize      int
	slowThresh   time.Duration
	fastThresh   time.Duration
	fastCount    int
	fastCountMax int
}

func newAdaptiveWriter(w io.Writer) *adaptiveWriter {
	return &adaptiveWriter{
		w:            w,
		targetSize:   adaptiveBufferInitial,
		minSize:      adaptiveBufferMin,
		maxSize:      adaptiveBufferMax,
		slowThresh:   adaptiveSlowFlushMs * time.Millisecond,
		fastThresh:   adaptiveFastFlushMs * time.Millisecond,
		fastCountMax: adaptiveFastCountShrink,
	}
}

func (a *adaptiveWriter) Write(p []byte) (int, error) {
	n, err := a.buf.Write(p)
	if err != nil {
		return n, err
	}
	for a.buf.Len() >= a.targetSize {
		if err := a.flushToClient(); err != nil {
			return n, err
		}
	}
	return n, nil
}

func (a *adaptiveWriter) flushToClient() error {
	if a.buf.Len() == 0 {
		return nil
	}
	start := time.Now()
	for a.buf.Len() > 0 {
		n, err := a.w.Write(a.buf.Bytes())
		if err != nil {
			return err
		}
		if n <= 0 {
			break
		}
		// bytes.Buffer doesn't advance on Write; drop leading n bytes
		remaining := a.buf.Bytes()[n:]
		a.buf.Reset()
		a.buf.Write(remaining)
	}
	d := time.Since(start)
	if d >= a.slowThresh {
		if a.targetSize < a.maxSize {
			a.targetSize *= 2
			if a.targetSize > a.maxSize {
				a.targetSize = a.maxSize
			}
		}
		a.fastCount = 0
	} else if d <= a.fastThresh {
		a.fastCount++
		if a.fastCount >= a.fastCountMax {
			a.fastCount = 0
			if a.targetSize > a.minSize {
				a.targetSize /= 2
				if a.targetSize < a.minSize {
					a.targetSize = a.minSize
				}
			}
		}
	} else {
		a.fastCount = 0
	}
	return nil
}

func (a *adaptiveWriter) Flush() error { return a.flushToClient() }

// streamWriter wraps w with an optional buffer. Call flush before returning.
// bufferBytes: >0 = fixed size (bufio); 0 = passthrough; -1 = adaptive.
func streamWriter(w http.ResponseWriter, bufferBytes int) (io.Writer, func()) {
	if bufferBytes == 0 {
		return w, func() {}
	}
	if bufferBytes == -1 {
		aw := newAdaptiveWriter(w)
		return aw, func() { _ = aw.Flush() }
	}
	bw := bufio.NewWriterSize(w, bufferBytes)
	return bw, func() { _ = bw.Flush() }
}

func startNullTSKeepalive(
	ctx context.Context,
	dst io.Writer,
	flushBody func(),
	flusher http.Flusher,
	channelName, channelID, modeLabel string,
	start time.Time,
	interval time.Duration,
	packetsPerTick int,
) func(string) {
	if dst == nil || interval <= 0 || packetsPerTick <= 0 {
		return func(string) {}
	}
	if interval < 25*time.Millisecond {
		interval = 25 * time.Millisecond
	}
	if packetsPerTick > 64 {
		packetsPerTick = 64
	}
	stopCh := make(chan string, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		reqField := gatewayReqIDField(ctx)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		// MPEG-TS null packet (PID 0x1FFF): valid transport packets at a tiny rate to prevent
		// a long idle socket window while startup gating waits for upstream to produce useful bytes.
		pkt := [188]byte{0x47, 0x1F, 0xFF, 0x10}
		for i := 4; i < len(pkt); i++ {
			pkt[i] = 0xFF
		}
		var sentBytes int64
		var ticks int
		reason := "done"
		for {
			select {
			case <-ctx.Done():
				reason = "client-done"
				log.Printf("gateway:%s channel=%q id=%s %s null-ts-keepalive stop=%s bytes=%d ticks=%d startup=%s",
					reqField, channelName, channelID, modeLabel, reason, sentBytes, ticks, time.Since(start).Round(time.Millisecond))
				return
			case reason = <-stopCh:
				log.Printf("gateway:%s channel=%q id=%s %s null-ts-keepalive stop=%s bytes=%d ticks=%d startup=%s",
					reqField, channelName, channelID, modeLabel, reason, sentBytes, ticks, time.Since(start).Round(time.Millisecond))
				return
			case <-ticker.C:
			}
			for i := 0; i < packetsPerTick; i++ {
				n, err := dst.Write(pkt[:])
				if n > 0 {
					sentBytes += int64(n)
				}
				if err != nil {
					reason = "write-error"
					log.Printf("gateway:%s channel=%q id=%s %s null-ts-keepalive stop=%s err=%v bytes=%d ticks=%d startup=%s",
						reqField, channelName, channelID, modeLabel, reason, err, sentBytes, ticks, time.Since(start).Round(time.Millisecond))
					return
				}
			}
			ticks++
			if flushBody != nil {
				flushBody()
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
	}()
	var once sync.Once
	return func(reason string) {
		once.Do(func() {
			select {
			case stopCh <- reason:
			default:
			}
			<-done
		})
	}
}

const (
	profileDefault    = "default"
	profilePlexSafe   = "plexsafe"
	profileAACCFR     = "aaccfr"
	profileVideoOnly  = "videoonlyfast"
	profileLowBitrate = "lowbitrate"
	profileDashFast   = "dashfast"
	profilePMSXcode   = "pmsxcode"
)

func normalizeProfileName(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "default":
		return profileDefault
	case "plexsafe", "plex-safe", "safe":
		return profilePlexSafe
	case "aaccfr", "aac-cfr", "aac":
		return profileAACCFR
	case "videoonlyfast", "video-only-fast", "videoonly", "video":
		return profileVideoOnly
	case "lowbitrate", "low-bitrate", "low":
		return profileLowBitrate
	case "dashfast", "dash-fast":
		return profileDashFast
	case "pmsxcode", "pms-xcode", "pmsforce", "pms-force":
		return profilePMSXcode
	default:
		return profileDefault
	}
}

func defaultProfileFromEnv() string {
	p := strings.TrimSpace(os.Getenv("PLEX_TUNER_PROFILE"))
	if p != "" {
		return normalizeProfileName(p)
	}
	// Back-compat for the old boolean.
	if strings.EqualFold(os.Getenv("PLEX_TUNER_PLEX_SAFE"), "1") ||
		strings.EqualFold(os.Getenv("PLEX_TUNER_PLEX_SAFE"), "true") ||
		strings.EqualFold(os.Getenv("PLEX_TUNER_PLEX_SAFE"), "yes") {
		return profilePlexSafe
	}
	return profileDefault
}

func loadProfileOverridesFile(path string) (map[string]string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw := map[string]string{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = normalizeProfileName(v)
	}
	return out, nil
}

func loadTranscodeOverridesFile(path string) (map[string]bool, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Accept either {"id": true} or {"id":"on"/"off"} for convenience.
	boolMap := map[string]bool{}
	if err := json.Unmarshal(b, &boolMap); err == nil {
		return boolMap, nil
	}
	strMap := map[string]string{}
	if err := json.Unmarshal(b, &strMap); err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(strMap))
	for k, v := range strMap {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on", "transcode":
			out[k] = true
		case "0", "false", "no", "off", "remux", "copy":
			out[k] = false
		}
	}
	return out, nil
}

func (g *Gateway) firstProfileOverride(keys ...string) (string, bool) {
	if g == nil || g.ProfileOverrides == nil {
		return "", false
	}
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if p, ok := g.ProfileOverrides[k]; ok && strings.TrimSpace(p) != "" {
			return normalizeProfileName(p), true
		}
	}
	return "", false
}

func (g *Gateway) profileForChannel(channelID string) string {
	if p, ok := g.firstProfileOverride(channelID); ok {
		return p
	}
	if g != nil && strings.TrimSpace(g.DefaultProfile) != "" {
		return normalizeProfileName(g.DefaultProfile)
	}
	return defaultProfileFromEnv()
}

func (g *Gateway) profileForChannelMeta(channelID, guideNumber, tvgID string) string {
	if p, ok := g.firstProfileOverride(channelID, guideNumber, tvgID); ok {
		return p
	}
	return g.profileForChannel("")
}

func (g *Gateway) effectiveTranscode(ctx context.Context, streamURL string) bool {
	switch strings.ToLower(strings.TrimSpace(g.StreamTranscodeMode)) {
	case "on":
		return true
	case "off", "":
		return false
	case "auto_cached", "cached_auto":
		// Caller should pass a channel-aware decision via effectiveTranscodeForChannel.
		return false
	case "auto":
		need, err := g.needTranscode(ctx, streamURL)
		if err != nil {
			log.Printf("gateway: ffprobe auto transcode check failed url=%s err=%v (using transcode)", safeurl.RedactURL(streamURL), err)
			return true
		}
		return need
	default:
		return false
	}
}

func (g *Gateway) firstTranscodeOverride(keys ...string) (bool, bool) {
	if g == nil || g.TranscodeOverrides == nil {
		return false, false
	}
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if v, ok := g.TranscodeOverrides[k]; ok {
			return v, true
		}
	}
	return false, false
}

func (g *Gateway) effectiveTranscodeForChannel(ctx context.Context, channelID, streamURL string) bool {
	return g.effectiveTranscodeForChannelMeta(ctx, channelID, "", "", streamURL)
}

func (g *Gateway) effectiveTranscodeForChannelMeta(ctx context.Context, channelID, guideNumber, tvgID, streamURL string) bool {
	mode := strings.ToLower(strings.TrimSpace(g.StreamTranscodeMode))
	if mode == "auto_cached" || mode == "cached_auto" {
		if v, ok := g.firstTranscodeOverride(channelID, guideNumber, tvgID); ok {
			log.Printf("gateway: transcode auto_cached match id=%q guide=%q tvg=%q -> %t", channelID, guideNumber, tvgID, v)
			return v
		}
		log.Printf("gateway: transcode auto_cached miss id=%q guide=%q tvg=%q (default remux)", channelID, guideNumber, tvgID)
		// Fast-path fallback for uncached channels: keep startup latency low.
		return false
	}
	return g.effectiveTranscode(ctx, streamURL)
}

func (g *Gateway) needTranscode(ctx context.Context, streamURL string) (bool, error) {
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		return true, err
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	probe := func(sel string) (string, error) {
		args := []string{"-v", "error", "-nostdin", "-rw_timeout", "5000000", "-user_agent", "PlexTuner/1.0"}
		if g.ProviderUser != "" || g.ProviderPass != "" {
			auth := base64.StdEncoding.EncodeToString([]byte(g.ProviderUser + ":" + g.ProviderPass))
			args = append(args, "-headers", "Authorization: Basic "+auth+"\r\n")
		}
		args = append(args, "-select_streams", sel, "-show_entries", "stream=codec_name", "-of", "default=noprint_wrappers=1:nokey=1", streamURL)
		out, err := exec.CommandContext(ctx, ffprobePath, args...).Output()
		return strings.TrimSpace(string(out)), err
	}
	v, err := probe("v:0")
	if err != nil || !isPlexFriendlyVideoCodec(v) {
		return true, err
	}
	a, err := probe("a:0")
	if err != nil || !isPlexFriendlyAudioCodec(a) {
		return true, err
	}
	return false, nil
}

func isPlexFriendlyVideoCodec(name string) bool {
	switch strings.ToLower(name) {
	case "h264", "avc", "mpeg2video", "mpeg4":
		return true
	default:
		return false
	}
}

func isPlexFriendlyAudioCodec(name string) bool {
	switch strings.ToLower(name) {
	case "aac", "ac3", "eac3", "mp3", "mp2":
		return true
	default:
		return false
	}
}

func (g *Gateway) effectiveBufferSize(transcode bool) int {
	if g.StreamBufferBytes >= 0 {
		return g.StreamBufferBytes
	}
	if transcode {
		return -1
	}
	return 0
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reqID := fmt.Sprintf("r%06d", atomic.AddUint64(&g.reqSeq, 1))
	r = r.WithContext(context.WithValue(r.Context(), gatewayReqIDKey{}, reqID))
	channelID, ok := channelIDFromRequestPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if channelID == "" {
		http.NotFound(w, r)
		return
	}
	var channel *catalog.LiveChannel
	for i := range g.Channels {
		if g.Channels[i].ChannelID == channelID {
			channel = &g.Channels[i]
			break
		}
	}
	if channel == nil {
		// Fallback: numeric index for backwards compatibility when ChannelID is not set
		if idx, err := strconv.Atoi(channelID); err == nil && idx >= 0 && idx < len(g.Channels) {
			channel = &g.Channels[idx]
		}
	}
	if channel == nil {
		// PMS may request /auto/v<GuideNumber> while our stream path uses a
		// non-numeric ChannelID (for example a tvg-id slug). Accept GuideNumber as
		// a fallback lookup for both /auto/ and /stream/ requests.
		for i := range g.Channels {
			if g.Channels[i].GuideNumber == channelID {
				channel = &g.Channels[i]
				break
			}
		}
	}
	if channel == nil {
		http.NotFound(w, r)
		return
	}
	log.Printf("gateway: req=%s recv path=%q channel=%q remote=%q ua=%q", reqID, r.URL.Path, channelID, r.RemoteAddr, r.UserAgent())
	debugOpts := streamDebugOptionsFromEnv()
	if debugOpts.HTTPHeaders {
		for _, line := range debugHeaderLines(r.Header) {
			log.Printf("gateway: req=%s channel=%q id=%s debug-http < %s", reqID, channel.GuideName, channelID, line)
		}
	}
	hasTranscodeOverride, forceTranscode, forcedProfile, adaptReason := g.requestAdaptation(r.Context(), r, channel, channelID)
	if adaptReason != "" && adaptReason != "adapt-disabled" {
		if hasTranscodeOverride {
			log.Printf("gateway: channel=%q id=%s adapt transcode=%t profile=%q reason=%s", channel.GuideName, channelID, forceTranscode, forcedProfile, adaptReason)
		} else {
			log.Printf("gateway: channel=%q id=%s adapt inherit profile=%q reason=%s", channel.GuideName, channelID, forcedProfile, adaptReason)
		}
	}
	start := time.Now()
	if debugOpts.enabled() {
		dw := newStreamDebugResponseWriter(w, reqID, channel.GuideName, channelID, start, debugOpts)
		defer dw.Close()
		w = dw
	}
	urls := channel.StreamURLs
	if len(urls) == 0 && channel.StreamURL != "" {
		urls = []string{channel.StreamURL}
	}
	if len(urls) == 0 {
		log.Printf("gateway: req=%s channel=%q id=%s no-stream-url", reqID, channel.GuideName, channelID)
		http.Error(w, "no stream URL", http.StatusBadGateway)
		return
	}

	g.mu.Lock()
	limit := g.TunerCount
	if limit <= 0 {
		limit = 2
	}
	if g.inUse >= limit {
		g.mu.Unlock()
		log.Printf("gateway: req=%s channel=%q id=%s reject all-tuners-in-use limit=%d ua=%q", reqID, channel.GuideName, channelID, limit, r.UserAgent())
		w.Header().Set("X-HDHomeRun-Error", "805") // All Tuners In Use
		http.Error(w, "All tuners in use", http.StatusServiceUnavailable)
		return
	}
	g.inUse++
	inUseNow := g.inUse
	g.mu.Unlock()
	log.Printf("gateway: req=%s channel=%q id=%s acquire inuse=%d/%d", reqID, channel.GuideName, channelID, inUseNow, limit)
	defer func() {
		g.mu.Lock()
		g.inUse--
		inUseLeft := g.inUse
		g.mu.Unlock()
		log.Printf("gateway: req=%s channel=%q id=%s release inuse=%d/%d dur=%s", reqID, channel.GuideName, channelID, inUseLeft, limit, time.Since(start).Round(time.Millisecond))
	}()

	// Try primary then backups until one works. Do not retry or backoff on 429/423 here:
	// that would block stream throughput. We only fail over to next URL and return 502 if all fail.
	// Reject non-http(s) URLs to prevent SSRF (e.g. file:// or provider-supplied internal URLs).
	for i, streamURL := range urls {
		if !safeurl.IsHTTPOrHTTPS(streamURL) {
			if i == 0 {
				log.Printf("gateway: channel %s: invalid stream URL scheme (rejected)", channel.GuideName)
			}
			continue
		}
		req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, streamURL, nil)
		if err != nil {
			continue
		}
		if g.ProviderUser != "" || g.ProviderPass != "" {
			req.SetBasicAuth(g.ProviderUser, g.ProviderPass)
		}
		req.Header.Set("User-Agent", "PlexTuner/1.0")

		client := g.Client
		if client == nil {
			client = httpclient.ForStreaming()
		}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("gateway: channel=%q id=%s upstream[%d/%d] error url=%s err=%v",
				channel.GuideName, channelID, i+1, len(urls), safeurl.RedactURL(streamURL), err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode == http.StatusTooManyRequests {
				log.Printf("gateway: channel=%q id=%s upstream[%d/%d] 429 rate limited url=%s",
					channel.GuideName, channelID, i+1, len(urls), safeurl.RedactURL(streamURL))
			} else {
				log.Printf("gateway: channel=%q id=%s upstream[%d/%d] status=%d url=%s",
					channel.GuideName, channelID, i+1, len(urls), resp.StatusCode, safeurl.RedactURL(streamURL))
			}
			resp.Body.Close()
			continue
		}
		// Reject 200 with empty body (e.g. Cloudflare/redirect returning 0 bytes) — try next URL (learned from k3s IPTV hardening).
		if resp.ContentLength == 0 {
			log.Printf("gateway: channel=%q id=%s upstream[%d/%d] empty-body url=%s ct=%q",
				channel.GuideName, channelID, i+1, len(urls), safeurl.RedactURL(streamURL), resp.Header.Get("Content-Type"))
			resp.Body.Close()
			continue
		}
		log.Printf("gateway: req=%s channel=%q id=%s start upstream[%d/%d] url=%s ct=%q cl=%d inuse=%d/%d ua=%q",
			reqID, channel.GuideName, channelID, i+1, len(urls), safeurl.RedactURL(streamURL), resp.Header.Get("Content-Type"), resp.ContentLength, inUseNow, limit, r.UserAgent())
		for k, v := range resp.Header {
			if k == "Content-Length" || k == "Transfer-Encoding" {
				continue
			}
			for _, vv := range v {
				w.Header().Add(k, vv)
			}
		}
		if isHLSResponse(resp, streamURL) {
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				log.Printf("gateway: channel=%q id=%s read-playlist-failed err=%v", channel.GuideName, channelID, err)
				continue
			}
			body = rewriteHLSPlaylist(body, streamURL)
			firstSeg := firstHLSMediaLine(body)
			transcode := g.effectiveTranscodeForChannelMeta(r.Context(), channelID, channel.GuideNumber, channel.TVGID, streamURL)
			if hasTranscodeOverride {
				transcode = forceTranscode
			}
			bufferSize := g.effectiveBufferSize(transcode)
			mode := "remux"
			if transcode {
				mode = "transcode"
			}
			bufDesc := strconv.Itoa(bufferSize)
			if bufferSize == -1 {
				bufDesc = "adaptive"
			}
			log.Printf("gateway: channel=%q id=%s hls-playlist bytes=%d first-seg=%q dur=%s (relaying as ts, %s buffer=%s)",
				channel.GuideName, channelID, len(body), firstSeg, time.Since(start).Round(time.Millisecond), mode, bufDesc)
			log.Printf("gateway: channel=%q id=%s hls-mode transcode=%t mode=%q guide=%q tvg=%q", channel.GuideName, channelID, transcode, g.StreamTranscodeMode, channel.GuideNumber, channel.TVGID)
			if ffmpegPath, ffmpegErr := resolveFFmpegPath(); ffmpegErr == nil {
				if err := g.relayHLSWithFFmpeg(w, r, ffmpegPath, streamURL, channel.GuideName, channelID, channel.GuideNumber, channel.TVGID, start, transcode, bufferSize, forcedProfile); err == nil {
					return
				} else {
					log.Printf("gateway: channel=%q id=%s ffmpeg-%s failed (falling back to go relay): %v",
						channel.GuideName, channelID, mode, err)
				}
			} else if strings.TrimSpace(os.Getenv("PLEX_TUNER_FFMPEG_PATH")) != "" {
				log.Printf("gateway: channel=%q id=%s ffmpeg unavailable path=%q err=%v",
					channel.GuideName, channelID, os.Getenv("PLEX_TUNER_FFMPEG_PATH"), ffmpegErr)
			} else if transcode {
				log.Printf("gateway: channel=%q id=%s ffmpeg unavailable transcode-requested=true err=%v (falling back to go relay; web clients may get incompatible audio/video codecs)", channel.GuideName, channelID, ffmpegErr)
			}
			if err := g.relayHLSAsTS(
				w,
				r,
				client,
				streamURL,
				body,
				channel.GuideName,
				channelID,
				channel.GuideNumber,
				channel.TVGID,
				start,
				transcode,
				forcedProfile,
				bufferSize,
				responseAlreadyStarted(w),
			); err != nil {
				log.Printf("gateway: channel=%q id=%s hls-relay failed: %v", channel.GuideName, channelID, err)
				continue
			}
			return
		}
		bufferSize := g.effectiveBufferSize(false)
		w.WriteHeader(resp.StatusCode)
		sw, flush := streamWriter(w, bufferSize)
		n, _ := io.Copy(sw, resp.Body)
		resp.Body.Close()
		flush()
		log.Printf("gateway: channel=%q id=%s proxied bytes=%d dur=%s", channel.GuideName, channelID, n, time.Since(start).Round(time.Millisecond))
		return
	}
	log.Printf("gateway: channel=%q id=%s all %d upstream(s) failed dur=%s", channel.GuideName, channelID, len(urls), time.Since(start).Round(time.Millisecond))
	http.Error(w, "All upstreams failed", http.StatusBadGateway)
}

func ffmpegRelayErr(phase string, err error, stderr string) error {
	msg := strings.TrimSpace(stderr)
	if msg != "" {
		if len(msg) > 600 {
			msg = msg[:600] + "..."
		}
		return fmt.Errorf("%s: %w (stderr=%q)", phase, err, msg)
	}
	return fmt.Errorf("%s: %w", phase, err)
}

func channelIDFromRequestPath(path string) (string, bool) {
	if strings.HasPrefix(path, "/stream/") {
		return strings.TrimPrefix(path, "/stream/"), true
	}
	if strings.HasPrefix(path, "/auto/") {
		rest := strings.TrimPrefix(path, "/auto/")
		// PMS fallback commonly uses /auto/v<channelID>.
		if strings.HasPrefix(rest, "v") {
			rest = strings.TrimPrefix(rest, "v")
		}
		return rest, true
	}
	return "", false
}

func buildFFmpegMPEGTSCodecArgs(transcode bool, profile string) []string {
	mpegtsFlags := mpegTSFlagsWithOptionalInitialDiscontinuity()
	var codecArgs []string
	if !transcode {
		codecArgs = []string{
			"-map", "0:v:0",
			"-map", "0:a?",
			"-c", "copy",
		}
	} else if profile == profilePMSXcode {
		// Diagnostic profile: make the source less likely to stay on Plex's copy path.
		codecArgs = []string{
			"-map", "0:v:0",
			"-map", "0:a:0?",
			"-sn",
			"-dn",
			"-vf", "fps=30000/1001,scale='min(960,iw)':-2,format=yuv420p",
			"-c:v", "mpeg2video",
			"-pix_fmt", "yuv420p",
			"-bf", "0",
			"-g", "15",
			"-b:v", "2200k",
			"-maxrate", "2500k",
			"-bufsize", "5000k",
			"-c:a", "mp2",
			"-ac", "2",
			"-ar", "48000",
			"-b:a", "128k",
			"-af", "aresample=async=1:first_pts=0",
		}
	} else {
		codecArgs = []string{
			"-map", "0:v:0",
			"-map", "0:a:0?",
			"-sn",
			"-dn",
			"-c:v", "libx264",
			"-a53cc", "0",
			"-preset", "veryfast",
			"-tune", "zerolatency",
			"-x264-params", "repeat-headers=1:keyint=30:min-keyint=30:scenecut=0:force-cfr=1",
			"-pix_fmt", "yuv420p",
			"-g", "30",
			"-keyint_min", "30",
			"-sc_threshold", "0",
			"-force_key_frames", "expr:gte(t,n_forced*1)",
		}
	}
	if transcode {
		switch profile {
		case profilePlexSafe:
			// More conservative output tends to make Plex Web's DASH startup happier.
			codecArgs = append(codecArgs,
				"-vf", "fps=30000/1001,format=yuv420p",
				"-b:v", "2200k",
				"-maxrate", "2500k",
				"-bufsize", "5000k",
				"-c:a", "libmp3lame",
				"-ac", "2",
				"-ar", "48000",
				"-b:a", "128k",
				"-af", "aresample=async=1:first_pts=0",
			)
		case profileAACCFR:
			codecArgs = append(codecArgs,
				// Browser-oriented "boring" output to help Plex Web DASH startup.
				"-vf", "fps=30000/1001,scale='min(854,iw)':-2,format=yuv420p",
				"-profile:v", "baseline",
				"-level:v", "3.1",
				"-bf", "0",
				"-refs", "1",
				"-b:v", "1400k",
				"-maxrate", "1400k",
				"-bufsize", "1400k",
				"-c:a", "aac",
				"-profile:a", "aac_low",
				"-ac", "2",
				"-ar", "48000",
				"-b:a", "96k",
				"-af", "aresample=async=1:first_pts=0",
				"-x264-params", "repeat-headers=1:keyint=30:min-keyint=30:scenecut=0:force-cfr=1:nal-hrd=cbr:bframes=0:aud=1",
			)
		case profileVideoOnly:
			codecArgs = append(codecArgs,
				"-vf", "fps=30000/1001,format=yuv420p",
				"-b:v", "2200k",
				"-maxrate", "2500k",
				"-bufsize", "5000k",
				"-an",
			)
		case profileLowBitrate:
			codecArgs = append(codecArgs,
				"-vf", "fps=30000/1001,scale='trunc(iw/2)*2:trunc(ih/2)*2',format=yuv420p",
				"-b:v", "1400k",
				"-maxrate", "1700k",
				"-bufsize", "3400k",
				"-c:a", "aac",
				"-profile:a", "aac_low",
				"-ac", "2",
				"-ar", "48000",
				"-b:a", "96k",
				"-af", "aresample=async=1:first_pts=0",
			)
		case profileDashFast:
			// Aggressively optimize for Plex Web DASH startup readiness.
			codecArgs = append(codecArgs,
				"-vf", "fps=30000/1001,scale='min(1280,iw)':-2,format=yuv420p",
				"-b:v", "1800k",
				"-maxrate", "1800k",
				"-bufsize", "1800k",
				"-c:a", "aac",
				"-profile:a", "aac_low",
				"-ac", "2",
				"-ar", "48000",
				"-b:a", "96k",
				"-af", "aresample=async=1:first_pts=0",
				"-x264-params", "repeat-headers=1:keyint=30:min-keyint=30:scenecut=0:force-cfr=1:nal-hrd=cbr",
			)
		case profilePMSXcode:
			// Handled in the transcode base branch above.
		default:
			codecArgs = append(codecArgs,
				"-b:v", "3500k",
				"-maxrate", "4000k",
				"-bufsize", "8000k",
				"-c:a", "aac",
				"-profile:a", "aac_low",
				"-ac", "2",
				"-ar", "48000",
				"-b:a", "128k",
				"-af", "aresample=async=1:first_pts=0",
			)
		}
		// Help Plex's live parser lock onto a clean TS timeline/header faster.
		codecArgs = append(codecArgs,
			"-muxrate", "3000000",
			"-pcr_period", "20",
			"-pat_period", "0.05",
		)
	}
	codecArgs = append(codecArgs,
		"-flush_packets", "1",
		"-max_interleave_delta", "0",
		"-muxdelay", "0",
		"-muxpreload", "0",
		"-mpegts_flags", mpegtsFlags,
		"-f", "mpegts",
		"pipe:1",
	)
	return codecArgs
}

func (g *Gateway) relayHLSWithFFmpeg(
	w http.ResponseWriter,
	r *http.Request,
	ffmpegPath string,
	playlistURL string,
	channelName string,
	channelID string,
	guideNumber string,
	tvgID string,
	start time.Time,
	transcode bool,
	bufferBytes int,
	forcedProfile string,
) error {
	reqField := gatewayReqIDField(r.Context())
	profile := g.profileForChannelMeta(channelID, guideNumber, tvgID)
	if strings.TrimSpace(forcedProfile) != "" {
		profile = normalizeProfileName(forcedProfile)
	}
	ffmpegPlaylistURL, ffmpegInputHost, ffmpegInputIP := canonicalizeFFmpegInputURL(r.Context(), playlistURL)

	// HLS inputs are more sensitive to over-aggressive probing/low-latency flags than raw TS.
	// Default to safer probing and allow env overrides when chasing startup races.
	hlsAnalyzeDurationUs := getenvInt("PLEX_TUNER_FFMPEG_HLS_ANALYZEDURATION_US", 5000000)
	hlsProbeSize := getenvInt("PLEX_TUNER_FFMPEG_HLS_PROBESIZE", 5000000)
	hlsRWTimeoutUs := getenvInt("PLEX_TUNER_FFMPEG_HLS_RW_TIMEOUT_US", 15000000)
	hlsLiveStartIndex := getenvInt("PLEX_TUNER_FFMPEG_HLS_LIVE_START_INDEX", -3)
	hlsUseNoBuffer := getenvBool("PLEX_TUNER_FFMPEG_HLS_NOBUFFER", false)
	// Let ffmpeg's HLS demuxer manage live playlist refreshes by default.
	// Generic HTTP reconnect flags (especially reconnect-at-EOF) can cause
	// live .m3u8 inputs to loop on playlist EOF and never start segment reads.
	hlsReconnect := getenvBool("PLEX_TUNER_FFMPEG_HLS_RECONNECT", false)
	hlsRealtime := getenvBool("PLEX_TUNER_FFMPEG_HLS_REALTIME", false)
	hlsLogLevel := strings.TrimSpace(os.Getenv("PLEX_TUNER_FFMPEG_HLS_LOGLEVEL"))
	if hlsLogLevel == "" {
		hlsLogLevel = "error"
	}
	fflags := "+discardcorrupt+genpts"
	if hlsUseNoBuffer {
		fflags += "+nobuffer"
	}
	args := []string{
		"-nostdin",
		"-hide_banner",
		"-loglevel", hlsLogLevel,
		"-fflags", fflags,
		"-analyzeduration", strconv.Itoa(hlsAnalyzeDurationUs),
		"-probesize", strconv.Itoa(hlsProbeSize),
		"-rw_timeout", strconv.Itoa(hlsRWTimeoutUs),
		"-user_agent", "PlexTuner/1.0",
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
	if hlsRealtime {
		// Pace ffmpeg reads to input timestamps (wall-clock-ish) to avoid racing
		// far ahead of Plex's live consumer attach during startup on HLS inputs.
		args = append(args, "-re")
	}
	if hlsLiveStartIndex != 0 {
		args = append(args, "-live_start_index", strconv.Itoa(hlsLiveStartIndex))
	}
	if g.ProviderUser != "" || g.ProviderPass != "" {
		auth := base64.StdEncoding.EncodeToString([]byte(g.ProviderUser + ":" + g.ProviderPass))
		args = append(args, "-headers", "Authorization: Basic "+auth+"\r\n")
	}
	args = append(args, "-i", ffmpegPlaylistURL)
	args = append(args, buildFFmpegMPEGTSCodecArgs(transcode, profile)...)

	cmd := exec.CommandContext(r.Context(), ffmpegPath, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	modeLabel := "ffmpeg-remux"
	if transcode {
		modeLabel = "ffmpeg-transcode"
	}
	if ffmpegInputHost != "" && ffmpegInputIP != "" {
		log.Printf("gateway:%s channel=%q id=%s %s input-host-resolved %q=>%q",
			reqField, channelName, channelID, modeLabel, ffmpegInputHost, ffmpegInputIP)
	}
	log.Printf("gateway:%s channel=%q id=%s %s profile=%s", reqField, channelName, channelID, modeLabel, profile)
	log.Printf("gateway:%s channel=%q id=%s %s hls-input analyzeduration_us=%d probesize=%d rw_timeout_us=%d live_start_index=%d nobuffer=%t reconnect=%t realtime=%t loglevel=%s",
		reqField, channelName, channelID, modeLabel, hlsAnalyzeDurationUs, hlsProbeSize, hlsRWTimeoutUs, hlsLiveStartIndex, hlsUseNoBuffer, hlsReconnect, hlsRealtime, hlsLogLevel)
	// In web-safe transcode modes, hold back the first bytes (and optionally prepend a short
	// deterministic H264/AAC TS bootstrap) so Plex's live DASH packager gets a clean start.
	startupMin := getenvInt("PLEX_TUNER_WEBSAFE_STARTUP_MIN_BYTES", 65536)
	startupMax := getenvInt("PLEX_TUNER_WEBSAFE_STARTUP_MAX_BYTES", 786432)
	startupTimeoutMs := getenvInt("PLEX_TUNER_WEBSAFE_STARTUP_TIMEOUT_MS", 12000)
	enableBootstrap := transcode && getenvBool("PLEX_TUNER_WEBSAFE_BOOTSTRAP", true)
	enableTimeoutBootstrap := getenvBool("PLEX_TUNER_WEBSAFE_TIMEOUT_BOOTSTRAP", true)
	continueOnStartupTimeout := transcode && getenvBool("PLEX_TUNER_WEBSAFE_TIMEOUT_CONTINUE_FFMPEG", false)
	bootstrapSec := getenvFloat("PLEX_TUNER_WEBSAFE_BOOTSTRAP_SECONDS", 1.5)
	requireGoodStart := transcode && getenvBool("PLEX_TUNER_WEBSAFE_REQUIRE_GOOD_START", true)
	enableNullTSKeepalive := transcode && getenvBool("PLEX_TUNER_WEBSAFE_NULL_TS_KEEPALIVE", false)
	nullTSKeepaliveMs := getenvInt("PLEX_TUNER_WEBSAFE_NULL_TS_KEEPALIVE_MS", 100)
	nullTSKeepalivePackets := getenvInt("PLEX_TUNER_WEBSAFE_NULL_TS_KEEPALIVE_PACKETS", 1)
	// PAT+PMT keepalive: sends real program-structure packets (not just null PIDs) so
	// Plex's DASH packager can instantiate its consumer before the first IDR arrives.
	enableProgramKeepalive := transcode && getenvBool("PLEX_TUNER_WEBSAFE_PROGRAM_KEEPALIVE", false)
	programKeepaliveMs := getenvInt("PLEX_TUNER_WEBSAFE_PROGRAM_KEEPALIVE_MS", 500)
	// Do not run both keepalives concurrently against the same ResponseWriter: parallel
	// writes can interleave/chunk-corrupt HTTP output and manifest as short writes.
	if enableProgramKeepalive && enableNullTSKeepalive {
		enableNullTSKeepalive = false
		log.Printf("gateway:%s channel=%q id=%s %s keepalive-select program=true null=false reason=program-priority",
			reqField, channelName, channelID, modeLabel)
	}
	var bodyOut io.Writer
	flushBody := func() {}
	responseStarted := false
	startResponse := func() {
		if responseStarted {
			return
		}
		w.Header().Set("Content-Type", "video/mp2t")
		w.Header().Del("Content-Length")
		w.WriteHeader(http.StatusOK)
		bodyOut, flushBody = streamWriter(w, bufferBytes)
		responseStarted = true
	}
	defer func() { flushBody() }()
	stopNullTSKeepalive := func(string) {}
	stopPATMPTKeepalive := func(string) {}
	bootstrapAlreadySent := false
	var prefetch []byte
	if transcode && startupMin > 0 {
		// Send HTTP 200 + Content-Type headers immediately, before any body bytes.
		// This separates "connection accepted" from "bytes available" and prevents
		// Plex from timing out on the HTTP response header wait during startup gate.
		startResponse()
		if fw, ok := w.(http.Flusher); ok {
			fw.Flush()
		}
		if enableNullTSKeepalive {
			flusher, _ := w.(http.Flusher)
			stopNullTSKeepalive = startNullTSKeepalive(
				r.Context(),
				bodyOut,
				flushBody,
				flusher,
				channelName,
				channelID,
				modeLabel,
				start,
				time.Duration(nullTSKeepaliveMs)*time.Millisecond,
				nullTSKeepalivePackets,
			)
			log.Printf("gateway:%s channel=%q id=%s %s null-ts-keepalive start interval_ms=%d packets=%d",
				reqField, channelName, channelID, modeLabel, nullTSKeepaliveMs, nullTSKeepalivePackets)
		}
		if enableProgramKeepalive {
			flusher, _ := w.(http.Flusher)
			stopPATMPTKeepalive = startPATMPTKeepalive(
				r.Context(),
				bodyOut,
				flushBody,
				flusher,
				channelName,
				channelID,
				modeLabel,
				start,
				time.Duration(programKeepaliveMs)*time.Millisecond,
			)
			log.Printf("gateway:%s channel=%q id=%s %s pat-pmt-keepalive start interval_ms=%d",
				reqField, channelName, channelID, modeLabel, programKeepaliveMs)
		}
		type prefetchRes struct {
			b     []byte
			err   error
			state startSignalState
		}
		ch := make(chan prefetchRes, 1)
		go func() {
			buf := make([]byte, 0, startupMin)
			tmp := make([]byte, 32768)
			if startupMax < startupMin {
				startupMax = startupMin
			}
			for {
				n, rerr := stdout.Read(tmp)
				if n > 0 {
					room := startupMax - len(buf)
					if room > 0 {
						if n > room {
							n = room
						}
						buf = append(buf, tmp[:n]...)
					}
					st := looksLikeGoodTSStart(buf)
					good := !requireGoodStart || (st.HasIDR && st.HasAAC && st.TSLikePackets >= 8)
					if len(buf) >= startupMin && good {
						ch <- prefetchRes{b: buf, state: st}
						return
					}
					if len(buf) >= startupMax {
						ch <- prefetchRes{b: buf, state: st}
						return
					}
				}
				if rerr != nil {
					st := looksLikeGoodTSStart(buf)
					if len(buf) > 0 {
						ch <- prefetchRes{b: buf, err: rerr, state: st}
					} else {
						ch <- prefetchRes{err: rerr, state: st}
					}
					return
				}
			}
		}()
		timeout := time.Duration(startupTimeoutMs) * time.Millisecond
		if timeout <= 0 {
			timeout = 12 * time.Second
		}
		select {
		case pr := <-ch:
			stopReason := "startup-gate-ready"
			if pr.err != nil && len(pr.b) == 0 {
				stopReason = "startup-gate-prefetch-error"
			}
			stopNullTSKeepalive(stopReason)
			stopPATMPTKeepalive(stopReason)
			prefetch = pr.b
			if pr.state.AlignedOffset > 0 && pr.state.AlignedOffset < len(prefetch) {
				prefetch = prefetch[pr.state.AlignedOffset:]
			}
			if len(prefetch) > 0 {
				log.Printf(
					"gateway:%s channel=%q id=%s %s startup-gate buffered=%d min=%d max=%d timeout_ms=%d ts_pkts=%d idr=%t aac=%t align=%d",
					reqField, channelName, channelID, modeLabel, len(prefetch), startupMin, startupMax, startupTimeoutMs,
					pr.state.TSLikePackets, pr.state.HasIDR, pr.state.HasAAC, pr.state.AlignedOffset,
				)
			}
			if pr.err != nil && len(prefetch) == 0 {
				_ = cmd.Process.Kill()
				waitErr := cmd.Wait()
				msg := strings.TrimSpace(stderr.String())
				if msg == "" {
					msg = pr.err.Error()
				}
				if pr.err != nil {
					errOut := error(pr.err)
					if waitErr != nil && waitErr.Error() != pr.err.Error() {
						errOut = fmt.Errorf("%w (wait=%v)", pr.err, waitErr)
					}
					return ffmpegRelayErr("startup-gate-prefetch", errOut, stderr.String())
				}
				return errors.New(msg)
			}
		case <-time.After(timeout):
			stopNullTSKeepalive("startup-gate-timeout")
			stopPATMPTKeepalive("startup-gate-timeout")
			if responseStarted && enableBootstrap && enableTimeoutBootstrap && bootstrapSec > 0 {
				if err := writeBootstrapTS(r.Context(), ffmpegPath, bodyOut, channelName, channelID, bootstrapSec, profile); err != nil {
					log.Printf("gateway:%s channel=%q id=%s %s timeout-bootstrap failed: %v", reqField, channelName, channelID, modeLabel, err)
				} else {
					bootstrapAlreadySent = true
					flushBody()
					log.Printf("gateway:%s channel=%q id=%s %s timeout-bootstrap emitted before relay fallback", reqField, channelName, channelID, modeLabel)
				}
			} else if responseStarted && enableBootstrap && !enableTimeoutBootstrap {
				log.Printf("gateway:%s channel=%q id=%s %s timeout-bootstrap disabled before relay fallback", reqField, channelName, channelID, modeLabel)
			}
			log.Printf("gateway:%s channel=%q id=%s %s startup-gate timeout after=%dms", reqField, channelName, channelID, modeLabel, startupTimeoutMs)
			if continueOnStartupTimeout {
				log.Printf("gateway:%s channel=%q id=%s %s startup-gate timeout continue-ffmpeg=true", reqField, channelName, channelID, modeLabel)
				break
			}
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			msg := strings.TrimSpace(stderr.String())
			if msg == "" {
				msg = "startup gate timeout"
			}
			if msg == "startup gate timeout" {
				return ffmpegRelayErr("startup-gate-timeout", errors.New(msg), stderr.String())
			}
			return ffmpegRelayErr("startup-gate-timeout", errors.New(msg), stderr.String())
		case <-r.Context().Done():
			stopNullTSKeepalive("startup-gate-cancel")
			stopPATMPTKeepalive("startup-gate-cancel")
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return nil
		}
	}

	startResponse()

	if enableBootstrap && bootstrapSec > 0 && !bootstrapAlreadySent {
		if err := writeBootstrapTS(r.Context(), ffmpegPath, bodyOut, channelName, channelID, bootstrapSec, profile); err != nil {
			log.Printf("gateway:%s channel=%q id=%s bootstrap failed: %v", reqField, channelName, channelID, err)
		}
		if joinDelayMs := getenvInt("PLEX_TUNER_WEBSAFE_JOIN_DELAY_MS", 0); joinDelayMs > 0 {
			if joinDelayMs > 5000 {
				joinDelayMs = 5000
			}
			log.Printf("gateway: channel=%q id=%s websafe-join-delay ms=%d", channelName, channelID, joinDelayMs)
			select {
			case <-time.After(time.Duration(joinDelayMs) * time.Millisecond):
			case <-r.Context().Done():
				return nil
			}
		}
	}

	mainReader := io.Reader(stdout)
	if len(prefetch) > 0 {
		mainReader = io.MultiReader(bytes.NewReader(prefetch), stdout)
	}
	dst := io.Writer(bodyOut)
	if fw, ok := w.(http.Flusher); ok {
		dst = &firstWriteLogger{
			w:           flushWriter{w: bodyOut, f: fw},
			channelName: channelName,
			channelID:   channelID,
			reqID:       gatewayReqIDFromContext(r.Context()),
			modeLabel:   modeLabel,
			start:       start,
		}
	} else {
		dst = &firstWriteLogger{
			w:           bodyOut,
			channelName: channelName,
			channelID:   channelID,
			reqID:       gatewayReqIDFromContext(r.Context()),
			modeLabel:   modeLabel,
			start:       start,
		}
	}
	dst = maybeWrapTSInspectorWriter(dst, gatewayReqIDFromContext(r.Context()), channelName, channelID, guideNumber, tvgID, modeLabel, start)
	if c, ok := dst.(interface{ Close() }); ok {
		defer c.Close()
	}
	n, copyErr := io.Copy(dst, mainReader)
	waitErr := cmd.Wait()

	if r.Context().Err() != nil {
		log.Printf("gateway:%s channel=%q id=%s %s client-done bytes=%d dur=%s",
			reqField, channelName, channelID, modeLabel, n, time.Since(start).Round(time.Millisecond))
		return nil
	}
	if copyErr != nil && r.Context().Err() == nil {
		return ffmpegRelayErr("copy", copyErr, stderr.String())
	}
	if waitErr != nil && r.Context().Err() == nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			return ffmpegRelayErr("wait", waitErr, stderr.String())
		}
		return ffmpegRelayErr("wait", errors.New(msg), stderr.String())
	}
	log.Printf("gateway:%s channel=%q id=%s %s bytes=%d dur=%s",
		reqField, channelName, channelID, modeLabel, n, time.Since(start).Round(time.Millisecond))
	return nil
}

type firstWriteLogger struct {
	w           io.Writer
	once        sync.Once
	channelName string
	channelID   string
	reqID       string
	modeLabel   string
	start       time.Time
}

func (f *firstWriteLogger) Write(p []byte) (int, error) {
	f.once.Do(func() {
		if f.modeLabel == "" {
			f.modeLabel = "ffmpeg-remux"
		}
		if f.reqID != "" {
			log.Printf("gateway: req=%s channel=%q id=%s %s first-bytes=%d startup=%s",
				f.reqID, f.channelName, f.channelID, f.modeLabel, len(p), time.Since(f.start).Round(time.Millisecond))
			return
		}
		log.Printf("gateway: channel=%q id=%s %s first-bytes=%d startup=%s",
			f.channelName, f.channelID, f.modeLabel, len(p), time.Since(f.start).Round(time.Millisecond))
	})
	return f.w.Write(p)
}

type flushWriter struct {
	w io.Writer
	f http.Flusher
}

func (fw flushWriter) Write(p []byte) (int, error) {
	n, err := fw.w.Write(p)
	if n > 0 {
		fw.f.Flush()
	}
	return n, err
}

type hlsRelayFFmpegOutputWriter struct {
	w             http.ResponseWriter
	bodyOut       io.Writer
	flushBody     func()
	flusher       http.Flusher
	started       bool
	startedSignal *atomic.Bool
}

func (w *hlsRelayFFmpegOutputWriter) Write(p []byte) (int, error) {
	if !w.started {
		w.w.Header().Set("Content-Type", "video/mp2t")
		w.w.Header().Del("Content-Length")
		w.w.WriteHeader(http.StatusOK)
		w.started = true
		if w.startedSignal != nil {
			w.startedSignal.Store(true)
		}
	}
	n, err := w.bodyOut.Write(p)
	if n > 0 {
		if w.flushBody != nil {
			w.flushBody()
		}
		if w.flusher != nil {
			w.flusher.Flush()
		}
		if w.startedSignal != nil {
			w.startedSignal.Store(true)
		}
	}
	return n, err
}

type hlsRelayFFmpegStdinNormalizer struct {
	stdin         io.WriteCloser
	done          chan error
	closeOnce     sync.Once
	responseStart atomic.Bool
}

func (n *hlsRelayFFmpegStdinNormalizer) Write(p []byte) (int, error) {
	if n == nil || n.stdin == nil {
		return 0, io.ErrClosedPipe
	}
	return n.stdin.Write(p)
}

func (n *hlsRelayFFmpegStdinNormalizer) ResponseStarted() bool {
	if n == nil {
		return false
	}
	return n.responseStart.Load()
}

func (n *hlsRelayFFmpegStdinNormalizer) CloseInput() error {
	if n == nil || n.stdin == nil {
		return nil
	}
	var err error
	n.closeOnce.Do(func() {
		err = n.stdin.Close()
	})
	return err
}

func (n *hlsRelayFFmpegStdinNormalizer) CloseAndWait() error {
	if n == nil {
		return nil
	}
	_ = n.CloseInput()
	if n.done == nil {
		return nil
	}
	return <-n.done
}

func (g *Gateway) startHLSRelayFFmpegStdinNormalizer(
	w http.ResponseWriter,
	r *http.Request,
	ffmpegPath string,
	channelName string,
	channelID string,
	start time.Time,
	transcode bool,
	profile string,
	bodyOut io.Writer,
	flushBody func(),
	bufferBytes int,
	responseStarted bool,
) (*hlsRelayFFmpegStdinNormalizer, error) {
	reqField := gatewayReqIDField(r.Context())
	modeLabel := "hls-relay-ffmpeg-stdin-remux"
	if transcode {
		modeLabel = "hls-relay-ffmpeg-stdin-transcode"
	}
	stdinAnalyzeDurationUs := getenvInt("PLEX_TUNER_FFMPEG_STDIN_ANALYZEDURATION_US", 3000000)
	stdinProbeSize := getenvInt("PLEX_TUNER_FFMPEG_STDIN_PROBESIZE", 3000000)
	stdinUseNoBuffer := getenvBool("PLEX_TUNER_FFMPEG_STDIN_NOBUFFER", false)
	stdinLogLevel := strings.TrimSpace(os.Getenv("PLEX_TUNER_FFMPEG_STDIN_LOGLEVEL"))
	if stdinLogLevel == "" {
		stdinLogLevel = strings.TrimSpace(os.Getenv("PLEX_TUNER_FFMPEG_HLS_LOGLEVEL"))
	}
	if stdinLogLevel == "" {
		stdinLogLevel = "error"
	}
	fflags := "+discardcorrupt+genpts"
	if stdinUseNoBuffer {
		fflags += "+nobuffer"
	}
	args := []string{
		"-nostdin",
		"-hide_banner",
		"-loglevel", stdinLogLevel,
		"-fflags", fflags,
		"-analyzeduration", strconv.Itoa(stdinAnalyzeDurationUs),
		"-probesize", strconv.Itoa(stdinProbeSize),
		"-f", "mpegts",
		"-i", "pipe:0",
	}
	args = append(args, buildFFmpegMPEGTSCodecArgs(transcode, profile)...)

	cmd := exec.CommandContext(r.Context(), ffmpegPath, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, err
	}

	norm := &hlsRelayFFmpegStdinNormalizer{
		stdin: stdin,
		done:  make(chan error, 1),
	}
	if responseStarted {
		norm.responseStart.Store(true)
	}
	flusher, _ := w.(http.Flusher)
	out := &hlsRelayFFmpegOutputWriter{
		w:             w,
		bodyOut:       bodyOut,
		flushBody:     flushBody,
		flusher:       flusher,
		started:       responseStarted,
		startedSignal: &norm.responseStart,
	}
	dst := io.Writer(out)
	if flusher != nil {
		// firstWriteLogger logs first client-visible bytes; flush behavior is handled by hlsRelayFFmpegOutputWriter.
		dst = &firstWriteLogger{
			w:           dst,
			channelName: channelName,
			channelID:   channelID,
			reqID:       gatewayReqIDFromContext(r.Context()),
			modeLabel:   modeLabel,
			start:       start,
		}
	} else {
		dst = &firstWriteLogger{
			w:           dst,
			channelName: channelName,
			channelID:   channelID,
			reqID:       gatewayReqIDFromContext(r.Context()),
			modeLabel:   modeLabel,
			start:       start,
		}
	}

	log.Printf("gateway:%s channel=%q id=%s %s start buffer=%d profile=%s analyzeduration_us=%d probesize=%d nobuffer=%t loglevel=%s",
		reqField, channelName, channelID, modeLabel, bufferBytes, profile, stdinAnalyzeDurationUs, stdinProbeSize, stdinUseNoBuffer, stdinLogLevel)
	go func() {
		nBytes, copyErr := io.Copy(dst, stdout)
		if flushBody != nil {
			flushBody()
		}
		waitErr := cmd.Wait()
		if r.Context().Err() != nil {
			log.Printf("gateway:%s channel=%q id=%s %s client-done bytes=%d dur=%s",
				reqField, channelName, channelID, modeLabel, nBytes, time.Since(start).Round(time.Millisecond))
			norm.done <- nil
			return
		}
		if copyErr != nil {
			norm.done <- ffmpegRelayErr("hls-relay-stdin-copy", copyErr, stderr.String())
			return
		}
		if waitErr != nil {
			norm.done <- ffmpegRelayErr("hls-relay-stdin-wait", waitErr, stderr.String())
			return
		}
		log.Printf("gateway:%s channel=%q id=%s %s bytes=%d dur=%s",
			reqField, channelName, channelID, modeLabel, nBytes, time.Since(start).Round(time.Millisecond))
		norm.done <- nil
	}()
	return norm, nil
}

func bootstrapAudioArgsForProfile(profile string) []string {
	switch normalizeProfileName(profile) {
	case profilePlexSafe:
		return []string{
			"-c:a", "libmp3lame",
			"-ac", "2",
			"-ar", "48000",
			"-b:a", "128k",
			"-af", "aresample=async=1:first_pts=0",
		}
	case profilePMSXcode:
		return []string{
			"-c:a", "mp2",
			"-ac", "2",
			"-ar", "48000",
			"-b:a", "128k",
			"-af", "aresample=async=1:first_pts=0",
		}
	case profileVideoOnly:
		return []string{"-an"}
	default:
		return []string{
			"-c:a", "aac",
			"-profile:a", "aac_low",
			"-ac", "2",
			"-ar", "48000",
			"-b:a", "96k",
			"-af", "aresample=async=1:first_pts=0",
		}
	}
}

func writeBootstrapTS(ctx context.Context, ffmpegPath string, dst io.Writer, channelName, channelID string, seconds float64, profile string) error {
	if seconds <= 0 {
		return nil
	}
	if seconds > 5 {
		seconds = 5
	}
	mpegtsFlags := mpegTSFlagsWithOptionalInitialDiscontinuity()
	args := []string{
		"-nostdin",
		"-hide_banner",
		"-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=black:s=1280x720:r=30000/1001",
		"-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo",
		"-t", strconv.FormatFloat(seconds, 'f', 2, 64),
		"-shortest",
		"-map", "0:v:0",
	}
	if normalizeProfileName(profile) != profileVideoOnly {
		// Keep bootstrap audio codec aligned with the active stream profile so Plex
		// does not see a mid-stream audio codec switch (for example AAC -> MP3).
		args = append(args, "-map", "1:a:0")
	}
	args = append(args,
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-tune", "zerolatency",
		"-pix_fmt", "yuv420p",
		"-g", "30",
		"-keyint_min", "30",
		"-sc_threshold", "0",
		"-x264-params", "repeat-headers=1:keyint=30:min-keyint=30:scenecut=0:force-cfr=1:nal-hrd=cbr",
		"-b:v", "900k",
		"-maxrate", "900k",
		"-bufsize", "900k",
	)
	args = append(args, bootstrapAudioArgsForProfile(profile)...)
	args = append(args,
		"-flush_packets", "1",
		"-muxdelay", "0",
		"-muxpreload", "0",
		"-mpegts_flags", mpegtsFlags,
		"-f", "mpegts",
		"pipe:1",
	)
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	n, copyErr := io.Copy(dst, stdout)
	waitErr := cmd.Wait()
	if copyErr != nil {
		return copyErr
	}
	if waitErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = waitErr.Error()
		}
		return errors.New(msg)
	}
	log.Printf("gateway:%s channel=%q id=%s bootstrap-ts bytes=%d dur=%.2fs profile=%s",
		gatewayReqIDField(ctx), channelName, channelID, n, seconds, normalizeProfileName(profile))
	return nil
}

func (g *Gateway) relayHLSAsTS(
	w http.ResponseWriter,
	r *http.Request,
	client *http.Client,
	playlistURL string,
	initialPlaylist []byte,
	channelName string,
	channelID string,
	guideNumber string,
	tvgID string,
	start time.Time,
	transcode bool,
	forcedProfile string,
	bufferBytes int,
	responseStarted bool,
) (retErr error) {
	reqField := gatewayReqIDField(r.Context())
	if client == nil {
		client = httpclient.ForStreaming()
	}
	profile := g.profileForChannelMeta(channelID, guideNumber, tvgID)
	if strings.TrimSpace(forcedProfile) != "" {
		profile = normalizeProfileName(forcedProfile)
	}
	sw, flush := streamWriter(w, bufferBytes)
	defer flush()
	flusher, _ := w.(http.Flusher)
	seen := map[string]struct{}{}
	lastProgress := time.Now()
	sentBytes := int64(0)
	sentSegments := 0
	headerSent := responseStarted
	firstRelayBytesLogged := false
	currentPlaylistURL := playlistURL
	currentPlaylist := initialPlaylist
	relayLogLabel := "hls-relay"

	enableFFmpegStdinNormalize := getenvBool("PLEX_TUNER_HLS_RELAY_FFMPEG_STDIN_NORMALIZE", false)
	var normalizer *hlsRelayFFmpegStdinNormalizer
	if enableFFmpegStdinNormalize {
		if ffmpegPath, ffmpegErr := resolveFFmpegPath(); ffmpegErr == nil {
			norm, err := g.startHLSRelayFFmpegStdinNormalizer(
				w,
				r,
				ffmpegPath,
				channelName,
				channelID,
				start,
				transcode,
				profile,
				sw,
				flush,
				bufferBytes,
				responseStarted,
			)
			if err != nil {
				log.Printf("gateway:%s channel=%q id=%s hls-relay-ffmpeg-stdin start failed (falling back to raw relay): %v",
					reqField, channelName, channelID, err)
			} else {
				normalizer = norm
				relayLogLabel = "hls-relay-ffmpeg-stdin-feed"
				log.Printf("gateway:%s channel=%q id=%s hls-relay-ffmpeg-stdin enabled transcode=%t profile=%s",
					reqField, channelName, channelID, transcode, profile)
			}
		} else if strings.TrimSpace(os.Getenv("PLEX_TUNER_FFMPEG_PATH")) != "" {
			log.Printf("gateway:%s channel=%q id=%s hls-relay-ffmpeg-stdin ffmpeg unavailable path=%q err=%v",
				reqField, channelName, channelID, os.Getenv("PLEX_TUNER_FFMPEG_PATH"), ffmpegErr)
		} else if transcode {
			log.Printf("gateway:%s channel=%q id=%s hls-relay-ffmpeg-stdin ffmpeg unavailable transcode-requested=true err=%v", reqField, channelName, channelID, ffmpegErr)
		}
	}
	if normalizer != nil {
		defer func() {
			if err := normalizer.CloseAndWait(); err != nil && retErr == nil && r.Context().Err() == nil {
				retErr = err
			}
		}()
	}
	if responseStarted {
		if normalizer != nil {
			log.Printf("gateway:%s channel=%q id=%s %s splice-start prior-bytes=true", reqField, channelName, channelID, relayLogLabel)
		} else {
			log.Printf("gateway:%s channel=%q id=%s hls-relay splice-start prior-bytes=true", reqField, channelName, channelID)
		}
	}
	clientStarted := func() bool {
		return headerSent || (normalizer != nil && normalizer.ResponseStarted())
	}

	for {
		select {
		case <-r.Context().Done():
			log.Printf("gateway:%s channel=%q id=%s %s client-done segs=%d bytes=%d dur=%s",
				reqField, channelName, channelID, relayLogLabel, sentSegments, sentBytes, time.Since(start).Round(time.Millisecond))
			return nil
		default:
		}

		mediaLines := hlsMediaLines(currentPlaylist)
		// Prune seen to only segment URLs still in playlist so map doesn't grow unbounded.
		segmentURLSet := make(map[string]struct{}, len(mediaLines))
		for _, u := range mediaLines {
			if !strings.HasSuffix(strings.ToLower(u), ".m3u8") {
				segmentURLSet[u] = struct{}{}
			}
		}
		for u := range seen {
			if _, inPlaylist := segmentURLSet[u]; !inPlaylist {
				delete(seen, u)
			}
		}
		if len(mediaLines) == 0 {
			if !clientStarted() {
				return errors.New("hls playlist has no media lines")
			}
			if time.Since(lastProgress) > 12*time.Second {
				return errors.New("hls relay stalled (no media lines)")
			}
			time.Sleep(1 * time.Second)
		} else {
			progressThisPass := false
			for _, segURL := range mediaLines {
				if strings.HasSuffix(strings.ToLower(segURL), ".m3u8") {
					// Some providers return a master/variant indirection; follow one level.
					next, err := g.fetchAndRewritePlaylist(r, client, segURL)
					if err != nil {
						if !clientStarted() {
							return err
						}
						log.Printf("gateway:%s channel=%q id=%s nested-playlist fetch failed url=%s err=%v",
							reqField, channelName, channelID, safeurl.RedactURL(segURL), err)
						continue
					}
					currentPlaylistURL = segURL
					currentPlaylist = next
					progressThisPass = true
					break
				}
				if _, ok := seen[segURL]; ok {
					continue
				}
				seen[segURL] = struct{}{}
				var segOut io.Writer = sw
				var spliceWriter *tsDiscontinuitySpliceWriter
				if normalizer != nil {
					segOut = normalizer
				} else if responseStarted && sentSegments == 0 {
					spliceWriter = newTSDiscontinuitySpliceWriter(r.Context(), sw, channelName, channelID)
					segOut = spliceWriter
				}
				n, err := g.fetchAndWriteSegment(w, segOut, r, client, segURL, headerSent || normalizer != nil)
				if err == nil && spliceWriter != nil {
					if ferr := spliceWriter.FlushRemainder(); ferr != nil {
						err = ferr
					}
				}
				if err != nil {
					if isClientDisconnectWriteError(err) {
						if n > 0 {
							sentBytes += n
						}
						log.Printf("gateway:%s channel=%q id=%s %s client-done write-closed segs=%d bytes=%d dur=%s",
							reqField, channelName, channelID, relayLogLabel, sentSegments, sentBytes, time.Since(start).Round(time.Millisecond))
						return nil
					}
					if !clientStarted() {
						return err
					}
					if r.Context().Err() != nil {
						return nil
					}
					log.Printf("gateway:%s channel=%q id=%s segment fetch failed url=%s err=%v",
						reqField, channelName, channelID, safeurl.RedactURL(segURL), err)
					continue
				}
				if normalizer != nil {
					headerSent = headerSent || normalizer.ResponseStarted()
				}
				if normalizer == nil && !headerSent {
					headerSent = true
					if flusher != nil {
						flusher.Flush()
					}
				}
				if n > 0 {
					if !firstRelayBytesLogged {
						firstRelayBytesLogged = true
						if normalizer != nil {
							log.Printf("gateway:%s channel=%q id=%s hls-relay-ffmpeg-stdin first-feed-bytes=%d seg=%q startup=%s",
								reqField, channelName, channelID, n, safeurl.RedactURL(segURL), time.Since(start).Round(time.Millisecond))
						} else {
							log.Printf("gateway:%s channel=%q id=%s hls-relay first-bytes=%d seg=%q startup=%s",
								reqField, channelName, channelID, n, safeurl.RedactURL(segURL), time.Since(start).Round(time.Millisecond))
						}
					}
					sentBytes += n
					sentSegments++
					lastProgress = time.Now()
					progressThisPass = true
				}
				if normalizer == nil && flusher != nil {
					flusher.Flush()
				}
			}

			if !progressThisPass && time.Since(lastProgress) > 12*time.Second {
				if !clientStarted() {
					return errors.New("hls relay stalled before first segment")
				}
				log.Printf("gateway:%s channel=%q id=%s %s ended no-new-segments segs=%d bytes=%d dur=%s",
					reqField, channelName, channelID, relayLogLabel, sentSegments, sentBytes, time.Since(start).Round(time.Millisecond))
				return nil
			}
			sleepHLSRefresh(currentPlaylist)
		}

		next, err := g.fetchAndRewritePlaylist(r, client, currentPlaylistURL)
		if err != nil {
			if !clientStarted() {
				return err
			}
			if r.Context().Err() != nil {
				return nil
			}
			if time.Since(lastProgress) > 12*time.Second {
				return err
			}
			log.Printf("gateway:%s channel=%q id=%s playlist refresh failed url=%s err=%v",
				reqField, channelName, channelID, safeurl.RedactURL(currentPlaylistURL), err)
			sleepHLSRefresh(currentPlaylist)
			continue
		}
		currentPlaylist = next
	}
}

// sleepHLSRefresh sleeps based on playlist EXT-X-TARGETDURATION to avoid hammering upstream (1–10s).
func sleepHLSRefresh(playlistBody []byte) {
	sec := hlsTargetDurationSeconds(playlistBody)
	if sec <= 0 {
		sec = 3
	}
	half := sec / 2
	if half < 1 {
		half = 1
	}
	if half > 10 {
		half = 10
	}
	time.Sleep(time.Duration(half) * time.Second)
}

func (g *Gateway) fetchAndRewritePlaylist(r *http.Request, client *http.Client, playlistURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, playlistURL, nil)
	if err != nil {
		return nil, err
	}
	if g.ProviderUser != "" || g.ProviderPass != "" {
		req.SetBasicAuth(g.ProviderUser, g.ProviderPass)
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("playlist http status " + strconv.Itoa(resp.StatusCode))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return rewriteHLSPlaylist(body, playlistURL), nil
}

func (g *Gateway) fetchAndWriteSegment(
	w http.ResponseWriter,
	bodyOut io.Writer,
	r *http.Request,
	client *http.Client,
	segURL string,
	headerSent bool,
) (int64, error) {
	if bodyOut == nil {
		bodyOut = w
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, segURL, nil)
	if err != nil {
		return 0, err
	}
	if g.ProviderUser != "" || g.ProviderPass != "" {
		req.SetBasicAuth(g.ProviderUser, g.ProviderPass)
	}
	req.Header.Set("User-Agent", "PlexTuner/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, errors.New("segment http status " + strconv.Itoa(resp.StatusCode))
	}
	if !headerSent {
		w.Header().Set("Content-Type", "video/mp2t")
		w.Header().Del("Content-Length")
		w.WriteHeader(http.StatusOK)
	}
	n, err := io.Copy(bodyOut, resp.Body)
	return n, err
}

func isHLSResponse(resp *http.Response, upstreamURL string) bool {
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ct, "mpegurl") || strings.Contains(ct, "m3u") {
		return true
	}
	return strings.Contains(strings.ToLower(upstreamURL), ".m3u8")
}

func rewriteHLSPlaylist(body []byte, upstreamURL string) []byte {
	base, err := url.Parse(upstreamURL)
	if err != nil || base == nil {
		return body
	}
	var out bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(body))
	first := true
	for sc.Scan() {
		if !first {
			out.WriteByte('\n')
		}
		first = false
		line := sc.Text()
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			out.WriteString(line)
			continue
		}
		if strings.HasPrefix(trim, "//") {
			out.WriteString(base.Scheme + ":" + trim)
			continue
		}
		ref, perr := url.Parse(trim)
		if perr != nil {
			out.WriteString(line)
			continue
		}
		if ref.IsAbs() {
			out.WriteString(trim)
			continue
		}
		out.WriteString(base.ResolveReference(ref).String())
	}
	// Preserve trailing newline if present in input.
	if len(body) > 0 && body[len(body)-1] == '\n' {
		out.WriteByte('\n')
	}
	return out.Bytes()
}

func firstHLSMediaLine(body []byte) string {
	lines := hlsMediaLines(body)
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

func hlsMediaLines(body []byte) []string {
	sc := bufio.NewScanner(bytes.NewReader(body))
	var out []string
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// hlsTargetDurationSeconds parses #EXT-X-TARGETDURATION from playlist body; returns 0 if missing/invalid.
func hlsTargetDurationSeconds(body []byte) int {
	sc := bufio.NewScanner(bytes.NewReader(body))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "#EXT-X-TARGETDURATION:") {
			v := strings.TrimSpace(strings.TrimPrefix(line, "#EXT-X-TARGETDURATION:"))
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				return n
			}
			return 0
		}
	}
	return 0
}
