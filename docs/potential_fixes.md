---
id: potential-fixes
type: reference
status: draft
tags: [startup-race, plex-live-tv, troubleshooting, improvements]
---

# Potential Fixes for Plex Live TV Startup Race

This document catalogs potential solutions for the **Plex Live TV startup race** issue, where Plex opens a tuner session but fails to find the consumer (`dash_init_404`, `Failed to find consumer`). The core problem is that Plex's internal DASH/HLS packager cannot instantiate its consumer before receiving valid MPEG-TS bytes.

## Current Implementation

The existing solution in [`internal/tuner/gateway.go`](internal/tuner/gateway.go) includes:

1. **Startup Gate** (lines 1762-1917): Buffers `startupMin` bytes (default 64KB) before streaming to Plex, waiting up to `startupTimeoutMs` (default 12s) for "good" TS (contains IDR frame + AAC audio)

2. **Bootstrap TS**: Generates deterministic MPEG-TS with ffmpeg as a fallback when startup gate times out

3. **Null TS Keepalive**: Sends null packets (PID 0x1FFF) while waiting for upstream to produce valid bytes

4. **PAT+PMT Keepalive**: Sends real program structure packets (PAT PID 0x0000, PMT PID 0x1000) so Plex's DASH packager has program map info

## Potential Fixes

---

### 1. Client-Aware Adaptation

**Description**: Detect Plex client type via User-Agent header and dynamically adjust startup parameters.

**Current Code Evidence**:
- Plex client hints are already parsed in [`gateway.go`](internal/tuner/gateway.go:670-683):
  ```go
  SessionIdentifier: get("X-Plex-Session-Identifier", ...)
  ClientIdentifier:  get("X-Plex-Client-Identifier", ...)
  Product:           get("X-Plex-Product"),
  Platform:          get("X-Plex-Platform", "X-Plex-Client-Platform"),
  Device:            get("X-Plex-Device", "X-Plex-Device-Name"),
  ```

- Client resolution exists at lines 688-820 with `resolvePlexClient()` and `looksLikePlexWeb()`

**Implementation**:
- Map known Plex clients to timing profiles:
  - **Plex Web**: Most sensitive to startup timing
  - **Roku**: Known for strict timing requirements  
  - **Apple TV**: More tolerant
  - **Shield TV**: Generally reliable
  - **Fire TV**: Variable by generation

- Adjust these parameters per client:
  - `WEBSAFE_STARTUP_MIN_BYTES`: 64KB (web) → 128KB (Roku)
  - `WEBSAFE_STARTUP_TIMEOUT_MS`: 12000 → 20000 (slower clients)
  - Prefer PAT+PMT keepalive for stricter clients

**Pros**:
- Already has client detection infrastructure
- Targeted optimization per client type

**Cons**:
- Requires maintaining client compatibility matrix
- Some clients may not send identifiable User-Agent

---

### 2. Pre-Segment Fetching

**Description**: Prefetch HLS manifest and first 3-5 segments before connecting to Plex, then stream them contiguously.

**Current Implementation Gap**:
- Current startup gate buffers ffmpeg output
- No pre-fetch of HLS segments before ffmpeg starts

**Implementation**:
```
1. Plex requests /stream/1
2. Fetch HLS manifest from provider
3. Download first N segments (e.g., segments 0-4)
4. Start ffmpeg, feeding it from cached segments
5. Once startup gate passes, switch to live ffmpeg output
```

**Technical Details**:
- HLS segments typically 2-10 seconds each
- First segments often larger (keyframe-aligned)
- Need to handle segment encryption (EXT-X-KEY)

**Pros**:
- Eliminates network round-trip from critical path
- More predictable first bytes

**Cons**:
- Increases memory usage during startup
- Adds complexity for segment caching
- May not help if ffmpeg is the bottleneck

---

### 3. Adaptive Transcode Priority for Startup

**Description**: Temporarily force transcoding during startup, then downgrade to remux after startup gate passes.

**Current Code**:
- Transcode mode is set globally via `StreamTranscodeMode` ("off", "on", "auto")
- The `auto` mode uses ffprobe to detect Plex-friendly codecs

**Implementation**:
```go
// Pseudo-code
if transcodeMode == "off" {
    // Force transcode during startup
    startupProfile := "libx264/aac"  
    actualProfile = startupProfile
}
// After startup gate passes:
// If original mode was "off" and codec is Plex-friendly, switch to remux
```

**Why It Might Help**:
- Transcoded streams have deterministic output timing
- Remuxed HLS depends on upstream segment availability
- Transcoded output is continuous MPEG-TS vs segment-based

**Pros**:
- More predictable timing during critical startup window
- Falls back to optimized mode after initialization

**Cons**:
- Higher CPU usage during startup
- May increase startup latency
- Complexity in profile switching

---

### 4. Metrics-Driven Auto-Tuning

**Description**: Add structured metrics collection and use data to automatically tune parameters.

**What to Measure**:
- Startup time percentiles (p50, p95, p99) per client type
- Startup failure rate per client/channel/time
- Time to first valid TS packet
- Keepalive effectiveness

