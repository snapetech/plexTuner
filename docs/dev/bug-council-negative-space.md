# IPTVtunerr Council Negative-Space Gate

The scanner sees call sites that exist. This document declares call sites that
must continue to exist because they protect trust boundaries in the Plex Live
TV proxy and tuner operator surface.

| Boundary | Source | Sink file(s) | Required validator |
| --- | --- | --- | --- |
| Plex owner-token elevation | Plex client token on proxied Live TV requests | `internal/plexlabelproxy/proxy.go` | `canElevate` |
| Proxy source identity | Caddy/cloudflared forwarded headers | `internal/plexlabelproxy/proxy.go` | `trustedCloudflareConnectingIP` |
| Tokenless session recovery | Plex segment/timeline requests missing tokens | `internal/plexlabelproxy/proxy.go` | `sessionRecordMatchesSource` |
| Tuner operator/debug HTTP | Browser/LAN requests to `/ui`, `/debug`, and `/ops/actions` | `internal/tuner/operator_ui.go` | `operatorUIAllowed` |
| Evidence/debug file tokens | Request/channel identifiers used in evidence filenames | `internal/tuner/gateway_debug.go` | `sanitizeFileToken` |
| Diagnostic header redaction | Stream-attempt/debug records that describe upstream request headers | `internal/tuner/gateway_attempts.go` | `sensitiveHeaderName` |

Each row has two gates:

1. `scripts/check-council-negative-space.sh` asserts the validator symbol is
   still present in the sink file.
2. `scripts/check-remediation-baseline.sh` asserts the same symbol is anchored
   in the remediation baseline, so the negative-space row cannot silently lose
   its paired gate.
