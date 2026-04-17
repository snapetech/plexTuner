package tuner

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
	"github.com/snapetech/iptvtunerr/internal/guidehealth"
	"github.com/snapetech/iptvtunerr/internal/plexharvest"
	"github.com/snapetech/iptvtunerr/internal/programming"
)

type programmingPreviewReport struct {
	GeneratedAt       string                                `json:"generated_at"`
	RecipeFile        string                                `json:"recipe_file,omitempty"`
	RecipeWritable    bool                                  `json:"recipe_writable"`
	HarvestFile       string                                `json:"harvest_file,omitempty"`
	HarvestReady      bool                                  `json:"harvest_ready"`
	HarvestLineups    []plexharvest.SummaryLineup           `json:"harvest_lineups,omitempty"`
	RawChannels       int                                   `json:"raw_channels"`
	CuratedChannels   int                                   `json:"curated_channels"`
	Recipe            programming.Recipe                    `json:"recipe"`
	Inventory         []programming.CategorySummary         `json:"inventory,omitempty"`
	Buckets           map[string]int                        `json:"buckets,omitempty"`
	Lineup            []catalog.LiveChannel                 `json:"lineup,omitempty"`
	LineupDescriptors map[string]programming.FeedDescriptor `json:"lineup_descriptors,omitempty"`
	BackupGroups      []programming.BackupGroup             `json:"backup_groups,omitempty"`
}

type programmingChannelDetailReport struct {
	GeneratedAt        string                          `json:"generated_at"`
	Channel            catalog.LiveChannel             `json:"channel"`
	Descriptor         programming.FeedDescriptor      `json:"descriptor,omitempty"`
	Curated            bool                            `json:"curated"`
	CategoryID         string                          `json:"category_id,omitempty"`
	CategoryLabel      string                          `json:"category_label,omitempty"`
	CategorySource     string                          `json:"category_source,omitempty"`
	Bucket             string                          `json:"bucket,omitempty"`
	SourceReady        bool                            `json:"source_ready"`
	UpcomingProgrammes []CatchupCapsule                `json:"upcoming_programmes,omitempty"`
	ExactBackupGroup   *programming.BackupGroup        `json:"exact_backup_group,omitempty"`
	AlternativeSources []programming.BackupGroupMember `json:"alternative_sources,omitempty"`
}

type programmingBrowseItem struct {
	CategoryID             string                     `json:"category_id,omitempty"`
	GroupTitle             string                     `json:"group_title,omitempty"`
	SourceTag              string                     `json:"source_tag,omitempty"`
	Bucket                 string                     `json:"bucket,omitempty"`
	ChannelID              string                     `json:"channel_id,omitempty"`
	GuideNumber            string                     `json:"guide_number,omitempty"`
	GuideName              string                     `json:"guide_name,omitempty"`
	TVGID                  string                     `json:"tvg_id,omitempty"`
	Descriptor             programming.FeedDescriptor `json:"descriptor,omitempty"`
	Curated                bool                       `json:"curated"`
	Included               bool                       `json:"included,omitempty"`
	Excluded               bool                       `json:"excluded,omitempty"`
	GuideStatus            string                     `json:"guide_status,omitempty"`
	HasGuideProgrammes     bool                       `json:"has_guide_programmes,omitempty"`
	HasRealGuideProgrammes bool                       `json:"has_real_guide_programmes,omitempty"`
	NextHourProgrammeCount int                        `json:"next_hour_programme_count,omitempty"`
	NextHourTitles         []string                   `json:"next_hour_titles,omitempty"`
	ExactBackupCount       int                        `json:"exact_backup_count,omitempty"`
}

type programmingBrowseReport struct {
	GeneratedAt    string                  `json:"generated_at"`
	CategoryID     string                  `json:"category_id,omitempty"`
	CategoryLabel  string                  `json:"category_label,omitempty"`
	CategorySource string                  `json:"category_source,omitempty"`
	SourceReady    bool                    `json:"source_ready"`
	Horizon        string                  `json:"horizon"`
	GuideFilter    string                  `json:"guide_filter,omitempty"`
	CuratedFilter  string                  `json:"curated_filter,omitempty"`
	Recipe         programming.Recipe      `json:"recipe"`
	TotalChannels  int                     `json:"total_channels"`
	FilteredCount  int                     `json:"filtered_count"`
	Items          []programmingBrowseItem `json:"items,omitempty"`
}

type programmingHarvestImportReport struct {
	GeneratedAt          string                `json:"generated_at"`
	HarvestFile          string                `json:"harvest_file,omitempty"`
	LineupTitle          string                `json:"lineup_title,omitempty"`
	FriendlyName         string                `json:"friendly_name,omitempty"`
	Replace              bool                  `json:"replace"`
	CollapseExactBackups bool                  `json:"collapse_exact_backups"`
	HarvestedChannels    int                   `json:"harvested_channels"`
	MatchedChannels      int                   `json:"matched_channels"`
	MatchStrategies      map[string]int        `json:"match_strategies,omitempty"`
	OrderedChannelIDs    []string              `json:"ordered_channel_ids,omitempty"`
	MissingGuideNames    []string              `json:"missing_guide_names,omitempty"`
	Recipe               programming.Recipe    `json:"recipe"`
	MatchedLineup        []catalog.LiveChannel `json:"matched_lineup,omitempty"`
}

