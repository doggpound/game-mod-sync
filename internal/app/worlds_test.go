package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doggpound/game-mod-sync/internal/state"
)

// worldWrite writes a chunked Valheim world (save number save) in dir.
func worldWrite(t *testing.T, dir, name string, save int) {
	t.Helper()
	w := filepath.Join(dir, name)
	old, _ := filepath.Glob(filepath.Join(w, "_main.*"))
	for _, f := range old {
		os.Remove(f)
	}
	for _, ext := range []string{"fwl2", "db2", "chunks"} {
		write(t, filepath.Join(w, fmt.Sprintf("_main.%d.%s", save, ext)), fmt.Sprint(ext, save))
	}
	write(t, filepath.Join(w, "00_00__0_1.chunk"), "terrain")
	write(t, filepath.Join(w, fmt.Sprintf("10_1a__1_%d.chunk", save)), fmt.Sprint("base ", save))
	write(t, filepath.Join(w, fmt.Sprintf("_main.%d.ok", save)), "ok")
}

type worldSetup struct {
	*appSetup
	sw, lw string // the server's and this computer's worlds folders
}

// newWorldSetup: the AMP instance runs Midgard and also has Ashlands, saved
// in Saves/worlds_local (from -savedir); this computer's worlds folder is
// where Windows keeps it, seen from WSL.
func newWorldSetup(t *testing.T) *worldSetup {
	s := newAppSetup(t)
	write(t, filepath.Join(s.srv.Root, ".ampdata", "instances", "Valheim01", "GenericModule.kvp"),
		"App.CommandLineArgs=-name \"{{ServerName}}\" -world \"{{WorldName}}\" -savedir \"Saves\" {{crossplay}}\n"+
			"App.AppSettings={\"WorldName\":\"Midgard\",\"saveinterval\":\"1800\"}\n")
	w := &worldSetup{appSetup: s, sw: filepath.Join(s.root, "Saves", "worlds_local")}
	worldWrite(t, w.sw, "Midgard", 1940)
	worldWrite(t, w.sw, "Ashlands", 55)
	win := t.TempDir()
	t.Setenv("GMS_WIN_USERS", filepath.Join(win, "mnt", "*", "Users", "*"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	w.lw = filepath.Join(win, "mnt", "c", "Users", "me", "AppData", "LocalLow", "IronGate", "Valheim", "worlds_local")
	os.MkdirAll(w.lw, 0o755)
	return w
}

func TestDownloadTheWorldTheServerRuns(t *testing.T) {
	w := newWorldSetup(t)
	out, err := w.run(t, "", "-p", "MyProfile", "--download-world", "-y")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, f := range []string{"_main.1940.db2", "_main.1940.ok", "10_1a__1_1940.chunk"} {
		if !exists(filepath.Join(w.lw, "Midgard", f)) {
			t.Errorf("missing %s", f)
		}
	}
	if exists(filepath.Join(w.lw, "Ashlands")) {
		t.Error("the other world was downloaded too")
	}
	if !strings.Contains(out, "Downloaded Midgard (save 1940)") || !strings.Contains(out, "Test with a spare character") {
		t.Errorf("output:\n%s", out)
	}
}

func TestDownloadAnotherWorldAndUnknownOnes(t *testing.T) {
	w := newWorldSetup(t)
	if out, err := w.run(t, "", "-p", "MyProfile", "--world", "Ashlands", "-y"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !exists(filepath.Join(w.lw, "Ashlands", "_main.55.ok")) || exists(filepath.Join(w.lw, "Midgard")) {
		t.Error("--world didn't pick Ashlands")
	}
	if _, err := w.run(t, "", "-p", "MyProfile", "--world", "Gondor", "-y"); err == nil || !strings.Contains(err.Error(), "no world named 'Gondor'") {
		t.Errorf("err = %v", err)
	}
}

func TestDownloadMenuOffersTheRunningWorldFirst(t *testing.T) {
	w := newWorldSetup(t)
	out, err := w.run(t, "1d\n2\ny\nx\n") // main menu: D on the profile, Ashlands, yes, exit
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "1) Midgard") || !strings.Contains(out, "the server runs this one") {
		t.Errorf("menu:\n%s", out)
	}
	if !exists(filepath.Join(w.lw, "Ashlands", "_main.55.ok")) {
		t.Errorf("not downloaded:\n%s", out)
	}
}

func TestDownloadKeepsYourCopyInABackup(t *testing.T) {
	w := newWorldSetup(t)
	worldWrite(t, w.lw, "Midgard", 2)
	worldWrite(t, w.lw, "WildThings", 9)
	out, err := w.run(t, "", "-p", "MyProfile", "--download-world", "-y")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !exists(filepath.Join(w.lw, "Midgard", "_main.1940.ok")) || exists(filepath.Join(w.lw, "Midgard", "_main.2.ok")) {
		t.Error("not replaced cleanly (a mix of both saves?)")
	}
	if m, _ := filepath.Glob(filepath.Join(filepath.Dir(w.lw), "game-mod-sync-backups", "*", "Midgard", "_main.2.ok")); len(m) != 1 {
		t.Errorf("your copy isn't in the backups:\n%s", out)
	}
	if !exists(filepath.Join(w.lw, "WildThings", "_main.9.ok")) {
		t.Error("another world was touched")
	}
}

func TestDownloadChangesNothingOnTheServer(t *testing.T) {
	w := newWorldSetup(t)
	before := tree(t, w.srv.Root)
	if out, err := w.run(t, "", "-p", "MyProfile", "--download-world", "-y"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if after := tree(t, w.srv.Root); after != before {
		t.Errorf("the server changed:\nbefore %s\nafter  %s", before, after)
	}
}

func tree(t *testing.T, root string) string {
	var b strings.Builder
	filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			c, _ := os.ReadFile(p)
			fmt.Fprintf(&b, "%s=%x;", p, c)
		}
		return nil
	})
	return b.String()
}

