package tuner

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
)

// Server runs the HDHR emulator + XMLTV + stream gateway.
type Server struct {
	Addr             string
	BaseURL          string
	TunerCount       int
	Channels         []catalog.LiveChannel
	ProviderUser     string
	ProviderPass     string
	EpgPruneUnlinked bool // when true, guide.xml and /live.m3u only include channels with tvg-id
}

// Run blocks until ctx is cancelled or the server fails to start. On shutdown it stops
// accepting new connections and waits briefly for in-flight requests to finish.
func (s *Server) Run(ctx context.Context) error {
	hdhr := &HDHR{
		BaseURL:    s.BaseURL,
		TunerCount: s.TunerCount,
		Channels:   s.Channels,
	}
	gateway := &Gateway{
		Channels:     s.Channels,
		ProviderUser: s.ProviderUser,
		ProviderPass: s.ProviderPass,
		TunerCount:   s.TunerCount,
	}
	xmltv := &XMLTV{Channels: s.Channels, EpgPruneUnlinked: s.EpgPruneUnlinked}
	m3uServe := &M3UServe{BaseURL: s.BaseURL, Channels: s.Channels, EpgPruneUnlinked: s.EpgPruneUnlinked}

	mux := http.NewServeMux()
	mux.Handle("/discover.json", hdhr)
	mux.Handle("/lineup_status.json", hdhr)
	mux.Handle("/lineup.json", hdhr)
	mux.Handle("/guide.xml", xmltv)
	mux.Handle("/live.m3u", m3uServe)
	mux.Handle("/stream/", gateway)

	addr := s.Addr
	if addr == "" {
		addr = ":5004"
	}
	srv := &http.Server{Addr: addr, Handler: mux}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Tuner listening on %s (BaseURL %s)", addr, s.BaseURL)
		serverErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	case <-ctx.Done():
		log.Print("Shutting down tuner ...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("Tuner shutdown: %v", err)
		}
		<-serverErr
		return nil
	}
}
