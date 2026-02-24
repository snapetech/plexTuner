package hdhomerun

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
)

const (
	// ControlPort is the TCP port for HDHomeRun control channel
	ControlPort = 65001

	// MaxPacketSize is the max HDHomeRun packet size
	MaxPacketSize = 1460
)

// TunerState represents the state of a tuner
type TunerState struct {
	Index       int
	Channel     string
	LockKey     int
	StreamURL   string
	InUse       bool
}

// ControlServer handles TCP control connections
type ControlServer struct {
	device    *Device
	tuners    []TunerState
	listener  net.Listener
	streamBuf chan []byte // Stream data to send
}

// NewControlServer creates a new control server
func NewControlServer(device *Device, tunerCount int, baseURL string) *ControlServer {
	tuners := make([]TunerState, tunerCount)
	for i := 0; i < tunerCount; i++ {
		tuners[i] = TunerState{
			Index:     i,
			Channel:   "",
			LockKey:   0,
			StreamURL: fmt.Sprintf("%s/stream/%%d", baseURL),
			InUse:    false,
		}
	}

	return &ControlServer{
		device:   device,
		tuners:   tuners,
		streamBuf: make(chan []byte, 10),
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

// SetStreamData sets the stream data to send
func (s *ControlServer) SetStreamData(data []byte) {
	select {
	case s.streamBuf <- data:
	default:
		// Buffer full, drop
	}
}

func (s *ControlServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	log.Printf("hdhomerun: connection from %s", conn.RemoteAddr())

	// Read loop
	readBuf := make([]byte, 4096)

	for {
		conn.SetReadDeadline(nil) // Blocking read

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
		response := s.processRequest(packetType, payload)

		// Send response
		responseBytes := response.Marshal()
		_, err = conn.Write(responseBytes)
		if err != nil {
			log.Printf("hdhomerun: write error: %v", err)
			return
		}

		// After successful GET/SET, check if we need to start streaming
		// This is where we'd start the MPEG-TS stream
	}

}

func (s *ControlServer) processRequest(packetType uint16, payload []byte) *Packet {
	switch packetType {
	case TypeGetSetReq:
		return s.handleGetSet(payload)
	default:
		log.Printf("hdhomerun: unknown packet type: 0x%04x", packetType)
		return NewGetSetRpy("", "", "Unknown packet type")
	}
}

func (s *ControlServer) handleGetSet(payload []byte) *Packet {
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
	return s.handleProperty(name, value)
}

func (s *ControlServer) handleProperty(name, value string) *Packet {
	log.Printf("hdhomerun: get/set: %s = %s", name, value)

	// Handle tuner properties
	switch {
	case strings.HasPrefix(name, "/tuner"):
		return s.handleTunerProperty(name, value)
	case name == "/lineup.json":
		// This is typically served via HTTP, but we can handle it here too
		return NewGetSetRpy(name, "/lineup.json", "")
	case name == "/lineup_status.json":
		return NewGetSetRpy(name, "scanning=0", "")
	case name == "/status":
		return s.getStatus()
	case name == "/discover":
		return NewGetSetRpy(name, "1", "")
	default:
		return NewGetSetRpy(name, "", "Unknown property")
	}
}

func (s *ControlServer) handleTunerProperty(name, value string) *Packet {
	// Parse tuner index from name like /tuner0/channel
	var tunerIdx int
	var prop string

	if _, err := fmt.Sscanf(name, "/tuner%d/%s", &tunerIdx, &prop); err != nil {
		return NewGetSetRpy(name, "", "Invalid tuner")
	}

	if tunerIdx < 0 || tunerIdx >= len(s.tuners) {
		return NewGetSetRpy(name, "", "Invalid tuner index")
	}

	tuner := &s.tuners[tunerIdx]

	switch prop {
	case "channel":
		if value != "" {
			// SET channel
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
			// SET stream - this should trigger stream start
			tuner.StreamURL = fmt.Sprintf(tuner.StreamURL, tunerIdx)
			return NewGetSetRpy(name, tuner.StreamURL, "")
		}
		// GET stream URL
		return NewGetSetRpy(name, tuner.StreamURL, "")

	default:
		return NewGetSetRpy(name, "", "Unknown property")
	}
}

func (s *ControlServer) getStatus() *Packet {
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
