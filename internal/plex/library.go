package plex

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
)

type LibrarySection struct {
	Key       string
	Type      string
	Title     string
	Locations []string
}

type LibrarySectionPrefs map[string]string

type libraryMediaContainer struct {
	Directories []libraryDirectory `xml:"Directory"`
	Settings    []librarySetting   `xml:"Setting"`
}

type libraryDirectory struct {
	Key       string            `xml:"key,attr"`
	Type      string            `xml:"type,attr"`
	Title     string            `xml:"title,attr"`
	Locations []libraryLocation `xml:"Location"`
}

type libraryLocation struct {
	Path string `xml:"path,attr"`
}

type librarySetting struct {
	ID    string `xml:"id,attr"`
	Value string `xml:"value,attr"`
}

type LibraryCreateSpec struct {
	Name     string
	Type     string // "movie" | "show"
	Path     string
	Language string // defaults to en-US
}

func plexHTTPClient() *http.Client {
	return &http.Client{Timeout: 60 * time.Second}
}

func plexURL(baseURL, path, token string, q url.Values) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", fmt.Errorf("plex base url required")
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "http://" + baseURL
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("parse plex base url: %w", err)
	}
	u.Path = path
	qq := u.Query()
	for k, vals := range q {
		for _, v := range vals {
			qq.Add(k, v)
		}
	}
	if token != "" {
		qq.Set("X-Plex-Token", token)
	}
	u.RawQuery = qq.Encode()
	return u.String(), nil
}

func ListLibrarySections(plexBaseURL, plexToken string) ([]LibrarySection, error) {
	u, err := plexURL(plexBaseURL, "/library/sections", plexToken, nil)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := plexHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("list library sections: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("list library sections returned %d", resp.StatusCode)
	}
	var mc libraryMediaContainer
	if err := xml.Unmarshal(body, &mc); err != nil {
		return nil, fmt.Errorf("parse library sections: %w", err)
	}
	out := make([]LibrarySection, 0, len(mc.Directories))
	for _, d := range mc.Directories {
		sec := LibrarySection{Key: d.Key, Type: d.Type, Title: d.Title}
		for _, loc := range d.Locations {
			sec.Locations = append(sec.Locations, filepath.Clean(loc.Path))
		}
		out = append(out, sec)
	}
	return out, nil
}

