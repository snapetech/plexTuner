package tuner

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

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
	teeBytes := getenvInt64("IPTV_TUNERR_DEBUG_TEE_BYTES", 0)
	teeDir := strings.TrimSpace(os.Getenv("IPTV_TUNERR_DEBUG_TEE_DIR"))
	if teeDir == "" {
		teeDir = "/tmp/iptvtunerr-debug-tee"
	}
	return streamDebugOptions{
		HTTPHeaders: getenvBool("IPTV_TUNERR_DEBUG_HTTP_HEADERS", false),
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

func debugHeaderNameLines(h http.Header) []string {
	if len(h) == 0 {
		return nil
	}
	names := make([]string, 0, len(h))
	for name := range h {
		names = append(names, http.CanonicalHeaderKey(name))
	}
	sort.Strings(names)
	lines := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true
		lines = append(lines, name+": <present>")
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
	w.ResponseWriter.Header().Set("X-Content-Type-Options", "nosniff")
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
	w.ResponseWriter.Header().Set("X-Content-Type-Options", "nosniff")
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
