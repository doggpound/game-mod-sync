# Design

game-mod-sync is one Go program, built for Windows, macOS and Linux with nothing else to install.
It reaches a server over one SSH connection, and everything it does works on **SFTP alone**, since
most game hosts give out SFTP details and don't let you run commands. (Versions 0.x were shell
scripts around rsync; their settings are still read, see [Settings](#settings).)

## Layout

```
main.go                       options, picks the interface, runs the app
plugins/                      everything particular to a mod manager, mod loader,
  sources/ loaders/ servers/  server type, world format or game: one folder each,
  worlds/ games/ all/         registered in plugins/all (see plugins/README.md)
internal/
  plugin/       the plugin interfaces and the registry
  app/          the flow: menus' content, setup, sync, configs step, world download, Edit
  ui/           the interface the app talks to; text/ and tui/ implement it
  remote/       SSH connection + SFTP, logins, known_hosts, ~/.ssh/config, finding game folders
  transfer/     compare by content and apply (the rsync replacement), the manifest
  configs/      config comparison, which mod a file is from, client/server tags
  worlds/       world download's shared part: reading worlds through an FS, Mirror
  games/        game settings: loader defaults + game files + the user's own files
  state/        config.toml and targets.toml; reading 0.x settings
  secrets/      passwords in the system's credential store
  update/       "a newer release exists"
  testserver/   a real SSH + SFTP server for the tests (and the screenshot demo)
docs/           this page, badges (make.py), screenshots (make.sh, demo/)
```

The program never names a plugin: it asks the registry (`internal/plugin`) for the mod managers,
loaders, server types and world formats, and each plugin registers itself in an `init` function.
`plugins/all` imports them all; a test fails if a plugin folder isn't in it. Optional abilities are
small extra interfaces a plugin implements only if it has them (`ModTagger`, `ConfigOwner`,
`ManagedFiles`, `SaveFinder`, `Fallback`). How to write one: [plugins/README.md](../plugins/README.md).

## Comparing by content

Each computer's mod manager downloads and unpacks its own copy of each mod, so file times differ
between computers; comparing by time would send everything again from another computer. Files are
compared by **SHA-256 of their contents**:

- **The server's side**: when the login can run commands (checked once per connection with
  `true`), one command per folder: `find . -type f -print0 | xargs -0 -r sha256sum -z`. Otherwise
  (SFTP only), the files are listed over SFTP and read to hash them, except those the **manifest**
  already knows: `.mod-sync-backups/manifest.json` on the server records each file's hash with the
  size and time it had. A file with the same size and time isn't read again; one changed by
  anything else gets a new time and is read once. So only the first SFTP-only sync reads every
  file. The manifest is shared by everyone who syncs to that server.
- **This computer's side**: hashed locally.
- **Excludes** follow rsync's pattern rules as the game files use them: `MMHOOK/` (a folder, at any
  depth), `/ValheimPlus.dll` (only at the top), `*.dat`, `**`. Excluded paths are never sent and
  never deleted, nor the folders holding them.

## Safety rules

- Every run shows what will change and asks before changing anything (`-y` doesn't ask; `-n`
  changes nothing).
- Only folders the mod manager fully owns are mirrored (`SyncDirs`, with deletions). Config
  folders are never mirrored: files are offered one by one and nothing is deleted there, because
  servers often have their own settings on purpose.
- A sync folder that's empty in the profile is skipped, and if nothing is left the run is refused:
  syncing it would wipe the server's mods.
- Replaced or deleted files are first **moved** (a rename on the same disk, instant) to
  `<game folder>/.mod-sync-backups/<time>/<folder>/`, outside the folders the mod loader reads.
  The newest `keep_backups` (5) are kept.
- New content is written to a temporary name next to its target and renamed into place, so a
  dropped connection never leaves half a file. Permissions 644 / 755.