type programmingHarvestAssist struct {
	LineupTitle          string         `json:"lineup_title"`
	FriendlyNames        []string       `json:"friendly_names,omitempty"`
	MatchedChannels      int            `json:"matched_channels"`
	OrderedChannelIDs    []string       `json:"ordered_channel_ids,omitempty"`
	MatchStrategies      map[string]int `json:"match_strategies,omitempty"`
	LocalBroadcastHits   int            `json:"local_broadcast_hits"`
	ExactGuideNameHits   int            `json:"exact_guide_name_hits"`
	ExactTVGIDHits       int            `json:"exact_tvg_id_hits"`
	GuideNumberHits      int            `json:"guide_number_hits"`
	Recommended          bool           `json:"recommended"`
	RecommendationReason string         `json:"recommendation_reason,omitempty"`
}

type programmingHarvestAssistReport struct {
	GeneratedAt string                     `json:"generated_at"`
	HarvestFile string                     `json:"harvest_file,omitempty"`
	Assists     []programmingHarvestAssist `json:"assists,omitempty"`
}

func normalizeHarvestGuideName(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.Join(strings.Fields(raw), " ")
	return raw
}

func normalizeHarvestBroadcastStem(raw string) string {
	raw = normalizeHarvestGuideName(raw)
	replacer := strings.NewReplacer(
		" east", "",
		" west", "",
		" hd", "",
		" us", "",
		" usa", "",
		" canada", "",
	)
	raw = replacer.Replace(raw)
	fields := strings.Fields(raw)
	if len(fields) > 1 {
		last := fields[len(fields)-1]
		if len(last) >= 3 && len(last) <= 12 && !strings.ContainsAny(last, "0123456789") {
			fields = fields[:len(fields)-1]
		}
	}
	return strings.Join(fields, " ")
}

func chooseHarvestResult(rep plexharvest.Report, lineupTitle, friendlyName string) (plexharvest.Result, bool) {
	lineupTitle = strings.TrimSpace(lineupTitle)
	friendlyName = strings.TrimSpace(friendlyName)
	best := plexharvest.Result{}
	found := false
	for _, row := range rep.Results {
		if lineupTitle != "" && !strings.EqualFold(strings.TrimSpace(row.LineupTitle), lineupTitle) {
			continue
		}
		if friendlyName != "" && !strings.EqualFold(strings.TrimSpace(row.FriendlyName), friendlyName) {
			continue
		}
		if len(row.Channels) == 0 {
			continue
		}
		if !found || len(row.Channels) > len(best.Channels) || row.ChannelMapRows > best.ChannelMapRows {
			best = row
			found = true
		}
	}
	if found {
		return best, true
	}
	for _, row := range rep.Results {
		if len(row.Channels) == 0 {
			continue
		}
		if !found || len(row.Channels) > len(best.Channels) || row.ChannelMapRows > best.ChannelMapRows {
			best = row
			found = true
		}
	}
	return best, found
}

func harvestCandidateKeys(ch catalog.LiveChannel) []string {
	keys := make([]string, 0, 4)
	if tvg := strings.TrimSpace(ch.TVGID); tvg != "" {
		keys = append(keys, "tvg:"+tvg)
	}
	if name := normalizeHarvestGuideName(ch.GuideName); name != "" {
		keys = append(keys, "name:"+name)
	}
	if num := strings.TrimSpace(ch.GuideNumber); num != "" {
		keys = append(keys, "number:"+num)
	}
	if programming.ClassifyChannel(ch) == programming.BucketLocalBroadcast {
		if stem := normalizeHarvestBroadcastStem(ch.GuideName); stem != "" {
			keys = append(keys, "local_stem:"+stem)
		}
	}
	return keys
}

func harvestLookupKeys(harvested plexharvest.HarvestedChannel) []struct {
	key      string
	strategy string
} {
	keys := make([]struct {
		key      string
		strategy string
	}, 0, 4)
	if tvg := strings.TrimSpace(harvested.TVGID); tvg != "" {
		keys = append(keys, struct {
			key      string
			strategy string
		}{key: "tvg:" + tvg, strategy: "tvg_id_exact"})
	}
	if name := normalizeHarvestGuideName(harvested.GuideName); name != "" {
		keys = append(keys, struct {
			key      string
			strategy string
		}{key: "name:" + name, strategy: "guide_name_exact"})
	}
	if num := strings.TrimSpace(harvested.GuideNumber); num != "" {
		keys = append(keys, struct {
			key      string
			strategy string
		}{key: "number:" + num, strategy: "guide_number_exact"})
	}
	if stem := normalizeHarvestBroadcastStem(harvested.GuideName); stem != "" {
		keys = append(keys, struct {
			key      string
			strategy string
		}{key: "local_stem:" + stem, strategy: "local_broadcast_stem"})
	}
	return keys
}

