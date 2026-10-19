<p align="center"><img src="docs/logo.svg" width="128" alt="game-mod-sync logo"></p>

<h1 align="center">game-mod-sync</h1>

<p align="center">
  <a href="tests"><img src="docs/badges/panel.svg" alt="Status: version, tests, end-to-end sync, interface choice. Plugins: sources, loaders, server types, games."></a>
</p>

> [!WARNING]
> **Made with AI.** This project was written by Claude, an AI model made by Anthropic, with a
> human deciding what it should do, reviewing the changes and testing it on a real game
> server. Read the code before you point it at yours. It keeps backups of everything it replaces
> or removes on the server, but use it at your own risk.

Push the mods from a Gale profile to a game server over SSH.

One command with two interfaces; the options and behaviour are the same in both:

- `game-mod-sync` (or `--cli`): text menus and prompts. Also works in scripts (`-g … -p … -y`).
- `game-mod-sync --tui`: dialog boxes on one full screen, with a progress bar whenever it waits on
  the server or copies (needs `whiptail` or `dialog`; on Debian/Ubuntu:
  `sudo apt install whiptail`).

To start in full screen without typing `--tui`, set **Edit → Default interface** to `tui` (saved
as `DEFAULT_UI=tui` in the config file). Without a terminal (scripts, cron) it always uses text.

## Install

Needs `bash`, `rsync` and `ssh` locally (on Windows, use WSL) and `rsync` on the server.

```bash
git clone https://github.com/doggpound/game-mod-sync.git
mkdir -p ~/.local/bin
ln -s "$PWD/game-mod-sync/game-mod-sync" ~/.local/bin/game-mod-sync
ssh-copy-id amp@game.example.com
```

Or download a version from [Releases](https://github.com/doggpound/game-mod-sync/releases)
(`game-mod-sync-X.Y.Z.tar.gz`), extract it and link `game-mod-sync` the same way.

Releases are made by `.forgejo/workflows/release.yml`: a push to `main` that raises `VERSION` in
`lib/core.sh` tags that commit `vX.Y.Z` and publishes the release (notes from the commit messages,
the `.tar.gz` and its `.sha256`). To rebuild an old one, run the workflow by hand with its tag.

## Use

```bash
game-mod-sync        # or: game-mod-sync --tui
```

1. Pick a profile from the menu. It lists every Gale game and profile.
2. **First time only for that profile:** pick the server type (AMP, or "Other" to type a folder),
   the host and the SSH user. Then pick the matching game folder on the server.
3. Pick which config files to copy, press OK, then check the **Review** screen (mods and the
   config files you picked) and answer Yes to apply.
4. Press OK on the result (also when it was already in sync) and you're back at the menu. Pick another profile or **Exit** (Tab to the button; `x` in the text version).
5. Restart the server.

After the first time, the only question is which profile to sync. With `-p PROFILE` it syncs that
profile once and exits, without the menu.

```
==> Valheim / MyProfile  ->  Valheim01 on game.example.com
   + new      Azumatt-AzuClock
   ~ updated  ValheimModding-Jotunn
   - removed  Old-RemovedMod
   ...config files checklist...

Valheim / MyProfile  ->  Valheim01 on game.example.com
Mods: 5 file(s) to send, 2 to delete
   + new      Azumatt-AzuClock
   ~ updated  ValheimModding-Jotunn
   - removed  Old-RemovedMod
Config files to copy: 1
   new      server  ServersideQoL_AutoStore  ArgusMagnus.ServersideQoL.AutoStore.cfg
Replaced and deleted files are kept in .mod-sync-backups on the server.
Apply these changes to the server? (5 file(s) to send, 2 to delete, 1 config file(s) to copy) [y/N]
```

## Configs

Config files (`BepInEx/config`) are compared too, file by file, and offered in a checklist before
anything is applied:

- **New** (missing on the server, e.g. a mod you just added): ticked.
- **Differs**: unticked, with the settings that differ (`Lock Configuration: Off -> On` = server,
  then yours). Files that only differ in comments or spacing aren't shown.
- **Client mod only** (Thunderstore tag *Client-side* without *Server-side*): listed, never copied.
- Files from mods that aren't in the profile anymore are hidden (`-v` shows them).

```
       STATUS   SIDE    MOD                      FILE
   [*] new      server  ServersideQoL_AutoStore  ArgusMagnus.ServersideQoL.AutoStore.cfg
 ▾ [ ] differs  both    AzuAutoStore             Azumatt.AzuAutoStore.cfg
             Lock Configuration
                 server: Off
                 yours:  On
```

In the full-screen interface (`--tui`), rows that differ have an arrow: **Right** opens the settings that differ
(server value, then yours) under the row, **Left** closes them, **+** / **-** open / close all,
**Space** ticks. In the text version, type `d` to see all differences. `-n -v` prints them in a
dry run.

Every menu has **H - Help Menu** at the bottom (`h` in the text version): what the screen is
for, and its keys.

The mods' client/server tags are looked up once and cached. To look them up again (after a mod
changes its tags), use **Edit → Refresh mod tags** or `--refresh-tags`.

