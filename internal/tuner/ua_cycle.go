package tuner

import "strings"

// uaCycleCandidates returns the ordered list of User-Agent values Tunerr tries automatically
// when a Cloudflare response is detected. Ordered by likelihood of bypassing CF Bot Management
// for IPTV streaming providers — media players first (most commonly allowlisted), browser UAs last.
//
// detectedLavfUA is the auto-detected "Lavf/X.Y.Z" from the installed ffmpeg binary.
// It is placed first because if "ffplay -i url" works directly, this UA is exactly what's needed.
func uaCycleCandidates(detectedLavfUA string) []string {
	lavf := strings.TrimSpace(detectedLavfUA)
	if lavf == "" {
		lavf = defaultLavfUA
	}
	candidates := []string{
		lavf,
		"VLC/3.0.21 LibVLC/3.0.21",
		"mpv/0.38.0",
		"Kodi/21.0 (X11; Linux x86_64) App_Bitness/64 Version/21.0-Git:20240205-a9cf89e8fd",
		"Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/115.0",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"curl/8.4.0",
		"IptvTunerr/1.0",
	}
	// Deduplicate: if detectedLavfUA happens to equal defaultLavfUA we'd have a duplicate.
	seen := make(map[string]bool, len(candidates))
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

// isCFLikeStatus returns true if the HTTP status code and/or body preview look like
// a Cloudflare challenge or block — used for deciding when to trigger UA cycling.
func isCFLikeStatus(statusCode int, preview string) bool {
	p := strings.ToLower(strings.TrimSpace(preview))
	bodyHasCF := strings.Contains(p, "checking your browser") ||
		strings.Contains(p, "ray id") ||
		strings.Contains(p, "cf-bypass") ||
		strings.Contains(p, "cloudflare")
	switch statusCode {
	case 403, 503, 520, 521, 524:
		return bodyHasCF || len(p) == 0 // be liberal on these codes
	case 401, 502:
		return bodyHasCF
	}
	return false
}
