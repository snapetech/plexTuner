package tuner

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/virtualchannels"
)

func (s *Server) serveVirtualChannelRules() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			if !operatorUIAllowed(w, r) {
				return
			}
			set := s.reloadVirtualChannels()
			body, err := json.MarshalIndent(map[string]interface{}{
				"generated_at":     time.Now().UTC().Format(time.RFC3339),
				"rules_file":       strings.TrimSpace(s.VirtualChannelsFile),
				"rules_writable":   strings.TrimSpace(s.VirtualChannelsFile) != "",
				"rules":            set,
				"enabled_channels": len(set.Channels),
			}, "", "  ")
			if err != nil {
				writeServerJSONError(w, http.StatusInternalServerError, "encode virtual channel rules")
				return
			}
			_, _ = w.Write(body)
		case http.MethodPost:
			if !operatorUIAllowed(w, r) {
				return
			}
			if strings.TrimSpace(s.VirtualChannelsFile) == "" {
				writeServerJSONError(w, http.StatusServiceUnavailable, "virtual channels file not configured")
				return
			}
			limited := http.MaxBytesReader(w, r.Body, 1<<20)
			defer limited.Close()
			var set virtualchannels.Ruleset
			if err := json.NewDecoder(limited).Decode(&set); err != nil {
				writeServerJSONError(w, http.StatusBadRequest, "invalid virtual channels json")
				return
			}
			saved, err := s.saveVirtualChannels(set)
			if err != nil {
				writeServerJSONError(w, http.StatusBadGateway, "save virtual channels failed")
				return
			}
			body, err := json.MarshalIndent(map[string]interface{}{
				"ok":         true,
				"rules_file": strings.TrimSpace(s.VirtualChannelsFile),
				"rules":      saved,
			}, "", "  ")
			if err != nil {
				writeServerJSONError(w, http.StatusInternalServerError, "encode virtual channel rules")
				return
			}
			_, _ = w.Write(body)
		default:
			writeMethodNotAllowedJSON(w, http.MethodGet, http.MethodPost)
		}
	})
}

func (s *Server) serveVirtualChannelPreview() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		perChannel := 4
		if raw := strings.TrimSpace(r.URL.Query().Get("per_channel")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 24 {
				perChannel = n
			}
		}
		report := virtualchannels.BuildPreview(s.reloadVirtualChannels(), s.Movies, s.Series, time.Now(), perChannel)
		body, err := json.MarshalIndent(map[string]interface{}{
			"generated_at": time.Now().UTC().Format(time.RFC3339),
			"rules_file":   strings.TrimSpace(s.VirtualChannelsFile),
			"report":       report,
		}, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode virtual channel preview")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveVirtualChannelSchedule() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			if !operatorUIAllowed(w, r) {
				return
			}
			horizon := 6 * time.Hour
			if raw := strings.TrimSpace(r.URL.Query().Get("horizon")); raw != "" {
				if d, err := time.ParseDuration(raw); err == nil && d > 0 {
					horizon = d
				}
			}
			set := s.reloadVirtualChannels()
			report := virtualchannels.BuildSchedule(set, s.Movies, s.Series, timeNow(), horizon)
			body, err := json.MarshalIndent(map[string]interface{}{
				"generated_at": time.Now().UTC().Format(time.RFC3339),
				"rules_file":   strings.TrimSpace(s.VirtualChannelsFile),
				"report":       report,
			}, "", "  ")
			if err != nil {
				writeServerJSONError(w, http.StatusInternalServerError, "encode virtual channel schedule")
				return
			}
			_, _ = w.Write(body)
		case http.MethodPost:
			if !operatorUIAllowed(w, r) {
				return
			}
			if strings.TrimSpace(s.VirtualChannelsFile) == "" {
				writeServerJSONError(w, http.StatusServiceUnavailable, "virtual channels file not configured")
				return
			}
			limited := http.MaxBytesReader(w, r.Body, 1<<20)
			defer limited.Close()
			var req virtualChannelScheduleMutationRequest
			if err := json.NewDecoder(limited).Decode(&req); err != nil {
				writeServerJSONError(w, http.StatusBadRequest, "invalid virtual channel schedule json")
				return
			}
			saved, channel, err := s.applyVirtualChannelScheduleMutation(req)
			if err != nil {
				writeServerJSONError(w, http.StatusBadRequest, err.Error())
				return
			}
			report := virtualchannels.BuildSchedule(saved, s.Movies, s.Series, timeNow(), 6*time.Hour)
			body, err := json.MarshalIndent(map[string]interface{}{
				"ok":           true,
				"generated_at": time.Now().UTC().Format(time.RFC3339),
				"rules_file":   strings.TrimSpace(s.VirtualChannelsFile),
				"channel":      channel,
				"report":       report,
			}, "", "  ")
			if err != nil {
				writeServerJSONError(w, http.StatusInternalServerError, "encode virtual channel schedule")
				return
			}
			_, _ = w.Write(body)
		default:
			writeMethodNotAllowedJSON(w, http.MethodGet, http.MethodPost)
		}
	})
}

