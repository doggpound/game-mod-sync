package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doggpound/game-mod-sync/internal/configs"
	"github.com/doggpound/game-mod-sync/internal/state"
)

// fakeDLL writes a file holding a [BepInPlugin] attribute blob (01 00, then
// id, name and version as length-prefixed strings).
func fakeDLL(t *testing.T, path, id string) {
	t.Helper()
	b := []byte("MZjunk\x01\x00")
	for _, s := range []string{id, "Name", "1.0.0"} {
		b = append(b, byte(len(s)))
		b = append(b, s...)
	}
	write(t, path, string(append(b, 0, 0)))
}

// configCase: a profile whose mods match the server's, with config files that
// are new, differ, differ only in comments, belong to a client-only mod, or
// are left over from a removed mod. Thunderstore is a fake.
func configCase(t *testing.T) (*appSetup, string, string) {
	s := newAppSetup(t)
	lp := filepath.Join(s.pdir, "BepInEx", "plugins")
	sp := filepath.Join(s.root, "BepInEx", "plugins")
	os.RemoveAll(sp)
	os.RemoveAll(filepath.Join(lp, "Azumatt-AzuClock"))
	for _, m := range []struct{ dir, id string }{
		{"Azumatt-AzuClock", "Azumatt.AzuClock"}, {"ArgusMagnus-QoL", "argusmagnus.QoL"}, {"Some-ClientMod", "some.clientmod"},
	} {
		fakeDLL(t, filepath.Join(lp, m.dir, "mod.dll"), m.id)
		fakeDLL(t, filepath.Join(sp, m.dir, "mod.dll"), m.id)
	}
	lc := filepath.Join(s.pdir, "BepInEx", "config")
	sc := filepath.Join(s.root, "BepInEx", "config")
	write(t, filepath.Join(lc, "Azumatt.AzuClock.cfg"), "[General]\nShow = true\n")               // new
	write(t, filepath.Join(lc, "argusmagnus.QoL.cfg"), "[General]\nOn = true\nRange = 20\n")      // differs
	write(t, filepath.Join(sc, "argusmagnus.QoL.cfg"), "[General]\nOn = false\nRange = 20\n")     //
	write(t, filepath.Join(lc, "argusmagnus.QoL", "Advanced.yml"), "Throttle:\n  Max: 100\n")     // differs (not .cfg)
	write(t, filepath.Join(sc, "argusmagnus.QoL", "Advanced.yml"), "Throttle:\n  Max: 50\n")      //
	write(t, filepath.Join(lc, "argusmagnus.QoL.Extra.cfg"), "# new comment\r\n[A]\r\nX = 1\r\n") // formatting only
	write(t, filepath.Join(sc, "argusmagnus.QoL.Extra.cfg"), "[A]\nX =   1\n")                    //
	write(t, filepath.Join(lc, "some.clientmod.cfg"), "[A]\nB = 1\n")                             // client-only mod
	write(t, filepath.Join(lc, "Old.RemovedMod.cfg"), "[A]\nB = 1\n")                             // mod not in the profile
	write(t, filepath.Join(lc, "BepInEx.cfg"), "[Logging]\nX = 1\n")                              // never offered

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cats := map[string]string{
			"/api/experimental/package/Azumatt/AzuClock/": `"Client-side","Server-side"`,
			"/api/experimental/package/ArgusMagnus/QoL/":  `"Server-side"`,
			"/api/experimental/package/Some/ClientMod/":   `"Client-side"`,
		}[r.URL.Path]
		if cats == "" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"community_listings":[{"community":"valheim","categories":[%s]}]}`, cats)
	}))
	t.Cleanup(ts.Close)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	testTags = &configs.Tags{CachePath: filepath.Join(t.TempDir(), "tags.tsv"), BaseURL: ts.URL}
	t.Cleanup(func() { testTags = nil })
	return s, lc, sc
}

func TestConfigsWithYCopyOnlyNewOnes(t *testing.T) {
	s, _, sc := configCase(t)
	out, err := s.run(t, "", "-p", "MyProfile", "-y")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !exists(filepath.Join(sc, "Azumatt.AzuClock.cfg")) {
		t.Errorf("the new config wasn't copied:\n%s", out)
	}
	if read(t, filepath.Join(sc, "argusmagnus.QoL.cfg")) != "[General]\nOn = false\nRange = 20\n" {
		t.Error("a differing config was replaced with -y")
	}
	for _, f := range []string{"some.clientmod.cfg", "Old.RemovedMod.cfg", "BepInEx.cfg"} {
		if exists(filepath.Join(sc, f)) {
			t.Errorf("%s was copied", f)
		}
	}
	for _, want := range []string{
		"Configs (new ones are copied, unused ones removed)",
		"Not copied (client mods only): ClientMod",
		"- unused   client  -         Old.RemovedMod.cfg", // the profile's left-over, removed with -y
		"On: false -> true",
		"2 lines differ", // the .yml, summed up: its line, then yours (-v or d shows them)
		"Mods are in sync.",
		"Config files done: 1 config file(s) to copy, 1 unused to remove on this computer.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "QoL.Extra.cfg") {
		t.Errorf("a comment-only difference was listed:\n%s", out)
	}
}

func TestConfigsTickADifferingOne(t *testing.T) {
	s, lc, sc := configCase(t)
	// rows: 1 new AzuClock.cfg, 2 differs QoL.cfg, 3 differs QoL/Advanced.yml -> tick 2, done, apply
	out, err := s.run(t, "2\n\ny\n", "-p", "MyProfile")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if read(t, filepath.Join(sc, "argusmagnus.QoL.cfg")) != read(t, filepath.Join(lc, "argusmagnus.QoL.cfg")) {
		t.Errorf("the ticked config wasn't copied:\n%s", out)
	}
	if exists(filepath.Join(sc, "argusmagnus.QoL", "Advanced.yml")) && read(t, filepath.Join(sc, "argusmagnus.QoL", "Advanced.yml")) != "Throttle:\n  Max: 50\n" {
		t.Error("an unticked config was replaced")
	}
	bk := filepath.Join(s.root, BackupsDir, "*", "BepInEx", "config", "argusmagnus.QoL.cfg")
	if m, _ := filepath.Glob(bk); len(m) != 1 || read(t, m[0]) != "[General]\nOn = false\nRange = 20\n" {
		t.Errorf("no backup of the replaced config: %v", m)
	}
	for _, want := range []string{"-- Config files --", "STATUS   SIDE", "Config files: 2 config file(s) to copy, 1 unused to remove on this computer"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestConfigsBackChangesNothing(t *testing.T) {
	s, _, sc := configCase(t)
	out, err := s.run(t, "b\n", "-p", "MyProfile")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if exists(filepath.Join(sc, "Azumatt.AzuClock.cfg")) || !strings.Contains(out, "Cancelled - nothing changed.") {
		t.Errorf("output:\n%s", out)
	}
	if strings.Contains(out, "Restart the server") {
		t.Errorf("the after-sync note was shown although nothing changed:\n%s", out)
	}
}

func TestConfigsNonePicked(t *testing.T) {
	s, _, _ := configCase(t)
	out, err := s.run(t, "n\n\n", "-p", "MyProfile") // untick all, done
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "No configs picked - nothing changed.") {
		t.Errorf("output:\n%s", out)
	}
}

func TestConfigsDryRun(t *testing.T) {
	s, _, sc := configCase(t)
	out, err := s.run(t, "", "-p", "MyProfile", "-n")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if exists(filepath.Join(sc, "Azumatt.AzuClock.cfg")) || !strings.Contains(out, "What differs (server -> yours") || !strings.Contains(out, "Dry run") {
		t.Errorf("output:\n%s", out)
	}
}

func TestConfigsInSyncIsAlreadyInSync(t *testing.T) {
	s, lc, sc := configCase(t)
	os.RemoveAll(sc)
	if err := os.CopyFS(sc, os.DirFS(lc)); err != nil {
		t.Fatal(err)
	}
	os.Remove(filepath.Join(lc, "Old.RemovedMod.cfg"))
	// The server's Old.RemovedMod.cfg is unused, but --no-clean leaves it alone.
	out, err := s.run(t, "", "-p", "MyProfile", "--no-clean")
	if err != nil || !strings.Contains(out, "Already in sync") {
		t.Errorf("%v\n%s", err, out)
	}
}

// cleanCase: the server also has configs no mod there uses (one in its own
// folder), one for a mod the server keeps (ValheimPlus), and one for a mod in
// the profile that the profile has no config for yet.
func cleanCase(t *testing.T) (*appSetup, string) {
	s, _, sc := configCase(t)
	write(t, filepath.Join(sc, "Gone.OtherMod.cfg"), "[A]\nB = 1\n")
	write(t, filepath.Join(sc, "Gone.Mod", "data.yml"), "x: 1\n")
	write(t, filepath.Join(sc, "Gone.Kept.cfg"), "[A]\nB = 1\n") // the mod manager kept it too
	write(t, filepath.Join(s.pdir, "BepInEx", "config", "Gone.Kept.cfg"), "[A]\nB = 1\n")
	write(t, filepath.Join(sc, "valheim_plus.cfg"), "[A]\nB = 1\n")
	write(t, filepath.Join(sc, "Azumatt.AzuClock.Server.cfg"), "[A]\nB = 1\n")
	write(t, filepath.Join(s.root, "BepInEx", "plugins", "ValheimPlus.dll"), "dll")
	s.settings(t, func(st *state.State) {
		tg, _ := st.Target("gale", "valheim", "MyProfile")
		tg.Kept = []string{"+ValheimPlus.dll"} // the server installs it: left alone
		st.SetTarget(tg)
	})
	return s, sc
}

func TestConfigsCleanRemovesUnused(t *testing.T) {
	s, sc := cleanCase(t)
	lc := filepath.Join(s.pdir, "BepInEx", "config")
	write(t, filepath.Join(lc, "valheim_plus.cfg"), "[A]\nB = 2\n") // your settings for the server's ValheimPlus
	out, err := s.run(t, "", "-p", "MyProfile", "-y")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, f := range []string{"Gone.OtherMod.cfg", "Gone.Kept.cfg"} {
		if exists(filepath.Join(sc, filepath.FromSlash(f))) {
			t.Errorf("unused %s is still on the server:\n%s", f, out)
		}
	}
	// This computer: the profile's unused ones are moved to the backups.
	for _, f := range []string{"Gone.Kept.cfg", "Old.RemovedMod.cfg"} {
		if exists(filepath.Join(lc, f)) {
			t.Errorf("unused %s is still in the profile:\n%s", f, out)
		}
		if m, _ := filepath.Glob(filepath.Join(s.cfg, "backups", "valheim", "MyProfile", "*", "BepInEx", "config", f)); len(m) != 1 {
			t.Errorf("no backup of %s on this computer", f)
		}
	}
	if !exists(filepath.Join(lc, "valheim_plus.cfg")) || !exists(filepath.Join(lc, "argusmagnus.QoL.cfg")) {
		t.Errorf("a used config was removed from the profile:\n%s", out)
	}
	// Not a .cfg: maybe read by a mod under another name, so only listed.
	for _, f := range []string{"valheim_plus.cfg", "Azumatt.AzuClock.Server.cfg", "argusmagnus.QoL.cfg", "Gone.Mod/data.yml"} {
		if !exists(filepath.Join(sc, f)) {
			t.Errorf("%s was removed, but a mod on the server uses it:\n%s", f, out)
		}
	}
	bk := filepath.Join(s.root, BackupsDir, "*", "BepInEx", "config", "Gone.OtherMod.cfg")
	if m, _ := filepath.Glob(bk); len(m) != 1 {
		t.Errorf("no backup of the removed config: %v", m)
	}
	flat := strings.Join(strings.Fields(out), " ") // columns' widths aside
	for _, want := range []string{"- unused server - Gone.Kept.cfg", "- unused client - Gone.Kept.cfg",
		"? unused? server - Gone.Mod/data.yml", "1 config file(s) to copy, 4 unused to remove (2 on the server, 2 on this computer)"} {
		if !strings.Contains(flat, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestConfigsCleanTickAMaybeUnused(t *testing.T) {
	s, sc := cleanCase(t)
	// rows: 1 new, 2-3 differs, 4-7 unused (server, client), 8 unused? Gone.Mod/data.yml -> tick 8, done, apply
	out, err := s.run(t, "8\n\ny\n", "-p", "MyProfile")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if exists(filepath.Join(sc, "Gone.Mod")) {
		t.Errorf("the ticked file (and its emptied folder) is still on the server:\n%s", out)
	}
}

func TestConfigsNoCleanKeepsUnused(t *testing.T) {
	s, sc := cleanCase(t)
	s.settings(t, func(st *state.State) { st.Config.Clean = new(bool) }) // clean = false
	out, err := s.run(t, "", "-p", "MyProfile", "-y")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !exists(filepath.Join(sc, "Gone.OtherMod.cfg")) || strings.Contains(out, "unused") {
		t.Errorf("clean = false, but unused configs were offered or removed:\n%s", out)
	}
}
