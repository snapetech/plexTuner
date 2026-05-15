#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

failures=0
pass() { printf 'PASS %s\n' "$1"; }
fail() { printf 'FAIL %s\n' "$1" >&2; failures=$((failures + 1)); }

require_file() {
  local path="$1"
  local label="$2"
  [[ -f "$path" ]] && pass "$label" || fail "$label: missing $path"
}

require_pattern() {
  local pattern="$1"
  local path="$2"
  local label="$3"
  if rg -n -U --pcre2 --hidden --glob '!.git/**' "$pattern" "$path" >/dev/null; then
    pass "$label"
  else
    fail "$label"
  fi
}

require_absent_pattern() {
  local pattern="$1"
  local path="$2"
  local label="$3"
  local hit_file
  hit_file="$(mktemp)"
  if rg -n -U --pcre2 --hidden --glob '!.git/**' --glob '!vendor/**' "$pattern" "$path" >"$hit_file" 2>/dev/null; then
    fail "$label"
    sed 's/^/  /' "$hit_file" >&2
  else
    pass "$label"
  fi
  rm -f "$hit_file"
}

require_file "docs/dev/bug-council-scan-registry.md" "council scan registry exists"
require_file "docs/dev/bug-burndown-ledger.md" "council burndown ledger exists"
require_file "docs/dev/bug-council-severity-schema.md" "council severity/confidence schema exists"
require_file "docs/dev/bug-council-sibling-search.md" "council sibling-search rule exists"
require_file "docs/dev/bug-council-negative-space.md" "council negative-space gate doc exists"
require_file "docs/dev/bug-council-behavior-pinning.md" "council behavior-pinning pattern exists"
require_file "scripts/scan-bug-council-candidates.sh" "candidate scanner exists"
require_file "scripts/check-council-sweep-counts.sh" "sweep-count drift gate exists"
require_file "scripts/check-council-negative-space.sh" "negative-space gate script exists"
require_file "scripts/run-bug-council-all-phases.sh" "all-phases council runner exists"
require_file "scripts/check-bug-council-all-phases.sh" "all-phases council runner registration gate exists"
require_file "scripts/run-council-active-bughunt.sh" "active bughunt runner exists"
require_file "scripts/check-council-active-backlog.sh" "active backlog gate exists"
require_file "docs/dev/bug-council-active-backlog.md" "active backlog exists"
require_pattern "not proof of no bugs" "scripts/run-council-active-bughunt.sh" "active bughunt runner states reports are not no-bug proofs"
require_pattern "Every active-bughunt section must have a row" "docs/dev/bug-council-active-backlog.md" "active backlog documents section coverage rule"
require_pattern "check-council-active-backlog.sh" "scripts/run-bug-council-all-phases.sh" "all-phases runner checks active backlog"

if bash scripts/check-bug-council-all-phases.sh >/dev/null 2>&1; then
  pass "all-phases council runner is registered"
else
  fail "all-phases council runner is not registered; run scripts/check-bug-council-all-phases.sh for details"
fi

if bash scripts/check-council-active-backlog.sh >/dev/null 2>&1; then
  pass "active backlog matches active bughunt report"
else
  fail "active backlog does not match active bughunt report; run scripts/check-council-active-backlog.sh for details"
fi

if bash scripts/check-council-negative-space.sh >/dev/null 2>&1; then
  pass "negative-space gate passes"
else
  fail "negative-space gate failed; run scripts/check-council-negative-space.sh for details"
fi

