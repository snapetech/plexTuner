#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

INCLUDE_MAC=0
INCLUDE_WINDOWS_PACKAGE=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --include-mac)
      INCLUDE_MAC=1
      shift
      ;;
    --include-windows-package)
      INCLUDE_WINDOWS_PACKAGE=1
      shift
      ;;
    *)
      echo "usage: $0 [--include-mac] [--include-windows-package]" >&2
      exit 2
      ;;
  esac
done

step() {
  printf '[release-readiness] ==> %s\n' "$*"
}

run() {
  step "$1"
  shift
  "$@"
}

run "repo verify" ./scripts/verify

run "focused parity suites" go test ./internal/tuner -run 'TestServer_Xtream(PlayerAPI_LiveCategories|PlayerAPI_VODAndSeries|PlayerAPI_ShortEPG|Exports_M3UAndXMLTV|LiveProxy|LiveProxy_VirtualChannel|MovieAndSeriesProxy|XtreamEntitlementsLimitOutput|programmingEndpoints|programmingBrowse|programmingChannelDetail|diagnosticsHarnessActions|diagnosticsWorkflowAndEvidenceAction|virtualChannelRulesAndPreview)$' -count=1

run "programming + backup preference suites" go test ./internal/programming ./internal/tuner -run 'Test(UpdateRecipeMutations|BuildBackupGroupsAndCollapse|BuildBackupGroupsAndCollapse_WithPreferences|Server_programmingEndpoints)$' -count=1

run "provider-account + shared-relay suites" go test ./internal/tuner -run 'Test(Gateway_(stream_rollsAcrossThreeXtreamPathAccounts|stream_twoChannelsPreferDifferentXtreamPathAccounts|stream_threeChannelsUseThreeXtreamPathAccounts|sharedRelaySessionFanout|tryServeSharedRelay|relayHLSAsTS_survivesPlaylistConcurrencyRetry|shouldPreferGoRelayForHLSRemux|relaySuccessfulHLSUpstream_crossHostPlaylistPrefersGoBeforeFFmpegFailure|relayHLSWithFFmpeg_nonTranscodeFirstBytesTimeout|stream_hlsDeadRemuxFallsBackQuickly)|Server_SharedRelayReport)' -count=1

run "webui auth + proxy suites" go test ./internal/webui -count=1

run "vod-webdav suites" go test ./internal/vodwebdav ./cmd/iptv-tunerr -count=1

if [[ "$INCLUDE_MAC" == "1" ]]; then
  run "macOS bare-metal smoke" ./scripts/mac-baremetal-smoke.sh
else
  step "macOS bare-metal smoke (skipped; pass --include-mac to run host proof)"
fi

if [[ "$INCLUDE_WINDOWS_PACKAGE" == "1" ]]; then
  run "windows package smoke-prep" ./scripts/windows-baremetal-package.sh
else
  step "windows package smoke-prep (skipped; pass --include-windows-package to run packaging proof)"
fi

printf '[release-readiness] ==> all requested checks OK\n'
