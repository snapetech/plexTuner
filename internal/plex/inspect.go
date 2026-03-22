package plex

import (
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/httpclient"

	_ "modernc.org/sqlite"
)

type DBInspectReport struct {
	PlexDataDir       string             `json:"plex_data_dir"`
	LibraryDBPath     string             `json:"library_db_path"`
	LibraryDBExists   bool               `json:"library_db_exists"`
	LibraryTables     []DBTableInspect   `json:"library_tables,omitempty"`
	ProviderResources []ProviderResource `json:"media_provider_resources,omitempty"`
	EPGDatabases      []EPGDBInspect     `json:"epg_databases,omitempty"`
}

type DBTableInspect struct {
	Name     string   `json:"name"`
	Columns  []string `json:"columns,omitempty"`
	RowCount int      `json:"row_count,omitempty"`
}

type ProviderResource struct {
	ID         int    `json:"id,omitempty"`
	Identifier string `json:"identifier"`
	URI        string `json:"uri"`
}

type EPGDBInspect struct {
	Path           string           `json:"path"`
	DVRUUID        string           `json:"dvr_uuid,omitempty"`
	Tables         []DBTableInspect `json:"tables,omitempty"`
	ChannelTagRows int              `json:"channel_tag_rows,omitempty"`
}

type APISnapshot struct {
	PlexBaseURL      string              `json:"plex_base_url"`
	TunerBaseURL     string              `json:"tuner_base_url,omitempty"`
	ServerIdentity   map[string]string   `json:"server_identity,omitempty"`
	Devices          []Device            `json:"devices,omitempty"`
	DVRs             []DVRInfo           `json:"dvrs,omitempty"`
	ProviderResponse *HTTPRequestRecord  `json:"media_providers,omitempty"`
	EndpointProbes   []HTTPRequestRecord `json:"endpoint_probes,omitempty"`
	TunerDiscover    *HTTPRequestRecord  `json:"tuner_discover,omitempty"`
	TunerLineup      *HTTPRequestRecord  `json:"tuner_lineup,omitempty"`
	TunerGuideHead   *HTTPRequestRecord  `json:"tuner_guide_head,omitempty"`
}

type DeviceAuditReport struct {
	PlexBaseURL string            `json:"plex_base_url"`
	Devices     []DeviceAuditItem `json:"devices,omitempty"`
}

type DeviceAuditItem struct {
	Key           string             `json:"key"`
	UUID          string             `json:"uuid"`
	DeviceID      string             `json:"device_id"`
	URI           string             `json:"uri"`
	PlexStatus    string             `json:"plex_status,omitempty"`
	Host          string             `json:"host,omitempty"`
	Port          string             `json:"port,omitempty"`
	ResolvedIPs   []string           `json:"resolved_ips,omitempty"`
	ResolveError  string             `json:"resolve_error,omitempty"`
	DiscoverProbe *HTTPRequestRecord `json:"discover_probe,omitempty"`
	LineupProbe   *HTTPRequestRecord `json:"lineup_probe,omitempty"`
	Reachable     bool               `json:"reachable"`
}

type HTTPRequestSpec struct {
	BaseURL string
	Method  string
	Path    string
	Query   url.Values
	Headers map[string]string
	Body    []byte
	Token   string
}

