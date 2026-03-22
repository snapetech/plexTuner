package virtualchannels

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/catalog"
)

const RulesVersion = 1

type Ruleset struct {
	Version   int       `json:"version"`
	Channels  []Channel `json:"channels,omitempty"`
	UpdatedAt string    `json:"updated_at,omitempty"`
}

type Channel struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	GuideNumber  string  `json:"guide_number,omitempty"`
	GroupTitle   string  `json:"group_title,omitempty"`
	Enabled      bool    `json:"enabled"`
	LoopDailyUTC bool    `json:"loop_daily_utc"`
	Entries      []Entry `json:"entries,omitempty"`
}

type Entry struct {
	Type         string `json:"type"` // movie | episode
	MovieID      string `json:"movie_id,omitempty"`
	SeriesID     string `json:"series_id,omitempty"`
	EpisodeID    string `json:"episode_id,omitempty"`
	DurationMins int    `json:"duration_mins,omitempty"`
	Title        string `json:"title,omitempty"`
}

type PreviewSlot struct {
	ChannelID    string `json:"channel_id"`
	ChannelName  string `json:"channel_name"`
	GuideNumber  string `json:"guide_number,omitempty"`
	StartsAtUTC  string `json:"starts_at_utc"`
	EndsAtUTC    string `json:"ends_at_utc"`
	EntryType    string `json:"entry_type"`
	EntryID      string `json:"entry_id"`
	ResolvedName string `json:"resolved_name"`
	DurationMins int    `json:"duration_mins"`
}

type PreviewReport struct {
	GeneratedAt string        `json:"generated_at"`
	Channels    int           `json:"channels"`
	Slots       []PreviewSlot `json:"slots,omitempty"`
}

type ResolvedSlot struct {
	PreviewSlot
	SourceURL string `json:"source_url,omitempty"`
}

type ScheduleReport struct {
	GeneratedAt string        `json:"generated_at"`
	StartsAtUTC string        `json:"starts_at_utc"`
	EndsAtUTC   string        `json:"ends_at_utc"`
	Channels    int           `json:"channels"`
	Slots       []PreviewSlot `json:"slots,omitempty"`
}

func LoadFile(path string) (Ruleset, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return NormalizeRuleset(Ruleset{}), nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return NormalizeRuleset(Ruleset{}), nil
		}
		return Ruleset{}, err
	}
	var set Ruleset
	if err := json.Unmarshal(data, &set); err != nil {
		return Ruleset{}, err
	}
	return NormalizeRuleset(set), nil
}

func SaveFile(path string, set Ruleset) (Ruleset, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Ruleset{}, fmt.Errorf("virtual channels file not configured")
	}
	set = NormalizeRuleset(set)
	data, err := json.MarshalIndent(set, "", "  ")
	if err != nil {
		return Ruleset{}, err
	}
	dir := filepath.Dir(filepath.Clean(path))
	tmp, err := os.CreateTemp(dir, ".virtual-channels-*.json.tmp")
	if err != nil {
		return Ruleset{}, err
	}
	tmpName := tmp.Name()
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(tmpName)
		if writeErr != nil {
			return Ruleset{}, writeErr
		}
		return Ruleset{}, closeErr
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		_ = os.Remove(tmpName)
		return Ruleset{}, err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return Ruleset{}, err
	}
	return set, nil
}

