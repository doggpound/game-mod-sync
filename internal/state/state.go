// Package state is what game-mod-sync remembers between runs: settings
// (config.toml) and where each profile syncs to (targets.toml), in the config
// folder. The first time, the settings of the 0.x versions (shell scripts:
// config and targets.tsv, in the same folder) are read and converted; those
// files are left in place.
package state

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

// Dir is the config folder: $GMS_CONFIG_DIR, else %APPDATA%\game-mod-sync on
// Windows, else $XDG_CONFIG_HOME or ~/.config/game-mod-sync (as 0.x did,
// also on macOS).
func Dir() string {
	if d := os.Getenv("GMS_CONFIG_DIR"); d != "" {
		return d
	}
	if runtime.GOOS == "windows" {
		if d := os.Getenv("APPDATA"); d != "" {
			return filepath.Join(d, "game-mod-sync")
		}
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "game-mod-sync")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "game-mod-sync")
}

// Config is the settings file.
type Config struct {
	SourceDirs      map[string]string `toml:"source_dirs,omitempty"`   // mod manager -> its data folder; empty = found automatically
	GaleDataDir     string            `toml:"gale_data_dir,omitempty"` // in older settings: moved to SourceDirs["gale"]
	LastBackend     string            `toml:"last_backend,omitempty"`  // offered first at setup
	LastHost        string            `toml:"last_host,omitempty"`
	LastUser        string            `toml:"last_user,omitempty"`
	LastPort        int               `toml:"last_port,omitempty"`
	AMPInstancesDir string            `toml:"amp_instances_dir,omitempty"` // relative to the SSH user's home
	ExtraExcludes   []string          `toml:"extra_excludes,omitempty"`    // for every game
	KeepBackups     int               `toml:"keep_backups"`                // server-side backups to keep, 0 = off
	DefaultUI       string            `toml:"default_ui,omitempty"`        // cli or tui
	LocalWorlds     map[string]string `toml:"local_worlds,omitempty"`      // game id -> local worlds folder
	UpdateCheck     *bool             `toml:"update_check,omitempty"`      // say when a newer release exists (default yes)
}

// DefaultConfig is used when there's no settings file.
func DefaultConfig() Config {
	return Config{AMPInstancesDir: ".ampdata/instances", KeepBackups: 5}
}

// Target is where one profile syncs to.
type Target struct {
	Source  string   `toml:"source"`  // e.g. gale
	Game    string   `toml:"game"`    // game id, e.g. valheim
	Profile string   `toml:"profile"` // profile name
	Backend string   `toml:"backend"` // server type, e.g. amp
	Host    string   `toml:"host"`
	Port    int      `toml:"port,omitempty"` // 0 = 22
	User    string   `toml:"user"`
	Root    string   `toml:"root"`             // game folder on the server
	Kept    []string `toml:"kept,omitempty"`   // server-managed files: "+pattern" leave alone, "-pattern" sync over
	Worlds  string   `toml:"worlds,omitempty"` // server worlds folder, when it had to be typed in
}

// Same reports whether t is for this profile.
func (t Target) Same(source, game, profile string) bool {
	return t.Source == source && t.Game == game && t.Profile == profile
}

// State is everything remembered, loaded from Dir.
type State struct {
	Dir      string
	Config   Config
	Targets  []Target
	Migrated bool // read from the 0.x versions' files this time
}

type targetsFile struct {
	Target []Target `toml:"target"`
}

// Load reads the settings and targets in dir (0.x files when there are no
// TOML ones yet; defaults when there's nothing).
func Load(dir string) (*State, error) {
	s := &State{Dir: dir, Config: DefaultConfig()}
	cfgFile, tgtFile := filepath.Join(dir, "config.toml"), filepath.Join(dir, "targets.toml")
	_, cerr := os.Stat(cfgFile)
	_, terr := os.Stat(tgtFile)
	if errors.Is(cerr, os.ErrNotExist) && errors.Is(terr, os.ErrNotExist) {
		return s, s.migrate()
	}
	if cerr == nil {
		if _, err := toml.DecodeFile(cfgFile, &s.Config); err != nil {
			return nil, fmt.Errorf("%s: %w", cfgFile, err)
		}
	}
	s.Config.moveGaleDir()
	if terr == nil {
		var tf targetsFile
		if _, err := toml.DecodeFile(tgtFile, &tf); err != nil {
			return nil, fmt.Errorf("%s: %w", tgtFile, err)
		}
		s.Targets = tf.Target
	}
	// Profiles set up with a 0.x version since then (not saved here
	// unless something else changes).
	for _, t := range bashTargets(filepath.Join(dir, "targets.tsv")) {
		if _, ok := s.Target(t.Source, t.Game, t.Profile); !ok {
			s.Targets = append(s.Targets, t)
		}
	}
	return s, nil
}

