// Package update checks whether a newer release of game-mod-sync exists, at
// most once a day (the answer is cached), with a short timeout so a missing
// network never holds the program up.
package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// LatestURL is the public mirror's latest release.
const LatestURL = "https://api.github.com/repos/doggpound/game-mod-sync/releases/latest"

// Checker looks up the latest release.
type Checker struct {
	URL       string // "" = LatestURL
	CachePath string // "" = DefaultCachePath()
	HTTP      *http.Client
	Now       func() time.Time
}

// Release is the latest release.
type Release struct {
	Tag     string    `json:"tag"` // e.g. v1.1.0
	URL     string    `json:"url"` // its page
	Checked time.Time `json:"checked"`
}

// DefaultCachePath is game-mod-sync/latest-release.json in the user's cache folder.
func DefaultCachePath() string {
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		dir, _ = os.UserCacheDir()
	}
	return filepath.Join(dir, "game-mod-sync", "latest-release.json")
}

// Newer returns the latest release when it's newer than current ("1.0.0"),
// else nil. Development and test builds (anything that isn't X.Y.Z) never
// check. Errors (no network) are nil too: there's nothing to say.
func (c Checker) Newer(current string) *Release {
	cur, ok := parse(current)
	if !ok {
		return nil
	}
	r, err := c.latest()
	if err != nil {
		return nil
	}
	if l, ok := parse(r.Tag); ok && less(cur, l) {
		return r
	}
	return nil
}

func (c Checker) latest() (*Release, error) {
	now := time.Now
	if c.Now != nil {
		now = c.Now
	}
	cache := c.CachePath
	if cache == "" {
		cache = DefaultCachePath()
	}
	var r Release
	if b, err := os.ReadFile(cache); err == nil && json.Unmarshal(b, &r) == nil && now().Sub(r.Checked) < 24*time.Hour {
		return &r, nil
	}
	url := c.URL
	if url == "" {
		url = LatestURL
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 2 * time.Second}
	}
	resp, err := hc.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	var gh struct {
		Tag string `json:"tag_name"`
		URL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&gh); err != nil {
		return nil, err
	}
	r = Release{Tag: gh.Tag, URL: gh.URL, Checked: now()}
	if b, err := json.Marshal(r); err == nil {
		os.MkdirAll(filepath.Dir(cache), 0o755)
		os.WriteFile(cache, b, 0o644)
	}
	return &r, nil
}

// parse reads "1.2.3" or "v1.2.3"; anything else (dev, 1.0.0-test-abc) isn't a release.
func parse(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func less(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}