func NormalizeRuleset(set Ruleset) Ruleset {
	set.Version = RulesVersion
	seen := map[string]struct{}{}
	out := make([]Channel, 0, len(set.Channels))
	for _, ch := range set.Channels {
		ch.ID = strings.TrimSpace(ch.ID)
		ch.Name = strings.TrimSpace(ch.Name)
		if ch.ID == "" || ch.Name == "" {
			continue
		}
		if _, ok := seen[ch.ID]; ok {
			continue
		}
		seen[ch.ID] = struct{}{}
		if ch.GuideNumber == "" {
			ch.GuideNumber = ch.ID
		}
		entries := make([]Entry, 0, len(ch.Entries))
		for _, entry := range ch.Entries {
			entry.Type = strings.ToLower(strings.TrimSpace(entry.Type))
			switch entry.Type {
			case "movie", "episode":
			default:
				continue
			}
			if entry.DurationMins <= 0 {
				entry.DurationMins = 30
			}
			entry.MovieID = strings.TrimSpace(entry.MovieID)
			entry.SeriesID = strings.TrimSpace(entry.SeriesID)
			entry.EpisodeID = strings.TrimSpace(entry.EpisodeID)
			entry.Title = strings.TrimSpace(entry.Title)
			entries = append(entries, entry)
		}
		ch.Entries = entries
		out = append(out, ch)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].GuideNumber == out[j].GuideNumber {
			return out[i].Name < out[j].Name
		}
		return out[i].GuideNumber < out[j].GuideNumber
	})
	set.Channels = out
	set.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return set
}

func BuildPreview(set Ruleset, movies []catalog.Movie, series []catalog.Series, now time.Time, perChannel int) PreviewReport {
	set = NormalizeRuleset(set)
	if perChannel <= 0 {
		perChannel = 4
	}
	report := PreviewReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Channels:    len(set.Channels),
	}
	start := now.UTC().Truncate(time.Minute)
	for _, ch := range set.Channels {
		if !ch.Enabled || len(ch.Entries) == 0 {
			continue
		}
		slots := previewSlotsForChannel(ch, movies, series, start, perChannel)
		report.Slots = append(report.Slots, slots...)
	}
	return report
}

func ResolveCurrentSlot(set Ruleset, channelID string, movies []catalog.Movie, series []catalog.Series, now time.Time) (ResolvedSlot, bool) {
	set = NormalizeRuleset(set)
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return ResolvedSlot{}, false
	}
	for _, ch := range set.Channels {
		if !ch.Enabled || strings.TrimSpace(ch.ID) != channelID || len(ch.Entries) == 0 {
			continue
		}
		dayStart := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
		totalMinutes := 0
		for _, entry := range ch.Entries {
			duration := entry.DurationMins
			if duration <= 0 {
				duration = 30
			}
			totalMinutes += duration
		}
		if totalMinutes <= 0 {
			return ResolvedSlot{}, false
		}
		offsetMinutes := int(now.UTC().Sub(dayStart) / time.Minute)
		if offsetMinutes < 0 {
			offsetMinutes = 0
		}
		offsetMinutes = offsetMinutes % totalMinutes
		cursor := dayStart
		for _, entry := range ch.Entries {
			duration := entry.DurationMins
			if duration <= 0 {
				duration = 30
			}
			entryEnd := cursor.Add(time.Duration(duration) * time.Minute)
			if offsetMinutes < duration {
				return ResolvedSlot{
					PreviewSlot: PreviewSlot{
						ChannelID:    ch.ID,
						ChannelName:  ch.Name,
						GuideNumber:  ch.GuideNumber,
						StartsAtUTC:  cursor.Format(time.RFC3339),
						EndsAtUTC:    entryEnd.Format(time.RFC3339),
						EntryType:    entry.Type,
						EntryID:      entryID(entry),
						ResolvedName: resolveEntryName(entry, movies, series),
						DurationMins: duration,
					},
					SourceURL: resolveEntryURL(entry, movies, series),
				}, true
			}
			offsetMinutes -= duration
			cursor = entryEnd
		}
		break
	}
	return ResolvedSlot{}, false
}

