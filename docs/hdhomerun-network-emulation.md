---
id: hdhomerun-network-emulation
type: reference
status: draft
tags: [hdhomerun, network-protocol, feature, startup-race]
---

# HDHomeRun Network Protocol Emulation

This document describes the implementation of native HDHomeRun protocol emulation to bypass the HTTP-based startup race in Plex Live TV.

## Problem

The current HTTP-based tuner implementation suffers from a startup race:
1. Plex opens HTTP connection to tuner
2. Waits for 200 OK response headers
3. Waits for valid MPEG-TS body bytes
4. Plex's DASH packager needs those bytes quickly to instantiate consumer
5. If too slow: `dash_init_404`, `Failed to find consumer`

## Solution

Implement HDHomeRun protocol at the **network level** instead of HTTP:
- UDP port 65001 for device discovery (broadcast)
- TCP port 65001 for control + streaming

This uses the native HDHomeRun protocol that Plex uses for real hardware tuners.

## Protocol Details

### Packet Format

All messages use this binary format (big-endian):
```
uint16_t  Packet type
uint16_t  Payload length  
uint8_t[] Payload (0-n bytes)
uint32_t  CRC32 (little-endian)
```

### Message Types

| Type | Value | Description |
|------|-------|-------------|
| DISCOVER_REQ | 0x0002 | Find devices (UDP broadcast) |
| DISCOVER_RPY | 0x0003 | Device response |
| GETSET_REQ | 0x0004 | Get/Set property |
| GETSET_RPY | 0x0005 | Response |

### TLV Format

Discovery and control use Tag-Length-Value:
```
uint8_t  Tag
varint   Length (1-2 bytes)
uint8_t[] Value
```

### Key Tags

| Tag | Value | Description |
|-----|-------|-------------|
| DEVICE_TYPE | 0x01 | Device type (0x00000001 = tuner) |
| DEVICE_ID | 0x02 | 32-bit unique ID |
| GETSET_NAME | 0x03 | Property name |
| GETSET_VALUE | 0x04 | Property value |
| TUNER_COUNT | 0x10 | Number of tuners |
| LINEUP_URL | 0x27 | URL to lineup |
| BASE_URL | 0x2A | Base URL for HTTP |

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                      New Architecture                            │
│                                                                 │
│  ┌──────────────┐    ┌─────────────────┐    ┌───────────────┐ │
│  │ UDP:65001    │    │ TCP:65001       │    │ HTTP:5004     │ │
│  │ Discovery    │    │ Control+Stream  │    │ (optional)    │ │
│  └──────┬───────┘    └────────┬────────┘    └───────┬───────┘ │
│         │                      │                      │         │
│         └──────────────────────┴──────────────────────┘         │
│                                │                                │
│                    ┌───────────┴───────────┐                   │
│                    │   HDHomeRun Handler   │                   │
│                    │   - Packet parse      │                   │
│                    │   - Device state      │                   │
│                    │   - Stream mux        │                   │
│                    └───────────┬───────────┘                   │
│                                │                                │
│                    ┌───────────┴───────────┐                   │
│                    │   Stream Gateway     │                   │
│                    │   (existing)          │                   │
│                    └──────────────────────┘                   │
└─────────────────────────────────────────────────────────────────┘
```

## Implementation

### Files to Create

1. `internal/hdhomerun/packet.go` - Packet parsing/building
2. `internal/hdhomerun/discover.go` - UDP discovery responder  
3. `internal/hdhomerun/control.go` - TCP control channel
4. `internal/hdhomerun/stream.go` - TCP stream delivery
5. `internal/hdhomerun/server.go` - Main server combining all

### Key Properties

```go
// Device properties we need to advertise
type Device struct {
    DeviceID     uint32   // e.g., 0x12345678
    TunerCount   int      // e.g., 2
    DeviceType   uint32   // 0x00000001 = tuner
    FriendlyName string   // e.g., "PlexTuner"
    BaseURL      string   // http://192.168.1.x:5004
    LineupURL    string   // http://192.168.1.x:5004/lineup.json
}
```

### Config Options

```bash
# Enable HDHomeRun network mode (disables HTTP tuner)
PLEX_TUNER_HDHR_NETWORK_MODE=true

# Device ID (auto-generated if not set)
PLEX_TUNER_HDHR_DEVICE_ID=12345678

# Ports (defaults shown)
PLEX_TUNER_HDHR_DISCOVER_PORT=65001
PLEX_TUNER_HDHR_CONTROL_PORT=65001
```

## Properties to Support

### Discovery

- `/discover` - responds to UDP broadcast
- Returns: device_id, tuner_count, base_url, lineup_url

### Control (TCP)

| Property | Description |
|----------|-------------|
| `/lineup.json` | Get channel lineup |
| `/lineup_status.json` | Get tuner status |
| `/status` | Get device status |
| `/tuner0/channel` | Set channel (frequency/program) |
| `/tuner0/lock` | Lock status |
| `/tuner0/stream` | Stream URL |

### Stream

After tuning, MPEG-TS is delivered directly over TCP - no HTTP wrapper.

## Backward Compatibility

The HTTP tuner (port 5004) remains the default. HDHomeRun network mode is opt-in.

## Testing

1. Use `hdhomerun_config` tool to discover device
2. Use Wireshark to capture Plex → HDHomeRun traffic
3. Verify stream plays in Plex without startup errors