func buildProgrammingHarvestImport(existing programming.Recipe, raw []catalog.LiveChannel, result plexharvest.Result, replace bool, collapse bool) programmingHarvestImportReport {
	report := programmingHarvestImportReport{
		GeneratedAt:          time.Now().UTC().Format(time.RFC3339),
		LineupTitle:          strings.TrimSpace(result.LineupTitle),
		FriendlyName:         strings.TrimSpace(result.FriendlyName),
		Replace:              replace,
		CollapseExactBackups: collapse,
		HarvestedChannels:    len(result.Channels),
		MatchStrategies:      map[string]int{},
	}
	indexed := map[string][]catalog.LiveChannel{}
	for _, ch := range raw {
		for _, key := range harvestCandidateKeys(ch) {
			indexed[key] = append(indexed[key], ch)
		}
	}
	seen := map[string]struct{}{}
	ordered := make([]string, 0)
	matched := make([]catalog.LiveChannel, 0)
	missing := make([]string, 0)
	for _, harvested := range result.Channels {
		var (
			candidates []catalog.LiveChannel
			matchedVia string
		)
		for _, rule := range harvestLookupKeys(harvested) {
			rows := indexed[rule.key]
			if len(rows) == 0 {
				continue
			}
			candidates = append(candidates, rows...)
			matchedVia = rule.strategy
			break
		}
		if len(candidates) == 0 {
			if name := strings.TrimSpace(harvested.GuideName); name != "" {
				missing = append(missing, name)
			}
			continue
		}
		report.MatchStrategies[matchedVia]++
		sort.SliceStable(candidates, func(i, j int) bool {
			hi := strings.TrimSpace(harvested.GuideNumber)
			ai := strings.TrimSpace(candidates[i].GuideNumber)
			aj := strings.TrimSpace(candidates[j].GuideNumber)
			if ai == hi && aj != hi {
				return true
			}
			if aj == hi && ai != hi {
				return false
			}
			if ai == aj {
				return strings.TrimSpace(candidates[i].GuideName) < strings.TrimSpace(candidates[j].GuideName)
			}
			return ai < aj
		})
		for _, candidate := range candidates {
			channelID := strings.TrimSpace(candidate.ChannelID)
			if _, ok := seen[channelID]; ok {
				continue
			}
			seen[channelID] = struct{}{}
			ordered = append(ordered, channelID)
			matched = append(matched, candidate)
		}
	}
	report.OrderedChannelIDs = append([]string(nil), ordered...)
	report.MatchedChannels = len(ordered)
	report.MatchedLineup = append([]catalog.LiveChannel(nil), matched...)
	report.MissingGuideNames = append([]string(nil), missing...)

	var recipe programming.Recipe
	if replace {
		excluded := make([]string, 0, len(raw))
		for _, ch := range raw {
			channelID := strings.TrimSpace(ch.ChannelID)
			if _, ok := seen[channelID]; ok {
				continue
			}
			excluded = append(excluded, channelID)
		}
		recipe = programming.Recipe{
			IncludedChannelIDs:   append([]string(nil), ordered...),
			ExcludedChannelIDs:   excluded,
			OrderMode:            "custom",
			CustomOrder:          append([]string(nil), ordered...),
			CollapseExactBackups: collapse,
		}
	} else {
		recipe = existing
		recipe.CollapseExactBackups = recipe.CollapseExactBackups || collapse
		recipe.OrderMode = "custom"
		recipe.IncludedChannelIDs = append(append([]string(nil), recipe.IncludedChannelIDs...), ordered...)
		recipe.CustomOrder = append(append([]string(nil), ordered...), recipe.CustomOrder...)
		if len(recipe.ExcludedChannelIDs) > 0 {
			excluded := make([]string, 0, len(recipe.ExcludedChannelIDs))
			for _, id := range recipe.ExcludedChannelIDs {
				if _, ok := seen[strings.TrimSpace(id)]; ok {
					continue
				}
				excluded = append(excluded, id)
			}
			recipe.ExcludedChannelIDs = excluded
		}
	}
	report.Recipe = programming.NormalizeRecipe(recipe)
	return report
}

func buildProgrammingHarvestAssist(raw []catalog.LiveChannel, row plexharvest.SummaryLineup, result plexharvest.Result) programmingHarvestAssist {
	preview := buildProgrammingHarvestImport(programming.Recipe{}, raw, result, true, true)
	assist := programmingHarvestAssist{
		LineupTitle:        strings.TrimSpace(row.LineupTitle),
		FriendlyNames:      append([]string(nil), row.FriendlyNames...),
		MatchedChannels:    preview.MatchedChannels,
		OrderedChannelIDs:  append([]string(nil), preview.OrderedChannelIDs...),
		MatchStrategies:    map[string]int{},
		LocalBroadcastHits: preview.MatchStrategies["local_broadcast_stem"],
		ExactGuideNameHits: preview.MatchStrategies["guide_name_exact"],
		ExactTVGIDHits:     preview.MatchStrategies["tvg_id_exact"],
		GuideNumberHits:    preview.MatchStrategies["guide_number_exact"],
	}
	for key, value := range preview.MatchStrategies {
		assist.MatchStrategies[key] = value
	}
	if assist.LocalBroadcastHits > 0 {
		assist.Recommended = true
		assist.RecommendationReason = fmt.Sprintf("%d local-broadcast lineup row(s) mapped back onto current raw channels.", assist.LocalBroadcastHits)
	} else if assist.ExactTVGIDHits > 0 || assist.ExactGuideNameHits > 0 {
		assist.Recommended = true
		assist.RecommendationReason = "Strong exact guide matches were found for this harvested lineup."
	} else if assist.MatchedChannels > 0 {
		assist.RecommendationReason = "Some rows matched, but this looks weaker as a local-market seed."
	}
	return assist
}

