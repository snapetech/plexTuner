package main

import (
	"log"
	"os"
	"strings"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/epglink"
	"github.com/snapetech/iptvtunerr/internal/guideinput"
)

func loadLiveReportCatalog(cfg *config.Config, catalogPath string) []catalog.LiveChannel {
	path := strings.TrimSpace(catalogPath)
	if path == "" {
		path = cfg.CatalogPath
	}
	c := catalog.New()
	if err := c.Load(path); err != nil {
		log.Printf("Load catalog %s: %v", path, err)
		os.Exit(1)
	}
	live := c.SnapshotLive()
	if len(live) == 0 {
		log.Printf("Catalog %s contains no live_channels", path)
		os.Exit(1)
	}
	return live
}

func loadOptionalMatchReport(live []catalog.LiveChannel, xmltvRef, aliasesRef string) *epglink.Report {
	rep, err := guideinput.LoadOptionalMatchReport(live, xmltvRef, aliasesRef)
	if err != nil {
		log.Printf("Load XMLTV match inputs failed: %v", err)
		os.Exit(1)
	}
	return rep
}
