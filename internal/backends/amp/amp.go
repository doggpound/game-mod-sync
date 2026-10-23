// Package amp is the CubeCoders AMP server type. AMP keeps every instance
// under the AMP user's home (~/.ampdata/instances/<Instance>/...), with the
// game somewhere below it (Valheim: Valheim/896660/).
package amp

import (
	"bufio"
	"fmt"
	"path"
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
