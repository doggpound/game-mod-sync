package app

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/doggpound/game-mod-sync/internal/backends"
	"github.com/doggpound/game-mod-sync/internal/backends/amp"
	"github.com/doggpound/game-mod-sync/internal/remote"
	"github.com/doggpound/game-mod-sync/internal/state"
	"github.com/doggpound/game-mod-sync/internal/ui"
)

// Server types offered at setup.
const otherName = "ssh" // the name 0.x settings use for it

var serverTypes = []struct{ name, label, help string }{
	{amp.Name, "CubeCoders AMP (finds the game folders of its instances)",
		"logs in as the AMP user (or an AMP panel user on AMP's SFTP port) and lists the instances that have this game"},
	{otherName, "Other: SSH or SFTP details from your host",
		"any server you can log in to with SSH or SFTP: the address, port, user name and password your host gave you"},
}

// Setup asks where a profile syncs to and saves it. It returns the open
// connection too, so a sync right after doesn't log in again (the caller
// closes it). Returns ui.ErrBack when the person backs out (nothing is saved).
func (a *App) Setup(e Entry) (state.Target, *remote.Client, error) {
	g, p := e.Game, e.Profile
	cfg := &a.State.Config
	a.UI.Say("")
	a.UI.Say(fmt.Sprintf("One-time setup for %s / %s", g.Name, p.Name))

	rows := make([]string, len(serverTypes))
	def := 0
	for i, st := range serverTypes {
		rows[i] = st.label
		if st.name == cfg.LastBackend {
			def = i
		}
	}
	rows[0], rows[def] = rows[def], rows[0] // the last one used first
	order := []int{0, 1}
	order[0], order[def] = order[def], order[0]
	i, _, err := a.UI.Choose(ui.Menu{Title: "Setup: server type", Question: "What kind of server is it?", Rows: rows,
		Help: "Setup: server type\n\n" + serverTypes[0].label + ":\n  " + serverTypes[0].help + "\n\n" +
			serverTypes[1].label + ":\n  " + serverTypes[1].help})
	if err != nil {
		return state.Target{}, nil, err
	}
	backend := serverTypes[order[i]].name

	t := state.Target{Source: e.Source, Game: p.Game, Profile: p.Name, Backend: backend}
	host, err := a.UI.Ask("Server address (or paste an sftp:// link)", cfg.LastHost)
	if err != nil {
		return state.Target{}, nil, err
	}
	var folder string
	t.Host, t.Port, t.User, folder = parseServer(host)
	if t.Host == "" {
		return state.Target{}, nil, errors.New("a server address is required")
	}
	if t.Port == 0 {
		defPort := "22"
		if cfg.LastPort != 0 && t.Host == cfg.LastHost {
			defPort = strconv.Itoa(cfg.LastPort)
		}
		ps, err := a.UI.Ask("Port", defPort)
		if err != nil {
			return state.Target{}, nil, err
		}
		if t.Port, err = strconv.Atoi(ps); err != nil || t.Port < 1 || t.Port > 65535 {
			return state.Target{}, nil, fmt.Errorf("%q isn't a port number", ps)
		}
	}
	if t.User == "" {
		defUser := ""
		if backend == amp.Name {
			defUser = "amp"
		}
		if backend == cfg.LastBackend && cfg.LastUser != "" {
			defUser = cfg.LastUser
		}
		if t.User, err = a.UI.Ask("User name", defUser); err != nil {
			return state.Target{}, nil, err
		}
		if t.User == "" {
			return state.Target{}, nil, errors.New("a user name is required")
		}
	}
	cfg.LastBackend, cfg.LastHost, cfg.LastPort, cfg.LastUser = backend, t.Host, t.Port, t.User
	if err := a.State.Save(); err != nil {
		return state.Target{}, nil, err
	}

	c, err := a.dial(t)
	if err != nil {
		return state.Target{}, nil, err
	}
	fail := func(err error) (state.Target, *remote.Client, error) {
		c.Close()
		return state.Target{}, nil, err
	}
	if folder != "" {
		if t.Root, err = a.checkFolder(c, g.GameMarker, folder); err != nil {
			return fail(err)
		}
	} else if t.Root, err = a.pickFolder(c, e, t); err != nil {
		return fail(err)
	}
	a.State.SetTarget(t)
	if err := a.State.Save(); err != nil {
		return fail(err)
	}
	a.UI.OK(fmt.Sprintf("Saved: %s / %s -> %s on %s", g.Name, p.Name, a.where(t), t.Host))
	return t, c, nil
}

