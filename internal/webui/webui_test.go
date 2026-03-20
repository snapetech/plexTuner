package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxyBase(t *testing.T) {
	tests := []struct {
		addr string
		want string
	}{
		{addr: ":5004", want: "http://127.0.0.1:5004"},
		{addr: "0.0.0.0:5004", want: "http://127.0.0.1:5004"},
		{addr: "127.0.0.1:5004", want: "http://127.0.0.1:5004"},
	}
	for _, tt := range tests {
		if got := proxyBase(tt.addr); got != tt.want {
			t.Fatalf("proxyBase(%q) = %q want %q", tt.addr, got, tt.want)
		}
	}
}

func TestProxyForwardsAPIPath(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/debug/runtime.json" {
			t.Fatalf("path=%q want /debug/runtime.json", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	s := &Server{tunerBase: upstream.URL}
	req := httptest.NewRequest(http.MethodGet, "/api/debug/runtime.json", nil)
	w := httptest.NewRecorder()
	s.proxy(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != `{"ok":true}` {
		t.Fatalf("body=%q", got)
	}
}