func (s *Server) serveProgrammingCategories() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			if !operatorUIAllowed(w, r) {
				return
			}
		case http.MethodPost:
			if !operatorUIAllowed(w, r) {
				return
			}
			if strings.TrimSpace(s.ProgrammingRecipeFile) == "" {
				writeServerJSONError(w, http.StatusServiceUnavailable, "programming recipe file not configured")
				return
			}
			limited := http.MaxBytesReader(w, r.Body, 65536)
			defer limited.Close()
			var req struct {
				Action      string   `json:"action"`
				CategoryID  string   `json:"category_id"`
				CategoryIDs []string `json:"category_ids"`
			}
			if err := json.NewDecoder(limited).Decode(&req); err != nil {
				writeServerJSONError(w, http.StatusBadRequest, "invalid programming category json")
				return
			}
			ids := append([]string(nil), req.CategoryIDs...)
			if strings.TrimSpace(req.CategoryID) != "" {
				ids = append(ids, strings.TrimSpace(req.CategoryID))
			}
			recipe := programming.UpdateRecipeCategories(s.reloadProgrammingRecipe(), req.Action, ids)
			saved, err := programming.SaveRecipeFile(s.ProgrammingRecipeFile, recipe)
			if err != nil {
				writeServerJSONError(w, http.StatusBadGateway, "save programming recipe failed")
				return
			}
			s.ProgrammingRecipe = saved
			s.rebuildCuratedChannelsFromRaw()
		default:
			writeMethodNotAllowedJSON(w, http.MethodGet, http.MethodPost)
			return
		}
		inventory := programming.BuildCategoryInventory(s.RawChannels)
		resp := map[string]interface{}{
			"generated_at":  time.Now().UTC().Format(time.RFC3339),
			"source_ready":  len(s.RawChannels) > 0,
			"raw_channels":  len(s.RawChannels),
			"categories":    inventory,
			"recipe_file":   strings.TrimSpace(s.ProgrammingRecipeFile),
			"recipe_loaded": s.reloadProgrammingRecipe().Version > 0,
			"recipe":        s.reloadProgrammingRecipe(),
		}
		if categoryID := strings.TrimSpace(r.URL.Query().Get("category")); categoryID != "" {
			resp["members"] = programming.CategoryMembers(s.RawChannels, categoryID)
		}
		body, err := json.MarshalIndent(resp, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode programming categories")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveProgrammingBrowse() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		categoryID := strings.TrimSpace(r.URL.Query().Get("category"))
		if categoryID == "" {
			writeServerJSONError(w, http.StatusBadRequest, "category required")
			return
		}
		horizon := time.Hour
		if raw := strings.TrimSpace(r.URL.Query().Get("horizon")); raw != "" {
			if d, err := time.ParseDuration(raw); err == nil && d > 0 {
				horizon = d
			}
		}
		memberLimit := streamAttemptLimitFromQuery(r.URL.Query().Get("limit"), 400)
		guideFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("guide")))
		curatedFilter := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("curated")))
		members := programming.CategoryMembers(s.RawChannels, categoryID)
		if len(members) == 0 {
			body, err := json.MarshalIndent(programmingBrowseReport{
				GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
				CategoryID:    categoryID,
				Horizon:       horizon.String(),
				GuideFilter:   guideFilter,
				CuratedFilter: curatedFilter,
				Recipe:        s.reloadProgrammingRecipe(),
				TotalChannels: 0,
			}, "", "  ")
			if err != nil {
				writeServerJSONError(w, http.StatusInternalServerError, "encode programming browse")
				return
			}
			_, _ = w.Write(body)
			return
		}
		if memberLimit > len(members) {
			memberLimit = len(members)
		}
		selected := map[string]struct{}{}
		excluded := map[string]struct{}{}
		recipe := s.reloadProgrammingRecipe()
		for _, id := range recipe.IncludedChannelIDs {
			selected[strings.TrimSpace(id)] = struct{}{}
		}
		for _, id := range recipe.ExcludedChannelIDs {
			excluded[strings.TrimSpace(id)] = struct{}{}
		}
		healthByID := map[string]guidehealth.ChannelHealth{}
		sourceReady := false
		if s.xmltv != nil {
			if rep, err := s.xmltv.GuideHealth(time.Now(), strings.TrimSpace(os.Getenv("IPTV_TUNERR_XMLTV_ALIASES"))); err == nil {
				sourceReady = rep.SourceReady
				for _, row := range rep.Channels {
					healthByID[strings.TrimSpace(row.ChannelID)] = row
				}
			}
		}
		titlesByChannelID := map[string][]string{}
		if s.xmltv != nil {
			if preview, err := s.xmltv.CatchupCapsulePreview(time.Now(), horizon, 4096); err == nil {
				sourceReady = sourceReady || preview.SourceReady
				for _, capsule := range preview.Capsules {
					channelID := strings.TrimSpace(capsule.ChannelID)
					if channelID == "" {
						continue
					}
					title := strings.TrimSpace(capsule.Title)
					if title == "" {
						continue
					}
					dup := false
					for _, existing := range titlesByChannelID[channelID] {
						if strings.TrimSpace(existing) == title {
							dup = true
							break
						}
					}
					if !dup {
						titlesByChannelID[channelID] = append(titlesByChannelID[channelID], title)
					}
				}
			}
		}
		backupCounts := map[string]int{}
		for _, group := range programming.BuildBackupGroups(s.RawChannels) {
			count := len(group.Members) - 1
			for _, member := range group.Members {
				backupCounts[strings.TrimSpace(member.ChannelID)] = count
			}
		}
		report := programmingBrowseReport{
			GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
			CategoryID:    categoryID,
			SourceReady:   sourceReady,
			Horizon:       horizon.String(),
			GuideFilter:   guideFilter,
			CuratedFilter: curatedFilter,
			Recipe:        recipe,
			TotalChannels: len(members),
		}
		filtered := make([]programmingBrowseItem, 0, len(members))
		for _, member := range members {
			channelID := strings.TrimSpace(member.ChannelID)
			item := programmingBrowseItem{
				CategoryID:       member.CategoryID,
				Bucket:           member.Bucket,
				ChannelID:        member.ChannelID,
				GuideNumber:      member.GuideNumber,
				GuideName:        member.GuideName,
				TVGID:            member.TVGID,
				SourceTag:        member.SourceTag,
				GroupTitle:       member.GroupTitle,
				Descriptor:       member.Descriptor,
				Curated:          containsLiveChannelID(s.Channels, channelID),
				ExactBackupCount: backupCounts[channelID],
			}
			if _, ok := selected[channelID]; ok {
				item.Included = true
			}
			if _, ok := excluded[channelID]; ok {
				item.Excluded = true
			}
			if health, ok := healthByID[channelID]; ok {
				item.GuideStatus = health.Status
				item.HasGuideProgrammes = health.HasProgrammes
				item.HasRealGuideProgrammes = health.HasRealProgrammes
			}
			item.NextHourTitles = append([]string(nil), titlesByChannelID[channelID]...)
			item.NextHourProgrammeCount = len(item.NextHourTitles)
			if guideFilter == "real" && !item.HasRealGuideProgrammes {
				continue
			}
			if curatedFilter == "missing" && item.Curated {
				continue
			}
			if curatedFilter == "curated" && !item.Curated {
				continue
			}
			filtered = append(filtered, item)
		}
		report.FilteredCount = len(filtered)
		if memberLimit > len(filtered) {
			memberLimit = len(filtered)
		}
		report.Items = append(report.Items, filtered[:memberLimit]...)
		if len(report.Items) > 0 {
			report.CategoryLabel = report.Items[0].GroupTitle
			report.CategorySource = report.Items[0].SourceTag
		} else if len(members) > 0 {
			report.CategoryLabel = strings.TrimSpace(members[0].GroupTitle)
			report.CategorySource = strings.TrimSpace(members[0].SourceTag)
		}
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode programming browse")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveProgrammingChannels() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			if !operatorUIAllowed(w, r) {
				return
			}
			recipe := s.reloadProgrammingRecipe()
			resp := map[string]interface{}{
				"generated_at":      time.Now().UTC().Format(time.RFC3339),
				"recipe_file":       strings.TrimSpace(s.ProgrammingRecipeFile),
				"included_channels": recipe.IncludedChannelIDs,
				"excluded_channels": recipe.ExcludedChannelIDs,
			}
			if categoryID := strings.TrimSpace(r.URL.Query().Get("category")); categoryID != "" {
				resp["members"] = programming.CategoryMembers(s.RawChannels, categoryID)
			}
			body, err := json.MarshalIndent(resp, "", "  ")
			if err != nil {
				writeServerJSONError(w, http.StatusInternalServerError, "encode programming channels")
				return
			}
			_, _ = w.Write(body)
		case http.MethodPost:
			if !operatorUIAllowed(w, r) {
				return
			}
			if strings.TrimSpace(s.ProgrammingRecipeFile) == "" {
				writeServerJSONError(w, http.StatusServiceUnavailable, "programming recipe file not configured")
				return
			}
			limited := http.MaxBytesReader(w, r.Body, 65536)
			defer limited.Close()
			var req struct {
				Action     string   `json:"action"`
				ChannelID  string   `json:"channel_id"`
				ChannelIDs []string `json:"channel_ids"`
			}
			if err := json.NewDecoder(limited).Decode(&req); err != nil {
				writeServerJSONError(w, http.StatusBadRequest, "invalid programming channel json")
				return
			}
			ids := append([]string(nil), req.ChannelIDs...)
			if strings.TrimSpace(req.ChannelID) != "" {
				ids = append(ids, strings.TrimSpace(req.ChannelID))
			}
			recipe := programming.UpdateRecipeChannels(s.reloadProgrammingRecipe(), req.Action, ids)
			saved, err := programming.SaveRecipeFile(s.ProgrammingRecipeFile, recipe)
			if err != nil {
				writeServerJSONError(w, http.StatusBadGateway, "save programming recipe failed")
				return
			}
			s.ProgrammingRecipe = saved
			s.rebuildCuratedChannelsFromRaw()
			body, err := json.MarshalIndent(map[string]interface{}{
				"ok":               true,
				"recipe":           saved,
				"curated_channels": len(s.Channels),
			}, "", "  ")
			if err != nil {
				writeServerJSONError(w, http.StatusInternalServerError, "encode programming channels")
				return
			}
			_, _ = w.Write(body)
		default:
			writeMethodNotAllowedJSON(w, http.MethodGet, http.MethodPost)
		}
	})
}

