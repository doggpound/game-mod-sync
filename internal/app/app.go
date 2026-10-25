package app

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/doggpound/game-mod-sync/internal/backends/amp"
	"github.com/doggpound/game-mod-sync/internal/configs"
	"github.com/doggpound/game-mod-sync/internal/games"
	"github.com/doggpound/game-mod-sync/internal/remote"
	"github.com/doggpound/game-mod-sync/internal/secrets"
	"github.com/doggpound/game-mod-sync/internal/sources/gale"
	"github.com/doggpound/game-mod-sync/internal/state"
	"github.com/doggpound/game-mod-sync/internal/transfer"
	"github.com/doggpound/game-mod-sync/internal/ui"
	"github.com/doggpound/game-mod-sync/internal/ui/text"
	fulltui "github.com/doggpound/game-mod-sync/internal/ui/tui"
)

// Options are the command line's choices.
type Options struct {
	Game     string // -g: only this game
	Profile  string // -p: this profile, no menu
	DryRun   bool   // -n
	Yes      bool   // -y
	Verbose  bool   // -v
	Setup    bool   // --setup
	Download bool   // --download-world: download a world instead of syncing
	World    string // --world NAME: the world to download
}

// App is one run of game-mod-sync.
type App struct {
	UI      ui.UI
	State   *state.State
	Opts    Options
	Version string
	// Dial connects to a target's server; nil = SSH with the usual logins.
	Dial func(t state.Target) (*remote.Client, error)
	// Secrets keeps login passwords; nil = the system's credential store.
	Secrets secrets.Store
	// Tags looks up mods' client/server tags; nil = Thunderstore with the usual cache.
	Tags *configs.Tags

	galeDir string  // Gale's data folder, once found
	entries []Entry // the profiles listed in the menu
}

// Entry is one profile that can be synced.
type Entry struct {
	Source  string
	Profile gale.Profile
	Game    *games.Game
}

// Label is the profile's line in menus: the profile, a tab, where it syncs
// to (the interfaces line the parts up in columns).
func (a *App) Label(e Entry) string {
	t, ok := a.State.Target(e.Source, e.Profile.Game, e.Profile.Name)
	if !ok {
		return fmt.Sprintf("%s / %s\tnot set up yet", e.Game.Name, e.Profile.Name)
	}
	return fmt.Sprintf("%s / %s\t→ %s on %s", e.Game.Name, e.Profile.Name, a.where(t), t.Host)
}

func (a *App) where(t state.Target) string {
	if t.Backend == amp.Name {
		return amp.ShortName(t.Root, a.State.Config.AMPInstancesDir)
	}
	return t.Root
}

// galeRoot finds Gale's data folder (asks once when there are several).
func (a *App) galeRoot() (string, error) {
	if d := a.State.Config.GaleDataDir; d != "" {
		if _, err := os.Stat(d); err != nil {
			return "", fmt.Errorf("Gale's data folder in the settings doesn't exist: %s", d)
		}
		return d, nil
	}
	c := gale.Candidates()
	switch len(c) {
	case 0:
		return "", errors.New("couldn't find Gale's data folder. If it's somewhere unusual, set gale_data_dir in " +
			filepath.Join(a.State.Dir, "config.toml"))
	case 1:
		return c[0], nil
	}
	i, _, err := a.UI.Choose(ui.Menu{Title: "Gale data folder", Question: "Found more than one Gale data folder. Which one is yours?", Rows: c})
	if err != nil {
		return "", err
	}
	a.State.Config.GaleDataDir = c[i]
	return c[i], a.State.Save()
}

// Entries lists the profiles that have something to sync.
func (a *App) Entries() ([]Entry, error) {
	root, err := a.galeRoot()
	if err != nil {
		return nil, err
	}
	a.galeDir = root
	profiles, err := gale.List(root)
	if err != nil {
		return nil, err
	}
	var out []Entry
	for _, p := range profiles {
		if a.Opts.Game != "" && p.Game != a.Opts.Game {
			continue
		}
		g, err := games.Load(p.Game, p.Dir, a.userGames())
		if err != nil {
			return nil, err
		}
		if g.HasSyncDirs(p.Dir) {
			out = append(out, Entry{Source: gale.Name, Profile: p, Game: g})
		}
	}
	return out, nil
}

func (a *App) userGames() string { return filepath.Join(a.State.Dir, "plugins", "games") }

