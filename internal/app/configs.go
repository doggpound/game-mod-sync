package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/term"

	"github.com/doggpound/game-mod-sync/internal/configs"
	"github.com/doggpound/game-mod-sync/internal/games"
	"github.com/doggpound/game-mod-sync/internal/plugin"
	"github.com/doggpound/game-mod-sync/internal/remote"
	"github.com/doggpound/game-mod-sync/internal/transfer"
	"github.com/doggpound/game-mod-sync/internal/ui"
)

const configsHelp = `Config files

The mods' config files from your profile, compared with the server's.
Ticked files are copied to the server when you apply. Nothing is ever
deleted on the server, and a file that gets replaced is kept in the
backups (.mod-sync-backups in the game folder).

STATUS
  new      the server doesn't have this file yet. Ticked, so a mod you
           just added starts with your settings instead of its defaults.
  differs  the server has the file with other settings. Not ticked:
           copying it replaces the server's settings with yours.
           d (text) or opening the row (full screen) shows each setting
           that differs: the server's value, then yours.

SIDE (from the mod's Thunderstore tags)
  server   runs on the server
  both     runs on the server and on players' machines
  client   runs only on players' machines - these files are never
           copied and are listed above the table instead
  ?        no tags known (Edit > Refresh mod tags looks them up again)

Files that differ only in comments, spacing or line endings are not
listed. Files from mods that are no longer in this profile are hidden;
run with -v to see them.`

// testTags replaces the tag lookup in tests that don't set App.Tags.
var testTags *configs.Tags

// tags is the client/server tag lookup for a game.
func (a *App) tags(game string) *configs.Tags {
	if a.Tags != nil {
		return a.Tags
	}
	if testTags != nil {
		return testTags
	}
	t := &configs.Tags{CachePath: configs.DefaultCachePath()}
	// Mod managers that know their mods' tags offline are asked first.
	var known []func() map[string]string
	for _, e := range a.entries {
		if s, ok := plugin.SourceNamed(e.Source); ok {
			if mt, ok := s.(plugin.ModTagger); ok {
				dir := e.DataDir
				known = append(known, func() map[string]string { return mt.ModTags(dir, game) })
			}
		}
	}
	t.Known = func() map[string]string {
		out := map[string]string{}
		for _, k := range known {
			for m, c := range k() {
				if _, ok := out[m]; !ok {
					out[m] = c
				}
			}
		}
		return out
	}
	return t
}

// configExtra offers a profile's config files in a sync (FolderSync.Extra).
type configExtra struct {
	a      *App
	e      Entry
	c      *remote.Client
	root   string
	res    *configs.Result
	rows   []string // checklist rows (items that can be copied)
	idx    []int    // their index in res.Items
	on     []bool
	header string
	client []string // client-only mods
	picked []int
}

func (a *App) configExtra(e Entry, c *remote.Client, root string) *configExtra {
	if len(e.Game.ConfigDirs) == 0 {
		return nil
	}
	return &configExtra{a: a, e: e, c: c, root: root}
}

func (x *configExtra) Scan(m *transfer.Manifest) error {
	g := x.e.Game
	var owners func(string) map[string]string
	if l, ok := plugin.DetectLoader(x.e.Profile.Dir); ok {
		if co, ok := l.(plugin.ConfigOwner); ok {
			owners = co.ConfigOwners
		}
	}
	s := &configs.Scanner{
		Client: x.c, Local: x.e.Profile.Dir, Root: x.root,
		ConfigDirs: g.ConfigDirs, Skip: transfer.Excludes(g.ConfigSkip), SyncDirs: g.SyncDirs,
		Owners: owners, Tags: x.a.tags(g.ID), Community: g.Thunderstore,
		TagClient: g.TagClient, TagServer: g.TagServer, Verbose: x.a.Opts.Verbose,
		Info: x.a.UI.Info, Warn: x.a.UI.Warn,
	}
	res, err := s.Scan(m)
	if err != nil {
		return err
	}
	x.res = res
	x.layout()
	return nil
}

func (x *configExtra) Offered() int { return x.res.Offered() }

// layout builds the aligned rows: status, side, mod, file (long paths keep their end).
func (x *configExtra) layout() {
	cols := 100
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 40 {
		cols = w
	}
	wm := 3
	for _, it := range x.res.Items {
		if it.Side != "client" {
			wm = max(wm, min(26, len(modLabel(it.Mod))))
		}
	}
	fmax := min(70, max(20, cols-wm-36))
	x.header = fmt.Sprintf("%-8s %-7s %-*s  %s", "STATUS", "SIDE", wm, "MOD", "FILE")
	x.rows, x.idx, x.on, x.client = nil, nil, nil, nil
	for i, it := range x.res.Items {
		if it.Side == "client" {
			x.client = append(x.client, configs.ModName(it.Mod))
			continue
		}
		side := it.Side
		if side == "" {
			side = "?"
		}
		name := modLabel(it.Mod)
		if len(name) > 26 {
			name = name[:26]
		}
		x.rows = append(x.rows, fmt.Sprintf("%-8s %-7s %-*s  %s", it.State, side, wm, name, cutStart(it.Path, fmax)))
		x.idx = append(x.idx, i)
		x.on = append(x.on, it.State == "new" && it.Mod != "")
	}
}