type HTTPRequestRecord struct {
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	Status      int               `json:"status"`
	ContentType string            `json:"content_type,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	BodyPreview string            `json:"body_preview,omitempty"`
	XMLRoot     string            `json:"xml_root,omitempty"`
	JSONKeys    []string          `json:"json_keys,omitempty"`
	DurationMS  int64             `json:"duration_ms"`
	Error       string            `json:"error,omitempty"`
}

func InspectPlexDB(plexDataDir string) (*DBInspectReport, error) {
	root := strings.TrimSpace(plexDataDir)
	if root == "" {
		return nil, fmt.Errorf("plex data dir required")
	}
	report := &DBInspectReport{
		PlexDataDir:   root,
		LibraryDBPath: filepath.Join(root, "Plug-in Support", "Databases", "com.plexapp.plugins.library.db"),
	}
	if _, err := os.Stat(report.LibraryDBPath); err != nil {
		if os.IsNotExist(err) {
			return report, nil
		}
		return nil, err
	}
	report.LibraryDBExists = true
	db, err := sql.Open("sqlite", report.LibraryDBPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	report.LibraryTables, _ = inspectSQLiteTables(db, []string{"%livetv%", "%channel%", "%lineup%", "media_provider_resources"})
	report.ProviderResources, _ = loadProviderResources(db)
	report.EPGDatabases, _ = inspectEPGDatabases(filepath.Dir(report.LibraryDBPath))
	return report, nil
}

func inspectSQLiteTables(db *sql.DB, filters []string) ([]DBTableInspect, error) {
	query := "SELECT name FROM sqlite_master WHERE type='table'"
	args := make([]any, 0, len(filters))
	if len(filters) > 0 {
		parts := make([]string, 0, len(filters))
		for _, f := range filters {
			if strings.Contains(f, "%") {
				parts = append(parts, "name LIKE ?")
			} else {
				parts = append(parts, "name = ?")
			}
			args = append(args, f)
		}
		query += " AND (" + strings.Join(parts, " OR ") + ")"
	}
	query += " ORDER BY name"
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DBTableInspect
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		item := DBTableInspect{Name: name}
		item.Columns, _ = sqliteColumnNames(db, name)
		item.RowCount = sqliteRowCount(db, name)
		out = append(out, item)
	}
	return out, rows.Err()
}

func sqliteColumnNames(db *sql.DB, table string) ([]string, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

func sqliteRowCount(db *sql.DB, table string) int {
	var n int
	_ = db.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", table)).Scan(&n)
	return n
}

func loadProviderResources(db *sql.DB) ([]ProviderResource, error) {
	rows, err := db.Query(`SELECT id, identifier, uri FROM media_provider_resources ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProviderResource
	for rows.Next() {
		var item ProviderResource
		if err := rows.Scan(&item.ID, &item.Identifier, &item.URI); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func inspectEPGDatabases(dbDir string) ([]EPGDBInspect, error) {
	paths, err := filepath.Glob(filepath.Join(dbDir, "tv.plex.providers.epg.xmltv-*.db"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	out := make([]EPGDBInspect, 0, len(paths))
	for _, path := range paths {
		item := EPGDBInspect{Path: path}
		base := filepath.Base(path)
		item.DVRUUID = strings.TrimSuffix(strings.TrimPrefix(base, "tv.plex.providers.epg.xmltv-"), ".db")
		db, err := sql.Open("sqlite", path)
		if err != nil {
			item.Tables = []DBTableInspect{{Name: "open_error:" + err.Error()}}
			out = append(out, item)
			continue
		}
		item.Tables, _ = inspectSQLiteTables(db, []string{"library_sections", "metadata_items", "tags"})
		if hasSQLiteTable(db, "tags") {
			_ = db.QueryRow(`SELECT COUNT(*) FROM tags WHERE tag_type = 310`).Scan(&item.ChannelTagRows)
		}
		_ = db.Close()
		out = append(out, item)
	}
	return out, nil
}

func hasSQLiteTable(db *sql.DB, table string) bool {
	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name = ?`, table).Scan(&name)
	return err == nil && name != ""
}

func SnapshotPlexAPI(plexBaseURL, plexToken, tunerBaseURL string, includeProbes bool) (*APISnapshot, error) {
	snap := &APISnapshot{
		PlexBaseURL:  strings.TrimSpace(plexBaseURL),
		TunerBaseURL: strings.TrimSpace(tunerBaseURL),
	}
	host, err := hostPortFromBaseURL(plexBaseURL)
	if err != nil {
		return nil, err
	}
	snap.ServerIdentity, _ = GetServerIdentity(plexBaseURL, plexToken)
	snap.Devices, _ = ListDevicesAPI(host, plexToken)
	snap.DVRs, _ = ListDVRsAPI(host, plexToken)
	if rec, err := DoHTTPRequest(HTTPRequestSpec{BaseURL: plexBaseURL, Method: http.MethodGet, Path: "/media/providers", Token: plexToken}); err == nil {
		snap.ProviderResponse = rec
	}
	if includeProbes {
		snap.EndpointProbes = probePlexEndpoints(plexBaseURL, plexToken)
		if strings.TrimSpace(tunerBaseURL) != "" {
			snap.TunerDiscover, _ = DoHTTPRequest(HTTPRequestSpec{BaseURL: tunerBaseURL, Method: http.MethodGet, Path: "/discover.json"})
			snap.TunerLineup, _ = DoHTTPRequest(HTTPRequestSpec{BaseURL: tunerBaseURL, Method: http.MethodGet, Path: "/lineup.json"})
			snap.TunerGuideHead, _ = DoHTTPRequest(HTTPRequestSpec{BaseURL: tunerBaseURL, Method: http.MethodHead, Path: "/guide.xml"})
		}
	}
	return snap, nil
}

func AuditPlexDevices(plexBaseURL, plexToken string) (*DeviceAuditReport, error) {
	host, err := hostPortFromBaseURL(plexBaseURL)
	if err != nil {
		return nil, err
	}
	devices, err := ListDevicesAPI(host, plexToken)
	if err != nil {
		return nil, err
	}
	report := &DeviceAuditReport{PlexBaseURL: strings.TrimSpace(plexBaseURL)}
	for _, dev := range devices {
		item := DeviceAuditItem{
			Key:        dev.Key,
			UUID:       dev.UUID,
			DeviceID:   dev.DeviceID,
			URI:        dev.URI,
			PlexStatus: dev.Status,
		}
		u, err := url.Parse(strings.TrimSpace(dev.URI))
		if err == nil {
			item.Host = u.Hostname()
			item.Port = u.Port()
			if item.Host != "" {
				if ips, err := net.LookupHost(item.Host); err == nil {
					item.ResolvedIPs = ips
				} else {
					item.ResolveError = err.Error()
				}
			}
		} else {
			item.ResolveError = err.Error()
		}
		if strings.TrimSpace(dev.URI) != "" {
			item.DiscoverProbe, _ = DoHTTPRequest(HTTPRequestSpec{BaseURL: dev.URI, Method: http.MethodGet, Path: "/discover.json"})
			item.LineupProbe, _ = DoHTTPRequest(HTTPRequestSpec{BaseURL: dev.URI, Method: http.MethodGet, Path: "/lineup_status.json"})
			item.Reachable = item.DiscoverProbe != nil && item.DiscoverProbe.Status == http.StatusOK
		}
		report.Devices = append(report.Devices, item)
	}
	return report, nil
}

func probePlexEndpoints(plexBaseURL, plexToken string) []HTTPRequestRecord {
	specs := []HTTPRequestSpec{
		{BaseURL: plexBaseURL, Method: http.MethodGet, Path: "/"},
		{BaseURL: plexBaseURL, Method: http.MethodGet, Path: "/identity"},
		{BaseURL: plexBaseURL, Method: http.MethodGet, Path: "/livetv/dvrs", Token: plexToken},
		{BaseURL: plexBaseURL, Method: http.MethodGet, Path: "/media/grabbers/devices", Token: plexToken},
		{BaseURL: plexBaseURL, Method: http.MethodGet, Path: "/media/grabbers/tv.plex.grabbers.hdhomerun/devices", Token: plexToken},
		{BaseURL: plexBaseURL, Method: http.MethodPost, Path: "/media/grabbers/devices/discover", Token: plexToken},
		{BaseURL: plexBaseURL, Method: http.MethodGet, Path: "/media/providers", Token: plexToken},
	}
	out := make([]HTTPRequestRecord, 0, len(specs))
	for _, spec := range specs {
		rec, err := DoHTTPRequest(spec)
		if err != nil {
			out = append(out, HTTPRequestRecord{Method: spec.Method, URL: strings.TrimRight(spec.BaseURL, "/") + spec.Path, Error: err.Error()})
			continue
		}
		out = append(out, *rec)
	}
	return out
}

func DoHTTPRequest(spec HTTPRequestSpec) (*HTTPRequestRecord, error) {
	method := strings.ToUpper(strings.TrimSpace(spec.Method))
	if method == "" {
		method = http.MethodGet
	}
	fullURL, err := plexURL(spec.BaseURL, spec.Path, spec.Token, spec.Query)
	if err != nil {
		return nil, err
	}
	var body io.Reader
	if len(spec.Body) > 0 {
		body = strings.NewReader(string(spec.Body))
	}
	req, err := http.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, err
	}
	for k, v := range spec.Headers {
		req.Header.Set(k, v)
	}
	start := time.Now()
	resp, err := httpclient.WithTimeout(60 * time.Second).Do(req)
	if err != nil {
		return &HTTPRequestRecord{
			Method:     method,
			URL:        fullURL,
			DurationMS: time.Since(start).Milliseconds(),
			Error:      err.Error(),
		}, nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 32*1024))
	record := &HTTPRequestRecord{
		Method:      method,
		URL:         fullURL,
		Status:      resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		Headers:     selectHeaders(resp.Header),
		BodyPreview: strings.TrimSpace(string(raw)),
		DurationMS:  time.Since(start).Milliseconds(),
	}
	if len(record.BodyPreview) > 2000 {
		record.BodyPreview = record.BodyPreview[:2000]
	}
	fillResponseHints(record, raw)
	return record, nil
}

func selectHeaders(h http.Header) map[string]string {
	keys := []string{"Allow", "Content-Type", "Content-Length", "Location", "X-Plex-Protocol", "X-Plex-Container-Start", "X-Plex-Container-Size"}
	out := map[string]string{}
	for _, k := range keys {
		if v := strings.TrimSpace(h.Get(k)); v != "" {
			out[k] = v
		}
	}
	return out
}

func fillResponseHints(record *HTTPRequestRecord, raw []byte) {
	if len(raw) == 0 {
		return
	}
	var root struct {
		XMLName xml.Name
	}
	if err := xml.Unmarshal(raw, &root); err == nil && root.XMLName.Local != "" {
		record.XMLRoot = root.XMLName.Local
		return
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err == nil {
		keys := make([]string, 0, len(obj))
		for k := range obj {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		record.JSONKeys = keys
	}
}

func GetServerIdentity(plexBaseURL, plexToken string) (map[string]string, error) {
	rec, err := DoHTTPRequest(HTTPRequestSpec{BaseURL: plexBaseURL, Method: http.MethodGet, Path: "/"})
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	if rec.XMLRoot == "" || strings.TrimSpace(rec.BodyPreview) == "" {
		return out, nil
	}
	var root struct {
		MachineIdentifier string `xml:"machineIdentifier,attr"`
		FriendlyName      string `xml:"friendlyName,attr"`
		Version           string `xml:"version,attr"`
		MyPlex            string `xml:"myPlex,attr"`
		MyPlexUsername    string `xml:"myPlexUsername,attr"`
		AllowTuners       string `xml:"allowTuners,attr"`
		LiveTV            string `xml:"livetv,attr"`
	}
	if err := xml.Unmarshal([]byte(rec.BodyPreview), &root); err != nil {
		return out, nil
	}
	if root.MachineIdentifier != "" {
		out["machine_identifier"] = root.MachineIdentifier
	}
	if root.FriendlyName != "" {
		out["friendly_name"] = root.FriendlyName
	}
	if root.Version != "" {
		out["version"] = root.Version
	}
	if root.MyPlex != "" {
		out["myplex"] = root.MyPlex
	}
	if root.MyPlexUsername != "" {
		out["myplex_username"] = root.MyPlexUsername
	}
	if root.AllowTuners != "" {
		out["allow_tuners"] = root.AllowTuners
	}
	if root.LiveTV != "" {
		out["livetv"] = root.LiveTV
	}
	return out, nil
}

func hostPortFromBaseURL(baseURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return "", err
	}
	if u.Host == "" {
		return "", fmt.Errorf("missing host")
	}
	return u.Host, nil
}
