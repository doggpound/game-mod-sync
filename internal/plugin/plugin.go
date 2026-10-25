// Package plugin is what a plugin implements, and the registry plugins add
// themselves to. The plugins live in plugins/ (one folder each) and register
// in an init function; plugins/all imports every one of them, so adding a
// plugin is a new folder plus one line there. The program only reaches
// plugins through this package.
//
// Kinds:
//
//	Source       where profiles come from (a mod manager, e.g. Gale)
//	Loader       where a mod loader keeps mods and configs (e.g. BepInEx)
//	Server       a type of server: how to find game folders on it (e.g. AMP)
//	WorldFormat  how a game saves a world (e.g. Valheim's chunked folders)
//	game files   plugins/games/*.toml: settings, no code (see games.Register)
//
// Optional abilities are separate interfaces a plugin implements only when
// it has them (e.g. ManagedFiles for a server type that installs mods itself).
package plugin

import (
	"fmt"
	"sort"
	"sync"

	"github.com/pkg/sftp"

	"github.com/doggpound/game-mod-sync/internal/state"
	"github.com/doggpound/game-mod-sync/internal/worlds"
)

// ---------------------------------------------------------------- sources --

// Profile is one profile of a mod manager.
type Profile struct {
	Game string // game id, e.g. valheim (picks the game's settings)
	Name string
	Dir  string // its folder, laid out like the game's install folder
}

// Source is a mod manager.
type Source interface {
	Name() string  // saved in the settings, e.g. "gale"; never change it
	Label() string // shown, e.g. "Gale"
	// Find returns the manager's data folders on this computer (when there
	// are several, the person picks one and it's remembered).
	Find() []string
	// Profiles lists the profiles in a data folder.
	Profiles(dataDir string) ([]Profile, error)
}

// ModTagger is a Source that knows its mods' Thunderstore categories
// offline ("Author-Name" -> "cat|cat"), so client/server checks need no network.
type ModTagger interface {
	ModTags(dataDir, game string) map[string]string
}

// ---------------------------------------------------------------- loaders --

// Layout is where a mod loader keeps things, relative to a profile or game folder.
type Layout struct {
	SyncDirs   []string // mod folders, mirrored (only folders the mod manager fully owns)
	Excludes   []string // never sent or deleted inside SyncDirs (rsync-style patterns)
	ConfigDirs []string // offered per file, never deleted
	ConfigSkip []string // never offered from ConfigDirs
}

// Loader is a mod loader.
type Loader interface {
	Name() string
	Label() string
	// Detect reports whether a profile folder uses this loader.
	Detect(profileDir string) bool
	Layout() Layout
}

// ConfigOwner is a Loader that can tell which mod a config file belongs to:
// "plugin id -> mod folder" for the mods in a sync folder.
type ConfigOwner interface {
	ConfigOwners(syncDir string) map[string]string
}

// ---------------------------------------------------------------- servers --

// Server is a type of server.
type Server interface {
	Name() string        // saved in the settings, e.g. "amp"; never change it
	Label() string       // shown at setup
	Help() string        // one or two lines for setup's help
	DefaultUser() string // offered at setup ("" = none)
	// SearchFrom is where to look for game folders, and how deep: home is
	// where the login starts, canRun whether it can run commands.
	SearchFrom(cfg *state.Config, home string, canRun bool) (start string, depth int)
	// ShortName is a game folder's name for menus (e.g. AMP's instance name).
	ShortName(cfg *state.Config, root string) string
}

// Fallback is a Server offered last at setup ("Other").
type Fallback interface{ Fallback() bool }

// ManagedFile is a file a server installs itself.
type ManagedFile struct {
	Pattern string // relative to a sync folder, e.g. ValheimPlus.dll
	Reason  string
}

