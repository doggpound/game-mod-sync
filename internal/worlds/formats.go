// Package worlds downloads a world from a server into this computer's saves
// for the game, to try mod updates on a copy before syncing them. One way
// only: nothing on the server is changed.
//
// How a game stores a world is a Format (valheim: chunked world folders;
// files: <world>.<ext> files). Formats read through FS, so the same code
// works on the server (over SFTP) and on this computer.
package worlds

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/pkg/sftp"
)

// FS is a place worlds are read from.
type FS interface {
	ReadDir(dir string) ([]iofs.FileInfo, error)
	Stat(p string) (iofs.FileInfo, error)
	Open(p string) (io.ReadCloser, error)
	Join(elem ...string) string
}

// ServerFS reads the server over SFTP.
type ServerFS struct{ C *sftp.Client }

func (s ServerFS) ReadDir(d string) ([]iofs.FileInfo, error) { return s.C.ReadDir(d) }
func (s ServerFS) Stat(p string) (iofs.FileInfo, error)      { return s.C.Stat(p) }
func (s ServerFS) Open(p string) (io.ReadCloser, error)      { return s.C.Open(p) }
func (s ServerFS) Join(e ...string) string                   { return path.Join(e...) }

// LocalFS reads this computer.
type LocalFS struct{}

func (LocalFS) ReadDir(d string) ([]iofs.FileInfo, error) {
	entries, err := os.ReadDir(d)
	if err != nil {
		return nil, err
	}
	var out []iofs.FileInfo
	for _, e := range entries {
		if fi, err := e.Info(); err == nil {
			out = append(out, fi)
		}
	}
	return out, nil
}
func (LocalFS) Stat(p string) (iofs.FileInfo, error) { return os.Stat(p) }
func (LocalFS) Open(p string) (io.ReadCloser, error) { return os.Open(p) }
func (LocalFS) Join(e ...string) string              { return filepath.Join(e...) }

// World is one world on offer.
type World struct {
	Name   string
	Detail string // e.g. "save 1940, 2026-10-22 18:55, 65M"
}

// Format is how a game saves a world.
type Format interface {
	Label() string
	// List returns the complete worlds in a worlds folder, in the order to offer them.
	List(f FS, dir string) ([]World, error)
	// Paths are the names in the worlds folder that make up a world.
	Paths(name string) []string
	// Stamp changes with every save of the world; "" when it has no complete save.
	// It's compared between the server and the downloaded copy, so it doesn't
	// depend on file times.
	Stamp(f FS, dir, name string) string
}

// Get returns a format by name; files are the extensions for the "files" format.
func Get(name string, files []string) (Format, error) {
	switch name {
	case "valheim":
		return Valheim{}, nil
	case "files":
		if len(files) == 0 {
			return nil, fmt.Errorf("world format files needs its extensions (worlds.files in the game file)")
		}
		return Files{Ext: files}, nil
	}
	return nil, fmt.Errorf("unknown world format %q", name)
}

// ---------------------------------------------------------------- valheim --

// Valheim is the newer Valheim versions' chunked saves: a folder per world
// with _main.<N>.fwl2 / .db2 / .chunks, a _main.<N>.ok written last (save N
// is complete) and *.chunk files. A save writes the new files and the .ok,
// and only then removes what save N-1 used. The game's own backups
// (<world>_backup_auto-<time>/) are worlds too, offered after the others,
// newest first. (Worked out from a live server's folders; not documented.)
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

func (v Valheim) List(f FS, dir string) ([]World, error) {
	entries, err := f.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var worlds, backups []World
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
		w := World{Name: e.Name(), Detail: fmt.Sprintf("save %d, %s, %s", n, ok.ModTime().Format("2006-01-02 15:04"), HumanSize(size))}
		if strings.Contains(e.Name(), "_backup") {
			backups = append(backups, w)
		} else {
			worlds = append(worlds, w)
		}
	}
	sort.Slice(worlds, func(i, j int) bool { return worlds[i].Name < worlds[j].Name })
	sort.Slice(backups, func(i, j int) bool { return backups[i].Name > backups[j].Name })
	return append(worlds, backups...), nil
}

func (Valheim) Stamp(f FS, dir, name string) string {
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

// ------------------------------------------------------------------ files --

// Files is a world saved as <world>.<ext> files side by side (e.g. older
// Valheim: MyWorld.fwl + MyWorld.db). The first extension is the file every
// world has.
type Files struct{ Ext []string }

func (Files) Label() string { return "Files (<world>.<ext>)" }

func (x Files) Paths(name string) []string {
	var out []string
	for _, e := range x.Ext {
		out = append(out, name+"."+e)
	}
	return out
}

func (x Files) List(f FS, dir string) ([]World, error) {
	entries, err := f.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []World
	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), "."+x.Ext[0])
		if !ok || e.IsDir() || name == "" {
			continue
		}
		out = append(out, World{Name: name, Detail: fmt.Sprintf("%s, %s", e.ModTime().Format("2006-01-02 15:04"), HumanSize(e.Size()))})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Stamp is the files' contents' hashes (times differ between the server and
// a copy).
func (x Files) Stamp(f FS, dir, name string) string {
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

// HumanSize is a size the way du -h writes it: 512, 4.0K, 65M, 1.5G.
func HumanSize(n int64) string {
	f := float64(n)
	for _, u := range []string{"", "K", "M", "G", "T"} {
		if f < 1024 || u == "T" {
			switch {
			case u == "":
				return fmt.Sprintf("%d", n)
			case f < 10:
				return fmt.Sprintf("%.1f%s", f, u)
			default:
				return fmt.Sprintf("%.0f%s", f, u)
			}
		}
		f /= 1024
	}
	return ""
}
