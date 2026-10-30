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

// Item is one config file that's new, differs, or is unused.
type Item struct {
	Dir    string   // config folder, e.g. BepInEx/config
	Path   string   // inside it, e.g. Azumatt.FastLink.cfg
	State  string   // "new" (not on the server), "differs", or "unused" (no mod uses it)
	Where  string   // unused: "server" or "client" (this computer's profile)
	Sure   bool     // unused: a .cfg (named after its mod's plugin id), so surely unused; other files may belong to a mod under any name
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
	// Clean also finds the config files that no mod uses, on the server and
	// in the profile (mod managers keep a removed mod's configs): from none
	// of the profile's mods, nor of the server's mods that the sync leaves
	// alone (Keep; the profile may hold their settings to copy over).
	Clean bool
	Keep  transfer.Excludes // the sync's excludes: what it leaves alone in SyncDirs
	Info  func(string)
	Warn  func(string)
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
	var items, unused []Item
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
		if s.Clean {
			for where, tree := range map[string]transfer.Tree{"server": st, "client": lt} {
				for rel, e := range tree {
					if !e.Dir && !s.Skip.Match(rel, false) {
						unused = append(unused, Item{Dir: cdir, Path: rel, State: "unused", Where: where,
							Sure: isCfg(rel) && !strings.Contains(rel, "/")})
					}
				}
			}
		}
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
			mods = append(mods, modName(e.Name(), e.IsDir()))
		}
		if s.Owners != nil {
			for id, mod := range s.Owners(dir) {
				ids[id] = mod
			}
		}
	}
	if len(unused) > 0 {
		// The server's mods that the sync leaves alone keep their configs.
		for _, d := range s.SyncDirs {
			entries, err := s.Client.SFTP().ReadDir(path.Join(s.Root, d))
			if err != nil {
				continue
			}
			for _, e := range entries {
				if s.Keep.Match(e.Name(), e.IsDir()) {
					mods = append(mods, modName(e.Name(), e.IsDir()))
				}
			}
		}
	}
	var files []string
	for _, it := range append(items, unused...) {
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
		if it.Mod == "" && s.Clean {
			continue // listed as unused (in the profile)
		}
		if it.Mod == "" && !s.Verbose {
			r.Hidden++ // left over from a mod that's no longer in the profile
			continue
		}
		it.Side = sides[it.Mod]
		r.Items = append(r.Items, it)
	}
	for _, it := range unused {
		if owners[it.Path] == "" {
			r.Items = append(r.Items, it)
		}
	}
	// A file that's unused on the server isn't offered for copying too.
	gone := map[string]bool{}
	for _, it := range r.Items {
		if it.State == "unused" {
			gone[it.Dir+"/"+it.Path] = true
		}
	}
	kept := r.Items[:0]
	for _, it := range r.Items {
		if it.State == "unused" || !gone[it.Dir+"/"+it.Path] {
			kept = append(kept, it)
		}
	}
	r.Items = kept
	// New ones first, then differing, then unused (sure ones first, the
	// server's, then this computer's); by mod and file.
	order := map[string]int{"new": 0, "differs": 1, "unused": 2}
	sort.SliceStable(r.Items, func(i, j int) bool {
		a, b := r.Items[i], r.Items[j]
		if a.State != b.State {
			return order[a.State] < order[b.State]
		}
		if a.Sure != b.Sure {
			return a.Sure
		}
		if a.Where != b.Where {
			return a.Where == "server"
		}
		if ma, mb := strings.ToLower(a.Mod), strings.ToLower(b.Mod); ma != mb {
			return ma < mb
		}
		return strings.ToLower(a.Path) < strings.ToLower(b.Path)
	})
	return r, nil
}

// modName is a mod's name from its entry in a mod folder: a folder's name, or
// a file's without its extension (ValheimPlus.dll -> ValheimPlus).
func modName(name string, dir bool) string {
	if dir {
		return name
	}
	return strings.TrimSuffix(name, path.Ext(name))
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

// Copy copies the picked items (indexes into Items) to the server, and
// removes the picked unused ones (and folders left empty by that). Replaced
// and removed files go to backup/<config folder>/... like the mods ("" = no
// backups).
func (r *Result) Copy(c *remote.Client, local, root, backup string, picked []int, m *transfer.Manifest) error {
	byDir := map[string][]Item{}
	var dirs []string
	for _, i := range picked {
		it := r.Items[i]
		if it.Where == "client" {
			continue // RemoveLocal
		}
		if _, ok := byDir[it.Dir]; !ok {
			dirs = append(dirs, it.Dir)
		}
		byDir[it.Dir] = append(byDir[it.Dir], it)
	}
	for _, cdir := range dirs {
		st := r.server[cdir]
		p := transfer.Plan{New: map[string]bool{}}
		need := map[string]bool{}
		gone := map[string]bool{}
		for _, it := range byDir[cdir] {
			if it.State == "unused" {
				p.Delete = append(p.Delete, it.Path)
				gone[it.Path] = true
				continue
			}
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
		sort.Strings(p.Delete)
		p.RmDirs = emptied(st, r.local[cdir], gone)
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

// emptied lists the server's folders (not in the profile) that hold nothing
// but removed files, deepest first.
func emptied(server, local transfer.Tree, gone map[string]bool) []string {
	if len(gone) == 0 {
		return nil
	}
	var out []string
	for d, e := range server {
		if _, mine := local[d]; !e.Dir || mine {
			continue
		}
		empty, prefix := true, d+"/"
		for p, f := range server {
			if strings.HasPrefix(p, prefix) && !f.Dir && !gone[p] {
				empty = false
				break
			}
		}
		if empty {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if a, b := strings.Count(out[i], "/"), strings.Count(out[j], "/"); a != b {
			return a > b
		}
		return out[i] < out[j]
	})
	return out
}

// RemoveLocal moves the picked unused files in the profile (local) to
// backup/<config folder>/..., and removes folders left empty by that.
func (r *Result) RemoveLocal(local, backup string, picked []int) (int, error) {
	n := 0
	dirs := map[string]bool{}
	for _, i := range picked {
		it := r.Items[i]
		if it.State != "unused" || it.Where != "client" {
			continue
		}
		src := filepath.Join(local, filepath.FromSlash(it.Dir), filepath.FromSlash(it.Path))
		dst := filepath.Join(backup, filepath.FromSlash(it.Dir), filepath.FromSlash(it.Path))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return n, err
		}
		if err := moveFile(src, dst); err != nil {
			return n, fmt.Errorf("moving %s to the backups: %w", src, err)
		}
		n++
		for d := path.Dir(it.Path); d != "." && d != "/"; d = path.Dir(d) {
			dirs[filepath.Join(local, filepath.FromSlash(it.Dir), filepath.FromSlash(d))] = true
		}
	}
	var list []string
	for d := range dirs {
		list = append(list, d)
	}
	sort.Slice(list, func(i, j int) bool { return len(list[i]) > len(list[j]) }) // deepest first
	for _, d := range list {
		_ = os.Remove(d) // only when empty
	}
	return n, nil
}

// moveFile renames, or copies and removes when the backup is on another disk.
func moveFile(src, dst string) error {
	if os.Rename(src, dst) == nil {
		return nil
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		return err
	}
	return os.Remove(src)
}