// ManagedFiles is a Server that installs some mods itself (e.g. AMP's
// "Install ValheimPlus"). specs are the game's managed.<server> entries
// ("setting=pattern[,pattern]"); it returns the ones that apply.
type ManagedFiles interface {
	Managed(cfg *state.Config, c *sftp.Client, root string, specs []string) ([]ManagedFile, error)
}

// SaveFinder is a Server that knows where the game saves and which world
// it runs (spec: the game's worlds.saves.<server>). saveDir is absolute or
// relative to root; either may be "".
type SaveFinder interface {
	Saves(cfg *state.Config, c *sftp.Client, root, spec string) (saveDir, world string)
}

// ---------------------------------------------------------- world formats --

// WorldFormat is how a game saves a world.
type WorldFormat interface {
	Label() string
	// List returns the complete worlds in a worlds folder, in the order to offer them.
	List(f worlds.FS, dir string) ([]worlds.World, error)
	// Paths are the names in the worlds folder that make up a world.
	Paths(name string) []string
	// Stamp changes with every save of the world ("" = no complete save). It's
	// compared between the server and a copy, so it must not depend on times.
	Stamp(f worlds.FS, dir, name string) string
}

// NewWorldFormat makes a format; files are the game file's worlds.files.
type NewWorldFormat func(files []string) (WorldFormat, error)

// --------------------------------------------------------------- registry --

var (
	mu      sync.Mutex
	sources = map[string]Source{}
	loaders = map[string]Loader{}
	servers = map[string]Server{}
	formats = map[string]NewWorldFormat{}
)

func add[T any](m map[string]T, kind, name string, v T) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := m[name]; dup {
		panic(fmt.Sprintf("plugin: two %s plugins named %q", kind, name))
	}
	m[name] = v
}

// RegisterSource adds a mod manager (call it from the plugin's init).
func RegisterSource(s Source) { add(sources, "source", s.Name(), s) }

// RegisterLoader adds a mod loader.
func RegisterLoader(l Loader) { add(loaders, "loader", l.Name(), l) }

// RegisterServer adds a type of server.
func RegisterServer(s Server) { add(servers, "server", s.Name(), s) }

// RegisterWorldFormat adds a world format under its name (the game file's worlds.format).
func RegisterWorldFormat(name string, f NewWorldFormat) { add(formats, "world format", name, f) }

func sorted[T any](m map[string]T) []T {
	mu.Lock()
	defer mu.Unlock()
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]T, 0, len(names))
	for _, n := range names {
		out = append(out, m[n])
	}
	return out
}

// Sources are the mod managers, by name.
func Sources() []Source { return sorted(sources) }

// Loaders are the mod loaders, by name (tried in this order).
func Loaders() []Loader { return sorted(loaders) }

// Servers are the server types, by name, with the Fallback ones last.
func Servers() []Server {
	all := sorted(servers)
	var out, last []Server
	for _, s := range all {
		if f, ok := s.(Fallback); ok && f.Fallback() {
			last = append(last, s)
		} else {
			out = append(out, s)
		}
	}
	return append(out, last...)
}

// SourceNamed returns the mod manager with this name.
func SourceNamed(name string) (Source, bool) {
	mu.Lock()
	defer mu.Unlock()
	s, ok := sources[name]
	return s, ok
}

// ServerNamed returns the server type with this name.
func ServerNamed(name string) (Server, bool) {
	mu.Lock()
	defer mu.Unlock()
	s, ok := servers[name]
	return s, ok
}

// DetectLoader returns the loader a profile uses, else the first one.
func DetectLoader(profileDir string) (Loader, bool) {
	all := Loaders()
	if len(all) == 0 {
		return nil, false
	}
	if profileDir != "" {
		for _, l := range all {
			if l.Detect(profileDir) {
				return l, true
			}
		}
	}
	return all[0], true
}

// WorldFormatNamed makes the world format a game file names.
func WorldFormatNamed(name string, files []string) (WorldFormat, error) {
	mu.Lock()
	f, ok := formats[name]
	mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("unknown world format %q", name)
	}
	return f(files)
}