func (s *Server) serveVirtualChannelDetail() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			if !operatorUIAllowed(w, r) {
				return
			}
			s.serveVirtualChannelDetailRead(w, r, s.reloadVirtualChannels())
		case http.MethodPost:
			if !operatorUIAllowed(w, r) {
				return
			}
			if strings.TrimSpace(s.VirtualChannelsFile) == "" {
				writeServerJSONError(w, http.StatusServiceUnavailable, "virtual channels file not configured")
				return
			}
			limited := http.MaxBytesReader(w, r.Body, 1<<20)
			defer limited.Close()
			var req virtualChannelChannelMutationRequest
			if err := json.NewDecoder(limited).Decode(&req); err != nil {
				writeServerJSONError(w, http.StatusBadRequest, "invalid virtual channel detail json")
				return
			}
			saved, channelID, err := s.applyVirtualChannelChannelMutation(req)
			if err != nil {
				writeServerJSONError(w, http.StatusBadRequest, err.Error())
				return
			}
			r2 := r.Clone(r.Context())
			q := r2.URL.Query()
			q.Set("channel_id", channelID)
			r2.URL.RawQuery = q.Encode()
			s.serveVirtualChannelDetailRead(w, r2, saved)
		default:
			writeMethodNotAllowedJSON(w, http.MethodGet, http.MethodPost)
		}
	})
}

func (s *Server) serveVirtualChannelRecoveryReport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		limit := 20
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}
		channelID := strings.TrimSpace(r.URL.Query().Get("channel_id"))
		body, err := json.MarshalIndent(virtualChannelRecoveryReport{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			ChannelID:   channelID,
			Events:      s.virtualRecoveryHistory(channelID, limit),
		}, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode virtual channel recovery report")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveVirtualChannelReport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		set := s.reloadVirtualChannels()
		rows := make([]virtualChannelStationReportRow, 0, len(set.Channels))
		for _, ch := range set.Channels {
			id := strings.TrimSpace(ch.ID)
			recentRecovery := s.virtualRecoveryHistory(id, 3)
			recoverySummary := summarizeVirtualRecoveryEvents(s.virtualRecoveryHistory(id, 50))
			row := virtualChannelStationReportRow{
				ChannelID:          id,
				Name:               strings.TrimSpace(ch.Name),
				GuideNumber:        strings.TrimSpace(ch.GuideNumber),
				Enabled:            ch.Enabled,
				StreamMode:         strings.TrimSpace(ch.Branding.StreamMode),
				LogoURL:            strings.TrimSpace(ch.Branding.LogoURL),
				BugText:            strings.TrimSpace(ch.Branding.BugText),
				BugImageURL:        strings.TrimSpace(ch.Branding.BugImageURL),
				BugPosition:        strings.TrimSpace(ch.Branding.BugPosition),
				BannerText:         strings.TrimSpace(ch.Branding.BannerText),
				ThemeColor:         strings.TrimSpace(ch.Branding.ThemeColor),
				RecoveryMode:       strings.TrimSpace(ch.Recovery.Mode),
				BlackScreenSeconds: ch.Recovery.BlackScreenSeconds,
				FallbackEntries:    len(ch.Recovery.FallbackEntries),
				RecoveryEvents:     recoverySummary.EventCount,
				RecoveryExhausted:  recoverySummary.RecoveryExhausted,
				LastRecoveryReason: recoverySummary.LastReason,
				PublishedStreamURL: strings.TrimSpace(s.virtualChannelPublishedStreamURL(id, ch)),
				SlateURL:           strings.TrimRight(strings.TrimSpace(s.BaseURL), "/") + "/virtual-channels/slate/" + id + ".svg",
				BrandedStreamURL:   strings.TrimRight(strings.TrimSpace(s.BaseURL), "/") + "/virtual-channels/branded-stream/" + id + ".ts",
				RecentRecovery:     recentRecovery,
			}
			if slot, ok := virtualchannels.ResolveCurrentSlot(set, id, s.Movies, s.Series, timeNow()); ok {
				row.ResolvedNow = &slot
			}
			rows = append(rows, row)
		}
		body, err := json.MarshalIndent(virtualChannelStationReport{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Count:       len(rows),
			Channels:    rows,
		}, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode virtual channel report")
			return
		}
		_, _ = w.Write(body)
	})
}

