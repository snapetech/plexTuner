package hdhomerun

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/snapetech/iptvtunerr/internal/httpclient"
	"github.com/snapetech/iptvtunerr/internal/probe"
)

// DiscoveredDevice is a physical HDHomeRun (or compatible) tuner seen on the LAN
// via UDP discovery, before any HTTP calls.
type DiscoveredDevice struct {
	DeviceID   uint32
	DeviceType uint32
	TunerCount int
	BaseURL    string
	LineupURL  string
	// SourceAddr is set when the device was found via UDP (remote endpoint).
	SourceAddr *net.UDPAddr
}

// DiscoverJSON is the HTTP discover.json document from a SiliconDust-style device.
type DiscoverJSON struct {
	DeviceID     string `json:"DeviceID"`
	DeviceAuth   string `json:"DeviceAuth,omitempty"`
	FriendlyName string `json:"FriendlyName"`
	BaseURL      string `json:"BaseURL"`
	LineupURL    string `json:"LineupURL"`
	TunerCount   int    `json:"TunerCount"`
}

// LineupDoc is the HTTP lineup.json payload from a real HDHomeRun.
type LineupDoc struct {
	ScanInProgress int                `json:"ScanInProgress"`
	ScanPossible   int                `json:"ScanPossible"`
	Source         string             `json:"Source"`
	Channels       []probe.LineupItem `json:"Channels"`
}

// ParseDiscoverReply parses a UDP discovery reply (TypeDiscoverRpy).
func ParseDiscoverReply(packet *Packet) (*DiscoveredDevice, error) {
	if packet.Type != TypeDiscoverRpy {
		return nil, fmt.Errorf("hdhomerun: expected discover reply (0x%04x), got 0x%04x", TypeDiscoverRpy, packet.Type)
	}
	tlvs, err := UnmarshalTLVs(packet.Payload)
	if err != nil {
		return nil, fmt.Errorf("hdhomerun: TLV: %w", err)
	}
	d := &DiscoveredDevice{}
	if t := FindTLV(tlvs, TagDeviceType); t != nil && len(t.Value) >= 4 {
		d.DeviceType = bytesToUint32(t.Value)
	}
	if t := FindTLV(tlvs, TagDeviceID); t != nil && len(t.Value) >= 4 {
		d.DeviceID = bytesToUint32(t.Value)
	}
	if t := FindTLV(tlvs, TagTunerCount); t != nil && len(t.Value) >= 1 {
		d.TunerCount = int(t.Value[0])
	}
	if t := FindTLV(tlvs, TagBaseURL); t != nil && len(t.Value) > 0 {
		d.BaseURL = strings.TrimRight(string(trimNull(t.Value)), "/")
	}
	if t := FindTLV(tlvs, TagLineupURL); t != nil && len(t.Value) > 0 {
		d.LineupURL = strings.TrimRight(string(trimNull(t.Value)), "/")
	} else if d.BaseURL != "" {
		d.LineupURL = d.BaseURL + "/lineup.json"
	}
	return d, nil
}

func trimNull(b []byte) []byte {
	for len(b) > 0 && b[len(b)-1] == 0 {
		b = b[:len(b)-1]
	}
	return b
}

// extraDiscoverBroadcastAddrs returns optional directed IPv4 broadcast targets from
// IPTV_TUNERR_HDHR_DISCOVER_BROADCASTS (comma-separated IPs or host:port).
// Useful when 255.255.255.255 is filtered but e.g. 192.168.1.255 works.
func extraDiscoverBroadcastAddrs() []*net.UDPAddr {
	raw := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HDHR_DISCOVER_BROADCASTS"))
	if raw == "" {
		return nil
	}
	var out []*net.UDPAddr
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		host, portStr, err := net.SplitHostPort(part)
		if err != nil {
			host = part
			portStr = strconv.Itoa(DiscoverPort)
		}
		port, err := strconv.Atoi(portStr)
		if err != nil || port <= 0 || port > 65535 {
			port = DiscoverPort
		}
		ip := net.ParseIP(host)
		if ip == nil {
			continue
		}
		v4 := ip.To4()
		if v4 == nil {
			continue
		}
		out = append(out, &net.UDPAddr{IP: v4, Port: port})
	}
	return out
}