// Main runs the program for the parsed options: one profile, or the menu.
func (a *App) Main() error {
	entries, err := a.Entries()
	if err != nil {
		return err
	}
	a.entries = entries
	if len(entries) == 0 {
		msg := "no profiles with mods found"
		if a.Opts.Game != "" {
			msg += " for game '" + a.Opts.Game + "'"
		}
		return errors.New(msg)
	}
	if a.Opts.Profile != "" {
		var pick []Entry
		for _, e := range entries {
			if e.Profile.Name == a.Opts.Profile {
				pick = append(pick, e)
			}
		}
		switch len(pick) {
		case 0:
			return fmt.Errorf("no profile named '%s'", a.Opts.Profile)
		case 1:
			if a.Opts.Download {
				return a.Download(pick[0])
			}
			return a.Sync(pick[0])
		}
		return fmt.Errorf("'%s' exists for several games - add -g GAME", a.Opts.Profile)
	}
	if len(entries) == 1 && a.Opts.Yes {
		if a.Opts.Download {
			return a.Download(entries[0])
		}
		return a.Sync(entries[0])
	}
	if a.Opts.Download {
		return errors.New("--download-world needs a profile: add -p PROFILE (or press D on it in the menu)")
	}
	for {
		rows := make([]string, len(entries))
		for i, e := range entries {
			rows[i] = a.Label(e)
		}
		i, key, err := a.UI.Choose(ui.Menu{
			Title:    "Main menu",
			Question: "Which profile do you want to sync?",
			Rows:     rows,
			Keys:     []ui.Key{{Letter: 'e', Name: "Edit"}, {Letter: 'd', Name: "Download"}},
			NoBack:   true,
			Help: `Main menu

Pick a profile to sync it to its server. Each line shows the game and
profile, then where it syncs to (or "not set up yet").

A sync previews what changes and asks before changing anything.
Afterwards you come back here.`,
		})
		if errors.Is(err, ui.ErrExit) {
			return nil
		}
		if err != nil {
			return err
		}
		switch key {
		case "Edit":
			if err := a.Edit(entries[i]); errors.Is(err, ui.ErrExit) {
				return nil
			} else if err != nil && !errors.Is(err, ui.ErrBack) {
				a.UI.Warn(err.Error())
			}
			continue
		case "Download":
			if err := a.Download(entries[i]); errors.Is(err, ui.ErrExit) {
				return nil
			} else if err != nil {
				a.UI.Warn(err.Error())
			}
			continue
		}
		if err := a.Sync(entries[i]); errors.Is(err, ui.ErrExit) {
			return nil
		} else if err != nil {
			a.UI.Warn(err.Error())
		}
		a.Opts.Setup = false
	}
}

// Sync syncs one profile to its server.
func (a *App) Sync(e Entry) error {
	g, p := e.Game, e.Profile
	t, ok := a.State.Target(e.Source, p.Game, p.Name)
	var c *remote.Client
	if !ok || a.Opts.Setup {
		if a.Opts.Yes && !ok {
			return fmt.Errorf("%s / %s has no server set up yet - run it once without -y to set it up", g.Name, p.Name)
		}
		var err error
		if t, c, err = a.Setup(e); err != nil {
			if errors.Is(err, ui.ErrBack) {
				return nil // backed out: nothing saved
			}
			return err
		}
	} else {
		var err error
		if c, err = a.dial(t); err != nil {
			return err
		}
	}
	defer c.Close()

	a.UI.Say("")
	a.UI.Say(fmt.Sprintf("%s / %s  ->  %s on %s", g.Name, p.Name, a.where(t), t.Host))
	keep, err := a.serverManaged(c, e, &t)
	if err != nil {
		return err
	}
	ex := append(append(append(transfer.Excludes{}, g.Excludes...), a.State.Config.ExtraExcludes...), keep...)
	fs := &FolderSync{
		Client: c, Local: p.Dir, Root: t.Root, Dirs: g.SyncDirs, Excludes: ex,
		KeepBackups: a.State.Config.KeepBackups,
		DryRun:      a.Opts.DryRun, Yes: a.Opts.Yes, Verbose: a.Opts.Verbose, UI: a.UI,
		Title: fmt.Sprintf("%s / %s  ->  %s on %s", g.Name, p.Name, a.where(t), t.Host),
	}
	if x := a.configExtra(e, c, t.Root); x != nil {
		fs.Extra = x
	}
	err = fs.Run()
	if errors.Is(err, ErrNothingToSync) {
		return fmt.Errorf("'%s' has %w", p.Name, err)
	}
	if err == nil && fs.Applied && g.AfterSync != "" {
		a.UI.Say(g.AfterSync)
	}
	return err
}

