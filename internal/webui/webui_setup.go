package webui

import (
	"net/http"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/setupdoctor"
)

func (s *Server) setupDoctor(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if r.Method != http.MethodGet {
		writeMethodNotAllowedJSON(w, http.MethodGet)
		return
	}
	cfg := config.Load()
	mode := setupdoctor.NormalizeMode(r.URL.Query().Get("mode"))
	report := setupdoctor.Build(cfg, mode, strings.TrimSpace(r.URL.Query().Get("base_url")))
	payload := map[string]interface{}{
		"generated_at": time.Now().UTC().Format(time.RFC3339),
		"configured":   report.Ready,
		"report":       report,
	}
	writeJSONPayload(w, payload)
}
