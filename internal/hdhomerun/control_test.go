package hdhomerun

import (
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestControlServer_getDiscoverJSONEscapesFriendlyName(t *testing.T) {
	s := &ControlServer{
		device: &Device{
			DeviceID:     0xdeadbeef,
			FriendlyName: `AT&T "Box"`,
			BaseURL:      "http://127.0.0.1:5004",
			TunerCount:   2,
		},
	}

	raw := s.getDiscoverJSON()
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("unmarshal discover.json: %v; raw=%s", err, raw)
	}
	if body["FriendlyName"] != `AT&T "Box"` {
		t.Fatalf("friendly name=%v raw=%s", body["FriendlyName"], raw)
	}
	if body["DeviceID"] != "deadbeef" {
		t.Fatalf("device id=%v raw=%s", body["DeviceID"], raw)
	}
	if body["LineupURL"] != "http://127.0.0.1:5004/lineup.json" {
		t.Fatalf("lineup_url=%v raw=%s", body["LineupURL"], raw)
	}
	if body["BaseURL"] != "http://127.0.0.1:5004" {
		t.Fatalf("base_url=%v raw=%s", body["BaseURL"], raw)
	}
}

func TestControlServer_getDiscoverJSONNormalizesLineupURL(t *testing.T) {
	s := &ControlServer{
		device: &Device{
			DeviceID:     0xdeadbeef,
			FriendlyName: "Friendly",
			BaseURL:      "http://127.0.0.1:5004/",
			TunerCount:   2,
		},
	}

	raw := s.getDiscoverJSON()
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("unmarshal discover.json: %v; raw=%s", err, raw)
	}
	if body["LineupURL"] != "http://127.0.0.1:5004/lineup.json" {
		t.Fatalf("lineup_url=%v raw=%s", body["LineupURL"], raw)
	}
}

func TestControlServer_httpResponseForPath(t *testing.T) {
	s := &ControlServer{
		device: &Device{
			DeviceID:     0xdeadbeef,
			FriendlyName: "Friendly",
			BaseURL:      "http://127.0.0.1:5004",
			TunerCount:   2,
		},
	}

	status, contentType, body, allow := s.httpResponseForRequest("GET", "/lineup.json")
	if status != "HTTP/1.1 200 OK" || contentType != "application/json" || body != "[]" {
		t.Fatalf("lineup.json response = %q %q %q", status, contentType, body)
	}
	if allow != "" {
		t.Fatalf("lineup.json allow=%q", allow)
	}
	var lineup []map[string]interface{}
	if err := json.Unmarshal([]byte(body), &lineup); err != nil {
		t.Fatalf("lineup.json unmarshal: %v", err)
	}

	status, contentType, body, allow = s.httpResponseForRequest("GET", "/lineup_status.json")
	if status != "HTTP/1.1 200 OK" || contentType != "application/json" {
		t.Fatalf("lineup_status response = %q %q", status, contentType)
	}
	if allow != "" {
		t.Fatalf("lineup_status allow=%q", allow)
	}
	var lineupStatus map[string]interface{}
	if err := json.Unmarshal([]byte(body), &lineupStatus); err != nil {
		t.Fatalf("lineup_status unmarshal: %v", err)
	}
	if _, ok := lineupStatus["Channels"]; ok {
		t.Fatalf("lineup_status should not masquerade as lineup.json: %s", body)
	}

	status, contentType, body, allow = s.httpResponseForRequest("GET", "/missing")
	if status != "HTTP/1.1 404 Not Found" || contentType != "text/plain" || !strings.Contains(body, "404 Not Found") {
		t.Fatalf("missing response = %q %q %q", status, contentType, body)
	}
	if allow != "" {
		t.Fatalf("missing allow=%q", allow)
	}
}

