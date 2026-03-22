package entitlements

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const Version = 1

type User struct {
	Username             string   `json:"username"`
	Password             string   `json:"password"`
	AllowLive            bool     `json:"allow_live"`
	AllowMovies          bool     `json:"allow_movies"`
	AllowSeries          bool     `json:"allow_series"`
	AllowedChannelIDs    []string `json:"allowed_channel_ids,omitempty"`
	AllowedTVGIDs        []string `json:"allowed_tvg_ids,omitempty"`
	AllowedCategoryIDs   []string `json:"allowed_category_ids,omitempty"`
	AllowedCategoryNames []string `json:"allowed_category_names,omitempty"`
	AllowedSourceTags    []string `json:"allowed_source_tags,omitempty"`
	AllowedMovieIDs      []string `json:"allowed_movie_ids,omitempty"`
	AllowedSeriesIDs     []string `json:"allowed_series_ids,omitempty"`
}

type Ruleset struct {
	Version   int    `json:"version"`
	UpdatedAt string `json:"updated_at,omitempty"`
	Users     []User `json:"users"`
}

func NormalizeRuleset(set Ruleset) Ruleset {
	set.Version = Version
	out := make([]User, 0, len(set.Users))
	seen := map[string]struct{}{}
	for _, user := range set.Users {
		user = normalizeUser(user)
		if user.Username == "" || user.Password == "" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(user.Username))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, user)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return strings.ToLower(out[i].Username) < strings.ToLower(out[j].Username)
	})
	set.Users = out
	set.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return set
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
		return Ruleset{}, fmt.Errorf("entitlements file not configured")
	}
	set = NormalizeRuleset(set)
	data, err := json.MarshalIndent(set, "", "  ")
	if err != nil {
		return Ruleset{}, err
	}
	dir := filepath.Dir(filepath.Clean(path))
	tmp, err := os.CreateTemp(dir, ".xtream-entitlements-*.json.tmp")
	if err != nil {
		return Ruleset{}, err
	}
	name := tmp.Name()
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(name)
		if writeErr != nil {
			return Ruleset{}, writeErr
		}
		return Ruleset{}, closeErr
	}
	if err := os.Chmod(name, 0o600); err != nil {
		_ = os.Remove(name)
		return Ruleset{}, err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return Ruleset{}, err
	}
	return set, nil
}

func Authenticate(set Ruleset, username, password string) (User, bool) {
	username = strings.TrimSpace(username)
	password = strings.TrimSpace(password)
	for _, user := range NormalizeRuleset(set).Users {
		if user.Username == username && user.Password == password {
			return user, true
		}
	}
	return User{}, false
}

func (u User) LiveRestricted() bool {
	return len(u.AllowedChannelIDs)+len(u.AllowedTVGIDs)+len(u.AllowedCategoryIDs)+len(u.AllowedCategoryNames)+len(u.AllowedSourceTags) > 0
}

func (u User) MovieRestricted() bool {
	return len(u.AllowedMovieIDs)+len(u.AllowedCategoryNames) > 0
}

func (u User) SeriesRestricted() bool {
	return len(u.AllowedSeriesIDs)+len(u.AllowedCategoryNames) > 0
}

func normalizeUser(user User) User {
	user.Username = strings.TrimSpace(user.Username)
	user.Password = strings.TrimSpace(user.Password)
	user.AllowedChannelIDs = dedupeSorted(user.AllowedChannelIDs)
	user.AllowedTVGIDs = dedupeLowerSorted(user.AllowedTVGIDs)
	user.AllowedCategoryIDs = dedupeLowerSorted(user.AllowedCategoryIDs)
	user.AllowedCategoryNames = dedupeLowerSorted(user.AllowedCategoryNames)
	user.AllowedSourceTags = dedupeLowerSorted(user.AllowedSourceTags)
	user.AllowedMovieIDs = dedupeSorted(user.AllowedMovieIDs)
	user.AllowedSeriesIDs = dedupeSorted(user.AllowedSeriesIDs)
	return user
}

func dedupeSorted(items []string) []string {
	return dedupe(items, false)
}

func dedupeLowerSorted(items []string) []string {
	return dedupe(items, true)
}

func dedupe(items []string, lower bool) []string {
	seen := map[string]string{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		key := item
		if lower {
			key = strings.ToLower(item)
			item = key
		}
		seen[key] = item
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out
}