// Save writes both files.
func (s *State) Save() error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	if err := writeTOML(filepath.Join(s.Dir, "config.toml"),
		"# game-mod-sync settings (see the README, Settings)\n", s.Config); err != nil {
		return err
	}
	return writeTOML(filepath.Join(s.Dir, "targets.toml"),
		"# Where each profile syncs to. Written by game-mod-sync; Edit > Change a profile's server.\n",
		targetsFile{s.Targets})
}

func writeTOML(path, header string, v any) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	f.WriteString(header)
	err = toml.NewEncoder(f).Encode(v)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// Target returns the saved target for a profile.
func (s *State) Target(source, game, profile string) (Target, bool) {
	for _, t := range s.Targets {
		if t.Same(source, game, profile) {
			return t, true
		}
	}
	return Target{}, false
}

// SetTarget adds or replaces a profile's target.
func (s *State) SetTarget(t Target) {
	for i, o := range s.Targets {
		if o.Same(t.Source, t.Game, t.Profile) {
			s.Targets[i] = t
			return
		}
	}
	s.Targets = append(s.Targets, t)
}

// RemoveTarget forgets a profile's target.
func (s *State) RemoveTarget(source, game, profile string) {
	out := s.Targets[:0]
	for _, t := range s.Targets {
		if !t.Same(source, game, profile) {
			out = append(out, t)
		}
	}
	s.Targets = out
}

// migrate reads the 0.x versions' config (shell KEY=value lines) and
// targets.tsv, if they're there.
func (s *State) migrate() error {
	cfg, tsv := filepath.Join(s.Dir, "config"), filepath.Join(s.Dir, "targets.tsv")
	if f, err := os.Open(cfg); err == nil {
		vals := map[string]string{}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			k, v, ok := strings.Cut(line, "=")
			if ok {
				vals[k] = shellUnquote(v)
			}
		}
		f.Close()
		c := &s.Config
		c.GaleDataDir = vals["GALE_DATA_DIR"]
		c.moveGaleDir()
		c.LastBackend, c.LastHost, c.LastUser = vals["LAST_BACKEND"], vals["LAST_HOST"], vals["LAST_USER"]
		if v := vals["AMP_INSTANCES_DIR"]; v != "" {
			c.AMPInstancesDir = v
		}
		c.ExtraExcludes = strings.Fields(vals["EXTRA_EXCLUDES"])
		if v, ok := vals["KEEP_BACKUPS"]; ok {
			fmt.Sscan(v, &c.KeepBackups)
		}
		c.DefaultUI = vals["DEFAULT_UI"]
		for _, e := range strings.Split(vals["LOCAL_WORLDS"], "|") {
			if g, d, ok := strings.Cut(e, "="); ok {
				if c.LocalWorlds == nil {
					c.LocalWorlds = map[string]string{}
				}
				c.LocalWorlds[g] = d
			}
		}
		s.Migrated = true
	}
	if _, err := os.Stat(tsv); err == nil {
		s.Targets = append(s.Targets, bashTargets(tsv)...)
		s.Migrated = true
	}
	if s.Migrated {
		return s.Save()
	}
	return nil
}

// bashTargets reads the 0.x versions' targets.tsv:
// source  game  profile  backend  user@host  root  [kept]  [worlds]
func bashTargets(path string) []Target {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []Target
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 6 {
			continue
		}
		for len(f) < 8 {
			f = append(f, "")
		}
		user, host, ok := strings.Cut(f[4], "@")
		if !ok {
			user, host = "", f[4]
		}
		t := Target{Source: f[0], Game: f[1], Profile: f[2], Backend: f[3], Host: host, User: user, Root: f[5], Worlds: f[7]}
		for _, k := range strings.Split(f[6], ",") {
			if k != "" {
				t.Kept = append(t.Kept, k)
			}
		}
		out = append(out, t)
	}
	return out
}

// shellUnquote reads a value as the 0.x scripts wrote it (bash's printf %q): ” or 'a b' or a\ b.
func shellUnquote(v string) string {
	if strings.HasPrefix(v, "'") && strings.HasSuffix(v, "'") && len(v) >= 2 {
		return strings.ReplaceAll(v[1:len(v)-1], `'\''`, "'")
	}
	if strings.HasPrefix(v, `"`) && strings.HasSuffix(v, `"`) && len(v) >= 2 {
		v = v[1 : len(v)-1]
	}
	var b strings.Builder
	for i := 0; i < len(v); i++ {
		if v[i] == '\\' && i+1 < len(v) {
			i++
		}
		b.WriteByte(v[i])
	}
	return b.String()
}

// moveGaleDir moves gale_data_dir (older settings) into source_dirs.
func (c *Config) moveGaleDir() {
	if c.GaleDataDir == "" {
		return
	}
	if c.SourceDirs == nil {
		c.SourceDirs = map[string]string{}
	}
	if c.SourceDirs["gale"] == "" {
		c.SourceDirs["gale"] = c.GaleDataDir
	}
	c.GaleDataDir = ""
}
