package app

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/doggpound/game-mod-sync/internal/plugins"
	"github.com/doggpound/game-mod-sync/internal/plugins/worlds"
	"github.com/doggpound/game-mod-sync/internal/remote"
	"github.com/doggpound/game-mod-sync/internal/state"
	"github.com/doggpound/game-mod-sync/internal/ui"
)

// worldTries is how often a world is copied before giving up because the
// server kept saving during the copy.
const worldTries = 3

// afterWorldCopy runs after each copy in tests (to make the server "save").
var afterWorldCopy func(try int)

// Download copies a world from the profile's server into this computer's
// saves for the game (D in the main menu, --download-world). Nothing on the
// server is changed.
func (a *App) Download(e Entry) error {
	g, p := e.Game, e.Profile
	if g.Worlds.Format == "" {
		a.UI.Say(g.Name + " has no world download (its game file sets no world format).")
		return nil
	}
	format, err := plugins.WorldFormatNamed(g.Worlds.Format, g.Worlds.Files)
	if err != nil {
		return err
	}
	t, ok := a.State.Target(e.Source, p.Game, p.Name)
	var c *remote.Client
	if !ok || a.Opts.Setup {
		if a.Opts.Yes && !ok {
			return fmt.Errorf("%s / %s has no server set up yet - run it once without -y to set it up", g.Name, p.Name)
		}
		if t, c, err = a.Setup(e); err != nil {
			if errors.Is(err, ui.ErrBack) {
				return nil
			}
			return err
		}
	} else if c, err = a.dial(t); err != nil {
		return err
	}
	defer c.Close()

	dir, running, err := a.serverWorlds(c, e, &t)
	if err != nil {
		if errors.Is(err, ui.ErrBack) {
			return nil
		}
		return err
	}
	a.UI.Info("Looking for worlds in " + dir + " ...")
	list, err := format.List(worlds.ServerFS{C: c.SFTP()}, dir)
	if err != nil {
		return fmt.Errorf("can't list %s on the server: %w", dir, err)
	}
	if len(list) == 0 {
		return fmt.Errorf("no %s worlds in %s on the server", g.Name, dir)
	}
	// The world the server runs first, then the rest in the format's order.
	sort.SliceStable(list, func(i, j int) bool { return list[i].Name == running && list[j].Name != running })

	var w worlds.World
	switch {
	case a.Opts.World != "":
		for _, x := range list {
			if x.Name == a.Opts.World {
				w = x
			}
		}
		if w.Name == "" {
			var names []string
			for _, x := range list {
				names = append(names, x.Name)
			}
			return fmt.Errorf("no world named '%s' on the server (there: %s)", a.Opts.World, strings.Join(names, " "))
		}
	case a.Opts.Yes || len(list) == 1:
		if len(list) > 1 && list[0].Name != running {
			return fmt.Errorf("several worlds on the server - pick one with --world NAME")
		}
		w = list[0]
	default:
		rows := make([]string, len(list))
		for i, x := range list {
			rows[i] = x.Name + "\t" + x.Detail
			if x.Name == running {
				rows[i] += ", the server's world"
			}
		}
		i, _, err := a.UI.Choose(ui.Menu{
			Title:    "Download a world: " + g.Name + " / " + p.Name,
			Question: "Which world do you want a copy of?",
			Rows:     rows,
			Help: "Download a world\n\nThe worlds on the server (" + a.where(t) + "). The one you pick is copied\n" +
				"into " + g.Name + "'s worlds on this computer, so you can play a copy of it with\n" +
				"your updated mods before syncing them. Your own copy of that world (if any)\n" +
				"is moved to a backup first. Nothing on the server is changed.",
		})
		if errors.Is(err, ui.ErrBack) {
			return nil
		}
		if err != nil {
			return err
		}
		w = list[i]
	}

	local, err := a.localWorlds(e)
	if err != nil {
		if errors.Is(err, ui.ErrBack) {
			return nil
		}
		return err
	}
	mine := format.Stamp(worlds.LocalFS{}, local, w.Name)
	backups := filepath.Join(filepath.Dir(local), "game-mod-sync-backups")
	a.UI.Say("")
	a.UI.Say(fmt.Sprintf("World %s (%s)", w.Name, w.Detail))
	a.UI.Say(fmt.Sprintf("  from  %s on %s: %s", a.where(t), t.Host, dir))
	a.UI.Say("  to    " + local)
	if mine != "" {
		a.UI.Say(fmt.Sprintf("  Your copy (%s) is moved to %s first.", mine, backups))
	}
	a.UI.Say("Nothing on the server is changed. Close " + g.Name + " on this computer first.")
	if g.Worlds.Note != "" {
		a.UI.Warn(g.Worlds.Note)
	}
	if a.Opts.DryRun {
		a.UI.OK("Dry run - nothing downloaded.")
		return nil
	}
	q := "Download " + w.Name + "?"
	if mine != "" {
		q = "Download " + w.Name + " and replace your copy?"
	}
	if !a.Opts.Yes && !a.UI.Confirm(q, false) {
		a.UI.Say("Cancelled - nothing changed.")
		return nil
	}

	stage := filepath.Join(worldsCache(), g.ID)
	stamp, err := a.fetchWorld(c, format, dir, w.Name, stage)
	if err != nil {
		return err
	}
	saved, err := installWorld(format, stage, w.Name, local, backups, a.State.Config.KeepBackups)
	if err != nil {
		return err
	}
	a.UI.OK(fmt.Sprintf("Downloaded %s (%s) to %s.", w.Name, stamp, local))
	if saved != "" {
		a.UI.Say("Your previous copy: " + saved)
	}
	a.UI.Say("Start " + g.Name + " with this profile and pick " + w.Name + " to play the copy.")
	return nil
}

