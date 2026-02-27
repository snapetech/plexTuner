package plex

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"
)

// ProbeOverridesConfig configures a plex-probe-overrides run.
type ProbeOverridesConfig struct {
	LineupJSON             string   // path or URL to lineup.json
	BaseURL                string   // base URL for relative lineup stream URLs
	ReplaceURLPrefixes     []string // "OLD=NEW" prefix replacements
	ChannelIDs             []string // if set, only probe these IDs
	Limit                  int      // max channels to probe (0=all)
	TimeoutSeconds         int      // ffprobe timeout per channel
	BitrateThreshold       int      // flag bitrate above this bps (0=disabled)
	EmitProfileOverrides   string   // write profile overrides JSON here
	EmitTranscodeOverrides string   // write transcode overrides JSON here
	NoTranscodeOverrides   bool     // do not emit transcode=true entries
	SleepBetweenProbes     time.Duration
	FFprobePath            string // path to ffprobe binary (default: "ffprobe")
}

// ProbeChannelResult is the result of probing one channel.
type ProbeChannelResult struct {
	ID             string   `json:"id"`
	Guide          string   `json:"guide"`
	Name           string   `json:"name"`
	URL            string   `json:"url"`
	OK             bool     `json:"ok"`
	Error          string   `json:"error,omitempty"`
	VideoCodec     string   `json:"video_codec,omitempty"`
	VideoProfile   string   `json:"video_profile,omitempty"`
	Width          int      `json:"width,omitempty"`
	Height         int      `json:"height,omitempty"`
	FPS            float64  `json:"fps,omitempty"`
	FieldOrder     string   `json:"field_order,omitempty"`
	H264Level      int      `json:"h264_level,omitempty"`
	AudioCodec     string   `json:"audio_codec,omitempty"`
	AudioProfile   string   `json:"audio_profile,omitempty"`
	BitRate        int      `json:"bit_rate,omitempty"`
	Reasons        []string `json:"reasons,omitempty"`
	SuggestProfile string   `json:"suggest_profile,omitempty"`
}

var videoFriendly = map[string]bool{"h264": true, "avc": true, "mpeg2video": true, "mpeg4": true}
var audioFriendly = map[string]bool{"aac": true, "ac3": true, "eac3": true, "mp3": true, "mp2": true}

