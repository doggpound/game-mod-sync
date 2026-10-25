package remote

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kevinburke/ssh_config"
)

// Resolved is where to connect after ~/.ssh/config.
type Resolved struct {
	Host string   // the real address (HostName), else the name as given
	Port int      // the config's Port when the setup has the default 22
	Keys []string // the config's IdentityFile entries
}

// Resolve applies ~/.ssh/config to a server name and port, like the ssh
// command: a Host block can give the real address (HostName), a port and key
// files. A port other than 22 in the setup wins over the config's.
func Resolve(host string, port int) Resolved {
	r := Resolved{Host: host, Port: port}
	if r.Port == 0 {
		r.Port = 22
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return r
	}
	f, err := os.Open(filepath.Join(home, ".ssh", "config"))
	if err != nil {
		return r
	}
	defer f.Close()
	cfg, err := ssh_config.Decode(f)
	if err != nil {
		return r
	}
	if hn, _ := cfg.Get(host, "HostName"); hn != "" {
		r.Host = strings.ReplaceAll(hn, "%h", host)
	}
	if p, _ := cfg.Get(host, "Port"); p != "" && r.Port == 22 {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			r.Port = n
		}
	}
	files, _ := cfg.GetAll(host, "IdentityFile")
	for _, k := range files {
		if strings.HasPrefix(k, "~/") {
			k = filepath.Join(home, k[2:])
		}
		r.Keys = append(r.Keys, k)
	}
	return r
}
