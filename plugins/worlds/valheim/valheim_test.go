package valheim

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doggpound/game-mod-sync/internal/worlds"
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

func names(ws []worlds.World) string {
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
	ws, err := Valheim{}.List(worlds.LocalFS{}, d)
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
	if got := (Valheim{}).Stamp(worlds.LocalFS{}, d, "W"); got != "save 12" {
		t.Errorf("got %q", got)
	}
	os.Remove(filepath.Join(w, "_main.12.db2"))
	if got := (Valheim{}).Stamp(worlds.LocalFS{}, d, "W"); got != "" {
		t.Errorf("a missing part: %q", got)
	}
	if got := (Valheim{}).Stamp(worlds.LocalFS{}, d, "Nope"); got != "" {
		t.Errorf("missing world: %q", got)
	}
}
