// Package gale reads games and profiles from the Gale mod manager
// (https://github.com/Kesomannen/gale). Nothing in Gale is changed.
//
// Gale keeps one folder per game, each with a profiles/ folder:
//
//	<data dir>/com.kesomannen.gale/<game id>/profiles/<profile>/...
//
// The game id (e.g. valheim, lethal-company) picks the game's settings.
package gale

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/doggpound/game-mod-sync/internal/plugins"
)

func init() { plugins.RegisterSource(Source{}) }

// Source is Gale.
type Source struct{}

func (Source) Name() string                                   { return "gale" }
func (Source) Label() string                                  { return "Gale" }
func (Source) Find() []string                                 { return Candidates() }
func (Source) Profiles(dir string) ([]plugins.Profile, error) { return List(dir) }
func (Source) ModTags(dir, game string) map[string]string     { return ModTags(dir, game) }

const folder = "com.kesomannen.gale"

// Candidates are the Gale data folders that exist on this computer: native
// Windows (%APPDATA%), Windows seen from WSL (every /mnt/*/Users/*, or
// $GMS_WIN_USERS), Linux, Flatpak and macOS.
func Candidates() []string {
	var cands []string
	add := func(p string) {
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			for _, c := range cands {
				if c == p {
					return
				}
			}
			cands = append(cands, p)
		}
	}
	if runtime.GOOS == "windows" {
		if d := os.Getenv("APPDATA"); d != "" {
			add(filepath.Join(d, folder))
		}
		return cands
	}
	users := os.Getenv("GMS_WIN_USERS")
	if users == "" {
		users = "/mnt/*/Users/*"
	}
	matches, _ := filepath.Glob(users)
	for _, u := range matches {
		add(filepath.Join(u, "AppData", "Roaming", folder))
	}
	home, _ := os.UserHomeDir()
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(home, ".local", "share")
	}
	add(filepath.Join(data, folder))
	add(filepath.Join(home, ".var", "app", "com.kesomannen.gale", "data", folder))
	add(filepath.Join(home, "Library", "Application Support", folder))
	return cands
}

// List returns every profile of every game in the data folder root, sorted
// by game then profile. Folders without a profiles/ folder are ignored.
func List(root string) ([]plugins.Profile, error) {
	games, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []plugins.Profile
	for _, g := range games {
		if !g.IsDir() {
			continue
		}
		pdir := filepath.Join(root, g.Name(), "profiles")
		profiles, err := os.ReadDir(pdir)
		if err != nil {
			continue
		}
		for _, p := range profiles {
			if p.IsDir() {
				out = append(out, plugins.Profile{Game: g.Name(), Name: p.Name(), Dir: filepath.Join(pdir, p.Name())})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Game != out[j].Game {
			return out[i].Game < out[j].Game
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// ModTags reads the Thunderstore packages Gale has looked up for a game
// (<root>/<game>/thunderstore_cache.json), as "Author-Name" -> "cat|cat", so
// client/server checks work offline for mods Gale knows.
func ModTags(root, game string) map[string]string {
	out := map[string]string{}
	f, err := os.Open(filepath.Join(root, game, "thunderstore_cache.json"))
	if err != nil {
		return out
	}
	defer f.Close()
	var pkgs []struct {
		FullName   string   `json:"full_name"`
		Categories []string `json:"categories"`
	}
	if json.NewDecoder(f).Decode(&pkgs) != nil {
		return out
	}
	for _, p := range pkgs {
		if p.FullName != "" {
			out[p.FullName] = strings.Join(p.Categories, "|")
		}
	}
	return out
}
