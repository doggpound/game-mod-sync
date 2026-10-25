package app

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/doggpound/game-mod-sync/internal/secrets"
	"github.com/doggpound/game-mod-sync/internal/ui"
)

// Edit is the Edit menu for one profile (E on it in the main menu).
func (a *App) Edit(e Entry) error {
	what := e.Game.Name + " / " + e.Profile.Name
	for {
		i, _, err := a.UI.Choose(ui.Menu{
			Title:    "Edit: " + what,
			Question: "Edit what?",
			Rows: []string{
				"Change this profile's server",
				"Forget this profile's server",
				"Forget the saved password for this profile's server",
				"Show file locations",
				"Refresh mod tags (client / server, from Thunderstore)",
				"Default interface (now: " + a.defaultUI() + ")",
			},
			Help: `Edit

Change this profile's server   run the first-time setup again
Forget this profile's server   remove the setup; the next sync asks again
Forget the saved password      remove it from the system's password store;
                               the next connection asks for it
Show file locations            where settings and your own game files are
Refresh mod tags               look up every mod's client/server tags on
                               Thunderstore again (they're cached otherwise)
Default interface              text (cli) or full screen (tui) when started
                               without --cli / --tui`,
		})
		if err != nil {
			return err
		}
		switch i {
		case 0:
			_, c, err := a.Setup(e)
			if c != nil {
				c.Close()
			}
			if errors.Is(err, ui.ErrExit) {
				return err
			} else if err != nil && !errors.Is(err, ui.ErrBack) {
				a.UI.Warn(err.Error())
			}
		case 1:
			if _, ok := a.State.Target(e.Source, e.Profile.Game, e.Profile.Name); !ok {
				a.UI.Say(what + " has no server set up.")
				continue
			}
			if a.UI.Confirm("Forget where "+what+" syncs to? The next sync will ask again.", false) {
				a.State.RemoveTarget(e.Source, e.Profile.Game, e.Profile.Name)
				if err := a.State.Save(); err != nil {
					return err
				}
				a.UI.OK("Forgot the server for " + what + ".")
			}
		case 2:
			t, ok := a.State.Target(e.Source, e.Profile.Game, e.Profile.Name)
			if !ok {
				a.UI.Say(what + " has no server set up.")
				continue
			}
			port := t.Port
			if port == 0 {
				port = 22
			}
			store := a.Secrets
			if store == nil {
				store = secrets.System{}
			}
			if err := store.Delete(fmt.Sprintf("%s@%s:%d", t.User, t.Host, port)); err != nil {
				a.UI.Warn(fmt.Sprintf("couldn't remove it: %v", err))
				continue
			}
			a.UI.OK("No password is saved for " + t.User + "@" + t.Host + " now.")
		case 3:
			a.UI.Say(a.locations())
		case 4:
			entries := a.entries
			if len(entries) == 0 {
				entries = []Entry{e}
			}
			if err := a.RefreshTags(entries); err != nil {
				a.UI.Warn(err.Error())
			}
		case 5:
			j, _, err := a.UI.Choose(ui.Menu{Title: "Default interface",
				Question: "Start game-mod-sync in which interface? (now: " + a.defaultUI() + ")",
				Rows:     []string{"cli - text menus and prompts", "tui - full screen"},
				Help: "Default interface\n\ngame-mod-sync starts in this interface when run without --cli or --tui.\n" +
					"Without a terminal (scripts) it always uses the text one."})
			if errors.Is(err, ui.ErrExit) {
				return err
			}
			if err != nil {
				continue
			}
			a.State.Config.DefaultUI = []string{"cli", "tui"}[j]
			if err := a.State.Save(); err != nil {
				return err
			}
			a.UI.OK("Default interface: " + a.State.Config.DefaultUI + " (from the next start). --cli / --tui still choose for one run.")
		}
	}
}

func (a *App) locations() string {
	return fmt.Sprintf("config:   %s\ntargets:  %s\ngames:    %s",
		filepath.Join(a.State.Dir, "config.toml"), filepath.Join(a.State.Dir, "targets.toml"), a.userGames())
}

func (a *App) defaultUI() string {
	if a.State.Config.DefaultUI == "cli" {
		return "cli"
	}
	return "tui"
}
