package configs

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/doggpound/game-mod-sync/internal/remote"
	"github.com/doggpound/game-mod-sync/internal/transfer"
)

// Item is one config file that's new or differs.
type Item struct {
	Dir    string   // config folder, e.g. BepInEx/config
	Path   string   // inside it, e.g. Azumatt.FastLink.cfg
	State  string   // "new" (not on the server) or "differs"
	Mod    string   // mod folder it belongs to; "" = unknown
	Side   string   // client, server, both, or "" (unknown)
	Detail []string // differs: the settings that differ (short)
	Full   string   // differs: every difference
}

// Scanner compares a profile's config folders with the server's.
type Scanner struct {
	Client     *remote.Client
	Local      string   // profile folder
	Root       string   // game folder on the server
	ConfigDirs []string // e.g. BepInEx/config
	Skip       transfer.Excludes
	SyncDirs   []string                           // where the mods are (to tell which mod a file is from)
	Owners     func(dir string) map[string]string // the loader's plugin id -> mod reader; may be nil
	Tags       *Tags
	Community  string
	TagClient  string // regex for the client-side category
	TagServer  string
	Verbose    bool // also list files of mods that aren't in the profile
	Info       func(string)
	Warn       func(string)
}

// Result is a scan: the items, and what's needed to copy them.
type Result struct {
	Items  []Item
	Hidden int // files from mods that aren't in the profile (left over), not listed
	local  map[string]transfer.Tree
	server map[string]transfer.Tree
}

// Offered is how many items can be copied (client-only mods' files can't).
func (r *Result) Offered() int {
	n := 0
	for _, it := range r.Items {
		if it.Side != "client" {
			n++
		}
	}
	return n
}

// Scan compares every config folder. m is the server's manifest (may be nil).
func (s *Scanner) Scan(m *transfer.Manifest) (*Result, error) {
	r := &Result{local: map[string]transfer.Tree{}, server: map[string]transfer.Tree{}}
	var items []Item
	for _, cdir := range s.ConfigDirs {
		local := filepath.Join(s.Local, filepath.FromSlash(cdir))
		if fi, err := os.Stat(local); err != nil || !fi.IsDir() {
			continue
		}
		s.info("Comparing configs (" + cdir + ") ...")
		lt, err := transfer.LocalTree(local)
		if err != nil {
			return nil, err
		}
		st, err := transfer.ServerTree(s.Client, path.Join(s.Root, cdir), m)
		if err != nil {
			s.warn(fmt.Sprintf("couldn't compare %s: %v", cdir, err))
			continue
		}
		r.local[cdir], r.server[cdir] = lt, st
		var found []Item
		for rel, l := range lt {
			if l.Dir || s.Skip.Match(rel, false) {
				continue
			}
			sv, ok := st[rel]
			switch {
			case !ok || sv.Dir:
				found = append(found, Item{Dir: cdir, Path: rel, State: "new"})
			case sv.Hash != l.Hash:
				found = append(found, Item{Dir: cdir, Path: rel, State: "differs"})
			}
		}
		if len(found) == 0 {
			continue
		}
		// What differs: the server's copies of the differing files.
		ndiff := 0
		for _, it := range found {
			if it.State == "differs" {
				ndiff++
			}
		}
		if ndiff > 0 {
			s.info(fmt.Sprintf("Reading the server's versions of %d %s file(s) ...", ndiff, path.Base(cdir)))
		}
		kept := found[:0]
		for _, it := range found {
			if it.State == "differs" {
				mine, err1 := os.ReadFile(filepath.Join(local, filepath.FromSlash(it.Path)))
				theirs, err2 := s.readServer(path.Join(s.Root, cdir, it.Path))
				if err1 != nil || err2 != nil {
					it.Detail = []string{"couldn't read the server's copy"}
				} else {
					it.Detail = Detail(it.Path, string(mine), theirs)
					if len(it.Detail) == 0 {
						continue // only comments, spacing or line endings differ
					}
					it.Full = Full(it.Path, string(mine), theirs)
				}
			}
			kept = append(kept, it)
		}
		items = append(items, kept...)
	}

	// Which mod each file belongs to, and that mod's side.
	var mods []string
	ids := map[string]string{}
	for _, d := range s.SyncDirs {
		dir := filepath.Join(s.Local, filepath.FromSlash(d))
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			mods = append(mods, e.Name())
		}
		if s.Owners != nil {
			for id, mod := range s.Owners(dir) {
				ids[id] = mod
			}
		}
	}
	var files []string
	for _, it := range items {
		files = append(files, it.Path)
	}
	owners := Owners(files, mods, ids)
	modSet := map[string]bool{}
	for _, mod := range owners {
		if mod != "" {
			modSet[mod] = true
		}
	}
	sides := map[string]string{}
	if s.Tags != nil && len(modSet) > 0 {
		var list []string
		for mod := range modSet {
			list = append(list, mod)
		}
		sort.Strings(list)
		cl, _ := regexp.Compile(s.TagClient)
		sv, _ := regexp.Compile(s.TagServer)
		sides, _ = s.Tags.Sides(s.Community, cl, sv, list)
	}
	for _, it := range items {
		it.Mod = owners[it.Path]
		if it.Mod == "" && !s.Verbose {
			r.Hidden++ // left over from a mod that's no longer in the profile
			continue
		}
		it.Side = sides[it.Mod]
		r.Items = append(r.Items, it)
	}
	// New ones first, then by mod and file.
	sort.SliceStable(r.Items, func(i, j int) bool {
		a, b := r.Items[i], r.Items[j]
		if a.State != b.State {
			return a.State == "new"
		}
		if ma, mb := strings.ToLower(a.Mod), strings.ToLower(b.Mod); ma != mb {
			return ma < mb
		}
		return strings.ToLower(a.Path) < strings.ToLower(b.Path)
	})
	return r, nil
}