// serverManaged handles files the server installs itself (e.g. AMP's
// "Install ValheimPlus"): asks once per file whether to leave the server's
// copy alone (default yes), remembers the answer in the target, and returns
// the excludes for the ones left alone (anchored to the sync folder's top, so
// a same-named file inside a mod's own folder still syncs).
func (a *App) serverManaged(c *remote.Client, e Entry, t *state.Target) (transfer.Excludes, error) {
	var keep transfer.Excludes
	answered := map[string]bool{}
	for _, k := range t.Kept {
		answered[k[1:]] = true
		if strings.HasPrefix(k, "+") {
			keep = append(keep, "/"+strings.TrimPrefix(k[1:], "/"))
		}
	}
	specs := e.Game.Managed[t.Backend]
	if t.Backend != amp.Name || len(specs) == 0 {
		return keep, nil
	}
	files, err := amp.Managed(c.SFTP(), t.Root, a.State.Config.AMPInstancesDir, specs)
	if err != nil {
		a.UI.Warn(fmt.Sprintf("couldn't read the AMP settings: %v", err))
		return keep, nil
	}
	changed := false
	for _, f := range files {
		if answered[f.Pattern] {
			if strings.Contains(strings.Join(t.Kept, ","), "+"+f.Pattern) {
				a.UI.Say(fmt.Sprintf("Leaving %s alone - the server installs it (%s)", f.Pattern, f.Reason))
			}
			continue
		}
		a.UI.Warn(fmt.Sprintf("The server installs %s itself (%s).", f.Pattern, f.Reason))
		if a.Opts.Yes || a.UI.Confirm("Leave the server's "+f.Pattern+" alone when syncing?", true) {
			t.Kept = append(t.Kept, "+"+f.Pattern)
			keep = append(keep, "/"+strings.TrimPrefix(f.Pattern, "/"))
		} else {
			t.Kept = append(t.Kept, "-"+f.Pattern)
		}
		changed = true
	}
	if changed {
		a.State.SetTarget(*t)
		return keep, a.State.Save()
	}
	return keep, nil
}

func (a *App) dial(t state.Target) (*remote.Client, error) {
	if a.Dial != nil {
		return a.Dial(t)
	}
	kh := remote.DefaultKnownHosts()
	hk, err := remote.HostKeys(kh, func(host, fp string) bool {
		a.UI.Say(fmt.Sprintf("First connection to %s. Its key fingerprint is\n  %s", host, fp))
		return a.UI.Confirm("Trust this server?", false)
	})
	if err != nil {
		return nil, err
	}
	port := t.Port
	if port == 0 {
		port = 22
	}
	store := a.Secrets
	if store == nil {
		store = secrets.System{}
	}
	account := fmt.Sprintf("%s@%s:%d", t.User, t.Host, port)
	pw := &passwords{ui: a.UI, store: store, account: account,
		question: fmt.Sprintf("Password for %s@%s", t.User, t.Host)}
	auth, closeAgent := remote.Auth(remote.DefaultKeyFiles(), pw.next)
	defer closeAgent()
	a.UI.Info("Connecting to " + t.User + "@" + t.Host + " ...")
	c, err := remote.Dial(remote.Options{
		Host: t.Host, Port: port, User: t.User, Auth: auth,
		HostKey: hk, HostKeyAlgorithms: remote.KnownAlgorithms(kh, t.Host, port),
	})
	if err != nil {
		return nil, err
	}
	pw.offerToSave()
	return c, nil
}

// passwords answers the server's password questions: the saved password
// first, then asking. After a login that needed a typed password, it offers
// to save it in the system's credential store.
type passwords struct {
	ui         ui.UI
	store      secrets.Store
	account    string
	question   string
	triedSaved bool
	typed      string
}

func (p *passwords) next() (string, error) {
	if !p.triedSaved {
		p.triedSaved = true
		if s, err := p.store.Get(p.account); err == nil && s != "" {
			return s, nil
		}
	}
	s, err := p.ui.Password(p.question)
	p.typed = s
	return s, err
}

func (p *passwords) offerToSave() {
	if p.typed == "" {
		return
	}
	if !p.ui.Confirm("Save the password on this computer (in the system's password store)?", true) {
		return
	}
	if err := p.store.Set(p.account, p.typed); err != nil {
		p.ui.Warn(fmt.Sprintf("couldn't save it (%v) - it will be asked each time", err))
		return
	}
	p.ui.OK("Saved - it won't be asked again.")
}

