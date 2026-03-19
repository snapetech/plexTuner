package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/snapetech/iptvtunerr/internal/config"
)

// harFile is the minimal structure we care about in a browser HAR export.
// HAR spec: http://www.softwareishard.com/blog/har-12-spec/
type harFile struct {
	Log struct {
		Entries []struct {
			Request struct {
				URL     string `json:"url"`
				Cookies []struct {
					Name    string `json:"name"`
					Value   string `json:"value"`
					Domain  string `json:"domain"`
					Path    string `json:"path"`
					Secure  bool   `json:"secure"`
					Expires string `json:"expires"` // ISO8601 or empty
				} `json:"cookies"`
				Headers []struct {
					Name  string `json:"name"`
					Value string `json:"value"`
				} `json:"headers"`
			} `json:"request"`
		} `json:"entries"`
	} `json:"log"`
}

func cookieImportCommands() []commandSpec {
	return []commandSpec{
		{
			Name:    "import-cookies",
			Summary: "Import browser cookies (Netscape/paste) into the Tunerr cookie jar for Cloudflare clearance",
			Section: "Lab/ops",
			Run:     runImportCookies,
		},
	}
}

// httpCookieJSON matches the persistentCookieJar storage format in internal/tuner/gateway_cookiejar.go.
type httpCookieJSON struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain,omitempty"`
	Path     string `json:"path,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	Expires  int64  `json:"expires,omitempty"`
	HttpOnly bool   `json:"http_only,omitempty"`
}

func runImportCookies(cfg *config.Config, args []string) {
	fs := flag.NewFlagSet("import-cookies", flag.ExitOnError)
	jarFile := fs.String("jar", "", "Path to cookie jar JSON file (default: IPTV_TUNERR_COOKIE_JAR_FILE env var)")
	cookieStr := fs.String("cookie", "", `Cookie string to import: "name=value; name2=value2" — use -domain to associate`)
	netscapeFile := fs.String("netscape", "", "Path to Netscape/Mozilla cookie file exported from browser")
	harFileFlag := fs.String("har", "", "Path to HAR file exported from browser DevTools (Network → Save all as HAR with content)")
	domain := fs.String("domain", "", "Domain to associate cookies with when using -cookie string (e.g. provider.example.com)")
	path := fs.String("path", "/", "Cookie path when using -cookie string")
	secure := fs.Bool("secure", false, "Mark cookies as Secure when using -cookie string")
	ttlDays := fs.Int("ttl-days", 7, "Cookie TTL in days from now when no explicit expiry is set (0 = session)")
	dryRun := fs.Bool("dry-run", false, "Print what would be imported without writing")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: iptv-tunerr import-cookies [flags]

Import cookies into the Tunerr cookie jar so the Go HTTP client can present Cloudflare
clearance tokens (cf_clearance) and other session cookies to upstream providers.

When Cloudflare challenges a browser, the browser solves the JS challenge and stores
a "cf_clearance" cookie. Importing that cookie lets Tunerr bypass the same challenge.

Workflow:
  1. Open the provider URL in a browser — Cloudflare solves automatically.
  2. Export cookies using a browser extension (e.g. "Cookie-Editor" → Export as Netscape),
     or copy the Cookie header from browser DevTools (Network tab → Copy as cURL → extract Cookie: header value).
  3. Run:
       iptv-tunerr import-cookies -netscape /tmp/cookies.txt
     or:
       iptv-tunerr import-cookies -cookie "cf_clearance=abc123xyz; _ga=GA1.2.456" -domain provider.example.com

Flags:
`)
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)

	jar := strings.TrimSpace(*jarFile)
	if jar == "" {
		jar = strings.TrimSpace(os.Getenv("IPTV_TUNERR_COOKIE_JAR_FILE"))
	}
	if jar == "" {
		log.Print("Set -jar or IPTV_TUNERR_COOKIE_JAR_FILE to specify the cookie jar path")
		os.Exit(1)
	}

	if *cookieStr == "" && *netscapeFile == "" && *harFileFlag == "" {
		log.Print("Provide -cookie, -netscape, or -har; see -help for usage")
		os.Exit(1)
	}

	// Read existing jar (if present), merge, then write back.
	saved := loadJarFile(jar)

	var imported int
	expiry := int64(0)
	if *ttlDays > 0 {
		expiry = time.Now().Add(time.Duration(*ttlDays) * 24 * time.Hour).Unix()
	}

	if *cookieStr != "" {
		d := strings.TrimSpace(*domain)
		if d == "" {
			log.Print("-domain is required when using -cookie string (e.g. -domain provider.example.com)")
			os.Exit(1)
		}
		d = strings.TrimPrefix(d, ".")
		cookies := parseCookieString(*cookieStr, d, *path, *secure, expiry)
		for _, c := range cookies {
			if *dryRun {
				fmt.Printf("  [dry-run] import: domain=%s name=%s value=%.40s...\n", c.Domain, c.Name, c.Value)
			} else {
				storeCookie(saved, c)
			}
			imported++
		}
	}

	if *netscapeFile != "" {
		f, err := os.Open(*netscapeFile)
		if err != nil {
			log.Printf("Cannot open %s: %v", *netscapeFile, err)
			os.Exit(1)
		}
		defer f.Close()
		cookies, err := parseNetscapeCookies(f, expiry)
		if err != nil {
			log.Printf("Parse %s: %v", *netscapeFile, err)
			os.Exit(1)
		}
		for _, c := range cookies {
			if *dryRun {
				fmt.Printf("  [dry-run] import: domain=%s name=%s value=%.40s...\n", c.Domain, c.Name, c.Value)
			} else {
				storeCookie(saved, c)
			}
			imported++
		}
	}

	if *harFileFlag != "" {
		data, err := os.ReadFile(*harFileFlag)
		if err != nil {
			log.Printf("Cannot open HAR %s: %v", *harFileFlag, err)
			os.Exit(1)
		}
		cookies, err := parseHARCookies(data, expiry)
		if err != nil {
			log.Printf("Parse HAR %s: %v", *harFileFlag, err)
			os.Exit(1)
		}
		for _, c := range cookies {
			if *dryRun {
				fmt.Printf("  [dry-run] import: domain=%s name=%s value=%.40s...\n", c.Domain, c.Name, c.Value)
			} else {
				storeCookie(saved, c)
			}
			imported++
		}
	}

	if *dryRun {
		fmt.Printf("[dry-run] would import %d cookie(s) into %s\n", imported, jar)
		return
	}

	if err := writeJarFile(jar, saved); err != nil {
		log.Printf("Write jar %s: %v", jar, err)
		os.Exit(1)
	}
	log.Printf("Imported %d cookie(s) into %s", imported, jar)
}

