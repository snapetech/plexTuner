package hdhomerun

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
)

/*
 * HDHomeRun Packet Format (from libhdhomerun):
 *
 * All values are big-endian except CRC which is little-endian.
 *
 * uint16_t  Packet type
 * uint16_t  Payload length (bytes)
 * uint8_t[] Payload data (0-n bytes)
 * uint32_t  CRC (Ethernet style 32-bit CRC)
 */

// Packet types
const (
	TypeDiscoverReq  = 0x0002
	TypeDiscoverRpy  = 0x0003
	TypeGetSetReq    = 0x0004
	TypeGetSetRpy    = 0x0005
	TypeUpgradeReq   = 0x0006
	TypeUpgradeRpy   = 0x0007
)

// Tags for TLV format
const (
	TagDeviceType      = 0x01
	TagDeviceID        = 0x02
	TagGetSetName      = 0x03
	TagGetSetValue     = 0x04
	TagErrorMessage    = 0x05
	TagTunerCount      = 0x10
	TagLineupURL       = 0x27
	TagStorageURL      = 0x28
	TagBaseURL         = 0x2A
	TagDeviceAuthStr   = 0x2B
	TagStorageID       = 0x2C
	TagMultiType       = 0x2D
)

// Device types
const (
	DeviceTypeWildcard = 0xFFFFFFFF
	DeviceTypeTuner    = 0x00000001
	DeviceTypeStorage  = 0x00000005
)

// Device ID special values
const (
	DeviceIDWildcard = 0xFFFFFFFF
)

// CRC table (IEEE 802.3)
var crc32Table *crc32.Table

func init() {
	crc32Table = crc32.MakeTable(crc32.IEEE)
}

// Packet represents a complete HDHomeRun packet
type Packet struct {
	Type    uint16
	Payload []byte
	CRC     uint32
}

// Marshal serializes the packet to bytes
func (p *Packet) Marshal() []byte {
	// Packet: type (2) + length (2) + payload + CRC (4)
	totalLen := 4 + len(p.Payload) + 4
	buf := make([]byte, totalLen)

	// Type (big-endian)
	binary.BigEndian.PutUint16(buf[0:2], p.Type)

	// Length (big-endian)
	binary.BigEndian.PutUint16(buf[2:4], uint16(len(p.Payload)))

	// Payload
	if len(p.Payload) > 0 {
		copy(buf[4:4+len(p.Payload)], p.Payload)
	}

	// CRC (little-endian) - over everything except CRC itself
	crc := crc32.Checksum(buf[:4+len(p.Payload)], crc32Table)
	binary.LittleEndian.PutUint32(buf[4+len(p.Payload):], crc)

	return buf
}

// Unmarshal parses a packet from bytes
func Unmarshal(data []byte) (*Packet, error) {
	if len(data) < 8 {
		return nil, errors.New("packet too short")
	}

	// Type
	packetType := binary.BigEndian.Uint16(data[0:2])

	// Length
	length := binary.BigEndian.Uint16(data[2:4])

	// Validate minimum size
	if len(data) < 4+int(length)+4 {
		return nil, fmt.Errorf("packet truncated: need %d, got %d", 4+int(length)+4, len(data))
	}

	// Payload
	var payload []byte
	if length > 0 {
		payload = make([]byte, length)
		copy(payload, data[4:4+length])
	}

	// CRC (received)
	receivedCRC := binary.LittleEndian.Uint32(data[4+length:])

	// CRC (calculated)
	calculatedCRC := crc32.Checksum(data[:4+length], crc32Table)

	if receivedCRC != calculatedCRC {
		return nil, fmt.Errorf("CRC mismatch: got 0x%08x, expected 0x%08x", receivedCRC, calculatedCRC)
	}

	return &Packet{
		Type:    packetType,
		Payload: payload,
		CRC:     receivedCRC,
	}, nil
}

// TLV represents a Tag-Length-Value item
type TLV struct {
	Tag    uint8
	Length uint16 // Actual length (0-32768)
	Value  []byte
}

// UnmarshalTLVs parses TLV items from payload
func UnmarshalTLVs(payload []byte) ([]TLV, error) {
	var tlvs []TLV
	pos := 0

	for pos < len(payload) {
		if pos + 2 > len(payload) {
			return nil, errors.New("truncated TLV")
		}

		tag := payload[pos]
		pos++

		// Parse length (1 or 2 bytes)
		length := uint16(payload[pos] & 0x7F)
		pos++

		if payload[pos-1]&0x80 != 0 {
			// Two-byte length
			if pos >= len(payload) {
				return nil, errors.New("truncated TLV length")
			}
			length = (length << 7) | uint16(payload[pos])
			pos++
		}

		if pos+int(length) > len(payload) {
			return nil, fmt.Errorf("truncated TLV value: need %d, have %d", length, len(payload)-pos)
		}

		value := make([]byte, length)
		copy(value, payload[pos:pos+int(length)])
		pos += int(length)

		tlvs = append(tlvs, TLV{
			Tag:    tag,
			Length: length,
			Value:  value,
		})
	}

	return tlvs, nil
}