func (s *Server) serveProgrammingOrder() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			if !operatorUIAllowed(w, r) {
				return
			}
			recipe := s.reloadProgrammingRecipe()
			body, err := json.MarshalIndent(map[string]interface{}{
				"generated_at":     time.Now().UTC().Format(time.RFC3339),
				"recipe_file":      strings.TrimSpace(s.ProgrammingRecipeFile),
				"order_mode":       recipe.OrderMode,
				"custom_order":     recipe.CustomOrder,
				"curated_channels": len(s.Channels),
				"collapse_backups": recipe.CollapseExactBackups,
			}, "", "  ")
			if err != nil {
				writeServerJSONError(w, http.StatusInternalServerError, "encode programming order")
				return
			}
			_, _ = w.Write(body)
		case http.MethodPost:
			if !operatorUIAllowed(w, r) {
				return
			}
			if strings.TrimSpace(s.ProgrammingRecipeFile) == "" {
				writeServerJSONError(w, http.StatusServiceUnavailable, "programming recipe file not configured")
				return
			}
			limited := http.MaxBytesReader(w, r.Body, 65536)
			defer limited.Close()
			var req struct {
				Action          string   `json:"action"`
				ChannelID       string   `json:"channel_id"`
				ChannelIDs      []string `json:"channel_ids"`
				BeforeChannelID string   `json:"before_channel_id"`
				AfterChannelID  string   `json:"after_channel_id"`
			}
			if err := json.NewDecoder(limited).Decode(&req); err != nil {
				writeServerJSONError(w, http.StatusBadRequest, "invalid programming order json")
				return
			}
			ids := append([]string(nil), req.ChannelIDs...)
			if strings.TrimSpace(req.ChannelID) != "" {
				ids = append(ids, strings.TrimSpace(req.ChannelID))
			}
			recipe := programming.UpdateRecipeOrder(s.reloadProgrammingRecipe(), req.Action, ids, req.BeforeChannelID, req.AfterChannelID)
			saved, err := programming.SaveRecipeFile(s.ProgrammingRecipeFile, recipe)
			if err != nil {
				writeServerJSONError(w, http.StatusBadGateway, "save programming recipe failed")
				return
			}
			s.ProgrammingRecipe = saved
			s.rebuildCuratedChannelsFromRaw()
			body, err := json.MarshalIndent(map[string]interface{}{
				"ok":               true,
				"recipe":           saved,
				"curated_channels": len(s.Channels),
			}, "", "  ")
			if err != nil {
				writeServerJSONError(w, http.StatusInternalServerError, "encode programming order")
				return
			}
			_, _ = w.Write(body)
		default:
			writeMethodNotAllowedJSON(w, http.MethodGet, http.MethodPost)
		}
	})
}

