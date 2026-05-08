package webui

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"unicode"

	"github.com/snapetech/iptvtunerr/internal/store"
)

// --- channels collection ---

func (s *Server) v2Channels(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		opts := store.ChannelListOpts{
			Search:    r.URL.Query().Get("search"),
			GroupID:   qInt64(r, "group_id"),
			ProfileID: qInt64(r, "profile_id"),
			OnlyEmpty: qBool(r, "only_empty"),
			Page:      qInt(r, "page", 1),
			PerPage:   qInt(r, "per_page", 0),
		}
		channels, total, err := s.store.ListChannels(opts)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"total":    total,
			"channels": channels,
		})
	case http.MethodPost:
		var ch store.Channel
		if err := json.NewDecoder(r.Body).Decode(&ch); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.store.CreateChannel(&ch); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, ch)
	default:
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// --- channels/:id ---

func (s *Server) v2ChannelItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}

	// sub-routes: /api/v2/channels/bulk and /api/v2/channels/reorder are registered
	// with higher specificity, so this handler only sees numeric IDs.
	id, ok := pathID(r, "/api/v2/channels/")
	if !ok {
		http.NotFound(w, r)
		return
	}

	sub := pathSuffix(r, "/api/v2/channels/")

	switch {
	case sub == "streams" && r.Method == http.MethodPost:
		// POST /api/v2/channels/:id/streams — add a stream to this channel
		var st store.Stream
		if err := json.NewDecoder(r.Body).Decode(&st); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		st.ChannelID = &id
		if err := s.store.CreateStream(&st); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, st)
	default:
		s.v2ChannelCRUD(w, r, id)
	}
}

