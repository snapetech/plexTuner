package hdhomerun

import (
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

const (
	// DiscoverPort is the UDP port for HDHomeRun device discovery
	DiscoverPort = 65001

	// BroadcastAddr is the broadcast address for discovery
	BroadcastAddr = "255.255.255.255"
)

// Device represents an HDHomeRun tuner device
type Device struct {
	DeviceID     uint32
	TunerCount   int
	DeviceType   uint32 // 0x00000001 = tuner
	FriendlyName string
	BaseURL      string
	LineupURL    string
}

// DiscoverServer handles UDP broadcast discovery
type DiscoverServer struct {
	device *Device
	conn   *net.UDPConn
}

// NewDiscoverServer creates a new discovery server
func NewDiscoverServer(device *Device, port int) (*DiscoverServer, error) {
	addr := &net.UDPAddr{
		Port: port,
		IP:   net.IPv4zero,
	}

	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, fmt.Errorf("listen UDP: %w", err)
	}

	return &DiscoverServer{
		device: device,
		conn:   conn,
	}, nil
}

// Run starts the discovery server (blocking)
func (s *DiscoverServer) Run() error {
	log.Printf("hdhomerun: discovery listening on UDP port %d", s.conn.LocalAddr().(*net.UDPAddr).Port)

	buf := make([]byte, 4096)

	for {
		s.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, clientAddr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue // Loop to refresh deadline
			}
			return fmt.Errorf("read: %w", err)
		}

		if n < 4 {
			continue // Too short to be valid
		}

		// Parse the discovery request
		packet, err := Unmarshal(buf[:n])
		if err != nil {
			log.Printf("hdhomerun: discover: parse error from %s: %v", clientAddr, err)
			continue
		}

		if packet.Type != TypeDiscoverReq {
			log.Printf("hdhomerun: discover: unexpected packet type 0x%04x from %s", packet.Type, clientAddr)
			continue
		}

		// Parse TLVs
		tlvs, err := UnmarshalTLVs(packet.Payload)
		if err != nil {
			log.Printf("hdhomerun: discover: TLV parse error from %s: %v", clientAddr, err)
			continue
		}

		// Check device type filter
		var reqDeviceType uint32 = DeviceTypeWildcard
		var reqDeviceID uint32 = DeviceIDWildcard

		if dt := FindTLV(tlvs, TagDeviceType); dt != nil && len(dt.Value) >= 4 {
			reqDeviceType = bytesToUint32(dt.Value)
		}
		if di := FindTLV(tlvs, TagDeviceID); di != nil && len(di.Value) >= 4 {
			reqDeviceID = bytesToUint32(di.Value)
		}

		// Check if we match the request
		if reqDeviceType != DeviceTypeWildcard && reqDeviceType != s.device.DeviceType {
			continue // Not the right type
		}
		if reqDeviceID != DeviceIDWildcard && reqDeviceID != s.device.DeviceID {
			continue // Not the right device
		}

		// Send discovery response
		response := NewDiscoverRpy(
			s.device.DeviceType,
			s.device.DeviceID,
			s.device.TunerCount,
			s.device.BaseURL,
			s.device.LineupURL,
		)

		responseBytes := response.Marshal()

		// Send back to client
		_, err = s.conn.WriteToUDP(responseBytes, clientAddr)
		if err != nil {
			log.Printf("hdhomerun: discover: write error to %s: %v", clientAddr, err)
		}

		log.Printf("hdhomerun: discover: responded to %s (device_id=0x%08x)", clientAddr, s.device.DeviceID)
	}
}

// Close stops the discovery server
func (s *DiscoverServer) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// CreateDefaultDevice creates a device with sensible defaults
func CreateDefaultDevice(deviceID uint32, tunerCount int, baseURL string) *Device {
	if deviceID == 0 {
		// Generate a random device ID if not provided
		deviceID = 0x12345678 // Placeholder - could use random
	}

	// Try to get friendly name from environment, fallback to default
	friendlyName := os.Getenv("PLEX_TUNER_HDHR_FRIENDLY_NAME")
	if friendlyName == "" {
		friendlyName = os.Getenv("PLEX_TUNER_FRIENDLY_NAME")
	}
	if friendlyName == "" {
		friendlyName = "PlexTuner-HDHR"
	}

	return &Device{
		DeviceID:     deviceID,
		TunerCount:   tunerCount,
		DeviceType:   DeviceTypeTuner,
		FriendlyName: friendlyName,
		BaseURL:      baseURL,
		LineupURL:    baseURL + "/lineup.json",
	}
}
