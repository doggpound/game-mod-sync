// Package amp is the CubeCoders AMP server type. AMP keeps every instance
// under the AMP user's home (~/.ampdata/instances/<Instance>/...), with the
// game somewhere below it (Valheim: Valheim/896660/).
package amp

import (
	"bufio"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/pkg/sftp"
)

// Name and Label identify the server type in settings and menus.
const (
	Name  = "amp"
	Label = "CubeCoders AMP"
)

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

// ManagedFile is a file the server installs itself.
type ManagedFile struct {
	Pattern string // relative to a sync folder, e.g. ValheimPlus.dll
	Reason  string
}

// Managed reports which of the game's managed files AMP installs for this
// instance: specs are "setting=pattern[,pattern]" (the game's managed.amp),
// and a setting counts when the instance's App.AppSettings JSON has it true.
// Settings files are read over SFTP, so this works on SFTP-only logins too.
func Managed(c *sftp.Client, root, instancesDir string, specs []string) ([]ManagedFile, error) {
	inst, name, ok := Instance(root, instancesDir)
	if !ok || len(specs) == 0 {
		return nil, nil
	}
	settings, err := appSettings(c, inst)
	if err != nil || settings == "" {
		return nil, err
	}
	var out []ManagedFile
	for _, spec := range specs {
		setting, pats, ok := strings.Cut(spec, "=")
		if !ok {
			continue
		}
		if !strings.Contains(settings, `"`+setting+`":"true"`) && !strings.Contains(settings, `"`+setting+`":true`) {
			continue
		}
		for _, p := range strings.Split(pats, ",") {
			out = append(out, ManagedFile{Pattern: p, Reason: fmt.Sprintf("AMP setting %q is on for %s", setting, name)})
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
