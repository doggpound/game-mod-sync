package update

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func server(t *testing.T, tag string, calls *int) *httptest.Server {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		fmt.Fprintf(w, `{"tag_name":%q,"html_url":"https://example.com/releases/tag/%s"}`, tag, tag)
	}))
	t.Cleanup(s.Close)
	return s
}

func TestNewer(t *testing.T) {
	calls := 0
	s := server(t, "v1.2.0", &calls)
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	c := Checker{URL: s.URL, CachePath: filepath.Join(t.TempDir(), "latest.json"), Now: func() time.Time { return now }}
	if r := c.Newer("1.0.0"); r == nil || r.Tag != "v1.2.0" || r.URL == "" {
		t.Errorf("1.0.0: %+v", r)
	}
	if r := c.Newer("1.2.0"); r != nil {
		t.Errorf("same version: %+v", r)
	}
	if r := c.Newer("1.10.0"); r != nil {
		t.Errorf("newer than the release (numbers, not text): %+v", r)
	}
	if calls != 1 {
		t.Errorf("asked %d times within a day, want 1", calls)
	}
	now = now.Add(25 * time.Hour)
	c.Newer("1.0.0")
	if calls != 2 {
		t.Errorf("not asked again after a day: %d", calls)
	}
}

func TestDevelopmentBuildsDontCheck(t *testing.T) {
	calls := 0
	s := server(t, "v9.0.0", &calls)
	c := Checker{URL: s.URL, CachePath: filepath.Join(t.TempDir(), "latest.json")}
	for _, v := range []string{"dev", "0.2.0-test-abc", ""} {
		if r := c.Newer(v); r != nil {
			t.Errorf("%q: %+v", v, r)
		}
	}
	if calls != 0 {
		t.Errorf("asked %d times", calls)
	}
}

func TestNoNetworkSaysNothing(t *testing.T) {
	c := Checker{URL: "http://127.0.0.1:1", CachePath: filepath.Join(t.TempDir(), "latest.json")}
	if r := c.Newer("1.0.0"); r != nil {
		t.Errorf("%+v", r)
	}
}