func summarizeVirtualRecoveryEvents(events []virtualChannelRecoveryEvent) virtualChannelRecoverySummary {
	out := virtualChannelRecoverySummary{EventCount: len(events)}
	if len(events) == 0 {
		return out
	}
	out.LastReason = strings.TrimSpace(events[0].Reason)
	for _, event := range events {
		if strings.HasSuffix(strings.ToLower(strings.TrimSpace(event.Reason)), "-exhausted") {
			out.RecoveryExhausted = true
			break
		}
	}
	return out
}

func (s *Server) serveVirtualChannelGuide() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeMethodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		horizon := 6 * time.Hour
		if raw := strings.TrimSpace(r.URL.Query().Get("horizon")); raw != "" {
			if d, err := time.ParseDuration(raw); err == nil && d > 0 {
				horizon = d
			}
		}
		set := s.reloadVirtualChannels()
		report := virtualchannels.BuildSchedule(set, s.Movies, s.Series, timeNow(), horizon)
		tv := &xmlTVRoot{
			XMLName: xml.Name{Local: "tv"},
			Source:  "IPTV Tunerr (virtual channels)",
		}
		seen := map[string]struct{}{}
		for _, slot := range report.Slots {
			channelID := strings.TrimSpace(slot.ChannelID)
			if _, ok := seen[channelID]; !ok {
				seen[channelID] = struct{}{}
				icons := []xmlIcon(nil)
				if channel, ok := virtualChannelByID(set, channelID); ok {
					if logoURL := strings.TrimSpace(channel.Branding.LogoURL); logoURL != "" {
						icons = append(icons, xmlIcon{Src: logoURL})
					}
				}
				tv.Channels = append(tv.Channels, buildXMLChannel("virtual."+channelID, slot.ChannelName, "", icons))
			}
			tv.Programmes = append(tv.Programmes, xmlProgramme{
				Start:      timeMustParseRFC3339(slot.StartsAtUTC).Format("20060102150405 -0700"),
				Stop:       timeMustParseRFC3339(slot.EndsAtUTC).Format("20060102150405 -0700"),
				Channel:    "virtual." + channelID,
				Title:      xmlValue{Value: slot.ResolvedName},
				SubTitle:   xmlValue{Value: slot.EntryType},
				Desc:       xmlValue{Value: fmt.Sprintf("Synthetic virtual channel slot sourced from %s.", firstNonEmptyString(slot.EntryID, slot.EntryType))},
				Categories: []xmlValue{{Value: "Virtual Channels"}},
			})
		}
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		_, _ = w.Write([]byte(xml.Header))
		enc := xml.NewEncoder(w)
		enc.Indent("", "  ")
		_ = enc.Encode(tv)
	})
}