func modLabel(mod string) string {
	if mod == "" {
		return "?"
	}
	return configs.ModName(mod)
}

// cutStart keeps a long text's end: "...ServersideQoL.AutoStore.cfg".
func cutStart(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-(n-3):]
}

func (x *configExtra) notes() []string {
	var out []string
	if x.res.Hidden > 0 {
		out = append(out, fmt.Sprintf("Not shown: %d file(s) from mods that aren't in this profile (-v shows them)", x.res.Hidden))
	}
	if len(x.client) > 0 {
		out = append(out, "Not copied (client mods only): "+strings.Join(x.client, ", "))
	}
	return out
}

func (x *configExtra) Preview(yes bool) {
	if len(x.res.Items) == 0 && x.res.Hidden == 0 {
		return
	}
	u := x.a.UI
	head := "  Configs"
	if yes {
		head += " (new ones are copied)"
	}
	u.Say("")
	u.Say(head)
	for _, n := range x.notes() {
		u.Say("   " + n)
	}
	if len(x.rows) > 0 {
		u.Say("     " + x.header)
	}
	for i, r := range x.rows {
		mark := "~"
		if x.on[i] {
			mark = "+"
		}
		u.Say("   " + mark + " " + r)
	}
	var diffs []string
	for _, i := range x.idx {
		it := x.res.Items[i]
		if it.State != "differs" {
			continue
		}
		if x.a.Opts.Verbose && it.Full != "" {
			diffs = append(diffs, "  "+it.Path, it.Full)
			continue
		}
		diffs = append(diffs, "  "+it.Path)
		for _, d := range it.Detail {
			diffs = append(diffs, "      "+d)
		}
	}
	if len(diffs) > 0 {
		if x.a.Opts.Verbose {
			u.Say("   What differs (server -> yours), in full:")
		} else {
			u.Say("   What differs (server -> yours; -v for all of it):")
		}
		for _, d := range diffs {
			u.Say("   " + d)
		}
	}
}

func (x *configExtra) Choose(yes bool) (bool, error) {
	x.picked = nil
	if len(x.rows) == 0 {
		return true, nil
	}
	if yes {
		for i, on := range x.on {
			if on {
				x.picked = append(x.picked, x.idx[i])
			}
		}
		return true, nil
	}
	q := "Ticked files are copied to the server."
	for _, n := range x.notes() {
		q += "\n" + n
	}
	details := make([]string, len(x.rows))
	for i, j := range x.idx {
		details[i] = x.res.Items[j].Full
	}
	on, err := x.a.UI.Check(ui.Checklist{Title: "Config files", Question: q, Header: x.header,
		Rows: x.rows, On: x.on, Details: details, Help: configsHelp})
	if errors.Is(err, ui.ErrBack) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for i, t := range on {
		if t {
			x.picked = append(x.picked, x.idx[i])
		}
	}
	return true, nil
}

func (x *configExtra) Picked() int { return len(x.picked) }

func (x *configExtra) Review() {
	for _, p := range x.picked {
		for i, j := range x.idx {
			if j == p {
				x.a.UI.Say("   " + x.rows[i])
			}
		}
	}
}

func (x *configExtra) Apply(backup string, m *transfer.Manifest) error {
	return x.res.Copy(x.c, x.e.Profile.Dir, x.root, backup, x.picked, m)
}

// RefreshTags looks up every mod of every listed profile on Thunderstore
// again (Edit > Refresh mod tags, --refresh-tags).
func (a *App) RefreshTags(entries []Entry) error {
	byGame := map[string]map[string]bool{}
	gameOf := map[string]*games.Game{}
	for _, e := range entries {
		for _, d := range e.Game.SyncDirs {
			list, err := os.ReadDir(filepath.Join(e.Profile.Dir, filepath.FromSlash(d)))
			if err != nil {
				continue
			}
			for _, m := range list {
				if strings.Contains(m.Name(), "-") {
					if byGame[e.Profile.Game] == nil {
						byGame[e.Profile.Game] = map[string]bool{}
					}
					byGame[e.Profile.Game][m.Name()] = true
				}
			}
		}
		gameOf[e.Profile.Game] = e.Game
	}
	counts := map[string]int{}
	for game, set := range byGame {
		g := gameOf[game]
		var mods []string
		for m := range set {
			mods = append(mods, m)
		}
		sort.Strings(mods)
		a.UI.Info(fmt.Sprintf("Looking up %d mod(s) on Thunderstore (client / server) ...", len(mods)))
		t := a.tags(game)
		if _, err := t.Fetch(g.Thunderstore, mods); err != nil {
			a.UI.Warn("Couldn't reach Thunderstore - mod tags not refreshed.")
			return nil
		}
		cl, _ := regexp.Compile(g.TagClient)
		sv, _ := regexp.Compile(g.TagServer)
		sides, _ := t.Sides(g.Thunderstore, cl, sv, mods)
		for _, s := range sides {
			counts[s]++
		}
	}
	a.UI.OK(fmt.Sprintf("Mod tags refreshed: %d client-only, %d server-only, %d both, %d without a tag.",
		counts["client"], counts["server"], counts["both"], counts[""]))
	return nil
}