// fetchWorld copies the world into the cache folder stage. The server may
// save meanwhile, so its save stamp is read before and after the copy, and
// the copy is good when both match the copy's own stamp.
func (a *App) fetchWorld(c *remote.Client, f plugins.WorldFormat, dir, name, stage string) (string, error) {
	srv := worlds.ServerFS{C: c.SFTP()}
	for try := 1; try <= worldTries; try++ {
		a.UI.Info("Checking " + name + "'s last save on the server ...")
		before := f.Stamp(srv, dir, name)
		if before == "" {
			return "", fmt.Errorf("%s has no complete save on the server", name)
		}
		a.UI.Info(fmt.Sprintf("Downloading %s (%s) from the server ...", name, before))
		err := worlds.Mirror(c.SFTP(), dir, stage, f.Paths(name), nil)
		if err != nil && !errors.Is(err, worlds.ErrChangedDuringCopy) {
			return "", fmt.Errorf("downloading %s: %w", name, err)
		}
		if afterWorldCopy != nil {
			afterWorldCopy(try)
		}
		after := f.Stamp(srv, dir, name)
		mine := f.Stamp(worlds.LocalFS{}, stage, name)
		if err == nil && before == after && after == mine {
			return mine, nil
		}
		a.UI.Warn(fmt.Sprintf("The server saved %s while it was being copied - copying again (%d of %d).", name, try, worldTries))
	}
	return "", fmt.Errorf("the server kept saving %s during the copy - try again in a minute", name)
}

// installWorld moves this computer's copy of the world (if any) to
// backups/<time>/ and puts the downloaded one in its place; if that fails,
// the old copy is moved back. Returns the backup folder ("" if there was
// nothing to back up).
func installWorld(f plugins.WorldFormat, stage, name, local, backups string, keep int) (string, error) {
	ts := time.Now().Format("20060102-150405")
	saved := ""
	var moved []string
	for _, p := range f.Paths(name) {
		cur := filepath.Join(local, filepath.FromSlash(p))
		if _, err := os.Lstat(cur); err != nil {
			continue
		}
		saved = filepath.Join(backups, ts)
		if err := os.MkdirAll(saved, 0o755); err != nil {
			return "", err
		}
		if err := os.Rename(cur, filepath.Join(saved, filepath.FromSlash(p))); err != nil {
			return "", fmt.Errorf("moving your copy of %s to the backups: %w", name, err)
		}
		moved = append(moved, p)
	}
	for _, p := range f.Paths(name) {
		src := filepath.Join(stage, filepath.FromSlash(p))
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := worlds.CopyTree(src, filepath.Join(local, filepath.FromSlash(p))); err != nil {
			for _, m := range moved { // put yours back as it was
				os.RemoveAll(filepath.Join(local, filepath.FromSlash(m)))
				os.Rename(filepath.Join(saved, filepath.FromSlash(m)), filepath.Join(local, filepath.FromSlash(m)))
			}
			return "", fmt.Errorf("copying %s into %s failed - your copy was left as it was: %w", name, local, err)
		}
	}
	if keep > 0 {
		if entries, err := os.ReadDir(backups); err == nil {
			var names []string
			for _, e := range entries {
				if e.IsDir() {
					names = append(names, e.Name())
				}
			}
			sort.Sort(sort.Reverse(sort.StringSlice(names)))
			for i, n := range names {
				if i >= keep {
					os.RemoveAll(filepath.Join(backups, n))
				}
			}
		}
	}
	return saved, nil
}

