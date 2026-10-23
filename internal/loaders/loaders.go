// Package loaders knows where each mod loader keeps mods and configs, so games
// using the same loader share it. Loaders are tried in order; without a
// profile folder the first one is used.
package loaders

import (
	"os"
	"path/filepath"
)

// Loader is one mod loader's layout.
type Loader struct {
	Name       string
	Label      string
	SyncDirs   []string // mod folders, mirrored
	Excludes   []string // never sent or deleted inside SyncDirs
	ConfigDirs []string // offered per file, never deleted
	ConfigSkip []string // never offered from ConfigDirs
	// ConfigOwners returns "plugin id -> mod folder" for the mods in a sync
	// folder, to tell which mod a config file belongs to; may be nil.
	ConfigOwners func(syncDir string) map[string]string
	detect       func(profileDir string) bool
}

// All loaders, in the order they're tried.
var All = []Loader{
	{
		Name: "bepinex", Label: "BepInEx",
		SyncDirs:     []string{"BepInEx/plugins"},
		Excludes:     []string{"MMHOOK/"},
		ConfigDirs:   []string{"BepInEx/config"},
		ConfigSkip:   []string{"/BepInEx.cfg", "*.dat"},
		ConfigOwners: BepInExOwners,
		detect: func(dir string) bool {
			return isDir(filepath.Join(dir, "BepInEx", "core")) || isDir(filepath.Join(dir, "BepInEx", "plugins"))
		},
	},
}

// Detect returns the loader the profile uses, else the first one.
func Detect(profileDir string) Loader {
	if profileDir != "" {
		for _, l := range All {
			if l.detect(profileDir) {
				return l
			}
		}
	}
	return All[0]
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
