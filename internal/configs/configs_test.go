package configs

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

const serverCfg = `## Settings file was created by plugin AzuAutoStore v1.0
[General]
## Lock it
Lock Configuration = Off
Range = 10

[Chests]
Allowed = Chest,Barrel
`

func TestDetailListsTheSettingsThatDiffer(t *testing.T) {
	mine := strings.ReplaceAll(strings.ReplaceAll(serverCfg, "= Off", "= On"), "Range = 10", "Range = 20")
	got := Detail("Azumatt.AzuAutoStore.cfg", mine, serverCfg)
	want := []string{"Lock Configuration: Off -> On", "Range: 10 -> 20"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestOnlyFormattingDiffersIsNoDifference(t *testing.T) {
	mine := strings.ReplaceAll("# a new comment\r\n"+serverCfg, "\n", "\r\n")
	mine = strings.ReplaceAll(mine, "Range = 10", "   Range =    10   ")
	if got := Detail("x.cfg", mine, serverCfg); len(got) != 0 {
		t.Errorf("got %q", got)
	}
	if got := Detail("x.yml", "a: 1\n\n# c\n", "a: 1\n"); len(got) != 0 {
		t.Errorf("yml: got %q", got)
	}
}

func TestDetailShortensAndCounts(t *testing.T) {
	var a, b strings.Builder
	for i := 0; i < 6; i++ {
		fmt.Fprintf(&a, "K%d = server\n", i)
		fmt.Fprintf(&b, "K%d = yours-a-very-long-value-indeed\n", i)
	}
	got := Detail("x.cfg", b.String(), a.String())
	if len(got) != 5 || got[0] != "K0: server -> yours-a-very-long-val..." || got[4] != "+2 more settings" {
		t.Errorf("got %q", got)
	}
	if got := Detail("x.cfg", "A = 1\n", ""); !reflect.DeepEqual(got, []string{"A: (none) -> 1"}) {
		t.Errorf("new setting: %q", got)
	}
	if got := Detail("x.yml", "a: 1\nb: 2\n", "a: 1\n"); !reflect.DeepEqual(got, []string{"1 line differ"}) {
		t.Errorf("lines: %q", got)
	}
}

func TestFullCfg(t *testing.T) {
	mine := strings.ReplaceAll(serverCfg, "= Off", "= On") + "New = x\n"
	want := "    [General] Lock Configuration\n        server: Off\n        yours:  On\n" +
		"    [Chests] New\n        server: (not set)\n        yours:  x"
	if got := Full("a.cfg", mine, serverCfg); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// A non-.cfg config that differs is shown as lines (this once crashed the 0.x scripts).
func TestFullLines(t *testing.T) {
	server := "Throttle:\n  # comment\n  Max: 50\nOther: 1\n"
	mine := "Throttle:\n  Max: 100\nOther: 1\nAdded: yes\n"
	want := "    server:   Max: 50\n    yours:    Max: 100\n    yours:  Added: yes"
	if got := Full("Advanced.yml", mine, server); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestOwners(t *testing.T) {
	mods := []string{"Azumatt-FastLink", "ArgusMagnus-ServersideQoL", "ArgusMagnus-ServersideQoL_AutoStore",
		"RandyKnapp-EpicLoot", "Someone-XRayVision", "A-Dup", "B-Dup", "ValheimModding-Jotunn"}
	ids := map[string]string{
		"Azumatt.FastLink":                    "Azumatt-FastLink",
		"argusmagnus.ServersideQoL":           "ArgusMagnus-ServersideQoL",
		"argusmagnus.ServersideQoL.AutoStore": "ArgusMagnus-ServersideQoL_AutoStore",
		"randyknapp.mods.epicloot":            "RandyKnapp-EpicLoot",
	}
	files := []string{
		"Azumatt.FastLink.cfg",         // the id
		"Azumatt.FastLink_servers.yml", // id + "_" suffix
		"argusmagnus.ServersideQoL.cfg",
		"ArgusMagnus.ServersideQoL.AutoStore.cfg", // the longest id wins
		"ArgusMagnus.ServersideQoL/Advanced.yml",  // a folder named after the id
		"randyknapp.mods.epicloot.cfg",
		"XRayVision_ModeratorList.txt", // mod name, shortened
		"some.author.Dup.cfg",          // ambiguous short name: unknown
		"Old.RemovedMod.cfg",           // no such mod
		"com.jotunn.jotunn.cfg",        // last part = mod name (Jotunn)
	}
	got := Owners(files, mods, ids)
	want := map[string]string{
		"Azumatt.FastLink.cfg":                    "Azumatt-FastLink",
		"Azumatt.FastLink_servers.yml":            "Azumatt-FastLink",
		"argusmagnus.ServersideQoL.cfg":           "ArgusMagnus-ServersideQoL",
		"ArgusMagnus.ServersideQoL.AutoStore.cfg": "ArgusMagnus-ServersideQoL_AutoStore",
		"ArgusMagnus.ServersideQoL/Advanced.yml":  "ArgusMagnus-ServersideQoL",
		"randyknapp.mods.epicloot.cfg":            "RandyKnapp-EpicLoot",
		"XRayVision_ModeratorList.txt":            "Someone-XRayVision",
		"some.author.Dup.cfg":                     "",
		"Old.RemovedMod.cfg":                      "",
		"com.jotunn.jotunn.cfg":                   "ValheimModding-Jotunn",
	}
	for f, w := range want {
		if got[f] != w {
			t.Errorf("%s: got %q, want %q", f, got[f], w)
		}
	}
	if ModName("Azumatt-FastLink") != "FastLink" || ModName("Plain") != "Plain" {
		t.Error("ModName")
	}
}

func fakeThunderstore(t *testing.T) (*httptest.Server, *atomic.Int32) {
	calls := &atomic.Int32{} // lookups run 6 at a time
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		switch r.URL.Path {
		case "/api/experimental/package/Azumatt/AzuClock/":
			fmt.Fprint(w, `{"community_listings":[{"community":"valheim","categories":["Mods","Client-side"]},{"community":"other","categories":["Server-side"]}]}`)
		case "/api/experimental/package/Azumatt/FastLink/":
			fmt.Fprint(w, `{"community_listings":[{"community":"valheim","categories":["Client-side","Server-side"]}]}`)
		case "/api/experimental/package/Argus/QoL/":
			fmt.Fprint(w, `{"community_listings":[{"community":"valheim","categories":["Server-side"]}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, calls
}

var (
	reClient = regexp.MustCompile("^(Client-side|Clientside|Client-only|Client Side)$")
	reServer = regexp.MustCompile("^(Server-side|Serverside|Server-only|Server Side)$")
)

func TestSidesLookedUpOnceAndCached(t *testing.T) {
	srv, calls := fakeThunderstore(t)
	cache := filepath.Join(t.TempDir(), "tags.tsv")
	tg := &Tags{CachePath: cache, BaseURL: srv.URL,
		Known: func() map[string]string { return map[string]string{"Gale-Knows": "Server-side"} }}
	mods := []string{"Azumatt-AzuClock", "Azumatt-FastLink", "Argus-QoL", "Nobody-Missing", "Gale-Knows", "NoDash"}
	sides, fetched := tg.Sides("valheim", reClient, reServer, mods)
	want := map[string]string{"Azumatt-AzuClock": "client", "Azumatt-FastLink": "both", "Argus-QoL": "server",
		"Nobody-Missing": "", "Gale-Knows": "server", "NoDash": ""}
	if !reflect.DeepEqual(sides, want) || fetched != 4 || calls.Load() != 4 {
		t.Errorf("sides %v, fetched %d, calls %d", sides, fetched, calls.Load())
	}
	b, _ := os.ReadFile(cache)
	if !strings.Contains(string(b), "Nobody-Missing\t") || !strings.Contains(string(b), "Azumatt-AzuClock\t") {
		t.Errorf("cache:\n%s", b)
	}
	// Again: everything's known, nothing is fetched.
	if _, fetched := tg.Sides("valheim", reClient, reServer, mods); fetched != 0 || calls.Load() != 4 {
		t.Errorf("second time: fetched %d, calls %d", fetched, calls.Load())
	}
}

func TestNoNetworkLeavesModsUnknownAndUncached(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "tags.tsv")
	tg := &Tags{CachePath: cache, BaseURL: "http://127.0.0.1:1"}
	sides, fetched := tg.Sides("valheim", reClient, reServer, []string{"Azumatt-AzuClock"})
	if sides["Azumatt-AzuClock"] != "" || fetched != 0 {
		t.Errorf("%v %d", sides, fetched)
	}
	if _, err := os.Stat(cache); err == nil {
		t.Error("a failed lookup was cached")
	}
}

// A mod in two sync folders (Gale puts HookGenPatcher in plugins and
// patchers) is still one mod, not two with the same name.
func TestOwnersSameModTwice(t *testing.T) {
	mods := []string{"ValheimModding-HookGenPatcher", "ValheimModding-HookGenPatcher"}
	if got := Owners([]string{"HookGenPatcher.cfg"}, mods, nil)["HookGenPatcher.cfg"]; got != "ValheimModding-HookGenPatcher" {
		t.Errorf("HookGenPatcher.cfg -> %q", got)
	}
}
