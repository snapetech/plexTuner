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
	Addr                string
	BaseURL             string
	TunerCount          int
	StreamBufferBytes   int    // 0 = no buffer; -1 = auto; e.g. 2097152 for 2 MiB
	StreamTranscodeMode string // "off" | "on" | "auto"
	Channels            []catalog.LiveChannel
	ProviderUser        string
	ProviderPass        string
	XMLTVSourceURL      string
	XMLTVTimeout        time.Duration
	EpgPruneUnlinked    bool // when true, guide.xml and /live.m3u only include channels with tvg-id
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
		Channels:            s.Channels,
		ProviderUser:        s.ProviderUser,
		ProviderPass:        s.ProviderPass,
		TunerCount:          s.TunerCount,
		StreamBufferBytes:   s.StreamBufferBytes,
		StreamTranscodeMode: s.StreamTranscodeMode,
	}
	xmltv := &XMLTV{
		Channels:         s.Channels,
		EpgPruneUnlinked: s.EpgPruneUnlinked,
		SourceURL:        s.XMLTVSourceURL,
		SourceTimeout:    s.XMLTVTimeout,
	}
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
	srv := &http.Server{Addr: addr, Handler: logRequests(mux)}

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

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *loggingResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *loggingResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytes += n
	return n, err
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &loggingResponseWriter{ResponseWriter: w}
		next.ServeHTTP(lw, r)
		status := lw.status
		if status == 0 {
			status = http.StatusOK
		}
		log.Printf(
			"http: %s %s status=%d bytes=%d dur=%s ua=%q remote=%s",
			r.Method, r.URL.Path, status, lw.bytes, time.Since(start).Round(time.Millisecond), r.UserAgent(), r.RemoteAddr,
		)
	})
}
