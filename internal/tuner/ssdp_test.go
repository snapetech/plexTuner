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
		{name: "existing device xml", in: "http://host:5004/device.xml", want: "http://host:5004/device.xml"},
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
	s := &Server{DeviceID: "abc123", FriendlyName: "My Tunerr"}
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
	if !strings.Contains(body, "<friendlyName>My Tunerr</friendlyName>") {
		t.Fatalf("missing friendly name: %q", body)
	}
	if !strings.Contains(body, "<UDN>uuid:abc123</UDN>") {
		t.Fatalf("missing device id UDN: %q", body)
	}
}

func TestServer_deviceXMLDefaultFriendlyName(t *testing.T) {
	s := &Server{DeviceID: "abc123"}
	req := httptest.NewRequest(http.MethodGet, "/device.xml", nil)
	w := httptest.NewRecorder()

	s.serveDeviceXML().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "<friendlyName>IPTV Tunerr</friendlyName>") {
		t.Fatalf("missing default friendly name: %q", w.Body.String())
	}
}

func TestServer_deviceXMLUsesEnvFallbacks(t *testing.T) {
	t.Setenv("IPTV_TUNERR_FRIENDLY_NAME", "Env Tunerr")
	t.Setenv("IPTV_TUNERR_DEVICE_ID", "env123")

	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/device.xml", nil)
	w := httptest.NewRecorder()

	s.serveDeviceXML().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<friendlyName>Env Tunerr</friendlyName>") {
		t.Fatalf("missing env friendly name: %q", body)
	}
	if !strings.Contains(body, "<UDN>uuid:env123</UDN>") {
		t.Fatalf("missing env device id: %q", body)
	}
}

func TestServer_deviceXMLEscapesConfiguredIdentity(t *testing.T) {
	s := &Server{
		DeviceID:     `box&1<2>`,
		FriendlyName: `AT&T <HDHR>`,
	}
	req := httptest.NewRequest(http.MethodGet, "/device.xml", nil)
	w := httptest.NewRecorder()

	s.serveDeviceXML().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("code: %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<friendlyName>AT&amp;T &lt;HDHR&gt;</friendlyName>") {
		t.Fatalf("missing escaped friendly name: %q", body)
	}
	if !strings.Contains(body, "<UDN>uuid:box&amp;1&lt;2&gt;</UDN>") {
		t.Fatalf("missing escaped device id: %q", body)
	}
}

func TestServer_deviceXMLRequiresGetOrHead(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodPost, "/device.xml", nil)
	w := httptest.NewRecorder()

	s.serveDeviceXML().ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code: %d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("allow: %q", got)
	}
}
