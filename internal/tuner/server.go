package tuner

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/plextuner/plex-tuner/internal/catalog"
	"github.com/plextuner/plex-tuner/internal/httpclient"
)

// Server runs the HDHR emulator + XMLTV + stream gateway.
// Handlers are kept so UpdateChannels can refresh the channel list without restart.
type Server struct {
	Addr                string
	BaseURL             string
	TunerCount          int
	DeviceID            string // HDHomeRun discover.json; set from PLEX_TUNER_DEVICE_ID
	StreamBufferBytes   int    // 0 = no buffer; -1 = auto; e.g. 2097152 for 2 MiB
	StreamTranscodeMode string // "off" | "on" | "auto"
	Channels            []catalog.LiveChannel
	ProviderUser        string
	ProviderPass        string
	XMLTVSourceURL      string
	XMLTVTimeout        time.Duration
	EpgPruneUnlinked    bool // when true, guide.xml and /live.m3u only include channels with tvg-id

	hdhr     *HDHR
	gateway  *Gateway
	xmltv    *XMLTV
	m3uServe *M3UServe
}

// UpdateChannels updates the channel list for all handlers so -refresh can serve new lineup without restart.
func (s *Server) UpdateChannels(live []catalog.LiveChannel) {
	s.Channels = live
	if s.hdhr != nil {
		s.hdhr.Channels = live
	}
	if s.gateway != nil {
		s.gateway.Channels = live
	}
	if s.xmltv != nil {
		s.xmltv.Channels = live
	}
	if s.m3uServe != nil {
		s.m3uServe.Channels = live
	}
}

// Run blocks until ctx is cancelled or the server fails to start. On shutdown it stops
// accepting new connections and waits briefly for in-flight requests to finish.
func (s *Server) Run(ctx context.Context) error {
	hdhr := &HDHR{
		BaseURL:    s.BaseURL,
		TunerCount: s.TunerCount,
		DeviceID:   s.DeviceID,
		Channels:   s.Channels,
	}
	s.hdhr = hdhr
	defaultProfile := defaultProfileFromEnv()
	overridePath := os.Getenv("PLEX_TUNER_PROFILE_OVERRIDES_FILE")
	overrides, err := loadProfileOverridesFile(overridePath)
	if err != nil {
		log.Printf("Profile overrides disabled: load %q failed: %v", overridePath, err)
	} else if len(overrides) > 0 {
		log.Printf("Profile overrides loaded: %d entries from %s (default=%s)", len(overrides), overridePath, defaultProfile)
	} else {
		log.Printf("Profile overrides: none (default=%s)", defaultProfile)
	}
	gateway := &Gateway{
		Channels:            s.Channels,
		ProviderUser:        s.ProviderUser,
		ProviderPass:        s.ProviderPass,
		TunerCount:          s.TunerCount,
		StreamBufferBytes:   s.StreamBufferBytes,
		StreamTranscodeMode: s.StreamTranscodeMode,
		DefaultProfile:      defaultProfile,
		ProfileOverrides:    overrides,
	}
	if gateway.Client == nil {
		gateway.Client = httpclient.ForStreaming()
	}
	s.gateway = gateway
	xmltv := &XMLTV{
		Channels:         s.Channels,
		EpgPruneUnlinked: s.EpgPruneUnlinked,
		SourceURL:        s.XMLTVSourceURL,
		SourceTimeout:    s.XMLTVTimeout,
	}
	s.xmltv = xmltv
	m3uServe := &M3UServe{BaseURL: s.BaseURL, Channels: s.Channels, EpgPruneUnlinked: s.EpgPruneUnlinked}
	s.m3uServe = m3uServe

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