func (s *Server) serveProgrammingBackups() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			if !operatorUIAllowed(w, r) {
				return
			}
		case http.MethodPost:
			if !operatorUIAllowed(w, r) {
				return
			}
			if strings.TrimSpace(s.ProgrammingRecipeFile) == "" {
				writeServerJSONError(w, http.StatusServiceUnavailable, "programming recipe file not configured")
				return
			}
			limited := http.MaxBytesReader(w, r.Body, 65536)
			defer limited.Close()
			var req struct {
				Action     string   `json:"action"`
				ChannelID  string   `json:"channel_id"`
				ChannelIDs []string `json:"channel_ids"`
			}
			if err := json.NewDecoder(limited).Decode(&req); err != nil {
				writeServerJSONError(w, http.StatusBadRequest, "invalid programming backups json")
				return
			}
			ids := append([]string(nil), req.ChannelIDs...)
			if strings.TrimSpace(req.ChannelID) != "" {
				ids = append(ids, strings.TrimSpace(req.ChannelID))
			}
			recipe := programming.UpdateRecipeBackupPreferences(s.reloadProgrammingRecipe(), req.Action, ids)
			saved, err := programming.SaveRecipeFile(s.ProgrammingRecipeFile, recipe)
			if err != nil {
				writeServerJSONError(w, http.StatusBadGateway, "save programming recipe failed")
				return
			}
			s.ProgrammingRecipe = saved
			s.rebuildCuratedChannelsFromRaw()
		default:
			writeMethodNotAllowedJSON(w, http.MethodGet, http.MethodPost)
			return
		}
		recipe := s.reloadProgrammingRecipe()
		preview := programming.ApplyRecipePreview(cloneLiveChannels(s.RawChannels), recipe)
		groups := programming.BuildBackupGroupsWithPreferences(preview, recipe.PreferredBackupIDs)
		body, err := json.MarshalIndent(map[string]interface{}{
			"generated_at":         time.Now().UTC().Format(time.RFC3339),
			"recipe_file":          strings.TrimSpace(s.ProgrammingRecipeFile),
			"collapse_enabled":     recipe.CollapseExactBackups,
			"preferred_backup_ids": append([]string(nil), recipe.PreferredBackupIDs...),
			"raw_channels":         len(s.RawChannels),
			"curated_preview":      len(preview),
			"group_count":          len(groups),
			"groups":               groups,
		}, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode programming backups")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveProgrammingHarvest() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			if !operatorUIAllowed(w, r) {
				return
			}
			rep := s.reloadPlexLineupHarvest()
			body, err := json.MarshalIndent(map[string]interface{}{
				"generated_at":     time.Now().UTC().Format(time.RFC3339),
				"harvest_file":     strings.TrimSpace(s.PlexLineupHarvestFile),
				"harvest_writable": strings.TrimSpace(s.PlexLineupHarvestFile) != "",
				"report":           rep,
				"lineups":          rep.Lineups,
				"report_ready":     len(rep.Results) > 0 || len(rep.Lineups) > 0,
			}, "", "  ")
			if err != nil {
				writeServerJSONError(w, http.StatusInternalServerError, "encode programming harvest")
				return
			}
			_, _ = w.Write(body)
		case http.MethodPost:
			if !operatorUIAllowed(w, r) {
				return
			}
			if strings.TrimSpace(s.PlexLineupHarvestFile) == "" {
				writeServerJSONError(w, http.StatusServiceUnavailable, "plex lineup harvest file not configured")
				return
			}
			limited := http.MaxBytesReader(w, r.Body, 1<<20)
			defer limited.Close()
			var rep plexharvest.Report
			if err := json.NewDecoder(limited).Decode(&rep); err != nil {
				writeServerJSONError(w, http.StatusBadRequest, "invalid programming harvest json")
				return
			}
			saved, err := s.savePlexLineupHarvest(rep)
			if err != nil {
				writeServerJSONError(w, http.StatusBadGateway, "save programming harvest failed")
				return
			}
			body, err := json.MarshalIndent(map[string]interface{}{
				"ok":           true,
				"harvest_file": strings.TrimSpace(s.PlexLineupHarvestFile),
				"report":       saved,
				"lineups":      saved.Lineups,
			}, "", "  ")
			if err != nil {
				writeServerJSONError(w, http.StatusInternalServerError, "encode programming harvest")
				return
			}
			_, _ = w.Write(body)
		default:
			writeMethodNotAllowedJSON(w, http.MethodGet, http.MethodPost)
		}
	})
}

