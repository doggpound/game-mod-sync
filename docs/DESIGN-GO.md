# Go version (in progress)

game-mod-sync is being rewritten in Go. The goal: **one file to download and run, on Windows,
Linux and macOS**, with nothing else to install. No WSL, rsync, perl or whiptail on the computer
that runs it, and nothing new on the server beyond SSH and standard Linux tools.

The bash version stays in the repo and keeps working until the Go version does everything it
does. Then the bash files are removed and the Go version is released as v1.0.0. Until then, this
page describes the plan; [DESIGN.md](DESIGN.md) describes what runs today, and is the reference
for behaviour: the Go version keeps its rules (content comparison, backups, server-managed files,
config handling, world download) unless this page says otherwise.

## What changes for people using it

| | Bash (today) | Go |
|---|---|---|
| Install | WSL on Windows, then rsync, git, whiptail, a clone and a link | Download `game-mod-sync.exe` (or the Linux/macOS file) and run it |
| Start | `game-mod-sync` in a WSL terminal | Double-click it, or `game-mod-sync` in any terminal |
| SSH keys | `ssh-copy-id` by hand | Asked once for the password; the program makes and installs a key |
| Interfaces | text (`--cli`) and whiptail (`--tui`) | text and full screen (Bubble Tea), same options |
| Settings | `~/.config/game-mod-sync/config` (shell) and `targets.tsv` | Same folder on Linux; `%APPDATA%\game-mod-sync` on Windows; `config.toml` and `targets.toml`. The bash files are read the first time and left in place for bash; profiles set up with bash later are picked up too |
| Your own plugins | Any `.sh` source, loader, server type or game | Game files (`plugins/games/<id>.toml` in the config folder) as before, now TOML. Sources, loaders, server types and world formats are built in; "Other (SSH)" still covers any server |

## Layout

```
go.mod                        module github.com/doggpound/game-mod-sync
cmd/game-mod-sync/            main: options, picks the interface, runs the app
internal/app/                 the flow: menus' content, setup, preview, review, apply
internal/ui/                  the UI interface the app talks to (like the bash core's
  text/                         ui_* functions): plain prompts, works in scripts
  tui/                          Bubble Tea screens
internal/remote/              SSH connection, SFTP, running commands on the server
internal/transfer/            compare by content and apply: the rsync replacement
internal/state/               config and targets (TOML), migration from the bash files
internal/sources/gale/        Gale's data folder and profiles
internal/loaders/bepinex/     BepInEx folders and config owners (DLL attribute reader)
internal/backends/amp|ssh/    finding game folders, server-managed files, save folders
internal/games/               game settings: built-in TOML files + your overrides
internal/configs/             config comparison, client/server tags, Thunderstore cache
internal/worlds/              world download; valheim/ and files/ formats
internal/testserver/          an SSH + SFTP server on a temp folder, for tests
```

Each kind of plugin is a Go interface with one implementation per file, registered by name, so
adding a server type or a world format still means adding one file and not changing the core.

## Replacing rsync

rsync did three jobs: find what differs by content, copy only that, and keep what it replaces.
The Go version does them over one SSH connection, and works with **SFTP alone**: most game hosts
give out SFTP details (address, port, user name, password) and don't let you run commands.

1. **Hash the server's side.** When the login can run commands, one command per sync folder:
   `find . -type f -print0 | xargs -0 -r sha256sum -z` (GNU coreutils, on every Linux server).
   When it can't (SFTP only), or `sha256sum` is missing, the files are listed over SFTP and read
   to hash them, except the ones the **manifest** already knows: `.mod-sync-backups/manifest.json`
   on the server records each file's hash with the size and modification time it had. A file with
   the same size and time isn't read again; anything changed by something else gets a new time and
   is read once. So only the first SFTP-only sync reads every file. The manifest sits in the
   backups folder, outside the synced folders, and is shared by everyone who syncs to that server.
2. **Hash this computer's side** in Go and compare by path and hash. The excludes keep rsync's
   pattern rules that the tool uses: `/anchored`, `dir/`, `*`, `**`.
3. **Plan**: new, changed and deleted files and folders, the same per-mod summary as today.
4. **Apply** over SFTP. A changed or deleted file is first **renamed** into
   `<game root>/.mod-sync-backups/<time>/<sync folder>/...` (a rename on the same disk: instant).
   New content is written to a temporary name next to the target and renamed into place, so a
   broken connection never leaves a half-written mod file. Permissions 644 / 755 as before.

Downloads (world download) are the same steps in the other direction, with the save-stamp check
around them.

## SSH