// Run parses the command line and runs.
// The interface is the full screen with --tui (or default_ui = "tui" in the
// settings, unless --cli), when in and out are a terminal; else text.
func Run(args []string, in io.Reader, out io.Writer, version string) error {
	fs := flag.NewFlagSet("game-mod-sync", flag.ContinueOnError)
	fs.SetOutput(out)
	var o Options
	var list, gamesFlag, config, cli, tui, refresh bool
	fs.StringVar(&o.Game, "g", "", "only this game (Gale's folder name, e.g. valheim)")
	fs.StringVar(&o.Game, "game", "", "same as -g")
	fs.StringVar(&o.Profile, "p", "", "this profile (skips the menu)")
	fs.StringVar(&o.Profile, "profile", "", "same as -p")
	fs.BoolVar(&o.DryRun, "n", false, "show what would change, change nothing")
	fs.BoolVar(&o.DryRun, "dry-run", false, "same as -n")
	fs.BoolVar(&o.Yes, "y", false, "don't ask for confirmation")
	fs.BoolVar(&o.Yes, "yes", false, "same as -y")
	fs.BoolVar(&o.Verbose, "v", false, "list every file, not just a per-mod summary")
	fs.BoolVar(&o.Setup, "setup", false, "(re)pick the server folder for the chosen profile")
	fs.BoolVar(&o.Download, "download-world", false, "copy a world from the profile's server into this computer's saves (with -p)")
	fs.Func("world", "the world to download (`NAME`; default: the one the server runs)", func(v string) error {
		o.World, o.Download = v, true
		return nil
	})
	fs.BoolVar(&refresh, "refresh-tags", false, "look up every mod's client/server tags on Thunderstore again")
	fs.BoolVar(&list, "list", false, "show where each profile syncs to and exit")
	fs.BoolVar(&gamesFlag, "games", false, "show detected games and what gets synced for each")
	fs.BoolVar(&config, "config", false, "print where settings are stored and exit")
	fs.BoolVar(&cli, "cli", false, "text menus and prompts (default)")
	fs.BoolVar(&tui, "tui", false, "full-screen interface (needs a terminal)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unknown argument: %s (see --help)", fs.Arg(0))
	}
	st, err := state.Load(state.Dir())
	if err != nil {
		return err
	}
	var u ui.UI = text.New(in, out)
	useTUI := tui || (st.Config.DefaultUI == "tui" && !cli)
	full := useTUI && !list && !gamesFlag && !config && fulltui.Usable(in, out)
	if full {
		t := fulltui.Start(in, out, version)
		defer t.Stop()
		u = t
	} else if tui {
		u.Say("The full-screen interface needs a terminal - using the text one.")
	}
	a := &App{UI: u, State: st, Opts: o, Version: version}
	if st.Migrated {
		u.Say("Read the settings of game-mod-sync 0.x from " + st.Dir + " (its own files are left in place).")
	}
	switch {
	case refresh:
		entries, err := a.Entries()
		if err != nil {
			return err
		}
		return a.RefreshTags(entries)
	case config:
		fmt.Fprintln(out, a.locations())
		return nil
	case list:
		if len(st.Targets) == 0 {
			fmt.Fprintln(out, "No profiles set up yet.")
		}
		for _, t := range st.Targets {
			g, err := games.Load(t.Game, "", a.userGames())
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "%s / %s -> %s on %s  (%s, %s)\n", g.Name, t.Profile, a.where(t), t.Host, t.Root, t.Backend)
			var kept []string
			for _, k := range t.Kept {
				if strings.HasPrefix(k, "+") {
					kept = append(kept, k[1:])
				}
			}
			if len(kept) > 0 {
				fmt.Fprintf(out, "    leaves alone: %s\n", strings.Join(kept, " "))
			}
		}
		return nil
	case gamesFlag:
		entries, err := a.Entries()
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, e := range entries {
			if seen[e.Profile.Game] {
				continue
			}
			seen[e.Profile.Game] = true
			g := e.Game
			fmt.Fprintf(out, "%s (%s, %s, %s)\n    sync: %s\n    keep on server: %s\n    configs: %s\n",
				g.Name, g.ID, gale.Label, g.Loader, strings.Join(g.SyncDirs, " "),
				strings.Join(append(append([]string{}, g.Excludes...), st.Config.ExtraExcludes...), " "),
				strings.Join(g.ConfigDirs, " "))
		}
		return nil
	}
	return a.Main()
}