func (s *Server) serveProgrammingHarvestImport() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		rep := s.reloadPlexLineupHarvest()
		switch r.Method {
		case http.MethodGet:
			if !operatorUIAllowed(w, r) {
				return
			}
			lineupTitle := strings.TrimSpace(r.URL.Query().Get("lineup_title"))
			friendlyName := strings.TrimSpace(r.URL.Query().Get("friendly_name"))
			replace := true
			if raw := strings.TrimSpace(r.URL.Query().Get("replace")); raw != "" {
				replace = raw != "0" && !strings.EqualFold(raw, "false")
			}
			collapse := false
			if raw := strings.TrimSpace(r.URL.Query().Get("collapse_exact_backups")); raw != "" {
				collapse = raw == "1" || strings.EqualFold(raw, "true")
			}
			result, ok := chooseHarvestResult(rep, lineupTitle, friendlyName)
			if !ok {
				writeServerJSONError(w, http.StatusNotFound, "harvest result not found")
				return
			}
			report := buildProgrammingHarvestImport(s.reloadProgrammingRecipe(), s.RawChannels, result, replace, collapse)
			report.HarvestFile = strings.TrimSpace(s.PlexLineupHarvestFile)
			body, err := json.MarshalIndent(report, "", "  ")
			if err != nil {
				writeServerJSONError(w, http.StatusInternalServerError, "encode programming harvest import")
				return
			}
			_, _ = w.Write(body)
		case http.MethodPost:
			if !operatorUIAllowed(w, r) {
				return
			}
			if strings.TrimSpace(s.ProgrammingRecipeFile) == "" {
				writeServerJSONError(w, http.StatusServiceUnavailable, "programming recipe file not configured")
				return
			}
			limited := http.MaxBytesReader(w, r.Body, 65536)
			defer limited.Close()
			var req struct {
				LineupTitle          string `json:"lineup_title"`
				FriendlyName         string `json:"friendly_name"`
				Replace              *bool  `json:"replace,omitempty"`
				CollapseExactBackups bool   `json:"collapse_exact_backups"`
			}
			if err := json.NewDecoder(limited).Decode(&req); err != nil {
				writeServerJSONError(w, http.StatusBadRequest, "invalid programming harvest import json")
				return
			}
			replace := true
			if req.Replace != nil {
				replace = *req.Replace
			}
			result, ok := chooseHarvestResult(rep, req.LineupTitle, req.FriendlyName)
			if !ok {
				writeServerJSONError(w, http.StatusNotFound, "harvest result not found")
				return
			}
			report := buildProgrammingHarvestImport(s.reloadProgrammingRecipe(), s.RawChannels, result, replace, req.CollapseExactBackups)
			saved, err := programming.SaveRecipeFile(s.ProgrammingRecipeFile, report.Recipe)
			if err != nil {
				writeServerJSONError(w, http.StatusBadGateway, "save programming recipe failed")
				return
			}
			s.ProgrammingRecipe = saved
			s.rebuildCuratedChannelsFromRaw()
			report.Recipe = saved
			report.HarvestFile = strings.TrimSpace(s.PlexLineupHarvestFile)
			body, err := json.MarshalIndent(map[string]interface{}{
				"ok":               true,
				"report":           report,
				"curated_channels": len(s.Channels),
			}, "", "  ")
			if err != nil {
				writeServerJSONError(w, http.StatusInternalServerError, "encode programming harvest import")
				return
			}
			_, _ = w.Write(body)
		default:
			writeMethodNotAllowedJSON(w, http.MethodGet, http.MethodPost)
		}
	})
}

func (s *Server) serveProgrammingHarvestAssist() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		rep := s.reloadPlexLineupHarvest()
		report := programmingHarvestAssistReport{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			HarvestFile: strings.TrimSpace(s.PlexLineupHarvestFile),
		}
		for _, row := range rep.Lineups {
			result, ok := chooseHarvestResult(rep, row.LineupTitle, "")
			if !ok {
				continue
			}
			report.Assists = append(report.Assists, buildProgrammingHarvestAssist(s.RawChannels, row, result))
		}
		sort.SliceStable(report.Assists, func(i, j int) bool {
			ai := report.Assists[i]
			aj := report.Assists[j]
			if ai.Recommended != aj.Recommended {
				return ai.Recommended
			}
			if ai.LocalBroadcastHits != aj.LocalBroadcastHits {
				return ai.LocalBroadcastHits > aj.LocalBroadcastHits
			}
			if ai.MatchedChannels != aj.MatchedChannels {
				return ai.MatchedChannels > aj.MatchedChannels
			}
			return ai.LineupTitle < aj.LineupTitle
		})
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode programming harvest assist")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveProgrammingChannelDetail() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		channelID := strings.TrimSpace(r.URL.Query().Get("channel_id"))
		if channelID == "" {
			writeServerJSONError(w, http.StatusBadRequest, "channel_id required")
			return
		}
		sourceChannels := s.RawChannels
		if len(sourceChannels) == 0 {
			sourceChannels = s.Channels
		}
		var target catalog.LiveChannel
		found := false
		for _, ch := range sourceChannels {
			if strings.TrimSpace(ch.ChannelID) == channelID {
				target = ch
				found = true
				break
			}
		}
		if !found {
			writeServerJSONError(w, http.StatusNotFound, "channel not found")
			return
		}
		categoryID, categoryLabel, categorySource := programming.CategoryIdentity(target)
		report := programmingChannelDetailReport{
			GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
			Channel:        target,
			Descriptor:     programming.DescribeChannel(target),
			Curated:        containsLiveChannelID(s.Channels, channelID),
			CategoryID:     categoryID,
			CategoryLabel:  categoryLabel,
			CategorySource: categorySource,
			Bucket:         string(programming.ClassifyChannel(target)),
		}
		for _, group := range programming.BuildBackupGroups(sourceChannels) {
			member := false
			for _, row := range group.Members {
				if strings.TrimSpace(row.ChannelID) == channelID {
					member = true
					continue
				}
				report.AlternativeSources = append(report.AlternativeSources, row)
			}
			if member {
				groupCopy := group
				report.ExactBackupGroup = &groupCopy
				break
			}
		}
		horizon := 3 * time.Hour
		if raw := strings.TrimSpace(r.URL.Query().Get("horizon")); raw != "" {
			if d, err := time.ParseDuration(raw); err == nil && d > 0 {
				horizon = d
			}
		}
		limit := 6
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 && n <= 50 {
				limit = n
			}
		}
		if s.xmltv != nil {
			if preview, err := s.xmltv.CatchupCapsulePreview(time.Now(), horizon, 256); err == nil {
				report.SourceReady = preview.SourceReady
				for _, capsule := range preview.Capsules {
					if strings.TrimSpace(capsule.GuideNumber) != strings.TrimSpace(target.GuideNumber) {
						continue
					}
					report.UpcomingProgrammes = append(report.UpcomingProgrammes, capsule)
					if len(report.UpcomingProgrammes) >= limit {
						break
					}
				}
			}
		}
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode programming channel detail")
			return
		}
		_, _ = w.Write(body)
	})
}