// RunProbeOverrides probes lineup channels via ffprobe and writes override files.
// resultFn is called for each channel result as it completes.
func RunProbeOverrides(cfg ProbeOverridesConfig, resultFn func(ProbeChannelResult, int, int)) error {
	if cfg.FFprobePath == "" {
		cfg.FFprobePath = "ffprobe"
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 12
	}

	lineupRaw, err := loadLineupJSON(cfg.LineupJSON)
	if err != nil {
		return fmt.Errorf("load lineup: %w", err)
	}
	rows, err := parseLineupRows(lineupRaw, cfg.BaseURL)
	if err != nil {
		return err
	}

	rewrites := parseRewrites(cfg.ReplaceURLPrefixes)
	if len(rewrites) > 0 {
		for i, r := range rows {
			rows[i].URL = applyURLRewrites(r.URL, rewrites)
		}
	}
	if len(cfg.ChannelIDs) > 0 {
		want := make(map[string]bool, len(cfg.ChannelIDs))
		for _, id := range cfg.ChannelIDs {
			want[id] = true
		}
		filtered := rows[:0]
		for _, r := range rows {
			if want[r.ID] {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}
	if cfg.Limit > 0 && len(rows) > cfg.Limit {
		rows = rows[:cfg.Limit]
	}

	profileOverrides := map[string]string{}
	transcodeOverrides := map[string]bool{}
	var results []ProbeChannelResult
	total := len(rows)

	for idx, row := range rows {
		res := ProbeChannelResult{
			ID:    row.ID,
			Guide: row.Guide,
			Name:  row.Name,
			URL:   row.URL,
		}
		data, err := runFFprobe(cfg.FFprobePath, row.URL, cfg.TimeoutSeconds)
		if err != nil {
			res.OK = false
			res.Error = err.Error()
		} else {
			summary, reasons, profile := classifyProbe(data, cfg.BitrateThreshold)
			res.OK = true
			res.VideoCodec = summary.VideoCodec
			res.VideoProfile = summary.VideoProfile
			res.Width = summary.Width
			res.Height = summary.Height
			res.FPS = summary.FPS
			res.FieldOrder = summary.FieldOrder
			res.H264Level = summary.H264Level
			res.AudioCodec = summary.AudioCodec
			res.AudioProfile = summary.AudioProfile
			res.BitRate = summary.BitRate
			res.Reasons = reasons
			res.SuggestProfile = profile
			if profile != "" {
				profileOverrides[row.ID] = profile
				if !cfg.NoTranscodeOverrides {
					transcodeOverrides[row.ID] = true
				}
			}
		}
		results = append(results, res)
		if resultFn != nil {
			resultFn(res, idx+1, total)
		}
		if cfg.SleepBetweenProbes > 0 && idx < total-1 {
			time.Sleep(cfg.SleepBetweenProbes)
		}
	}

	if cfg.EmitProfileOverrides != "" {
		if err := writeJSONFile(cfg.EmitProfileOverrides, profileOverrides); err != nil {
			return fmt.Errorf("write profile overrides: %w", err)
		}
	}
	if cfg.EmitTranscodeOverrides != "" {
		if err := writeJSONFile(cfg.EmitTranscodeOverrides, transcodeOverrides); err != nil {
			return fmt.Errorf("write transcode overrides: %w", err)
		}
	}

	errs := 0
	for _, r := range results {
		if !r.OK {
			errs++
		}
	}
	if errs > 0 {
		return fmt.Errorf("probe completed: %d errors out of %d channels", errs, total)
	}
	return nil
}

// lineupRow holds a parsed lineup entry.
type lineupRow struct {
	ID    string
	Name  string
	Guide string
	URL   string
}

func loadLineupJSON(source string) (any, error) {
	var body []byte
	var err error
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		resp, err := http.Get(source) //nolint:noctx
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
	} else {
		body, err = os.ReadFile(source)
		if err != nil {
			return nil, err
		}
	}
	var v any
	return v, json.Unmarshal(body, &v)
}

func parseLineupRows(raw any, baseURL string) ([]lineupRow, error) {
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("expected lineup JSON array")
	}
	var rows []lineupRow
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		u := strings.TrimSpace(stringAny(m, "URL", "url"))
		if u == "" {
			continue
		}
		if baseURL != "" && !strings.HasPrefix(u, "http") {
			u = strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(u, "/")
		}
		guide := strings.TrimSpace(stringAny(m, "GuideNumber", "guideNumber"))
		name := strings.TrimSpace(stringAny(m, "GuideName", "guideName", "Name"))
		id := channelIDFromURL(u)
		rows = append(rows, lineupRow{ID: id, Name: name, Guide: guide, URL: u})
	}
	return rows, nil
}

func channelIDFromURL(u string) string {
	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}
	p := parsed.Path
	if idx := strings.LastIndex(p, "/stream/"); idx >= 0 {
		return p[idx+len("/stream/"):]
	}
	if idx := strings.LastIndex(p, "/auto/"); idx >= 0 {
		rest := p[idx+len("/auto/"):]
		rest = strings.TrimPrefix(rest, "v")
		return rest
	}
	return path.Base(p)
}

func parseRewrites(raw []string) [][2]string {
	var out [][2]string
	for _, s := range raw {
		idx := strings.Index(s, "=")
		if idx < 0 {
			continue
		}
		old := strings.TrimSpace(s[:idx])
		new := strings.TrimSpace(s[idx+1:])
		if old != "" && new != "" {
			out = append(out, [2]string{old, new})
		}
	}
	return out
}

func applyURLRewrites(u string, rewrites [][2]string) string {
	for _, rw := range rewrites {
		if strings.HasPrefix(u, rw[0]) {
			return rw[1] + u[len(rw[0]):]
		}
	}
	return u
}

// ffprobeStreams is a minimal model for ffprobe JSON output.
type ffprobeOutput struct {
	Streams []map[string]any `json:"streams"`
	Format  map[string]any   `json:"format"`
}

type streamSummary struct {
	VideoCodec   string
	VideoProfile string
	Width        int
	Height       int
	FPS          float64
	FieldOrder   string
	H264Level    int
	AudioCodec   string
	AudioProfile string
	BitRate      int
}

func runFFprobe(ffprobePath, streamURL string, timeoutSeconds int) (ffprobeOutput, error) {
	args := []string{
		"-v", "error",
		"-rw_timeout", "15000000",
		"-read_intervals", "%+4",
		"-show_streams",
		"-show_format",
		"-of", "json",
		streamURL,
	}
	cmd := exec.Command(ffprobePath, args...)
	out, err := runWithTimeout(cmd, time.Duration(timeoutSeconds)*time.Second)
	if err != nil {
		return ffprobeOutput{}, err
	}
	var result ffprobeOutput
	if err := json.Unmarshal(out, &result); err != nil {
		return ffprobeOutput{}, fmt.Errorf("parse ffprobe output: %w", err)
	}
	return result, nil
}

