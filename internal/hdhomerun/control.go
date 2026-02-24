package hdhomerun

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	// ControlPort is the TCP port for HDHomeRun control channel
	ControlPort = 65001

	// MaxPacketSize is the max HDHomeRun packet size
	MaxPacketSize = 1460
)

// StreamHandler is an interface for getting stream data
type StreamHandler interface {
	GetStream(ctx context.Context, channelID string) (io.ReadCloser, error)
}

// TunerState represents the state of a tuner
type TunerState struct {
	Index       int
	Channel     string
	LockKey     int
	StreamURL   string
	InUse       bool
	Conn        net.Conn
}

// ControlServer handles TCP control connections
type ControlServer struct {
	device     *Device
	tuners     []TunerState
	listener   net.Listener
	streamFunc func(ctx context.Context, channelID string) (io.ReadCloser, error)
	mu         sync.Mutex
}

// NewControlServer creates a new control server
func NewControlServer(device *Device, tunerCount int, baseURL string, streamFunc func(ctx context.Context, channelID string) (io.ReadCloser, error)) *ControlServer {
	tuners := make([]TunerState, tunerCount)
	for i := 0; i < tunerCount; i++ {
		tuners[i] = TunerState{
			Index:     i,
			Channel:   "",
			LockKey:   0,
			StreamURL: fmt.Sprintf("hdhr://%d", i),
			InUse:    false,
		}
	}

	return &ControlServer{
		device:     device,
		tuners:     tuners,
		streamFunc: streamFunc,
	}
}

// Serve starts the control server (blocking)
func (s *ControlServer) Serve(listener net.Listener) error {
	s.listener = listener

	log.Printf("hdhomerun: control listening on TCP %s", listener.Addr().String())

	for {
		conn, err := listener.Accept()
		if err != nil {
			if s.listener == nil {
				return nil // Closed
			}
			log.Printf("hdhomerun: accept error: %v", err)
			continue
		}

		go s.handleConnection(conn)
	}
}

func (s *ControlServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	log.Printf("hdhomerun: connection from %s", conn.RemoteAddr())

	// Read loop
	readBuf := make([]byte, 4096)

	for {
		conn.SetReadDeadline(time.Now().Add(30 * time.Second)) // 30s timeout

		// Read packet header (4 bytes: type + length)
		header := make([]byte, 4)
		n, err := io.ReadFull(conn, header)
		if err != nil {
			if err != io.EOF {
				log.Printf("hdhomerun: read error: %v", err)
			}
			return
		}

		if n < 4 {
			continue
		}

		// Parse header
		packetType := binary.BigEndian.Uint16(header[0:2])
		payloadLen := binary.BigEndian.Uint16(header[2:4])

		// Read payload
		var payload []byte
		if payloadLen > 0 {
			if payloadLen > MaxPacketSize {
				log.Printf("hdhomerun: payload too large: %d", payloadLen)
				return
			}
			payload = make([]byte, payloadLen)
			n, err = io.ReadFull(conn, payload)
			if err != nil {
				log.Printf("hdhomerun: read payload error: %v", err)
				return
			}
			if n != int(payloadLen) {
				log.Printf("hdhomerun: short payload: %d vs %d", n, payloadLen)
				return
			}
		}

		// Read CRC (4 bytes)
		crcBuf := make([]byte, 4)
		_, err = io.ReadFull(conn, crcBuf)
		if err != nil {
			log.Printf("hdhomerun: read CRC error: %v", err)
			return
		}

		// Process request
		response := s.processRequest(packetType, payload, conn)

		// Send response
		responseBytes := response.Marshal()
		_, err = conn.Write(responseBytes)
		if err != nil {
			log.Printf("hdhomerun: write error: %v", err)
			return
		}
	}

}

func (s *ControlServer) processRequest(packetType uint16, payload []byte, conn net.Conn) *Packet {
	switch packetType {
	case TypeGetSetReq:
		return s.handleGetSet(payload, conn)
	default:
		log.Printf("hdhomerun: unknown packet type: 0x%04x", packetType)
		return NewGetSetRpy("", "", "Unknown packet type")
	}
}

func (s *ControlServer) handleGetSet(payload []byte, conn net.Conn) *Packet {
	tlvs, err := UnmarshalTLVs(payload)
	if err != nil {
		return NewGetSetRpy("", "", err.Error())
	}

	// Get the name
	nameTLV := FindTLV(tlvs, TagGetSetName)
	if nameTLV == nil {
		return NewGetSetRpy("", "", "Missing name")
	}
	name := string(nameTLV.Value)

	// Check if this is a SET (has value)
	valueTLV := FindTLV(tlvs, TagGetSetValue)
	var value string
	if valueTLV != nil {
		value = string(valueTLV.Value)
	}

	// Handle the property
	return s.handleProperty(name, value, conn)
}