Configs are never deleted on the server, and a replaced one goes to the backups. With `-y`, only
new configs are copied.

## Edit

Highlight a profile in the main menu and press **E** (`1e` for profile 1 in the text version) to change what you've set up for it. **Back** (`b` in the text version) returns to
the previous menu from any step, including halfway through a server setup, without saving anything.

| Choice | What it does |
|---|---|
| Change a profile's server | Runs the first-time setup again for that profile (same as `--setup`) |
| Server-managed files | Tick a file to leave the server's copy alone, untick it to sync over it |
| Forget a profile's server | Removes the setup; the next sync asks again |
| Edit the config file | Opens it in `$EDITOR` (nano or vi if unset) and reloads it |
| Show file locations | Same as `--config` |
| Refresh mod tags | Looks up every mod's client/server tags on Thunderstore again (same as `--refresh-tags`) |
| Default interface | `cli` (text) or `tui` (full screen) when started without `--cli` / `--tui` |

## Options

Every option works in both interfaces.

| Command | What it does |
|---|---|
| `game-mod-sync --tui` | Full-screen interface for this run |
| `game-mod-sync --cli` | Text interface for this run |
| `game-mod-sync -n` | Preview only, change nothing |
| `game-mod-sync -g valheim -p MyProfile` | Skip the menu |
| `game-mod-sync -g valheim -p MyProfile -y` | No questions |
| `game-mod-sync -v` | List every file |
| `game-mod-sync --setup` | Point a profile at a different server folder |
| `game-mod-sync --refresh-tags` | Look up the mods' client/server tags again |
| `game-mod-sync --list` | Show where each profile syncs to |
| `game-mod-sync --games` | Show detected games and what gets synced |
| `game-mod-sync --config` | Show where settings are stored |

## Server types

- [AMP](plugins/backends/amp.md)
- [Other (SSH)](plugins/backends/ssh.md)

## Profile sources 

- [Gale](plugins/sources/gale.md)

## Mod loaders
- [BepInEx](plugins/loaders/bepinex.md)

## Good to know

- Only changed files are sent: files are compared by content, not by date.
- Replaced or removed files are kept on the server in `<game folder>/.mod-sync-backups/` (last 5).
- Server-only files are never touched. For Valheim, that's `MMHOOK/` and `Valheim.DisplayBepInExInfo.dll`.
- If the server installs a mod itself (for example, AMP's "Install ValheimPlus" option), you're asked once
  whether to leave it alone. Enter means yes. Change it later with **Edit → Server-managed files**.
- Settings are saved per computer in `~/.config/game-mod-sync/`.
- Your own plugins or overrides go in `~/.config/game-mod-sync/plugins/{sources,backends,games}/`.
  A file with the same name as one in the repo replaces it.

## Tests

Every plugin has its own test file: `tests/plugins/<kind>/<name>.sh` (sources, loaders, server
types, games), plus an end-to-end sync in `tests/core/sync.sh`. They run against a fake server
(a local folder reached through a stand-in `ssh`, with real `rsync`), so nothing needs a network.

```bash
tests/run.sh            # everything
tests/run.sh amp gale   # only these files
```

`.forgejo/workflows/tests.yml` runs them (and `shellcheck`) on every push and pull request.
`main` is protected: changes come in through pull requests, and these checks must pass before
one is merged. The badge panel at the top of this page (`docs/badges/panel.svg`, made from the
single badges next to it) is generated; after changing
tests, plugins or `VERSION`, run `tests/run.sh --badges docs/badges` and commit them (CI checks
they match). A new plugin gets a test file next to the
others, with a `# badge: KIND: NAME` line and functions named `t_...`; it appears in its group's badge
above by itself.

For how it works and how to add games or server types, see [docs/DESIGN.md](docs/DESIGN.md).
