package tuner

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path/filepath"
	"testing"
	"time"
)

func TestPrepareCloudflareAwareClient_UpgradesSharedCookieJar(t *testing.T) {
	t.Setenv("IPTV_TUNERR_CF_AUTO_BOOT", "false")
	t.Setenv("IPTV_TUNERR_COOKIE_JAR_FILE", filepath.Join(t.TempDir(), "cookies.json"))

	baseJar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	target, err := url.Parse("http://example.com/get.php")
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	baseJar.SetCookies(target, []*http.Cookie{{
		Name:    "cf_clearance",
		Value:   "abc123",
		Domain:  "example.com",
		Path:    "/",
		Expires: time.Now().Add(time.Hour),
	}})
	baseClient := &http.Client{
		Timeout: 5 * time.Second,
		Jar:     baseJar,
	}

	client, userAgents, err := PrepareCloudflareAwareClient(context.Background(), target.String(), baseClient, "")
	if err != nil {
		t.Fatalf("PrepareCloudflareAwareClient: %v", err)
	}
	if len(userAgents) != 0 {
		t.Fatalf("expected no learned user agents when CF auto-boot is disabled, got %v", userAgents)
	}
	if client == baseClient {
		t.Fatalf("expected PrepareCloudflareAwareClient to clone the shared client when upgrading its jar")
	}
	jar, ok := client.Jar.(*persistentCookieJar)
	if !ok {
		t.Fatalf("expected persistentCookieJar, got %T", client.Jar)
	}
	cookies := jar.Cookies(target)
	if len(cookies) != 1 {
		t.Fatalf("expected imported cookie, got %d cookies", len(cookies))
	}
	if cookies[0].Name != "cf_clearance" || cookies[0].Value != "abc123" {
		t.Fatalf("expected imported cf_clearance cookie, got %+v", cookies[0])
	}
}