// MarshalTLVs serializes TLV items to payload
func MarshalTLVs(tlvs []TLV) []byte {
	// Estimate size (conservative)
	size := 0
	for _, tlv := range tlvs {
		size += 2 + int(tlv.Length) // tag + length bytes + value
		if tlv.Length >= 128 {
			size++ // extra length byte
		}
	}

	buf := make([]byte, 0, size)

	for _, tlv := range tlvs {
		// Tag
		buf = append(buf, tlv.Tag)

		// Length
		if tlv.Length < 128 {
			buf = append(buf, uint8(tlv.Length))
		} else {
			// Two-byte length encoding
			buf = append(buf, uint8(0x80|((tlv.Length>>7)&0x7F)))
			buf = append(buf, uint8(tlv.Length&0x7F))
		}

		// Value
		if len(tlv.Value) > 0 {
			buf = append(buf, tlv.Value...)
		}
	}

	return buf
}

// FindTLV finds a TLV by tag
func FindTLV(tlvs []TLV, tag uint8) *TLV {
	for i := range tlvs {
		if tlvs[i].Tag == tag {
			return &tlvs[i]
		}
	}
	return nil
}

// NewDiscoverReq creates a discovery request packet
func NewDiscoverReq(deviceType, deviceID uint32) *Packet {
	tlvs := []TLV{
		{Tag: TagDeviceType, Length: 4, Value: uint32ToBytes(deviceType)},
		{Tag: TagDeviceID, Length: 4, Value: uint32ToBytes(deviceID)},
	}
	return &Packet{
		Type:    TypeDiscoverReq,
		Payload: MarshalTLVs(tlvs),
	}
}

// NewDiscoverRpy creates a discovery response packet
func NewDiscoverRpy(deviceType, deviceID uint32, tunerCount int, baseURL, lineupURL string) *Packet {
	tlvs := []TLV{
		{Tag: TagDeviceType, Length: 4, Value: uint32ToBytes(deviceType)},
		{Tag: TagDeviceID, Length: 4, Value: uint32ToBytes(deviceID)},
		{Tag: TagTunerCount, Length: 1, Value: []byte{uint8(tunerCount)}},
	}

	// Add base URL if provided
	if baseURL != "" {
		tlvs = append(tlvs, TLV{
			Tag:    TagBaseURL,
			Length: uint16(len(baseURL) + 1), // +1 for null terminator
			Value:  append([]byte(baseURL), 0),
		})
	}

	// Add lineup URL if provided
	if lineupURL != "" {
		tlvs = append(tlvs, TLV{
			Tag:    TagLineupURL,
			Length: uint16(len(lineupURL) + 1),
			Value:  append([]byte(lineupURL), 0),
		})
	}

	return &Packet{
		Type:    TypeDiscoverRpy,
		Payload: MarshalTLVs(tlvs),
	}
}

// NewGetReq creates a GET request packet
func NewGetReq(name string) *Packet {
	return &Packet{
		Type:    TypeGetSetReq,
		Payload: marshalGetSet(name, ""),
	}
}

// NewSetReq creates a SET request packet
func NewSetReq(name, value string) *Packet {
	return &Packet{
		Type:    TypeGetSetReq,
		Payload: marshalGetSet(name, value),
	}
}

// NewGetSetRpy creates a GET/SET response packet
func NewGetSetRpy(name, value string, errMsg string) *Packet {
	var tlvs []TLV

	if errMsg != "" {
		tlvs = append(tlvs, TLV{
			Tag:    TagErrorMessage,
			Length: uint16(len(errMsg) + 1),
			Value:  append([]byte(errMsg), 0),
		})
	} else {
		tlvs = append(tlvs, TLV{
			Tag:    TagGetSetName,
			Length: uint16(len(name) + 1),
			Value:  append([]byte(name), 0),
		})
		if value != "" {
			tlvs = append(tlvs, TLV{
				Tag:    TagGetSetValue,
				Length: uint16(len(value) + 1),
				Value:  append([]byte(value), 0),
			})
		}
	}

	return &Packet{
		Type:    TypeGetSetRpy,
		Payload: MarshalTLVs(tlvs),
	}
}

func marshalGetSet(name, value string) []byte {
	nameBytes := append([]byte(name), 0) // null terminated

	if value == "" {
		return []byte{
			TagGetSetName,
			uint8(len(nameBytes)),
		}
	}

	valueBytes := append([]byte(value), 0)

	tlvs := []TLV{
		{Tag: TagGetSetName, Length: uint16(len(nameBytes)), Value: nameBytes},
		{Tag: TagGetSetValue, Length: uint16(len(valueBytes)), Value: valueBytes},
	}

	return MarshalTLVs(tlvs)
}

func uint32ToBytes(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

func bytesToUint32(b []byte) uint32 {
	return binary.BigEndian.Uint32(b)
}