func (s *ControlServer) handleProperty(name, value string, conn net.Conn) *Packet {
	log.Printf("hdhomerun: get/set: %s = %s", name, value)

	// Handle tuner properties
	switch {
	case strings.HasPrefix(name, "/tuner"):
		return s.handleTunerProperty(name, value, conn)
	case name == "/lineup.json":
		return NewGetSetRpy(name, "/lineup.json", "")
	case name == "/lineup_status.json":
		return NewGetSetRpy(name, "scanning=0", "")
	case name == "/discover":
		return NewGetSetRpy(name, "1", "")
	case name == "/status":
		return s.getStatus()
	default:
		return NewGetSetRpy(name, "", "Unknown property")
	}
}

func (s *ControlServer) handleTunerProperty(name, value string, conn net.Conn) *Packet {
	// Parse tuner index from name like /tuner0/channel
	var tunerIdx int
	var prop string

	if _, err := fmt.Sscanf(name, "/tuner%d/%s", &tunerIdx, &prop); err != nil {
		return NewGetSetRpy(name, "", "Invalid tuner")
	}

	if tunerIdx < 0 || tunerIdx >= len(s.tuners) {
		return NewGetSetRpy(name, "", "Invalid tuner index")
	}

	s.mu.Lock()
	tuner := &s.tuners[tunerIdx]
	s.mu.Unlock()

	switch prop {
	case "channel":
		if value != "" {
			// SET channel - e.g., "auto:program=123" or "http://provider/stream.m3u8"
			tuner.Channel = value
			tuner.InUse = true
			return NewGetSetRpy(name, value, "")
		}
		// GET channel
		return NewGetSetRpy(name, tuner.Channel, "")

	case "lock":
		if value != "" {
			// SET lock
			tuner.LockKey = 1
			return NewGetSetRpy(name, "1", "")
		}
		// GET lock
		lockStatus := "none"
		if tuner.InUse {
			lockStatus = fmt.Sprintf("lockkey=%d", tuner.LockKey)
		}
		return NewGetSetRpy(name, lockStatus, "")

	case "stream":
		if value != "" {
			// SET stream - start streaming!
			// value is the stream URL or program number
			log.Printf("hdhomerun: tuner %d starting stream: %s", tunerIdx, value)
			
			// Start streaming in background
			go s.startStream(tunerIdx, value, conn)
			
			// Return success
			return NewGetSetRpy(name, "ok", "")
		}
		// GET stream URL
		return NewGetSetRpy(name, tuner.StreamURL, "")

	case "target":
		// Set target (where to stream to)
		if value != "" {
			log.Printf("hdhomerun: tuner %d target set to: %s", tunerIdx, value)
		}
		return NewGetSetRpy(name, value, "")

	default:
		return NewGetSetRpy(name, "", "Unknown property")
	}
}

func (s *ControlServer) startStream(tunerIdx int, channelOrProgram string, conn net.Conn) {
	s.mu.Lock()
	tuner := &s.tuners[tunerIdx]
	s.mu.Unlock()

	// Extract channel ID from the channel setting
	// Could be "auto:program=123" or just "123" or a URL
	channelID := tuner.Channel
	if channelID == "" {
		channelID = channelOrProgram
	}

	// Get stream from handler
	if s.streamFunc == nil {
		log.Printf("hdhomerun: no stream function configured")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream, err := s.streamFunc(ctx, channelID)
	if err != nil {
		log.Printf("hdhomerun: failed to get stream for channel %s: %v", channelID, err)
		return
	}
	defer stream.Close()

	log.Printf("hdhomerun: streaming channel %s to %s", channelID, conn.RemoteAddr())

	// Stream data over TCP using HDHomeRun protocol
	// The format is: length-prefixed MPEG-TS packets
	buf := make([]byte, 64*1024) // 64KB buffer

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Set read deadline
		stream.SetReadDeadline(time.Now().Add(10 * time.Second))

		n, err := stream.Read(buf[6:]) // Leave 6 bytes for header
		if err != nil {
			if err == io.EOF || strings.Contains(err.Error(), "timeout") {
				log.Printf("hdhomerun: stream ended for channel %s", channelID)
			} else {
				log.Printf("hdhomerun: stream read error: %v", err)
			}
			return
		}

		if n == 0 {
			continue
		}

		// Wrap with HDHomeRun stream header
		// Format: type (0x0001 = stream data), length (2 bytes), data
		packetLen := n
		buf[0] = 0x00
		buf[1] = 0x01 // Stream data type
		binary.BigEndian.PutUint16(buf[2:4], uint16(packetLen))

		// Write header + data
		_, err = conn.Write(buf[:4+n])
		if err != nil {
			log.Printf("hdhomerun: stream write error: %v", err)
			return
		}
	}

}

func (s *ControlServer) getStatus() *Packet {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Format: key=value pairs
	status := fmt.Sprintf("deviceid=0x%08x", s.device.DeviceID)
	status += fmt.Sprintf(";tuner_count=%d", s.device.TunerCount)

	// Add tuner statuses
	for i, t := range s.tuners {
		status += fmt.Sprintf(";tuner%d_status=", i)
		if t.InUse {
			status += fmt.Sprintf("lockkey=%d", t.LockKey)
		} else {
			status += "none"
		}
	}

	return NewGetSetRpy("/status", status, "")
}

// Close stops the control server
func (s *ControlServer) Close() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}