func (s *Server) serveVirtualChannelDetailRead(w http.ResponseWriter, r *http.Request, set virtualchannels.Ruleset) {
	channelID := strings.TrimSpace(r.URL.Query().Get("channel_id"))
	if channelID == "" {
		writeServerJSONError(w, http.StatusBadRequest, "channel_id required")
		return
	}
	var target *virtualchannels.Channel
	for i := range set.Channels {
		if strings.TrimSpace(set.Channels[i].ID) == channelID {
			ch := set.Channels[i]
			target = &ch
			break
		}
	}
	if target == nil {
		http.NotFound(w, r)
		return
	}
	report := virtualChannelDetailReport{
		GeneratedAt:        time.Now().UTC().Format(time.RFC3339),
		Channel:            *target,
		PublishedStreamURL: strings.TrimSpace(s.virtualChannelPublishedStreamURL(channelID, *target)),
		SlateURL:           strings.TrimRight(strings.TrimSpace(s.BaseURL), "/") + "/virtual-channels/slate/" + channelID + ".svg",
		BrandedStreamURL:   strings.TrimRight(strings.TrimSpace(s.BaseURL), "/") + "/virtual-channels/branded-stream/" + channelID + ".ts",
		RecentRecovery:     s.virtualRecoveryHistory(channelID, 8),
	}
	if slot, ok := virtualchannels.ResolveCurrentSlot(set, channelID, s.Movies, s.Series, timeNow()); ok {
		report.ResolvedNow = &slot
	}
	perChannel := 4
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 24 {
			perChannel = n
		}
	}
	for _, slot := range virtualchannels.BuildPreview(set, s.Movies, s.Series, timeNow(), perChannel).Slots {
		if strings.TrimSpace(slot.ChannelID) == channelID {
			report.Upcoming = append(report.Upcoming, slot)
		}
	}
	horizon := 6 * time.Hour
	if raw := strings.TrimSpace(r.URL.Query().Get("horizon")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			horizon = d
		}
	}
	for _, slot := range virtualchannels.BuildSchedule(set, s.Movies, s.Series, timeNow(), horizon).Slots {
		if strings.TrimSpace(slot.ChannelID) == channelID {
			report.Schedule = append(report.Schedule, slot)
		}
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		writeServerJSONError(w, http.StatusInternalServerError, "encode virtual channel detail")
		return
	}
	_, _ = w.Write(body)
}

func (s *Server) applyVirtualChannelChannelMutation(req virtualChannelChannelMutationRequest) (virtualchannels.Ruleset, string, error) {
	channelID := strings.TrimSpace(req.ChannelID)
	if channelID == "" {
		return virtualchannels.Ruleset{}, "", fmt.Errorf("channel_id required")
	}
	if strings.TrimSpace(req.Action) != "" && strings.TrimSpace(req.Action) != "update_metadata" {
		return virtualchannels.Ruleset{}, "", fmt.Errorf("unsupported action")
	}
	set := s.reloadVirtualChannels()
	idx := virtualChannelIndex(set, channelID)
	if idx < 0 {
		return virtualchannels.Ruleset{}, "", fmt.Errorf("virtual channel not found")
	}
	ch := set.Channels[idx]
	if raw := strings.TrimSpace(req.Name); raw != "" {
		ch.Name = raw
	}
	if raw := strings.TrimSpace(req.GuideNumber); raw != "" {
		ch.GuideNumber = raw
	}
	if raw := strings.TrimSpace(req.GroupTitle); raw != "" {
		ch.GroupTitle = raw
	}
	if raw := strings.TrimSpace(req.Description); raw != "" {
		ch.Description = strings.TrimSpace(req.Description)
	}
	if req.Enabled != nil {
		ch.Enabled = *req.Enabled
	}
	if hasVirtualChannelBrandingFields(req.Branding) || len(req.BrandingClear) > 0 {
		ch.Branding = mergeVirtualChannelBranding(ch.Branding, req.Branding, req.BrandingClear)
	}
	if hasVirtualChannelRecoveryFields(req.Recovery) || len(req.RecoveryClear) > 0 {
		ch.Recovery = mergeVirtualChannelRecovery(ch.Recovery, req.Recovery, req.RecoveryClear)
	}
	set.Channels[idx] = ch
	saved, err := s.saveVirtualChannels(set)
	if err != nil {
		return virtualchannels.Ruleset{}, "", err
	}
	return saved, channelID, nil
}