func (s *Server) v2ChannelCRUD(w http.ResponseWriter, r *http.Request, id int64) {
	switch r.Method {
	case http.MethodGet:
		ch, err := s.store.GetChannel(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if ch == nil {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, ch)
	case http.MethodPatch:
		existing, err := s.store.GetChannel(id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if existing == nil {
			http.NotFound(w, r)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(existing); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		existing.ID = id
		if err := s.store.UpdateChannel(existing); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, existing)
	case http.MethodDelete:
		if err := s.store.DeleteChannel(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "GET, PATCH, DELETE")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// --- channels/bulk ---

type bulkRequest struct {
	IDs    []int64                 `json:"ids"`
	Update store.ChannelBulkUpdate `json:"update"`
}

func (s *Server) v2ChannelsBulk(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req bulkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.BulkUpdateChannels(req.IDs, req.Update); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- channels/automatch ---

type automatchRequest struct {
	ChannelIDs     []int64  `json:"channel_ids"`
	IgnorePrefixes []string `json:"ignore_prefixes"`
	IgnoreSuffixes []string `json:"ignore_suffixes"`
	IgnoreStrings  []string `json:"ignore_strings"`
}

type automatchResult struct {
	Matched int `json:"matched"`
	Skipped int `json:"skipped"`
	Total   int `json:"total"`
}

func normalizeName(s string, prefixes, suffixes, strs []string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, p := range prefixes {
		lp := strings.ToLower(p)
		if strings.HasPrefix(s, lp) {
			s = strings.TrimSpace(s[len(lp):])
		}
	}
	for _, sf := range suffixes {
		lsf := strings.ToLower(sf)
		if strings.HasSuffix(s, lsf) {
			s = strings.TrimSpace(s[:len(s)-len(lsf)])
		}
	}
	for _, sub := range strs {
		s = strings.ReplaceAll(s, strings.ToLower(sub), "")
	}
	// collapse whitespace and strip non-alphanumeric
	var b strings.Builder
	prev := ' '
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prev = r
		} else if prev != ' ' {
			b.WriteRune(' ')
			prev = ' '
		}
	}
	return strings.TrimSpace(b.String())
}

func (s *Server) v2ChannelsAutoMatch(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req automatchRequest
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	// fetch XMLTV channels from tuner
	if s.tunerBase == "" {
		writeError(w, http.StatusServiceUnavailable, "tuner not configured")
		return
	}
	resp, err := http.Get(strings.TrimRight(s.tunerBase, "/") + "/api/guide.xml")
	if err != nil || resp.StatusCode != http.StatusOK {
		writeError(w, http.StatusBadGateway, "guide.xml unavailable")
		return
	}
	defer resp.Body.Close()
	xmlBody, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		writeError(w, http.StatusBadGateway, "guide.xml read error")
		return
	}

	// re-use guideXMLTV struct from apiv2_guide.go (same package)
	var xmltv guideXMLTV
	if err := unmarshalXMLTV(xmlBody, &xmltv); err != nil {
		writeError(w, http.StatusBadGateway, "guide.xml parse error")
		return
	}

	// build normalized name → xmltv channel ID map
	epgByNorm := make(map[string]string, len(xmltv.Channels))
	for _, ch := range xmltv.Channels {
		norm := normalizeName(ch.DisplayName, req.IgnorePrefixes, req.IgnoreSuffixes, req.IgnoreStrings)
		if norm != "" && epgByNorm[norm] == "" {
			epgByNorm[norm] = ch.ID
		}
	}

	// load channels to match
	channels, _, err := s.store.ListChannels(store.ChannelListOpts{PerPage: 0})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	idSet := map[int64]bool{}
	for _, id := range req.ChannelIDs {
		idSet[id] = true
	}

	matched, skipped, total := 0, 0, 0
	for i := range channels {
		ch := &channels[i]
		if len(idSet) > 0 && !idSet[ch.ID] {
			continue
		}
		total++
		norm := normalizeName(ch.Name, req.IgnorePrefixes, req.IgnoreSuffixes, req.IgnoreStrings)
		tvgID, ok := epgByNorm[norm]
		if !ok || tvgID == "" {
			skipped++
			continue
		}
		ch.TVGID = tvgID
		if err := s.store.UpdateChannel(ch); err == nil {
			matched++
		} else {
			skipped++
		}
	}

	writeJSON(w, http.StatusOK, automatchResult{Matched: matched, Skipped: skipped, Total: total})
}

// --- channels/reorder ---

func (s *Server) v2ChannelsReorder(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var ids []int64
	if err := json.NewDecoder(r.Body).Decode(&ids); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.ReorderChannels(ids); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- streams collection ---

func (s *Server) v2Streams(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		opts := store.StreamListOpts{
			Search:         r.URL.Query().Get("search"),
			M3UAccountID:   qInt64(r, "account_id"),
			OnlyUnassigned: qBool(r, "unassigned"),
			HideStale:      qBool(r, "hide_stale"),
			Page:           qInt(r, "page", 1),
			PerPage:        qInt(r, "per_page", 0),
		}
		streams, total, err := s.store.ListStreams(opts)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"total":   total,
			"streams": streams,
		})
	case http.MethodPost:
		var st store.Stream
		if err := json.NewDecoder(r.Body).Decode(&st); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.store.CreateStream(&st); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, st)
	default:
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// --- streams/:id ---

func (s *Server) v2StreamItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id, ok := pathID(r, "/api/v2/streams/")
	if !ok {
		http.NotFound(w, r)
		return
	}

	sub := strings.TrimPrefix(r.URL.Path, "/api/v2/streams/")
	sub = strings.TrimPrefix(sub, strings.Split(sub, "/")[0])
	sub = strings.TrimPrefix(sub, "/")

	switch {
	case sub == "assign" && r.Method == http.MethodPost:
		var req struct {
			ChannelID int64 `json:"channel_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.store.AssignStreamToChannel(id, req.ChannelID); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodDelete && sub == "":
		if err := s.store.DeleteStream(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "DELETE")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// --- channel-profiles ---

func (s *Server) v2ChannelProfiles(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		profiles, err := s.store.ListChannelProfiles()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, profiles)
	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		p, err := s.store.CreateChannelProfile(req.Name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, p)
	default:
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) v2ChannelProfileItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id, ok := pathID(r, "/api/v2/channel-profiles/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	sub := pathSuffix(r, "/api/v2/channel-profiles/")
	switch {
	case sub == "duplicate" && r.Method == http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		p, err := s.store.DuplicateChannelProfile(id, req.Name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, p)
	case r.Method == http.MethodPatch && sub == "":
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.store.RenameChannelProfile(id, req.Name); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodDelete && sub == "":
		if err := s.store.DeleteChannelProfile(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "PATCH, DELETE")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// --- channel-groups ---

func (s *Server) v2ChannelGroups(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		groups, err := s.store.ListChannelGroups()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, groups)
	case http.MethodPost:
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		g, err := s.store.CreateChannelGroup(req.Name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, g)
	default:
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (s *Server) v2ChannelGroupItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireStore(w) {
		return
	}
	id, ok := pathID(r, "/api/v2/channel-groups/")
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodPatch:
		var req struct {
			Name string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := s.store.UpdateChannelGroup(id, req.Name); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodDelete:
		if err := s.store.DeleteChannelGroup(id); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.Header().Set("Allow", "PATCH, DELETE")
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