- A question that gets no answer because the input ended (scripts) takes its default, which is
  always the safe one (don't apply; leave a server-installed file alone).

## Server-managed files

Some server types install mods themselves: AMP's Valheim template has "Install ValheimPlus", which
unpacks `ValheimPlus.dll` into `BepInEx/plugins` on every update. A plain mirror would delete it on
every sync. A game file names such files per server type (`[managed] amp =
["valheim_plus_install=ValheimPlus.dll"]`); the server type (`ManagedFiles`) says which are on
(AMP: `App.AppSettings` in the instance's `.kvp`, read over SFTP). The first time a file is
reported, the person is asked whether to leave it alone (default yes); the answer is kept in the
target (`kept`: `+pattern` leave alone, `-pattern` sync over), and kept patterns are excluded
anchored to the top of the sync folder, so a copy inside a mod's own folder still syncs.

## Config files

Each config folder is compared like the mods; files that are new or differ are offered in a
checklist, never deleted. To show what differs, the server's copies of differing files are read
over SFTP: `.cfg` files are compared per `[Section] Key = value` (`Lock Configuration: Off -> On`),
other files as a line diff (a small longest-common-subsequence diff). Files that differ only in
comments, blank lines, spacing or line endings are dropped, as are files from mods no longer in
the profile (`-v` shows them).

- **Which mod**: a loader that implements `ConfigOwner` maps plugin ids to mod folders. BepInEx
  reads each DLL's `[BepInPlugin(id, name, version)]` attribute blob (01 00, then three
  length-prefixed strings): reading the declaration, not any mention of the id, avoids matching
  mods that only depend on it. A file belongs to the mod with the longest id it starts with; then
  a mod folder with the same name (ignoring case and punctuation, also with parts cut off the end),
  then a unique mod whose name is the file name's last part.
- **Client or server**: Thunderstore categories (game file: `tag_client`, `tag_server`), from the
  tag cache (`thunderstore-tags.tsv` in the cache folder; a package Thunderstore doesn't have is
  cached with no tags), then mod managers that know them offline (`ModTagger`: Gale's
  `thunderstore_cache.json`), then Thunderstore (6 at a time, 10 s each; a network error caches
  nothing). Client-only mods' files are listed but never copied.
- **Defaults**: new files ticked (if their mod is known), differing ones not; `-y` copies new ones.

The mod changes and the picked config files are reviewed together and applied with one backup time.

## World download

D on a profile (or `--download-world`) copies a world from the server into this computer's saves.
Nothing on the server is written.

- **Formats** (`plugin.WorldFormat`): Valheim's chunked world folders (`_main.<N>.fwl2/.db2/.chunks`,
  `_main.<N>.ok` written last, `*.chunk`; the game's own `*_backup*` worlds listed last, newest
  first) and `files` (`<world>.<ext>`). A format lists worlds, names a world's paths and gives a
  save stamp; it reads through `worlds.FS`, so the same code reads the server and this computer.
- **Folders**: on the server, one typed in before for the profile, else the server type's
  (`SaveFinder`: AMP's `-savedir` from the instance's settings, which also name the world it runs),
  else the game's usual folder under the login's start; when none exists it asks and remembers. On
  this computer, under the Windows user folder (natively or seen from WSL) or `$HOME`; it asks once
  when there are several.
- **A consistent copy while the server runs**: the stamp is read before and after the copy and from
  the copy. When they differ, a save happened: copy again (3 tries; files that vanished mid-copy
  count as a save). Valheim's stamp is the newest save number with its `.ok` and all three `_main`
  files; a save only removes the previous save's files after its `.ok` (worked out from a live
  server's folders, not documented). Copies go to a cache folder first and only files whose size
  or time changed are fetched again.
- **Install**: this computer's copy is moved to `game-mod-sync-backups/<time>/` next to the worlds
  folder, then the copy is put in place; if that fails, the old one is moved back.

## Logging in

