# Plugins

Everything game-mod-sync knows about a particular mod manager, mod loader, type of server, world
format or game is a plugin in this folder. The program itself (`internal/`) only talks to them
through the interfaces in [`internal/plugin`](../internal/plugin/plugin.go).

| Folder | Kind | Now |
|---|---|---|
| `sources/` | mod managers: where profiles come from | `gale` |
| `loaders/` | mod loaders: where mods and configs live | `bepinex` |
| `servers/` | server types: how to find game folders on a server | `amp`, `ssh` ("Other") |
| `worlds/` | world formats: how a game saves a world | `valheim`, `files` |
| `games/` | game files: settings per game, **no code** | `valheim.toml`, `_default.toml` |
| `all/` | the list of plugins built in | |

## Adding a game

Most BepInEx games work without a game file. To change something, add `games/<game id>.toml`,
where `<game id>` is the mod manager's folder name for the game (Gale: `lethal-company`). List only
what differs; [`games/_default.toml`](games/_default.toml) describes every setting.

```toml
# games/lethal-company.toml
name = "Lethal Company"
server_match = "lethal|1966720"          # server folders that match are listed first at setup
after_sync = "Restart the server in AMP."

[managed]                                 # files a server type installs itself
amp = ["some_setting=SomeMod.dll"]
```

To try it without building anything, put the file in your settings folder's `plugins/games/`
(`game-mod-sync --config` shows where): it's used the next time the program starts, and can also
change a built-in game's settings. A pull request that adds it here ships it to everyone; a test
loads every game file, so a mistake fails the checks.

## Adding a plugin with code

1. Make a folder: `servers/pterodactyl/`, `sources/r2modman/`, ...
2. Implement the kind's interface (below) and register it in `init`.
3. Add one line to [`all/all.go`](all/all.go). (A test fails if you forget.)
4. Add tests next to it (`*_test.go`). `internal/testserver` gives you a real SSH and SFTP server
   on a temporary folder, with an SFTP-only mode like most game hosts.

Names (`Name()`) are saved in people's settings: pick them once and don't change them.

### Mod manager (`plugin.Source`)

```go
package r2modman

func init() { plugin.RegisterSource(Source{}) }

type Source struct{}

func (Source) Name() string   { return "r2modman" }
func (Source) Label() string  { return "r2modman" }
func (Source) Find() []string { /* its data folders on this computer */ }
func (Source) Profiles(dataDir string) ([]plugin.Profile, error) {
	// one plugin.Profile{Game, Name, Dir} per profile; Game is the game's id,
	// Dir the profile's folder (laid out like the game's install folder)
}
```

Optional: `ModTags(dataDir, game)` (`plugin.ModTagger`) returns the Thunderstore categories the
manager has cached, so client/server checks work offline. See `sources/gale`.

### Mod loader (`plugin.Loader`)

`Detect(profileDir)` says whether a profile uses it; `Layout()` gives the folders: mods
(`SyncDirs`, mirrored), what to leave alone (`Excludes`), configs (`ConfigDirs`, offered per file)
and what never to offer (`ConfigSkip`). Optional: `ConfigOwners(syncDir)` (`plugin.ConfigOwner`)
maps config files to their mods. See `loaders/bepinex`.

### Server type (`plugin.Server`)

```go
func (Server) Name() string        { return "pterodactyl" }
func (Server) Label() string       { return "Pterodactyl panel" }  // shown at setup
func (Server) Help() string        { return "one or two lines for the setup's help" }
func (Server) DefaultUser() string { return "" }
// Where to search for game folders (home: where the login starts; canRun:
// the login can run commands) and how many folders deep.
func (Server) SearchFrom(cfg *state.Config, home string, canRun bool) (string, int) { return home, 4 }
// A game folder's name for menus.
func (Server) ShortName(cfg *state.Config, root string) string { return path.Base(root) }
```

Optional:

- `Managed(...)` (`plugin.ManagedFiles`): mods the server installs itself (AMP's "Install
  ValheimPlus"). The game file names them per server type (`[managed]`).
- `Saves(...)` (`plugin.SaveFinder`): where the game saves and which world it runs.
- `Fallback() bool` (`plugin.Fallback`): offered last at setup.

Everything on the server goes through the connection the program passes (`*sftp.Client`), so it
works on SFTP-only logins. See `servers/amp` and `servers/ssh`.

### World format (`plugin.WorldFormat`)

Registered by name (`plugin.RegisterWorldFormat("name", ...)`); a game file picks it
(`[worlds] format = "name"`). It lists the worlds in a folder, names the paths a world is made of,
and gives a save stamp that changes with every save (the download compares it before and after
the copy). It reads through `worlds.FS`, so the same code works on the server and this computer.
See `worlds/valheim` and `worlds/files`.