func TestDownloadCopiesAgainWhenTheServerSavesMeanwhile(t *testing.T) {
	w := newWorldSetup(t)
	afterWorldCopy = func(try int) {
		if try == 1 {
			worldWrite(t, w.sw, "Midgard", 1941) // the server saves during the first copy
		}
	}
	t.Cleanup(func() { afterWorldCopy = nil })
	out, err := w.run(t, "", "-p", "MyProfile", "--download-world", "-y")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "copying again") {
		t.Errorf("output:\n%s", out)
	}
	if !exists(filepath.Join(w.lw, "Midgard", "_main.1941.ok")) || exists(filepath.Join(w.lw, "Midgard", "_main.1940.ok")) {
		t.Error("the copy isn't the newest save")
	}
}

func TestDownloadGivesUpWhenTheServerKeepsSaving(t *testing.T) {
	w := newWorldSetup(t)
	afterWorldCopy = func(try int) { worldWrite(t, w.sw, "Midgard", 2000+try) }
	t.Cleanup(func() { afterWorldCopy = nil })
	_, err := w.run(t, "", "-p", "MyProfile", "--download-world", "-y")
	if err == nil || !strings.Contains(err.Error(), "kept saving") {
		t.Errorf("err = %v", err)
	}
}

func TestDownloadDryRun(t *testing.T) {
	w := newWorldSetup(t)
	out, err := w.run(t, "1\n", "-p", "MyProfile", "--download-world", "-n") // pick Midgard
	if err != nil || !strings.Contains(out, "Dry run - nothing downloaded.") || exists(filepath.Join(w.lw, "Midgard")) {
		t.Errorf("%v\n%s", err, out)
	}
}

func TestDownloadPlainSSHUsesTheGamesUsualSaveFolder(t *testing.T) {
	w := newWorldSetup(t)
	st, _ := state.Load(w.cfg)
	tg, _ := st.Target("gale", "valheim", "MyProfile")
	tg.Backend = "ssh"
	st.SetTarget(tg)
	st.Save()
	worldWrite(t, filepath.Join(w.srv.Root, ".config", "unity3d", "IronGate", "Valheim", "worlds_local"), "Solo", 4)
	out, err := w.run(t, "", "-p", "MyProfile", "--download-world", "-y")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !exists(filepath.Join(w.lw, "Solo", "_main.4.ok")) {
		t.Errorf("output:\n%s", out)
	}
}

func TestDownloadATypedWorldsFolderIsRemembered(t *testing.T) {
	w := newWorldSetup(t)
	st, _ := state.Load(w.cfg)
	tg, _ := st.Target("gale", "valheim", "MyProfile")
	tg.Backend = "ssh"
	st.SetTarget(tg)
	st.Save()
	worldWrite(t, filepath.Join(w.srv.Root, "elsewhere", "worlds"), "Solo", 4)
	out, err := w.run(t, "elsewhere/worlds\ny\n", "-p", "MyProfile", "--download-world")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	st, _ = state.Load(w.cfg)
	if tg, _ := st.Target("gale", "valheim", "MyProfile"); tg.Worlds != filepath.ToSlash(filepath.Join(w.srv.Root, "elsewhere", "worlds")) {
		t.Errorf("not remembered: %q\n%s", tg.Worlds, out)
	}
	os.RemoveAll(filepath.Join(w.lw, "Solo"))
	if out, err := w.run(t, "", "-p", "MyProfile", "--download-world", "-y"); err != nil || !exists(filepath.Join(w.lw, "Solo", "_main.4.ok")) {
		t.Errorf("second time: %v\n%s", err, out)
	}
}

func TestDownloadAsksWhichLocalFolderWhenThereAreSeveral(t *testing.T) {
	w := newWorldSetup(t)
	other := strings.Replace(w.lw, filepath.Join("mnt", "c"), filepath.Join("mnt", "d"), 1)
	os.MkdirAll(other, 0o755)
	out, err := w.run(t, "2\n", "-p", "MyProfile", "--download-world", "-y")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !exists(filepath.Join(other, "Midgard", "_main.1940.ok")) {
		t.Errorf("output:\n%s", out)
	}
	st, _ := state.Load(w.cfg)
	if st.Config.LocalWorlds["valheim"] != other {
		t.Errorf("not remembered: %v", st.Config.LocalWorlds)
	}
}

func TestDownloadAgainOnlyFetchesWhatChanged(t *testing.T) {
	w := newWorldSetup(t)
	if out, err := w.run(t, "", "-p", "MyProfile", "--download-world", "-y"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	cached := filepath.Join(os.Getenv("XDG_CACHE_HOME"), "game-mod-sync", "worlds", "valheim", "Midgard", "00_00__0_1.chunk")
	fi, err := os.Stat(cached)
	if err != nil {
		t.Fatal(err)
	}
	worldWrite(t, w.sw, "Midgard", 1941)
	if out, err := w.run(t, "", "-p", "MyProfile", "--download-world", "-y"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if fi2, _ := os.Stat(cached); !fi2.ModTime().Equal(fi.ModTime()) || !os.SameFile(fi, fi2) {
		t.Error("an unchanged chunk was downloaded again")
	}
	if !exists(filepath.Join(w.lw, "Midgard", "_main.1941.ok")) || exists(filepath.Join(w.lw, "Midgard", "_main.1940.ok")) {
		t.Error("not the newest save")
	}
}