func CreateLibrarySection(plexBaseURL, plexToken string, spec LibraryCreateSpec) (*LibrarySection, error) {
	if strings.TrimSpace(spec.Name) == "" {
		return nil, fmt.Errorf("library section name required")
	}
	spec.Path = filepath.Clean(strings.TrimSpace(spec.Path))
	if spec.Path == "" || spec.Path == "." || spec.Path == "/" {
		return nil, fmt.Errorf("library section path must be a specific directory")
	}
	if spec.Type != "movie" && spec.Type != "show" {
		return nil, fmt.Errorf("unsupported library section type %q", spec.Type)
	}
	agent, scanner := "tv.plex.agents.movie", "Plex Movie"
	if spec.Type == "show" {
		agent, scanner = "tv.plex.agents.series", "Plex TV Series"
	}
	lang := strings.TrimSpace(spec.Language)
	if lang == "" {
		lang = "en-US"
	}
	q := url.Values{}
	q.Set("type", spec.Type)
	q.Set("name", spec.Name)
	q.Set("agent", agent)
	q.Set("scanner", scanner)
	q.Set("language", lang)
	q.Set("location", spec.Path)
	u, err := plexURL(plexBaseURL, "/library/sections", plexToken, q)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := plexHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("create library section %q: %w", spec.Name, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("create library section %q returned %d: %s", spec.Name, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var mc libraryMediaContainer
	if err := xml.Unmarshal(body, &mc); err != nil {
		return nil, fmt.Errorf("parse create library section %q: %w", spec.Name, err)
	}
	if len(mc.Directories) == 0 {
		return nil, fmt.Errorf("create library section %q returned no Directory", spec.Name)
	}
	d := mc.Directories[0]
	sec := &LibrarySection{Key: d.Key, Type: d.Type, Title: d.Title}
	for _, loc := range d.Locations {
		sec.Locations = append(sec.Locations, filepath.Clean(loc.Path))
	}
	return sec, nil
}

func RefreshLibrarySection(plexBaseURL, plexToken, key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("library section key required")
	}
	u, err := plexURL(plexBaseURL, "/library/sections/"+key+"/refresh", plexToken, nil)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return err
	}
	resp, err := plexHTTPClient().Do(req)
	if err != nil {
		return fmt.Errorf("refresh library section %s: %w", key, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("refresh library section %s returned %d: %s", key, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func EnsureLibrarySection(plexBaseURL, plexToken string, spec LibraryCreateSpec) (section *LibrarySection, created bool, err error) {
	sections, err := ListLibrarySections(plexBaseURL, plexToken)
	if err != nil {
		return nil, false, err
	}
	wantPath := filepath.Clean(spec.Path)
	for _, sec := range sections {
		if sec.Title != spec.Name {
			continue
		}
		if sec.Type != spec.Type {
			return nil, false, fmt.Errorf("library %q exists with type=%s (wanted %s)", spec.Name, sec.Type, spec.Type)
		}
		for _, p := range sec.Locations {
			if filepath.Clean(p) == wantPath {
				return &sec, false, nil
			}
		}
		return nil, false, fmt.Errorf("library %q exists but path differs (have %v, want %s)", spec.Name, sec.Locations, wantPath)
	}
	sec, err := CreateLibrarySection(plexBaseURL, plexToken, spec)
	if err != nil {
		return nil, false, err
	}
	return sec, true, nil
}

func GetLibrarySectionPrefs(plexBaseURL, plexToken, key string) (LibrarySectionPrefs, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("library section key required")
	}
	u, err := plexURL(plexBaseURL, "/library/sections/"+key+"/prefs", plexToken, nil)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := plexHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("get library section prefs %s: %w", key, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get library section prefs %s returned %d: %s", key, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var mc libraryMediaContainer
	if err := xml.Unmarshal(body, &mc); err != nil {
		return nil, fmt.Errorf("parse library section prefs %s: %w", key, err)
	}
	out := LibrarySectionPrefs{}
	for _, s := range mc.Settings {
		if strings.TrimSpace(s.ID) == "" {
			continue
		}
		out[s.ID] = s.Value
	}
	return out, nil
}

func UpdateLibrarySectionPrefs(plexBaseURL, plexToken, key string, updates map[string]string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("library section key required")
	}
	if len(updates) == 0 {
		return nil
	}
	q := url.Values{}
	for k, v := range updates {
		if strings.TrimSpace(k) == "" {
			continue
		}
		q.Set(k, v)
	}
	if len(q) == 0 {
		return nil
	}
	u, err := plexURL(plexBaseURL, "/library/sections/"+key+"/prefs", plexToken, q)
	if err != nil {
		return err
	}
	var lastErr error
	for _, method := range []string{"PUT", "POST"} {
		req, err := http.NewRequest(method, u, nil)
		if err != nil {
			return err
		}
		resp, err := plexHTTPClient().Do(req)
		if err != nil {
			lastErr = fmt.Errorf("%s library section prefs %s: %w", method, key, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			lastErr = fmt.Errorf("%s library section prefs %s returned %d: %s", method, key, resp.StatusCode, strings.TrimSpace(string(body)))
			continue
		}
		// Plex may return 200 even when a method/path combo is a no-op. Verify.
		prefs, err := GetLibrarySectionPrefs(plexBaseURL, plexToken, key)
		if err != nil {
			lastErr = err
			continue
		}
		mismatch := []string{}
		for k, want := range updates {
			if got, ok := prefs[k]; ok && !prefsValuesEquivalent(got, want) {
				mismatch = append(mismatch, fmt.Sprintf("%s=%s (got %s)", k, want, got))
			}
		}
		if len(mismatch) == 0 {
			return nil
		}
		lastErr = fmt.Errorf("prefs update not applied for section %s: %s", key, strings.Join(mismatch, ", "))
	}
	if lastErr != nil {
		return lastErr
	}
	return nil
}

func prefsValuesEquivalent(got, want string) bool {
	got = strings.TrimSpace(strings.ToLower(got))
	want = strings.TrimSpace(strings.ToLower(want))
	if got == want {
		return true
	}
	// Plex section prefs often return bools as true/false even when 0/1 was accepted.
	switch {
	case (got == "true" && want == "1") || (got == "1" && want == "true"):
		return true
	case (got == "false" && want == "0") || (got == "0" && want == "false"):
		return true
	}
	return false
}