func (s *Scanner) readServer(p string) (string, error) {
	f, err := s.Client.SFTP().Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 16<<20))
	return string(b), err
}

func (s *Scanner) info(msg string) {
	if s.Info != nil {
		s.Info(msg)
	}
}

func (s *Scanner) warn(msg string) {
	if s.Warn != nil {
		s.Warn(msg)
	}
}

// Copy copies the picked items (indexes into Items) to the server. Replaced
// files go to backup/<config folder>/... like the mods ("" = no backups);
// nothing is ever deleted.
func (r *Result) Copy(c *remote.Client, local, root, backup string, picked []int, m *transfer.Manifest) error {
	byDir := map[string][]Item{}
	var dirs []string
	for _, i := range picked {
		it := r.Items[i]
		if _, ok := byDir[it.Dir]; !ok {
			dirs = append(dirs, it.Dir)
		}
		byDir[it.Dir] = append(byDir[it.Dir], it)
	}
	for _, cdir := range dirs {
		st := r.server[cdir]
		p := transfer.Plan{New: map[string]bool{}}
		need := map[string]bool{}
		for _, it := range byDir[cdir] {
			p.Send = append(p.Send, it.Path)
			p.Bytes += r.local[cdir][it.Path].Size
			if it.State == "new" {
				p.New[it.Path] = true
			}
			for d := path.Dir(it.Path); d != "." && d != "/"; d = path.Dir(d) {
				if e, ok := st[d]; !ok || !e.Dir {
					need[d] = true
				}
			}
		}
		for d := range need {
			p.MkDirs = append(p.MkDirs, d)
		}
		sort.Slice(p.MkDirs, func(i, j int) bool {
			return strings.Count(p.MkDirs[i], "/") < strings.Count(p.MkDirs[j], "/") ||
				(strings.Count(p.MkDirs[i], "/") == strings.Count(p.MkDirs[j], "/") && p.MkDirs[i] < p.MkDirs[j])
		})
		sort.Strings(p.Send)
		a := &transfer.Apply{
			SFTP:     c.SFTP(),
			Local:    filepath.Join(local, filepath.FromSlash(cdir)),
			Server:   path.Join(root, cdir),
			Hashes:   r.local[cdir],
			Manifest: m,
		}
		if backup != "" {
			a.Backup = path.Join(backup, cdir)
		}
		if err := a.Run(p); err != nil {
			return fmt.Errorf("copying configs (%s): %w", cdir, err)
		}
	}
	return nil
}
