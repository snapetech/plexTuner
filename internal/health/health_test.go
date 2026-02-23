package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckProvider_ok(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	ctx := context.Background()
	if err := CheckProvider(ctx, srv.URL); err != nil {
		t.Fatalf("CheckProvider: %v", err)
	}
}

func TestCheckProvider_badStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	ctx := context.Background()
	err := CheckProvider(ctx, srv.URL)
	if err == nil {
		t.Fatal("expected error for 401")
	}
}

func TestCheckProvider_emptyURL(t *testing.T) {
	ctx := context.Background()
	err := CheckProvider(ctx, "")
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestCheckEndpoints_ok(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/discover.json", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/lineup.json", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/guide.xml", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ctx := context.Background()
	if err := CheckEndpoints(ctx, srv.URL); err != nil {
		t.Fatalf("CheckEndpoints: %v", err)
	}
}

func TestCheckEndpoints_missing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	ctx := context.Background()
	err := CheckEndpoints(ctx, srv.URL)
	if err == nil {
		t.Fatal("expected error for 404")
	}
}