func hasVirtualChannelBrandingFields(in virtualchannels.Branding) bool {
	return strings.TrimSpace(in.LogoURL) != "" ||
		strings.TrimSpace(in.BugText) != "" ||
		strings.TrimSpace(in.BugImageURL) != "" ||
		strings.TrimSpace(in.BugPosition) != "" ||
		strings.TrimSpace(in.BannerText) != "" ||
		strings.TrimSpace(in.ThemeColor) != "" ||
		strings.TrimSpace(in.StreamMode) != ""
}

func mergeVirtualChannelBranding(base, patch virtualchannels.Branding, clearFields []string) virtualchannels.Branding {
	for _, field := range clearFields {
		switch strings.ToLower(strings.TrimSpace(field)) {
		case "logo_url":
			base.LogoURL = ""
		case "bug_text":
			base.BugText = ""
		case "bug_image_url":
			base.BugImageURL = ""
		case "bug_position":
			base.BugPosition = ""
		case "banner_text":
			base.BannerText = ""
		case "theme_color":
			base.ThemeColor = ""
		case "stream_mode":
			base.StreamMode = ""
		}
	}
	if raw := strings.TrimSpace(patch.LogoURL); raw != "" {
		base.LogoURL = raw
	}
	if raw := strings.TrimSpace(patch.BugText); raw != "" {
		base.BugText = raw
	}
	if raw := strings.TrimSpace(patch.BugImageURL); raw != "" {
		base.BugImageURL = raw
	}
	if raw := strings.TrimSpace(patch.BugPosition); raw != "" {
		base.BugPosition = raw
	}
	if raw := strings.TrimSpace(patch.BannerText); raw != "" {
		base.BannerText = raw
	}
	if raw := strings.TrimSpace(patch.ThemeColor); raw != "" {
		base.ThemeColor = raw
	}
	if raw := strings.TrimSpace(patch.StreamMode); raw != "" {
		base.StreamMode = raw
	}
	return virtualchannels.NormalizeRuleset(virtualchannels.Ruleset{
		Channels: []virtualchannels.Channel{{ID: "tmp", Name: "tmp", GuideNumber: "tmp", Branding: base}},
	}).Channels[0].Branding
}

func mergeVirtualChannelRecovery(base, patch virtualchannels.RecoveryPolicy, clearFields []string) virtualchannels.RecoveryPolicy {
	for _, field := range clearFields {
		switch strings.ToLower(strings.TrimSpace(field)) {
		case "mode":
			base.Mode = ""
		case "black_screen_seconds":
			base.BlackScreenSeconds = 0
		case "fallback_entries":
			base.FallbackEntries = nil
		}
	}
	if raw := strings.TrimSpace(patch.Mode); raw != "" {
		base.Mode = raw
	}
	if patch.BlackScreenSeconds != 0 {
		base.BlackScreenSeconds = patch.BlackScreenSeconds
	}
	if len(patch.FallbackEntries) > 0 {
		base.FallbackEntries = patch.FallbackEntries
	}
	return virtualchannels.NormalizeRuleset(virtualchannels.Ruleset{
		Channels: []virtualchannels.Channel{{ID: "tmp", Name: "tmp", GuideNumber: "tmp", Recovery: base}},
	}).Channels[0].Recovery
}

