// Package plexlabelproxy is a Plex Media Server reverse proxy that rewrites
// Live TV provider labels so multiple DVR/EPG tabs render with distinct names.
//
// Plex stamps every Live TV MediaProvider's friendlyName with the PMS server's
// own friendly name (often the pod hostname, e.g. "plexKube"), making source
// tabs indistinguishable. This package proxies PMS responses and substitutes
// per-DVR labels sourced from /livetv/dvrs lineupTitle.
//
// Two scopes of rewrite:
//   - /media/providers — per-MediaProvider attrs (works on TV/native clients)
//   - /identity, /, /:/prefs, provider-scoped paths — top-level MediaContainer
//     friendlyName ("identity spoofing", opt-in via SpoofIdentity). Required for
//     Plex Web because its source-tab UI ignores per-provider friendlyName and
//     uses the server-level friendlyName instead.
//
// Identity spoofing is best-effort: the chosen label is derived from the
// request's path (provider-scoped) or Referer header. When no scope can be
// determined, the upstream value is left as-is. Spoofing /identity/root carries
// risk for Plex Web sync/auth that depends on machineIdentifier+friendlyName
// pairing; machineIdentifier is never rewritten.
package plexlabelproxy

import (
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"strings"
)

// liveProviderRE matches "tv.plex.providers.epg.xmltv:NNN" identifiers and
// captures NNN. It is the canonical Plex Live TV XMLTV provider identifier.
var liveProviderRE = regexp.MustCompile(`^tv\.plex\.providers\.epg\.xmltv:(\d+)$`)

// liveProviderPathRE matches a request path scoped to a single Live TV
// provider, e.g. "/tv.plex.providers.epg.xmltv:135/grid". Capture group 1 is
// the numeric DVR id.
var liveProviderPathRE = regexp.MustCompile(`^/tv\.plex\.providers\.epg\.xmltv:(\d+)(?:/|$)`)

// liveProviderInTextRE finds a Live TV provider identifier anywhere in a string,
// used to scrape Plex Web SPA fragments that embed the provider in a hash route.
var liveProviderInTextRE = regexp.MustCompile(`tv\.plex\.providers\.epg\.xmltv:(\d+)`)

// LiveProviderIdentFromPath returns the full Live TV provider identifier
// (e.g. "tv.plex.providers.epg.xmltv:135") if the path is provider-scoped,
// or "" when the path does not name a provider.
func LiveProviderIdentFromPath(path string) string {
	m := liveProviderPathRE.FindStringSubmatch(path)
	if len(m) < 2 {
		return ""
	}
	return "tv.plex.providers.epg.xmltv:" + m[1]
}

// rewriteMediaProvidersXML walks the XML token stream of a /media/providers
// response and substitutes per-MediaProvider labels for any LiveTV provider
// that has a mapping in labels. Non-LiveTV providers are passed through.
//
// Returns the original bytes unchanged if no MediaProvider matched (avoids
// re-serialising and disturbing whitespace/order on a no-op).
func rewriteMediaProvidersXML(in []byte, labels map[string]string) ([]byte, error) {
	if len(labels) == 0 {
		return in, nil
	}

	out, changed, err := rewriteTokens(in, func(start *xml.StartElement, ctx *rewriteCtx) {
		if start.Name.Local != "MediaProvider" {
			return
		}
		ident := attrValue(start.Attr, "identifier")
		if !liveProviderRE.MatchString(ident) {
			return
		}
		label, ok := labels[ident]
		if !ok || label == "" {
			return
		}
		// Cover the various attrs different Plex client UIs source from.
		setAttr(&start.Attr, "friendlyName", label)
		setAttr(&start.Attr, "sourceTitle", label)
		setAttr(&start.Attr, "title", label)
		// Track scope so child Directory rewrites know which provider they belong to.
		ctx.providerScope = ident
		ctx.providerLabel = label
		ctx.markChanged()
	})
	if err != nil {
		return nil, err
	}
	if !changed {
		return in, nil
	}

	// Second pass: rewrite Directory titles within scoped MediaProvider blocks.
	out2, _, err := rewriteTokens(out, func(start *xml.StartElement, ctx *rewriteCtx) {
		if start.Name.Local == "MediaProvider" {
			ident := attrValue(start.Attr, "identifier")
			if label, ok := labels[ident]; ok && label != "" {
				ctx.providerScope = ident
				ctx.providerLabel = label
			} else {
				ctx.providerScope = ""
				ctx.providerLabel = ""
			}
			return
		}
		if start.Name.Local != "Directory" || ctx.providerScope == "" {
			return
		}
		dID := attrValue(start.Attr, "id")
		dKey := attrValue(start.Attr, "key")
		dTitle := attrValue(start.Attr, "title")
		switch {
		case dID == ctx.providerScope:
			setAttr(&start.Attr, "title", ctx.providerLabel)
			ctx.markChanged()
		case dKey == "/"+ctx.providerScope+"/watchnow" && dTitle == "Guide":
			setAttr(&start.Attr, "title", ctx.providerLabel+" Guide")
			ctx.markChanged()
		}
	})
	if err != nil {
		return nil, err
	}
	return out2, nil
}

