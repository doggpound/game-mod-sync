// Package files is the world format for games that save a world as <world>.<ext> files side by side (e.g. older
// Valheim: MyWorld.fwl + MyWorld.db). The first extension is the file every
// world has: the game file's worlds.files.
package files

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/doggpound/game-mod-sync/internal/plugin"
	"github.com/doggpound/game-mod-sync/internal/worlds"
)

func init() { plugin.RegisterWorldFormat("files", newFormat) }

func newFormat(ext []string) (plugin.WorldFormat, error) {
	if len(ext) == 0 {
		return nil, fmt.Errorf("world format files needs its extensions (worlds.files in the game file)")
	}
	return Files{Ext: ext}, nil
}

// Files is the format.
type Files struct{ Ext []string }

func (Files) Label() string { return "Files (<world>.<ext>)" }

func (x Files) Paths(name string) []string {
	var out []string
	for _, e := range x.Ext {
		out = append(out, name+"."+e)
	}
	return out
}

func (x Files) List(f worlds.FS, dir string) ([]worlds.World, error) {
	entries, err := f.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []worlds.World
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), "."+x.Ext[0])
		if !ok || e.IsDir() || name == "" {
			continue
		}
		out = append(out, worlds.World{Name: name, Detail: fmt.Sprintf("%s, %s", e.ModTime().Format("2006-01-02 15:04"), worlds.HumanSize(e.Size()))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Stamp is the files' contents' hashes (times differ between the server and
// a copy).
func (x Files) Stamp(f worlds.FS, dir, name string) string {
	var parts []string
	for _, p := range x.Paths(name) {
		r, err := f.Open(f.Join(dir, p))
		if err != nil {
			return ""
		}
		h := sha256.New()
		_, err = io.Copy(h, r)
		r.Close()
		if err != nil {
			return ""
		}
		parts = append(parts, p+":"+hex.EncodeToString(h.Sum(nil))[:16])
	}
	return strings.Join(parts, " ")
}
