package plex

import (
	"regexp"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

var (
	plexDiagnosticURLPattern          = regexp.MustCompile(`https?://[^\s"'<>]+`)
	plexDiagnosticQuerySecretPattern  = regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|api[_-]?key|apikey|key|secret|x-plex-token|cf_clearance)=([^&\s"'<>]+)`)
	plexDiagnosticHeaderSecretPattern = regexp.MustCompile(`(?im)\b(authorization|cookie|x-plex-token|x-api-key|x-auth-token|x-session-id)\s*:\s*[^\r\n"']+`)
)

func redactPlexDiagnosticText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = plexDiagnosticURLPattern.ReplaceAllStringFunc(text, func(raw string) string {
		return safeurl.RedactURL(raw)
	})
	text = plexDiagnosticQuerySecretPattern.ReplaceAllString(text, `$1=<redacted>`)
	text = plexDiagnosticHeaderSecretPattern.ReplaceAllString(text, `$1: <redacted>`)
	return text
}