func enableBroadcast(c *net.UDPConn) error {
	raw, err := c.SyscallConn()
	if err != nil {
		return err
	}
	var opErr error
	err = raw.Control(func(fd uintptr) {
		opErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
	})
	if err != nil {
		return err
	}
	return opErr
}

// DiscoverLAN broadcasts an HDHomeRun discovery request and collects responses until timeout.
// Uses IPv4 global broadcast (255.255.255.255:DiscoverPort) plus any
// IPTV_TUNERR_HDHR_DISCOVER_BROADCASTS directed subnet broadcasts.
func DiscoverLAN(ctx context.Context, timeout time.Duration) ([]DiscoveredDevice, error) {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("hdhomerun: listen udp: %w", err)
	}
	defer conn.Close()

	if err := enableBroadcast(conn); err != nil {
		return nil, fmt.Errorf("hdhomerun: SO_BROADCAST: %w", err)
	}

	req := NewDiscoverReq(DeviceTypeWildcard, DeviceIDWildcard)
	payload := req.Marshal()

	broadcast := &net.UDPAddr{IP: net.IPv4(255, 255, 255, 255), Port: DiscoverPort}
	if _, err := conn.WriteToUDP(payload, broadcast); err != nil {
		return nil, fmt.Errorf("hdhomerun: broadcast discover: %w", err)
	}
	for _, dst := range extraDiscoverBroadcastAddrs() {
		_, _ = conn.WriteToUDP(payload, dst)
	}

	seen := make(map[uint32]struct{})
	var out []DiscoveredDevice

	buf := make([]byte, 4096)
	poll := 250 * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			return out, nil
		default:
		}
		_ = conn.SetReadDeadline(time.Now().Add(poll))
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return nil, fmt.Errorf("hdhomerun: read: %w", err)
		}

		pkt, err := Unmarshal(buf[:n])
		if err != nil {
			continue
		}
		dev, err := ParseDiscoverReply(pkt)
		if err != nil {
			continue
		}
		if _, dup := seen[dev.DeviceID]; dup {
			continue
		}
		seen[dev.DeviceID] = struct{}{}
		dev.SourceAddr = addr
		out = append(out, *dev)
	}
}

// DiscoverURLFromBase returns the discover.json URL for a device base URL.
func DiscoverURLFromBase(base string) string {
	b := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(strings.ToLower(b), "/discover.json") {
		return b
	}
	return b + "/discover.json"
}

// LineupURLFromBase returns the lineup.json URL for a device base URL.
func LineupURLFromBase(base string) string {
	b := strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(strings.ToLower(b), "/lineup.json") {
		return b
	}
	return b + "/lineup.json"
}

// FetchDiscoverJSON GETs discover.json from a device base URL (e.g. http://192.168.1.100).
func FetchDiscoverJSON(ctx context.Context, client *http.Client, baseURL string) (*DiscoverJSON, error) {
	if client == nil {
		client = httpclient.WithTimeout(15 * time.Second)
	}
	u := DiscoverURLFromBase(baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("hdhomerun: discover.json %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var d DiscoverJSON
	if err := json.NewDecoder(resp.Body).Decode(&d); err != nil {
		return nil, fmt.Errorf("hdhomerun: decode discover.json: %w", err)
	}
	return &d, nil
}

// FetchLineupJSON GETs lineup.json from a base URL or full lineup URL.
func FetchLineupJSON(ctx context.Context, client *http.Client, baseOrLineupURL string) (*LineupDoc, error) {
	if client == nil {
		client = httpclient.Default()
	}
	u := LineupURLFromBase(baseOrLineupURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("hdhomerun: lineup.json %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var doc LineupDoc
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("hdhomerun: decode lineup.json: %w", err)
	}
	return &doc, nil
}