func (s *Server) serveProgrammingRecipe() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			if !operatorUIAllowed(w, r) {
				return
			}
			recipe := s.reloadProgrammingRecipe()
			resp := map[string]interface{}{
				"recipe":          recipe,
				"recipe_file":     strings.TrimSpace(s.ProgrammingRecipeFile),
				"recipe_writable": strings.TrimSpace(s.ProgrammingRecipeFile) != "",
			}
			body, err := json.MarshalIndent(resp, "", "  ")
			if err != nil {
				writeServerJSONError(w, http.StatusInternalServerError, "encode programming recipe")
				return
			}
			_, _ = w.Write(body)
		case http.MethodPost:
			if !operatorUIAllowed(w, r) {
				return
			}
			if strings.TrimSpace(s.ProgrammingRecipeFile) == "" {
				writeServerJSONError(w, http.StatusServiceUnavailable, "programming recipe file not configured")
				return
			}
			limited := http.MaxBytesReader(w, r.Body, 65536)
			defer limited.Close()
			var recipe programming.Recipe
			if err := json.NewDecoder(limited).Decode(&recipe); err != nil {
				writeServerJSONError(w, http.StatusBadRequest, "invalid programming recipe json")
				return
			}
			saved, err := programming.SaveRecipeFile(s.ProgrammingRecipeFile, recipe)
			if err != nil {
				writeServerJSONError(w, http.StatusBadGateway, "save programming recipe failed")
				return
			}
			s.ProgrammingRecipe = saved
			s.rebuildCuratedChannelsFromRaw()
			body, err := json.MarshalIndent(map[string]interface{}{
				"ok":               true,
				"recipe":           saved,
				"recipe_file":      strings.TrimSpace(s.ProgrammingRecipeFile),
				"curated_channels": len(s.Channels),
			}, "", "  ")
			if err != nil {
				writeServerJSONError(w, http.StatusInternalServerError, "encode programming recipe")
				return
			}
			_, _ = w.Write(body)
		default:
			writeMethodNotAllowedJSON(w, http.MethodGet, http.MethodPost)
		}
	})
}

func containsLiveChannelID(channels []catalog.LiveChannel, channelID string) bool {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return false
	}
	for _, ch := range channels {
		if strings.TrimSpace(ch.ChannelID) == channelID {
			return true
		}
	}
	return false
}

func (s *Server) serveProgrammingPreview() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowedJSON(w, http.MethodGet)
			return
		}
		if !operatorUIAllowed(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		limit := streamAttemptLimitFromQuery(r.URL.Query().Get("limit"), 25)
		recipe := s.reloadProgrammingRecipe()
		lineupPreview := programming.ApplyRecipe(cloneLiveChannels(s.RawChannels), recipe)
		backupPreview := programming.ApplyRecipePreview(cloneLiveChannels(s.RawChannels), recipe)
		report := programmingPreviewReport{
			GeneratedAt:     time.Now().UTC().Format(time.RFC3339),
			RecipeFile:      strings.TrimSpace(s.ProgrammingRecipeFile),
			RecipeWritable:  strings.TrimSpace(s.ProgrammingRecipeFile) != "",
			HarvestFile:     strings.TrimSpace(s.PlexLineupHarvestFile),
			RawChannels:     len(s.RawChannels),
			CuratedChannels: len(lineupPreview),
			Recipe:          recipe,
			Inventory:       programming.BuildCategoryInventory(s.RawChannels),
		}
		harvest := s.reloadPlexLineupHarvest()
		report.HarvestReady = len(harvest.Results) > 0 || len(harvest.Lineups) > 0
		report.HarvestLineups = append([]plexharvest.SummaryLineup(nil), harvest.Lineups...)
		if limit > len(lineupPreview) {
			limit = len(lineupPreview)
		}
		report.Lineup = append([]catalog.LiveChannel(nil), lineupPreview[:limit]...)
		report.LineupDescriptors = programming.DescribeChannels(report.Lineup)
		report.Buckets = make(map[string]int)
		for _, ch := range lineupPreview {
			report.Buckets[string(programming.ClassifyChannel(ch))]++
		}
		report.BackupGroups = programming.BuildBackupGroups(backupPreview)
		body, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			writeServerJSONError(w, http.StatusInternalServerError, "encode programming preview")
			return
		}
		_, _ = w.Write(body)
	})
}