`golang.org/x/crypto/ssh` with `pkg/sftp`. Logins try the SSH agent (Linux `SSH_AUTH_SOCK`; on
Windows the OpenSSH agent pipe, which password managers' agents also serve), keys from
`~/.ssh/config` and `~/.ssh`, then a password: the saved one first, then asking (3 tries, so a
password the host changed is simply asked again). After a login with a typed password the program
offers to save it in the system's credential store (`internal/secrets`; Windows Credential
Manager, macOS Keychain, Linux Secret Service); without one it asks each time. `~/.ssh/config`
can give a server's real address, port and keys, like the `ssh` command. An unknown server's
fingerprint is shown once and saved to `known_hosts`; a changed one is refused. Only the kinds of
host key already saved are asked for, as `ssh` does (a server usually has several and
`known_hosts` often holds one). Setup's connection is reused for the sync right after it.

## Interfaces

The app never prints or reads directly; it calls `ui.UI`: `Choose` (a menu with row keys such as E
and D), `Check` (a checklist with details per row), `Ask`, `Password`, `Confirm`, `View`, and
messages (`Info` for what's happening, `Say`, `OK`, `Warn`).

- **Text** (`internal/ui/text`): numbered menus and prompts; works without a terminal, so scripts
  get it.
- **Full screen** (`internal/ui/tui`, Bubble Tea): one screen for the whole session; each question
  is a view on it. The app runs in its own goroutine; each call sends a request to the screen and
  waits for the answer, and Ctrl+C makes every waiting call return "exit". The messages since the
  last question are shown above it in a box (PgUp/PgDn when they don't fit), a spinner shows the
  last `Info` while the app works, and the last messages are printed to the terminal at the end.

Full screen is the default; `--cli` / `--tui` choose for one run, `default_ui` in the settings
(Edit → Default interface) for every run. Without a terminal it's always text. On Windows, a
program started by double-click waits for Enter before its window closes.

## Settings

`config.toml` and `targets.toml` in `%APPDATA%\game-mod-sync` (Windows) or
`$XDG_CONFIG_HOME` / `~/.config/game-mod-sync`. `targets.toml` has one entry per profile: mod
manager, game, profile, server type, host, port, user, game folder, the answers about
server-managed files, and a worlds folder when one had to be typed. The first time, the 0.x
settings in the same folder (`config`, shell syntax, and `targets.tsv`) are converted (they're
left in place), and profiles set up with 0.x later are picked up too. Users' own game files go in
`plugins/games/` there.

## Builds and releases

`go build .` makes the program. Releases (`.forgejo/release.sh`, run by `release.yml` when a merge
raises the version in `VERSION`) build it with `CGO_ENABLED=0` for Windows, macOS and Linux on
amd64 and arm64, package each with the README (`.zip` for Windows, `.tar.gz` otherwise), add
`SHA256SUMS`, and publish them on Forgejo and the GitHub mirror. The version is set at build time
(`-X main.version=...`). At start the program asks the mirror for its latest release, at most once a
day with a short timeout, and says when a newer one exists (not for development builds;
`update_check = false` turns it off). Builds aren't code-signed, so Windows' SmartScreen and macOS'
Gatekeeper warn the first time.

## Tests

`go test ./...`. The end-to-end tests run against `internal/testserver`: a real SSH and SFTP server
on a temporary folder, in a normal and an SFTP-only mode, with a second host key when needed, so
they need no network and no `ssh` program. The full screen is tested by feeding keys to its model.
CI (`.forgejo/workflows/tests.yml`, on pull requests) checks formatting, `go vet`, the tests, that
every platform compiles, the badges and the screenshots. A Go cache (toolchain, modules, build
cache) is kept between runs, filled on `main` by `go-cache.yml`.

- **Badges**: `docs/badges/make.py` draws the README's panel from `go test -json`; CI makes them
  again and fails if the committed ones differ.
- **Screenshots**: `docs/screenshots/make.sh` builds the program and a made-up demo
  (`docs/screenshots/demo`: a Gale profile and an AMP-style test server shown as
  `game.example.com` through `~/.ssh/config`, with fixed dates and tags), drives both interfaces in
  `tmux` and turns the screens into SVGs (`ansi2svg.py`). Two runs give identical pictures; CI's
  are the reference, uploaded as the run's `screenshots` artifact.
