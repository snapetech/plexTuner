package tuner

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJoinDeviceXMLURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "invalid", in: "://bad", want: ""},
		{name: "host only", in: "http://192.168.1.10:5004", want: "http://192.168.1.10:5004/device.xml"},
		{name: "trim slash", in: "http://host:5004/", want: "http://host:5004/device.xml"},
		{name: "path base", in: "http://host:5004/tuner", want: "http://host:5004/tuner/device.xml"},
		{name: "strip query", in: "http://host:5004?t=1", want: "http://host:5004/device.xml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := joinDeviceXMLURL(tt.in); got != tt.want {
				t.Fatalf("joinDeviceXMLURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSSDP_searchResponse(t *testing.T) {
	s := &SSDP{
		DeviceXMLURL: "http://10.0.0.5:5004/device.xml",
		DeviceID:     "abc123",
	}

	resp := s.searchResponse()
	if !strings.Contains(resp, "HTTP/1.1 200 OK\r\n") {
		t.Fatalf("missing status line: %q", resp)
	}
	if !strings.Contains(resp, "LOCATION: http://10.0.0.5:5004/device.xml\r\n") {
		t.Fatalf("missing LOCATION header: %q", resp)
	}
	if !strings.Contains(resp, "USN: uuid:abc123::urn:schemas-upnp-org:device:MediaServer:1\r\n") {
		t.Fatalf("missing USN header: %q", resp)
	}
	if !strings.HasSuffix(resp, "\r\n\r\n") {
		t.Fatalf("response must end with CRLF CRLF: %q", resp)
	}
}

func TestServer_deviceXML(t *testing.T) {
	s := &Server{DeviceID: "abc123"}
	req := httptest.NewRequest(http.MethodGet, "/device.xml", nil)
	w := httptest.NewRecorder()

	s.serveDeviceXML().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/xml" {
		t.Fatalf("content-type: %q", got)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<friendlyName>Plex Tuner</friendlyName>") {
		t.Fatalf("missing friendly name: %q", body)
	}
	if !strings.Contains(body, "<UDN>uuid:abc123</UDN>") {
		t.Fatalf("missing device id UDN: %q", body)
	}
}