func BuildSchedule(set Ruleset, movies []catalog.Movie, series []catalog.Series, start time.Time, horizon time.Duration) ScheduleReport {
	set = NormalizeRuleset(set)
	if horizon <= 0 {
		horizon = 6 * time.Hour
	}
	start = start.UTC().Truncate(time.Minute)
	end := start.Add(horizon)
	report := ScheduleReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		StartsAtUTC: start.Format(time.RFC3339),
		EndsAtUTC:   end.Format(time.RFC3339),
		Channels:    len(set.Channels),
	}
	for _, ch := range set.Channels {
		if !ch.Enabled || len(ch.Entries) == 0 {
			continue
		}
		dayStart := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
		cursor := dayStart
		for cursor.Before(end) {
			for _, entry := range ch.Entries {
				duration := entry.DurationMins
				if duration <= 0 {
					duration = 30
				}
				slotEnd := cursor.Add(time.Duration(duration) * time.Minute)
				if slotEnd.After(start) && cursor.Before(end) {
					report.Slots = append(report.Slots, PreviewSlot{
						ChannelID:    ch.ID,
						ChannelName:  ch.Name,
						GuideNumber:  ch.GuideNumber,
						StartsAtUTC:  cursor.Format(time.RFC3339),
						EndsAtUTC:    slotEnd.Format(time.RFC3339),
						EntryType:    entry.Type,
						EntryID:      entryID(entry),
						ResolvedName: resolveEntryName(entry, movies, series),
						DurationMins: duration,
					})
				}
				cursor = slotEnd
				if !cursor.Before(end) {
					break
				}
			}
		}
	}
	return report
}

func previewSlotsForChannel(ch Channel, movies []catalog.Movie, series []catalog.Series, start time.Time, perChannel int) []PreviewSlot {
	cursor := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	out := make([]PreviewSlot, 0, perChannel)
	for i := 0; i < perChannel; i++ {
		entry := ch.Entries[i%len(ch.Entries)]
		duration := entry.DurationMins
		if duration <= 0 {
			duration = 30
		}
		out = append(out, PreviewSlot{
			ChannelID:    ch.ID,
			ChannelName:  ch.Name,
			GuideNumber:  ch.GuideNumber,
			StartsAtUTC:  cursor.Format(time.RFC3339),
			EndsAtUTC:    cursor.Add(time.Duration(duration) * time.Minute).Format(time.RFC3339),
			EntryType:    entry.Type,
			EntryID:      entryID(entry),
			ResolvedName: resolveEntryName(entry, movies, series),
			DurationMins: duration,
		})
		cursor = cursor.Add(time.Duration(duration) * time.Minute)
	}
	return out
}

func entryID(entry Entry) string {
	if entry.Type == "movie" {
		return entry.MovieID
	}
	return entry.SeriesID + ":" + entry.EpisodeID
}

func resolveEntryName(entry Entry, movies []catalog.Movie, series []catalog.Series) string {
	if entry.Title != "" {
		return entry.Title
	}
	if entry.Type == "movie" {
		for _, movie := range movies {
			if strings.TrimSpace(movie.ID) == entry.MovieID {
				return strings.TrimSpace(movie.Title)
			}
		}
		return entry.MovieID
	}
	for _, show := range series {
		if strings.TrimSpace(show.ID) != entry.SeriesID {
			continue
		}
		for _, season := range show.Seasons {
			for _, episode := range season.Episodes {
				if strings.TrimSpace(episode.ID) == entry.EpisodeID {
					title := strings.TrimSpace(show.Title)
					epTitle := strings.TrimSpace(episode.Title)
					if title != "" && epTitle != "" {
						return title + " · " + epTitle
					}
					if epTitle != "" {
						return epTitle
					}
					return title
				}
			}
		}
	}
	if entry.EpisodeID != "" {
		return entry.EpisodeID
	}
	return entry.SeriesID
}

func resolveEntryURL(entry Entry, movies []catalog.Movie, series []catalog.Series) string {
	if entry.Type == "movie" {
		for _, movie := range movies {
			if strings.TrimSpace(movie.ID) == entry.MovieID {
				return strings.TrimSpace(movie.StreamURL)
			}
		}
		return ""
	}
	for _, show := range series {
		if strings.TrimSpace(show.ID) != entry.SeriesID {
			continue
		}
		for _, season := range show.Seasons {
			for _, episode := range season.Episodes {
				if strings.TrimSpace(episode.ID) == entry.EpisodeID {
					return strings.TrimSpace(episode.StreamURL)
				}
			}
		}
	}
	return ""
}