// cookieStorageKey builds the same key used by persistentCookieJar.
func cookieStorageKey(name, domain, path string) string {
	return strings.Join([]string{name, domain, path}, "\x00")
}

func storeCookie(saved map[string]map[string]*httpCookieJSON, c *httpCookieJSON) {
	host := strings.TrimPrefix(strings.ToLower(c.Domain), ".")
	if host == "" {
		return
	}
	if saved[host] == nil {
		saved[host] = make(map[string]*httpCookieJSON)
	}
	key := cookieStorageKey(c.Name, c.Domain, c.Path)
	saved[host][key] = c
}

func loadJarFile(path string) map[string]map[string]*httpCookieJSON {
	out := make(map[string]map[string]*httpCookieJSON)
	data, err := os.ReadFile(path)
	if err != nil {
		return out // new jar
	}
	_ = json.Unmarshal(data, &out)
	return out
}

func writeJarFile(path string, saved map[string]map[string]*httpCookieJSON) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// parseCookieString parses "name=value; name2=value2" into cookie records.
func parseCookieString(raw, domain, path string, secure bool, expiry int64) []*httpCookieJSON {
	var out []*httpCookieJSON
	for _, part := range strings.Split(raw, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, value, _ := strings.Cut(part, "=")
		name = strings.TrimSpace(name)
		value = strings.TrimSpace(value)
		if name == "" {
			continue
		}
		out = append(out, &httpCookieJSON{
			Name:    name,
			Value:   value,
			Domain:  domain,
			Path:    path,
			Secure:  secure,
			Expires: expiry,
		})
	}
	return out
}

