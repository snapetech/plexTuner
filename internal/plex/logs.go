package plex

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type LogInspectReport struct {
	LogDir      string              `json:"log_dir"`
	LogFiles    []string            `json:"log_files,omitempty"`
	Endpoints   []LogEndpointRecord `json:"endpoints,omitempty"`
	MatchedRows int                 `json:"matched_rows"`
}

type LogEndpointRecord struct {
	Method         string `json:"method"`
	Path           string `json:"path"`
	NormalizedPath string `json:"normalized_path"`
	Count          int    `json:"count"`
	Example        string `json:"example,omitempty"`
}

type plexLogEndpointAgg struct {
	LogEndpointRecord
}

var plexLogRequestPattern = regexp.MustCompile(`\b(?:Request|Completed): .*?\b(GET|POST|PUT|DELETE|HEAD|OPTIONS|PATCH)\s+(\S+)`)
var plexLogNumericSegment = regexp.MustCompile(`/\d+`)

func InspectPlexLogs(plexDataDir string) (*LogInspectReport, error) {
	root := strings.TrimSpace(plexDataDir)
	if root == "" {
		return nil, fmt.Errorf("plex data dir required")
	}
	logDir := filepath.Join(root, "Logs")
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return nil, err
	}
	report := &LogInspectReport{LogDir: logDir}
	byKey := map[string]*plexLogEndpointAgg{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "Plex Media Server") || !strings.HasSuffix(name, ".log") {
			continue
		}
		path := filepath.Join(logDir, name)
		report.LogFiles = append(report.LogFiles, path)
		if err := scanPlexLogFile(path, byKey, &report.MatchedRows); err != nil {
			return nil, err
		}
	}
	sort.Strings(report.LogFiles)
	report.Endpoints = make([]LogEndpointRecord, 0, len(byKey))
	for _, item := range byKey {
		report.Endpoints = append(report.Endpoints, item.LogEndpointRecord)
	}
	sort.Slice(report.Endpoints, func(i, j int) bool {
		if report.Endpoints[i].Count == report.Endpoints[j].Count {
			if report.Endpoints[i].Method == report.Endpoints[j].Method {
				return report.Endpoints[i].NormalizedPath < report.Endpoints[j].NormalizedPath
			}
			return report.Endpoints[i].Method < report.Endpoints[j].Method
		}
		return report.Endpoints[i].Count > report.Endpoints[j].Count
	})
	return report, nil
}

func scanPlexLogFile(path string, byKey map[string]*plexLogEndpointAgg, matchedRows *int) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		method, reqPath, ok := parsePlexLogRequest(line)
		if !ok {
			continue
		}
		*matchedRows++
		key := method + " " + normalizePlexPath(reqPath)
		item := byKey[key]
		if item == nil {
			item = &plexLogEndpointAgg{LogEndpointRecord{
				Method:         method,
				Path:           reqPath,
				NormalizedPath: normalizePlexPath(reqPath),
				Example:        strings.TrimSpace(line),
			}}
			byKey[key] = item
		}
		item.Count++
	}
	return sc.Err()
}

func parsePlexLogRequest(line string) (method, reqPath string, ok bool) {
	m := plexLogRequestPattern.FindStringSubmatch(line)
	if len(m) != 3 {
		return "", "", false
	}
	reqPath = strings.TrimSpace(m[2])
	if !isInterestingPlexPath(reqPath) {
		return "", "", false
	}
	return m[1], reqPath, true
}

func isInterestingPlexPath(path string) bool {
	path = strings.TrimSpace(path)
	for _, prefix := range []string{
		"/livetv/",
		"/media/grabbers/",
		"/media/providers",
		"/tv.plex.providers.epg.xmltv/",
	} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func normalizePlexPath(path string) string {
	base, _, _ := strings.Cut(path, "?")
	base = plexLogNumericSegment.ReplaceAllString(base, "/:id")
	return base
}
