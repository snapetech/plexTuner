// Command plex-tuner indexes IPTV sources (M3U or Xtream player_api), saves a catalog,
// and serves HDHomeRun-style discovery + lineup so Plex can use it as a tuner.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/plextuner/plex-tuner/internal/catalog"
	"github.com/plextuner/plex-tuner/internal/gateway"
	"github.com/plextuner/plex-tuner/internal/httpclient"
	"github.com/plextuner/plex-tuner/internal/indexer"
	"github.com/plextuner/plex-tuner/internal/probe"
	"github.com/plextuner/plex-tuner/internal/vodfs"
)

func main() {
	m3uURL := flag.String("m3u", "", "M3U URL to index (optional)")
	apiBase := flag.String("api", "", "Xtream player_api base URL (e.g. http://host:port)")
	apiUser := flag.String("user", "", "API username")
	apiPass := flag.String("pass", "", "API password")
	liveOnly := flag.Bool("live-only", false, "Only index live channels (player_api)")
	catalogPath := flag.String("catalog", "catalog.json", "Path to catalog file")
	addr := flag.String("addr", ":8080", "HTTP listen address")
	mountDir := flag.String("mount", "", "Optional FUSE mount point for VOD (Movies/TV)")
	flag.Parse()

	cat := catalog.New()
	if *catalogPath != "" {
		if err := cat.Load(*catalogPath); err != nil && !os.IsNotExist(err) {
			log.Printf("load catalog: %v", err)
		}
	}

	if *m3uURL != "" {
		movies, series, live, err := indexer.ParseM3U(*m3uURL, httpclient.Default())
		if err != nil {
			log.Fatalf("index M3U: %v", err)
		}
		cat.Replace(movies, series, live)
		if err := cat.Save(*catalogPath); err != nil {
			log.Printf("save catalog: %v", err)
		}
		log.Printf("indexed M3U: %d movies, %d series, %d live", len(movies), len(series), len(live))
	} else if *apiBase != "" && *apiUser != "" && *apiPass != "" {
		movies, series, live, err := indexer.IndexFromPlayerAPI(*apiBase, *apiUser, *apiPass, "m3u8", *liveOnly, nil, httpclient.Default())
		if err != nil {
			log.Fatalf("index player_api: %v", err)
		}
		cat.Replace(movies, series, live)
		if err := cat.Save(*catalogPath); err != nil {
			log.Printf("save catalog: %v", err)
		}
		log.Printf("indexed API: %d movies, %d series, %d live", len(movies), len(series), len(live))
	}

	movies, series, _ := cat.Copy()
	baseURL := "http://" + *addr
	mux := http.NewServeMux()
	mux.Handle("/lineup.json", probe.LineupHandler(func() []probe.LineupItem {
		_, _, live := cat.Copy()
		return probe.Lineup(live, baseURL)
	}))
	mux.Handle("/device.xml", probe.DiscoveryHandler("plextuner-1", "Plex Tuner"))
	mux.Handle("/stream", gateway.Handler())
	mux.Handle("/stream/", gateway.Handler())

	log.Printf("listening on %s", *addr)
	go func() {
		if err := http.ListenAndServe(*addr, mux); err != nil {
			log.Fatalf("http: %v", err)
		}
	}()

	if *mountDir != "" && (len(movies) > 0 || len(series) > 0) {
		root := vodfs.NewRoot(movies, series, nil)
		server, err := vodfs.Mount(*mountDir, root)
		if err != nil {
			log.Printf("mount VODFS: %v", err)
		} else {
			log.Printf("VOD mounted at %s", *mountDir)
			defer server.Unmount()
		}
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	fmt.Println("shutting down")
}
