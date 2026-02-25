package tuner

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/url"
	"strings"
	"time"
)

type SSDP struct {
	BaseURL      string
	DeviceID     string
	FriendlyName string
	DeviceXMLURL string
	HTTPAddr     string
}

func (s *SSDP) Run(ctx context.Context) error {
	pc, err := net.ListenPacket("udp", ":1900")
	if err != nil {
		return fmt.Errorf("listen UDP: %w", err)
	}
	defer pc.Close()

	log.Printf("SSDP listening on :1900")

	if s.FriendlyName == "" {
		s.FriendlyName = "Plex Tuner"
	}
	if s.DeviceID == "" {
		s.DeviceID = "plextuner01"
	}
	if s.DeviceXMLURL == "" && s.BaseURL != "" {
		s.DeviceXMLURL = joinDeviceXMLURL(s.BaseURL)
	}

	buf := make([]byte, 2048)
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		pc.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, addr, err := pc.ReadFrom(buf)
		if err != nil {
			netErr, ok := err.(net.Error)
			if !ok {
				continue
			}
			if netErr.Timeout() {
				continue
			}
			log.Printf("SSDP read error: %v", err)
			continue
		}

		udpAddr, ok := addr.(*net.UDPAddr)
		if !ok {
			continue
		}

		msg := string(buf[:n])
		if strings.Contains(msg, "M-SEARCH") {
			if strings.Contains(msg, "ssdp:all") || strings.Contains(msg, "urn:schemas-upnp-org:device:MediaServer") || strings.Contains(msg, "urn:schemas-upnp-org:device:Basic:1") {
				s.sendSearchResponse(pc, udpAddr)
			}
		}
	}
}

func (s *SSDP) sendSearchResponse(pc net.PacketConn, addr *net.UDPAddr) {
	if s.DeviceXMLURL == "" {
		return
	}

	resp := s.searchResponse()

	pc.WriteTo([]byte(resp), addr)
	log.Printf("SSDP: responded to M-SEARCH from %s", addr.String())
}

func StartSSDP(ctx context.Context, httpAddr, baseURL, deviceID string) error {
	deviceXMLURL := joinDeviceXMLURL(baseURL)
	if deviceXMLURL == "" {
		log.Printf("SSDP disabled: BaseURL is empty or invalid (set a reachable BaseURL for Plex discovery)")
		return nil
	}
	ssdp := &SSDP{
		BaseURL:      baseURL,
		DeviceID:     deviceID,
		FriendlyName: "Plex Tuner",
		DeviceXMLURL: deviceXMLURL,
		HTTPAddr:     httpAddr,
	}
	go func() {
		if err := ssdp.Run(ctx); err != nil {
			log.Printf("SSDP error: %v", err)
		}
	}()
	return nil
}

func (s *SSDP) searchResponse() string {
	return fmt.Sprintf(
		"HTTP/1.1 200 OK\r\n"+
			"CACHE-CONTROL: max-age=300\r\n"+
			"EXT:\r\n"+
			"LOCATION: %s\r\n"+
			"SERVER: Plex-Tuner/1.0\r\n"+
			"ST: urn:schemas-upnp-org:device:MediaServer:1\r\n"+
			"USN: uuid:%s::urn:schemas-upnp-org:device:MediaServer:1\r\n"+
			"\r\n",
		s.DeviceXMLURL, s.DeviceID,
	)
}

func joinDeviceXMLURL(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return ""
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/device.xml"
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}