require_pattern "func \\(p \\*Proxy\\) canElevate" "internal/plexlabelproxy/proxy.go" "canElevate remains owner-token gate"
require_pattern "connectionHeaderNames" "internal/plexlabelproxy/proxy.go" "hop-by-hop header names are stripped"
require_pattern "trustedCloudflareConnectingIP" "internal/plexlabelproxy/proxy.go" "Cloudflare source header trust is explicit"
require_pattern "sessionRecordMatchesSource" "internal/plexlabelproxy/proxy.go" "tokenless recovery is source-bound"
require_pattern "pathCanCarryTunerEntitlement" "internal/plexlabelproxy/proxy.go" "allowTuners rewrite is path-scoped"
require_pattern "operatorUIAllowed" "internal/tuner/operator_ui.go" "operator/debug UI has locality gate"
require_pattern "func repoScriptPath\\(name string\\) \\(string, error\\)" "internal/tuner/server_diagnostics_recordings.go" "diagnostic harness script paths are validated"
require_pattern "createDownloadFile" "internal/materializer/download.go" "materializer downloads validate destination files before overwrite"
require_pattern "os\\.MkdirAll\\(filepath\\.Dir\\(destPath\\), 0o700\\)" "internal/materializer/download.go" "materializer download directories are private on disk"
require_pattern "writeProviderEPGCacheFile" "internal/tuner/epg_pipeline.go" "provider EPG disk cache writes are private and symlink-aware"
require_pattern "redactProviderEPGDiagnosticText" "internal/tuner/epg_pipeline.go" "provider EPG diagnostics redact credential-bearing URLs"
require_pattern "inMemoryGuideEmpty" "internal/tuner/epg_pipeline.go" "provider EPG startup uses stale disk cache before slow network probes"
require_pattern "ensureLeaseDir" "internal/tuner/gateway_shared_leases.go" "shared provider-account lease directory is private"
require_pattern "writeProviderSharedLeaseFile" "internal/tuner/gateway_shared_leases.go" "shared provider-account lease files are private and symlink-aware"
require_pattern "writeAutopilotStateFile" "internal/tuner/autopilot.go" "autopilot state writes are private and symlink-aware"
require_pattern "writeAbuseStateFile" "internal/plexlabelproxy/proxy.go" "Plex proxy abuse state writes are private and symlink-aware"
require_pattern "openCatchupSpoolFile" "internal/tuner/catchup_record_resilient.go" "catchup recorder spool writes are private and symlink-aware"
require_pattern "writePrivateCatchupArtifact" "internal/tuner/catchup_capsules_export.go" "catchup capsule artifacts write privately and symlink-aware"
require_pattern "preparePublishDestination" "internal/tuner/catchup_record_publish.go" "recorded catchup publish media writes are symlink-aware"
require_pattern "writeRegistrationStateFile" "internal/emby/state.go" "Emby registration state writes are private and symlink-aware"
require_pattern "writeDeckStateFile" "internal/webui/webui.go" "WebUI deck state writes are private and symlink-aware"
require_pattern "sanitizeFileToken" "internal/tuner/gateway_debug.go" "debug evidence file tokens are sanitized"
require_pattern "os\\.OpenFile\\(path, os\\.O_WRONLY\\|os\\.O_CREATE\\|os\\.O_TRUNC, 0o600\\)" "internal/tuner/gateway_debug.go" "debug evidence files are private on disk"
require_pattern "sensitiveHeaderName" "internal/tuner/gateway_attempts.go" "diagnostic header summaries have credential-shaped header redaction"
require_pattern "redactOperatorDiagnosticText" "internal/tuner/diagnostic_redaction.go" "operator diagnostic text has credential redaction"
require_pattern "firstSeg\\), time\\.Since" "internal/tuner/gateway_stream_response.go" "successful HLS startup first segment log is redacted"
require_pattern "redactPlexDiagnosticText" "internal/plex/redaction.go" "Plex API diagnostic response bodies have credential redaction"
require_pattern "sanitizeLogoFilename" "internal/webui/apiv2_logos.go" "WebUI logo upload filenames are constrained"
require_pattern "os\\.MkdirAll\\(dir, 0o700\\)" "internal/webui/apiv2_logos.go" "WebUI logo directory is private on disk"
require_pattern "os\\.CreateTemp\\(dir, \"\\.upload-\\*\\.tmp\"\\)" "internal/webui/apiv2_logos.go" "WebUI logo uploads avoid following destination symlinks"
require_pattern "redactRuntimeSnapshotText" "cmd/iptv-tunerr/cmd_runtime_server.go" "runtime snapshot redacts credential-shaped URL text"
require_pattern "logExternalXMLTVEnabled" "cmd/iptv-tunerr/cmd_runtime.go" "external XMLTV startup logging uses redaction helper"
require_pattern "redactDebugBundleText" "cmd/iptv-tunerr/cmd_debug_bundle.go" "debug-bundle fetched payloads have credential redaction"
require_pattern "redactedHooks" "internal/eventhooks/eventhooks.go" "event hook reports redact configured hook secrets"
require_pattern "sensitiveHookHeaderName" "internal/eventhooks/eventhooks.go" "event hook reports detect sensitive custom headers"
require_pattern "xtreamPathSegment" "internal/tuner/server_xtream.go" "Xtream proxy URLs encode path segments"
require_pattern "r\\.URL\\.EscapedPath\\(\\)" "internal/tuner/server_xtream.go" "Xtream proxy parses escaped path segments"
require_pattern "xtreamVODDenyLiteralPrivateUpstream" "internal/tuner/server_xtream.go" "Xtream VOD proxy has literal private upstream guard"
require_pattern "IPTV_TUNERR_XTREAM_VOD_DENY_LITERAL_PRIVATE_UPSTREAM\", true" "internal/tuner/server_xtream.go" "Xtream VOD private upstream guard defaults on"
require_pattern "virtualChannelDenyLiteralPrivateUpstream" "internal/tuner/server_virtual_channel_streams.go" "virtual-channel playback has literal private upstream guard"
require_pattern "IPTV_TUNERR_VIRTUAL_CHANNEL_DENY_LITERAL_PRIVATE_UPSTREAM\", true" "internal/tuner/server_virtual_channel_streams.go" "virtual-channel private upstream guard defaults on"
require_pattern "sanitizeVirtualChannelRecoveryEvent" "internal/tuner/server_virtual_channel_streams.go" "virtual-channel recovery events are sanitized before reuse"
require_pattern "os\\.WriteFile\\(path, data, 0o600\\)" "internal/tuner/server_virtual_channel_streams.go" "virtual-channel recovery state is private on disk"
require_pattern "getenvBool\\(\"IPTV_TUNERR_HLS_MUX_DENY_LITERAL_PRIVATE_UPSTREAM\", true\\)" "internal/tuner/gateway_hls.go" "literal private mux upstream block defaults on"
require_pattern "operatorUIAllowed\\(w, r\\)" "internal/tuner/server_diagnostics_recordings.go" "HLS mux demo uses operator locality gate"
require_pattern "os\\.MkdirAll\\(filepath\\.Join\\(outDir, sub\\), 0o700\\)" "internal/tuner/server_diagnostics_recordings.go" "evidence intake directories are private on disk"

