// Package backends finds game folders on a server: every folder that contains
// the game's marker folder (e.g. BepInEx/plugins). With a login that runs
// commands it's one find; on SFTP-only logins the folders are walked over
// SFTP, to a limited depth and number of folders.
package backends

import (
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/doggpound/game-mod-sync/internal/remote"
)

// MaxWalk is how many folders an SFTP search looks at before stopping.
const MaxWalk = 5000

// skipped are folders never searched (backups the tool makes, and the
// usual huge or irrelevant ones).
var skipped = map[string]bool{".mod-sync-backups": true, ".git": true, "node_modules": true, "proc": true, "sys": true, "dev": true}

// FindRoots returns the game folders below start (absolute, sorted) that
// contain marker, searching depth folders deep.
func FindRoots(c *remote.Client, start, marker string, depth int) ([]string, error) {
	if c.CanRun() {
		cmd := "find " + remote.Quote(start) + " -mindepth 1 -maxdepth " + strconv.Itoa(depth+strings.Count(marker, "/")+1) +
			" -type d -path " + remote.Quote("*/"+marker) + " -not -path '*/.mod-sync-backups/*' 2>/dev/null | sort"
		out, err := c.Run(cmd)
		if err == nil || len(out) > 0 { // find exits 1 on unreadable folders; its output still counts
			var roots []string
			for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if l != "" {
					roots = append(roots, strings.TrimSuffix(l, "/"+marker))
				}
			}
			return roots, nil
		}
	}
	return walk(c, start, marker, depth)
}

func walk(c *remote.Client, start, marker string, depth int) ([]string, error) {
	fc := c.SFTP()
	type item struct {
		dir   string
		level int
	}
	queue := []item{{start, 0}}
	seen := 0
	var roots []string
	for len(queue) > 0 && seen < MaxWalk {
		it := queue[0]
		queue = queue[1:]
		seen++
		if fi, err := fc.Stat(path.Join(it.dir, marker)); err == nil && fi.IsDir() {
			roots = append(roots, it.dir)
			continue // a game folder isn't searched further
		}
		if it.level >= depth {
			continue
		}
		entries, err := fc.ReadDir(it.dir)
		if err != nil {
			if it.dir == start {
				return nil, err
			}
			continue // unreadable: skip
		}
		for _, e := range entries {
			if e.IsDir() && !skipped[e.Name()] {
				queue = append(queue, item{path.Join(it.dir, e.Name()), it.level + 1})
			}
		}
	}
	sort.Strings(roots)
	return roots, nil
}

// Home is the folder a login starts in (the SSH user's home, or the SFTP
// root a host gives).
func Home(c *remote.Client) (string, error) {
	return c.SFTP().Getwd()
}

// Resolve turns a typed folder into an absolute one on the server: ~ and
// relative paths are relative to the login's start folder.
func Resolve(c *remote.Client, typed string) (string, error) {
	p := strings.TrimSpace(typed)
	switch {
	case p == "~" || p == "":
		p = "."
	case strings.HasPrefix(p, "~/"):
		p = strings.TrimPrefix(p, "~/")
	}
	return c.SFTP().RealPath(p)
}
