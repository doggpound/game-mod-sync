package worlds

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// valheimWorld writes a chunked Valheim world with save number save.
func valheimWorld(t *testing.T, dir, name string, save int) string {
	t.Helper()
	w := filepath.Join(dir, name)
	os.MkdirAll(w, 0o755)
	old, _ := filepath.Glob(filepath.Join(w, "_main.*"))
	for _, f := range old {
		os.Remove(f)
	}
	for ext, body := range map[string]string{"fwl2": "fwl", "db2": "db", "chunks": "index"} {
		os.WriteFile(filepath.Join(w, fmt.Sprintf("_main.%d.%s", save, ext)), []byte(fmt.Sprintf("%s %d", body, save)), 0o644)
	}
	os.WriteFile(filepath.Join(w, "00_00__0_1.chunk"), []byte("terrain"), 0o644)
	os.WriteFile(filepath.Join(w, fmt.Sprintf("10_1a__1_%d.chunk", save)), []byte(fmt.Sprint("base ", save)), 0o644)
	os.WriteFile(filepath.Join(w, fmt.Sprintf("_main.%d.ok", save)), []byte("ok"), 0o644)
	return w
}

func names(ws []World) string {
	var n []string
	for _, w := range ws {
		n = append(n, w.Name)
	}
	return strings.Join(n, " ")
}

func TestValheimListsCompleteWorldsWithBackupsLast(t *testing.T) {
	d := t.TempDir()
	valheimWorld(t, d, "Midgard", 1940)
	valheimWorld(t, d, "Ashlands", 9)
	valheimWorld(t, d, "Ashlands_backup_auto-20261022-200802", 8)
	valheimWorld(t, d, "Ashlands_backup_auto-20261022-220802", 9)
	os.MkdirAll(filepath.Join(d, "Half"), 0o755)
	os.WriteFile(filepath.Join(d, "Half", "_main.1.fwl2"), nil, 0o644) // never finished
	os.WriteFile(filepath.Join(d, "OldStyle.fwl"), nil, 0o644)         // converted by the game on load
	ws, err := Valheim{}.List(LocalFS{}, d)
	if err != nil {
		t.Fatal(err)
	}
	if got := names(ws); got != "Ashlands Midgard Ashlands_backup_auto-20261022-220802 Ashlands_backup_auto-20261022-200802" {
		t.Errorf("got %s", got)
	}
	if !strings.HasPrefix(ws[1].Detail, "save 1940, ") {
		t.Errorf("detail %q", ws[1].Detail)
	}
}

func TestValheimStamp(t *testing.T) {
	d := t.TempDir()
	w := valheimWorld(t, d, "W", 12)
	os.WriteFile(filepath.Join(w, "_main.13.fwl2"), nil, 0o644) // save 13 in progress
	if got := (Valheim{}).Stamp(LocalFS{}, d, "W"); got != "save 12" {
		t.Errorf("got %q", got)
	}
	os.Remove(filepath.Join(w, "_main.12.db2"))
	if got := (Valheim{}).Stamp(LocalFS{}, d, "W"); got != "" {
		t.Errorf("a missing part: %q", got)
	}
	if got := (Valheim{}).Stamp(LocalFS{}, d, "Nope"); got != "" {
		t.Errorf("missing world: %q", got)
	}
}

func TestFiles(t *testing.T) {
	d := t.TempDir()
	f := Files{Ext: []string{"fwl", "db"}}
	for _, n := range []string{"Alpha.fwl", "Alpha.db", "Beta.fwl", "Alpha.fwl.old", "notes.txt"} {
		os.WriteFile(filepath.Join(d, n), []byte(n), 0o644)
	}
	ws, _ := f.List(LocalFS{}, d)
	if got := names(ws); got != "Alpha Beta" {
		t.Errorf("list: %s", got)
	}
	if got := strings.Join(f.Paths("My World"), ","); got != "My World.fwl,My World.db" {
		t.Errorf("paths: %s", got)
	}
	a := f.Stamp(LocalFS{}, d, "Alpha")
	old := time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)
	os.Chtimes(filepath.Join(d, "Alpha.db"), old, old)
	if a == "" || f.Stamp(LocalFS{}, d, "Alpha") != a {
		t.Errorf("stamp changed with the time: %q", a)
	}
	os.WriteFile(filepath.Join(d, "Alpha.db"), []byte("more"), 0o644)
	if f.Stamp(LocalFS{}, d, "Alpha") == a {
		t.Error("stamp didn't change with the contents")
	}
	if f.Stamp(LocalFS{}, d, "Beta") != "" {
		t.Error("a world missing a part has a stamp")
	}
}

func TestHumanSize(t *testing.T) {
	for n, want := range map[int64]string{512: "512", 4096: "4.0K", 65 << 20: "65M", 3 << 29: "1.5G"} {
		if got := HumanSize(n); got != want {
			t.Errorf("%d: %s, want %s", n, got, want)
		}
	}
}

func TestGet(t *testing.T) {
	if _, err := Get("valheim", nil); err != nil {
		t.Error(err)
	}
	if _, err := Get("files", nil); err == nil {
		t.Error("files without extensions accepted")
	}
	if _, err := Get("nope", nil); err == nil {
		t.Error("unknown format accepted")
	}
}
