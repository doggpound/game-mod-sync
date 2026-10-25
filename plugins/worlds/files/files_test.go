package files

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/doggpound/game-mod-sync/internal/worlds"
)

func names(ws []worlds.World) string {
	var n []string
	for _, w := range ws {
		n = append(n, w.Name)
	}
	return strings.Join(n, " ")
}

func TestFiles(t *testing.T) {
	d := t.TempDir()
	f := Files{Ext: []string{"fwl", "db"}}
	for _, n := range []string{"Alpha.fwl", "Alpha.db", "Beta.fwl", "Alpha.fwl.old", "notes.txt"} {
		os.WriteFile(filepath.Join(d, n), []byte(n), 0o644)
	}
	ws, _ := f.List(worlds.LocalFS{}, d)
	if got := names(ws); got != "Alpha Beta" {
		t.Errorf("list: %s", got)
	}
	if got := strings.Join(f.Paths("My World"), ","); got != "My World.fwl,My World.db" {
		t.Errorf("paths: %s", got)
	}
	a := f.Stamp(worlds.LocalFS{}, d, "Alpha")
	old := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	os.Chtimes(filepath.Join(d, "Alpha.db"), old, old)
	if a == "" || f.Stamp(worlds.LocalFS{}, d, "Alpha") != a {
		t.Errorf("stamp changed with the time: %q", a)
	}
	os.WriteFile(filepath.Join(d, "Alpha.db"), []byte("more"), 0o644)
	if f.Stamp(worlds.LocalFS{}, d, "Alpha") == a {
		t.Error("stamp didn't change with the contents")
	}
	if f.Stamp(worlds.LocalFS{}, d, "Beta") != "" {
		t.Error("a world missing a part has a stamp")
	}
}

func TestNeedsItsExtensions(t *testing.T) {
	if _, err := newFormat(nil); err == nil {
		t.Error("no extensions accepted")
	}
}
