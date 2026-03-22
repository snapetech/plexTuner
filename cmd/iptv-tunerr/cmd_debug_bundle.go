package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/config"
	"github.com/snapetech/iptvtunerr/internal/httpclient"
)

var debugBundleHTTPClient = httpclient.WithTimeout(15 * time.Second)

func debugBundleCommands() []commandSpec {
	return []commandSpec{
		{
			Name:    "debug-bundle",
			Summary: "Collect Tunerr-side diagnostic state into a shareable bundle directory or .tar.gz",
			Section: "Lab/ops",
			Run:     runDebugBundle,
		},
	}
}

func runDebugBundle(cfg *config.Config, args []string) {
	fs := flag.NewFlagSet("debug-bundle", flag.ExitOnError)
	serverURL := fs.String("url", "http://localhost:5004", "Base URL of running Tunerr server")
	outDir := fs.String("out", "", "Output directory (default: debug-scratch/ in current dir)")
	tarball := fs.Bool("tar", false, "Also create a .tar.gz archive of the bundle")
	redact := fs.Bool("redact", true, "Redact secrets from env vars and URLs (default: true)")
	noServer := fs.Bool("no-server", false, "Skip live server fetch (only collect local state files)")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: iptv-tunerr debug-bundle [flags]

Collect Tunerr-side diagnostic state and write it to a bundle directory.
The bundle can be shared with maintainers or fed into scripts/analyze-bundle.py.

What is collected:
  stream-attempts.json    Recent stream attempt records from /debug/stream-attempts.json
  provider-profile.json   Provider autopilot state from /provider/profile.json
  cf-learned.json         Per-host CF state (working UA, cf_clearance present)
  cookie-meta.json        Cookie jar metadata (names/domains/expiry, NO cookie values)
  env.json                IPTV_TUNERR_* env vars (secrets redacted by default)
  bundle-info.json        Timestamp, version, and collection summary

Flags:
`)
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	dir := strings.TrimSpace(*outDir)
	if dir == "" {
		dir = "debug-scratch"
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		fmt.Fprintf(os.Stderr, "Cannot create output dir %q: %v\n", dir, err)
		os.Exit(1)
	}

	base := strings.TrimRight(strings.TrimSpace(*serverURL), "/")
	now := time.Now().UTC()
	summary := map[string]string{}

	// Collect from live server.
	if !*noServer {
		for _, ep := range []struct {
			name string
			path string
		}{
			{"stream-attempts.json", "/debug/stream-attempts.json?limit=500"},
			{"provider-profile.json", "/provider/profile.json"},
		} {
			dest := filepath.Join(dir, ep.name)
			err := fetchURLToFile(base+ep.path, dest)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warn: fetch %s: %v\n", ep.path, err)
				summary[ep.name] = "error: " + err.Error()
			} else {
				summary[ep.name] = "ok"
			}
		}
	} else {
		summary["stream-attempts.json"] = "skipped (--no-server)"
		summary["provider-profile.json"] = "skipped (--no-server)"
	}

	// CF learned state file.
	cfLearnedPath := strings.TrimSpace(os.Getenv("IPTV_TUNERR_CF_LEARNED_FILE"))
	if cfLearnedPath == "" {
		if jar := strings.TrimSpace(os.Getenv("IPTV_TUNERR_COOKIE_JAR_FILE")); jar != "" {
			cfLearnedPath = filepath.Join(filepath.Dir(jar), "cf-learned.json")
		}
	}
	if cfLearnedPath != "" {
		dest := filepath.Join(dir, "cf-learned.json")
		if err := copyFile(cfLearnedPath, dest); err != nil {
			if !os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "warn: copy cf-learned: %v\n", err)
				summary["cf-learned.json"] = "error: " + err.Error()
			} else {
				summary["cf-learned.json"] = "not found at " + cfLearnedPath
			}
		} else {
			summary["cf-learned.json"] = "ok"
		}
	} else {
		summary["cf-learned.json"] = "skipped (IPTV_TUNERR_COOKIE_JAR_FILE not set)"
	}

	// Cookie jar metadata (no values).
	jarPath := strings.TrimSpace(os.Getenv("IPTV_TUNERR_COOKIE_JAR_FILE"))
	if jarPath != "" {
		dest := filepath.Join(dir, "cookie-meta.json")
		if err := writeCookieMetadata(jarPath, dest); err != nil {
			fmt.Fprintf(os.Stderr, "warn: cookie metadata: %v\n", err)
			summary["cookie-meta.json"] = "error: " + err.Error()
		} else {
			summary["cookie-meta.json"] = "ok"
		}
	} else {
		summary["cookie-meta.json"] = "skipped (IPTV_TUNERR_COOKIE_JAR_FILE not set)"
	}

	// Env vars.
	envDest := filepath.Join(dir, "env.json")
	if err := writeEnvDump(envDest, *redact); err != nil {
		fmt.Fprintf(os.Stderr, "warn: env dump: %v\n", err)
		summary["env.json"] = "error: " + err.Error()
	} else {
		summary["env.json"] = "ok"
	}

	// Bundle info.
	infoDest := filepath.Join(dir, "bundle-info.json")
	info := map[string]any{
		"collected_at": now.Format(time.RFC3339),
		"version":      Version,
		"server_url":   base,
		"files":        summary,
	}
	if data, err := json.MarshalIndent(info, "", "  "); err == nil {
		_ = os.WriteFile(infoDest, data, 0o600)
	}

	fmt.Printf("Bundle written to: %s\n", dir)
	keys := make([]string, 0, len(summary))
	for k := range summary {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Printf("  %-30s  %s\n", k, summary[k])
	}

	if *tarball {
		ts := now.Format("20060102-150405")
		tarPath := "tunerr-debug-" + ts + ".tar.gz"
		if err := createTarGz(tarPath, dir); err != nil {
			fmt.Fprintf(os.Stderr, "tarball: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Archive:  %s\n", tarPath)
	}
}

func fetchURLToFile(rawURL, destPath string) error {
	resp, err := debugBundleHTTPClient.Get(rawURL) //nolint:gosec
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	return os.WriteFile(destPath, data, 0o600)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600)
}

// writeCookieMetadata reads the cookie jar and writes a metadata-only version:
// cookie names, domains, expiry — no values.
func writeCookieMetadata(jarPath, destPath string) error {
	data, err := os.ReadFile(jarPath)
	if err != nil {
		return err
	}
	// Cookie jar is map[host]map[cookieName]*cookieJSON
	var raw map[string]map[string]map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse jar: %w", err)
	}
	type cookieMeta struct {
		Name    string `json:"name"`
		Domain  string `json:"domain,omitempty"`
		Path    string `json:"path,omitempty"`
		Expires int64  `json:"expires,omitempty"`
		Secure  bool   `json:"secure,omitempty"`
	}
	type hostEntry struct {
		Host    string       `json:"host"`
		Cookies []cookieMeta `json:"cookies"`
	}
	hosts := make([]hostEntry, 0, len(raw))
	for host, cookies := range raw {
		entry := hostEntry{Host: host}
		for name, ck := range cookies {
			m := cookieMeta{Name: name}
			if v, ok := ck["domain"].(string); ok {
				m.Domain = v
			}
			if v, ok := ck["path"].(string); ok {
				m.Path = v
			}
			if v, ok := ck["secure"].(bool); ok {
				m.Secure = v
			}
			if v, ok := ck["expires"].(float64); ok {
				m.Expires = int64(v)
			}
			entry.Cookies = append(entry.Cookies, m)
		}
		sort.Slice(entry.Cookies, func(i, j int) bool {
			return entry.Cookies[i].Name < entry.Cookies[j].Name
		})
		hosts = append(hosts, entry)
	}
	sort.Slice(hosts, func(i, j int) bool { return hosts[i].Host < hosts[j].Host })
	out, err := json.MarshalIndent(hosts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(destPath, out, 0o600)
}

// secretEnvSuffixes lists env var name suffixes whose values should be redacted.
var secretEnvSuffixes = []string{
	"_PASS", "_PASSWORD", "_TOKEN", "_SECRET", "_KEY", "_USER", "_USERNAME",
}

func writeEnvDump(destPath string, redact bool) error {
	prefix := "IPTV_TUNERR_"
	out := map[string]string{}
	for _, kv := range os.Environ() {
		idx := strings.IndexByte(kv, '=')
		if idx < 0 {
			continue
		}
		key := kv[:idx]
		val := kv[idx+1:]
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		if redact && isSecretEnvKey(key) {
			val = "[REDACTED]"
		}
		out[key] = val
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(destPath, data, 0o600)
}

func isSecretEnvKey(key string) bool {
	upper := strings.ToUpper(key)
	for _, suffix := range secretEnvSuffixes {
		if strings.HasSuffix(upper, suffix) {
			return true
		}
		if idx := strings.LastIndexByte(upper, '_'); idx > 0 {
			prefix := upper[:idx]
			if strings.HasSuffix(prefix, suffix) {
				return true
			}
		}
		if strings.Contains(upper, suffix+"_") {
			return true
		}
	}
	return false
}

func createTarGz(destPath, srcDir string) error {
	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(rel)
		if info.IsDir() {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	})
}
