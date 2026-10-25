// Package amp is the CubeCoders AMP server type. AMP keeps every instance
// under the AMP user's home (~/.ampdata/instances/<Instance>/...), with the
// game somewhere below it (Valheim: Valheim/896660/). Game folders are found
// below the instances folder (or, for an AMP panel login on AMP's own SFTP,
// from where it starts); menus show the instance's name. It also reads the
// instance's settings: which mods AMP installs itself, where the game saves
// and which world it runs.
package amp

import (
	"bufio"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/pkg/sftp"

	"github.com/doggpound/game-mod-sync/internal/plugins"
	"github.com/doggpound/game-mod-sync/internal/state"
)

func init() { plugins.RegisterServer(Server{}) }

// Server is the AMP server type.
type Server struct{}

func (Server) Name() string  { return "amp" }
func (Server) Label() string { return "CubeCoders AMP (finds the game folders of its instances)" }
func (Server) Help() string {
	return "logs in as the AMP user (or an AMP panel user on AMP's SFTP port) and lists the instances that have this game"
}
func (Server) DefaultUser() string { return "amp" }

func (Server) SearchFrom(cfg *state.Config, home string, canRun bool) (string, int) {
	if canRun {
		return path.Join(home, cfg.AMPInstancesDir), 8
	}
	return home, 8 // an AMP panel login over SFTP starts in its instance
}

func (Server) ShortName(cfg *state.Config, root string) string {
	return ShortName(root, cfg.AMPInstancesDir)
}

func (Server) Managed(cfg *state.Config, c *sftp.Client, root string, specs []string) ([]plugins.ManagedFile, error) {
	return Managed(c, root, cfg.AMPInstancesDir, specs)
}

func (Server) Saves(cfg *state.Config, c *sftp.Client, root, spec string) (string, string) {
	return Saves(c, root, cfg.AMPInstancesDir, spec)
}

// Instance returns the instance folder and name for a game folder inside
// AMP's instances folder (instancesDir, e.g. .ampdata/instances), or false.
func Instance(root, instancesDir string) (dir, name string, ok bool) {
	marker := "/" + strings.Trim(instancesDir, "/") + "/"
	i := strings.Index(root, marker)
	if i < 0 {
		return "", "", false
	}
	rest := root[i+len(marker):]
	name, _, _ = strings.Cut(rest, "/")
	if name == "" {
		return "", "", false
	}
	return root[:i+len(marker)] + name, name, true
}

// ShortName is the instance name, for menus ("Valheim01"), else root.
func ShortName(root, instancesDir string) string {
	if _, name, ok := Instance(root, instancesDir); ok {
		return name
	}
	return root
}

// Managed reports which of the game's managed files AMP installs for this
// instance: specs are "setting=pattern[,pattern]" (the game's managed.amp),
// and a setting counts when the instance's App.AppSettings JSON has it true.
// Settings files are read over SFTP, so this works on SFTP-only logins too.
func Managed(c *sftp.Client, root, instancesDir string, specs []string) ([]plugins.ManagedFile, error) {
	inst, name, ok := Instance(root, instancesDir)
	if !ok || len(specs) == 0 {
		return nil, nil
	}
	settings, err := appSettings(c, inst)
	if err != nil || settings == "" {
		return nil, err
	}
	var out []plugins.ManagedFile
	for _, spec := range specs {
		setting, pats, ok := strings.Cut(spec, "=")
		if !ok {
			continue
		}
		if !strings.Contains(settings, `"`+setting+`":"true"`) && !strings.Contains(settings, `"`+setting+`":true`) {
			continue
		}
		for _, p := range strings.Split(pats, ",") {
			out = append(out, plugins.ManagedFile{Pattern: p, Reason: fmt.Sprintf("AMP setting %q is on for %s", setting, name)})
		}
	}
	return out, nil
}

// appSettings is the App.AppSettings line's JSON from the instance's .kvp files.
func appSettings(c *sftp.Client, inst string) (string, error) {
	files, err := c.Glob(path.Join(inst, "*.kvp"))
	if err != nil {
		return "", err
	}
	for _, f := range files {
		r, err := c.Open(f)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for sc.Scan() {
			if v, ok := strings.CutPrefix(sc.Text(), "App.AppSettings="); ok {
				r.Close()
				return v, nil
			}
		}
		r.Close()
	}
	return "", nil
}

// Saves reads where the game saves and which world it runs from the
// instance's settings. spec is the game's worlds.saves.amp: "ARG SETTING",
// the command-line option that names the save folder (in
// App.CommandLineArgs, e.g. -savedir "Saves", relative to the game folder)
// and the AppSettings entry that names the world (e.g. WorldName). Values
// still holding {{templates}} are left out; either may be "".
func Saves(c *sftp.Client, root, instancesDir, spec string) (saveDir, world string) {
	inst, _, ok := Instance(root, instancesDir)
	arg, setting, _ := strings.Cut(spec, " ")
	if !ok || arg == "" {
		return "", ""
	}
	files, err := c.Glob(path.Join(inst, "*.kvp"))
	if err != nil {
		return "", ""
	}
	var args, settings string
	for _, f := range files {
		r, err := c.Open(f)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for sc.Scan() {
			l := sc.Text()
			if v, ok := strings.CutPrefix(l, "App.CommandLineArgs="); ok && args == "" {
				args = v
			}
			if v, ok := strings.CutPrefix(l, "App.AppSettings="); ok && settings == "" {
				settings = v
			}
		}
		r.Close()
	}
	saveDir = optionValue(args, arg)
	if setting != "" {
		if m := regexp.MustCompile(`"` + regexp.QuoteMeta(setting) + `":"([^"]*)"`).FindStringSubmatch(settings); m != nil {
			world = m[1]
		}
	}
	if strings.Contains(saveDir, "{{") {
		saveDir = ""
	}
	if strings.Contains(world, "{{") {
		world = ""
	}
	return saveDir, world
}

// optionValue is the value after option in a command line: a word, or
// "several words" in quotes.
func optionValue(cmdline, option string) string {
	words := strings.Fields(cmdline)
	for i := 0; i < len(words)-1; i++ {
		if words[i] != option {
			continue
		}
		v := words[i+1]
		if !strings.HasPrefix(v, `"`) {
			return v
		}
		for j := i + 2; !(len(v) > 1 && strings.HasSuffix(v, `"`)) && j < len(words); j++ {
			v += " " + words[j]
		}
		return strings.Trim(v, `"`)
	}
	return ""
}
