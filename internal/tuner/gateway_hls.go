package tuner

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// sleepHLSRefresh sleeps based on playlist EXT-X-TARGETDURATION to avoid hammering upstream (1-10s).
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

func (g *Gateway) fetchAndRewritePlaylist(r *http.Request, client *http.Client, playlistURL string) ([]byte, string, error) {
	req, err := g.newUpstreamRequest(r.Context(), r, playlistURL)
	if err != nil {
		return nil, "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", errors.New("playlist http status " + strconv.Itoa(resp.StatusCode))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	effectiveURL := playlistURL
	if resp.Request != nil && resp.Request.URL != nil {
		effectiveURL = resp.Request.URL.String()
	}
	return rewriteHLSPlaylist(body, effectiveURL), effectiveURL, nil
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
	req, err := g.newUpstreamRequest(r.Context(), r, segURL)
	if err != nil {
		return 0, err
	}
	if debugOpts := streamDebugOptionsFromEnv(); debugOpts.HTTPHeaders {
		for _, line := range debugHeaderLines(req.Header) {
			reqID := gatewayReqIDFromContext(r.Context())
			log.Printf("gateway: req=%s segment-fetch > %s", reqID, line)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		if g.FetchCFReject && strings.Contains(err.Error(), "cloudflare-terms-of-service-abuse.com") {
			return 0, errCFBlock
		}
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