func runWithTimeout(cmd *exec.Cmd, timeout time.Duration) ([]byte, error) {
	done := make(chan struct{})
	var out []byte
	var err error
	go func() {
		out, err = cmd.Output()
		close(done)
	}()
	select {
	case <-done:
		return out, err
	case <-time.After(timeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		return nil, fmt.Errorf("ffprobe timed out after %v", timeout)
	}
}

func classifyProbe(data ffprobeOutput, bitrateThreshold int) (streamSummary, []string, string) {
	var video, audio map[string]any
	for _, s := range data.Streams {
		ct, _ := s["codec_type"].(string)
		if ct == "video" && video == nil {
			video = s
		}
		if ct == "audio" && audio == nil {
			audio = s
		}
	}
	if video == nil {
		video = map[string]any{}
	}
	if audio == nil {
		audio = map[string]any{}
	}
	if data.Format == nil {
		data.Format = map[string]any{}
	}

	vcodec := strings.ToLower(stringAny(video, "codec_name"))
	acodec := strings.ToLower(stringAny(audio, "codec_name"))
	aprofile := stringAny(audio, "profile")
	vprofile := stringAny(video, "profile")
	fieldOrder := strings.ToLower(stringAny(video, "field_order"))
	fps := fpsFromRatio(stringAny(video, "avg_frame_rate", "r_frame_rate"))
	width := intAny(video, "width")
	height := intAny(video, "height")
	level := intAny(video, "level")
	bFrames := intAny(video, "has_b_frames")
	bitrate := intAny(data.Format, "bit_rate")
	if bitrate == 0 {
		bitrate = intAny(video, "bit_rate")
	}
	if bitrate == 0 {
		bitrate = intAny(audio, "bit_rate")
	}

	var reasons []string
	if vcodec != "" && !videoFriendly[vcodec] {
		reasons = append(reasons, "video_codec="+vcodec)
	}
	if acodec != "" && !audioFriendly[acodec] {
		reasons = append(reasons, "audio_codec="+acodec)
	}
	if acodec == "aac" && aprofile != "" && strings.ToLower(aprofile) != "lc" && strings.ToLower(aprofile) != "aac-lc" {
		reasons = append(reasons, "aac_profile="+aprofile)
	}
	if fieldOrder != "" && fieldOrder != "progressive" && fieldOrder != "unknown" {
		reasons = append(reasons, "field_order="+fieldOrder)
	}
	if fps > 30.5 {
		reasons = append(reasons, fmt.Sprintf("fps=%.2f", fps))
	}
	if bitrateThreshold > 0 && bitrate > bitrateThreshold {
		reasons = append(reasons, fmt.Sprintf("bitrate=%d", bitrate))
	}
	if vcodec == "h264" && level > 41 {
		reasons = append(reasons, fmt.Sprintf("h264_level=%d", level))
	}
	if vcodec == "h264" && bFrames > 2 {
		reasons = append(reasons, fmt.Sprintf("bframes=%d", bFrames))
	}

	severe := false
	for _, r := range reasons {
		if strings.HasPrefix(r, "field_order=") || strings.HasPrefix(r, "fps=") ||
			strings.HasPrefix(r, "bitrate=") || strings.HasPrefix(r, "aac_profile=") {
			severe = true
			break
		}
	}
	profile := ""
	if severe {
		profile = "aaccfr"
	} else if len(reasons) > 0 {
		profile = "plexsafe"
	}

	summary := streamSummary{
		VideoCodec:   vcodec,
		VideoProfile: vprofile,
		Width:        width,
		Height:       height,
		FPS:          roundFloat(fps, 3),
		FieldOrder:   fieldOrder,
		H264Level:    level,
		AudioCodec:   acodec,
		AudioProfile: aprofile,
		BitRate:      bitrate,
	}
	return summary, reasons, profile
}

func fpsFromRatio(v string) float64 {
	parts := strings.SplitN(v, "/", 2)
	if len(parts) != 2 {
		return 0
	}
	num, err1 := strconv.ParseFloat(parts[0], 64)
	den, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil || den == 0 {
		return 0
	}
	return num / den
}

func roundFloat(f float64, decimals int) float64 {
	p := 1.0
	for i := 0; i < decimals; i++ {
		p *= 10
	}
	return float64(int(f*p+0.5)) / p
}

func stringAny(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func intAny(m map[string]any, keys ...string) int {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch val := v.(type) {
			case float64:
				return int(val)
			case int:
				return val
			case string:
				if n, err := strconv.Atoi(val); err == nil {
					return n
				}
			}
		}
	}
	return 0
}

func writeJSONFile(p string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0644)
}
