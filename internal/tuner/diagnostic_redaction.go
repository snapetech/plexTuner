package tuner

import (
	"regexp"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/safeurl"
)

var (
	diagnosticURLPattern          = regexp.MustCompile(`https?://[^\s"'<>]+`)
	diagnosticQuerySecretPattern  = regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|api[_-]?key|apikey|key|secret|x-plex-token|cf_clearance)=([^&\s"'<>]+)`)
	diagnosticHeaderSecretPattern = regexp.MustCompile(`(?im)\b(authorization|cookie|x-plex-token|x-api-key|x-auth-token|x-session-id)\s*:\s*[^\r\n"']+`)
)

func redactOperatorDiagnosticText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	text = diagnosticURLPattern.ReplaceAllStringFunc(text, func(raw string) string {
		return safeurl.RedactURL(raw)
	})
	text = diagnosticQuerySecretPattern.ReplaceAllString(text, `$1=<redacted>`)
	text = diagnosticHeaderSecretPattern.ReplaceAllString(text, `$1: <redacted>`)
	return text
}