**Implementation**:
```go
// Add metrics
var (
    startupDuration = prometheus.NewHistogramVec(...)
    startupFailures = prometheus.NewCounterVec(...)
)

// In stream handler
timer := prometheus.NewTimer(startupDuration.WithLabelValues(clientType, channelName))
defer timer.ObserveDuration()
```

**Tools to Use**:
- Prometheus client for Go (`prometheus/client_golang`)
- Export at `/metrics` endpoint

**Pros**:
- Data-driven optimization decisions
- Identifies problematic clients/channels
- Enables alerting on failure spikes

**Cons**:
- Adds dependencies
- Requires operational overhead

---

### 5. Simulated "Live" Buffer Offset

**Description**: Start streaming 2-5 seconds behind live edge instead of at live edge.

**Implementation**:
- HLS: Set `-hls_start_offset` in ffmpeg
- For 3 second offset at 2s segments: `-hls_start_offset 3`
- This gives Plex's packager time to initialize while stream still appears "live"

**Configuration**:
```bash
PLEX_TUNER_STARTUP_LIVE_OFFSET_SECONDS=3
```

**Trade-offs**:
- Small delay (acceptable for live TV)
- Must calculate based on segment duration

---

### 6. HTTP/2 Multiplexing

**Description**: Upgrade from HTTP/1.1 to HTTP/2 for upstream connections.

**Current State**:
- Uses standard Go HTTP client (HTTP/1.1 by default)
- Each backup URL attempt creates new connection

**Implementation**:
```go
// In httpclient or gateway
transport := &http2.Transport{}
client := &http.Client{Transport: transport}
```

**Pros**:
- Reduced connection overhead
- Better handling of multiple backup URLs
- Header compression

**Cons**:
- Single long-lived stream - minimal benefit
- May not work with all IPTV providers
- Adds HTTP/2 dependency

---

### 7. Graceful Degradation Chain

**Description**: Automatic fallback chain: primary URL → backup URLs → transcode → different protocol.

**Current Behavior**:
- Backup URLs are tried in order (lines 93-101 in main.go)
- If all fail, stream fails
- No automatic fallback to transcoding if remux fails

**Implementation**:
```
Priority chain:
1. Primary URL + remux (current default)
2. Backup URL 1 + remux
3. Backup URL 2 + remux
4. Primary URL + transcode
5. Bootstrap TS (last resort)
```

**Pros**:
- Maximizes chances of successful stream
- Leverages existing backup URL infrastructure

**Cons**:
- Increases startup latency if multiple fallbacks needed
- Complex error handling

---

### 8. Alternative Protocol Support

**Description**: Support additional streaming protocols beyond HLS.

**Candidates**:
- **MPEG-DASH**: Some providers offer this instead of HLS
- **RTMP**: Older but reliable
- **UDP multicast**: For local network sources
- **SRT**: Modern secure reliable transport

**Implementation**:
- Detect available protocols from provider
- Add protocol-specific handlers in gateway
- Test compatibility with Plex

**Pros**:
- Different timing characteristics
- May work better with certain providers

**Cons**:
- Significant development effort
- Plex must support the output format

---

### 9. Improved Bootstrap TS Generation

**Description**: Enhance the deterministic bootstrap TS generation for timeout fallback.

**Current Implementation**:
- [`writeBootstrapTS()`](internal/tuner/gateway.go:1689-1713) generates basic MPEG-TS
- Uses ffmpeg with `-f mpegts` and null input

**Improvements**:
- Include PAT/PMT with proper program numbers
- Add video (black frame with IDR) + audio (silence)
- Make it longer/deterministic
- Include PCR timestamps for DASH compatibility

**Pros**:
- More realistic fallback when upstream is slow
- Better chance of satisfying Plex's packager

**Cons**:
- Slightly larger bootstrap payload
- More ffmpeg overhead

---

### 10. Diagnostic and Tuner Self-Test on Startup

**Description**: Run a quick self-test when Plex connects, rather than immediately trying the live stream.

**Implementation**:
1. When Plex opens `/stream/<id>`:
2. Send a known test pattern (small TS with PAT/PMT/IDR)
3. Verify Plex acknowledged the stream
4. Then switch to live stream

**Why It Might Help**:
- Separates "can Plex see this?" from "here's live content"
- May pre-initialize Plex's consumer

**Pros**:
- Early detection of Plex compatibility issues
- Gives Plex more time

**Cons**:
- Adds latency
- Complex state management

---

## Recommendations

Based on the current codebase and implementation:

1. **Highest Priority**: Client-Aware Adaptation - leverage existing client detection infrastructure
2. **Second Priority**: Improved Bootstrap TS - extend current fallback mechanism  
3. **Third Priority**: Metrics-Driven Auto-Tuning - enables data-driven decisions
4. **Lower Priority**: Pre-Segment Fetching, HTTP/2 - more complex with uncertain benefit

The PAT+PMT keepalive feature (already implemented) is theoretically the most promising current approach, but may need tuning of the interval (`WEBSAFE_PROGRAM_KEEPALIVE_MS`) for different clients.

## References

- [`docs/runbooks/plextuner-troubleshooting.md`](docs/runbooks/plextuner-troubleshooting.md) - Full troubleshooting guide
- [`internal/tuner/gateway.go`](internal/tuner/gateway.go:1724-1737) - Current startup parameters
- [.env.example](.env.example) - All configuration options
