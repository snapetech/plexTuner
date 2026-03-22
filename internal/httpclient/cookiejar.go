package httpclient

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"
)

type persistedCookie struct {
	Name     string `json:"name"`
	Value    string `json:"value"`
	Domain   string `json:"domain,omitempty"`
	Path     string `json:"path,omitempty"`
	Secure   bool   `json:"secure,omitempty"`
	Expires  int64  `json:"expires,omitempty"`
	HttpOnly bool   `json:"http_only,omitempty"`
}

func maybeCookieJarFromEnv() http.CookieJar {
	path := strings.TrimSpace(os.Getenv("IPTV_TUNERR_COOKIE_JAR_FILE"))
	if path == "" {
		return nil
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return jar
	}
	var saved map[string]map[string]*persistedCookie
	if err := json.Unmarshal(data, &saved); err != nil {
		return jar
	}
	now := time.Now().Unix()
	for host, cookies := range saved {
		host = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(host), "."))
		if host == "" {
			continue
		}
		for _, c := range cookies {
			if c == nil {
				continue
			}
			if c.Expires > 0 && c.Expires <= now {
				continue
			}
			scheme := "http"
			if c.Secure {
				scheme = "https"
			}
			u := &url.URL{Scheme: scheme, Host: host}
			jar.SetCookies(u, []*http.Cookie{{
				Name:     c.Name,
				Value:    c.Value,
				Domain:   c.Domain,
				Path:     c.Path,
				Secure:   c.Secure,
				Expires:  time.Unix(c.Expires, 0),
				HttpOnly: c.HttpOnly,
			}})
		}
	}
	return jar
}