func (s *Server) applyVirtualChannelScheduleMutation(req virtualChannelScheduleMutationRequest) (virtualchannels.Ruleset, virtualchannels.Channel, error) {
	channelID := strings.TrimSpace(req.ChannelID)
	if channelID == "" {
		return virtualchannels.Ruleset{}, virtualchannels.Channel{}, fmt.Errorf("channel_id required")
	}
	set := s.reloadVirtualChannels()
	idx := virtualChannelIndex(set, channelID)
	if idx < 0 {
		return virtualchannels.Ruleset{}, virtualchannels.Channel{}, fmt.Errorf("virtual channel not found")
	}
	ch := set.Channels[idx]
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "append"
	}
	switch strings.ToLower(strings.TrimSpace(req.Action)) {
	case "append_entry":
		if req.Entry == nil {
			return virtualchannels.Ruleset{}, virtualchannels.Channel{}, fmt.Errorf("entry required")
		}
		ch.Entries = appendEntriesByMode(ch.Entries, []virtualchannels.Entry{*req.Entry}, mode)
	case "replace_entries":
		ch.Entries = appendEntriesByMode(ch.Entries, req.Entries, "replace")
	case "append_slot":
		if req.Slot == nil {
			return virtualchannels.Ruleset{}, virtualchannels.Channel{}, fmt.Errorf("slot required")
		}
		ch.Slots = appendSlotsByMode(ch.Slots, []virtualchannels.Slot{*req.Slot}, mode)
	case "replace_slots":
		ch.Slots = appendSlotsByMode(ch.Slots, req.Slots, "replace")
	case "fill_daypart":
		entries, err := s.buildEntriesForScheduleMutation(req)
		if err != nil {
			return virtualchannels.Ruleset{}, virtualchannels.Channel{}, err
		}
		slots, err := buildDaypartSlots(req.DaypartStart, req.DaypartEnd, req.LabelPrefix, entries)
		if err != nil {
			return virtualchannels.Ruleset{}, virtualchannels.Channel{}, err
		}
		ch.Slots = mergeDaypartSlots(ch.Slots, slots, req.DaypartStart, req.DaypartEnd)
	case "fill_movie_category":
		entries, err := s.buildEntriesForScheduleMutation(virtualChannelScheduleMutationRequest{
			Action:       "fill_movie_category",
			Category:     req.Category,
			DurationMins: req.DurationMins,
		})
		if err != nil {
			return virtualchannels.Ruleset{}, virtualchannels.Channel{}, err
		}
		slots, err := buildDaypartSlots(req.DaypartStart, req.DaypartEnd, req.LabelPrefix, entries)
		if err != nil {
			return virtualchannels.Ruleset{}, virtualchannels.Channel{}, err
		}
		ch.Slots = mergeDaypartSlots(ch.Slots, slots, req.DaypartStart, req.DaypartEnd)
	case "fill_series":
		entries, err := s.buildEntriesForScheduleMutation(virtualChannelScheduleMutationRequest{
			Action:       "fill_series",
			SeriesID:     req.SeriesID,
			EpisodeIDs:   req.EpisodeIDs,
			DurationMins: req.DurationMins,
		})
		if err != nil {
			return virtualchannels.Ruleset{}, virtualchannels.Channel{}, err
		}
		slots, err := buildDaypartSlots(req.DaypartStart, req.DaypartEnd, req.LabelPrefix, entries)
		if err != nil {
			return virtualchannels.Ruleset{}, virtualchannels.Channel{}, err
		}
		ch.Slots = mergeDaypartSlots(ch.Slots, slots, req.DaypartStart, req.DaypartEnd)
	case "append_movies":
		entries, err := s.buildEntriesForScheduleMutation(req)
		if err != nil {
			return virtualchannels.Ruleset{}, virtualchannels.Channel{}, err
		}
		ch.Entries = appendEntriesByMode(ch.Entries, entries, mode)
	case "append_episodes":
		entries, err := s.buildEntriesForScheduleMutation(req)
		if err != nil {
			return virtualchannels.Ruleset{}, virtualchannels.Channel{}, err
		}
		ch.Entries = appendEntriesByMode(ch.Entries, entries, mode)
	case "remove_entries":
		if len(req.RemoveEntryIDs) == 0 {
			return virtualchannels.Ruleset{}, virtualchannels.Channel{}, fmt.Errorf("remove_entry_ids required")
		}
		ch.Entries = removeVirtualChannelEntries(ch.Entries, req.RemoveEntryIDs)
	case "remove_slots":
		if len(req.RemoveSlots) == 0 {
			return virtualchannels.Ruleset{}, virtualchannels.Channel{}, fmt.Errorf("remove_slots required")
		}
		ch.Slots = removeVirtualChannelSlots(ch.Slots, req.RemoveSlots)
	default:
		return virtualchannels.Ruleset{}, virtualchannels.Channel{}, fmt.Errorf("unsupported action")
	}
	set.Channels[idx] = ch
	saved, err := s.saveVirtualChannels(set)
	if err != nil {
		return virtualchannels.Ruleset{}, virtualchannels.Channel{}, err
	}
	channel, ok := virtualChannelByID(saved, channelID)
	if !ok {
		return virtualchannels.Ruleset{}, virtualchannels.Channel{}, fmt.Errorf("virtual channel not found after save")
	}
	return saved, channel, nil
}

