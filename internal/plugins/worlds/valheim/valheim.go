// Package valheim is the world format of newer Valheim versions: chunked saves, a folder per world
// with _main.<N>.fwl2 / .db2 / .chunks, a _main.<N>.ok written last (save N
// is complete) and *.chunk files. A save writes the new files and the .ok,
// and only then removes what save N-1 used. The game's own backups
// (<world>_backup_auto-<time>/) are worlds too, offered after the others,
// newest first. (Worked out from a live server's folders; not documented.)
package valheim

import (
	"fmt"
	iofs "io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/doggpound/game-mod-sync/internal/plugins"
	"github.com/doggpound/game-mod-sync/internal/plugins/worlds"
)

func init() {
	plugins.RegisterWorldFormat("valheim", func([]string) (plugins.WorldFormat, error) { return Valheim{}, nil })
}

// Valheim is the format.
type Valheim struct{}

var mainOK = regexp.MustCompile(`^_main\.(\d+)\.ok$`)

func (Valheim) Label() string { return "Valheim (chunked saves)" }

func (Valheim) Paths(name string) []string { return []string{name} }

// lastSave is the newest save number with its .ok, and that file's info.
func lastSave(entries []iofs.FileInfo) (int, iofs.FileInfo) {
	best, info := -1, iofs.FileInfo(nil)
	for _, e := range entries {
		if m := mainOK.FindStringSubmatch(e.Name()); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n > best {
				best, info = n, e
			}
		}
	}
	return best, info
}

func (v Valheim) List(f worlds.FS, dir string) ([]worlds.World, error) {
	entries, err := f.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var found, backups []worlds.World
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		inner, err := f.ReadDir(f.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		n, ok := lastSave(inner)
		if n < 0 {
			continue
		}
		var size int64
		for _, i := range inner {
			if !i.IsDir() {
				size += i.Size()
			}
		}
		w := worlds.World{Name: e.Name(), Detail: fmt.Sprintf("save %d, %s, %s", n, ok.ModTime().Format("2006-01-02 15:04"), worlds.HumanSize(size))}
		if strings.Contains(e.Name(), "_backup") {
			backups = append(backups, w)
		} else {
			found = append(found, w)
		}
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Name < found[j].Name })
	sort.Slice(backups, func(i, j int) bool { return backups[i].Name > backups[j].Name })
	return append(found, backups...), nil
}

func (Valheim) Stamp(f worlds.FS, dir, name string) string {
	entries, err := f.ReadDir(f.Join(dir, name))
	if err != nil {
		return ""
	}
	n, _ := lastSave(entries)
	if n < 0 {
		return ""
	}
	have := map[string]bool{}
	for _, e := range entries {
		have[e.Name()] = true
	}
	for _, ext := range []string{"fwl2", "db2", "chunks"} {
		if !have[fmt.Sprintf("_main.%d.%s", n, ext)] {
			return ""
		}
	}
	return fmt.Sprintf("save %d", n)
}