`golang.org/x/crypto/ssh` with `pkg/sftp`. Whether a login can run commands is checked once per
connection (`true`); everything else works either way. Logging in tries, in order: the SSH agent (Linux
`SSH_AUTH_SOCK`; on Windows the OpenSSH agent pipe, which password managers' agents also use),
keys in `~/.ssh` and the program's own key, then a password. Settings from `~/.ssh/config` (host
aliases, port, user, key) are used when present. An unknown server shows its fingerprint and asks
once, then it's saved to `known_hosts`. Like the `ssh` command, it only asks the server for the
kinds of key already saved for it (a server usually has an ed25519, an ECDSA and an RSA key, and
`known_hosts` often holds just one); otherwise a known server would look like it had changed. When a password works, the program offers to install its
own key, so later runs don't ask.

## Interfaces

The app never prints or reads directly; it calls a UI interface (`internal/ui`): `Choose` (a menu
with row keys such as E and D), `Check` (a checklist with details per row), `Ask`, `Password`,
`Confirm`, `View`, and messages (`Info` for what's happening, `Say`, `OK`, `Warn`). The text UI
(`internal/ui/text`) is numbered menus and prompts, and works without a terminal.

The full-screen UI (`internal/ui/tui`, Bubble Tea v1) is **one screen for the whole session**: each
question becomes a view on it, so there's no flicker between dialogs (the bash version needed
terminfo tricks for that). The app runs in its own goroutine and each `ui.UI` call sends a request
to the screen and waits for the answer. The messages since the last question are shown above it
(PgUp/PgDn scroll them when they don't fit), and while the app works a spinner shows its last
`Info`. Menus: arrows, Enter, the row keys, 1-9, `?` help, Esc back (Exit on the main menu).
Checklists: Space ticks, Right/Left open and close a row's details (the full differences), `+`/`-`
all, `a`/`n` tick all or none. Ctrl+C quits from anywhere (every waiting question returns "exit").
When it ends, the last messages are printed to the normal terminal.

Which one: `--tui` or `--cli` for one run, else `default_ui` in the settings (Edit → Default
interface); the full screen only on a terminal, so scripts always get text.

## Builds and releases

`CGO_ENABLED=0 go build` for windows/amd64, windows/arm64, linux/amd64, linux/arm64, darwin/amd64
and darwin/arm64, all from the existing Linux runner. Each release gets a `.zip` (Windows) or
`.tar.gz` per platform plus `.sha256` files, on Forgejo and the GitHub mirror, through the existing
release workflow. The version is set at build time (`-ldflags "-X main.version=..."`). The program
can check the latest release and say when a newer one exists.

Windows shows "Windows protected your PC" for programs that aren't code-signed; signing can come
later. macOS needs right-click → Open (or `xattr -d com.apple.quarantine`) for the same reason.

A host's SFTP details map one to one onto the setup questions: server type, address, port, user
name, then the folder. A link like `sftp://user@host:2022/folder` can be pasted as the address
and fills in the rest. The game folder is found by searching for the game's marker folder (e.g.
`BepInEx/plugins`): with `find` when the login runs commands (AMP: below the instances folder),
else by walking folders over SFTP from where the login starts (at most 6 levels, 8 for AMP, and
5000 folders). The folders that look like this game come first, and "Type the folder" is always
offered. The connection made for setup is used for the sync right after, so nothing is asked twice.

Passwords: the saved one is tried first, then the person is asked (up to 3 tries, so a password
the host changed is simply asked again), and after a login with a typed password the program
offers to save it. Hosts that only allow SFTP usually don't allow installing a key, so the
password goes into the system's credential store (Windows Credential Manager, macOS Keychain,
Linux Secret Service), never into the settings files. Where there's no store (WSL often has
none), it's asked each time. Edit → "Forget the saved password" removes it.

