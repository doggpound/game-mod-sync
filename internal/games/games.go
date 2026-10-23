// Package games holds each game's settings: what to sync, what to leave
// alone, how to recognise it on a server. Built-in files (builtin/*.toml) are
// read in order - _default.toml, then <game id>.toml - after the mod loader's
// defaults, and a file with the same name in the user's games folder changes
// them again (only the settings it lists).
package games

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/doggpound/game-mod-sync/internal/loaders"
)

//go:embed builtin/*.toml
var builtin embed.FS

// Game is one game's settings.
type Game struct {
	ID     string `toml:"-"`
	Loader string `toml:"-"` // mod loader used (detected from the profile)

	Name         string              `toml:"name"`
	SyncDirs     []string            `toml:"sync_dirs"`
	Excludes     []string            `toml:"excludes"`
	ConfigDirs   []string            `toml:"config_dirs"`
	ConfigSkip   []string            `toml:"config_skip"`
	GameMarker   string              `toml:"game_marker"`
	ServerMatch  string              `toml:"server_match"`
	Managed      map[string][]string `toml:"managed"` // server type -> "setting=pattern[,pattern]"
	AfterSync    string              `toml:"after_sync"`
	Thunderstore string              `toml:"thunderstore_community"`
	TagClient    string              `toml:"tag_client"`
	TagServer    string              `toml:"tag_server"`
	Worlds       Worlds              `toml:"worlds"`
}

// Worlds are the world download's settings (see the bash DESIGN.md).
type Worlds struct {
	Format       string            `toml:"format"` // world format; empty = no world download
	Files        []string          `toml:"files"`  // for format "files": the extensions
	ServerSaves  string            `toml:"server_saves"`
	ServerWorlds string            `toml:"server_worlds"`
	LocalWindows string            `toml:"local_windows"`
	LocalHome    string            `toml:"local_home"`
	Note         string            `toml:"note"`
	Saves        map[string]string `toml:"saves"` // server type -> how it finds the save folder
}

// Load builds the settings for a game. profileDir (may be "") picks the mod
// loader; userDir is the user's games folder (may be "").
func Load(id, profileDir, userDir string) (*Game, error) {
	g := &Game{ID: id}
	l := loaders.Detect(profileDir)
	g.Loader = l.Name
	g.SyncDirs, g.Excludes, g.ConfigDirs, g.ConfigSkip = l.SyncDirs, l.Excludes, l.ConfigDirs, l.ConfigSkip
	for _, name := range []string{"_default", id} {
		if b, err := builtin.ReadFile("builtin/" + name + ".toml"); err == nil {
			if err := decode(g, b, name+".toml"); err != nil {
				return nil, err
			}
		}
		if userDir != "" {
			p := filepath.Join(userDir, name+".toml")
			b, err := os.ReadFile(p)
			if err == nil {
				if err := decode(g, b, p); err != nil {
					return nil, err
				}
			} else if !errors.Is(err, fs.ErrNotExist) {
				return nil, err
			}
		}
	}
	if g.Name == "" {
		g.Name = titleCase(id)
	}
	if g.ServerMatch == "" {
		g.ServerMatch = strings.ReplaceAll(regexp.QuoteMeta(id), "-", "[-_ ]?")
	}
	if g.Thunderstore == "" {
		g.Thunderstore = id
	}
	if g.GameMarker == "" && len(g.SyncDirs) > 0 {
		g.GameMarker = g.SyncDirs[0]
	}
	return g, nil
}

func decode(g *Game, b []byte, name string) error {
	if _, err := toml.Decode(string(b), g); err != nil {
		return fmt.Errorf("game file %s: %w", name, err)
	}
	return nil
}

func titleCase(id string) string {
	parts := strings.Split(id, "-")
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// HasSyncDirs reports whether the profile has at least one of the game's
// sync folders (profiles without any are hidden).
func (g *Game) HasSyncDirs(profileDir string) bool {
	for _, d := range g.SyncDirs {
		if fi, err := os.Stat(filepath.Join(profileDir, filepath.FromSlash(d))); err == nil && fi.IsDir() {
			return true
		}
	}
	return false
}

// MatchesServer reports whether a server folder looks like this game.
func (g *Game) MatchesServer(path string) bool {
	re, err := regexp.Compile("(?i)" + g.ServerMatch)
	return err == nil && re.MatchString(path)
}