func (s *Server) virtualChannelLiveRows() []catalog.LiveChannel {
	rules := s.reloadVirtualChannels()
	if len(rules.Channels) == 0 {
		return nil
	}
	rows := make([]catalog.LiveChannel, 0, len(rules.Channels))
	for _, ch := range rules.Channels {
		if !ch.Enabled {
			continue
		}
		channelID := "virtual-" + strings.TrimSpace(ch.ID)
		streamURL := s.virtualChannelPublishedStreamURL(strings.TrimSpace(ch.ID), ch)
		rows = append(rows, catalog.LiveChannel{
			ChannelID:   channelID,
			DNAID:       channelID,
			GuideNumber: strings.TrimSpace(ch.GuideNumber),
			GuideName:   strings.TrimSpace(ch.Name),
			StreamURL:   streamURL,
			StreamURLs:  []string{streamURL},
			EPGLinked:   false,
			TVGID:       "virtual." + strings.TrimSpace(ch.ID),
			GroupTitle:  firstNonEmptyString(strings.TrimSpace(ch.GroupTitle), "Virtual Channels"),
			SourceTag:   "virtual",
		})
	}
	return rows
}

func (s *Server) serveVirtualChannelM3U() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			writeMethodNotAllowed(w, http.MethodGet, http.MethodHead)
			return
		}
		set := s.reloadVirtualChannels()
		w.Header().Set("Content-Type", "application/x-mpegURL")
		_, _ = io.WriteString(w, "#EXTM3U\n")
		for _, ch := range s.virtualChannelLiveRows() {
			attrParts := []string{
				fmt.Sprintf("tvg-id=\"%s\"", strings.TrimSpace(ch.TVGID)),
				fmt.Sprintf("tvg-name=\"%s\"", strings.TrimSpace(ch.GuideName)),
				fmt.Sprintf("group-title=\"%s\"", strings.TrimSpace(ch.GroupTitle)),
			}
			virtualID := strings.TrimPrefix(strings.TrimSpace(ch.ChannelID), "virtual-")
			if station, ok := virtualChannelByID(set, virtualID); ok {
				if artworkURL := strings.TrimSpace(station.Branding.LogoURL); artworkURL != "" {
					attrParts = append(attrParts, fmt.Sprintf("tvg-logo=\"%s\"", artworkURL))
				}
			}
			_, _ = io.WriteString(w, fmt.Sprintf("#EXTINF:-1 %s,%s\n%s\n",
				strings.Join(attrParts, " "),
				strings.TrimSpace(ch.GuideName),
				strings.TrimSpace(ch.StreamURL),
			))
		}
	})
}