A question that gets no answer because the input ended (scripts) takes its default, which is
always the safe choice (e.g. leave a server-installed file alone; don't apply).

## Tests

Go's own tests, run in CI next to the bash ones until the bash version is gone. The end-to-end
tests run against `internal/testserver`: a real SSH and SFTP server on a temporary folder inside
the test, so they need no network, no `ssh` program and no fake shell, and also run on Windows.
The bash tests' scenarios (sync, backups, excludes, server-managed files, configs, world download)
are ported one by one.

## Config files

Same rules as the bash version (DESIGN.md, "Configs"): each config folder is compared by content
like the mods (on SFTP-only hosts through the manifest), files are offered one by one and never
deleted on the server, new ones ticked, differing ones unticked, and `-y` copies only new ones.
The server's copies of differing files are read over SFTP to show what differs: `.cfg` files per
`[Section] Key`, other files as a line diff (`internal/configs`, a small longest-common-subsequence
diff, so nothing outside Go is needed). Files that differ only in comments, blank lines, spacing
or line endings are dropped before anything else, so the "not shown" count of files from mods
that aren't in the profile only counts files that really differ.

Which mod a file belongs to comes from the `[BepInPlugin]` declarations read out of the profile's
DLLs (`internal/loaders/bepinex.go`, the perl reader's rules), then the folder-name fallbacks.
Client/server tags come from the bash version's cache file (shared), then Gale's
`thunderstore_cache.json`, then Thunderstore (6 lookups at a time, 10 s each). The mod changes and
the picked config files are reviewed together and applied with one backup timestamp.

## World download

D on a profile, or `-p PROFILE --download-world` (`--world NAME` picks one; `-y` takes the one the
server runs; `-n` stops before downloading): a copy of a world from the profile's server is put in
this computer's saves for the game, to try mod updates on it before syncing them. Nothing on the
server is written, and it works on SFTP-only logins (everything is read over SFTP).

- **Formats** (`internal/worlds`): `valheim` (newer Valheim's chunked world folders; the game's
  own `*_backup*` worlds listed last, newest first) and `files` (`<world>.<ext>`, any game; the
  game file lists the extensions). A format lists worlds, names the paths a world is made of, and
  gives a save stamp. Both read through one small file-system interface, so the same code reads
  the server (SFTP) and this computer.
- **Server folder**: one typed in before for the profile, else AMP's `-savedir` (from the
  instance's settings, which also name the world it runs), else the game's usual folder under the
  login's start; when none exists it asks and remembers.
- **This computer's folder**: picked before, else under the Windows user folder (natively, or seen
  from WSL) and under `$HOME`; asks once when there are several.
- **A consistent copy while the server runs**: the stamp is read before and after the copy and
  from the copy itself; when they differ, a save happened and it copies again (3 tries; files that
  vanished mid-copy count as a save). Valheim's stamp is the newest save number with its `.ok`
  and all three `_main` files. Copies go to a cache folder first and only files whose size or time
  changed are fetched again, so a second download only fetches what the server saved since.
- **Install**: this computer's copy is moved to `game-mod-sync-backups/<time>/` next to the worlds
  folder (`keep_backups` kept), then the copy is put in place; on failure the old one is moved back.

## Adding a game

Games are TOML files, not code. The built-in ones are `internal/games/builtin/*.toml` (packed
into the program when it's built); `_default.toml` describes every setting. A file is read on top
of the mod loader's defaults and `_default.toml`, so it only lists what differs:

```toml
# internal/games/builtin/lethal-company.toml (named after Gale's folder for the game)
name = "Lethal Company"
server_match = "lethal|1966720"
after_sync = "Restart the server in AMP."
```

- **For yourself, right away:** put the file in the config folder's `plugins/games/` folder.
  It's used the next time the program starts, nothing to build. It can also change a built-in
  game's settings (only the ones it lists).
- **For everyone:** a pull request that adds the file to `internal/games/builtin/`. A test loads
  every built-in file, so a mistake fails the checks instead of reaching users. It ships in the
  next release.

## Building it yourself

`go build -o dist/game-mod-sync ./cmd/game-mod-sync` (add `.exe` on Windows). Always give `-o`: a
plain `go build` in the repo's root would write the program over the bash `game-mod-sync` file
while both versions live side by side. `dist/` is ignored by git. `go test ./...` runs the tests.

## Phases

Each phase is its own pull request and leaves the bash version untouched.

1. **Core transfer:** module, SSH, the testserver, hashing (commands or SFTP only, with the
   manifest), compare, apply with backups; a bare `game-mod-sync sync` for one folder. CI builds
   all six platforms.
2. **Profiles and servers**, in two pull requests:
   - 2a: settings and migration from the bash files, Gale, BepInEx, game files, the text interface
     with the main menu and the bash version's options, syncing a set-up profile including
     server-managed files (AMP's settings are read over SFTP).
   - 2b: first-time setup (server type, address and port, SSH or SFTP-only login, the password in
     the system's credential store), finding the game folder (AMP over SSH or SFTP; any server by
     typing it), Edit menu basics.
3. **Configs:** comparison, owners from DLLs, client/server tags (Thunderstore), the checklist.
4. **Full screen:** Bubble Tea screens for everything above, Edit menu (incl. Default interface), help.
5. **World download** (Valheim chunked worlds and `<world>.<ext>` files; AMP's save folder).
6. **Release v1.0.0:** packaging, update check, README and screenshots for the Go version; the bash
   files are removed. With the bash launcher gone, `cmd/game-mod-sync/main.go` moves to the
   repository's root, so a plain `go build` makes `game-mod-sync` and
   `go install github.com/doggpound/game-mod-sync@latest` works. `internal/` stays: it's Go's
   place for code only this program uses, split by job so each part stays small and tested.
