package transfer

import (
	"sort"
	"strings"
)

// Plan is what it takes to make the server's folder match the local one.
type Plan struct {
	Delete []string        // server files to remove (each kept in the backups first)
	RmDirs []string        // server folders to remove, deepest first
	MkDirs []string        // folders to create, parents first
	Send   []string        // files to upload, new or changed
	New    map[string]bool // which files in Send don't exist on the server yet
	Bytes  int64           // size of everything in Send
}

// Empty reports whether the folders already match.
func (p Plan) Empty() bool {
	return len(p.Delete)+len(p.RmDirs)+len(p.MkDirs)+len(p.Send) == 0
}

// Compare plans the changes. Excluded paths are left alone on both sides: not
// sent, and not deleted (nor the folders that contain them).
func Compare(local, server Tree, ex Excludes) Plan {
	p := Plan{New: map[string]bool{}}
	keep := map[string]bool{} // server folders that must stay: they hold excluded files
	for rel, e := range server {
		if ex.Match(rel, e.Dir) {
			for d := parent(rel); d != ""; d = parent(d) {
				keep[d] = true
			}
		}
	}
	for rel, s := range server {
		if ex.Match(rel, s.Dir) {
			continue
		}
		l, ok := local[rel]
		if ok && l.Dir == s.Dir {
			continue
		}
		if s.Dir {
			if !keep[rel] {
				p.RmDirs = append(p.RmDirs, rel)
			}
		} else {
			p.Delete = append(p.Delete, rel)
		}
	}
	for rel, l := range local {
		if ex.Match(rel, l.Dir) {
			continue
		}
		s, ok := server[rel]
		same := ok && s.Dir == l.Dir
		if l.Dir {
			if !same {
				p.MkDirs = append(p.MkDirs, rel)
			}
			continue
		}
		if same && s.Hash == l.Hash {
			continue
		}
		p.Send = append(p.Send, rel)
		p.Bytes += l.Size
		if !same {
			p.New[rel] = true
		}
	}
	sort.Strings(p.Delete)
	sort.Strings(p.Send)
	sort.Slice(p.RmDirs, func(i, j int) bool { // deepest first
		a, b := depth(p.RmDirs[i]), depth(p.RmDirs[j])
		if a != b {
			return a > b
		}
		return p.RmDirs[i] < p.RmDirs[j]
	})
	sort.Slice(p.MkDirs, func(i, j int) bool { // parents first
		a, b := depth(p.MkDirs[i]), depth(p.MkDirs[j])
		if a != b {
			return a < b
		}
		return p.MkDirs[i] < p.MkDirs[j]
	})
	return p
}

func parent(rel string) string {
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[:i]
	}
	return ""
}

func depth(rel string) int { return strings.Count(rel, "/") }

// Change is one line of the per-mod summary.
type Change struct {
	Kind string // "new", "updated" or "removed"
	Mod  string // top-level file or folder in the synced folder
}

// Summary groups the plan by mod (the first part of each path), the way the
// preview has always done: a mod whose folder appears is new, one whose
// folder goes is removed, anything else that changes is updated. Sorted new,
// updated, removed, then by name.
func (p Plan) Summary() []Change {
	top := func(rel string) string { return strings.SplitN(rel, "/", 2)[0] }
	seen, added, gone := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, d := range p.MkDirs {
		if !strings.Contains(d, "/") {
			added[d] = true
		}
	}
	for _, f := range p.Send {
		m := top(f)
		seen[m] = true
		if f == m && p.New[f] {
			added[m] = true
		}
	}
	for _, f := range p.Delete {
		m := top(f)
		seen[m] = true
		if f == m {
			gone[m] = true
		}
	}
	for _, d := range p.RmDirs {
		m := top(d)
		seen[m] = true
		if d == m {
			gone[m] = true
		}
	}
	rank := map[string]int{"new": 0, "updated": 1, "removed": 2}
	var out []Change
	for m := range seen {
		k := "updated"
		switch {
		case gone[m] && !added[m]:
			k = "removed"
		case added[m]:
			k = "new"
		}
		out = append(out, Change{Kind: k, Mod: m})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return rank[out[i].Kind] < rank[out[j].Kind]
		}
		return out[i].Mod < out[j].Mod
	})
	return out
}
