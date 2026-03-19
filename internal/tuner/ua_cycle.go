package tuner

import "strings"

// browserHeadersForUA returns a complete HTTP header set that matches the given User-Agent.
// When UA cycling promotes a browser UA to bypass Cloudflare Bot Management, CF also scores
// Accept/Accept-Language/Accept-Encoding/Sec-Ch-Ua headers. A mismatched header profile
// (e.g. Firefox UA with no Accept header) can still trigger a bot score even if UA matches.
// Returns nil for media-player UAs (Lavf/VLC/mpv/Kodi) — those don't need a browser profile.
func browserHeadersForUA(ua string) map[string]string {
	lower := strings.ToLower(ua)
	if strings.Contains(lower, "firefox") {
		return map[string]string{
			"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8",
			"Accept-Language":           "en-US,en;q=0.5",
			"Accept-Encoding":           "gzip, deflate, br",
			"Connection":                "keep-alive",
			"Upgrade-Insecure-Requests": "1",
			"Cache-Control":             "max-age=0",
		}
	}
	if strings.Contains(lower, "chrome") || strings.Contains(lower, "safari") {
		return map[string]string{
			"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7",
			"Accept-Language":           "en-US,en;q=0.9",
			"Accept-Encoding":           "gzip, deflate, br",
			"Connection":                "keep-alive",
			"Upgrade-Insecure-Requests": "1",
			"Sec-Ch-Ua":                 `"Not_A Brand";v="8", "Chromium";v="120", "Google Chrome";v="120"`,
			"Sec-Ch-Ua-Mobile":          "?0",
			"Sec-Ch-Ua-Platform":        `"Linux"`,
			"Cache-Control":             "max-age=0",
		}
	}
	return nil
}

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