require_pattern "TestProxy_DoesNotTrustHopByHopHeaderTokenForElevation" "internal/plexlabelproxy/proxy_test.go" "hop-by-hop token behavior test exists"
require_pattern "TestProxy_DoesNotRecoverTokenlessLiveTVSegmentFromDifferentSource" "internal/plexlabelproxy/proxy_test.go" "tokenless cross-source behavior test exists"
require_pattern "TestProxy_RejectsSpoofedCFConnectingIPWhenForwardedPeerIsLAN" "internal/plexlabelproxy/proxy_test.go" "source spoof behavior test exists"
require_pattern "TestProxy_DoesNotRewriteAllowTunersOnUnrelatedPaths" "internal/plexlabelproxy/proxy_test.go" "entitlement rewrite behavior test exists"
require_pattern "TestSanitizeHeaderSummary_redactsCredentialShapedHeaders" "internal/tuner/gateway_test.go" "diagnostic header summary redaction behavior test exists"
require_pattern "TestDebugHeaderLines_redactsCredentialShapedHeaders" "internal/tuner/gateway_test.go" "debug header log redaction behavior test exists"
require_pattern "TestCappedBodyTee_CreatesPrivateEvidenceFiles" "internal/tuner/gateway_test.go" "debug evidence private file behavior test exists"
require_pattern "TestStreamAttemptBuilder_RedactsDiagnosticURLFields" "internal/tuner/gateway_test.go" "stream attempt URL/error redaction behavior test exists"
require_pattern "TestGateway_applyUpstreamRequestHeaders_stillForwardsCredentialHeaders" "internal/tuner/gateway_test.go" "upstream credential header forwarding behavior test exists"
require_pattern "TestGateway_ffmpegInputHeaderBlock_stillIncludesCredentialHeaders" "internal/tuner/gateway_test.go" "ffmpeg credential header forwarding behavior test exists"
require_pattern "TestGateway_hlsMuxSeg_literalPrivateBlocked_returnsForbidden" "internal/tuner/gateway_test.go" "literal private mux upstream default-block behavior test exists"
require_pattern "TestGateway_hlsMuxSeg_literalPrivateBlockCanBeDisabledForLabUse" "internal/tuner/gateway_test.go" "literal private mux lab override preservation test exists"
require_pattern "TestGateway_hlsPlaylistStartupLogRedactsFirstSegmentURL" "internal/tuner/gateway_test.go" "successful HLS startup first segment log redaction behavior test exists"
require_pattern "TestRedactPlexDiagnosticText_RedactsResponseBodySecrets" "internal/plex/redaction_test.go" "Plex API response body redaction behavior test exists"
require_pattern "TestV2LogosUploadCreatesPrivateDirAndDoesNotFollowExistingSymlink" "internal/webui/webui_test.go" "WebUI logo symlink overwrite regression test exists"
require_pattern "TestV2LogoItemStoredTraversalFilenameDoesNotEscapeLogoDir" "internal/webui/webui_test.go" "WebUI logo stored traversal regression test exists"
require_pattern "TestServer_hlsMuxWebDemoRequiresLocalOperatorAccess" "internal/tuner/server_test.go" "HLS mux demo locality behavior test exists"
require_pattern "TestRedactOperatorDiagnosticText_redactsURLsHeadersAndTokens" "internal/tuner/server_test.go" "operator diagnostic text redaction behavior test exists"
require_pattern "TestRunDiagnosticsHarnessAction_redactsCapturedStdout" "internal/tuner/server_test.go" "harness stdout redaction behavior test exists"
require_pattern "TestRunDiagnosticsHarnessAction_rejectsScriptPathEscape" "internal/tuner/server_test.go" "harness script path escape behavior test exists"
require_pattern "TestDownloadToFile_refusesDestinationSymlink" "internal/materializer/materializer_test.go" "materializer symlink overwrite behavior test exists"
require_pattern "TestWriteProviderEPGCacheFile_refusesSymlinkOverwrite" "internal/tuner/epg_pipeline_test.go" "provider EPG cache symlink overwrite behavior test exists"
require_pattern "TestFetchProviderXMLTVLogsRedactProviderCredentials" "internal/tuner/epg_pipeline_test.go" "provider EPG diagnostic redaction behavior test exists"
require_pattern "TestFetchProviderXMLTVUsesDiskCacheWhenMemoryGuideEmpty" "internal/tuner/epg_pipeline_test.go" "provider EPG startup stale-cache behavior test exists"
require_pattern "TestProviderSharedLeaseManagerCreatesPrivateLeaseFiles" "internal/tuner/gateway_test.go" "shared provider-account private lease file behavior test exists"
require_pattern "TestWriteProviderSharedLeaseFileRefusesSymlinkOverwrite" "internal/tuner/gateway_test.go" "shared provider-account symlink overwrite behavior test exists"
require_pattern "TestAutopilotStoreRefusesSymlinkOverwrite" "internal/tuner/autopilot_test.go" "autopilot symlink overwrite behavior test exists"
require_pattern "TestProxy_AbuseStateRefusesSymlinkOverwrite" "internal/plexlabelproxy/proxy_test.go" "Plex proxy abuse state symlink overwrite behavior test exists"
require_pattern "TestSpoolCopyFromHTTPRefusesSymlinkedSpool" "internal/tuner/catchup_record_resilient_test.go" "catchup spool symlink overwrite behavior test exists"
require_pattern "TestRecordCatchupCapsuleResilientCreatesPrivateArtifacts" "internal/tuner/catchup_record_resilient_test.go" "catchup recorder private artifact behavior test exists"
require_pattern "TestSaveCatchupCapsuleLanesRefusesSymlinkedArtifact" "internal/tuner/catchup_capsules_export_test.go" "catchup capsule export symlink overwrite behavior test exists"
require_pattern "TestSaveCatchupCapsuleLibraryLayoutRefusesSymlinkedArtifact" "internal/tuner/catchup_publish_test.go" "catchup capsule publish symlink overwrite behavior test exists"
require_pattern "TestLinkOrCopyFileRefusesSymlinkedDestination" "internal/tuner/catchup_publish_test.go" "recorded catchup publish symlink overwrite behavior test exists"
require_pattern "TestPublishRecordedCatchupItemCreatesPrivateArtifacts" "internal/tuner/catchup_publish_test.go" "recorded catchup publish private artifact behavior test exists"
require_pattern "TestSaveState_refusesSymlinkOverwrite" "internal/emby/state_test.go" "Emby registration state symlink overwrite behavior test exists"
require_pattern "TestPersistStateRefusesSymlinkOverwrite" "internal/webui/webui_test.go" "WebUI deck state symlink overwrite behavior test exists"
require_pattern "evidence subdir .*want 0700" "internal/tuner/server_test.go" "evidence intake private directory behavior test exists"
require_pattern "TestBuildRuntimeSnapshot_RedactsCredentialBearingRuntimeURLs" "cmd/iptv-tunerr/cmd_runtime_test.go" "runtime snapshot URL redaction behavior test exists"
require_pattern "TestLogExternalXMLTVEnabled_RedactsCredentialBearingURL" "cmd/iptv-tunerr/cmd_runtime_test.go" "external XMLTV log redaction behavior test exists"
require_pattern "TestRedactDebugBundleText_RedactsDiagnosticPayloadSecrets" "cmd/iptv-tunerr/cmd_debug_bundle_test.go" "debug-bundle payload redaction behavior test exists"
require_pattern "TestRunImportCookiesDryRunRedactsCookieValues" "cmd/iptv-tunerr/cmd_cookie_import_test.go" "cookie import dry-run redaction behavior test exists"
require_pattern "TestReportRedactsWebhookURLsAndHeaders" "internal/eventhooks/eventhooks_test.go" "event hook report redaction behavior test exists"
require_pattern "TestServer_XtreamPathSegmentsAreEscapedAndParsedLosslessly" "internal/tuner/server_test.go" "Xtream path segment round-trip behavior test exists"
require_pattern "TestServer_XtreamVODProxyBlocksLiteralPrivateUpstreamByDefault" "internal/tuner/server_test.go" "Xtream VOD private upstream default-block behavior test exists"
require_pattern "TestServer_XtreamVODProxyPrivateUpstreamBlockCanBeDisabledForLabUse" "internal/tuner/server_test.go" "Xtream VOD private upstream lab override behavior test exists"
require_pattern "TestServer_virtualChannelStreamBlocksLiteralPrivateUpstreamByDefault" "internal/tuner/server_test.go" "virtual-channel private upstream default-block behavior test exists"
require_pattern "TestServer_virtualChannelStreamPrivateUpstreamBlockCanBeDisabledForLabUse" "internal/tuner/server_test.go" "virtual-channel private upstream lab override behavior test exists"
require_pattern "TestServer_virtualChannelStreamFallsBackBeforeProbeWhenPrivateUpstreamBlocked" "internal/tuner/server_test.go" "virtual-channel private upstream recovery behavior test exists"
require_pattern "TestServer_virtualRecoveryStateRedactsLoadedLegacyURLs" "internal/tuner/server_test.go" "virtual-channel recovery state legacy URL redaction test exists"

secret_pattern='-----BEGIN (RSA |DSA |EC |OPENSSH |PGP )?PRIVATE KEY-----|gh[pousr]_[A-Za-z0-9_]{36,}|xox[baprs]-[A-Za-z0-9-]{20,}|AKIA[0-9A-Z]{16}'
require_absent_pattern "$secret_pattern" "." "tracked text files do not contain high-confidence private keys or platform tokens"

if [[ "$failures" -gt 0 ]]; then
  printf '\n%d remediation baseline check(s) failed.\n' "$failures" >&2
  exit 1
fi

printf '\nAll remediation baseline checks passed.\n'
