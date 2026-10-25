// Package bepinex is the BepInEx mod loader: mods in BepInEx/plugins (the
// server's own MMHOOK/ left alone), configs in BepInEx/config (without
// BepInEx.cfg itself and per-player .dat files). It reads the plugins'
// [BepInPlugin] declarations to tell which mod a config file belongs to.
package bepinex

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/doggpound/game-mod-sync/internal/plugin"
)

func init() { plugin.RegisterLoader(Loader{}) }

// Loader is BepInEx.
type Loader struct{}

func (Loader) Name() string  { return "bepinex" }
func (Loader) Label() string { return "BepInEx" }

// Detect: a profile with BepInEx/core or BepInEx/plugins.
func (Loader) Detect(dir string) bool {
	return isDir(filepath.Join(dir, "BepInEx", "core")) || isDir(filepath.Join(dir, "BepInEx", "plugins"))
}

func (Loader) Layout() plugin.Layout {
	return plugin.Layout{
		SyncDirs:   []string{"BepInEx/plugins"},
		Excludes:   []string{"MMHOOK/"},
		ConfigDirs: []string{"BepInEx/config"},
		ConfigSkip: []string{"/BepInEx.cfg", "*.dat"},
	}
}

func (Loader) ConfigOwners(syncDir string) map[string]string { return BepInExOwners(syncDir) }

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

var (
	versionRe = regexp.MustCompile(`^\d+(\.\d+){1,3}$`)
	idRe      = regexp.MustCompile(`^[\x21-\x7e]+$`)
	nameRe    = regexp.MustCompile(`^[\x20-\x7e]+$`)
)

// BepInExOwners reads every plugin DLL under dir (a plugins folder) and
// returns "plugin id -> mod folder". Every BepInEx plugin declares
// [BepInPlugin("id", "Name", "1.2.3")]; in the DLL that's a custom-attribute
// blob: 01 00, then three length-prefixed UTF-8 strings. Config files are
// named after the id (Azumatt.FastLink.cfg). Reading the declaration, rather
// than any mention of the id, avoids matching mods that only depend on it.
func BepInExOwners(dir string) map[string]string {
	out := map[string]string{}
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "MMHOOK" {
				return filepath.SkipDir // generated hooks, not plugins
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(p), ".dll") {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		mod, _, _ := strings.Cut(rel, "/")
		for _, id := range pluginIDs(b) {
			out[id] = mod
		}
		return nil
	})
	return out
}

// pluginIDs finds the ids of [BepInPlugin] declarations in a DLL's bytes.
func pluginIDs(b []byte) []string {
	var ids []string
	for i := 0; ; {
		j := bytes.Index(b[i:], []byte{1, 0})
		if j < 0 {
			return ids
		}
		p := i + j + 2
		i = p
		var s []string
		for len(s) < 3 && p < len(b) {
			n := int(b[p])
			if n == 0 || n > 0x7f || p+1+n > len(b) {
				break
			}
			s = append(s, string(b[p+1:p+1+n]))
			p += 1 + n
		}
		if len(s) == 3 && versionRe.MatchString(s[2]) && idRe.MatchString(s[0]) && nameRe.MatchString(s[1]) {
			ids = append(ids, s[0])
		}
	}
}
