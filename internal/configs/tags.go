package configs

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Tags says whether mods run on clients, servers or both, from their
// Thunderstore categories (Valheim: "Client-side", "Server-side"). Answers are
// cached for good in CachePath (the bash version's file: "mod<TAB>time<TAB>cat|cat";
// a mod Thunderstore doesn't know is cached with no categories). Before going
// online, Known (e.g. Gale's own cache) is asked too.
type Tags struct {
	CachePath string
	Known     func() map[string]string // "Author-Name" -> "cat|cat"; may be nil
	BaseURL   string                   // "" = https://thunderstore.io
	HTTP      *http.Client             // nil = 10 s timeout
	Now       func() time.Time
}

// DefaultCachePath is game-mod-sync/thunderstore-tags.tsv in the user's cache folder.
func DefaultCachePath() string {
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		if d, err := os.UserCacheDir(); err == nil {
			dir = d
		}
	}
	return filepath.Join(dir, "game-mod-sync", "thunderstore-tags.tsv")
}

func (t *Tags) cache() map[string]string {
	out := map[string]string{}
	b, err := os.ReadFile(t.CachePath)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Split(line, "\t")
		if len(f) >= 3 && f[0] != "" {
			if _, ok := out[f[0]]; !ok {
				out[f[0]] = f[2]
			}
		}
	}
	return out
}

func (t *Tags) known() map[string]string {
	k := t.cache()
	if t.Known != nil {
		for m, c := range t.Known() {
			if _, ok := k[m]; !ok {
				k[m] = c
			}
		}
	}
	return k
}

// Sides returns "client", "server", "both" or "" (unknown) for each mod. Mods
// not known yet are looked up online first (only Author-Name folders);
// lookups that fail leave them unknown. fetched is how many were looked up.
func (t *Tags) Sides(community string, client, server *regexp.Regexp, mods []string) (sides map[string]string, fetched int) {
	k := t.known()
	var need []string
	for _, m := range mods {
		if _, ok := k[m]; !ok && strings.Contains(m, "-") {
			need = append(need, m)
		}
	}
	if len(need) > 0 {
		if got, err := t.Fetch(community, need); err == nil {
			for m, c := range got {
				k[m] = c
			}
			fetched = len(need)
		}
	}
	sides = map[string]string{}
	for _, m := range mods {
		cl, sv := false, false
		for _, c := range strings.Split(k[m], "|") {
			if client != nil && client.MatchString(c) {
				cl = true
			}
			if server != nil && server.MatchString(c) {
				sv = true
			}
		}
		switch {
		case cl && sv:
			sides[m] = "both"
		case cl:
			sides[m] = "client"
		case sv:
			sides[m] = "server"
		default:
			sides[m] = ""
		}
	}
	return sides, fetched
}

// Fetch looks the mods up on Thunderstore (6 at a time) and stores the answers
// in the cache, replacing older ones. A package Thunderstore doesn't have gets
// no categories; a network error gets nothing. Returns what was learned, or
// an error if nothing could be looked up at all.
func (t *Tags) Fetch(community string, mods []string) (map[string]string, error) {
	base := t.BaseURL
	if base == "" {
		base = "https://thunderstore.io"
	}
	hc := t.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	var mu sync.Mutex
	got := map[string]string{}
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup
	for _, m := range mods {
		author, name, ok := strings.Cut(m, "-")
		if !ok {
			continue
		}
		wg.Add(1)
		go func(m string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			cats, ok := fetchOne(hc, fmt.Sprintf("%s/api/experimental/package/%s/%s/", base, author, name), community)
			if ok {
				mu.Lock()
				got[m] = cats
				mu.Unlock()
			}
		}(m)
	}
	wg.Wait()
	if len(got) == 0 {
		return nil, fmt.Errorf("couldn't reach Thunderstore")
	}
	return got, t.store(got)
}

func fetchOne(hc *http.Client, url, community string) (string, bool) {
	resp, err := hc.Get(url)
	if err != nil {
		return "", false // network error: unknown, ask again next time
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", true // not on Thunderstore: remembered as "no categories"
	}
	var pkg struct {
		Listings []struct {
			Community  string   `json:"community"`
			Categories []string `json:"categories"`
		} `json:"community_listings"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&pkg); err != nil {
		return "", false
	}
	var mine []int
	for i, l := range pkg.Listings {
		if l.Community == community {
			mine = append(mine, i)
		}
	}
	if len(mine) == 0 {
		for i := range pkg.Listings {
			mine = append(mine, i)
		}
	}
	set := map[string]bool{}
	for _, i := range mine {
		for _, c := range pkg.Listings[i].Categories {
			set[c] = true
		}
	}
	cats := make([]string, 0, len(set))
	for c := range set {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	return strings.Join(cats, "|"), true
}

func (t *Tags) store(got map[string]string) error {
	now := time.Now
	if t.Now != nil {
		now = t.Now
	}
	ts := strconv.FormatInt(now().Unix(), 10)
	var lines []string
	for m, c := range got {
		lines = append(lines, m+"\t"+ts+"\t"+c)
	}
	if b, err := os.ReadFile(t.CachePath); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			m, _, _ := strings.Cut(l, "\t")
			if _, replaced := got[m]; m != "" && !replaced {
				lines = append(lines, l)
			}
		}
	}
	sort.Strings(lines)
	if err := os.MkdirAll(filepath.Dir(t.CachePath), 0o755); err != nil {
		return err
	}
	tmp := t.CachePath + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, t.CachePath)
}
