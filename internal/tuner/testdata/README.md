# `internal/tuner/testdata`

Small fixtures for gateway / mux rewrite tests (no live CDN fetches).

| Fixture | Used by |
|--------|---------|
| `hls_mux_small_playlist.golden` | `TestRewriteHLSPlaylistToGatewayProxy_matchesGolden` |
| `stream_compare_hls_mux_capture_*.m3u8` | `TestRewriteHLSPlaylistToGatewayProxy_streamCompareCaptureGolden` |
| `stream_compare_dash_mux_capture_*.mpd` | `TestRewriteDASHManifestToGatewayProxy_streamCompareCaptureGolden` (expects **`IPTV_TUNERR_HLS_MUX_DASH_EXPAND_SEGMENT_TEMPLATE=1`**: template → **SegmentList**, then proxy rewrite) |

Regenerate expected files when proxy URL encoding or rewrite rules change intentionally.
