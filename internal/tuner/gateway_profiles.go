package tuner

import (
	"context"
	"encoding/json"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

const (
	profileDefault    = "default"
	profilePlexSafe   = "plexsafe"
	profileAACCFR     = "aaccfr"
	profileVideoOnly  = "videoonlyfast"
	profileLowBitrate = "lowbitrate"
	profileDashFast   = "dashfast"
	profilePMSXcode   = "pmsxcode"
)

type NamedStreamProfile struct {
	BaseProfile string `json:"base_profile"`
	Transcode   *bool  `json:"transcode,omitempty"`
	OutputMux   string `json:"output_mux,omitempty"`
	Description string `json:"description,omitempty"`
}

type resolvedStreamProfile struct {
	Name           string
	BaseProfile    string
	ForceTranscode bool
	OutputMux      string
	Known          bool
}

// compactProfileKey strips non-alphanumeric characters for HDHR-style labels
// (e.g. "Internet-1080" → "internet1080") so aliases match SiliconDust spellings.
func compactProfileKey(lower string) string {
	var b strings.Builder
	b.Grow(len(lower))
	for _, r := range lower {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func builtinProfileName(v string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(v))
	switch lower {
	case "", "default":
		return profileDefault, true
	case "plexsafe", "plex-safe", "safe":
		return profilePlexSafe, true
	case "aaccfr", "aac-cfr", "aac":
		return profileAACCFR, true
	case "videoonlyfast", "video-only-fast", "videoonly", "video":
		return profileVideoOnly, true
	case "lowbitrate", "low-bitrate", "low":
		return profileLowBitrate, true
	case "dashfast", "dash-fast":
		return profileDashFast, true
	case "pmsxcode", "pms-xcode", "pmsforce", "pms-force":
		return profilePMSXcode, true
	case "native", "heavy", "max", "super":
		return profileDefault, true
	case "internet", "internet720", "internet1080", "hd":
		return profileDashFast, true
	case "internet240", "internet360", "internet480":
		return profileAACCFR, true
	case "mobile", "cell", "light":
		return profileLowBitrate, true
	default:
		switch compactProfileKey(lower) {
		case "native", "heavy", "max", "super":
			return profileDefault, true
		case "internet", "internet720", "internet1080", "hd":
			return profileDashFast, true
		case "internet240", "internet360", "internet480":
			return profileAACCFR, true
		case "mobile", "cell", "light":
			return profileLowBitrate, true
		default:
			return "", false
		}
	}
}

func normalizeProfileName(v string) string {
	if builtIn, ok := builtinProfileName(v); ok {
		return builtIn
	}
	return profileDefault
}

func normalizeConfiguredProfileName(v string) string {
	lower := strings.ToLower(strings.TrimSpace(v))
	if lower == "" {
		return ""
	}
	if builtIn, ok := builtinProfileName(lower); ok {
		return builtIn
	}
	return lower
}

func normalizeStreamOutputMuxName(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "mpegts", "ts":
		return streamMuxMPEGTS
	case "fmp4", "mp4", "dash":
		return streamMuxFMP4
	case "hls":
		return streamMuxHLS
	default:
		return ""
	}
}

func defaultProfileFromEnv() string {
	p := strings.TrimSpace(os.Getenv("IPTV_TUNERR_PROFILE"))
	if p != "" {
		return normalizeConfiguredProfileName(p)
	}
	if strings.EqualFold(os.Getenv("IPTV_TUNERR_PLEX_SAFE"), "1") ||
		strings.EqualFold(os.Getenv("IPTV_TUNERR_PLEX_SAFE"), "true") ||
		strings.EqualFold(os.Getenv("IPTV_TUNERR_PLEX_SAFE"), "yes") {
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
		out[k] = normalizeConfiguredProfileName(v)
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

func loadNamedProfilesFile(path string) (map[string]NamedStreamProfile, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw := map[string]NamedStreamProfile{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]NamedStreamProfile, len(raw))
	for key, spec := range raw {
		name := normalizeConfiguredProfileName(key)
		if name == "" {
			continue
		}
		base := normalizeProfileName(spec.BaseProfile)
		if strings.TrimSpace(spec.BaseProfile) == "" {
			base = profileDefault
		}
		out[name] = NamedStreamProfile{
			BaseProfile: base,
			Transcode:   spec.Transcode,
			OutputMux:   normalizeStreamOutputMuxName(spec.OutputMux),
			Description: strings.TrimSpace(spec.Description),
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

func (g *Gateway) resolveProfileSelection(name string) resolvedStreamProfile {
	resolvedName := normalizeConfiguredProfileName(name)
	if resolvedName == "" {
		resolvedName = profileDefault
	}
	if g != nil {
		if spec, ok := g.NamedProfiles[resolvedName]; ok {
			forceTranscode := true
			if spec.Transcode != nil {
				forceTranscode = *spec.Transcode
			}
			return resolvedStreamProfile{
				Name:           resolvedName,
				BaseProfile:    normalizeProfileName(spec.BaseProfile),
				ForceTranscode: forceTranscode,
				OutputMux:      normalizeStreamOutputMuxName(spec.OutputMux),
				Known:          true,
			}
		}
	}
	if builtIn, ok := builtinProfileName(resolvedName); ok {
		return resolvedStreamProfile{
			Name:           builtIn,
			BaseProfile:    builtIn,
			ForceTranscode: builtIn != profileDefault || strings.TrimSpace(name) != "",
			Known:          true,
		}
	}
	return resolvedStreamProfile{
		Name:        resolvedName,
		BaseProfile: profileDefault,
		Known:       false,
	}
}

func (g *Gateway) preferredOutputMuxForProfile(profileName, requestMux string, transcode bool) string {
	requestMux = strings.TrimSpace(strings.ToLower(requestMux))
	if requestMux != "" {
		return normalizeStreamOutputMux(requestMux, transcode)
	}
	if !transcode {
		return streamMuxMPEGTS
	}
	selection := g.resolveProfileSelection(profileName)
	if selection.Known && selection.OutputMux != "" {
		return normalizeStreamOutputMux(selection.OutputMux, transcode)
	}
	return streamMuxMPEGTS
}

func effectiveProfileName(g *Gateway, channel *catalog.LiveChannel, channelID, forcedProfile string) string {
	if strings.TrimSpace(forcedProfile) != "" {
		return normalizeConfiguredProfileName(forcedProfile)
	}
	if g == nil || channel == nil {
		return ""
	}
	return g.profileForChannelMeta(channelID, channel.GuideNumber, channel.TVGID)
}

const (
	streamMuxMPEGTS = "mpegts"
	streamMuxFMP4   = "fmp4"
	streamMuxHLS    = "hls"
)

// buildFFmpegMPEGTSCodecArgs targets MPEG-TS output (default gateway / Plex HDHR).
func buildFFmpegMPEGTSCodecArgs(transcode bool, profile string) []string {
	return buildFFmpegStreamOutputArgs(transcode, profile, streamMuxMPEGTS)
}

// buildFFmpegStreamOutputArgs builds ffmpeg output args for MPEG-TS or fragmented MP4 (LP-010/011).
func buildFFmpegStreamOutputArgs(transcode bool, profile string, outputMux string) []string {
	if outputMux == "" {
		outputMux = streamMuxMPEGTS
	}
	codecArgs := buildFFmpegStreamCodecArgs(transcode, profile, outputMux)
	if outputMux == streamMuxFMP4 {
		codecArgs = append(codecArgs,
			"-flush_packets", "1",
			"-max_interleave_delta", "0",
			"-movflags", "frag_keyframe+empty_moov+default_base_moof",
			"-f", "mp4",
			"pipe:1",
		)
		return codecArgs
	}
	codecArgs = append(codecArgs,
		"-flush_packets", "1",
		"-max_interleave_delta", "0",
		"-muxdelay", "0",
		"-muxpreload", "0",
		"-mpegts_flags", mpegTSFlagsWithOptionalInitialDiscontinuity(),
		"-f", "mpegts",
		"pipe:1",
	)
	return codecArgs
}

func buildFFmpegStreamCodecArgs(transcode bool, profile string, outputMux string) []string {
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
		if outputMux == streamMuxMPEGTS {
			// Help Plex's live parser lock onto a clean TS timeline/header faster.
			codecArgs = append(codecArgs,
				"-muxrate", "3000000",
				"-pcr_period", "20",
				"-pat_period", "0.05",
			)
		}
	}
	return codecArgs
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

// canonicalizeFFmpegInputURL resolves the input host in Go and rewrites the URL
// to a numeric host for ffmpeg. This avoids resolver differences where Go can
// resolve a host (for example a k8s short service hostname) but the bundled
// ffmpeg binary cannot.
// Set IPTV_TUNERR_FFMPEG_NO_DNS_RESOLVE=1 to disable and keep original hostname.
func canonicalizeFFmpegInputURL(ctx context.Context, raw string, disableResolve bool) (rewritten string, fromHost string, toHost string) {
	if disableResolve {
		return raw, "", ""
	}
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
	ip := pickPreferredResolvedIP(ips)
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