// worldsCache is where downloaded worlds are kept between downloads (only
// changed files are fetched again).
func worldsCache() string {
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		dir, _ = os.UserCacheDir()
	}
	return filepath.Join(dir, "game-mod-sync", "worlds")
}

// serverWorlds finds the server's worlds folder: one typed in before for
// this profile, else the server type's save folder (AMP: from the
// instance's settings), else the game's usual one under the login's start
// folder; when none exists, it asks and remembers. Also returns the world the
// server runs, when the server type knows it.
func (a *App) serverWorlds(c *remote.Client, e Entry, t *state.Target) (string, string, error) {
	g := e.Game
	fc := c.SFTP()
	isDir := func(p string) bool { fi, err := fc.Stat(p); return err == nil && fi.IsDir() }
	running := ""
	var cands []string
	if t.Worlds != "" {
		cands = append(cands, t.Worlds)
	}
	srv, _ := plugins.ServerNamed(t.Backend)
	if sf, ok := srv.(plugins.SaveFinder); ok && g.Worlds.Saves[t.Backend] != "" {
		saves, world := sf.Saves(&a.State.Config, fc, t.Root, g.Worlds.Saves[t.Backend])
		running = world
		if saves != "" {
			if !path.IsAbs(saves) {
				saves = path.Join(t.Root, saves)
			}
			cands = append(cands, path.Join(saves, g.Worlds.ServerWorlds))
		}
	}
	if g.Worlds.ServerSaves != "" {
		if home, err := remote.Home(c); err == nil {
			cands = append(cands, path.Join(home, g.Worlds.ServerSaves, g.Worlds.ServerWorlds))
		}
	}
	for _, d := range cands {
		if isDir(d) {
			return d, running, nil
		}
	}
	a.UI.Say("Couldn't find " + g.Name + "'s worlds on the server.")
	typed, err := a.UI.Ask(g.Name+"'s worlds folder on the server", "")
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(typed) == "" {
		return "", "", errors.New("a folder is required")
	}
	dir, err := remote.ResolvePath(c, typed)
	if err != nil || !isDir(dir) {
		return "", "", fmt.Errorf("%s doesn't exist on the server", typed)
	}
	t.Worlds = dir
	a.State.SetTarget(*t)
	return dir, running, a.State.Save()
}

// localWorlds finds this computer's worlds folder for the game: the one
// picked before, else under the Windows user folder (natively, or seen from
// WSL) and under $HOME; when there are several it asks once and remembers.
func (a *App) localWorlds(e Entry) (string, error) {
	g := e.Game
	cfg := &a.State.Config
	isDir := func(p string) bool { fi, err := os.Stat(p); return err == nil && fi.IsDir() }
	if d := cfg.LocalWorlds[g.ID]; d != "" && isDir(d) {
		return d, nil
	}
	var cands []string
	add := func(p string) {
		if isDir(p) {
			for _, c := range cands {
				if c == p {
					return
				}
			}
			cands = append(cands, p)
		}
	}
	if w := g.Worlds.LocalWindows; w != "" {
		if runtime.GOOS == "windows" {
			if u := os.Getenv("USERPROFILE"); u != "" {
				add(filepath.Join(u, filepath.FromSlash(w)))
			}
		} else {
			users := os.Getenv("GMS_WIN_USERS")
			if users == "" {
				users = "/mnt/*/Users/*"
			}
			matches, _ := filepath.Glob(users)
			for _, u := range matches {
				add(filepath.Join(u, filepath.FromSlash(w)))
			}
		}
	}
	if h := g.Worlds.LocalHome; h != "" {
		if home, err := os.UserHomeDir(); err == nil {
			add(filepath.Join(home, filepath.FromSlash(h)))
		}
	}
	switch len(cands) {
	case 0:
		return "", fmt.Errorf("can't find %s's worlds folder on this computer. Start %s once so it exists, "+
			"or set it in %s:\n  [local_worlds]\n  %s = '/path/to/worlds'",
			g.Name, g.Name, filepath.Join(a.State.Dir, "config.toml"), g.ID)
	case 1:
		return cands[0], nil
	}
	i, _, err := a.UI.Choose(ui.Menu{Title: "Local worlds folder", Question: "Which of these is your " + g.Name + " worlds folder?", Rows: cands})
	if err != nil {
		return "", err
	}
	if cfg.LocalWorlds == nil {
		cfg.LocalWorlds = map[string]string{}
	}
	cfg.LocalWorlds[g.ID] = cands[i]
	return cands[i], a.State.Save()
}