func TestControlServer_httpResponseForRequestMethodHandling(t *testing.T) {
	s := &ControlServer{
		device: &Device{
			DeviceID:     0xdeadbeef,
			FriendlyName: "Friendly",
			BaseURL:      "http://127.0.0.1:5004",
			TunerCount:   2,
		},
	}

	status, contentType, body, allow := s.httpResponseForRequest("HEAD", "/discover.json")
	if status != "HTTP/1.1 200 OK" || contentType != "application/json" || body == "" {
		t.Fatalf("head discover response = %q %q %q", status, contentType, body)
	}
	if allow != "" {
		t.Fatalf("head discover allow=%q", allow)
	}

	status, contentType, body, allow = s.httpResponseForRequest("POST", "/discover.json")
	if status != "HTTP/1.1 405 Method Not Allowed" || contentType != "text/plain" || !strings.Contains(body, "method not allowed") {
		t.Fatalf("post discover response = %q %q %q", status, contentType, body)
	}
	if allow != "GET, HEAD" {
		t.Fatalf("post discover allow=%q", allow)
	}

	status, contentType, body, allow = s.httpResponseForRequest("POST", "/missing")
	if status != "HTTP/1.1 404 Not Found" || contentType != "text/plain" || !strings.Contains(body, "404 Not Found") {
		t.Fatalf("post missing response = %q %q %q", status, contentType, body)
	}
	if allow != "" {
		t.Fatalf("post missing allow=%q", allow)
	}
}

func TestControlServer_handleConnectionBinaryRequestUsesSniffedHeader(t *testing.T) {
	s := &ControlServer{
		device: &Device{
			DeviceID:     0xdeadbeef,
			FriendlyName: "Friendly",
			BaseURL:      "http://127.0.0.1:5004",
			TunerCount:   2,
		},
	}

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	go s.handleConnection(serverConn)

	if err := clientConn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	if _, err := clientConn.Write(NewGetReq("/status").Marshal()); err != nil {
		t.Fatalf("write request: %v", err)
	}

	raw := readControlPacket(t, clientConn)
	resp, err := Unmarshal(raw)
	if err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.Type != TypeGetSetRpy {
		t.Fatalf("response type = 0x%04x", resp.Type)
	}
	tlvs, err := UnmarshalTLVs(resp.Payload)
	if err != nil {
		t.Fatalf("unmarshal tlvs: %v", err)
	}
	nameTLV := FindTLV(tlvs, TagGetSetName)
	if nameTLV == nil {
		t.Fatalf("missing name tlv")
	}
	if got := string(trimNull(nameTLV.Value)); got != "/status" {
		t.Fatalf("name = %q", got)
	}
	valueTLV := FindTLV(tlvs, TagGetSetValue)
	if valueTLV == nil {
		t.Fatalf("missing value tlv")
	}
	if got := string(trimNull(valueTLV.Value)); !strings.Contains(got, "deviceid=0xdeadbeef") || !strings.Contains(got, "tuner_count=2") {
		t.Fatalf("status value = %q", got)
	}
}

func TestControlServer_handleConnectionRecognizesPutAsHTTP(t *testing.T) {
	s := &ControlServer{
		device: &Device{
			DeviceID:     0xdeadbeef,
			FriendlyName: "Friendly",
			BaseURL:      "http://127.0.0.1:5004",
			TunerCount:   2,
		},
	}

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	go s.handleConnection(serverConn)

	if err := clientConn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set deadline: %v", err)
	}
	if _, err := io.WriteString(clientConn, "PUT /discover.json HTTP/1.1\r\nHost: 127.0.0.1\r\n\r\n"); err != nil {
		t.Fatalf("write request: %v", err)
	}
	resp, err := io.ReadAll(clientConn)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if !strings.HasPrefix(string(resp), "HTTP/1.1 405 Method Not Allowed\r\n") {
		t.Fatalf("unexpected response: %q", string(resp))
	}
	if !strings.Contains(string(resp), "\r\nAllow: GET, HEAD\r\n") {
		t.Fatalf("missing Allow header in response: %q", string(resp))
	}
}

func TestNewGetReqIncludesPropertyName(t *testing.T) {
	pkt, err := Unmarshal(NewGetReq("/status").Marshal())
	if err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	tlvs, err := UnmarshalTLVs(pkt.Payload)
	if err != nil {
		t.Fatalf("unmarshal tlvs: %v", err)
	}
	nameTLV := FindTLV(tlvs, TagGetSetName)
	if nameTLV == nil {
		t.Fatalf("missing name tlv")
	}
	if got := string(trimNull(nameTLV.Value)); got != "/status" {
		t.Fatalf("name = %q", got)
	}
}

func readControlPacket(t *testing.T, conn net.Conn) []byte {
	t.Helper()

	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		t.Fatalf("read header: %v", err)
	}
	payloadLen := int(header[2])<<8 | int(header[3])
	raw := make([]byte, 4+payloadLen+4)
	copy(raw, header)
	if _, err := io.ReadFull(conn, raw[4:]); err != nil {
		t.Fatalf("read payload+crc: %v", err)
	}
	return raw
}