// parseNetscapeCookies parses the Netscape/Mozilla cookie file format.
// Format per line: domain  flag  path  secure  expiry  name  value
// Lines starting with # or empty are skipped.
// defaultExpiry is used when the file's expiry field is 0 (session cookie).
func parseNetscapeCookies(r io.Reader, defaultExpiry int64) ([]*httpCookieJSON, error) {
	var out []*httpCookieJSON
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		domain := strings.TrimSpace(fields[0])
		path := strings.TrimSpace(fields[2])
		secureStr := strings.TrimSpace(fields[3])
		expiryStr := strings.TrimSpace(fields[4])
		name := strings.TrimSpace(fields[5])
		value := strings.TrimSpace(fields[6])
		if domain == "" || name == "" {
			continue
		}
		secure := strings.EqualFold(secureStr, "true") || strings.EqualFold(secureStr, "TRUE")
		expiry, _ := strconv.ParseInt(expiryStr, 10, 64)
		if expiry == 0 && defaultExpiry > 0 {
			expiry = defaultExpiry
		}
		out = append(out, &httpCookieJSON{
			Name:    name,
			Value:   value,
			Domain:  strings.TrimPrefix(domain, "."),
			Path:    path,
			Secure:  secure,
			Expires: expiry,
		})
	}
	return out, scanner.Err()
}

// parseHARCookies extracts cookies from a browser HAR (HTTP Archive) export.
// HAR is the standard "Save all as HAR with content" format from Chrome/Firefox DevTools.
// All unique cookies across all entries are collected and deduplicated by name+domain+path.
func parseHARCookies(data []byte, defaultExpiry int64) ([]*httpCookieJSON, error) {
	var har harFile
	if err := json.Unmarshal(data, &har); err != nil {
		return nil, err
	}
	type key struct{ name, domain, path string }
	seen := make(map[key]*httpCookieJSON)
	for _, entry := range har.Log.Entries {
		for _, ck := range entry.Request.Cookies {
			name := strings.TrimSpace(ck.Name)
			value := strings.TrimSpace(ck.Value)
			domain := strings.TrimPrefix(strings.TrimSpace(ck.Domain), ".")
			if domain == "" {
				// Fall back to Host header in this entry's request.
				for _, h := range entry.Request.Headers {
					if strings.EqualFold(h.Name, "host") {
						domain = strings.TrimSpace(h.Value)
						// strip port if present
						if idx := strings.LastIndex(domain, ":"); idx > 0 {
							domain = domain[:idx]
						}
						break
					}
				}
			}
			if name == "" || domain == "" {
				continue
			}
			path := strings.TrimSpace(ck.Path)
			if path == "" {
				path = "/"
			}
			k := key{name, domain, path}
			expiry := defaultExpiry
			if ck.Expires != "" {
				if t, err := time.Parse(time.RFC3339, ck.Expires); err == nil {
					expiry = t.Unix()
				} else if t, err := time.Parse("2006-01-02T15:04:05.000Z", ck.Expires); err == nil {
					expiry = t.Unix()
				}
			}
			seen[k] = &httpCookieJSON{
				Name:    name,
				Value:   value,
				Domain:  domain,
				Path:    path,
				Secure:  ck.Secure,
				Expires: expiry,
			}
		}
	}
	out := make([]*httpCookieJSON, 0, len(seen))
	for _, c := range seen {
		out = append(out, c)
	}
	return out, nil
}