// parseServer reads "host", "user@host", "host:port" or an sftp:// or ssh://
// link with any of user, port and folder.
func parseServer(s string) (host string, port int, user, folder string) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "://") {
		if u, err := url.Parse(s); err == nil {
			host = u.Hostname()
			port, _ = strconv.Atoi(u.Port())
			user = u.User.Username()
			if u.Path != "" && u.Path != "/" {
				folder = u.Path
			}
			return
		}
	}
	if at := strings.LastIndex(s, "@"); at >= 0 {
		user, s = s[:at], s[at+1:]
	}
	if h, p, ok := strings.Cut(s, ":"); ok && !strings.Contains(p, ":") {
		if n, err := strconv.Atoi(p); err == nil {
			return h, n, user, ""
		}
	}
	return s, 0, user, ""
}

// pickFolder finds the game folders on the server and lets the person pick
// one, or type one.
func (a *App) pickFolder(c *remote.Client, e Entry, t state.Target) (string, error) {
	g := e.Game
	home, err := backends.Home(c)
	if err != nil {
		return "", err
	}
	start, depth := home, 6
	if t.Backend == amp.Name {
		depth = 8
		// The AMP user's instances; an AMP panel login over SFTP starts in
		// its instance instead, so search from wherever it starts.
		if c.CanRun() {
			start = path.Join(home, a.State.Config.AMPInstancesDir)
		}
	}
	a.UI.Info("Looking for " + g.GameMarker + " on the server ...")
	roots, err := backends.FindRoots(c, start, g.GameMarker, depth)
	if err != nil {
		a.UI.Warn(fmt.Sprintf("couldn't search %s: %v", start, err))
	}
	var mine, others []string
	for _, r := range roots {
		if g.MatchesServer(r) {
			mine = append(mine, r)
		} else {
			others = append(others, r)
		}
	}
	roots = append(mine, others...)
	if len(roots) == 1 && len(others) == 0 {
		a.UI.Say("Found " + roots[0])
		return roots[0], nil
	}
	rows := make([]string, 0, len(roots)+1)
	for _, r := range roots {
		label := r
		if t.Backend == amp.Name {
			label = amp.ShortName(r, a.State.Config.AMPInstancesDir) + "  " + r
		}
		if !g.MatchesServer(r) {
			label += "  (other game?)"
		}
		rows = append(rows, label)
	}
	rows = append(rows, "Type the folder")
	if len(roots) == 0 {
		a.UI.Say("No " + g.GameMarker + " folders found below " + start + ".")
	}
	i, _, err := a.UI.Choose(ui.Menu{Title: "Setup: game folder",
		Question: fmt.Sprintf("Where should '%s' sync to?", e.Profile.Name), Rows: rows,
		Help: "Setup: game folder\n\nThe game folders found on the server (the folder that contains " + g.GameMarker + ").\n" +
			"Ones that look like this game are listed first; '(other game?)' marks other games\n" +
			"that use the same mod loader. Pick 'Type the folder' to enter it yourself."})
	if err != nil {
		return "", err
	}
	if i < len(roots) {
		return roots[i], nil
	}
	typed, err := a.UI.Ask("Game folder on the server (the one that contains "+g.GameMarker+")", "")
	if err != nil {
		return "", err
	}
	return a.checkFolder(c, g.GameMarker, typed)
}

// checkFolder resolves a typed folder and makes sure it exists.
func (a *App) checkFolder(c *remote.Client, marker, typed string) (string, error) {
	if strings.TrimSpace(typed) == "" {
		return "", errors.New("a folder is required")
	}
	root, err := backends.Resolve(c, typed)
	if err != nil {
		return "", fmt.Errorf("%s: %w", typed, err)
	}
	fi, err := c.SFTP().Stat(root)
	if err != nil || !fi.IsDir() {
		return "", fmt.Errorf("%s doesn't exist on the server", root)
	}
	if fi, err := c.SFTP().Stat(path.Join(root, marker)); err != nil || !fi.IsDir() {
		a.UI.Warn(root + "/" + marker + " doesn't exist yet - it will be created on the first sync")
	}
	return root, nil
}
