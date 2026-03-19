package tuner

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type persistentCookieJar struct {
	file  string
	mu    sync.Mutex
	jar   http.CookieJar
	saved map[string]map[string]*httpCookie
}

type httpCookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain,omitempty"`
	Path     string `json:"path,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	Expires  int64  `json:"expires,omitempty"`
	HttpOnly bool   `json:"http_only,omitempty"`
}

func newPersistentCookieJar(file string) (*persistentCookieJar, error) {
	pj := &persistentCookieJar{
		file:  file,
		saved: make(map[string]map[string]*httpCookie),
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	pj.jar = jar
	if file != "" {
		if err := pj.loadFromFile(); err != nil {
			log.Printf("persistentCookieJar: load %q failed: %v (starting fresh)", file, err)
		} else {
			log.Printf("persistentCookieJar: loaded cookies from %q", file)
		}
	}
	return pj, nil
}

func cookieBucketHost(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	return strings.TrimPrefix(raw, ".")
}

func cookieStorageKey(c *http.Cookie) string {
	if c == nil {
		return ""
	}
	return strings.Join([]string{c.Name, c.Domain, c.Path}, "\x00")
}

func cookieRequestURL(host string, c *httpCookie) *url.URL {
	if c != nil && c.Secure {
		return &url.URL{Scheme: "https", Host: host}
	}
	return &url.URL{Scheme: "http", Host: host}
}

func cloneStoredCookie(c *httpCookie) *httpCookie {
	if c == nil {
		return nil
	}
	out := *c
	return &out
}

func (p *persistentCookieJar) Cookies(u *url.URL) []*http.Cookie {
	if p == nil {
		return nil
	}
	return p.jar.Cookies(u)
}

func (p *persistentCookieJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	if p == nil || len(cookies) == 0 {
		return
	}
	p.jar.SetCookies(u, cookies)
	host := ""
	if u != nil {
		host = cookieBucketHost(u.Hostname())
	}
	now := time.Now().Unix()
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, c := range cookies {
		if c == nil || strings.TrimSpace(c.Name) == "" {
			continue
		}
		bucket := host
		if domain := cookieBucketHost(c.Domain); domain != "" {
			bucket = domain
		}
		if bucket == "" {
			continue
		}
		if p.saved[bucket] == nil {
			p.saved[bucket] = make(map[string]*httpCookie)
		}
		if !c.Expires.IsZero() && c.Expires.Unix() <= now {
			delete(p.saved[bucket], cookieStorageKey(c))
			if len(p.saved[bucket]) == 0 {
				delete(p.saved, bucket)
			}
			continue
		}
		p.saved[bucket][cookieStorageKey(c)] = &httpCookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			Expires:  c.Expires.Unix(),
			HttpOnly: c.HttpOnly,
		}
	}
}

func (p *persistentCookieJar) loadFromFile() error {
	if p == nil || p.file == "" {
		return nil
	}
	data, err := os.ReadFile(p.file)
	if err != nil {
		return err
	}
	var saved map[string]map[string]*httpCookie
	if err := json.Unmarshal(data, &saved); err != nil {
		return err
	}
	now := time.Now().Unix()
	p.mu.Lock()
	defer p.mu.Unlock()
	for host, cookies := range saved {
		host = cookieBucketHost(host)
		if host == "" {
			continue
		}
		for key, c := range cookies {
			if c == nil {
				continue
			}
			if c.Expires > 0 && c.Expires <= now {
				continue
			}
			p.jar.SetCookies(cookieRequestURL(host, c), []*http.Cookie{{
				Name:     c.Name,
				Value:    c.Value,
				Domain:   c.Domain,
				Path:     c.Path,
				Secure:   c.Secure,
				Expires:  time.Unix(c.Expires, 0),
				HttpOnly: c.HttpOnly,
			}})
			if p.saved[host] == nil {
				p.saved[host] = make(map[string]*httpCookie)
			}
			p.saved[host][key] = cloneStoredCookie(c)
		}
	}
	return nil
}

func (p *persistentCookieJar) Save() error {
	if p == nil || p.file == "" {
		return nil
	}
	now := time.Now().Unix()
	p.mu.Lock()
	pruned := make(map[string]map[string]*httpCookie, len(p.saved))
	for host, cookies := range p.saved {
		for key, c := range cookies {
			if c == nil {
				continue
			}
			if c.Expires > 0 && c.Expires <= now {
				continue
			}
			if pruned[host] == nil {
				pruned[host] = make(map[string]*httpCookie)
			}
			pruned[host][key] = cloneStoredCookie(c)
		}
	}
	p.saved = pruned
	p.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(p.file), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(pruned, "", "  ")
	if err != nil {
		return err
	}
	tmp := p.file + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p.file)
}
