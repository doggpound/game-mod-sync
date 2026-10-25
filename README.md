<p align="center"><img src="docs/logo.svg" width="128" alt="game-mod-sync logo"></p>

<h1 align="center">game-mod-sync</h1>

<p align="center">
  <img src="docs/badges/panel.svg" alt="Status: version and tests. Core and plugin test results.">
</p>

> [!WARNING]
> **Made with AI.** This project was written by Claude, an AI model made by Anthropic, with a
> human deciding what it should do, reviewing the changes and testing it on a real game
> server. Read the code before you point it at yours. It keeps backups of everything it replaces
> or removes on the server, but use it at your own risk.

Send the mods and configs of a [Gale](https://github.com/Kesomannen/gale) profile to your game
server, and copy the server's world back to try mod updates on. One program for Windows, macOS and
Linux, with nothing else to install. It works with the SFTP details a game host gives out, with
SSH logins, and finds the game folders of [CubeCoders AMP](https://cubecoders.com/AMP) instances.

<p align="center"><img src="docs/screenshots/tui-configs.svg" width="760" alt="Full-screen interface: the config files of a profile compared with the server's, one opened to show the settings that differ (the server's value, then yours)"></p>

<details>
<summary>More screenshots</summary>

**Main menu** (`--tui`)

<img src="docs/screenshots/tui-menu.svg" width="760" alt="The main menu: a profile and the server it syncs to, with the keys under the list">

**Review before anything is applied**

<img src="docs/screenshots/tui-review.svg" width="760" alt="The review: new, updated and removed mods, the config file to copy, and a Yes / No question">

**Download a world**

<img src="docs/screenshots/tui-worlds.svg" width="760" alt="The worlds on the server, the one the server runs first">

**Text interface** (`--cli`)

<img src="docs/screenshots/cli-configs.svg" width="760" alt="Text interface: the compare and the config files checklist">

<img src="docs/screenshots/cli-review.svg" width="760" alt="Text interface: the review and the apply question">

</details>

## Install

Download the file for your computer from
[Releases](https://github.com/doggpound/game-mod-sync/releases/latest), extract it, and run
`game-mod-sync`. Nothing else is needed.

| Computer | File | Run |
|---|---|---|
| Windows | `game-mod-sync-X.Y.Z-windows-amd64.zip` (`-arm64` for ARM PCs) | double-click `game-mod-sync.exe`, or run it in Windows Terminal |
| macOS | `game-mod-sync-X.Y.Z-macos-arm64.tar.gz` (Apple silicon) or `-macos-amd64` (Intel) | `./game-mod-sync` in Terminal |
| Linux | `game-mod-sync-X.Y.Z-linux-amd64.tar.gz` (or `-linux-arm64`) | `./game-mod-sync` |

- **Windows** may say "Windows protected your PC" the first time, because the program isn't
  code-signed: click **More info → Run anyway**.
- **macOS** may refuse to open it for the same reason: right-click it → **Open**, or run
  `xattr -d com.apple.quarantine game-mod-sync` once.
- `SHA256SUMS` on the release has every file's checksum.

With Go installed you can also build it: `go install github.com/doggpound/game-mod-sync@latest`.

## First run: connect to your server

Start `game-mod-sync` (full screen; `--cli` for plain text). It lists your Gale profiles; pick
one and answer the setup questions once:

1. **Server type:** *CubeCoders AMP* if the server runs AMP, otherwise *Other: SSH or SFTP details
   from your host*.
2. **Server address, port, user name**, and the **password** when it's asked. These are exactly
   what a game host's "SFTP details" say. You can also paste an `sftp://user@host:port/folder`
   link as the address; it fills in the rest.
3. **The game folder:** it searches the server for the game's mod folder (e.g. `BepInEx/plugins`)
   and lists what it finds, the ones that look like this game first. Pick one, or *Type the
   folder*.

After a login with a typed password it offers to save the password in your system's password
store (Windows Credential Manager, macOS Keychain); it's never written to a settings file. A host
that lets you install an SSH key, an SSH agent (1Password, Windows' OpenSSH agent) or keys in
`~/.ssh` work too, and `~/.ssh/config` is used like the `ssh` command uses it.

## Use

Pick a profile in the main menu (Enter) to sync it:

1. It compares the profile's mods with the server's **by content** and shows what's new, updated
   and removed, per mod.
2. **Config files** that are new or differ are listed with the settings that differ. New ones are
   ticked; differing ones aren't (copying replaces the server's settings with yours). Files of
   mods that only run on players' machines (client-side, from Thunderstore's tags) are never
   copied.
3. **Review**, then Yes to apply. Replaced and deleted files are kept on the server in
   `<game folder>/.mod-sync-backups/<time>/` (the last 5).
4. Restart the server so it loads the new mods.

Keys in the full screen: arrows and Enter, **E** to edit a profile, **D** to download a world,
**?** for help, Esc to go back. In checklists Space ticks and → opens a file's differences. In the
text interface the same keys are typed after a number (`1e`, `1d`), `d` shows the differences.

### Download a world

**D** on a profile copies a world from its server into this computer's saves for the game, to
try mod updates on a copy before syncing them. The world the server runs is listed first, the
game's own backups last. Your copy of that world is moved to `game-mod-sync-backups` next to the
worlds folder first. The server keeps running: if it saves during the copy, the copy is made
again. Nothing on the server is changed. Close the game first. In Valheim your character is
saved on your computer, so play the copy with a spare character.

### Edit

**E** on a profile: change or forget its server, forget its saved password, show where the
settings are, look the mods' client/server tags up again, and choose the interface it starts in.

## Options

| Command | What it does |
|---|---|
| `game-mod-sync --tui` / `--cli` | Full screen / text for this run (the default, full screen, can be changed in Edit) |
| `game-mod-sync -p MyProfile` | Sync that profile, without the menu (`-g valheim` if the name is in several games) |
| `game-mod-sync -p MyProfile -y` | No questions: new configs are copied, differing ones aren't |
| `game-mod-sync -n` | Dry run: show what would change, change nothing |
| `game-mod-sync -v` | List every file, and every config difference in full |
| `game-mod-sync --setup` | Set up a profile's server again |
| `game-mod-sync -p MyProfile --download-world` | Download the world the server runs (`--world NAME` for another) |
| `game-mod-sync --refresh-tags` | Look the mods' client/server tags up on Thunderstore again |
| `game-mod-sync --list` | Where each profile syncs to |
| `game-mod-sync --games` | The games found, and what's synced for each |
| `game-mod-sync --config` | Where the settings are |
| `game-mod-sync sync --host H --user U --local DIR --root DIR` | Sync any folders, without profiles (see `sync -h`) |

## Settings

In `%APPDATA%\game-mod-sync` on Windows, `~/.config/game-mod-sync` elsewhere: `config.toml`
(e.g. `keep_backups`, `default_ui`, `update_check = false`) and `targets.toml` (where each
profile syncs to). Your own game files go in its `plugins/games/` folder.

Coming from 0.x (the shell-script version)? Its settings in `~/.config/game-mod-sync` are read and
converted the first time; nothing needs setting up again.

## Supported

| | |
|---|---|
| Mod managers | [Gale](https://github.com/Kesomannen/gale) |
| Mod loaders | BepInEx |
| Server types | CubeCoders AMP; any SSH or SFTP server ("Other") |
| Games | Valheim (with world download); any BepInEx game works without a game file |

Adding a game is a small settings file, no code; mod managers, mod loaders, server types and world
formats are plugins. See [plugins/README.md](plugins/README.md).

## Development

- How it works: [docs/DESIGN.md](docs/DESIGN.md). Adding plugins: [plugins/README.md](plugins/README.md).
- Build: `go build .` Test: `go test ./...` (the tests start their own SSH/SFTP server; no network needed).
- `main` is protected: changes come in through pull requests, and the checks
  (`.forgejo/workflows/tests.yml`) must pass. After changing tests or `VERSION`, regenerate the
  badges (`go test -json ./... > results.json && python3 docs/badges/make.py results.json docs/badges`);
  after changing anything visible, the screenshots (`docs/screenshots/make.sh`).
- Releases: raising the version in `VERSION` and merging publishes it, with the program for every
  platform, on Forgejo and the GitHub mirror (`.forgejo/release.sh`).
