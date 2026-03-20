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
	"sync"
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

// parseExtraDiscoverAddrs returns optional directed discovery targets from
// IPTV_TUNERR_HDHR_DISCOVER_BROADCASTS (comma-separated literal IPs or host:port).
// IPv4 entries are sent on the IPv4 discovery socket; IPv6 entries (including
// link-local with zone, e.g. fe80::1%eth0) use a separate UDP6 socket. Hostnames
// are not resolved (only net.ParseIP literals, matching the IPv4-only behavior).
func parseExtraDiscoverAddrs() (v4 []*net.UDPAddr, v6 []*net.UDPAddr) {
	raw := strings.TrimSpace(os.Getenv("IPTV_TUNERR_HDHR_DISCOVER_BROADCASTS"))
	if raw == "" {
		return nil, nil
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		addr, ok := parseLiteralUDPAddr(part)
		if !ok {
			continue
		}
		if addr.IP.To4() != nil {
			v4 = append(v4, addr)
		} else {
			v6 = append(v6, addr)
		}
	}
	return v4, v6
}

func parseLiteralUDPAddr(part string) (*net.UDPAddr, bool) {
	part = strings.TrimSpace(part)
	host, portStr, err := net.SplitHostPort(part)
	if err != nil {
		// fe80::1%eth0:65001 and ::1:65001 may not split reliably; never apply this to IPv4 dotted quads (192.168.1.255).
		if strings.Contains(part, "%") || strings.Count(part, ":") >= 2 {
			if i := strings.LastIndex(part, ":"); i > 0 {
				tail := part[i+1:]
				if port, err := strconv.Atoi(tail); err == nil && port > 0 && port <= 65535 {
					cand := strings.TrimSpace(part[:i])
					ip := net.ParseIP(cand)
					if strings.Contains(part, "%") || (ip != nil && ip.To4() == nil) {
						return udpAddrFromLiteralHost(cand, port)
					}
				}
			}
		}
		host = part
		portStr = strconv.Itoa(DiscoverPort)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		port = DiscoverPort
	}
	host = strings.TrimSpace(host)
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	return udpAddrFromLiteralHost(host, port)
}

func udpAddrFromLiteralHost(host string, port int) (*net.UDPAddr, bool) {
	host = strings.TrimSpace(host)
	if i := strings.IndexByte(host, '%'); i >= 0 {
		addr := strings.TrimSpace(host[:i])
		zone := strings.TrimSpace(host[i+1:])
		ip := net.ParseIP(addr)
		if ip == nil || ip.To4() != nil {
			return nil, false
		}
		if zone == "" {
			return nil, false
		}
		return &net.UDPAddr{IP: ip, Port: port, Zone: zone}, true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil, false
	}
	if v4 := ip.To4(); v4 != nil {
		return &net.UDPAddr{IP: v4, Port: port}, true
	}
	return &net.UDPAddr{IP: ip, Port: port}, true
}

func discoverReadLoop(ctx context.Context, conn *net.UDPConn, seen map[uint32]struct{}, out *[]DiscoveredDevice, mu *sync.Mutex) error {
	buf := make([]byte, 4096)
	poll := 250 * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		_ = conn.SetReadDeadline(time.Now().Add(poll))
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return fmt.Errorf("hdhomerun: read: %w", err)
		}

		pkt, err := Unmarshal(buf[:n])
		if err != nil {
			continue
		}
		dev, err := ParseDiscoverReply(pkt)
		if err != nil {
			continue
		}
		mu.Lock()
		if _, dup := seen[dev.DeviceID]; dup {
			mu.Unlock()
			continue
		}
		seen[dev.DeviceID] = struct{}{}
		d := *dev
		d.SourceAddr = addr
		*out = append(*out, d)
		mu.Unlock()
	}
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
// IPTV_TUNERR_HDHR_DISCOVER_BROADCASTS directed IPv4 subnet broadcasts, and
// optional literal IPv6 targets (unicast, multicast, or link-local with zone)
// on a separate UDP6 socket when listed.
func DiscoverLAN(ctx context.Context, timeout time.Duration) ([]DiscoveredDevice, error) {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	v4Extra, v6Extra := parseExtraDiscoverAddrs()
	seen := make(map[uint32]struct{})
	var out []DiscoveredDevice
	var mu sync.Mutex

	req := NewDiscoverReq(DeviceTypeWildcard, DeviceIDWildcard)
	payload := req.Marshal()

	var wg sync.WaitGroup
	if len(v6Extra) > 0 {
		conn6, err := net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6zero, Port: 0})
		if err == nil {
			defer conn6.Close()
			for _, dst := range v6Extra {
				_, _ = conn6.WriteToUDP(payload, dst)
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = discoverReadLoop(ctx, conn6, seen, &out, &mu)
			}()
		}
	}

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return nil, fmt.Errorf("hdhomerun: listen udp: %w", err)
	}
	defer conn.Close()

	if err := enableBroadcast(conn); err != nil {
		return nil, fmt.Errorf("hdhomerun: SO_BROADCAST: %w", err)
	}

	broadcast := &net.UDPAddr{IP: net.IPv4(255, 255, 255, 255), Port: DiscoverPort}
	if _, err := conn.WriteToUDP(payload, broadcast); err != nil {
		return nil, fmt.Errorf("hdhomerun: broadcast discover: %w", err)
	}
	for _, dst := range v4Extra {
		_, _ = conn.WriteToUDP(payload, dst)
	}

	if err := discoverReadLoop(ctx, conn, seen, &out, &mu); err != nil {
		return nil, err
	}
	wg.Wait()
	return out, nil
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