// rewriteProviderScopedXML rewrites the root MediaContainer of a response that
// is scoped to one Live TV provider (path like /tv.plex.providers.epg.xmltv:NNN/...).
// Substitutes friendlyName, title1/title2, and per-provider Directory titles.
//
// This addresses Plex Web's behavior of using the top-level MediaContainer
// friendlyName as the source-tab label.
func rewriteProviderScopedXML(in []byte, providerIdent, label string) ([]byte, error) {
	if label == "" || providerIdent == "" {
		return in, nil
	}

	depth := 0
	out, _, err := rewriteTokens(in, func(start *xml.StartElement, ctx *rewriteCtx) {
		if depth == 0 && start.Name.Local == "MediaContainer" {
			// Root MediaContainer: rewrite descriptive titles and friendlyName.
			if v := attrValue(start.Attr, "title1"); v == "" || isGenericTitle(v) {
				setAttr(&start.Attr, "title1", label)
				ctx.markChanged()
			}
			if v := attrValue(start.Attr, "title2"); v == "" || isGenericTitle(v) {
				setAttr(&start.Attr, "title2", label)
				ctx.markChanged()
			}
			if hasAttr(start.Attr, "friendlyName") {
				setAttr(&start.Attr, "friendlyName", label)
				ctx.markChanged()
			}
			if hasAttr(start.Attr, "title") && isGenericTitle(attrValue(start.Attr, "title")) {
				setAttr(&start.Attr, "title", label)
				ctx.markChanged()
			}
		}
		if start.Name.Local == "Directory" {
			dID := attrValue(start.Attr, "id")
			dKey := attrValue(start.Attr, "key")
			dTitle := attrValue(start.Attr, "title")
			switch {
			case dID == providerIdent && (dTitle == "" || isGenericTitle(dTitle)):
				setAttr(&start.Attr, "title", label)
				ctx.markChanged()
			case strings.HasSuffix(dKey, "/watchnow") && dTitle == "Guide":
				setAttr(&start.Attr, "title", label+" Guide")
				ctx.markChanged()
			case dTitle == "Live TV & DVR":
				setAttr(&start.Attr, "title", label)
				ctx.markChanged()
			}
		}
		depth++
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// rewriteRootIdentityXML rewrites the friendlyName attribute on the root
// element of /identity-style responses (/, /identity). machineIdentifier is
// never touched. Used in identity-spoof mode when a per-tab label can be
// inferred from the request scope.
func rewriteRootIdentityXML(in []byte, label string) ([]byte, error) {
	if label == "" {
		return in, nil
	}
	depth := 0
	out, _, err := rewriteTokens(in, func(start *xml.StartElement, ctx *rewriteCtx) {
		if depth == 0 && hasAttr(start.Attr, "friendlyName") {
			setAttr(&start.Attr, "friendlyName", label)
			ctx.markChanged()
		}
		depth++
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// isGenericTitle reports whether title is one of the generic Plex Live TV
// container titles that we are willing to overwrite with a per-DVR label.
// Distinct titles (already set by another mechanism) are preserved.
func isGenericTitle(s string) bool {
	switch strings.TrimSpace(s) {
	case "", "Plex Library", "Live TV & DVR", "Guide":
		return true
	}
	return false
}

// rewriteCtx is mutable state threaded through a token-stream rewrite pass.
type rewriteCtx struct {
	providerScope string
	providerLabel string
	changed       bool
}

func (c *rewriteCtx) markChanged() { c.changed = true }

// rewriteTokens parses in as XML, calls mutate on every StartElement (which may
// modify attrs in place), and re-emits the document. Returns the output bytes
// and whether mutate marked any change. Falls back to returning in unchanged
// when no mutation occurred, to preserve byte-for-byte compatibility on no-ops.
//
// The XML declaration is preserved literally from the source rather than
// re-emitted via encoding/xml — which restricts ProcInst tokens to the very
// first position and disallows them after any element token, even when our
// rewrite loop is purely structural.
func rewriteTokens(in []byte, mutate func(start *xml.StartElement, ctx *rewriteCtx)) ([]byte, bool, error) {
	declHead := []byte{}
	body := in
	trimmed := bytes.TrimLeft(in, " \t\r\n")
	if bytes.HasPrefix(trimmed, []byte("<?xml")) {
		if end := bytes.Index(trimmed, []byte("?>")); end > 0 {
			declHead = append(declHead, trimmed[:end+2]...)
			declHead = append(declHead, '\n')
			body = trimmed[end+2:]
		}
	}

	dec := xml.NewDecoder(bytes.NewReader(body))
	dec.Strict = false

	var buf bytes.Buffer
	buf.Write(declHead)
	enc := xml.NewEncoder(&buf)
	ctx := &rewriteCtx{}

	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, false, err
		}
		// Skip stray ProcInst tokens — the source declaration was already
		// written verbatim above.
		if _, isProc := tok.(xml.ProcInst); isProc {
			continue
		}
		if start, ok := tok.(xml.StartElement); ok {
			mutate(&start, ctx)
			tok = start
		}
		if err := enc.EncodeToken(tok); err != nil {
			return nil, false, err
		}
	}
	if err := enc.Flush(); err != nil {
		return nil, false, err
	}
	return buf.Bytes(), ctx.changed, nil
}

// attrValue returns the value of the attribute with the given local name, or "".
func attrValue(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

// hasAttr reports whether attrs contains an attribute with the given local name.
func hasAttr(attrs []xml.Attr, name string) bool {
	for _, a := range attrs {
		if a.Name.Local == name {
			return true
		}
	}
	return false
}

// setAttr sets the value of the named attribute, adding it if not already
// present. Modifies *attrs in place.
func setAttr(attrs *[]xml.Attr, name, value string) {
	for i := range *attrs {
		if (*attrs)[i].Name.Local == name {
			(*attrs)[i].Value = value
			return
		}
	}
	*attrs = append(*attrs, xml.Attr{Name: xml.Name{Local: name}, Value: value})
}
