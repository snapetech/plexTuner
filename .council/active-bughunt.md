# IPTVtunerr Active Council Bughunt Candidate Report

This report is not a pass/fail proof. It is a focused queue of suspicious
shapes around the Plex Live TV proxy, tuner HTTP surface, provider IO, process
launch, and filesystem boundaries.

Classification rule: any accepted row must be ledgered, fixed with behavior
coverage, sibling-swept, and promoted into a durable gate before closure.

## Proxy elevation trust boundary
internal/plexlabelproxy/proxy.go:276:		connectionHeaders := connectionHeaderNames(req.Header)
internal/plexlabelproxy/proxy.go:314:		if p.cfg.ElevateAll && strings.TrimSpace(p.cfg.OwnerToken) != "" && p.canElevate(req, originalToken) {
internal/plexlabelproxy/proxy.go:341:func (p *Proxy) canElevate(req *http.Request, originalToken string) bool {
internal/plexlabelproxy/proxy.go:395:		p.auditElevation(r, "blocked_bad_actor", inboundPlexToken(r), "temporary block after repeated bad elevation attempts")
internal/plexlabelproxy/proxy.go:426:			tokenFingerprint(inboundPlexToken(r)),
internal/plexlabelproxy/proxy.go:584:	if !p.canElevate(req, elevationToken) {
internal/plexlabelproxy/proxy.go:788:func inboundPlexToken(req *http.Request) string {
internal/plexlabelproxy/proxy.go:795:	if _, hopByHopToken := connectionHeaderNames(req.Header)["x-plex-token"]; hopByHopToken {
internal/plexlabelproxy/proxy.go:801:func connectionHeaderNames(h http.Header) map[string]struct{} {
internal/plexlabelproxy/proxy.go:924:	if p.blockBypassAllowed(req, inboundPlexToken(req)) {
internal/plexlabelproxy/proxy.go:930:func (p *Proxy) blockBypassAllowed(req *http.Request, token string) bool {
internal/plexlabelproxy/proxy.go:960:	if ip := trustedCloudflareConnectingIP(req); ip != "" {
internal/plexlabelproxy/proxy.go:963:	if ip := closestTrustedForwardedFor(req); ip != "" {
internal/plexlabelproxy/proxy.go:969:func closestTrustedForwardedFor(req *http.Request) string {
internal/plexlabelproxy/proxy.go:983:func trustedCloudflareConnectingIP(req *http.Request) string {
internal/plexlabelproxy/proxy.go:988:	closest := closestTrustedForwardedFor(req)
internal/plexlabelproxy/proxy.go:1409:		tokenFingerprint(inboundPlexToken(req)),

## Tokenless session recovery boundary
internal/plexlabelproxy/proxy_test.go:1574:	if !strings.Contains(logBuf.String(), "outcome=source_mismatch") {
internal/plexlabelproxy/proxy_test.go:1575:		t.Fatalf("expected source_mismatch playback log, got %s", logBuf.String())
internal/plexlabelproxy/proxy.go:149:	sessions  map[string]sessionRecord
internal/plexlabelproxy/proxy.go:188:type sessionRecord struct {
internal/plexlabelproxy/proxy.go:256:		sessions:    make(map[string]sessionRecord),
internal/plexlabelproxy/proxy.go:322:				p.trackSession(req, originalToken)
internal/plexlabelproxy/proxy.go:573:		if token, matched := p.sessionTokenForRequest(req); token != "" {
internal/plexlabelproxy/proxy.go:613:		p.trackSession(req, elevationToken)
internal/plexlabelproxy/proxy.go:617:// trackSession stores request correlation keys → original user token mappings
internal/plexlabelproxy/proxy.go:621:func (p *Proxy) trackSession(req *http.Request, originalToken string) {
internal/plexlabelproxy/proxy.go:622:	keys := sessionCorrelationKeys(req)
internal/plexlabelproxy/proxy.go:638:		p.sessions = make(map[string]sessionRecord)
internal/plexlabelproxy/proxy.go:641:	rec := sessionRecord{userToken: originalToken, sourceKey: source, expiresAt: time.Now().Add(elevatedSessionTTL)}
internal/plexlabelproxy/proxy.go:656:func sessionCorrelationKeys(req *http.Request) []string {
internal/plexlabelproxy/proxy.go:715:func (p *Proxy) sessionTokenForRequest(req *http.Request) (string, string) {
internal/plexlabelproxy/proxy.go:716:	keys := sessionCorrelationKeys(req)
internal/plexlabelproxy/proxy.go:732:		if strings.TrimSpace(rec.userToken) != "" && sessionRecordMatchesSource(rec, req) {
internal/plexlabelproxy/proxy.go:746:func sessionRecordMatchesSource(rec sessionRecord, req *http.Request) bool {
internal/plexlabelproxy/proxy.go:1284:	keys := sessionCorrelationKeys(req)
internal/plexlabelproxy/proxy.go:1306:		if !sessionRecordMatchesSource(rec, req) {
internal/plexlabelproxy/proxy.go:1324:			p.logPlaybackCorrelation(req, "source_mismatch", sourceMismatch, keys)
internal/plexlabelproxy/proxy.go:1355:		p.logPlaybackCorrelation(req, "elevate_progress", matched, sessionCorrelationKeys(req))
internal/plexlabelproxy/proxy.go:1368:	keys := sessionCorrelationKeys(req)
internal/plexlabelproxy/proxy.go:1384:		if !sessionRecordMatchesSource(rec, req) {

## Response rewrite boundary
internal/plexlabelproxy/rewrite.go:66:	out, changed, err := rewriteTokens(in, func(start *xml.StartElement, ctx *rewriteCtx) {
internal/plexlabelproxy/rewrite.go:95:	out2, _, err := rewriteTokens(out, func(start *xml.StartElement, ctx *rewriteCtx) {
internal/plexlabelproxy/rewrite.go:140:	out, _, err := rewriteTokens(in, func(start *xml.StartElement, ctx *rewriteCtx) {
internal/plexlabelproxy/rewrite.go:193:	out, _, err := rewriteTokens(in, func(start *xml.StartElement, ctx *rewriteCtx) {
internal/plexlabelproxy/rewrite.go:226:// rewriteTokens parses in as XML, calls mutate on every StartElement (which may
internal/plexlabelproxy/rewrite.go:235:func rewriteTokens(in []byte, mutate func(start *xml.StartElement, ctx *rewriteCtx)) ([]byte, bool, error) {
internal/plexlabelproxy/proxy.go:269:	rp.ModifyResponse = p.modifyResponse
internal/plexlabelproxy/proxy.go:482:	if scope == scopeNone && !p.shouldRewriteTunerEntitlement(resp) {
internal/plexlabelproxy/proxy.go:500:		if p.shouldRewriteTunerEntitlement(resp) {
internal/plexlabelproxy/proxy.go:501:			return restoreBody(resp, RewriteTunerEntitlementFlags(body), encoding)
internal/plexlabelproxy/proxy.go:512:	if p.shouldRewriteTunerEntitlement(resp) {
internal/plexlabelproxy/proxy.go:513:		rewritten = RewriteTunerEntitlementFlags(rewritten)
internal/plexlabelproxy/proxy.go:1480:func (p *Proxy) shouldRewriteTunerEntitlement(resp *http.Response) bool {
internal/plexlabelproxy/proxy.go:1487:	if !pathCanCarryTunerEntitlement(resp.Request.URL.EscapedPath()) {
internal/plexlabelproxy/proxy.go:1494:func pathCanCarryTunerEntitlement(path string) bool {
internal/plexlabelproxy/entitlement.go:193:// RewriteTunerEntitlementFlags rewrites the small XML/JSON hints Plex Web uses to
internal/plexlabelproxy/entitlement.go:197:func RewriteTunerEntitlementFlags(body []byte) []byte {

## Operator/debug HTTP boundary
internal/tuner/server_status_reports.go:175:		if !operatorUIAllowed(w, r) {
internal/tuner/server_status_reports.go:195:		if !operatorUIAllowed(w, r) {
internal/tuner/server_status_reports.go:220:		if !operatorUIAllowed(w, r) {
internal/tuner/server_status_reports.go:239:		if !operatorUIAllowed(w, r) {
internal/tuner/server_status_reports.go:384:		if !operatorUIAllowed(w, r) {
internal/tuner/server_status_reports.go:432:		if !operatorUIAllowed(w, r) {
internal/tuner/server_status_reports.go:449:func (s *Server) serveRecentStreamAttempts() http.Handler {
internal/tuner/server_status_reports.go:455:		if !operatorUIAllowed(w, r) {
internal/tuner/server_status_reports.go:473:func (s *Server) serveSharedRelayReport() http.Handler {
internal/tuner/server_status_reports.go:479:		if !operatorUIAllowed(w, r) {
internal/tuner/server_status_reports.go:498:func (s *Server) serveOperatorActionStatus() http.Handler {
internal/tuner/server_status_reports.go:504:		if !operatorUIAllowed(w, r) {
internal/tuner/server_status_reports.go:522:				"endpoint":  "/debug/active-streams.json",
internal/tuner/server_status_reports.go:526:				"endpoint":     "/ops/actions/stream-stop",
internal/tuner/server_status_reports.go:536:				"endpoint":         "/ops/actions/shared-relay-replay",
internal/tuner/server_status_reports.go:547:				"endpoint":         "/ops/actions/virtual-channel-live-stall",
internal/tuner/server_status_reports.go:571:				"endpoint":     "/ops/actions/mux-seg-decode",
internal/tuner/server_status_reports.go:578:				"endpoint":     "/ops/actions/evidence-intake-start",
internal/tuner/server_status_reports.go:585:				"endpoint":     "/ops/actions/channel-diff-run",
internal/tuner/server_status_reports.go:592:				"endpoint":     "/ops/actions/stream-compare-run",
internal/tuner/server_test.go:86:	remote := httptest.NewRequest(http.MethodGet, "/debug/hls-mux-demo.html", nil)
internal/tuner/server_test.go:94:	local := httptest.NewRequest(http.MethodGet, "/debug/hls-mux-demo.html", nil)
internal/tuner/server_test.go:233:		{name: "guide_refresh_action", req: httptest.NewRequest(http.MethodGet, "/ops/actions/guide-refresh", nil), h: s.serveGuideRefreshAction(), allow: http.MethodPost},
internal/tuner/server_test.go:235:		{name: "runtime_snapshot", req: httptest.NewRequest(http.MethodPost, "/debug/runtime.json", nil), h: s.serveRuntimeSnapshot(), allow: http.MethodGet},
internal/tuner/server_test.go:236:		{name: "event_hooks", req: httptest.NewRequest(http.MethodPost, "/debug/event-hooks.json", nil), h: s.serveEventHooksReport(), allow: http.MethodGet},
internal/tuner/server_test.go:237:		{name: "active_streams", req: httptest.NewRequest(http.MethodPost, "/debug/active-streams.json", nil), h: s.serveActiveStreamsReport(), allow: http.MethodGet},
internal/tuner/server_test.go:571:	s.serveOperatorActionStatus().ServeHTTP(w, req)
internal/tuner/server_test.go:579:	req = httptest.NewRequest(http.MethodGet, "/ops/actions/guide-refresh", nil)
internal/tuner/server_test.go:649:		{name: "recent_stream_attempts", req: httptest.NewRequest(http.MethodPost, "/debug/stream-attempts.json", nil), h: s.serveRecentStreamAttempts()},
internal/tuner/server_test.go:650:		{name: "shared_relay_report", req: httptest.NewRequest(http.MethodPost, "/debug/shared-relays.json", nil), h: s.serveSharedRelayReport()},
internal/tuner/server_test.go:651:		{name: "runtime_snapshot", req: httptest.NewRequest(http.MethodPost, "/debug/runtime.json", nil), h: s.serveRuntimeSnapshot()},
internal/tuner/server_test.go:652:		{name: "event_hooks_report", req: httptest.NewRequest(http.MethodPost, "/debug/event-hooks.json", nil), h: s.serveEventHooksReport()},
internal/tuner/server_test.go:653:		{name: "active_streams_report", req: httptest.NewRequest(http.MethodPost, "/debug/active-streams.json", nil), h: s.serveActiveStreamsReport()},
internal/tuner/server_test.go:2904:	req = httptest.NewRequest(http.MethodPost, "/ops/actions/evidence-intake-start", strings.NewReader(`{"case_id":"smoke-case"}`))
internal/tuner/server_test.go:2935:	req = httptest.NewRequest(http.MethodPost, "/ops/actions/evidence-intake-start", strings.NewReader(`{"case_id":"../../escape/me"}`))
internal/tuner/server_test.go:3074:	req := httptest.NewRequest(http.MethodPost, "/ops/actions/channel-diff-run", strings.NewReader(`{}`))
internal/tuner/server_test.go:3088:	req = httptest.NewRequest(http.MethodPost, "/ops/actions/channel-diff-run", strings.NewReader(`{"good_channel_id":"bad-1","bad_channel_id":"good-1"}`))
internal/tuner/server_test.go:3099:	req = httptest.NewRequest(http.MethodPost, "/ops/actions/stream-compare-run", strings.NewReader(`{}`))
internal/tuner/server_test.go:3110:	req = httptest.NewRequest(http.MethodPost, "/ops/actions/stream-compare-run", strings.NewReader(`{"channel_id":"good-1"}`))
internal/tuner/server_test.go:3337:			handler: s.serveOperatorGuidePreviewJSON(),
internal/tuner/server_test.go:3342:				r := httptest.NewRequest(http.MethodGet, "/ops/actions/status.json", nil)
internal/tuner/server_test.go:3346:			handler: s.serveOperatorActionStatus(),
internal/tuner/server_test.go:3351:				r := httptest.NewRequest(http.MethodPost, "/ops/actions/guide-refresh", nil)
internal/tuner/server_test.go:3734:	req := httptest.NewRequest(http.MethodGet, "/debug/stream-attempts.json?limit=1", nil)
internal/tuner/server_test.go:3737:	s.serveRecentStreamAttempts().ServeHTTP(w, req)
internal/tuner/server_test.go:3767:	req := httptest.NewRequest(http.MethodGet, "/debug/stream-attempts.json?limit=999999", nil)
internal/tuner/server_test.go:3770:	s.serveRecentStreamAttempts().ServeHTTP(w, req)
internal/tuner/server_test.go:3852:	s.serveOperatorGuidePreviewJSON().ServeHTTP(w, req)
internal/tuner/server_test.go:3875:	s.serveOperatorGuidePreviewJSON().ServeHTTP(w, req)
internal/tuner/server_test.go:3890:	(&Server{}).serveOperatorGuidePreviewJSON().ServeHTTP(w, req)
internal/tuner/server_test.go:3920:	s.serveOperatorGuidePreviewJSON().ServeHTTP(w, req)
internal/tuner/server_test.go:3955:			h: s.serveOperatorUI(),
internal/tuner/server_test.go:3964:			h: s.serveOperatorGuidePreviewPage(),
internal/tuner/server_test.go:3973:			h:     s.serveOperatorUI(),
internal/tuner/server_test.go:3983:			h:     s.serveOperatorGuidePreviewPage(),
internal/tuner/server_test.go:4027:	s.serveOperatorUI().ServeHTTP(uiW, uiReq)
internal/tuner/server_test.go:4042:	s.serveOperatorGuidePreviewPage().ServeHTTP(guideW, guideReq)
internal/tuner/server_test.go:4257:	req := httptest.NewRequest(http.MethodGet, "/debug/runtime.json", nil)
internal/tuner/server_test.go:4293:		{name: "recent_stream_attempts", handler: (&Server{}).serveRecentStreamAttempts(), code: http.StatusServiceUnavailable, want: "gateway unavailable"},
internal/tuner/server_test.go:4392:				r := httptest.NewRequest(http.MethodPost, "/ops/actions/mux-seg-decode", strings.NewReader(`{`))
internal/tuner/server_test.go:4460:	s.serveOperatorGuidePreviewJSON().ServeHTTP(w, req)
internal/tuner/server_test.go:4477:	req := httptest.NewRequest(http.MethodGet, "/ops/actions/status.json", nil)
internal/tuner/server_test.go:4480:	s.serveOperatorActionStatus().ServeHTTP(w, req)
internal/tuner/server_test.go:4529:	if body.SharedRelayReplayUpdate.Endpoint != "/ops/actions/shared-relay-replay" {
internal/tuner/server_test.go:4544:	if body.VirtualChannelLiveStallUpdate.Endpoint != "/ops/actions/virtual-channel-live-stall" {
internal/tuner/server_test.go:4564:	req := httptest.NewRequest(http.MethodGet, "/ops/actions/status.json", nil)
internal/tuner/server_test.go:4567:	s.serveOperatorActionStatus().ServeHTTP(w, req)
internal/tuner/server_test.go:4588:	req := httptest.NewRequest(http.MethodPost, "/ops/actions/guide-refresh", nil)
internal/tuner/server_test.go:4616:	req := httptest.NewRequest(http.MethodPost, "/ops/actions/stream-attempts-clear", nil)
internal/tuner/server_test.go:4654:	req := httptest.NewRequest(http.MethodPost, "/ops/actions/provider-profile-reset", nil)
internal/tuner/server_test.go:4685:	req := httptest.NewRequest(http.MethodPost, "/ops/actions/shared-relay-replay", strings.NewReader(`{"shared_relay_replay_bytes":65536}`))
internal/tuner/server_test.go:4723:	req := httptest.NewRequest(http.MethodPost, "/ops/actions/shared-relay-replay", strings.NewReader(`{"shared_relay_replay_bytes":-1}`))
internal/tuner/server_test.go:4743:	req := httptest.NewRequest(http.MethodPost, "/ops/actions/virtual-channel-live-stall", strings.NewReader(`{"virtual_channel_recovery_live_stall_sec":9}`))
internal/tuner/server_test.go:4781:	req := httptest.NewRequest(http.MethodPost, "/ops/actions/virtual-channel-live-stall", strings.NewReader(`{"virtual_channel_recovery_live_stall_sec":-1}`))
internal/tuner/server_test.go:4809:	req := httptest.NewRequest(http.MethodPost, "/ops/actions/autopilot-reset", nil)
internal/tuner/server_test.go:4846:	req := httptest.NewRequest(http.MethodPost, "/ops/actions/ghost-visible-stop", nil)
internal/tuner/server_test.go:4943:	req := httptest.NewRequest(http.MethodPost, "/ops/actions/ghost-hidden-recover?mode=dry-run", nil)
internal/tuner/server_test.go:6324:	req := httptest.NewRequest(http.MethodGet, "/debug/event-hooks.json", nil)
internal/tuner/server_test.go:6332:	req = httptest.NewRequest(http.MethodGet, "/debug/event-hooks.json", nil)
internal/tuner/server_test.go:6353:	req := httptest.NewRequest(http.MethodGet, "/debug/runtime.json", nil)
internal/tuner/server_test.go:6361:	req = httptest.NewRequest(http.MethodGet, "/debug/runtime.json", nil)
internal/tuner/server_test.go:6390:	req := httptest.NewRequest(http.MethodGet, "/debug/active-streams.json", nil)
internal/tuner/server_test.go:6412:	req := httptest.NewRequest(http.MethodGet, "/debug/active-streams.json", nil)
internal/tuner/server_test.go:6437:	req := httptest.NewRequest(http.MethodGet, "/debug/shared-relays.json", nil)
internal/tuner/server_test.go:6440:	srv.serveSharedRelayReport().ServeHTTP(rr, req)
internal/tuner/server_test.go:6467:	req := httptest.NewRequest(http.MethodPost, "/ops/actions/stream-stop", bytes.NewBufferString(`{"request_id":"r000001"}`))
internal/tuner/operator_ui.go:18:// operatorUIAllowed enforces IPTV_TUNERR_UI_DISABLED and localhost-only access (unless IPTV_TUNERR_UI_ALLOW_LAN=1).
internal/tuner/operator_ui.go:19:func operatorUIAllowed(w http.ResponseWriter, r *http.Request) bool {
internal/tuner/operator_ui.go:154:func (s *Server) serveOperatorGuidePreviewPage() http.Handler {
internal/tuner/operator_ui.go:165:		if !operatorUIAllowed(w, r) {
internal/tuner/operator_ui.go:199:func (s *Server) serveOperatorGuidePreviewJSON() http.Handler {
internal/tuner/operator_ui.go:205:		if !operatorUIAllowed(w, r) {
internal/tuner/operator_ui.go:227:func (s *Server) serveOperatorUI() http.Handler {
internal/tuner/operator_ui.go:238:		if !operatorUIAllowed(w, r) {
internal/tuner/server_diagnostics_recordings.go:493:		if !operatorUIAllowed(w, r) {
internal/tuner/server_diagnostics_recordings.go:524:			if !operatorUIAllowed(w, r) {
internal/tuner/server_diagnostics_recordings.go:534:			if !operatorUIAllowed(w, r) {
internal/tuner/server_diagnostics_recordings.go:590:		if !operatorUIAllowed(w, r) {
internal/tuner/server_diagnostics_recordings.go:630:		if !operatorUIAllowed(w, r) {
internal/tuner/server_diagnostics_recordings.go:666:		if !operatorUIAllowed(w, r) {
internal/tuner/server_diagnostics_recordings.go:686:		if !operatorUIAllowed(w, r) {
internal/tuner/server_programming.go:381:			if !operatorUIAllowed(w, r) {
internal/tuner/server_programming.go:385:			if !operatorUIAllowed(w, r) {
internal/tuner/server_programming.go:447:		if !operatorUIAllowed(w, r) {
internal/tuner/server_programming.go:614:			if !operatorUIAllowed(w, r) {
internal/tuner/server_programming.go:634:			if !operatorUIAllowed(w, r) {
internal/tuner/server_programming.go:685:			if !operatorUIAllowed(w, r) {
internal/tuner/server_programming.go:703:			if !operatorUIAllowed(w, r) {
internal/tuner/server_programming.go:756:			if !operatorUIAllowed(w, r) {
internal/tuner/server_programming.go:760:			if !operatorUIAllowed(w, r) {
internal/tuner/server_programming.go:820:			if !operatorUIAllowed(w, r) {
internal/tuner/server_programming.go:838:			if !operatorUIAllowed(w, r) {
internal/tuner/server_programming.go:880:			if !operatorUIAllowed(w, r) {
internal/tuner/server_programming.go:907:			if !operatorUIAllowed(w, r) {
internal/tuner/server_programming.go:967:		if !operatorUIAllowed(w, r) {
internal/tuner/server_programming.go:1012:		if !operatorUIAllowed(w, r) {
internal/tuner/server_programming.go:1104:			if !operatorUIAllowed(w, r) {
internal/tuner/server_programming.go:1120:			if !operatorUIAllowed(w, r) {
internal/tuner/server_programming.go:1177:		if !operatorUIAllowed(w, r) {
internal/tuner/server_virtual_channels.go:22:			if !operatorUIAllowed(w, r) {
internal/tuner/server_virtual_channels.go:39:			if !operatorUIAllowed(w, r) {
internal/tuner/server_virtual_channels.go:80:		if !operatorUIAllowed(w, r) {
internal/tuner/server_virtual_channels.go:109:			if !operatorUIAllowed(w, r) {
internal/tuner/server_virtual_channels.go:131:			if !operatorUIAllowed(w, r) {
internal/tuner/server_virtual_channels.go:174:			if !operatorUIAllowed(w, r) {
internal/tuner/server_virtual_channels.go:179:			if !operatorUIAllowed(w, r) {
internal/tuner/server_virtual_channels.go:216:		if !operatorUIAllowed(w, r) {
internal/tuner/server_virtual_channels.go:246:		if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:52:		if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:73:				"/ops/actions/guide-refresh",
internal/tuner/server_operator_workflows.go:77:				"/debug/runtime.json",
internal/tuner/server_operator_workflows.go:95:		if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:119:				"/ops/actions/stream-attempts-clear",
internal/tuner/server_operator_workflows.go:120:				"/ops/actions/provider-profile-reset",
internal/tuner/server_operator_workflows.go:121:				"/ops/actions/autopilot-reset",
internal/tuner/server_operator_workflows.go:122:				"/debug/stream-attempts.json",
internal/tuner/server_operator_workflows.go:125:				"/debug/runtime.json",
internal/tuner/server_operator_workflows.go:143:		if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:170:				"/debug/stream-attempts.json",
internal/tuner/server_operator_workflows.go:171:				"/ops/actions/channel-diff-run",
internal/tuner/server_operator_workflows.go:172:				"/ops/actions/stream-compare-run",
internal/tuner/server_operator_workflows.go:173:				"/ops/actions/evidence-intake-start",
internal/tuner/server_operator_workflows.go:384:		if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:469:		if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:527:				"/ops/actions/ghost-visible-stop",
internal/tuner/server_operator_workflows.go:528:				"/ops/actions/ghost-hidden-recover?mode=dry-run",
internal/tuner/server_operator_workflows.go:529:				"/ops/actions/ghost-hidden-recover?mode=restart",
internal/tuner/server_operator_workflows.go:530:				"/ops/actions/autopilot-reset",
internal/tuner/server_operator_workflows.go:534:				"/debug/runtime.json",
internal/tuner/server_operator_workflows.go:570:			if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:609:			if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:737:		if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:758:		if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:776:		if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:819:		if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:860:		if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:904:		if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:948:		if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:969:		if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:996:		if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:1032:		if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:1074:		if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:1109:		if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:1143:		if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:1185:		if !operatorUIAllowed(w, r) {
internal/tuner/server_operator_workflows.go:1212:		if !operatorUIAllowed(w, r) {
internal/tuner/server.go:155:// RuntimeSnapshot is returned by /debug/runtime.json for the dedicated web UI and operator tooling.
internal/tuner/server.go:2138:	mux.Handle("/ui/guide-preview.json", s.serveOperatorGuidePreviewJSON())
internal/tuner/server.go:2147:	mux.Handle("/ui/guide/", s.serveOperatorGuidePreviewPage())
internal/tuner/server.go:2156:	mux.Handle("/ui/", s.serveOperatorUI())
internal/tuner/server.go:2167:	mux.Handle("/debug/active-streams.json", s.serveActiveStreamsReport())
internal/tuner/server.go:2168:	mux.Handle("/debug/shared-relays.json", s.serveSharedRelayReport())
internal/tuner/server.go:2169:	mux.Handle("/debug/stream-attempts.json", s.serveRecentStreamAttempts())
internal/tuner/server.go:2170:	mux.Handle("/debug/event-hooks.json", s.serveEventHooksReport())
internal/tuner/server.go:2171:	mux.Handle("/debug/runtime.json", s.serveRuntimeSnapshot())
internal/tuner/server.go:2172:	mux.Handle("/debug/hls-mux-demo.html", s.serveHlsMuxWebDemo())
internal/tuner/server.go:2178:	mux.Handle("/ops/actions/mux-seg-decode", s.serveMuxSegDecodeAction())
internal/tuner/server.go:2179:	mux.Handle("/ops/actions/status.json", s.serveOperatorActionStatus())
internal/tuner/server.go:2185:	mux.Handle("/ops/actions/guide-refresh", s.serveGuideRefreshAction())
internal/tuner/server.go:2186:	mux.Handle("/ops/actions/stream-attempts-clear", s.serveStreamAttemptsClearAction())
internal/tuner/server.go:2187:	mux.Handle("/ops/actions/stream-stop", s.serveStreamStopAction())
internal/tuner/server.go:2188:	mux.Handle("/ops/actions/provider-profile-reset", s.serveProviderProfileResetAction())
internal/tuner/server.go:2189:	mux.Handle("/ops/actions/shared-relay-replay", s.serveSharedRelayReplayUpdateAction())
internal/tuner/server.go:2190:	mux.Handle("/ops/actions/virtual-channel-live-stall", s.serveVirtualChannelLiveStallUpdateAction())
internal/tuner/server.go:2191:	mux.Handle("/ops/actions/autopilot-reset", s.serveAutopilotResetAction())
internal/tuner/server.go:2192:	mux.Handle("/ops/actions/ghost-visible-stop", s.serveGhostVisibleStopAction())
internal/tuner/server.go:2193:	mux.Handle("/ops/actions/ghost-hidden-recover", s.serveGhostHiddenRecoverAction())
internal/tuner/server.go:2194:	mux.Handle("/ops/actions/evidence-intake-start", s.serveEvidenceIntakeStartAction())
internal/tuner/server.go:2195:	mux.Handle("/ops/actions/channel-diff-run", s.serveChannelDiffRunAction())
internal/tuner/server.go:2196:	mux.Handle("/ops/actions/stream-compare-run", s.serveStreamCompareRunAction())
internal/tuner/server.go:2277:		if !operatorUIAllowed(w, r) {
internal/tuner/server.go:2420:			if !operatorUIAllowed(w, r) {
internal/tuner/server.go:2436:			if !operatorUIAllowed(w, r) {

## Provider process and file boundary
scripts/check-remediation-baseline.sh:100:require_pattern "sanitizeFileToken" "internal/tuner/gateway_debug.go" "debug evidence file tokens are sanitized"
scripts/check-remediation-baseline.sh:135:require_pattern "TestGateway_ffmpegInputHeaderBlock_stillIncludesCredentialHeaders" "internal/tuner/gateway_test.go" "ffmpeg credential header forwarding behavior test exists"
scripts/check-council-negative-space.sh:51:assert_validator_present "evidence-file-token" "internal/tuner/gateway_debug.go" "sanitizeFileToken"
scripts/check-council-negative-space.sh:52:assert_baseline_anchor "evidence-file-token" "sanitizeFileToken"
scripts/run-council-active-bughunt.sh:57:  'exec\.Command|ffmpeg|sanitizeFileToken|filepath\.(Join|Clean)|os\.(ReadFile|WriteFile|Create|MkdirAll)' \
scripts/scan-bug-council-candidates.sh:39:  'exec\.Command|ffmpeg|url\.Parse|http\.NewRequest|os\.(ReadFile|WriteFile|Create|Open|MkdirAll)|filepath\.(Join|Clean)|sanitizeFileToken|SetBasicAuth' \
scripts/build-linux-package-assets.sh:55:Recommends: ffmpeg
internal/store/migrations.go:161:-- Stream profiles (ffmpeg, proxy, redirect, streamlink, vlc, yt-dlp, custom).
internal/store/migrations.go:165:    type        TEXT NOT NULL DEFAULT 'ffmpeg',
internal/store/store.go:26:	path = filepath.Clean(strings.TrimSpace(path))
internal/store/store.go:31:		if err := os.MkdirAll(dir, 0o755); err != nil {
scripts/ci-smoke.sh:447:    "description": "binary smoke ffmpeg fMP4 shared-session profile"
scripts/ci-smoke.sh:809:fake_packager_ffmpeg="$TMP_DIR/fake-packager-ffmpeg.sh"
scripts/ci-smoke.sh:810:cat >"$fake_packager_ffmpeg" <<'SH'
scripts/ci-smoke.sh:834:chmod +x "$fake_packager_ffmpeg"
scripts/ci-smoke.sh:841:IPTV_TUNERR_FFMPEG_PATH="$fake_packager_ffmpeg" \
scripts/ci-smoke.sh:853:grep -qi '^X-IptvTunerr-Shared-Upstream: ffmpeg_hls_packager' "$packager_second_headers" || fail "packaged hls second consumer missing shared upstream header"
scripts/ci-smoke.sh:887:fake_shared_ffmpeg="$TMP_DIR/fake-shared-ffmpeg.sh"
scripts/ci-smoke.sh:888:cat >"$fake_shared_ffmpeg" <<'SH'
scripts/ci-smoke.sh:893:printf 'ffmpeg'
scripts/ci-smoke.sh:895:chmod +x "$fake_shared_ffmpeg"
scripts/ci-smoke.sh:897:port_ffmpeg_shared="$(pick_port)"
scripts/ci-smoke.sh:902:IPTV_TUNERR_FFMPEG_PATH="$fake_shared_ffmpeg" \
scripts/ci-smoke.sh:904:"$BIN" serve -catalog "$TMP_DIR/catalog-remux.json" -addr ":$port_ffmpeg_shared" -base-url "http://127.0.0.1:$port_ffmpeg_shared" \
scripts/ci-smoke.sh:905:  >"$TMP_DIR/serve-ffmpeg-shared-$port_ffmpeg_shared.log" 2>&1 &
scripts/ci-smoke.sh:907:wait_http_code "http://127.0.0.1:$port_ffmpeg_shared/discover.json" "200" || fail "ffmpeg shared discover.json not ready"
scripts/ci-smoke.sh:908:curl -sS "http://127.0.0.1:$port_ffmpeg_shared/stream/remux1" -o "$TMP_DIR/ffmpeg-shared-first.out" &
scripts/ci-smoke.sh:909:ffmpeg_shared_first_pid=$!
scripts/ci-smoke.sh:911:ffmpeg_shared_headers="$TMP_DIR/ffmpeg-shared-second.headers"
scripts/ci-smoke.sh:912:curl -sS -D "$ffmpeg_shared_headers" "http://127.0.0.1:$port_ffmpeg_shared/stream/remux1" -o "$TMP_DIR/ffmpeg-shared-second.out" &
scripts/ci-smoke.sh:913:ffmpeg_shared_second_pid=$!
scripts/ci-smoke.sh:915:grep -q '"count": 1' <(curl -sS "http://127.0.0.1:$port_ffmpeg_shared/debug/shared-relays.json") || fail "ffmpeg shared relay report missing active relay"
scripts/ci-smoke.sh:916:grep -q '"shared_upstream": "hls_ffmpeg"' <(curl -sS "http://127.0.0.1:$port_ffmpeg_shared/debug/shared-relays.json") || fail "ffmpeg shared relay report missing upstream label"
scripts/ci-smoke.sh:917:wait "$ffmpeg_shared_first_pid"
scripts/ci-smoke.sh:918:wait "$ffmpeg_shared_second_pid"
scripts/ci-smoke.sh:919:grep -qi '^X-IptvTunerr-Shared-Upstream: hls_ffmpeg' "$ffmpeg_shared_headers" || fail "ffmpeg shared second consumer missing shared upstream header"
scripts/ci-smoke.sh:920:[[ -s "$TMP_DIR/ffmpeg-shared-first.out" ]] || fail "ffmpeg shared first consumer got no bytes"
scripts/ci-smoke.sh:921:[[ -s "$TMP_DIR/ffmpeg-shared-second.out" ]] || fail "ffmpeg shared second consumer got no bytes"
scripts/ci-smoke.sh:922:assert_file_prefix "$TMP_DIR/ffmpeg-shared-second.out" "shared-"
scripts/ci-smoke.sh:924:port_ffmpeg_fmp4="$(pick_port)"
scripts/ci-smoke.sh:929:IPTV_TUNERR_FFMPEG_PATH="$fake_shared_ffmpeg" \
scripts/ci-smoke.sh:932:"$BIN" serve -catalog "$TMP_DIR/catalog-remux.json" -addr ":$port_ffmpeg_fmp4" -base-url "http://127.0.0.1:$port_ffmpeg_fmp4" \
scripts/ci-smoke.sh:933:  >"$TMP_DIR/serve-ffmpeg-fmp4-$port_ffmpeg_fmp4.log" 2>&1 &
scripts/ci-smoke.sh:935:wait_http_code "http://127.0.0.1:$port_ffmpeg_fmp4/discover.json" "200" || fail "ffmpeg fmp4 shared discover.json not ready"
scripts/ci-smoke.sh:936:curl -sS "http://127.0.0.1:$port_ffmpeg_fmp4/stream/remux1?profile=shared-fmp4" -o "$TMP_DIR/ffmpeg-fmp4-first.out" &
scripts/ci-smoke.sh:937:ffmpeg_fmp4_first_pid=$!
scripts/ci-smoke.sh:939:ffmpeg_fmp4_headers="$TMP_DIR/ffmpeg-fmp4-second.headers"
scripts/ci-smoke.sh:940:curl -sS -D "$ffmpeg_fmp4_headers" "http://127.0.0.1:$port_ffmpeg_fmp4/stream/remux1?profile=shared-fmp4" -o "$TMP_DIR/ffmpeg-fmp4-second.out" &
scripts/ci-smoke.sh:941:ffmpeg_fmp4_second_pid=$!
scripts/ci-smoke.sh:943:grep -q '"content_type": "video/mp4"' <(curl -sS "http://127.0.0.1:$port_ffmpeg_fmp4/debug/shared-relays.json") || fail "ffmpeg fmp4 shared relay report missing mp4 content type"
scripts/ci-smoke.sh:944:wait "$ffmpeg_fmp4_first_pid"
scripts/ci-smoke.sh:945:wait "$ffmpeg_fmp4_second_pid"
scripts/ci-smoke.sh:946:grep -qi '^X-IptvTunerr-Shared-Upstream: hls_ffmpeg' "$ffmpeg_fmp4_headers" || fail "ffmpeg fmp4 second consumer missing shared upstream header"
scripts/ci-smoke.sh:947:grep -qi '^Content-Type: video/mp4' "$ffmpeg_fmp4_headers" || fail "ffmpeg fmp4 second consumer missing video/mp4 content type"
scripts/ci-smoke.sh:948:[[ -s "$TMP_DIR/ffmpeg-fmp4-first.out" ]] || fail "ffmpeg fmp4 first consumer got no bytes"
scripts/ci-smoke.sh:949:[[ -s "$TMP_DIR/ffmpeg-fmp4-second.out" ]] || fail "ffmpeg fmp4 second consumer got no bytes"
scripts/ci-smoke.sh:950:assert_file_prefix "$TMP_DIR/ffmpeg-fmp4-second.out" "shared-"
scripts/ci-smoke.sh:996:fake_ffmpeg="$TMP_DIR/fake-ffmpeg.sh"
scripts/ci-smoke.sh:997:cat >"$fake_ffmpeg" <<'SH'
scripts/ci-smoke.sh:1002:chmod +x "$fake_ffmpeg"
scripts/ci-smoke.sh:1009:IPTV_TUNERR_FFMPEG_PATH="$fake_ffmpeg" \
scripts/channel-diff-report.py:123:    if "ffmpeg_hls_failed" in bad_outcomes or "ffmpeg" in bad_outcomes:
scripts/channel-diff-report.py:124:        findings.append("Bad channel still traversed an ffmpeg failure path before relay; remux avoidance may still need a tighter classifier for this channel class.")
cmd/iptv-tunerr/cmd_debug_bundle.go:67:	if err := os.MkdirAll(dir, 0o750); err != nil {
cmd/iptv-tunerr/cmd_debug_bundle.go:85:			dest := filepath.Join(dir, ep.name)
cmd/iptv-tunerr/cmd_debug_bundle.go:103:			cfLearnedPath = filepath.Join(filepath.Dir(jar), "cf-learned.json")
cmd/iptv-tunerr/cmd_debug_bundle.go:107:		dest := filepath.Join(dir, "cf-learned.json")
cmd/iptv-tunerr/cmd_debug_bundle.go:125:		dest := filepath.Join(dir, "cookie-meta.json")
cmd/iptv-tunerr/cmd_debug_bundle.go:137:	envDest := filepath.Join(dir, "env.json")
cmd/iptv-tunerr/cmd_debug_bundle.go:146:	infoDest := filepath.Join(dir, "bundle-info.json")
cmd/iptv-tunerr/cmd_debug_bundle.go:154:		_ = os.WriteFile(infoDest, data, 0o600)
cmd/iptv-tunerr/cmd_debug_bundle.go:194:	return os.WriteFile(destPath, data, 0o600)
cmd/iptv-tunerr/cmd_debug_bundle.go:198:	data, err := os.ReadFile(src)
cmd/iptv-tunerr/cmd_debug_bundle.go:202:	return os.WriteFile(dst, data, 0o600)
cmd/iptv-tunerr/cmd_debug_bundle.go:208:	data, err := os.ReadFile(jarPath)
cmd/iptv-tunerr/cmd_debug_bundle.go:257:	return os.WriteFile(destPath, out, 0o600)
cmd/iptv-tunerr/cmd_debug_bundle.go:289:	return os.WriteFile(destPath, data, 0o600)
cmd/iptv-tunerr/cmd_debug_bundle.go:330:	f, err := os.Create(destPath)
cmd/iptv-tunerr/cmd_debug_bundle.go:362:		data, err := os.ReadFile(path)
cmd/iptv-tunerr/cmd_catalog_test.go:115:	cacheFile := filepath.Join(dir, "provider-epg.xml")
cmd/iptv-tunerr/cmd_catalog_test.go:117:	if err := os.WriteFile(cacheFile, []byte(body), 0644); err != nil {
cmd/iptv-tunerr/free_sources.go:105:		return filepath.Join(d, "free-sources")
cmd/iptv-tunerr/free_sources.go:118:		cacheFile := filepath.Join(cacheDir, urlCacheKey(rawURL))
cmd/iptv-tunerr/free_sources.go:120:			if data, err := os.ReadFile(cacheFile); err == nil {
cmd/iptv-tunerr/free_sources.go:147:		if mkErr := os.MkdirAll(cacheDir, 0o750); mkErr == nil {
cmd/iptv-tunerr/free_sources.go:148:			cacheFile := filepath.Join(cacheDir, urlCacheKey(rawURL))
cmd/iptv-tunerr/free_sources.go:149:			_ = os.WriteFile(cacheFile, data, 0o600)
scripts/live-race-harness.sh:25:SYN_LOG="$OUT_DIR/synth-ffmpeg.log"
scripts/live-race-harness.sh:26:REPLAY_LOG="$OUT_DIR/replay-ffmpeg.log"
scripts/live-race-harness.sh:59:HARNESS_FFMPEG_BIN="${HARNESS_FFMPEG_BIN:-${IPTV_TUNERR_FFMPEG_PATH:-ffmpeg}}"
scripts/live-race-harness.sh:94:resolve_ffmpeg_bin() {
scripts/live-race-harness.sh:514:    echo "  synth ffmpeg log: $SYN_LOG"
scripts/live-race-harness.sh:515:    echo "  replay ffmpeg log: $REPLAY_LOG"
scripts/live-race-harness.sh:545:  FFMPEG_BIN="$(resolve_ffmpeg_bin)"
scripts/live-race-harness.sh:546:  [[ -n "$FFMPEG_BIN" ]] || die "ffmpeg binary not found: $HARNESS_FFMPEG_BIN"
scripts/live-race-harness.sh:547:  log "Using ffmpeg binary: $FFMPEG_BIN"
cmd/iptv-tunerr/main.go:5://     /stream/{id}) backed by M3U/Xtream provider with optional ffmpeg transcode.
cmd/iptv-tunerr/cmd_ops.go:73:	manifestPath := filepath.Join(outDir, "manifest.json")
cmd/iptv-tunerr/cmd_ops.go:75:		"source_catalog": filepath.Clean(path),
cmd/iptv-tunerr/cmd_ops.go:79:	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
cmd/iptv-tunerr/cmd_ops.go:99:	moviesPath := filepath.Clean(filepath.Join(mp, "Movies"))
cmd/iptv-tunerr/cmd_ops.go:100:	tvPath := filepath.Clean(filepath.Join(mp, "TV"))
cmd/iptv-tunerr/cmd_migrate_db.go:64:			`INSERT INTO stream_profiles (name, type, config_json, is_default) VALUES (?, 'ffmpeg', ?, 1)`,
internal/plexlabelproxy/proxy.go:1064:	data, err := os.ReadFile(path)
internal/plexlabelproxy/proxy.go:1110:	tmp, err := os.CreateTemp(dir, ".abuse-state-*.json.tmp")
internal/plexlabelproxy/proxy.go:1174:	if err := os.MkdirAll(dir, 0o700); err != nil {
cmd/iptv-tunerr/cmd_cf_status.go:80:		learned = filepath.Join(filepath.Dir(jar), "cf-learned.json")
cmd/iptv-tunerr/cmd_cf_status.go:86:		data, err := os.ReadFile(jar)
cmd/iptv-tunerr/cmd_cf_status.go:108:		data, err := os.ReadFile(learned)
scripts/live-race-harness-report.py:72:    ffmpeg_modes: Counter = field(default_factory=Counter)
scripts/live-race-harness-report.py:81:    ffmpeg_mode_re = re.compile(r'(ffmpeg-(?:transcode|remux))')
scripts/live-race-harness-report.py:174:                if m := self.ffmpeg_mode_re.search(msg):
scripts/live-race-harness-report.py:176:                        self.req(req_id).ffmpeg_modes[m.group(1)] += 1
scripts/live-race-harness-report.py:377:                    "ffmpeg_modes": dict(r.ffmpeg_modes),
scripts/live-race-harness-report.py:425:            hypotheses.append("Startup gate timeouts observed: upstream/ffmpeg readiness latency remains a primary suspect.")
scripts/live-race-harness-report.py:462:            f"- First ffmpeg bytes startup (ms): count={int(fb['count'])} min={fb['min']:.1f} avg={fb['avg']:.1f} max={fb['max']:.1f}"
cmd/iptv-tunerr/cmd_lineup_harvest.go:142:		if err := os.WriteFile(p, data, 0o600); err != nil {
cmd/iptv-tunerr/cmd_guide_reports.go:78:		if err := os.WriteFile(p, data, 0o600); err != nil {
cmd/iptv-tunerr/cmd_guide_reports.go:88:		if err := os.WriteFile(p, data, 0o600); err != nil {
cmd/iptv-tunerr/cmd_guide_reports.go:105:		if err := os.WriteFile(p, out, 0o600); err != nil {
cmd/iptv-tunerr/cmd_guide_reports.go:126:		if err := os.WriteFile(p, aliasOut, 0o600); err != nil {
cmd/iptv-tunerr/cmd_guide_reports.go:134:		if err := os.WriteFile(p, out, 0o600); err != nil {
cmd/iptv-tunerr/cmd_oracle_ops.go:147:		if err := os.WriteFile(p, data, 0o600); err != nil {
cmd/iptv-tunerr/cmd_catchup_publish.go:117:		if err := os.WriteFile(p, out, 0o600); err != nil {
cmd/iptv-tunerr/cmd_live_tv_bundle.go:685:	data, err := os.ReadFile(strings.TrimSpace(path))
cmd/iptv-tunerr/cmd_live_tv_bundle.go:701:	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
cmd/iptv-tunerr/cmd_cookie_import.go:164:		data, err := os.ReadFile(*harFileFlag)
cmd/iptv-tunerr/cmd_cookie_import.go:215:	data, err := os.ReadFile(path)
cmd/iptv-tunerr/cmd_cookie_import.go:224:	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
cmd/iptv-tunerr/cmd_cookie_import.go:232:	if err := os.WriteFile(tmp, data, 0o600); err != nil {
cmd/iptv-tunerr/main_integration_test.go:61:	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess")
cmd/iptv-tunerr/main_integration_test.go:80:	catalogPath := filepath.Join(t.TempDir(), "catalog.json")
cmd/iptv-tunerr/main_integration_test.go:84:	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess")
cmd/iptv-tunerr/main_integration_test.go:112:	catalogPath := filepath.Join(t.TempDir(), "catalog.json")
cmd/iptv-tunerr/main_integration_test.go:113:	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess")
cmd/iptv-tunerr/main_integration_test.go:134:	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess")
cmd/iptv-tunerr/main_integration_test.go:149:	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess")
cmd/iptv-tunerr/main_integration_test.go:164:	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess")
cmd/iptv-tunerr/main_integration_test.go:179:	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelperProcess")
cmd/iptv-tunerr/cmd_catalog.go:1067:	data, cacheErr := os.ReadFile(cachePath)
cmd/iptv-tunerr/cmd_runtime_test.go:212:	path := filepath.Join(t.TempDir(), "guide.db")
cmd/iptv-tunerr/cmd_runtime_test.go:232:	path := filepath.Join(t.TempDir(), "catalog.json")
cmd/iptv-tunerr/cmd_runtime_test.go:257:	path := filepath.Join(t.TempDir(), "catalog.json")
cmd/iptv-tunerr/cmd_plex_ops.go:359:		if err := os.WriteFile(p, data, 0o600); err != nil {
cmd/iptv-tunerr/cmd_runtime_integration_test.go:55:	path := filepath.Join(t.TempDir(), "catalog.json")
cmd/iptv-tunerr/cmd_runtime_integration_test.go:84:	cmd := exec.Command(os.Args[0], "-test.run=TestRuntimeCommandHelperProcess")
cmd/iptv-tunerr/cmd_runtime_integration_test.go:134:	cmd := exec.Command(os.Args[0], "-test.run=TestRuntimeCommandHelperProcess")
cmd/iptv-tunerr/cmd_runtime_integration_test.go:160:	catalogPath := filepath.Join(t.TempDir(), "catalog.json")
cmd/iptv-tunerr/cmd_runtime_integration_test.go:164:	cmd := exec.Command(os.Args[0], "-test.run=TestRuntimeCommandHelperProcess")
cmd/iptv-tunerr/cmd_runtime_integration_test.go:189:	cmd := exec.Command(os.Args[0], "-test.run=TestRuntimeCommandHelperProcess")
cmd/iptv-tunerr/cmd_runtime_integration_test.go:193:		"IPTV_TUNERR_HELPER_CATALOG="+filepath.Join(t.TempDir(), "missing-catalog.json"),
internal/plexlabelproxy/proxy_test.go:963:	target := filepath.Join(dir, "target.json")
internal/plexlabelproxy/proxy_test.go:964:	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
internal/plexlabelproxy/proxy_test.go:967:	link := filepath.Join(dir, "blocks.json")
internal/plexlabelproxy/proxy_test.go:987:	if got, err := os.ReadFile(target); err != nil {
cmd/iptv-tunerr/cmd_vod_integration_test.go:35:	catalogPath := filepath.Join(t.TempDir(), "catalog.json")
cmd/iptv-tunerr/cmd_vod_integration_test.go:53:	cmd := exec.Command(os.Args[0], "-test.run=TestVODCommandHelperProcess")
cmd/iptv-tunerr/cmd_vod_integration_test.go:97:	cmd := exec.Command(os.Args[0], "-test.run=TestVODCommandHelperProcess")
cmd/iptv-tunerr/cmd_vod_integration_test.go:101:		"IPTV_TUNERR_HELPER_CATALOG="+filepath.Join(t.TempDir(), "missing-catalog.json"),
cmd/iptv-tunerr/cmd_vod_integration_test.go:114:	cmd := exec.Command(os.Args[0], "-test.run=TestVODCommandHelperProcess")
cmd/iptv-tunerr/cmd_vod_integration_test.go:118:		"IPTV_TUNERR_HELPER_CATALOG="+filepath.Join(t.TempDir(), "missing-catalog.json"),
cmd/iptv-tunerr/cmd_vod_integration_test.go:119:		"IPTV_TUNERR_HELPER_MOUNT="+filepath.Join(t.TempDir(), "mnt"),
cmd/iptv-tunerr/cmd_debug_bundle_test.go:42:	dest := filepath.Join(t.TempDir(), "env.json")
cmd/iptv-tunerr/cmd_debug_bundle_test.go:46:	data, err := os.ReadFile(dest)
cmd/iptv-tunerr/cmd_debug_bundle_test.go:76:	dest := filepath.Join(t.TempDir(), "out.json")
cmd/iptv-tunerr/cmd_reports.go:175:		if err := os.WriteFile(p, data, 0o600); err != nil {
cmd/iptv-tunerr/cmd_reports.go:195:		if err := os.WriteFile(p, data, 0o600); err != nil {
cmd/iptv-tunerr/cmd_reports.go:209:		if err := os.WriteFile(p, data, 0o600); err != nil {
cmd/iptv-tunerr/cmd_reports.go:290:		if err := os.WriteFile(p, out, 0o600); err != nil {
cmd/iptv-tunerr/cmd_reports.go:453:		if err := os.WriteFile(p, data, 0o600); err != nil {
cmd/iptv-tunerr/free_sources_test.go:28:	want := filepath.Join("/var/cache/iptvtunerr", "free-sources")
cmd/iptv-tunerr/free_sources_test.go:148:	blocklistPath := filepath.Join(cacheDir, urlCacheKey(iptvOrgBlocklistURL))
cmd/iptv-tunerr/free_sources_test.go:149:	channelsPath := filepath.Join(cacheDir, urlCacheKey(iptvOrgChannelsURL))
cmd/iptv-tunerr/free_sources_test.go:150:	if err := os.WriteFile(blocklistPath, []byte(`[{"channel":"blocked.us","reason":"legal"}]`), 0o600); err != nil {
cmd/iptv-tunerr/free_sources_test.go:153:	if err := os.WriteFile(channelsPath, []byte(`[{"id":"adult.us","name":"Adult","categories":["xxx"],"is_nsfw":true},{"id":"closed.us","name":"Closed","closed":"2025-01-01"}]`), 0o600); err != nil {
cmd/iptv-tunerr/free_sources_test.go:223:	blocklistPath := filepath.Join(cacheDir, urlCacheKey(iptvOrgBlocklistURL))
cmd/iptv-tunerr/free_sources_test.go:224:	channelsPath := filepath.Join(cacheDir, urlCacheKey(iptvOrgChannelsURL))
cmd/iptv-tunerr/free_sources_test.go:225:	if err := os.WriteFile(blocklistPath, []byte(`[{"channel":"blocked.us","reason":"legal"}]`), 0o600); err != nil {
cmd/iptv-tunerr/free_sources_test.go:228:	if err := os.WriteFile(channelsPath, []byte(`[{"id":"closed.us","name":"Closed","closed":"2025-01-01"}]`), 0o600); err != nil {
internal/cache/path.go:12:	return filepath.Join(cacheDir, "vod", safe+".mp4")
internal/cache/path.go:18:	return filepath.Join(cacheDir, "vod", safe+".partial")
internal/httpclient/cookiejar.go:32:	data, err := os.ReadFile(path)
internal/catalog/catalog.go:168:	dir := filepath.Dir(filepath.Clean(path))
internal/catalog/catalog.go:169:	tmp, err := os.CreateTemp(dir, ".catalog-*.json.tmp")
internal/catalog/catalog.go:196:	data, err := os.ReadFile(path)
internal/catalog/catalog_test.go:11:	path := filepath.Join(dir, "catalog.json")
internal/catalog/catalog_test.go:46:	path := filepath.Join(dir, "catalog.json")
internal/catalog/catalog_test.go:68:	path := filepath.Join(dir, "catalog.json")
internal/catalog/catalog_test.go:105:	path := filepath.Join(dir, "catalog.json")
internal/catalog/catalog_test.go:130:	err := c.Load(filepath.Join(t.TempDir(), "nonexistent.json"))
internal/catalog/catalog_test.go:138:	path := filepath.Join(dir, "bad.json")
internal/catalog/catalog_test.go:139:	if err := os.WriteFile(path, []byte("{not valid json"), 0600); err != nil {
internal/supervisor/supervisor_test.go:11:	p := filepath.Join(dir, "multi.json")
internal/supervisor/supervisor_test.go:12:	if err := os.WriteFile(p, []byte(`{
internal/supervisor/supervisor_test.go:50:	p := filepath.Join(dir, "dup.json")
internal/supervisor/supervisor_test.go:51:	if err := os.WriteFile(p, []byte(`{"instances":[{"name":"x","args":["run"]},{"name":"x","args":["run"]}]}`), 0o644); err != nil {
internal/supervisor/supervisor_test.go:121:	p := filepath.Join(dir, "cfg.json")
internal/supervisor/supervisor_test.go:122:	if err := os.WriteFile(p, []byte(`{
internal/supervisor/supervisor_test.go:143:	path := filepath.Join(dir, "envfile.env")
internal/supervisor/supervisor_test.go:144:	if err := os.WriteFile(path, []byte("export IPTV_TUNERR_PROVIDER_USER=\"demo user\"\nIPTV_TUNERR_PROVIDER_PASS='demo-pass'\n"), 0o600); err != nil {
internal/plex/inspect.go:114:		LibraryDBPath: filepath.Join(root, "Plug-in Support", "Databases", "com.plexapp.plugins.library.db"),
internal/plex/inspect.go:215:	paths, err := filepath.Glob(filepath.Join(dbDir, "tv.plex.providers.epg.xmltv-*.db"))
internal/plex/dvr_test.go:27:	dbDir := filepath.Join(dir, "Plug-in Support", "Databases")
internal/plex/dvr_test.go:28:	if err := os.MkdirAll(dbDir, 0755); err != nil {
internal/plex/dvr_test.go:31:	dbPath := filepath.Join(dbDir, "com.plexapp.plugins.library.db")
internal/plex/dvr_test.go:62:	dbDir := filepath.Join(dir, "Plug-in Support", "Databases")
internal/plex/dvr_test.go:63:	if err := os.MkdirAll(dbDir, 0755); err != nil {
internal/plex/dvr_test.go:66:	dbPath := filepath.Join(dbDir, "com.plexapp.plugins.library.db")
internal/catalog/vod_split.go:209:	if err := os.MkdirAll(outDir, 0o755); err != nil {
internal/catalog/vod_split.go:217:		p := filepath.Join(outDir, lane.Name+".json")
internal/supervisor/supervisor.go:249:	cmd := exec.CommandContext(ctx, exe, inst.Args...)
internal/supervisor/supervisor.go:322:		if err := os.MkdirAll(dir, 0o755); err != nil {
internal/plex/cutover_test.go:11:	path := filepath.Join(dir, "cutover.tsv")
internal/plex/cutover_test.go:15:	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
internal/config/config.go:329:		pattern := filepath.Join(home, "Documents", "iptv.subscription.*.txt")
internal/config/config.go:337:	path = filepath.Clean(path)
internal/plex/epg.go:26:	dbPath := filepath.Join(plexDataDir, "Plug-in Support", "Databases", fmt.Sprintf("tv.plex.providers.epg.xmltv-%s.db", dvrUUID))
internal/config/env_test.go:10:	err := LoadEnvFile(filepath.Join(t.TempDir(), "nonexistent"))
internal/config/env_test.go:18:	path := filepath.Join(dir, ".env")
internal/config/env_test.go:19:	if err := os.WriteFile(path, []byte("FOO=bar\n# comment\nBAZ=quux\n"), 0644); err != nil {
internal/config/env_test.go:35:	path := filepath.Join(dir, ".env")
internal/config/env_test.go:36:	if err := os.WriteFile(path, []byte(`X="hello world"`), 0644); err != nil {
internal/config/env_test.go:49:	path := filepath.Join(dir, ".env")
internal/config/env_test.go:50:	if err := os.WriteFile(path, []byte("export FOO=bar\n"), 0644); err != nil {
internal/plex/dvr.go:1054:	dbPath := filepath.Join(plexDataDir, "Plug-in Support", "Databases", "com.plexapp.plugins.library.db")
internal/plex/lineup_test.go:14:	plugSupport := filepath.Join(dir, "Plug-in Support", "Databases")
internal/plex/lineup_test.go:15:	if err := os.MkdirAll(plugSupport, 0755); err != nil {
internal/plex/lineup_test.go:18:	dbPath := filepath.Join(plugSupport, "com.plexapp.plugins.library.db")
internal/plex/lineup_test.go:20:	if err := os.WriteFile(dbPath, []byte{}, 0644); err != nil {
internal/plex/lineup_test.go:53:	plugSupport := filepath.Join(dir, "Plug-in Support", "Databases")
internal/plex/lineup_test.go:54:	if err := os.MkdirAll(plugSupport, 0755); err != nil {
internal/plex/lineup_test.go:57:	dbPath := filepath.Join(plugSupport, "com.plexapp.plugins.library.db")
internal/config/env.go:12:// Path is cleaned with filepath.Clean to avoid traversal if path is user-influenced.
internal/config/env.go:14:	path = filepath.Clean(path)
internal/plex/library.go:121:			sec.Locations = append(sec.Locations, filepath.Clean(loc.Path))
internal/plex/library.go:229:	spec.Path = filepath.Clean(strings.TrimSpace(spec.Path))
internal/plex/library.go:278:		sec.Locations = append(sec.Locations, filepath.Clean(loc.Path))
internal/plex/library.go:313:	wantPath := filepath.Clean(spec.Path)
internal/plex/library.go:322:			if filepath.Clean(p) == wantPath {
internal/plex/lineup.go:28:	dbPath := filepath.Join(plexDataDir, "Plug-in Support", "Databases", "com.plexapp.plugins.library.db")
internal/plex/logs.go:40:	logDir := filepath.Join(root, "Logs")
internal/plex/logs.go:55:		path := filepath.Join(logDir, name)
internal/config/config_test.go:290:	path := filepath.Join(dir, "sub.txt")
internal/config/config_test.go:291:	if err := os.WriteFile(path, []byte("Username: myuser\nPassword: mypass\n"), 0644); err != nil {
internal/config/config_test.go:304:	path := filepath.Join(dir, "sub.txt")
internal/config/config_test.go:305:	if err := os.WriteFile(path, []byte("Username: u\n"), 0644); err != nil {
internal/config/config_test.go:318:	path := filepath.Join(dir, "sub.txt")
internal/config/config_test.go:319:	if err := os.WriteFile(path, []byte("Username: fileuser\nPassword: filepass\n"), 0644); err != nil {
internal/provider/probe.go:48:// This matches what ffplay/ffmpeg sends by default and is often whitelisted by Cloudflare Bot Management.
internal/plex/inspect_test.go:16:	dbDir := filepath.Join(dir, "Plug-in Support", "Databases")
internal/plex/inspect_test.go:17:	if err := os.MkdirAll(dbDir, 0o755); err != nil {
internal/plex/inspect_test.go:20:	libDB := filepath.Join(dbDir, "com.plexapp.plugins.library.db")
internal/plex/inspect_test.go:31:	epgDB := filepath.Join(dbDir, "tv.plex.providers.epg.xmltv-demo.db")
internal/indexer/smoketest_cache.go:32:	data, err := os.ReadFile(path)
internal/indexer/smoketest_cache.go:50:	dir := filepath.Dir(filepath.Clean(path))
internal/indexer/smoketest_cache.go:51:	tmp, err := os.CreateTemp(dir, ".smoketest-*.json.tmp")
internal/indexer/smoketest_cache_test.go:17:	path := filepath.Join(dir, "smoketest.json")
internal/indexer/smoketest_cache_test.go:42:	c := LoadSmoketestCache(filepath.Join(t.TempDir(), "nonexistent.json"))
internal/indexer/smoketest_cache_test.go:89:	path := filepath.Join(dir, "smoketest.json")
internal/indexer/smoketest_cache_test.go:97:	entries, err := filepath.Glob(filepath.Join(dir, "*.tmp"))
internal/materializer/materializer_test.go:36:	dest := filepath.Join(dir, "out.mp4")
internal/materializer/materializer_test.go:60:	dest := filepath.Join(dir, "dl.bin")
internal/materializer/materializer_test.go:64:	got, err := os.ReadFile(dest)
internal/materializer/materializer_test.go:104:	dest := filepath.Join(dir, "r.mp4")
internal/materializer/materializer_test.go:109:	got, _ := os.ReadFile(dest)
internal/materializer/materializer_test.go:130:	target := filepath.Join(dir, "target.txt")
internal/materializer/materializer_test.go:131:	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
internal/materializer/materializer_test.go:134:	dest := filepath.Join(dir, "dl.partial")
internal/materializer/materializer_test.go:142:	got, err := os.ReadFile(target)
internal/materializer/materializer_test.go:161:	err := DownloadToFile(context.Background(), ts.URL+"/x.mp4", filepath.Join(dir, "x.mp4"), ts.Client())
internal/materializer/materializer_test.go:206:	got, err := os.ReadFile(path)
internal/materializer/materializer_test.go:283:	final := filepath.Join(cacheDir, "vod", "same.mp4")
internal/materializer/materializer_test.go:301:	final := filepath.Join(cacheDir, "vod", asset+".mp4")
internal/materializer/materializer_test.go:302:	if err := os.MkdirAll(filepath.Dir(final), 0755); err != nil {
internal/materializer/materializer_test.go:305:	if err := os.WriteFile(final, []byte("x"), 0644); err != nil {
internal/materializer/materializer_test.go:354:	got, err := os.ReadFile(p)
internal/emby/library.go:36:			loc = filepath.Clean(strings.TrimSpace(loc))
internal/emby/library.go:184:	spec.Path = filepath.Clean(strings.TrimSpace(spec.Path))
internal/emby/library.go:240:	wantPath := filepath.Clean(strings.TrimSpace(spec.Path))
internal/emby/library.go:250:			if filepath.Clean(loc) == wantPath {
internal/emby/state.go:22:	data, err := os.ReadFile(file)
internal/emby/state.go:39:	if err := os.MkdirAll(dir, 0o700); err != nil {
internal/emby/state.go:52:	tmp, err := os.CreateTemp(dir, ".emby-state-*.tmp")
internal/emby/state_test.go:12:	file := filepath.Join(dir, "state.json")
internal/emby/state_test.go:48:	file := filepath.Join(dir, "subdir", "nested", "state.json")
internal/emby/state_test.go:71:	file := filepath.Join(dir, "state.json")
internal/emby/state_test.go:84:	target := filepath.Join(dir, "target.json")
internal/emby/state_test.go:85:	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
internal/emby/state_test.go:88:	link := filepath.Join(dir, "state.json")
internal/emby/state_test.go:95:	if got, err := os.ReadFile(target); err != nil {
internal/emby/state_test.go:113:	file := filepath.Join(dir, "state.json")
internal/emby/state_test.go:114:	if err := os.WriteFile(file, []byte("not-json"), 0o644); err != nil {
internal/materializer/download.go:30:	if err := os.MkdirAll(filepath.Dir(destPath), 0o700); err != nil {
internal/materializer/cache.go:16:// Cache materializes both direct-MP4 and HLS URLs to the cache (DirectFile + HLS via ffmpeg).
internal/materializer/cache.go:83:	if err := os.MkdirAll(filepath.Dir(partialPath), 0o700); err != nil {
internal/virtualchannels/virtualchannels_test.go:12:	path := filepath.Join(t.TempDir(), "virtual-channels.json")
internal/materializer/hls.go:9:// materializeHLS writes an HLS (m3u8) stream to destPath as MP4 using ffmpeg remux (no transcode).
internal/materializer/hls.go:10:// Requires ffmpeg in PATH.
internal/materializer/hls.go:20:	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
internal/materializer/hls.go:24:		return fmt.Errorf("ffmpeg: %w", err)
internal/eventhooks/eventhooks_test.go:27:	cfgPath := filepath.Join(t.TempDir(), "hooks.json")
internal/eventhooks/eventhooks_test.go:28:	if err := os.WriteFile(cfgPath, []byte(`{"webhooks":[{"name":"test","url":"`+srv.URL+`","events":["lineup.updated"]}]}`), 0o644); err != nil {
internal/programming/programming_test.go:107:	path := filepath.Join(t.TempDir(), "programming.json")
internal/refio/refio.go:78:	absPath, err := filepath.Abs(filepath.Clean(raw))
internal/entitlements/entitlements_test.go:9:	path := filepath.Join(t.TempDir(), "xtream-users.json")
internal/refio/refio_test.go:14:	path := filepath.Join(dir, "sample.txt")
internal/refio/refio_test.go:15:	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
internal/refio/refio_test.go:53:	path := filepath.Join(dir, "guide.xml")
internal/refio/refio_test.go:54:	if err := os.WriteFile(path, []byte("<tv/>"), 0o600); err != nil {
internal/guideinput/guideinput_test.go:43:	path := filepath.Join(dir, "guide.xml")
internal/guideinput/guideinput_test.go:45:	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
internal/eventhooks/eventhooks.go:76:	raw, err := os.ReadFile(path)
internal/entitlements/entitlements.go:65:	data, err := os.ReadFile(path)
internal/entitlements/entitlements.go:89:	dir := filepath.Dir(filepath.Clean(path))
internal/entitlements/entitlements.go:90:	tmp, err := os.CreateTemp(dir, ".xtream-entitlements-*.json.tmp")
internal/guideinput/guideinput.go:97:	return os.ReadFile(local.Path())
internal/virtualchannels/virtualchannels.go:105:	data, err := os.ReadFile(path)
internal/virtualchannels/virtualchannels.go:129:	dir := filepath.Dir(filepath.Clean(path))
internal/virtualchannels/virtualchannels.go:130:	tmp, err := os.CreateTemp(dir, ".virtual-channels-*.json.tmp")
internal/webui/apiv2_settings.go:243:	data, err := os.ReadFile(path)
internal/webui/apiv2_settings.go:252:	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
internal/webui/apiv2_settings.go:260:	if err := os.WriteFile(tmp, data, 0o600); err != nil {
internal/livetvbundle/bundle_test.go:221:	stateFile := filepath.Join(t.TempDir(), "emby-state.json")
internal/plexharvest/plexharvest_test.go:133:	path := filepath.Join(t.TempDir(), "harvest.json")
internal/tuner/catchup_record_resilient.go:47:	laneDir := filepath.Join(outDir, firstNonEmptyString(capsule.Lane, "general"))
internal/tuner/catchup_record_resilient.go:48:	if err := os.MkdirAll(laneDir, 0o700); err != nil {
internal/tuner/catchup_record_resilient.go:247:	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
internal/programming/programming.go:132:	data, err := os.ReadFile(path)
internal/programming/programming.go:156:	dir := filepath.Dir(filepath.Clean(path))
internal/programming/programming.go:157:	tmp, err := os.CreateTemp(dir, ".programming-recipe-*.json.tmp")
internal/plexharvest/plexharvest.go:388:	data, err := os.ReadFile(path)
internal/plexharvest/plexharvest.go:420:	dir := filepath.Dir(filepath.Clean(path))
internal/plexharvest/plexharvest.go:421:	tmp, err := os.CreateTemp(dir, ".plex-lineup-harvest-*.json.tmp")
internal/tuner/cf_learned_store.go:45:	data, err := os.ReadFile(s.path)
internal/tuner/cf_learned_store.go:159:	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
internal/tuner/cf_learned_store.go:163:	if err := os.WriteFile(tmp, data, 0o600); err != nil {
internal/vodwebdav/webdav_test.go:33:	local := filepath.Join(tmp, "movie.mp4")
internal/vodwebdav/webdav_test.go:34:	if err := os.WriteFile(local, []byte("movie-bytes"), 0o600); err != nil {
internal/vodwebdav/webdav_test.go:83:	localMovie := filepath.Join(tmp, "movie.mp4")
internal/vodwebdav/webdav_test.go:84:	if err := os.WriteFile(localMovie, []byte("movie-bytes"), 0o600); err != nil {
internal/vodwebdav/webdav_test.go:87:	localEpisode := filepath.Join(tmp, "episode.mp4")
internal/vodwebdav/webdav_test.go:88:	if err := os.WriteFile(localEpisode, []byte("episode-bytes"), 0o600); err != nil {
internal/tuner/autopilot_test.go:22:	path := filepath.Join(t.TempDir(), "autopilot.json")
internal/tuner/autopilot_test.go:77:	target := filepath.Join(dir, "target.json")
internal/tuner/autopilot_test.go:78:	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
internal/tuner/autopilot_test.go:81:	link := filepath.Join(dir, "autopilot.json")
internal/tuner/autopilot_test.go:94:	if got, err := os.ReadFile(target); err != nil {
internal/tuner/autopilot_test.go:166:	path := filepath.Join(t.TempDir(), "host-policy.json")
internal/tuner/autopilot_test.go:167:	if err := os.WriteFile(path, []byte(`{"global_preferred_hosts":["cdn.file.example"],"global_blocked_hosts":["bad.file.example"]}`), 0o600); err != nil {
internal/migrationident/bundle.go:1296:	data, err := os.ReadFile(strings.TrimSpace(path))
internal/tuner/gateway_cookiejar.go:136:	data, err := os.ReadFile(p.file)
internal/tuner/gateway_cookiejar.go:200:	if err := os.MkdirAll(filepath.Dir(p.file), 0o700); err != nil {
internal/tuner/gateway_cookiejar.go:208:	if err := os.WriteFile(tmp, data, 0o600); err != nil {
internal/webui/webui.go:429:	data, err := os.ReadFile(s.StateFile)
internal/webui/webui.go:482:	if err := os.MkdirAll(dir, 0o700); err != nil {
internal/webui/webui.go:498:	tmp, err := os.CreateTemp(dir, ".deck-state-*.tmp")
internal/tuner/autopilot.go:72:	data, err := os.ReadFile(s.path)
internal/tuner/autopilot.go:156:	if err := os.MkdirAll(dir, 0o700); err != nil {
internal/tuner/autopilot.go:172:	tmp, err := os.CreateTemp(dir, ".autopilot-*.json.tmp")
internal/tuner/cookie_browser.go:56:			filepath.Join(home, ".config", "google-chrome", "Default", "Cookies"),
internal/tuner/cookie_browser.go:57:			filepath.Join(home, ".config", "google-chrome-beta", "Default", "Cookies"),
internal/tuner/cookie_browser.go:58:			filepath.Join(home, ".config", "chromium", "Default", "Cookies"),
internal/tuner/cookie_browser.go:59:			filepath.Join(home, ".config", "chromium-browser", "Default", "Cookies"),
internal/tuner/cookie_browser.go:60:			filepath.Join(home, ".config", "BraveSoftware", "Brave-Browser", "Default", "Cookies"),
internal/tuner/cookie_browser.go:64:			filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default", "Cookies"),
internal/tuner/cookie_browser.go:65:			filepath.Join(home, "Library", "Application Support", "Chromium", "Default", "Cookies"),
internal/tuner/cookie_browser.go:66:			filepath.Join(home, "Library", "Application Support", "BraveSoftware", "Brave-Browser", "Default", "Cookies"),
internal/tuner/cookie_browser.go:84:		profileBase = filepath.Join(home, ".mozilla", "firefox")
internal/tuner/cookie_browser.go:86:		profileBase = filepath.Join(home, "Library", "Application Support", "Firefox", "Profiles")
internal/tuner/cookie_browser.go:99:		p := filepath.Join(profileBase, e.Name(), "cookies.sqlite")
internal/tuner/gateway_ffmpeg_relay.go:32:			f.modeLabel = "ffmpeg-remux"
internal/tuner/gateway_ffmpeg_relay.go:138:	ffmpegPath string,
internal/tuner/gateway_ffmpeg_relay.go:151:	modeLabel := "hls-relay-ffmpeg-stdin-remux"
internal/tuner/gateway_ffmpeg_relay.go:153:		modeLabel = "hls-relay-ffmpeg-stdin-transcode"
internal/tuner/gateway_ffmpeg_relay.go:181:	cmd := exec.CommandContext(r.Context(), ffmpegPath, args...)
internal/tuner/gateway_ffmpeg_relay.go:253:			norm.done <- ffmpegRelayErr("hls-relay-stdin-copy", copyErr, stderr.String())
internal/tuner/gateway_ffmpeg_relay.go:257:			norm.done <- ffmpegRelayErr("hls-relay-stdin-wait", waitErr, stderr.String())
internal/tuner/gateway_ffmpeg_relay.go:267:func writeBootstrapTS(ctx context.Context, ffmpegPath string, dst io.Writer, channelName, channelID string, seconds float64, profile string) error {
internal/tuner/gateway_ffmpeg_relay.go:310:	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
internal/tuner/catchup_record_test.go:38:	data, err := os.ReadFile(item.OutputPath)
internal/tuner/catchup_record_test.go:45:	manifestData, err := os.ReadFile(filepath.Join(dir, "record-manifest.json"))
internal/tuner/catchup_record_test.go:60:	if want := filepath.Join("/out", "sports", "dna-test-1.partial.ts"); spool != want {
internal/tuner/catchup_record_test.go:63:	if want := filepath.Join("/out", "sports", "dna-test-1.ts"); final != want {
internal/tuner/catchup_record_test.go:84:	data, err := os.ReadFile(item.OutputPath)
internal/webui/webui_test.go:26:	st, err := store.Open(filepath.Join(root, "tunerr.db"))
internal/webui/webui_test.go:76:	logosDir := filepath.Join(root, "logos")
internal/webui/webui_test.go:77:	target := filepath.Join(root, "target.txt")
internal/webui/webui_test.go:78:	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
internal/webui/webui_test.go:81:	if err := os.MkdirAll(logosDir, 0o777); err != nil {
internal/webui/webui_test.go:84:	link := filepath.Join(logosDir, "logo.png")
internal/webui/webui_test.go:112:	if got, err := os.ReadFile(target); err != nil || string(got) != "original" {
internal/webui/webui_test.go:136:	secret := filepath.Join(root, "secret.png")
internal/webui/webui_test.go:137:	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
internal/webui/webui_test.go:140:	logosDir := filepath.Join(root, "logos")
internal/webui/webui_test.go:141:	if err := os.MkdirAll(logosDir, 0o700); err != nil {
internal/webui/webui_test.go:511:	bundlePath := filepath.Join(dir, "migration-bundle.json")
internal/webui/webui_test.go:516:	if err := os.WriteFile(bundlePath, data, 0o600); err != nil {
internal/webui/webui_test.go:566:	bundlePath := filepath.Join(dir, "identity-bundle.json")
internal/webui/webui_test.go:571:	if err := os.WriteFile(bundlePath, data, 0o644); err != nil {
internal/webui/webui_test.go:631:	planPath := filepath.Join(dir, "oidc-plan.json")
internal/webui/webui_test.go:636:	if err := os.WriteFile(planPath, data, 0o644); err != nil {
internal/webui/webui_test.go:749:	planPath := filepath.Join(dir, "oidc-plan.json")
internal/webui/webui_test.go:754:	if err := os.WriteFile(planPath, data, 0o644); err != nil {
internal/webui/webui_test.go:820:	planPath := filepath.Join(dir, "oidc-plan.json")
internal/webui/webui_test.go:825:	if err := os.WriteFile(planPath, data, 0o644); err != nil {
internal/webui/webui_test.go:884:	planPath := filepath.Join(dir, "oidc-plan.json")
internal/webui/webui_test.go:889:	if err := os.WriteFile(planPath, data, 0o644); err != nil {
internal/webui/webui_test.go:1101:	stateFile := filepath.Join(dir, "deck-state.json")
internal/webui/webui_test.go:1116:	data, err := os.ReadFile(stateFile)
internal/webui/webui_test.go:1149:	target := filepath.Join(dir, "target.json")
internal/webui/webui_test.go:1150:	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
internal/webui/webui_test.go:1153:	stateFile := filepath.Join(dir, "deck-state.json")
internal/webui/webui_test.go:1168:	if got, err := os.ReadFile(target); err != nil {
internal/webui/webui_test.go:1177:	stateFile := filepath.Join(dir, "deck-state.json")
internal/webui/webui_test.go:1178:	if err := os.WriteFile(stateFile, []byte(`{
internal/tuner/catchup_recorder_report_test.go:12:	stateFile := filepath.Join(dir, "recorder-state.json")
internal/tuner/catchup_recorder_report_test.go:27:			{CapsuleID: "done-1", Lane: "sports", Title: "Sports Done", PublishedPath: filepath.Join(dir, "sports", "done.ts")},
internal/tuner/catchup_recorder_report_test.go:39:	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
internal/tuner/gateway_provider_profile_test.go:123:	store := loadAccountLimitStore(filepath.Join(t.TempDir(), "provider-account-limits.json"), 12*time.Hour)
internal/tuner/catchup_record_publish.go:41:	itemDir := filepath.Join(rootDir, lane, dirName)
internal/tuner/catchup_record_publish.go:42:	if err := os.MkdirAll(itemDir, 0o700); err != nil {
internal/tuner/catchup_record_publish.go:47:	mediaPath := filepath.Join(itemDir, baseName+".ts")
internal/tuner/catchup_record_publish.go:51:	nfoPath := filepath.Join(itemDir, baseName+".nfo")
internal/tuner/catchup_record_publish.go:52:	if err := os.WriteFile(nfoPath, BuildCatchupMovieNFO(capsule), 0o600); err != nil {
internal/tuner/catchup_record_publish.go:80:	return os.WriteFile(filepath.Join(rootDir, "recorded-publish-manifest.json"), data, 0o600)
internal/tuner/catchup_record_publish.go:89:	data, err := os.ReadFile(filepath.Join(rootDir, "recorded-publish-manifest.json"))
internal/tuner/catchup_record_publish.go:119:				Path:           filepath.Join(rootDir, lane),
internal/tuner/catchup_record_publish.go:171:	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
internal/tuner/catchup_record_publish.go:193:	tmp, err := os.CreateTemp(dir, ".publish-*.tmp")
internal/livetvbundle/bundle.go:1166:	return os.ReadFile(path)
internal/livetvbundle/bundle.go:1170:	return os.WriteFile(path, data, 0o600)
internal/livetvbundle/bundle.go:1280:	return filepath.Clean(strings.ReplaceAll(value, `\`, `/`))
internal/tuner/catchup_recorder_report.go:40:	data, err := os.ReadFile(path)
internal/tuner/catchup_replay_test.go:76:	data, err := os.ReadFile(manifest.Items[0].StreamPath)
internal/tuner/catchup_publish_test.go:41:	streamData, err := os.ReadFile(item.StreamPath)
internal/tuner/catchup_publish_test.go:48:	nfoData, err := os.ReadFile(item.NFOPath)
internal/tuner/catchup_publish_test.go:58:	manifestPath := filepath.Join(dir, "publish-manifest.json")
internal/tuner/catchup_publish_test.go:59:	data, err := os.ReadFile(manifestPath)
internal/tuner/catchup_publish_test.go:75:	if info, err := os.Stat(filepath.Join(dir, "general")); err != nil {
internal/tuner/catchup_publish_test.go:94:	itemDir := filepath.Join(dir, "general", "Adventure-Time-2026-03-18-18-00-UTC")
internal/tuner/catchup_publish_test.go:95:	if err := os.MkdirAll(itemDir, 0o700); err != nil {
internal/tuner/catchup_publish_test.go:98:	target := filepath.Join(dir, "target.strm")
internal/tuner/catchup_publish_test.go:99:	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
internal/tuner/catchup_publish_test.go:102:	if err := os.Symlink(target, filepath.Join(itemDir, "Adventure-Time-2026-03-18-18-00-UTC.strm")); err != nil {
internal/tuner/catchup_publish_test.go:125:	if got, err := os.ReadFile(target); err != nil {
internal/tuner/catchup_publish_test.go:164:		Directory: filepath.Join(dir, "sports", "x"),
internal/tuner/catchup_publish_test.go:165:		MediaPath: filepath.Join(dir, "sports", "x", "x.ts"),
internal/tuner/catchup_publish_test.go:182:	recordedPath := filepath.Join(recordedDir, "recorded.ts")
internal/tuner/catchup_publish_test.go:183:	if err := os.WriteFile(recordedPath, []byte("media"), 0o600); err != nil {
internal/tuner/catchup_publish_test.go:214:	src := filepath.Join(dir, "src.ts")
internal/tuner/catchup_publish_test.go:215:	if err := os.WriteFile(src, []byte("media"), 0o600); err != nil {
internal/tuner/catchup_publish_test.go:218:	target := filepath.Join(dir, "target.ts")
internal/tuner/catchup_publish_test.go:219:	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
internal/tuner/catchup_publish_test.go:222:	dst := filepath.Join(dir, "dst.ts")
internal/tuner/catchup_publish_test.go:229:	if got, err := os.ReadFile(target); err != nil {
internal/tuner/gateway_upstream_ua_test.go:9:	for _, name := range []string{"lavf", "ffmpeg", "FFMPEG", "Lavf", "libavformat"} {
internal/tuner/gateway_upstream_ua_test.go:58:		t.Skip("ffprobe/ffmpeg not installed; skipping UA detection test")
internal/webui/apiv2_logos.go:57:	dir := filepath.Clean(s.logosDir())
internal/webui/apiv2_logos.go:58:	dest := filepath.Join(dir, safe)
internal/webui/apiv2_logos.go:107:		if err := os.MkdirAll(dir, 0o700); err != nil {
internal/webui/apiv2_logos.go:120:		tmp, err := os.CreateTemp(dir, ".upload-*.tmp")
internal/tuner/ts_inspector.go:185:		return "ffmpeg-remux"
internal/tuner/psi_keepalive.go:14:// PID values match ffmpeg mpegts muxer defaults (mpegts_pmt_start_pid=0x1000,
internal/tuner/psi_keepalive.go:18:	patPMTKeepPMTPID   = 0x1000 // ffmpeg default first PMT PID
internal/tuner/psi_keepalive.go:19:	patPMTKeepVideoPID = 0x0100 // ffmpeg default video elementary stream PID
internal/tuner/psi_keepalive.go:20:	patPMTKeepAudioPID = 0x0101 // ffmpeg default audio elementary stream PID
internal/tuner/psi_keepalive.go:147:// waits for ffmpeg to produce a valid IDR frame. By sending MPEG-TS program-structure
internal/tuner/psi_keepalive.go:154:// These PIDs match ffmpeg's mpegts muxer defaults so the keepalive packets are
internal/tuner/catchup_publish.go:69:	if err := os.MkdirAll(outDir, 0o700); err != nil {
internal/tuner/catchup_publish.go:82:		laneDir := filepath.Join(outDir, lane)
internal/tuner/catchup_publish.go:83:		if err := os.MkdirAll(laneDir, 0o700); err != nil {
internal/tuner/catchup_publish.go:104:		itemDir := filepath.Join(outDir, lane, dirName)
internal/tuner/catchup_publish.go:105:		if err := os.MkdirAll(itemDir, 0o700); err != nil {
internal/tuner/catchup_publish.go:110:		streamPath := filepath.Join(itemDir, baseName+".strm")
internal/tuner/catchup_publish.go:118:		nfoPath := filepath.Join(itemDir, baseName+".nfo")
internal/tuner/catchup_publish.go:147:	manifestPath := filepath.Join(outDir, "publish-manifest.json")
internal/tuner/catchup_capsules_export.go:26:	if err := os.MkdirAll(outDir, 0o700); err != nil {
internal/tuner/catchup_capsules_export.go:54:		path := filepath.Join(outDir, lane+".json")
internal/tuner/catchup_capsules_export.go:70:	manifestPath := filepath.Join(outDir, "manifest.json")
internal/tuner/catchup_capsules_export.go:83:	if err := os.MkdirAll(dir, 0o700); err != nil {
internal/tuner/catchup_capsules_export.go:97:	tmp, err := os.CreateTemp(dir, ".catchup-artifact-*.tmp")
internal/tuner/gateway_attempts.go:50:	FFmpegHeaders     []string `json:"ffmpeg_headers,omitempty"`
internal/tuner/gateway_attempts.go:207:func ffmpegHeaderSummary(block string) []string {
internal/tuner/gateway_debug.go:51:func sanitizeFileToken(s string) string {
internal/tuner/gateway_debug.go:148:	if err := os.MkdirAll(dir, 0o700); err != nil {
internal/tuner/gateway_debug.go:155:		sanitizeFileToken(reqID),
internal/tuner/gateway_debug.go:156:		sanitizeFileToken(channelID),
internal/tuner/gateway_debug.go:157:		sanitizeFileToken(channelName),
internal/tuner/gateway_debug.go:159:	path := filepath.Join(dir, name)
internal/tuner/catchup_daemon_test.go:57:	data, err := os.ReadFile(state.Completed[0].OutputPath)
internal/tuner/catchup_daemon_test.go:64:	stateData, err := os.ReadFile(filepath.Join(dir, "recorder-state.json"))
internal/tuner/catchup_daemon_test.go:85:	publishDir := filepath.Join(dir, "published")
internal/tuner/catchup_daemon_test.go:88:		OutDir:         filepath.Join(dir, "recordings"),
internal/tuner/catchup_daemon_test.go:121:	if _, err := os.Stat(filepath.Join(publishDir, "recorded-publish-manifest.json")); err != nil {
internal/tuner/catchup_daemon_test.go:134:	publishDir := filepath.Join(dir, "published")
internal/tuner/catchup_daemon_test.go:138:		OutDir:         filepath.Join(dir, "recordings"),
internal/tuner/catchup_daemon_test.go:202:	stateFile := filepath.Join(dir, "recorder-state.json")
internal/tuner/catchup_daemon_test.go:203:	expiredTS := filepath.Join(dir, "old.ts")
internal/tuner/catchup_daemon_test.go:204:	if err := os.WriteFile(expiredTS, []byte("old"), 0o600); err != nil {
internal/tuner/catchup_daemon_test.go:219:	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
internal/tuner/catchup_daemon_test.go:405:	stateFile := filepath.Join(dir, "recorder-state.json")
internal/tuner/catchup_daemon_test.go:406:	oldPath := filepath.Join(dir, "sports", "old.ts")
internal/tuner/catchup_daemon_test.go:407:	if err := os.MkdirAll(filepath.Dir(oldPath), 0o755); err != nil {
internal/tuner/catchup_daemon_test.go:410:	if err := os.WriteFile(oldPath, []byte("old"), 0o600); err != nil {
internal/tuner/catchup_daemon_test.go:425:				OutputPath: filepath.Join(dir, "sports", "newest.ts"),
internal/tuner/catchup_daemon_test.go:440:	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
internal/tuner/catchup_daemon_test.go:484:	stateFile := filepath.Join(dir, "recorder-state.json")
internal/tuner/catchup_daemon_test.go:485:	keepPath := filepath.Join(dir, "movies", "keep.ts")
internal/tuner/catchup_daemon_test.go:486:	dropPath := filepath.Join(dir, "movies", "drop.ts")
internal/tuner/catchup_daemon_test.go:487:	if err := os.MkdirAll(filepath.Dir(keepPath), 0o755); err != nil {
internal/tuner/catchup_daemon_test.go:490:	if err := os.WriteFile(keepPath, []byte("12345"), 0o600); err != nil {
internal/tuner/catchup_daemon_test.go:493:	if err := os.WriteFile(dropPath, []byte("67890"), 0o600); err != nil {
internal/tuner/catchup_daemon_test.go:505:	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
internal/tuner/catchup_daemon_test.go:535:	stateFile := filepath.Join(dir, "recorder-state.json")
internal/tuner/catchup_daemon_test.go:536:	partialPath := filepath.Join(dir, "sports", "active-1.partial.ts")
internal/tuner/catchup_daemon_test.go:537:	if err := os.MkdirAll(filepath.Dir(partialPath), 0o755); err != nil {
internal/tuner/catchup_daemon_test.go:540:	if err := os.WriteFile(partialPath, []byte("partial"), 0o600); err != nil {
internal/tuner/catchup_daemon_test.go:559:	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
internal/tuner/catchup_daemon_test.go:597:	stateFile := filepath.Join(dir, "recorder-state.json")
internal/tuner/catchup_daemon_test.go:613:	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
internal/tuner/catchup_capsules_export_test.go:27:	manifestPath := filepath.Join(dir, "manifest.json")
internal/tuner/catchup_capsules_export_test.go:28:	data, err := os.ReadFile(manifestPath)
internal/tuner/catchup_capsules_export_test.go:53:	target := filepath.Join(dir, "target.json")
internal/tuner/catchup_capsules_export_test.go:54:	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
internal/tuner/catchup_capsules_export_test.go:57:	if err := os.Symlink(target, filepath.Join(dir, "sports.json")); err != nil {
internal/tuner/catchup_capsules_export_test.go:70:	if got, err := os.ReadFile(target); err != nil {
internal/tuner/catchup_record.go:47:	laneDir := filepath.Join(outDir, firstNonEmptyString(capsule.Lane, "general"))
internal/tuner/catchup_record.go:49:	return filepath.Join(laneDir, base+".partial.ts"), filepath.Join(laneDir, base+".ts")
internal/tuner/catchup_record.go:69:	if err := os.MkdirAll(outDir, 0o700); err != nil {
internal/tuner/catchup_record.go:97:	if err := os.WriteFile(filepath.Join(outDir, "record-manifest.json"), data, 0o600); err != nil {
internal/tuner/catchup_record_resilient_test.go:15:	spool := filepath.Join(dir, "x.partial.ts")
internal/tuner/catchup_record_resilient_test.go:16:	if err := os.WriteFile(spool, []byte("abc"), 0o600); err != nil {
internal/tuner/catchup_record_resilient_test.go:35:	data, err := os.ReadFile(spool)
internal/tuner/catchup_record_resilient_test.go:51:	target := filepath.Join(dir, "target.ts")
internal/tuner/catchup_record_resilient_test.go:52:	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
internal/tuner/catchup_record_resilient_test.go:55:	spool := filepath.Join(dir, "x.partial.ts")
internal/tuner/catchup_record_resilient_test.go:69:	if got, err := os.ReadFile(target); err != nil {
internal/tuner/catchup_record_resilient_test.go:103:	laneDir := filepath.Join(dir, "sports")
internal/tuner/ua_cycle.go:42:// detectedLavfUA is the auto-detected "Lavf/X.Y.Z" from the installed ffmpeg binary.
internal/tuner/cf_bootstrap.go:286:	cmd := exec.CommandContext(timeoutCtx, bin, args...)
internal/tuner/cf_bootstrap.go:292:	cookieDB := filepath.Join(dir, "Default", "Cookies")
internal/tuner/cf_bootstrap.go:336:	_ = exec.CommandContext(ctx, openCmd, rawURL).Start()
internal/tuner/gateway.go:36:	CustomUserAgent            string            // override User-Agent sent to upstream; supports preset names: lavf, ffmpeg, vlc, kodi, firefox
internal/tuner/gateway.go:37:	DetectedFFmpegUA           string            // auto-detected Lavf/X.Y.Z from installed ffmpeg, used when CustomUserAgent is "lavf"/"ffmpeg"
internal/tuner/gateway.go:59:	hlsPackagerSessions        map[string]*ffmpegHLSPackagerSession
internal/tuner/gateway.go:60:	hlsPackagerSessionsByKey   map[string]*ffmpegHLSPackagerSession
internal/epgstore/quota_test.go:10:	s, err := Open(filepath.Join(dir, "q.db"))
internal/epgstore/quota_test.go:23:	s, err := Open(filepath.Join(dir, "q2.db"))
internal/tuner/catchup_daemon.go:137:	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
internal/tuner/catchup_daemon.go:141:		if err := os.MkdirAll(strings.TrimSpace(cfg.PublishDir), 0o755); err != nil {
internal/tuner/catchup_daemon.go:147:		stateFile = filepath.Join(cfg.OutDir, "recorder-state.json")
internal/tuner/catchup_daemon.go:276:	data, err := os.ReadFile(m.stateFile)
internal/tuner/catchup_daemon.go:562:	if err := os.WriteFile(m.stateFile, data, 0o600); err != nil {
internal/tuner/catchup_daemon.go:715:	return os.WriteFile(m.stateFile, data, 0o600)
internal/tuner/account_limit_store_test.go:10:	path := filepath.Join(t.TempDir(), "provider-account-limits.json")
internal/tuner/server_diagnostics_recordings.go:21:	return filepath.Clean(".diag")
internal/tuner/server_diagnostics_recordings.go:28:		dir := filepath.Join(root, family)
internal/tuner/server_diagnostics_recordings.go:49:					Path:    filepath.Join(dir, entry.Name()),
internal/tuner/server_diagnostics_recordings.go:69:	reportPath := filepath.Join(ref.Path, "report.json")
internal/tuner/server_diagnostics_recordings.go:70:	body, err := os.ReadFile(reportPath)
internal/tuner/server_diagnostics_recordings.go:84:	textPath := filepath.Join(ref.Path, "report.txt")
internal/tuner/server_diagnostics_recordings.go:85:	body, err = os.ReadFile(textPath)
internal/tuner/server_diagnostics_recordings.go:224:	if err := os.MkdirAll(outDir, 0o700); err != nil {
internal/tuner/server_diagnostics_recordings.go:228:		if err := os.MkdirAll(filepath.Join(outDir, sub), 0o700); err != nil {
internal/tuner/server_diagnostics_recordings.go:274:	if err := os.WriteFile(filepath.Join(outDir, "notes.md"), []byte(notes), 0o600); err != nil {
internal/tuner/server_diagnostics_recordings.go:277:	if err := os.WriteFile(filepath.Join(outDir, "README.txt"), []byte(readme), 0o600); err != nil {
internal/tuner/server_diagnostics_recordings.go:288:	return sanitizeFileToken(value)
internal/tuner/server_diagnostics_recordings.go:310:	scriptsDir := filepath.Clean("scripts")
internal/tuner/server_diagnostics_recordings.go:311:	path := filepath.Join(scriptsDir, name)
internal/tuner/server_diagnostics_recordings.go:328:	cmd := exec.CommandContext(ctx, "bash", scriptPath)
internal/tuner/server_diagnostics_recordings.go:342:		outDir = filepath.Join(outRoot, runID)
internal/tuner/server_diagnostics_recordings.go:349:	if reportPath := filepath.Join(outDir, "report.json"); outDir != "" {
internal/tuner/server_diagnostics_recordings.go:353:		if _, statErr := os.Stat(filepath.Join(outDir, "report.txt")); statErr == nil {
internal/tuner/server_diagnostics_recordings.go:354:			detail["report_text_path"] = filepath.Join(outDir, "report.txt")
internal/tuner/server_diagnostics_recordings.go:437:		"OUT_ROOT":        filepath.Join(repoDiagRoot(), "channel-diff"),
internal/tuner/server_diagnostics_recordings.go:473:		"OUT_ROOT":          filepath.Join(repoDiagRoot(), "stream-compare"),
internal/tuner/epg_pipeline_test.go:43:	cacheFile := filepath.Join(t.TempDir(), "provider.xml")
internal/tuner/epg_pipeline_test.go:101:	cacheFile := filepath.Join(t.TempDir(), "provider.xml")
internal/tuner/epg_pipeline_test.go:103:	if err := os.WriteFile(cacheFile, []byte(cacheBody), 0644); err != nil {
internal/tuner/epg_pipeline_test.go:159:	cacheFile := filepath.Join(t.TempDir(), "provider.xml")
internal/tuner/epg_pipeline_test.go:185:	cached, err := os.ReadFile(cacheFile)
internal/tuner/epg_pipeline_test.go:196:	target := filepath.Join(dir, "target.xml")
internal/tuner/epg_pipeline_test.go:197:	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
internal/tuner/epg_pipeline_test.go:200:	cacheFile := filepath.Join(dir, "provider.xml")
internal/tuner/epg_pipeline_test.go:207:	got, err := os.ReadFile(target)
internal/tuner/epg_pipeline_test.go:712:	cacheFile := filepath.Join(t.TempDir(), "provider.xml")
internal/tuner/epg_pipeline_test.go:717:	if err := os.WriteFile(cacheFile, []byte(cacheBody), 0o600); err != nil {
internal/tuner/ghost_hunter_recovery.go:44:	cmd := exec.CommandContext(ctx, path, args...)
internal/epgstore/store_test.go:11:	path := filepath.Join(dir, "epg", "test.db")
internal/epgstore/store_test.go:42:	path := filepath.Join(dir, "p.db")
internal/epgstore/store_test.go:80:	path := filepath.Join(dir, "g.db")
internal/epgstore/store_test.go:128:	path := filepath.Join(dir, "u.db")
internal/tuner/account_limit_store.go:43:	data, err := os.ReadFile(s.path)
internal/tuner/account_limit_store.go:122:	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
internal/tuner/account_limit_store.go:130:	if err := os.WriteFile(tmp, data, 0o600); err != nil {
internal/tuner/lineup_probe.go:89:	ffmpegPath, err := resolveFFmpegPath()
internal/tuner/lineup_probe.go:91:		log.Printf("Lineup visual probe skipped: ffmpeg unavailable: %v", err)
internal/tuner/lineup_probe.go:169:			pass := probeStreamVisual(ctx, ffmpegPath, cand.url, sample, timeout)
internal/tuner/lineup_probe.go:268:func probeStreamVisual(parent context.Context, ffmpegPath, streamURL string, sample, timeout time.Duration) bool {
internal/tuner/lineup_probe.go:286:	out, err := exec.CommandContext(ctx, ffmpegPath, args...).CombinedOutput()
internal/tuner/gateway_upstream.go:21:// defaultLavfUA is the fallback Lavf User-Agent when ffmpeg is not installed or detection fails.
internal/tuner/gateway_upstream.go:22:// Matches the libavformat version shipped with ffmpeg 7.1 (2024).
internal/tuner/gateway_upstream.go:25:// detectFFmpegLavfUA runs ffprobe (or ffmpeg) to read the libavformat version and returns
internal/tuner/gateway_upstream.go:28:	for _, bin := range []string{"ffprobe", "ffmpeg"} {
internal/tuner/gateway_upstream.go:29:		out, err := exec.Command(bin, "-version").Output()
internal/tuner/gateway_upstream.go:65:// detectedLavfUA is the auto-detected value from the installed ffmpeg, used for the
internal/tuner/gateway_upstream.go:66:// "lavf"/"ffmpeg" preset so the Go HTTP client sends the same UA as the ffmpeg subprocess.
internal/tuner/gateway_upstream.go:70:	case "lavf", "ffmpeg", "libavformat":
internal/tuner/gateway_upstream.go:210:func (g *Gateway) ffmpegCookiesOptionForURL(rawURL string) string {
internal/tuner/gateway_upstream.go:359:func (g *Gateway) ffmpegInputHeaderBlock(incoming *http.Request, rawURL, hostOverride string) string {
internal/tuner/cf_client_test.go:15:	t.Setenv("IPTV_TUNERR_COOKIE_JAR_FILE", filepath.Join(t.TempDir(), "cookies.json"))
internal/tuner/autopilot_policy.go:29:	data, err := os.ReadFile(path)
internal/tuner/recording_rules.go:121:	data, err := os.ReadFile(path)
internal/tuner/recording_rules.go:145:	dir := filepath.Dir(filepath.Clean(path))
internal/tuner/recording_rules.go:146:	tmp, err := os.CreateTemp(dir, ".recording-rules-*.json.tmp")
internal/epgstore/store.go:24:	path = filepath.Clean(strings.TrimSpace(path))
internal/epgstore/store.go:30:		if err := os.MkdirAll(dir, 0o755); err != nil {
internal/tuner/gateway_ffmpeg_options_test.go:7:	if ffmpegHLSHTTPPersistentEnabled() {
internal/tuner/gateway_ffmpeg_options_test.go:14:	if !ffmpegHLSHTTPPersistentEnabled() {
internal/tuner/gateway_ffmpeg_options_test.go:21:	if got := ffmpegHLSLiveStartIndex(); got != 0 {
internal/tuner/gateway_ffmpeg_options_test.go:28:	if got := ffmpegHLSLiveStartIndex(); got != -3 {
internal/tuner/gateway_shared_leases.go:101:	leasePath := filepath.Join(m.dir, m.leaseFilename(identity.Key, token))
internal/tuner/gateway_shared_leases.go:181:		path := filepath.Join(m.dir, entry.Name())
internal/tuner/gateway_shared_leases.go:190:		data, err := os.ReadFile(path)
internal/tuner/gateway_shared_leases.go:242:	path := filepath.Join(m.dir, m.lockFilename(key))
internal/tuner/gateway_shared_leases.go:291:		out = append(out, filepath.Join(m.dir, name))
internal/tuner/gateway_shared_leases.go:297:	if err := os.MkdirAll(m.dir, 0o700); err != nil {
internal/tuner/gateway_shared_leases.go:308:	tmp, err := os.CreateTemp(dir, ".lease-*.tmp")
internal/tuner/gateway_ffmpeg_options.go:3:// Some ffmpeg/libavformat builds do not support the `-http_persistent` input
internal/tuner/gateway_ffmpeg_options.go:6:func ffmpegHLSHTTPPersistentEnabled() bool {
internal/tuner/gateway_ffmpeg_options.go:10:// Keep live-start seeking opt-in too: some ffmpeg builds reject the option,
internal/tuner/gateway_ffmpeg_options.go:12:func ffmpegHLSLiveStartIndex() int {
internal/tuner/gateway_policy.go:236:// shouldPreferGoRelayForHLS decides whether to skip direct ffmpeg HLS input and use the Go HLS
internal/tuner/gateway_policy.go:345:		out, err := exec.CommandContext(ctx, ffprobePath, args...).Output()
internal/tuner/gateway_profiles.go:158:	b, err := os.ReadFile(path)
internal/tuner/gateway_profiles.go:182:	b, err := os.ReadFile(path)
internal/tuner/gateway_profiles.go:215:	b, err := os.ReadFile(path)
internal/tuner/gateway_profiles.go:351:// buildFFmpegStreamOutputArgs builds ffmpeg output args for MPEG-TS or fragmented MP4 (LP-010/011).
internal/tuner/gateway_profiles.go:675:// to a numeric host for ffmpeg. This avoids resolver differences where Go can
internal/tuner/gateway_profiles.go:677:// ffmpeg binary cannot.
internal/tuner/gateway_provider_profile.go:51:	FFMPEGHLSReconnect     bool                        `json:"ffmpeg_hls_reconnect"`
internal/tuner/gateway_provider_profile.go:298:	row.LastKind = "ffmpeg_hls_failed"
internal/tuner/server_virtual_channel_streams.go:155:		ffmpegPath, err := resolveFFmpegPath()
internal/tuner/server_virtual_channel_streams.go:157:			writeServerJSONError(w, http.StatusServiceUnavailable, "ffmpeg not available for branded stream")
internal/tuner/server_virtual_channel_streams.go:165:		if !relayVirtualChannelBrandedStream(w, r, ffmpegPath, resp.Body, channel) {
internal/tuner/server_virtual_channel_streams.go:765:	ffmpegPath, err := resolveFFmpegPath()
internal/tuner/server_virtual_channel_streams.go:782:	out, err := exec.CommandContext(ctx, ffmpegPath, args...).CombinedOutput()
internal/tuner/server_virtual_channel_streams.go:800:	ffmpegPath, err := resolveFFmpegPath()
internal/tuner/server_virtual_channel_streams.go:816:	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
internal/tuner/server_virtual_channel_streams.go:970:	data, err := os.ReadFile(path)
internal/tuner/server_virtual_channel_streams.go:1016:	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
internal/tuner/server_virtual_channel_streams.go:1020:	if err := os.WriteFile(path, data, 0o600); err != nil {
internal/tuner/server_virtual_channel_streams.go:1045:func relayVirtualChannelBrandedStream(w http.ResponseWriter, r *http.Request, ffmpegPath string, src io.ReadCloser, ch virtualchannels.Channel) bool {
internal/tuner/server_virtual_channel_streams.go:1080:	cmd := exec.CommandContext(r.Context(), ffmpegPath, args...)
internal/tuner/server_virtual_channel_streams.go:1106:				ffmpegEscapeText(text), x, y,
internal/tuner/server_virtual_channel_streams.go:1112:				fmt.Sprintf("drawtext=text='%s':fontcolor=white:fontsize=28:x=60:y=h-70", ffmpegEscapeText(banner)),
internal/tuner/server_virtual_channel_streams.go:1131:			ffmpegEscapeText(text), tx, ty, next,
internal/tuner/server_virtual_channel_streams.go:1143:			fmt.Sprintf("%sdrawtext=text='%s':fontcolor=white:fontsize=28:x=60:y=h-70%s", boxStage, ffmpegEscapeText(banner), next),
internal/tuner/server_virtual_channel_streams.go:1176:func ffmpegEscapeText(raw string) string {
internal/tuner/gateway_hls_packager_test.go:16:func TestGateway_ffmpegPackagedHLS_namedProfileServesPlaylistAndSegment(t *testing.T) {
internal/tuner/gateway_hls_packager_test.go:18:	ffmpegPath := filepath.Join(dir, "fake-ffmpeg.sh")
internal/tuner/gateway_hls_packager_test.go:42:	if err := os.WriteFile(ffmpegPath, []byte(script), 0755); err != nil {
internal/tuner/gateway_hls_packager_test.go:45:	t.Setenv("IPTV_TUNERR_FFMPEG_PATH", ffmpegPath)
internal/tuner/gateway_hls_packager_test.go:118:func TestGateway_ffmpegPackagedHLS_targetRequiresGetOrHead(t *testing.T) {
internal/tuner/gateway_hls_packager_test.go:134:func TestGateway_ffmpegPackagedHLS_sameProfileReusesExistingSession(t *testing.T) {
internal/tuner/gateway_hls_packager_test.go:136:	ffmpegPath := filepath.Join(dir, "fake-ffmpeg.sh")
internal/tuner/gateway_hls_packager_test.go:160:	if err := os.WriteFile(ffmpegPath, []byte(script), 0755); err != nil {
internal/tuner/gateway_hls_packager_test.go:163:	t.Setenv("IPTV_TUNERR_FFMPEG_PATH", ffmpegPath)
internal/tuner/gateway_hls_packager_test.go:223:	if got := rec2.Header().Get("X-IptvTunerr-Shared-Upstream"); got != "ffmpeg_hls_packager" {
internal/tuner/gateway_hls_packager_test.go:257:		hlsPackagerSessions:      map[string]*ffmpegHLSPackagerSession{},
internal/tuner/gateway_hls_packager_test.go:258:		hlsPackagerSessionsByKey: map[string]*ffmpegHLSPackagerSession{},
internal/tuner/gateway_hls_packager_test.go:267:	sess := &ffmpegHLSPackagerSession{
internal/tuner/gateway_profiles_test.go:154:	path := filepath.Join(dir, "profiles.json")
internal/tuner/gateway_profiles_test.go:160:	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
internal/tuner/gateway_profiles_test.go:223:	path := filepath.Join(dir, "bad.json")
internal/tuner/gateway_profiles_test.go:224:	if err := os.WriteFile(path, []byte(`{`), 0600); err != nil {
internal/tuner/gateway_profiles_test.go:235:	path := filepath.Join(dir, "profiles.json")
internal/tuner/gateway_profiles_test.go:237:	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
internal/tuner/gateway_stream_response.go:268:			return "ok", "hls_ffmpeg_packaged", effectiveURL, true
internal/tuner/gateway_stream_response.go:270:		log.Printf("gateway: channel=%q id=%s ffmpeg-hls-packager failed (falling back to normal relay): profile=%q",
internal/tuner/gateway_stream_response.go:278:		log.Printf("gateway: channel=%q id=%s cross-host-hls prefers go relay over ffmpeg-remux playlist_host=%q refs=%q",
internal/tuner/gateway_stream_response.go:282:		log.Printf("gateway: channel=%q id=%s provider-pressure prefers go relay over ffmpeg-remux",
internal/tuner/gateway_stream_response.go:286:		if ffmpegPath, ffmpegErr := resolveFFmpegPath(); ffmpegErr == nil {
internal/tuner/gateway_stream_response.go:287:			attempt.setFFmpegHeaders(attemptIdx, ffmpegHeaderSummary(g.ffmpegInputHeaderBlock(r, effectiveURL, "")))
internal/tuner/gateway_stream_response.go:292:				"hls_ffmpeg",
internal/tuner/gateway_stream_response.go:295:			ffmpegRelayErr := g.relayHLSWithFFmpeg(w, r, ffmpegPath, streamURL, channel.GuideName, channelID, channel.GuideNumber, channel.TVGID, start, transcode, bufferSize, forcedProfile, hotStart, outputMux, sharedSession)
internal/tuner/gateway_stream_response.go:296:			if ffmpegRelayErr == nil {
internal/tuner/gateway_stream_response.go:299:				return "ok", "hls_ffmpeg", effectiveURL, true
internal/tuner/gateway_stream_response.go:301:			attempt.markUpstreamError(attemptIdx, "ffmpeg_hls_failed", ffmpegRelayErr)
internal/tuner/gateway_stream_response.go:303:			g.noteUpstreamFailure(streamURL, 0, "ffmpeg_hls_failed")
internal/tuner/gateway_stream_response.go:304:			log.Printf("gateway: channel=%q id=%s ffmpeg-%s failed (falling back to go relay): %v",
internal/tuner/gateway_stream_response.go:305:				channel.GuideName, channelID, mode, ffmpegRelayErr)
internal/tuner/gateway_stream_response.go:307:				log.Printf("gateway: channel=%q id=%s ffmpeg-%s response already started; not attempting go-relay fallback on same response",
internal/tuner/gateway_stream_response.go:309:				return "ffmpeg_hls_failed_started", "hls_ffmpeg_failed_started", effectiveURL, true
internal/tuner/gateway_stream_response.go:312:			log.Printf("gateway: channel=%q id=%s ffmpeg unavailable path=%q err=%v",
internal/tuner/gateway_stream_response.go:313:				channel.GuideName, channelID, os.Getenv("IPTV_TUNERR_FFMPEG_PATH"), ffmpegErr)
internal/tuner/gateway_stream_response.go:315:			log.Printf("gateway: channel=%q id=%s ffmpeg unavailable transcode-requested=true err=%v (falling back to go relay; web clients may get incompatible audio/video codecs)", channel.GuideName, channelID, ffmpegErr)
internal/tuner/gateway_stream_response.go:318:		log.Printf("gateway: channel=%q id=%s go relay preferred over direct ffmpeg hls input", channel.GuideName, channelID)
internal/tuner/gateway_stream_response.go:320:		log.Printf("gateway: channel=%q id=%s ffmpeg disabled by config (using go relay)", channel.GuideName, channelID)
internal/tuner/gateway_stream_response.go:328:			"hls_relay_ffmpeg_stdin",
internal/tuner/gateway_stream_response.go:444:		if ffmpegPath, ffmpegErr := resolveFFmpegPath(); ffmpegErr == nil {
internal/tuner/gateway_stream_response.go:449:				"raw_ts_ffmpeg",
internal/tuner/gateway_stream_response.go:452:			if g.relayRawTSWithFFmpeg(w, r, ffmpegPath, resp.Body, channel.GuideName, channelID, resp.StatusCode, start, bufferSize, sharedSession) {
internal/tuner/gateway_stream_response.go:455:			log.Printf("gateway: channel=%q id=%s ffmpeg-ts-norm failed to launch; falling back to raw proxy", channel.GuideName, channelID)
internal/tuner/gateway_servehttp.go:104:				finalMode = "hls_ffmpeg_packaged_shared"
internal/tuner/gateway_servehttp.go:114:			finalMode = "hls_ffmpeg_shared"
internal/tuner/gateway_servehttp.go:119:			finalMode = "raw_ts_ffmpeg_shared"
internal/tuner/gateway_servehttp.go:171:		finalMode = "hls_ffmpeg_packaged_target"
internal/tuner/gateway_shared_relay.go:79:		"hls_ffmpeg",
internal/tuner/gateway_shared_relay.go:89:	return "raw_ts_ffmpeg\x1f" + strings.TrimSpace(channelID)
internal/tuner/gateway_adapt.go:267:	if strings.Contains(p, "segmenter") || strings.Contains(p, "ffmpeg") {
internal/tuner/gateway_relay.go:24:func ffmpegHLSFirstBytesTimeout() time.Duration {
internal/tuner/gateway_relay.go:50:	return exec.LookPath("ffmpeg")
internal/tuner/gateway_relay.go:61:	ffmpegPath string,
internal/tuner/gateway_relay.go:83:	cmd := exec.CommandContext(r.Context(), ffmpegPath, args...)
internal/tuner/gateway_relay.go:105:	log.Printf("gateway: channel=%q id=%s ffmpeg-ts-norm bytes=%d dur=%s",
internal/tuner/gateway_relay.go:110:func ffmpegRelayErr(phase string, err error, stderr string) error {
internal/tuner/gateway_relay.go:124:	ffmpegPath string,
internal/tuner/gateway_relay.go:145:	ffmpegPlaylistURL, ffmpegInputHost, ffmpegInputIP := canonicalizeFFmpegInputURL(r.Context(), playlistURL, g.DisableFFmpegDNS)
internal/tuner/gateway_relay.go:150:	hlsLiveStartIndex := ffmpegHLSLiveStartIndex()
internal/tuner/gateway_relay.go:153:	hlsHTTPPersistent := ffmpegHLSHTTPPersistentEnabled()
internal/tuner/gateway_relay.go:186:	if cookies := g.ffmpegCookiesOptionForURL(playlistURL); cookies != "" {
internal/tuner/gateway_relay.go:204:	if headers := g.ffmpegInputHeaderBlock(r, playlistURL, ffmpegInputHost); headers != "" {
internal/tuner/gateway_relay.go:207:	args = append(args, "-i", ffmpegPlaylistURL)
internal/tuner/gateway_relay.go:211:	cmd := exec.CommandContext(r.Context(), ffmpegPath, args...)
internal/tuner/gateway_relay.go:221:	modeLabel := "ffmpeg-remux"
internal/tuner/gateway_relay.go:223:		modeLabel = "ffmpeg-transcode"
internal/tuner/gateway_relay.go:225:	if ffmpegInputHost != "" && ffmpegInputIP != "" {
internal/tuner/gateway_relay.go:227:			reqField, channelName, channelID, modeLabel, ffmpegInputHost, ffmpegInputIP)
internal/tuner/gateway_relay.go:433:					return ffmpegRelayErr("startup-gate-prefetch", errOut, stderr.String())
internal/tuner/gateway_relay.go:441:				if err := writeBootstrapTS(r.Context(), ffmpegPath, bodyOut, channelName, channelID, bootstrapSec, profileSelection.BaseProfile); err != nil {
internal/tuner/gateway_relay.go:453:				log.Printf("gateway:%s channel=%q id=%s %s startup-gate timeout continue-ffmpeg=true", reqField, channelName, channelID, modeLabel)
internal/tuner/gateway_relay.go:462:			return ffmpegRelayErr("startup-gate-timeout", errors.New(msg), stderr.String())
internal/tuner/gateway_relay.go:473:		if timeout := ffmpegHLSFirstBytesTimeout(); timeout > 0 {
internal/tuner/gateway_relay.go:499:						errOut = errors.New("ffmpeg exited before first bytes")
internal/tuner/gateway_relay.go:501:					return ffmpegRelayErr("startup-first-bytes", errOut, stderr.String())
internal/tuner/gateway_relay.go:508:				return ffmpegRelayErr("first-bytes-timeout", errors.New("ffmpeg produced no bytes before timeout"), stderr.String())
internal/tuner/gateway_relay.go:520:		if err := writeBootstrapTS(r.Context(), ffmpegPath, bodyOut, channelName, channelID, bootstrapSec, profileSelection.BaseProfile); err != nil {
internal/tuner/gateway_relay.go:585:		return ffmpegRelayErr("copy", copyErr, stderr.String())
internal/tuner/gateway_relay.go:590:			return ffmpegRelayErr("wait", waitErr, stderr.String())
internal/tuner/gateway_relay.go:592:		return ffmpegRelayErr("wait", errors.New(msg), stderr.String())
internal/tuner/gateway_relay.go:647:		if ffmpegPath, ffmpegErr := resolveFFmpegPath(); ffmpegErr == nil {
internal/tuner/gateway_relay.go:651:				ffmpegPath,
internal/tuner/gateway_relay.go:664:				log.Printf("gateway:%s channel=%q id=%s hls-relay-ffmpeg-stdin start failed (falling back to raw relay): %v",
internal/tuner/gateway_relay.go:668:				relayLogLabel = "hls-relay-ffmpeg-stdin-feed"
internal/tuner/gateway_relay.go:669:				log.Printf("gateway:%s channel=%q id=%s hls-relay-ffmpeg-stdin enabled transcode=%t profile=%s",
internal/tuner/gateway_relay.go:673:			log.Printf("gateway:%s channel=%q id=%s hls-relay-ffmpeg-stdin ffmpeg unavailable path=%q err=%v",
internal/tuner/gateway_relay.go:674:				reqField, channelName, channelID, os.Getenv("IPTV_TUNERR_FFMPEG_PATH"), ffmpegErr)
internal/tuner/gateway_relay.go:676:			log.Printf("gateway:%s channel=%q id=%s hls-relay-ffmpeg-stdin ffmpeg unavailable transcode-requested=true err=%v", reqField, channelName, channelID, ffmpegErr)
internal/tuner/gateway_relay.go:809:							log.Printf("gateway:%s channel=%q id=%s hls-relay-ffmpeg-stdin first-feed-bytes=%d seg=%q startup=%s",
internal/tuner/gateway_hls_packager.go:25:type ffmpegHLSPackagerSession struct {
internal/tuner/gateway_hls_packager.go:44:func (s *ffmpegHLSPackagerSession) touch(now time.Time) {
internal/tuner/gateway_hls_packager.go:50:func (s *ffmpegHLSPackagerSession) markExit(err error) {
internal/tuner/gateway_hls_packager.go:57:func (s *ffmpegHLSPackagerSession) snapshot() (createdAt, lastAccess time.Time, exited bool, waitErr error) {
internal/tuner/gateway_hls_packager.go:220:	var expired []*ffmpegHLSPackagerSession
internal/tuner/gateway_hls_packager.go:240:func (g *Gateway) stopHLSPackagerSession(sess *ffmpegHLSPackagerSession, reason string) {
internal/tuner/gateway_hls_packager.go:258:func (g *Gateway) removeHLSPackagerSessionLocked(sessionID string, sess *ffmpegHLSPackagerSession) {
internal/tuner/gateway_hls_packager.go:273:func (g *Gateway) registerHLSPackagerSession(sess *ffmpegHLSPackagerSession) {
internal/tuner/gateway_hls_packager.go:280:		g.hlsPackagerSessions = make(map[string]*ffmpegHLSPackagerSession)
internal/tuner/gateway_hls_packager.go:283:		g.hlsPackagerSessionsByKey = make(map[string]*ffmpegHLSPackagerSession)
internal/tuner/gateway_hls_packager.go:298:	var sess *ffmpegHLSPackagerSession
internal/tuner/gateway_hls_packager.go:310:func (g *Gateway) lookupHLSPackagerSession(sessionID string) *ffmpegHLSPackagerSession {
internal/tuner/gateway_hls_packager.go:323:func (g *Gateway) lookupReusableHLSPackagerSession(reuseKey string) *ffmpegHLSPackagerSession {
internal/tuner/gateway_hls_packager.go:328:	var stale *ffmpegHLSPackagerSession
internal/tuner/gateway_hls_packager.go:361:		if err := os.MkdirAll(base, 0755); err != nil {
internal/tuner/gateway_hls_packager.go:372:	segPattern := filepath.Join(filepath.Dir(playlistPath), "seg-%06d.ts")
internal/tuner/gateway_hls_packager.go:391:	ffmpegPath string,
internal/tuner/gateway_hls_packager.go:396:) (*ffmpegHLSPackagerSession, error) {
internal/tuner/gateway_hls_packager.go:401:	playlistPath := filepath.Join(dir, "index.m3u8")
internal/tuner/gateway_hls_packager.go:403:	ffmpegPlaylistURL, ffmpegInputHost, ffmpegInputIP := canonicalizeFFmpegInputURL(r.Context(), playlistURL, g.DisableFFmpegDNS)
internal/tuner/gateway_hls_packager.go:407:	hlsLiveStartIndex := ffmpegHLSLiveStartIndex()
internal/tuner/gateway_hls_packager.go:409:	hlsHTTPPersistent := ffmpegHLSHTTPPersistentEnabled()
internal/tuner/gateway_hls_packager.go:437:	if cookies := g.ffmpegCookiesOptionForURL(playlistURL); cookies != "" {
internal/tuner/gateway_hls_packager.go:452:	if headers := g.ffmpegInputHeaderBlock(r, playlistURL, ffmpegInputHost); headers != "" {
internal/tuner/gateway_hls_packager.go:455:	args = append(args, "-i", ffmpegPlaylistURL)
internal/tuner/gateway_hls_packager.go:457:	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
internal/tuner/gateway_hls_packager.go:465:	sess := &ffmpegHLSPackagerSession{
internal/tuner/gateway_hls_packager.go:476:		segmentGlobs: []string{filepath.Join(dir, "seg-*.ts"), filepath.Join(dir, "seg-*.tmp")},
internal/tuner/gateway_hls_packager.go:491:	if ffmpegInputHost != "" && ffmpegInputIP != "" {
internal/tuner/gateway_hls_packager.go:492:		log.Printf("gateway: channel=%q id=%s hls-packager input-host-resolved %q=>%q", channelName, channelID, ffmpegInputHost, ffmpegInputIP)
internal/tuner/gateway_hls_packager.go:497:func (g *Gateway) serveFFmpegPackagedHLSPlaylist(w http.ResponseWriter, channelID string, sess *ffmpegHLSPackagerSession, shared bool) error {
internal/tuner/gateway_hls_packager.go:505:	body, err := os.ReadFile(sess.playlistPath)
internal/tuner/gateway_hls_packager.go:512:		w.Header().Set("X-IptvTunerr-Shared-Upstream", "ffmpeg_hls_packager")
internal/tuner/gateway_hls_packager.go:539:func packagedHLSFilePath(sess *ffmpegHLSPackagerSession, file string) (string, error) {
internal/tuner/gateway_hls_packager.go:547:	clean := strings.TrimPrefix(filepath.Clean("/"+name), "/")
internal/tuner/gateway_hls_packager.go:551:	full := filepath.Join(sess.dir, filepath.FromSlash(clean))
internal/tuner/gateway_hls_packager.go:567:	ffmpegPath, err := resolveFFmpegPath()
internal/tuner/gateway_hls_packager.go:571:	sess, err := g.startFFmpegPackagedHLS(r, ffmpegPath, playlistURL, channelName, channelID, profile)
internal/tuner/gateway_hls_packager.go:612:		body, err := os.ReadFile(filePath)
internal/tuner/recording_rules_test.go:17:	path := filepath.Join(t.TempDir(), "rules.json")
internal/tuner/recording_rules_test.go:46:	path := filepath.Join(t.TempDir(), "rules.json")
internal/tuner/recording_rules_test.go:63:	data, err := os.ReadFile(path)
internal/tuner/recording_rules_test.go:117:	stateFile := filepath.Join(dir, "recorder-state.json")
internal/tuner/recording_rules_test.go:124:			PublishedPath: filepath.Join(dir, "news.ts"),
internal/tuner/recording_rules_test.go:135:	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
internal/tuner/server.go:1895:			cfLearnedPath = filepath.Join(dir, "cf-learned.json")
internal/tuner/server.go:1923:			accountLimitPath = filepath.Join(dir, "provider-account-limits.json")
internal/tuner/server.go:1979:		log.Printf("Gateway ffmpeg relay disabled by config")
internal/tuner/server.go:1982:		log.Printf("Gateway ffmpeg input DNS rewrite disabled")
internal/tuner/server.go:2001:			log.Printf("Gateway detected ffmpeg Lavf UA: %s", gateway.DetectedFFmpegUA)
internal/webui/webui_migration.go:234:	planData, err := os.ReadFile(planPath)
internal/webui/webui_migration.go:362:	planData, err := os.ReadFile(planPath)
internal/tuner/server_test.go:359:	path := filepath.Join(t.TempDir(), "programming.json")
internal/tuner/server_test.go:360:	if err := os.WriteFile(path, []byte(`{
internal/tuner/server_test.go:380:	path := filepath.Join(t.TempDir(), "programming.json")
internal/tuner/server_test.go:671:	path := filepath.Join(t.TempDir(), "harvest.json")
internal/tuner/server_test.go:719:	path := filepath.Join(t.TempDir(), "harvest.json")
internal/tuner/server_test.go:817:	recipePath := filepath.Join(t.TempDir(), "programming.json")
internal/tuner/server_test.go:818:	harvestPath := filepath.Join(t.TempDir(), "harvest.json")
internal/tuner/server_test.go:851:	recipePath := filepath.Join(t.TempDir(), "programming.json")
internal/tuner/server_test.go:852:	harvestPath := filepath.Join(t.TempDir(), "harvest.json")
internal/tuner/server_test.go:949:	path := filepath.Join(t.TempDir(), "virtual-channels.json")
internal/tuner/server_test.go:1476:	path := filepath.Join(t.TempDir(), "virtual-channels.json")
internal/tuner/server_test.go:1523:	path := filepath.Join(t.TempDir(), "virtual-channels.json")
internal/tuner/server_test.go:1570:	path := filepath.Join(t.TempDir(), "virtual-channels.json")
internal/tuner/server_test.go:1617:	path := filepath.Join(t.TempDir(), "virtual-channels.json")
internal/tuner/server_test.go:1690:	path := filepath.Join(t.TempDir(), "virtual-channels.json")
internal/tuner/server_test.go:1806:	path := filepath.Join(t.TempDir(), "virtual-channels.json")
internal/tuner/server_test.go:1898:	path := filepath.Join(t.TempDir(), "virtual-channels.json")
internal/tuner/server_test.go:2002:	path := filepath.Join(t.TempDir(), "virtual-channels.json")
internal/tuner/server_test.go:2104:	path := filepath.Join(t.TempDir(), "virtual-channels.json")
internal/tuner/server_test.go:2196:	path := filepath.Join(t.TempDir(), "virtual-recovery-state.json")
internal/tuner/server_test.go:2204:	data, err := os.ReadFile(path)
internal/tuner/server_test.go:2235:	path := filepath.Join(t.TempDir(), "virtual-recovery-state.json")
internal/tuner/server_test.go:2236:	if err := os.WriteFile(path, []byte(`{
internal/tuner/server_test.go:2271:	ffmpegPath := filepath.Join(t.TempDir(), "fake-ffmpeg.sh")
internal/tuner/server_test.go:2272:	if err := os.WriteFile(ffmpegPath, []byte(`#!/bin/sh
internal/tuner/server_test.go:2283:		t.Fatalf("write fake ffmpeg: %v", err)
internal/tuner/server_test.go:2285:	t.Setenv("IPTV_TUNERR_FFMPEG_PATH", ffmpegPath)
internal/tuner/server_test.go:2301:	path := filepath.Join(t.TempDir(), "virtual-channels.json")
internal/tuner/server_test.go:2375:	ffmpegPath := filepath.Join(t.TempDir(), "fake-ffmpeg.sh")
internal/tuner/server_test.go:2376:	if err := os.WriteFile(ffmpegPath, []byte(`#!/bin/sh
internal/tuner/server_test.go:2389:		t.Fatalf("write fake ffmpeg: %v", err)
internal/tuner/server_test.go:2391:	t.Setenv("IPTV_TUNERR_FFMPEG_PATH", ffmpegPath)
internal/tuner/server_test.go:2416:	path := filepath.Join(t.TempDir(), "virtual-channels.json")
internal/tuner/server_test.go:2484:	ffmpegPath := filepath.Join(t.TempDir(), "fake-ffmpeg.sh")
internal/tuner/server_test.go:2485:	if err := os.WriteFile(ffmpegPath, []byte(`#!/bin/sh
internal/tuner/server_test.go:2498:		t.Fatalf("write fake ffmpeg: %v", err)
internal/tuner/server_test.go:2500:	t.Setenv("IPTV_TUNERR_FFMPEG_PATH", ffmpegPath)
internal/tuner/server_test.go:2530:	path := filepath.Join(t.TempDir(), "virtual-channels.json")
internal/tuner/server_test.go:2597:	path := filepath.Join(t.TempDir(), "virtual-channels.json")
internal/tuner/server_test.go:2645:	argsPath := filepath.Join(t.TempDir(), "ffmpeg-args.txt")
internal/tuner/server_test.go:2646:	ffmpegPath := filepath.Join(t.TempDir(), "fake-ffmpeg.sh")
internal/tuner/server_test.go:2647:	if err := os.WriteFile(ffmpegPath, []byte("#!/bin/sh\nprintf '%s\n' \"$@\" > \""+argsPath+"\"\ncat >/dev/null\nprintf 'branded-output'\n"), 0o755); err != nil {
internal/tuner/server_test.go:2648:		t.Fatalf("write fake ffmpeg: %v", err)
internal/tuner/server_test.go:2650:	t.Setenv("IPTV_TUNERR_FFMPEG_PATH", ffmpegPath)
internal/tuner/server_test.go:2651:	bugImagePath := filepath.Join(t.TempDir(), "bug.png")
internal/tuner/server_test.go:2652:	if err := os.WriteFile(bugImagePath, []byte("fakepng"), 0o600); err != nil {
internal/tuner/server_test.go:2662:	path := filepath.Join(t.TempDir(), "virtual-channels.json")
internal/tuner/server_test.go:2712:	argsRaw, err := os.ReadFile(argsPath)
internal/tuner/server_test.go:2714:		t.Fatalf("read ffmpeg args: %v", err)
internal/tuner/server_test.go:2718:		t.Fatalf("ffmpeg args missing bug image: %s", argsText)
internal/tuner/server_test.go:2721:		t.Fatalf("ffmpeg args missing filter_complex: %s", argsText)
internal/tuner/server_test.go:2852:	if err := os.MkdirAll(filepath.Join(".diag", "channel-diff", "run-a"), 0o755); err != nil {
internal/tuner/server_test.go:2855:	if err := os.WriteFile(filepath.Join(".diag", "channel-diff", "run-a", "report.json"), []byte(`{
internal/tuner/server_test.go:2918:	evidenceDir := filepath.Join(tmp, ".diag", "evidence", "smoke-case")
internal/tuner/server_test.go:2925:		if info, err := os.Stat(filepath.Join(evidenceDir, sub)); err != nil {
internal/tuner/server_test.go:2931:	if _, err := os.Stat(filepath.Join(evidenceDir, "notes.md")); err != nil {
internal/tuner/server_test.go:2957:	expectedPrefix := filepath.Join(".diag", "evidence") + string(os.PathSeparator)
internal/tuner/server_test.go:2961:	if _, err := os.Stat(filepath.Join(gotOutputDir, "notes.md")); err != nil {
internal/tuner/server_test.go:2994:	if err := os.MkdirAll("scripts", 0o755); err != nil {
internal/tuner/server_test.go:2997:	script := filepath.Join("scripts", "channel-diff-harness.sh")
internal/tuner/server_test.go:2998:	if err := os.WriteFile(script, []byte(`#!/usr/bin/env bash
internal/tuner/server_test.go:3030:	if err := os.WriteFile("escape.sh", []byte("#!/bin/sh\nexit 42\n"), 0o700); err != nil {
internal/tuner/server_test.go:3123:	path := filepath.Join(t.TempDir(), "programming.json")
internal/tuner/server_test.go:3124:	if err := os.WriteFile(path, []byte(`{
internal/tuner/server_test.go:3153:	path := filepath.Join(t.TempDir(), "programming.json")
internal/tuner/server_test.go:3208:	path := filepath.Join(t.TempDir(), "programming.json")
internal/tuner/server_test.go:3209:	if err := os.WriteFile(path, []byte(`{
internal/tuner/server_test.go:3222:	if err := os.WriteFile(path, []byte(`{
internal/tuner/server_test.go:3247:	path := filepath.Join(t.TempDir(), "programming.json")
internal/tuner/server_test.go:3248:	if err := os.WriteFile(path, []byte(`{"selected_categories":["iptv--news"]}`), 0o600); err != nil {
internal/tuner/server_test.go:3271:	path := filepath.Join(t.TempDir(), "programming.json")
internal/tuner/server_test.go:3272:	if err := os.WriteFile(path, []byte(`{"selected_categories":["iptv--news"]}`), 0o600); err != nil {
internal/tuner/server_test.go:3689:	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
internal/tuner/server_test.go:3723:		FinalMode:    "hls_ffmpeg",
internal/tuner/server_test.go:4110:	dbPath := filepath.Join(dir, "epg.db")
internal/tuner/server_test.go:4145:	dbPath := filepath.Join(dir, "epg.db")
internal/tuner/server_test.go:4795:	path := filepath.Join(t.TempDir(), "autopilot.json")
internal/tuner/server_test.go:5008:	stateFile := filepath.Join(dir, "recorder-state.json")
internal/tuner/server_test.go:5020:	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
internal/tuner/server_test.go:5869:	ffmpegPath := filepath.Join(t.TempDir(), "fake-ffmpeg.sh")
internal/tuner/server_test.go:5870:	if err := os.WriteFile(ffmpegPath, []byte(`#!/bin/sh
internal/tuner/server_test.go:5877:		t.Fatalf("write fake ffmpeg: %v", err)
internal/tuner/server_test.go:5879:	t.Setenv("IPTV_TUNERR_FFMPEG_PATH", ffmpegPath)
internal/tuner/server_test.go:6293:	cfgPath := filepath.Join(t.TempDir(), "eventhooks.json")
internal/tuner/server_test.go:6294:	if err := os.WriteFile(cfgPath, []byte(`{"webhooks":[{"name":"test","url":"`+webhook.URL+`","events":["lineup.updated"]}]}`), 0o644); err != nil {
internal/tuner/server_test.go:6782:	usersPath := filepath.Join(t.TempDir(), "xtream-users.json")
internal/tuner/server_test.go:7246:	usersPath := filepath.Join(t.TempDir(), "xtream-users.json")
internal/tuner/server_test.go:7339:	usersPath := filepath.Join(t.TempDir(), "xtream-users.json")
internal/tuner/server_test.go:7372:	path := filepath.Join(t.TempDir(), "harvest.json")
internal/tuner/server_test.go:7399:	stateFile := filepath.Join(t.TempDir(), "recorder-state.json")
internal/tuner/server_test.go:7401:	if err := os.WriteFile(stateFile, data, 0o600); err != nil {
internal/tuner/server_test.go:7424:	path := filepath.Join(t.TempDir(), "virtual.json")
internal/tuner/server_test.go:7450:	path := filepath.Join(t.TempDir(), "virtual.json")
internal/tuner/server_test.go:7481:	path := filepath.Join(t.TempDir(), "recording-rules.json")
internal/tuner/server_operator_workflows.go:1048:		outDir := filepath.Join(repoDiagRoot(), "evidence", caseID)
internal/tuner/epg_pipeline.go:545:	b, err := os.ReadFile(path)
internal/tuner/epg_pipeline.go:570:	if err := os.MkdirAll(dir, 0o700); err != nil {
internal/tuner/epg_pipeline.go:580:	tmp, err := os.CreateTemp(dir, ".provider-epg-*.tmp")
internal/tuner/gateway_test.go:276:	want, err := os.ReadFile("testdata/hls_mux_small_playlist.golden")
internal/tuner/gateway_test.go:291:	upstream, err := os.ReadFile("testdata/stream_compare_hls_mux_capture_upstream.m3u8")
internal/tuner/gateway_test.go:295:	want, err := os.ReadFile("testdata/stream_compare_hls_mux_capture_tunerr_expected.m3u8")
internal/tuner/gateway_test.go:312:	upstream, err := os.ReadFile("testdata/stream_compare_dash_mux_capture_upstream.mpd")
internal/tuner/gateway_test.go:316:	want, err := os.ReadFile("testdata/stream_compare_dash_mux_capture_tunerr_expected.mpd")
internal/tuner/gateway_test.go:1334:		t.Fatal("expected Peacock TS to bypass ffmpeg")
internal/tuner/gateway_test.go:1337:		t.Fatal("did not expect Peacock HLS to bypass ffmpeg")
internal/tuner/gateway_test.go:1340:		t.Fatal("did not expect non-Peacock TS to bypass ffmpeg")
internal/tuner/gateway_test.go:1487:	path := filepath.Join(t.TempDir(), "host-policy.json")
internal/tuner/gateway_test.go:1488:	if err := os.WriteFile(path, []byte(`{"global_blocked_hosts":["bad.example"]}`), 0o600); err != nil {
internal/tuner/gateway_test.go:1505:	path := filepath.Join(t.TempDir(), "host-policy.json")
internal/tuner/gateway_test.go:1506:	if err := os.WriteFile(path, []byte(`{"global_preferred_hosts":["cdn.file.example"]}`), 0o600); err != nil {
internal/tuner/gateway_test.go:2140:	lockPath := filepath.Join(dir, mgr.lockFilename(identity.Key))
internal/tuner/gateway_test.go:2150:	target := filepath.Join(dir, "target.json")
internal/tuner/gateway_test.go:2151:	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
internal/tuner/gateway_test.go:2154:	link := filepath.Join(dir, "lease-link.json")
internal/tuner/gateway_test.go:2161:	if got, err := os.ReadFile(target); err != nil {
internal/tuner/gateway_test.go:2172:	target := filepath.Join(dir, "target.lock")
internal/tuner/gateway_test.go:2173:	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
internal/tuner/gateway_test.go:2176:	lockPath := filepath.Join(dir, mgr.lockFilename(key))
internal/tuner/gateway_test.go:2185:	if got, err := os.ReadFile(target); err != nil {
internal/tuner/gateway_test.go:2226:	ffmpegPath := filepath.Join(dir, "fake-ffmpeg.sh")
internal/tuner/gateway_test.go:2230:printf 'ffmpeg'
internal/tuner/gateway_test.go:2232:	if err := os.WriteFile(ffmpegPath, []byte(script), 0755); err != nil {
internal/tuner/gateway_test.go:2235:	t.Setenv("IPTV_TUNERR_FFMPEG_PATH", ffmpegPath)
internal/tuner/gateway_test.go:2265:		if rep.Count == 1 && len(rep.Relays) == 1 && rep.Relays[0].SharedUpstream == "hls_ffmpeg" {
internal/tuner/gateway_test.go:2271:	if rep.Count != 1 || len(rep.Relays) != 1 || rep.Relays[0].SharedUpstream != "hls_ffmpeg" {
internal/tuner/gateway_test.go:2272:		t.Fatalf("expected live shared ffmpeg relay, got %#v", rep)
internal/tuner/gateway_test.go:2286:	if got := secondRec.Header().Get("X-IptvTunerr-Shared-Upstream"); got != "hls_ffmpeg" {
internal/tuner/gateway_test.go:2299:	ffmpegPath := filepath.Join(dir, "fake-ffmpeg.sh")
internal/tuner/gateway_test.go:2305:	if err := os.WriteFile(ffmpegPath, []byte(script), 0755); err != nil {
internal/tuner/gateway_test.go:2308:	t.Setenv("IPTV_TUNERR_FFMPEG_PATH", ffmpegPath)
internal/tuner/gateway_test.go:2346:		if rep.Count == 1 && len(rep.Relays) == 1 && rep.Relays[0].SharedUpstream == "hls_ffmpeg" && rep.Relays[0].ContentType == "video/mp4" {
internal/tuner/gateway_test.go:2352:	if rep.Count != 1 || len(rep.Relays) != 1 || rep.Relays[0].SharedUpstream != "hls_ffmpeg" || rep.Relays[0].ContentType != "video/mp4" {
internal/tuner/gateway_test.go:2353:		t.Fatalf("expected live shared ffmpeg fmp4 relay, got %#v", rep)
internal/tuner/gateway_test.go:2367:	if got := secondRec.Header().Get("X-IptvTunerr-Shared-Upstream"); got != "hls_ffmpeg" {
internal/tuner/gateway_test.go:2555:		g.noteUpstreamFailure("http://provider.example/live/1.m3u8", 0, "ffmpeg_hls_failed")
internal/tuner/gateway_test.go:2563:		g.noteUpstreamFailure("http://provider.example/live/1.m3u8", 0, "ffmpeg_hls_failed")
internal/tuner/gateway_test.go:3822:func TestGateway_ffmpegInputHeaderBlock_includesForwardedHeaders(t *testing.T) {
internal/tuner/gateway_test.go:3829:	block := g.ffmpegInputHeaderBlock(req, "http://cdn.example/live/u/p/1.m3u8", "cdn.example")
internal/tuner/gateway_test.go:3991:func TestGateway_ffmpegInputHeaderBlock_includesCustomHeaders(t *testing.T) {
internal/tuner/gateway_test.go:3999:	block := g.ffmpegInputHeaderBlock(nil, "http://cdn.example/live/u/p/1.m3u8", "cdn.example")
internal/tuner/gateway_test.go:4064:	dir := filepath.Join(t.TempDir(), "tee")
internal/tuner/gateway_test.go:4144:func TestGateway_ffmpegInputHeaderBlock_stillIncludesCredentialHeaders(t *testing.T) {
internal/tuner/gateway_test.go:4155:	block := g.ffmpegInputHeaderBlock(incoming, "http://provider.example/plain.m3u8", "provider.example")
internal/tuner/gateway_test.go:4163:			t.Fatalf("ffmpeg header block missing %q in:\n%s", want, block)
internal/tuner/gateway_test.go:4168:func TestGateway_ffmpegInputHeaderBlock_customHostOverridesResolvedHost(t *testing.T) {
internal/tuner/gateway_test.go:4174:	block := g.ffmpegInputHeaderBlock(nil, "http://resolved.example/live/u/p/1.m3u8", "resolved.example")
internal/tuner/gateway_test.go:4219:		t.Skip("ffmpeg not installed; skipping cross-host HLS relay integration test")
internal/tuner/gateway_test.go:4312:		t.Fatalf("ffmpeg attempted cross-host segment with stale Host header badSegmentHost=%d", bad)
internal/tuner/gateway_test.go:4318:	ffmpegPath := filepath.Join(dir, "fake-ffmpeg.sh")
internal/tuner/gateway_test.go:4323:	if err := os.WriteFile(ffmpegPath, []byte(script), 0755); err != nil {
internal/tuner/gateway_test.go:4336:		ffmpegPath,
internal/tuner/gateway_test.go:4363:	ffmpegPath := filepath.Join(dir, "fake-ffmpeg.sh")
internal/tuner/gateway_test.go:4368:	if err := os.WriteFile(ffmpegPath, []byte(script), 0755); err != nil {
internal/tuner/gateway_test.go:4384:		ffmpegPath,
internal/tuner/gateway_test.go:4411:	ffmpegPath := filepath.Join(dir, "fake-ffmpeg.sh")
internal/tuner/gateway_test.go:4416:	if err := os.WriteFile(ffmpegPath, []byte(script), 0755); err != nil {
internal/tuner/gateway_test.go:4443:	t.Setenv("IPTV_TUNERR_FFMPEG_PATH", ffmpegPath)
internal/tuner/gateway_test.go:5085:	data, err := os.ReadFile(path)
internal/tuner/gateway_test.go:5112:func TestGateway_ffmpegInputHeaderBlock_usesPerStreamAuthAndCookies(t *testing.T) {
internal/tuner/gateway_test.go:5137:	block := g.ffmpegInputHeaderBlock(req, playlistURL, "provider2.example")
internal/tuner/gateway_test.go:5146:func TestGateway_ffmpegCookiesOptionForURL(t *testing.T) {
internal/tuner/gateway_test.go:5158:	got := g.ffmpegCookiesOptionForURL(playlistURL)
internal/tuner/gateway_test.go:5180:	cfgPath := filepath.Join(t.TempDir(), "hooks.json")
internal/tuner/gateway_test.go:5181:	if err := os.WriteFile(cfgPath, []byte(`{"webhooks":[{"name":"test","url":"`+webhook.URL+`","events":["stream.requested","stream.rejected","stream.finished"]}]}`), 0o644); err != nil {
internal/webui/static/dist/assets/index-C5KHYVYH.js:452:`;function oX({opened:n,onClose:e,initial:t}){const s=cn(),r=!!t,[i,o]=A.useState((t==null?void 0:t.name)??""),[c,u]=A.useState((t==null?void 0:t.kind)??"webhook"),[d,m]=A.useState((t==null?void 0:t.target)??""),[g,p]=A.useState((t==null?void 0:t.event_types)??[]),[y,x]=A.useState((t==null?void 0:t.enabled)??!0),[b,T]=A.useState(!1);function R(){o(""),u("webhook"),m(""),p([]),x(!0),T(!1)}const w=dt({mutationFn:()=>{const C={name:i,kind:c,target:d,event_types:g,enabled:y};return r?Sg.update(t.id,C):Sg.create(C)},onSuccess:()=>{s.invalidateQueries({queryKey:["connections"]}),Oe.show({message:r?"Connection updated":"Connection created",color:"teal"}),R(),e()},onError:C=>Oe.show({message:C.message,color:"red"})});return f.jsx(Sn,{opened:n,onClose:()=>{R(),e()},title:r?`Edit — ${t==null?void 0:t.name}`:"New Connection",size:"md",children:f.jsxs(Je,{gap:"sm",children:[f.jsx(Lt,{label:"Name",value:i,onChange:C=>o(C.currentTarget.value),required:!0}),f.jsx(wn,{label:"Kind",data:[{value:"webhook",label:"Webhook (HTTP POST)"},{value:"script",label:"Script (shell)"}],value:c,onChange:C=>u(C??"webhook")}),f.jsx(Lt,{label:c==="script"?"Script path":"URL",value:d,onChange:C=>m(C.currentTarget.value),placeholder:c==="script"?"/state/scripts/notify.sh":"https://hooks.example.com/…",required:!0}),f.jsx(If,{label:"Event types (empty = all)",data:iX,value:g,onChange:p,placeholder:"All events",clearable:!0}),f.jsx(Is,{label:"Enabled",checked:y,onChange:C=>x(C.currentTarget.checked)}),c==="script"&&f.jsxs(f.Fragment,{children:[f.jsx(Ve,{size:"xs",variant:"subtle",onClick:()=>T(C=>!C),children:b?"Hide template":"Show starter script"}),f.jsx(Ig,{in:b,children:f.jsx(ji,{block:!0,fz:"xs",style:{whiteSpace:"pre"},children:aX})})]}),f.jsxs(_e,{justify:"flex-end",mt:"sm",children:[f.jsx(Ve,{variant:"default",onClick:()=>{R(),e()},children:"Cancel"}),f.jsx(Ve,{color:"teal",loading:w.isPending,onClick:()=>w.mutate(),children:r?"Save":"Create"})]})]})})}function lX(){const n=cn(),[e,t]=A.useState(!1),[s,r]=A.useState(null),{data:i=[],isLoading:o}=ht({queryKey:["connections"],queryFn:()=>Sg.list()}),c=dt({mutationFn:u=>Sg.delete(u),onSuccess:()=>n.invalidateQueries({queryKey:["connections"]}),onError:u=>Oe.show({message:u.message,color:"red"})});return f.jsxs(f.Fragment,{children:[f.jsxs(_e,{justify:"space-between",mb:"md",children:[f.jsx(Q,{fw:500,children:"Event Connections"}),f.jsx(Ve,{size:"xs",leftSection:f.jsx(Ma,{size:14}),color:"teal",onClick:()=>{r(null),t(!0)},children:"New Connection"})]}),o?f.jsx(Q,{size:"sm",c:"dimmed",children:"Loading…"}):i.length===0?f.jsx(jt,{icon:f.jsx(lM,{size:16}),color:"gray",children:"No connections yet. Wire up webhooks or scripts to react to stream and guide events."}):f.jsx(gn,{children:f.jsxs(O,{striped:!0,highlightOnHover:!0,withRowBorders:!1,fz:"sm",children:[f.jsx(O.Thead,{children:f.jsxs(O.Tr,{children:[f.jsx(O.Th,{children:"Name"}),f.jsx(O.Th,{children:"Kind"}),f.jsx(O.Th,{children:"Target"}),f.jsx(O.Th,{children:"Events"}),f.jsx(O.Th,{children:"Status"}),f.jsx(O.Th,{style:{width:80}})]})}),f.jsx(O.Tbody,{children:i.map(u=>f.jsxs(O.Tr,{children:[f.jsx(O.Td,{children:f.jsx(Q,{size:"sm",children:u.name})}),f.jsx(O.Td,{children:f.jsx(Vt,{size:"xs",color:u.kind==="script"?"grape":"blue",variant:"outline",children:u.kind})}),f.jsx(O.Td,{children:f.jsx(Q,{size:"xs",c:"dimmed",lineClamp:1,maw:220,children:u.target})}),f.jsx(O.Td,{children:f.jsx(Q,{size:"xs",c:"dimmed",children:u.event_types.length===0?"All":u.event_types.join(", ")})}),f.jsx(O.Td,{children:f.jsx(Vt,{size:"xs",color:u.enabled?"teal":"gray",children:u.enabled?"Active":"Disabled"})}),f.jsx(O.Td,{children:f.jsxs(_e,{gap:4,wrap:"nowrap",children:[f.jsx(St,{label:"Edit",children:f.jsx(bt,{size:"xs",variant:"subtle",color:"yellow",onClick:()=>{r(u),t(!0)},children:f.jsx(Gi,{size:14})})}),f.jsx(St,{label:"Delete",children:f.jsx(bt,{size:"xs",variant:"subtle",color:"red",onClick:()=>{confirm(`Delete "${u.name}"?`)&&c.mutate(u.id)},children:f.jsx(Vi,{size:14})})})]})})]},u.id))})]})}),f.jsx(oX,{opened:e,onClose:()=>{t(!1),r(null)},initial:s})]})}function cX(){const n=cn(),e=ht({queryKey:["provider-profile"],queryFn:()=>we.get("/api/provider/profile.json"),staleTime:3e4}),t=ht({queryKey:["shared-relays"],queryFn:()=>we.get("/api/debug/shared-relays.json"),staleTime:3e4}),s=ht({queryKey:["stream-attempts"],queryFn:()=>we.get("/api/debug/stream-attempts.json?limit=20"),staleTime:3e4}),r=dt({mutationFn:()=>we.post("/api/ops/actions/stream-attempts-clear"),onSuccess:()=>{n.invalidateQueries({queryKey:["stream-attempts"]}),Oe.show({message:"Attempt history cleared",color:"teal"})},onError:d=>Oe.show({message:d.message,color:"red"})}),i=dt({mutationFn:()=>we.post("/api/ops/actions/provider-profile-reset"),onSuccess:()=>{n.invalidateQueries({queryKey:["provider-profile"]}),Oe.show({message:"Provider penalties reset",color:"teal"})},onError:d=>Oe.show({message:d.message,color:"red"})}),o=e.data,c=t.data,u=s.data;return f.jsx(gn,{children:f.jsxs(Je,{gap:"md",children:[f.jsxs(ft,{withBorder:!0,p:"md",children:[f.jsxs(_e,{justify:"space-between",mb:"xs",children:[f.jsx(Q,{fw:600,children:"Provider Profile"}),f.jsx(Ve,{size:"xs",color:"orange",variant:"outline",onClick:()=>{confirm("Reset provider penalties?")&&i.mutate()},loading:i.isPending,children:"Reset Penalties"})]}),e.isLoading?f.jsx(Q,{size:"sm",c:"dimmed",children:"Loading…"}):e.isError?f.jsx(jt,{icon:f.jsx(Zt,{size:16}),color:"gray",children:"Provider profile unavailable."}):f.jsx(O,{withRowBorders:!1,fz:"sm",children:f.jsxs(O.Tbody,{children:[f.jsxs(O.Tr,{children:[f.jsx(O.Td,{c:"dimmed",w:220,children:"Effective tuner limit"}),f.jsx(O.Td,{children:f.jsx(Vt,{size:"sm",color:"teal",children:String((o==null?void 0:o.effective_tuner_limit)??"—")})})]}),f.jsxs(O.Tr,{children:[f.jsx(O.Td,{c:"dimmed",children:"Learned tuner limit"}),f.jsx(O.Td,{children:String((o==null?void 0:o.learned_tuner_limit)??"—")})]}),f.jsxs(O.Tr,{children:[f.jsx(O.Td,{c:"dimmed",children:"Penalized hosts"}),f.jsx(O.Td,{children:Array.isArray(o==null?void 0:o.penalized_hosts)?o.penalized_hosts.length:"0"})]}),f.jsxs(O.Tr,{children:[f.jsx(O.Td,{c:"dimmed",children:"CF block hits"}),f.jsx(O.Td,{children:String((o==null?void 0:o.cf_block_hits)??"0")})]}),f.jsxs(O.Tr,{children:[f.jsx(O.Td,{c:"dimmed",children:"Concurrency signals"}),f.jsx(O.Td,{children:String((o==null?void 0:o.concurrency_signals_seen)??"0")})]})]})}),o&&Array.isArray(o.remediation_hints)&&o.remediation_hints.length>0&&f.jsxs(he,{mt:"xs",children:[f.jsx(Q,{size:"xs",c:"dimmed",mb:4,children:"Remediation hints:"}),o.remediation_hints.map((d,m)=>f.jsx(jt,{color:"yellow",p:"xs",mb:4,children:f.jsx(Q,{size:"xs",children:d})},m))]})]}),f.jsxs(ft,{withBorder:!0,p:"md",children:[f.jsx(Q,{fw:600,mb:"xs",children:"Shared Relays"}),t.isLoading?f.jsx(Q,{size:"sm",c:"dimmed",children:"Loading…"}):t.isError?f.jsx(jt,{icon:f.jsx(Zt,{size:16}),color:"gray",children:"Relay info unavailable."}):f.jsxs(_e,{gap:"xl",children:[f.jsxs(he,{children:[f.jsx(Q,{size:"xs",c:"dimmed",children:"Active relays"}),f.jsx(Q,{fw:500,children:String((c==null?void 0:c.relay_count)??(c==null?void 0:c.count)??"—")})]}),f.jsxs(he,{children:[f.jsx(Q,{size:"xs",c:"dimmed",children:"Total subscribers"}),f.jsx(Q,{fw:500,children:String((c==null?void 0:c.subscriber_total)??(c==null?void 0:c.subscribers)??"—")})]})]})]}),f.jsxs(ft,{withBorder:!0,p:"md",children:[f.jsxs(_e,{justify:"space-between",mb:"xs",children:[f.jsx(Q,{fw:600,children:"Recent Stream Attempts"}),f.jsx(Ve,{size:"xs",color:"red",variant:"outline",onClick:()=>{confirm("Clear attempt history?")&&r.mutate()},loading:r.isPending,children:"Clear History"})]}),s.isLoading?f.jsx(Q,{size:"sm",c:"dimmed",children:"Loading…"}):s.isError?f.jsx(jt,{icon:f.jsx(Zt,{size:16}),color:"gray",children:"Attempt log unavailable."}):(()=>{const d=Array.isArray(u)?u:Array.isArray(u==null?void 0:u.attempts)?u.attempts:[];return d.length===0?f.jsx(Q,{size:"sm",c:"dimmed",children:"No recent attempts."}):f.jsxs(O,{withRowBorders:!1,fz:"xs",striped:!0,children:[f.jsx(O.Thead,{children:f.jsxs(O.Tr,{children:[f.jsx(O.Th,{children:"Channel"}),f.jsx(O.Th,{children:"Outcome"}),f.jsx(O.Th,{children:"When"})]})}),f.jsx(O.Tbody,{children:d.slice(0,20).map((m,g)=>f.jsxs(O.Tr,{children:[f.jsx(O.Td,{children:String(m.channel_name??m.channel_id??"—")}),f.jsx(O.Td,{children:f.jsx(Vt,{size:"xs",color:String(m.outcome??m.result??"ok")==="ok"?"teal":"red",children:String(m.outcome??m.result??"—")})}),f.jsx(O.Td,{c:"dimmed",children:m.at?new Date(String(m.at)).toLocaleTimeString():"—"})]},g))})]})})()]})]})})}function uX(){const n=cn(),e=ht({queryKey:["autopilot-report"],queryFn:()=>we.get("/api/autopilot/report.json?limit=8"),staleTime:3e4}),t=dt({mutationFn:()=>we.post("/api/ops/actions/autopilot-reset"),onSuccess:()=>{n.invalidateQueries({queryKey:["autopilot-report"]}),Oe.show({message:"Autopilot memory reset",color:"teal"})},onError:i=>Oe.show({message:i.message,color:"red"})}),s=e.data,r=Array.isArray(s==null?void 0:s.hot_channels)?s.hot_channels:[];return f.jsx(Je,{gap:"md",children:f.jsxs(ft,{withBorder:!0,p:"md",children:[f.jsxs(_e,{justify:"space-between",mb:"xs",children:[f.jsx(Q,{fw:600,children:"Autopilot Report"}),f.jsx(Ve,{size:"xs",color:"orange",variant:"outline",onClick:()=>{confirm("Reset autopilot memory? This will clear learned channel routing.")&&t.mutate()},loading:t.isPending,children:"Reset Autopilot Memory"})]}),e.isLoading?f.jsx(Q,{size:"sm",c:"dimmed",children:"Loading…"}):e.isError?f.jsx(jt,{icon:f.jsx(Zt,{size:16}),color:"gray",children:"Autopilot report unavailable."}):f.jsxs(Je,{gap:"sm",children:[f.jsxs(_e,{gap:"xl",children:[f.jsxs(he,{children:[f.jsx(Q,{size:"xs",c:"dimmed",children:"Decisions made"}),f.jsx(Q,{fw:500,children:String((s==null?void 0:s.decision_count)??"—")})]}),!!(s!=null&&s.consensus_host)&&f.jsxs(he,{children:[f.jsx(Q,{size:"xs",c:"dimmed",children:"Consensus host"}),f.jsx(ji,{children:String(s.consensus_host)})]}),(s==null?void 0:s.consensus_dna_count)!==void 0&&f.jsxs(he,{children:[f.jsx(Q,{size:"xs",c:"dimmed",children:"DNA samples"}),f.jsx(Q,{fw:500,children:String(s.consensus_dna_count)})]})]}),r.length>0&&f.jsxs(f.Fragment,{children:[f.jsx(Jn,{label:"Hot Channels"}),f.jsxs(O,{withRowBorders:!1,fz:"sm",striped:!0,children:[f.jsx(O.Thead,{children:f.jsxs(O.Tr,{children:[f.jsx(O.Th,{children:"Channel"}),f.jsx(O.Th,{children:"Score"})]})}),f.jsx(O.Tbody,{children:r.map((i,o)=>f.jsxs(O.Tr,{children:[f.jsx(O.Td,{children:String(i.name??i.channel_name??"—")}),f.jsx(O.Td,{children:f.jsx(Vt,{size:"sm",color:"blue",variant:"outline",children:String(i.score??"—")})})]},o))})]})]}),r.length===0&&f.jsx(Q,{size:"sm",c:"dimmed",children:"No hot channel data yet."})]})]})})}function dX(){const n=ht({queryKey:["plex-ghost-report"],queryFn:()=>we.get("/api/plex/ghost-report.json?observe=0s"),staleTime:3e4}),e=dt({mutationFn:()=>we.post("/api/ops/actions/ghost-visible-stop"),onSuccess:()=>Oe.show({message:"Stop visible ghosts requested",color:"teal"}),onError:o=>Oe.show({message:o.message,color:"red"})}),t=dt({mutationFn:()=>we.post("/api/ops/actions/ghost-hidden-recover?mode=dry-run"),onSuccess:()=>Oe.show({message:"Dry-run recovery triggered",color:"teal"}),onError:o=>Oe.show({message:o.message,color:"red"})}),s=dt({mutationFn:()=>we.post("/api/ops/actions/ghost-hidden-recover?mode=restart"),onSuccess:()=>Oe.show({message:"Hidden grab restart requested",color:"teal"}),onError:o=>Oe.show({message:o.message,color:"red"})}),r=n.data,i=Array.isArray(r==null?void 0:r.visible_ghosts)?r.visible_ghosts:[];return f.jsx(Je,{gap:"md",children:f.jsxs(ft,{withBorder:!0,p:"md",children:[f.jsxs(_e,{justify:"space-between",mb:"xs",children:[f.jsx(Q,{fw:600,children:"Plex Ghost Hunter"}),f.jsxs(_e,{gap:"xs",children:[f.jsx(Ve,{size:"xs",color:"red",variant:"outline",onClick:()=>{confirm("Stop all visible ghost sessions?")&&e.mutate()},loading:e.isPending,children:"Stop Visible Ghosts"}),f.jsx(Ve,{size:"xs",variant:"outline",onClick:()=>{confirm("Run dry-run hidden recovery?")&&t.mutate()},loading:t.isPending,children:"Dry-Run Hidden Recovery"}),f.jsx(Ve,{size:"xs",color:"orange",variant:"outline",onClick:()=>{confirm("Restart all hidden grabs? This will interrupt them.")&&s.mutate()},loading:s.isPending,children:"Restart Hidden Grabs"})]})]}),n.isLoading?f.jsx(Q,{size:"sm",c:"dimmed",children:"Loading…"}):n.isError?f.jsx(jt,{icon:f.jsx(Zt,{size:16}),color:"gray",children:"Ghost report unavailable. Plex integration may not be configured."}):f.jsxs(Je,{gap:"sm",children:[f.jsxs(_e,{gap:"xl",children:[f.jsxs(he,{children:[f.jsx(Q,{size:"xs",c:"dimmed",children:"Visible ghosts"}),f.jsx(Q,{fw:500,c:i.length>0?"red":"teal",children:i.length})]}),f.jsxs(he,{children:[f.jsx(Q,{size:"xs",c:"dimmed",children:"Hidden grabs"}),f.jsx(Q,{fw:500,children:String((r==null?void 0:r.hidden_grabs)??"0")})]})]}),i.length>0&&f.jsxs(f.Fragment,{children:[f.jsx(Jn,{label:"Visible Ghost Sessions"}),f.jsxs(O,{withRowBorders:!1,fz:"sm",striped:!0,children:[f.jsx(O.Thead,{children:f.jsxs(O.Tr,{children:[f.jsx(O.Th,{children:"Session"}),f.jsx(O.Th,{children:"When"})]})}),f.jsx(O.Tbody,{children:i.map((o,c)=>f.jsxs(O.Tr,{children:[f.jsx(O.Td,{children:String(o.session_name??o.session_id??o.name??"—")}),f.jsx(O.Td,{c:"dimmed",children:o.at?new Date(String(o.at)).toLocaleTimeString():"—"})]},c))})]})]}),i.length===0&&f.jsx(Q,{size:"sm",c:"dimmed",children:"No visible ghost sessions."})]})]})})}function fX(){return f.jsxs(Je,{gap:"md",h:"100%",style:{overflow:"hidden"},children:[f.jsx(_e,{justify:"space-between",children:f.jsx(Q,{size:"lg",fw:600,children:"Stats"})}),f.jsx(ft,{withBorder:!0,p:"md",style:{flex:1,overflow:"hidden"},children:f.jsxs(Ge,{defaultValue:"streams",keepMounted:!1,children:[f.jsxs(Ge.List,{children:[f.jsx(Ge.Tab,{value:"streams",leftSection:f.jsx(Kz,{size:14}),children:"Active Streams"}),f.jsx(Ge.Tab,{value:"events",leftSection:f.jsx(Zt,{size:14}),children:"System Events"}),f.jsx(Ge.Tab,{value:"connections",leftSection:f.jsx(lM,{size:14}),children:"Connections"}),f.jsx(Ge.Tab,{value:"routing",leftSection:f.jsx(ZG,{size:14}),children:"Routing"}),f.jsx(Ge.Tab,{value:"autopilot",leftSection:f.jsx(XG,{size:14}),children:"Autopilot"}),f.jsx(Ge.Tab,{value:"plex",leftSection:f.jsx(AG,{size:14}),children:"Plex"})]}),f.jsx(Jn,{}),f.jsxs(he,{pt:"md",children:[f.jsx(Ge.Panel,{value:"streams",children:f.jsx(nX,{})}),f.jsx(Ge.Panel,{value:"events",children:f.jsx(rX,{})}),f.jsx(Ge.Panel,{value:"connections",children:f.jsx(lX,{})}),f.jsx(Ge.Panel,{value:"routing",children:f.jsx(cX,{})}),f.jsx(Ge.Panel,{value:"autopilot",children:f.jsx(uX,{})}),f.jsx(Ge.Panel,{value:"plex",children:f.jsx(dX,{})})]})]})})]})}const $c={list:()=>we.get("/api/v2/plugins"),create:n=>we.post("/api/v2/plugins",n),update:(n,e)=>we.patch(`/api/v2/plugins/${n}`,e),enable:n=>we.post(`/api/v2/plugins/${n}/enable`,{}),disable:n=>we.post(`/api/v2/plugins/${n}/disable`,{}),delete:n=>we.del(`/api/v2/plugins/${n}`)};function hX({opened:n,onClose:e,initial:t}){const s=cn(),r=!!t,[i,o]=A.useState((t==null?void 0:t.name)??""),[c,u]=A.useState((t==null?void 0:t.version)??""),[d,m]=A.useState((t==null?void 0:t.description)??""),[g,p]=A.useState((t==null?void 0:t.path)??""),[y,x]=A.useState((t==null?void 0:t.manifest)??""),[b,T]=A.useState((t==null?void 0:t.enabled)??!0);function R(){o(""),u(""),m(""),p(""),x(""),T(!0)}const w=dt({mutationFn:()=>{const C={name:i,version:c||void 0,description:d||void 0,path:g,manifest:y||void 0,enabled:b};return r?$c.update(t.id,C):$c.create(C)},onSuccess:()=>{s.invalidateQueries({queryKey:["plugins"]}),Oe.show({message:r?"Plugin updated":"Plugin registered",color:"teal"}),R(),e()},onError:C=>Oe.show({message:C.message,color:"red"})});return f.jsxs(Sn,{opened:n,onClose:()=>{R(),e()},title:r?`Edit — ${t==null?void 0:t.name}`:"Register Plugin",size:"md",children:[f.jsxs(Je,{gap:"sm",children:[f.jsx(Lt,{label:"Name",value:i,onChange:C=>o(C.currentTarget.value),required:!0}),f.jsx(Lt,{label:"Version",value:c,onChange:C=>u(C.currentTarget.value),placeholder:"1.0.0"}),f.jsx(Lt,{label:"Description",value:d,onChange:C=>m(C.currentTarget.value)}),f.jsx(Lt,{label:"Path / entry point",value:g,onChange:C=>p(C.currentTarget.value),required:!0,placeholder:"/opt/plugins/my-plugin.so"}),f.jsx(wu,{label:"Manifest JSON",value:y,onChange:C=>x(C.currentTarget.value),placeholder:'{"capabilities": []}',autosize:!0,minRows:3,maxRows:8,styles:{input:{fontFamily:"monospace",fontSize:12}}}),f.jsx(Is,{label:"Enabled",checked:b,onChange:C=>T(C.currentTarget.checked)})]}),f.jsx(Jn,{my:"sm"}),f.jsxs(_e,{justify:"flex-end",children:[f.jsx(Ve,{variant:"default",onClick:()=>{R(),e()},children:"Cancel"}),f.jsx(Ve,{color:"teal",loading:w.isPending,onClick:()=>w.mutate(),children:r?"Save":"Register"})]})]})}function mX(){const n=cn(),[e,t]=A.useState(!1),[s,r]=A.useState(null),{data:i=[],isLoading:o}=ht({queryKey:["plugins"],queryFn:()=>$c.list()}),c=dt({mutationFn:({id:d,enabled:m})=>m?$c.enable(d):$c.disable(d),onSuccess:()=>n.invalidateQueries({queryKey:["plugins"]}),onError:d=>Oe.show({message:d.message,color:"red"})}),u=dt({mutationFn:d=>$c.delete(d),onSuccess:()=>n.invalidateQueries({queryKey:["plugins"]}),onError:d=>Oe.show({message:d.message,color:"red"})});return f.jsxs(Je,{gap:"md",h:"100%",style:{overflow:"hidden"},children:[f.jsxs(_e,{justify:"space-between",children:[f.jsx(Q,{size:"lg",fw:600,children:"Plugins"}),f.jsx(Ve,{size:"xs",leftSection:f.jsx(Ma,{size:14}),color:"teal",onClick:()=>{r(null),t(!0)},children:"Register Plugin"})]}),f.jsx(ft,{withBorder:!0,p:"md",style:{flex:1,overflow:"hidden"},children:o?f.jsx(Q,{size:"sm",c:"dimmed",children:"Loading…"}):i.length===0?f.jsxs(jt,{icon:f.jsx(Zt,{size:16}),color:"gray",children:["No plugins registered."," ",f.jsx(Q,{span:!0,size:"sm",children:"Register a plugin by providing its path and manifest."})]}):f.jsx(gn,{children:f.jsxs(O,{striped:!0,highlightOnHover:!0,withRowBorders:!1,fz:"sm",children:[f.jsx(O.Thead,{children:f.jsxs(O.Tr,{children:[f.jsx(O.Th,{children:"Name"}),f.jsx(O.Th,{children:"Version"}),f.jsx(O.Th,{children:"Path"}),f.jsx(O.Th,{children:"Status"}),f.jsx(O.Th,{children:"Registered"}),f.jsx(O.Th,{style:{width:90}})]})}),f.jsx(O.Tbody,{children:i.map(d=>f.jsxs(O.Tr,{children:[f.jsxs(O.Td,{children:[f.jsxs(_e,{gap:"xs",children:[f.jsx(mM,{size:14,style:{opacity:.6}}),f.jsx(Q,{size:"sm",fw:500,children:d.name})]}),d.description&&f.jsx(Q,{size:"xs",c:"dimmed",children:d.description})]}),f.jsx(O.Td,{children:f.jsx(Q,{size:"xs",c:"dimmed",children:d.version??"—"})}),f.jsx(O.Td,{children:f.jsx(Q,{size:"xs",c:"dimmed",style:{fontFamily:"monospace",maxWidth:240,overflow:"hidden",textOverflow:"ellipsis",whiteSpace:"nowrap"},children:d.path})}),f.jsx(O.Td,{children:f.jsx(Vt,{size:"xs",color:d.enabled?"teal":"gray",style:{cursor:"pointer"},onClick:()=>c.mutate({id:d.id,enabled:!d.enabled}),children:d.enabled?"enabled":"disabled"})}),f.jsx(O.Td,{children:f.jsx(Q,{size:"xs",c:"dimmed",children:new Date(d.created_at).toLocaleDateString()})}),f.jsx(O.Td,{children:f.jsxs(_e,{gap:4,wrap:"nowrap",children:[f.jsx(St,{label:"Edit",children:f.jsx(bt,{size:"xs",variant:"subtle",color:"yellow",onClick:()=>{r(d),t(!0)},children:f.jsx(Gi,{size:14})})}),f.jsx(St,{label:"Delete",children:f.jsx(bt,{size:"xs",variant:"subtle",color:"red",onClick:()=>{confirm(`Delete plugin "${d.name}"?`)&&u.mutate(d.id)},children:f.jsx(Vi,{size:14})})})]})})]},d.id))})]})})}),f.jsx(hX,{opened:e,onClose:()=>{t(!1),r(null)},initial:s})]})}const Tg={list:()=>we.get("/api/v2/users"),create:n=>we.post("/api/v2/users",n),update:(n,e)=>we.patch(`/api/v2/users/${n}`,e),delete:n=>we.del(`/api/v2/users/${n}`)},gX={admin:"red",standard:"blue",streamer:"teal"};function pX({opened:n,onClose:e,initial:t}){const s=cn(),r=!!t,{data:i}=ht({queryKey:["profiles"],queryFn:()=>jc.list()}),o=i??[],[c,u]=A.useState((t==null?void 0:t.username)??""),[d,m]=A.useState(""),[g,p]=A.useState((t==null?void 0:t.role)??"standard"),[y,x]=A.useState((t==null?void 0:t.xc_password)??""),[b,T]=A.useState((t==null?void 0:t.hide_mature)??!1),[R,w]=A.useState((t==null?void 0:t.stream_limit)??0),[C,_]=A.useState((t==null?void 0:t.epg_days_back)??0),[D,P]=A.useState((t==null?void 0:t.epg_days_fwd)??7),[L,k]=A.useState(((t==null?void 0:t.profile_ids)??[]).map(String));function F(){u(""),m(""),p("standard"),x(""),T(!1),w(0),_(0),P(7),k([])}const B=dt({mutationFn:()=>{const U={username:c,password:d||void 0,role:g,xc_password:y,hide_mature:b,stream_limit:R,epg_days_back:C,epg_days_fwd:D,profile_ids:L.map(Number)};return r?Tg.update(t.id,U):Tg.create(U)},onSuccess:()=>{s.invalidateQueries({queryKey:["users"]}),Oe.show({message:r?"User updated":"User created",color:"teal"}),F(),e()},onError:U=>Oe.show({message:U.message,color:"red"})});return f.jsxs(Sn,{opened:n,onClose:()=>{F(),e()},title:r?`Edit — ${t==null?void 0:t.username}`:"New User",size:"md",children:[f.jsxs(Ge,{defaultValue:"account",children:[f.jsxs(Ge.List,{children:[f.jsx(Ge.Tab,{value:"account",children:"Account"}),f.jsx(Ge.Tab,{value:"access",children:"Access"}),f.jsx(Ge.Tab,{value:"epg",children:"EPG & Prefs"})]}),f.jsx(Ge.Panel,{value:"account",pt:"sm",children:f.jsxs(Je,{gap:"sm",children:[f.jsx(Lt,{label:"Username",value:c,onChange:U=>u(U.currentTarget.value),required:!0}),f.jsx(wS,{label:r?"New password (leave blank to keep)":"Password",value:d,onChange:U=>m(U.currentTarget.value),required:!r}),f.jsx(wn,{label:"Role",data:[{value:"admin",label:"Admin"},{value:"standard",label:"Standard"},{value:"streamer",label:"Streamer"}],value:g,onChange:U=>p(U??"standard")}),f.jsx(Lt,{label:"Xtream Codes password",value:y,onChange:U=>x(U.currentTarget.value),placeholder:"For XC API compatibility"})]})}),f.jsx(Ge.Panel,{value:"access",pt:"sm",children:f.jsxs(Je,{gap:"sm",children:[f.jsx(If,{label:"Allowed channel profiles (empty = all)",data:o.map(U=>({value:String(U.id),label:U.name})),value:L,onChange:k,placeholder:"All profiles",clearable:!0}),f.jsx(Hs,{label:"Max concurrent streams (0 = unlimited)",value:R,onChange:U=>w(Number(U)),min:0}),f.jsx(Is,{label:"Hide mature content",checked:b,onChange:U=>T(U.currentTarget.checked)})]})}),f.jsx(Ge.Panel,{value:"epg",pt:"sm",children:f.jsxs(Je,{gap:"sm",children:[f.jsx(Hs,{label:"EPG days back (catch-up)",value:C,onChange:U=>_(Number(U)),min:0}),f.jsx(Hs,{label:"EPG days forward",value:D,onChange:U=>P(Number(U)),min:1})]})})]}),f.jsx(Jn,{my:"sm"}),f.jsxs(_e,{justify:"flex-end",children:[f.jsx(Ve,{variant:"default",onClick:()=>{F(),e()},children:"Cancel"}),f.jsx(Ve,{color:"teal",loading:B.isPending,onClick:()=>B.mutate(),children:r?"Save":"Create"})]})]})}function vX(){const n=cn(),[e,t]=A.useState(!1),[s,r]=A.useState(null),{data:i=[],isLoading:o}=ht({queryKey:["users"],queryFn:()=>Tg.list()}),c=dt({mutationFn:d=>Tg.delete(d),onSuccess:()=>n.invalidateQueries({queryKey:["users"]}),onError:d=>Oe.show({message:d.message,color:"red"})}),u=d=>d==="admin"?f.jsx(yM,{size:14}):d==="streamer"?f.jsx(pp,{size:14}):f.jsx(f9,{size:14});return f.jsxs(Je,{gap:"md",h:"100%",style:{overflow:"hidden"},children:[f.jsxs(_e,{justify:"space-between",children:[f.jsx(Q,{size:"lg",fw:600,children:"Users"}),f.jsx(Ve,{size:"xs",leftSection:f.jsx(Ma,{size:14}),color:"teal",onClick:()=>{r(null),t(!0)},children:"New User"})]}),f.jsx(ft,{withBorder:!0,p:"md",style:{flex:1,overflow:"hidden"},children:o?f.jsx(Q,{size:"sm",c:"dimmed",children:"Loading…"}):i.length===0?f.jsx(jt,{icon:f.jsx(Zt,{size:16}),color:"gray",children:"No users yet. Create an admin account to enable authentication."}):f.jsx(gn,{children:f.jsxs(O,{striped:!0,highlightOnHover:!0,withRowBorders:!1,fz:"sm",children:[f.jsx(O.Thead,{children:f.jsxs(O.Tr,{children:[f.jsx(O.Th,{children:"Username"}),f.jsx(O.Th,{children:"Role"}),f.jsx(O.Th,{children:"Stream limit"}),f.jsx(O.Th,{children:"Profiles"}),f.jsx(O.Th,{children:"Created"}),f.jsx(O.Th,{style:{width:80}})]})}),f.jsx(O.Tbody,{children:i.map(d=>f.jsxs(O.Tr,{children:[f.jsx(O.Td,{children:f.jsxs(_e,{gap:"xs",children:[u(d.role),f.jsx(Q,{size:"sm",children:d.username})]})}),f.jsx(O.Td,{children:f.jsx(Vt,{size:"xs",color:gX[d.role]??"gray",children:d.role})}),f.jsx(O.Td,{children:f.jsx(Q,{size:"xs",c:"dimmed",children:d.stream_limit===0?"Unlimited":d.stream_limit})}),f.jsx(O.Td,{children:f.jsx(Q,{size:"xs",c:"dimmed",children:d.profile_ids.length===0?"All":d.profile_ids.length})}),f.jsx(O.Td,{children:f.jsx(Q,{size:"xs",c:"dimmed",children:new Date(d.created_at).toLocaleDateString()})}),f.jsx(O.Td,{children:f.jsxs(_e,{gap:4,wrap:"nowrap",children:[f.jsx(St,{label:"Edit",children:f.jsx(bt,{size:"xs",variant:"subtle",color:"yellow",onClick:()=>{r(d),t(!0)},children:f.jsx(Gi,{size:14})})}),f.jsx(St,{label:"Delete",children:f.jsx(bt,{size:"xs",variant:"subtle",color:"red",onClick:()=>{confirm(`Delete user "${d.username}"?`)&&c.mutate(d.id)},children:f.jsx(Vi,{size:14})})})]})})]},d.id))})]})})}),f.jsx(pX,{opened:e,onClose:()=>{t(!1),r(null)},initial:s})]})}const Uy={list:()=>we.get("/api/v2/logos"),delete:n=>we.del(`/api/v2/logos/${n}`),upload:async n=>{const e=new FormData;e.append("file",n);const t={},s=Lo().csrf;s&&(t["X-IPTVTunerr-CSRF"]=s);const r=await fetch("/api/v2/logos",{method:"POST",headers:t,body:e});if(!r.ok)throw new Error(`Upload failed: ${r.status}`);return r.json()}};function yX(n){return n<1024?`${n} B`:n<1024*1024?`${(n/1024).toFixed(1)} KB`:`${(n/(1024*1024)).toFixed(1)} MB`}function xX(){const n=cn(),[e,t]=A.useState(!1),s=A.useRef(0),{data:r=[],isLoading:i}=ht({queryKey:["logos"],queryFn:()=>Uy.list()}),o=dt({mutationFn:p=>Uy.upload(p),onSuccess:p=>{n.invalidateQueries({queryKey:["logos"]}),Oe.show({message:`Uploaded ${p.filename}`,color:"teal"})},onError:p=>Oe.show({message:p.message,color:"red"})}),c=dt({mutationFn:p=>Uy.delete(p),onSuccess:()=>n.invalidateQueries({queryKey:["logos"]}),onError:p=>Oe.show({message:p.message,color:"red"})}),u=A.useCallback(p=>{if(!p)return;const y=Array.from(p),x=y.filter(b=>b.type.startsWith("image/"));x.length!==y.length&&Oe.show({message:"Only image files are accepted",color:"orange"}),x.forEach(b=>o.mutate(b))},[o]),d=A.useCallback(p=>{p.preventDefault(),s.current=0,t(!1),u(p.dataTransfer.files)},[u]),m=A.useCallback(p=>{p.preventDefault(),s.current++,t(!0)},[]),g=A.useCallback(p=>{p.preventDefault(),s.current--,s.current===0&&t(!1)},[]);return f.jsxs(Je,{gap:"md",h:"100%",style:{overflow:"hidden"},children:[f.jsxs(_e,{justify:"space-between",children:[f.jsx(Q,{size:"lg",fw:600,children:"Logo Manager"}),f.jsxs(_e,{gap:"xs",children:[o.isPending&&f.jsx(zi,{size:"xs",color:"teal"}),f.jsx(vI,{onChange:p=>p&&u([p]),accept:"image/*",children:p=>f.jsx(Ve,{size:"xs",leftSection:f.jsx(u9,{size:14}),color:"teal",...p,children:"Upload"})})]})]}),f.jsx(he,{onDrop:d,onDragEnter:m,onDragLeave:g,onDragOver:p=>p.preventDefault(),style:{flex:1,overflow:"auto",border:`2px dashed ${e?"var(--mantine-color-teal-6)":"transparent"}`,borderRadius:"var(--mantine-radius-md)",transition:"border-color 0.15s"},children:i?f.jsx(Q,{size:"sm",c:"dimmed",children:"Loading…"}):r.length===0?f.jsxs(ft,{withBorder:!0,p:"xl",ta:"center",children:[f.jsx(GS,{size:48,style:{opacity:.3}}),f.jsx(Q,{mt:"sm",c:"dimmed",size:"sm",children:"No logos yet. Upload images or drag & drop files here."})]}):f.jsxs(Je,{gap:"sm",children:[f.jsx(jt,{icon:f.jsx(Zt,{size:16}),color:"gray",variant:"light",children:"Drag & drop image files anywhere on this page to upload. Max 2 MB per file."}),f.jsx(kf,{cols:{base:3,sm:4,md:6,lg:8},spacing:"sm",children:r.map(p=>f.jsx(bX,{logo:p,onDelete:()=>{confirm(`Delete logo "${p.filename}"?`)&&c.mutate(p.id)}},p.id))})]})})]})}function bX({logo:n,onDelete:e}){const t=n.url??`/api/v2/logos/${n.id}/image`;return f.jsxs(ft,{withBorder:!0,p:"xs",style:{position:"relative"},children:[f.jsx(he,{style:{aspectRatio:"1",overflow:"hidden",display:"flex",alignItems:"center",justifyContent:"center"},children:f.jsx(Lf,{src:t,alt:n.filename,fit:"contain",style:{maxHeight:80,maxWidth:"100%"},fallbackSrc:"data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='80' height='80'%3E%3C/svg%3E"})}),f.jsx(Q,{size:"xs",c:"dimmed",truncate:!0,mt:4,title:n.filename,children:n.filename}),f.jsx(Q,{size:"xs",c:"dimmed",children:yX(n.size_bytes)}),f.jsx(Vt,{size:"xs",variant:"dot",color:"gray",style:{position:"absolute",top:4,right:24},children:n.content_type.replace("image/","")}),f.jsx(St,{label:"Delete",children:f.jsx(bt,{size:"xs",variant:"subtle",color:"red",style:{position:"absolute",top:4,right:4},onClick:e,children:f.jsx(Vi,{size:12})})})]})}const SX=[{value:"ffmpeg",label:"FFmpeg (transcode)"},{value:"proxy",label:"Proxy (passthrough)"},{value:"redirect",label:"Redirect (HTTP 302)"},{value:"streamlink",label:"Streamlink"},{value:"vlc",label:"VLC"},{value:"yt-dlp",label:"yt-dlp"},{value:"custom",label:"Custom"}],TX=JSON.stringify({video_codec:"copy",audio_codec:"copy",extra_args:[]},null,2),EX=["","Lavf/58.76.100","VLC/3.0.18 LibVLC/3.0.18","Mozilla/5.0 (Windows NT 10.0; Win64; x64)"];function AX({opened:n,onClose:e,initial:t}){const s=cn(),r=!!t,[i,o]=A.useState((t==null?void 0:t.name)??""),[c,u]=A.useState((t==null?void 0:t.type)??"proxy"),[d,m]=A.useState((t==null?void 0:t.config_json)??""),[g,p]=A.useState((t==null?void 0:t.is_default)??!1);A.useEffect(()=>{n&&(o((t==null?void 0:t.name)??""),u((t==null?void 0:t.type)??"proxy"),m((t==null?void 0:t.config_json)??""),p((t==null?void 0:t.is_default)??!1))},[n,t]);const y=dt({mutationFn:()=>{const x={name:i,type:c,config_json:d||void 0,is_default:g};return r?mu.update(t.id,x):mu.create(x)},onSuccess:()=>{s.invalidateQueries({queryKey:["stream-profiles"]}),Oe.show({message:r?"Profile updated":"Profile created",color:"teal"}),e()},onError:x=>Oe.show({message:x.message,color:"red"})});return f.jsxs(Sn,{opened:n,onClose:e,title:r?`Edit — ${t==null?void 0:t.name}`:"New Stream Profile",size:"md",children:[f.jsxs(Je,{gap:"sm",children:[f.jsx(Lt,{label:"Name",value:i,onChange:x=>o(x.currentTarget.value),required:!0}),f.jsx(wn,{label:"Type",data:SX,value:c,onChange:x=>{const b=x??"proxy";u(b),b==="ffmpeg"&&!d&&m(TX)}}),f.jsx(wu,{label:"Config JSON",value:d,onChange:x=>m(x.currentTarget.value),placeholder:'{"key": "value"}',autosize:!0,minRows:4,maxRows:12,styles:{input:{fontFamily:"monospace",fontSize:12}}}),f.jsx(Is,{label:"Set as default profile",checked:g,onChange:x=>p(x.currentTarget.checked)})]}),f.jsx(Jn,{my:"sm"}),f.jsxs(_e,{justify:"flex-end",children:[f.jsx(Ve,{variant:"default",onClick:e,children:"Cancel"}),f.jsx(Ve,{color:"teal",loading:y.isPending,onClick:()=>y.mutate(),children:r?"Save":"Create"})]})]})}function RX(){const n=cn(),{data:e}=ht({queryKey:["settings"],queryFn:()=>lg.get()}),[t,s]=A.useState("iptvTunerr"),[r,i]=A.useState(1),[o,c]=A.useState("{state_dir}/recordings/{title}/{title} - {date}.ts"),[u,d]=A.useState(0),[m,g]=A.useState(30);A.useEffect(()=>{e&&(s(e["tuner.device_name"]??"iptvTunerr"),i(Number(e["tuner.device_count"]??1)),c(e["dvr.path_template"]??"{state_dir}/recordings/{title}/{title} - {date}.ts"),d(Number(e["dvr.pad_before_sec"]??0)),g(Number(e["dvr.pad_after_sec"]??30)))},[e]);const p=dt({mutationFn:()=>lg.patch({"tuner.device_name":t,"tuner.device_count":String(r),"dvr.path_template":o,"dvr.pad_before_sec":String(u),"dvr.pad_after_sec":String(m)}),onSuccess:()=>{n.invalidateQueries({queryKey:["settings"]}),Oe.show({message:"Settings saved",color:"teal"})},onError:b=>Oe.show({message:b.message,color:"red"})}),{version:y,port:x}=Lo();return f.jsxs(Je,{gap:"md",children:[f.jsxs(ft,{withBorder:!0,p:"md",children:[f.jsx(Q,{fw:600,mb:"sm",children:"Tuner Device"}),f.jsxs(Je,{gap:"sm",children:[f.jsx(Lt,{label:"Device name (shown to Plex/Emby)",value:t,onChange:b=>s(b.currentTarget.value)}),f.jsx(Hs,{label:"Tuner count (max concurrent streams)",value:r,onChange:b=>i(Number(b)),min:1,max:100})]})]}),f.jsxs(ft,{withBorder:!0,p:"md",children:[f.jsx(Q,{fw:600,mb:"sm",children:"DVR"}),f.jsxs(Je,{gap:"sm",children:[f.jsx(Lt,{label:"Recording path template",value:o,onChange:b=>c(b.currentTarget.value),description:"Tokens: {state_dir} {title} {channel} {date} {time} {year} {month} {day}"}),f.jsxs(_e,{grow:!0,children:[f.jsx(Hs,{label:"Pad before (seconds)",value:u,onChange:b=>d(Number(b)),min:0}),f.jsx(Hs,{label:"Pad after (seconds)",value:m,onChange:b=>g(Number(b)),min:0})]})]})]}),f.jsxs(ft,{withBorder:!0,p:"md",children:[f.jsx(Q,{fw:600,mb:"sm",children:"System"}),f.jsxs(Je,{gap:"xs",children:[f.jsxs(_e,{gap:"xs",children:[f.jsx(Q,{size:"sm",c:"dimmed",w:120,children:"Version"}),f.jsx(ji,{children:y})]}),f.jsxs(_e,{gap:"xs",children:[f.jsx(Q,{size:"sm",c:"dimmed",w:120,children:"Port"}),f.jsx(ji,{children:x})]}),f.jsxs(_e,{gap:"xs",children:[f.jsx(Q,{size:"sm",c:"dimmed",w:120,children:"API"}),f.jsx(Tu,{size:"sm",href:"/api/",target:"_blank",children:"/api/"})]})]})]}),f.jsx(_e,{justify:"flex-end",children:f.jsx(Ve,{color:"teal",leftSection:f.jsx(fM,{size:14}),loading:p.isPending,onClick:()=>p.mutate(),children:"Save Settings"})})]})}function wX(){const n=cn(),[e,t]=A.useState(!1),[s,r]=A.useState(null),{data:i=[],isLoading:o}=ht({queryKey:["stream-profiles"],queryFn:()=>mu.list()}),c=dt({mutationFn:u=>mu.delete(u),onSuccess:()=>n.invalidateQueries({queryKey:["stream-profiles"]}),onError:u=>Oe.show({message:u.message,color:"red"})});return f.jsxs(Je,{gap:"md",children:[f.jsxs(_e,{justify:"space-between",children:[f.jsx(Q,{size:"sm",c:"dimmed",children:"Stream profiles control how channels are delivered (transcode, proxy, redirect, or via external tools)."}),f.jsx(Ve,{size:"xs",leftSection:f.jsx(Ma,{size:14}),color:"teal",onClick:()=>{r(null),t(!0)},children:"New Profile"})]}),o?f.jsx(Q,{size:"sm",c:"dimmed",children:"Loading…"}):i.length===0?f.jsx(jt,{icon:f.jsx(Zt,{size:16}),color:"gray",children:"No stream profiles. The built-in proxy mode is used by default."}):f.jsx(ft,{withBorder:!0,style:{overflow:"hidden"},children:f.jsx(gn,{children:f.jsxs(O,{striped:!0,highlightOnHover:!0,withRowBorders:!1,fz:"sm",children:[f.jsx(O.Thead,{children:f.jsxs(O.Tr,{children:[f.jsx(O.Th,{children:"Name"}),f.jsx(O.Th,{children:"Type"}),f.jsx(O.Th,{children:"Default"}),f.jsx(O.Th,{children:"Created"}),f.jsx(O.Th,{style:{width:80}})]})}),f.jsx(O.Tbody,{children:i.map(u=>f.jsxs(O.Tr,{children:[f.jsx(O.Td,{children:f.jsxs(_e,{gap:"xs",children:[f.jsx(mM,{size:14,style:{opacity:.6}}),f.jsx(Q,{size:"sm",children:u.name})]})}),f.jsx(O.Td,{children:f.jsx(Vt,{size:"xs",color:"blue",variant:"light",children:u.type})}),f.jsx(O.Td,{children:u.is_default&&f.jsx(Vt,{size:"xs",color:"teal",children:"default"})}),f.jsx(O.Td,{children:f.jsx(Q,{size:"xs",c:"dimmed",children:new Date(u.created_at).toLocaleDateString()})}),f.jsx(O.Td,{children:f.jsxs(_e,{gap:4,wrap:"nowrap",children:[f.jsx(St,{label:"Edit",children:f.jsx(bt,{size:"xs",variant:"subtle",color:"yellow",onClick:()=>{r(u),t(!0)},children:f.jsx(Gi,{size:14})})}),f.jsx(St,{label:"Delete",children:f.jsx(bt,{size:"xs",variant:"subtle",color:"red",onClick:()=>{confirm(`Delete profile "${u.name}"?`)&&c.mutate(u.id)},children:f.jsx(Vi,{size:14})})})]})})]},u.id))})]})})}),f.jsx(AX,{opened:e,onClose:()=>{t(!1),r(null)},initial:s})]})}function CX(){const n=cn(),{data:e}=ht({queryKey:["settings"],queryFn:()=>lg.get()}),[t,s]=A.useState(""),[r,i]=A.useState(""),[o,c]=A.useState(""),[u,d]=A.useState(""),[m,g]=A.useState("idle");A.useEffect(()=>{e&&(s(e["provider.user_agent"]??""),i(e["xtream.user"]??""),c(e["xtream.pass"]??""))},[e]);const p=dt({mutationFn:()=>lg.patch({"provider.user_agent":t,"xtream.user":r,"xtream.pass":o}),onSuccess:()=>{n.invalidateQueries({queryKey:["settings"]}),Oe.show({message:"Provider settings saved",color:"teal"})},onError:b=>Oe.show({message:b.message,color:"red"})});async function y(){if(u.trim()){g("saving");try{const b=Lo().csrf,T={"Content-Type":"text/plain"};b&&(T["X-IPTVTunerr-CSRF"]=b);const R=await fetch("/api/v2/settings/cookie-jar",{method:"POST",headers:T,body:u});if(!R.ok)throw new Error(`${R.status}`);g("ok"),d(""),Oe.show({message:"Cookie jar imported",color:"teal"})}catch(b){g("error"),Oe.show({message:`Import failed: ${b}`,color:"red"})}}}const x=EX.includes(t)?t:"custom";return f.jsxs(Je,{gap:"md",children:[f.jsxs(ft,{withBorder:!0,p:"md",children:[f.jsx(Q,{fw:600,mb:"sm",children:"User-Agent"}),f.jsxs(Je,{gap:"sm",children:[f.jsx(wn,{label:"Preset",data:[{value:"",label:"Default (iptvTunerr)"},{value:"Lavf/58.76.100",label:"Lavf (FFmpeg)"},{value:"VLC/3.0.18 LibVLC/3.0.18",label:"VLC"},{value:"Mozilla/5.0 (Windows NT 10.0; Win64; x64)",label:"Browser (generic)"},{value:"custom",label:"Custom…"}],value:x,onChange:b=>{b&&b!=="custom"&&s(b)},clearable:!1}),f.jsx(Lt,{label:"User-Agent string",value:t,onChange:b=>s(b.currentTarget.value),placeholder:"Leave blank to use iptvTunerr default"})]}),f.jsx(_e,{justify:"flex-end",mt:"sm",children:f.jsx(Ve,{size:"xs",color:"teal",leftSection:f.jsx(fM,{size:14}),loading:p.isPending,onClick:()=>p.mutate(),children:"Save"})})]}),f.jsxs(ft,{withBorder:!0,p:"md",children:[f.jsx(Q,{fw:600,mb:"sm",children:"Xtream Credentials"}),f.jsx(Q,{size:"sm",c:"dimmed",mb:"sm",children:"Used by the VODs page to query movie and series data from the tuner's Xtream API."}),f.jsxs(Je,{gap:"sm",children:[f.jsx(Lt,{label:"Xtream username",value:r,onChange:b=>i(b.currentTarget.value)}),f.jsx(Lt,{label:"Xtream password",value:o,onChange:b=>c(b.currentTarget.value)})]})]}),f.jsxs(ft,{withBorder:!0,p:"md",children:[f.jsx(Q,{fw:600,mb:"sm",children:"Cookie Jar"}),f.jsx(Q,{size:"sm",c:"dimmed",mb:"sm",children:'Import a Netscape-format cookie file to bypass Cloudflare and similar systems. Export using a browser extension such as "Get cookies.txt LOCALLY".'}),f.jsx(wu,{placeholder:`# Netscape HTTP Cookie File

## Red-team abuse lens
docs/dev/bug-council-negative-space.md:9:| Plex owner-token elevation | Plex client token on proxied Live TV requests | `internal/plexlabelproxy/proxy.go` | `canElevate` |
docs/dev/bug-council-negative-space.md:11:| Tokenless session recovery | Plex segment/timeline requests missing tokens | `internal/plexlabelproxy/proxy.go` | `sessionRecordMatchesSource` |
docs/dev/bug-council-negative-space.md:13:| Evidence/debug file tokens | Request/channel identifiers used in evidence filenames | `internal/tuner/gateway_debug.go` | `sanitizeFileToken` |
docs/dev/bug-council-negative-space.md:14:| Diagnostic header redaction | Stream-attempt/debug records that describe upstream request headers | `internal/tuner/gateway_attempts.go` | `sensitiveHeaderName` |
scripts/check-remediation-baseline.sh:79:require_pattern "func \\(p \\*Proxy\\) canElevate" "internal/plexlabelproxy/proxy.go" "canElevate remains owner-token gate"
scripts/check-remediation-baseline.sh:82:require_pattern "sessionRecordMatchesSource" "internal/plexlabelproxy/proxy.go" "tokenless recovery is source-bound"
scripts/check-remediation-baseline.sh:100:require_pattern "sanitizeFileToken" "internal/tuner/gateway_debug.go" "debug evidence file tokens are sanitized"
scripts/check-remediation-baseline.sh:102:require_pattern "sensitiveHeaderName" "internal/tuner/gateway_attempts.go" "diagnostic header summaries have credential-shaped header redaction"
scripts/check-remediation-baseline.sh:112:require_pattern "redactedHooks" "internal/eventhooks/eventhooks.go" "event hook reports redact configured hook secrets"
scripts/check-remediation-baseline.sh:126:require_pattern "TestProxy_DoesNotTrustHopByHopHeaderTokenForElevation" "internal/plexlabelproxy/proxy_test.go" "hop-by-hop token behavior test exists"
scripts/check-remediation-baseline.sh:127:require_pattern "TestProxy_DoesNotRecoverTokenlessLiveTVSegmentFromDifferentSource" "internal/plexlabelproxy/proxy_test.go" "tokenless cross-source behavior test exists"
scripts/check-remediation-baseline.sh:166:require_pattern "TestRunImportCookiesDryRunRedactsCookieValues" "cmd/iptv-tunerr/cmd_cookie_import_test.go" "cookie import dry-run redaction behavior test exists"
scripts/check-remediation-baseline.sh:176:secret_pattern='-----BEGIN (RSA |DSA |EC |OPENSSH |PGP )?PRIVATE KEY-----|gh[pousr]_[A-Za-z0-9_]{36,}|xox[baprs]-[A-Za-z0-9-]{20,}|AKIA[0-9A-Z]{16}'
scripts/check-remediation-baseline.sh:177:require_absent_pattern "$secret_pattern" "." "tracked text files do not contain high-confidence private keys or platform tokens"
internal/tuner/gateway_attempts.go:57:	CookiesForwarded  bool     `json:"cookies_forwarded,omitempty"`
internal/tuner/gateway_attempts.go:90:		CookiesForwarded:  cookiesForwarded,
internal/tuner/gateway_attempts.go:165:func sanitizeHeaderSummary(lines []string) []string {
internal/tuner/gateway_attempts.go:182:		if sensitiveHeaderName(name) {
internal/tuner/gateway_attempts.go:191:func sensitiveHeaderName(name string) bool {
internal/tuner/gateway_attempts.go:197:	case "authorization", "cookie", "proxy-authorization", "set-cookie", "x-plex-token":
internal/tuner/gateway_attempts.go:200:	return strings.Contains(name, "token") ||
internal/tuner/gateway_attempts.go:201:		strings.Contains(name, "secret") ||
internal/tuner/gateway_attempts.go:219:	return sanitizeHeaderSummary(out)
internal/tuner/gateway_attempts.go:249:	return sanitizeHeaderSummary(lines)
internal/tuner/gateway_test.go:168:		t.Fatalf("printf width token broken (double %%25): %q", q)
internal/tuner/gateway_test.go:328:	redir := "http://127.0.0.1:9/secret.ts"
internal/tuner/gateway_test.go:469:http://user:pass@segments.example/live/u/p/seg.ts?password=p&token=secret
internal/tuner/gateway_test.go:493:	for _, secret := range []string{"user:pass", "password=p", "token=secret"} {
internal/tuner/gateway_test.go:494:		if strings.Contains(got, secret) {
internal/tuner/gateway_test.go:495:			t.Fatalf("HLS startup log leaked %q in %q", secret, got)
internal/tuner/gateway_test.go:506:#EXT-X-KEY:METHOD=AES-128,URI="https://keys.example.com/secret.key"
internal/tuner/gateway_test.go:519:	if !strings.Contains(s, "https%3A%2F%2Fkeys.example.com%2Fsecret.key") {
internal/tuner/gateway_test.go:1267:			{GuideNumber: "1", GuideName: "Peacock Event", StreamURL: up.URL + "/live/event.m3u8?token=keep"},
internal/tuner/gateway_test.go:1318:	got, ok := hlsTSFallbackURL("http://provider.example/live/u/p/1910860.m3u8?token=abc#frag")
internal/tuner/gateway_test.go:1322:	want := "http://provider.example/live/u/p/1910860.ts?token=abc#frag"
internal/tuner/gateway_test.go:1333:	if !shouldBypassFFmpegForRawMPEGTS(ch, "http://provider.example/live/u/p/1910860.ts?token=abc") {
internal/tuner/gateway_test.go:1350:	if got := req.Header.Get("Authorization"); got != "" {
internal/tuner/gateway_test.go:1351:		t.Fatalf("Authorization=%q want empty", got)
internal/tuner/gateway_test.go:3825:	req.Header.Set("Cookie", "session=abc123")
internal/tuner/gateway_test.go:3828:	req.Header.Set("Authorization", "Bearer upstream-token")
internal/tuner/gateway_test.go:3833:	if !strings.Contains(block, "Cookie: session=abc123") {
internal/tuner/gateway_test.go:3842:	if !strings.Contains(block, "Authorization: Bearer upstream-token") {
internal/tuner/gateway_test.go:3887:			raw:    "Authorization: Bearer token:with:colons",
internal/tuner/gateway_test.go:3888:			expect: map[string]string{"Authorization": "Bearer token:with:colons"},
internal/tuner/gateway_test.go:4016:		"Authorization: Bearer upstream-token",
internal/tuner/gateway_test.go:4017:		"Cookie: cf_clearance=secret",
internal/tuner/gateway_test.go:4018:		"X-Plex-Token: plex-secret",
internal/tuner/gateway_test.go:4019:		"X-API-Key: provider-key",
internal/tuner/gateway_test.go:4020:		"X-Auth-Token: provider-token",
internal/tuner/gateway_test.go:4021:		"X-Session-Id: session-secret",
internal/tuner/gateway_test.go:4024:	got := sanitizeHeaderSummary(lines)
internal/tuner/gateway_test.go:4026:	for _, secret := range []string{"upstream-token", "cf_clearance=secret", "plex-secret", "provider-key", "provider-token", "session-secret"} {
internal/tuner/gateway_test.go:4027:		if strings.Contains(joined, secret) {
internal/tuner/gateway_test.go:4028:			t.Fatalf("sanitizeHeaderSummary leaked %q in:\n%s", secret, joined)
internal/tuner/gateway_test.go:4032:		"Authorization: <redacted>",
internal/tuner/gateway_test.go:4033:		"Cookie: <redacted>",
internal/tuner/gateway_test.go:4034:		"X-Plex-Token: <redacted>",
internal/tuner/gateway_test.go:4035:		"X-API-Key: <redacted>",
internal/tuner/gateway_test.go:4036:		"X-Auth-Token: <redacted>",
internal/tuner/gateway_test.go:4041:			t.Fatalf("sanitizeHeaderSummary missing %q in:\n%s", want, joined)
internal/tuner/gateway_test.go:4048:	h.Set("Authorization", "Bearer upstream-token")
internal/tuner/gateway_test.go:4049:	h.Set("X-Plex-Token", "plex-secret")
internal/tuner/gateway_test.go:4050:	h.Set("X-API-Key", "provider-key")
internal/tuner/gateway_test.go:4052:	joined := strings.Join(debugHeaderLines(h), "\n")
internal/tuner/gateway_test.go:4053:	for _, secret := range []string{"upstream-token", "plex-secret", "provider-key"} {
internal/tuner/gateway_test.go:4054:		if strings.Contains(joined, secret) {
internal/tuner/gateway_test.go:4055:			t.Fatalf("debugHeaderLines leaked %q in:\n%s", secret, joined)
internal/tuner/gateway_test.go:4059:		t.Fatalf("debugHeaderLines should keep non-secret Referer, got:\n%s", joined)
internal/tuner/gateway_test.go:4065:	tee := newCappedBodyTee(dir, 16, "req/../secret", "Channel: One", "ch/1")
internal/tuner/gateway_test.go:4087:		t.Fatalf("tee path was not filename-tokenized: %q", tee.path)
internal/tuner/gateway_test.go:4094:	idx := attempt.addUpstream(1, "http://user:pass@provider.example/live/u/p/1.m3u8?password=p&token=abc", nil, false, false, false, false)
internal/tuner/gateway_test.go:4095:	attempt.markPlaylist(idx, true, 128, "http://user:pass@provider.example/live/u/p/seg.ts?api_key=secret&token=abc")
internal/tuner/gateway_test.go:4096:	attempt.markUpstreamError(idx, "failed", errors.New("fetch http://provider.example/live/u/p/seg.ts?password=p&token=abc failed: Authorization: Bearer provider-secret"))
internal/tuner/gateway_test.go:4097:	rec := attempt.finish("failed", "hls", errors.New("final http://provider.example/live/u/p/seg.ts?password=p&token=abc failed: Cookie: cf_clearance=secret"), "http://provider.example/live/u/p/1.m3u8?password=p&token=abc")
internal/tuner/gateway_test.go:4104:	for _, secret := range []string{"user:pass", "password=p", "token=abc", "api_key=secret", "provider-secret", "cf_clearance=secret"} {
internal/tuner/gateway_test.go:4105:		if strings.Contains(body, secret) {
internal/tuner/gateway_test.go:4106:			t.Fatalf("stream attempt diagnostic leaked %q in:\n%s", secret, body)
internal/tuner/gateway_test.go:4109:	for _, want := range []string{"http://provider.example/live/redacted/redacted/seg.ts", "Authorization: \\u003credacted\\u003e", "Cookie: \\u003credacted\\u003e"} {
internal/tuner/gateway_test.go:4119:			"X-API-Key":    "provider-key",
internal/tuner/gateway_test.go:4120:			"X-Auth-Token": "provider-token",
internal/tuner/gateway_test.go:4124:	incoming.Header.Set("Cookie", "cf_clearance=secret")
internal/tuner/gateway_test.go:4125:	incoming.Header.Set("Authorization", "Bearer upstream-token")
internal/tuner/gateway_test.go:4130:	if got := upstream.Header.Get("Cookie"); got != "cf_clearance=secret" {
internal/tuner/gateway_test.go:4131:		t.Fatalf("Cookie was not forwarded to upstream, got %q", got)
internal/tuner/gateway_test.go:4133:	if got := upstream.Header.Get("Authorization"); got != "Bearer upstream-token" {
internal/tuner/gateway_test.go:4134:		t.Fatalf("Authorization was not forwarded to upstream, got %q", got)
internal/tuner/gateway_test.go:4136:	if got := upstream.Header.Get("X-API-Key"); got != "provider-key" {
internal/tuner/gateway_test.go:4137:		t.Fatalf("X-API-Key custom header was not forwarded to upstream, got %q", got)
internal/tuner/gateway_test.go:4139:	if got := upstream.Header.Get("X-Auth-Token"); got != "provider-token" {
internal/tuner/gateway_test.go:4140:		t.Fatalf("X-Auth-Token custom header was not forwarded to upstream, got %q", got)
internal/tuner/gateway_test.go:4147:			"X-API-Key":    "provider-key",
internal/tuner/gateway_test.go:4148:			"X-Auth-Token": "provider-token",
internal/tuner/gateway_test.go:4152:	incoming.Header.Set("Cookie", "cf_clearance=secret")
internal/tuner/gateway_test.go:4153:	incoming.Header.Set("Authorization", "Bearer upstream-token")
internal/tuner/gateway_test.go:4157:		"Cookie: cf_clearance=secret",
internal/tuner/gateway_test.go:4158:		"Authorization: Bearer upstream-token",
internal/tuner/gateway_test.go:4159:		"X-API-Key: provider-key",
internal/tuner/gateway_test.go:4160:		"X-Auth-Token: provider-token",
internal/tuner/gateway_test.go:4859:func TestCopyStreamResponseHeaders_StripsSetCookie(t *testing.T) {
internal/tuner/gateway_test.go:4863:			"Set-Cookie":        []string{"session=secret"},
internal/tuner/gateway_test.go:4870:	if got := w.Header().Get("Set-Cookie"); got != "" {
internal/tuner/gateway_test.go:4871:		t.Fatalf("Set-Cookie leaked: %q", got)
internal/tuner/gateway_test.go:5031:func TestPersistentCookieJarPersistsNewCookies(t *testing.T) {
internal/tuner/gateway_test.go:5034:	jar, err := newPersistentCookieJar(path)
internal/tuner/gateway_test.go:5036:		t.Fatalf("newPersistentCookieJar: %v", err)
internal/tuner/gateway_test.go:5042:	jar.SetCookies(u, []*http.Cookie{{
internal/tuner/gateway_test.go:5044:		Value:   "token123",
internal/tuner/gateway_test.go:5053:	loaded, err := newPersistentCookieJar(path)
internal/tuner/gateway_test.go:5055:		t.Fatalf("newPersistentCookieJar(load): %v", err)
internal/tuner/gateway_test.go:5057:	got := loaded.Cookies(u)
internal/tuner/gateway_test.go:5058:	if len(got) != 1 || got[0].Name != "cf_clearance" || got[0].Value != "token123" {
internal/tuner/gateway_test.go:5059:		t.Fatalf("loaded cookies = %#v, want cf_clearance token", got)
internal/tuner/gateway_test.go:5063:func TestPersistentCookieJarRemovesExpiredCookies(t *testing.T) {
internal/tuner/gateway_test.go:5066:	jar, err := newPersistentCookieJar(path)
internal/tuner/gateway_test.go:5068:		t.Fatalf("newPersistentCookieJar: %v", err)
internal/tuner/gateway_test.go:5074:	jar.SetCookies(u, []*http.Cookie{{
internal/tuner/gateway_test.go:5112:func TestGateway_ffmpegInputHeaderBlock_usesPerStreamAuthAndCookies(t *testing.T) {
internal/tuner/gateway_test.go:5113:	jar, err := newPersistentCookieJar("")
internal/tuner/gateway_test.go:5115:		t.Fatalf("newPersistentCookieJar: %v", err)
internal/tuner/gateway_test.go:5122:	jar.SetCookies(u, []*http.Cookie{{Name: "cf_clearance", Value: "token123", Path: "/"}})
internal/tuner/gateway_test.go:5138:	if !strings.Contains(block, "Authorization: Basic dTI6cDI=") {
internal/tuner/gateway_test.go:5141:	if !strings.Contains(block, "Cookie: cf_clearance=token123") {
internal/tuner/gateway_test.go:5146:func TestGateway_ffmpegCookiesOptionForURL(t *testing.T) {
internal/tuner/gateway_test.go:5147:	jar, err := newPersistentCookieJar("")
internal/tuner/gateway_test.go:5149:		t.Fatalf("newPersistentCookieJar: %v", err)
internal/tuner/gateway_test.go:5156:	jar.SetCookies(u, []*http.Cookie{{Name: "cf_clearance", Value: "token123", Path: "/"}})
internal/tuner/gateway_test.go:5158:	got := g.ffmpegCookiesOptionForURL(playlistURL)
internal/tuner/gateway_test.go:5159:	if !strings.Contains(got, "cf_clearance=token123;") {
internal/tuner/gateway_test.go:5160:		t.Fatalf("cookies option missing token: %q", got)
internal/tuner/gateway_debug.go:75:func debugHeaderLines(h http.Header) []string {
internal/tuner/gateway_debug.go:95:		if sensitiveHeaderName(k) {
internal/tuner/gateway_debug.go:256:	for _, line := range debugHeaderLines(w.ResponseWriter.Header()) {
