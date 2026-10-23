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
| Settings | `~/.config/game-mod-sync/config` (shell) and `targets.tsv` | Same folder on Linux; `%APPDATA%\game-mod-sync` on Windows; TOML files. The old files are read once and converted |
| Your own plugins | Any `.sh` source, loader, server type or game | Game files (`games/<id>.toml`) as before. Sources, loaders, server types and world formats are built in; "Other (SSH)" still covers any server |

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

The app never prints or reads directly; it calls a UI interface with the same operations the bash
version has (`Choose`, `Checklist` with details per row, `Ask`, `Confirm`, `Wait`, `Progress`,
`View`, messages). The text UI is plain prompts with colours (Lip Gloss) and works without a
terminal (`-y`). The full-screen UI is Bubble Tea: menus with row keys (E, D), checklists whose
rows open to show differences, help overlays and progress bars.

## Builds and releases

`CGO_ENABLED=0 go build` for windows/amd64, windows/arm64, linux/amd64, linux/arm64, darwin/amd64
and darwin/arm64, all from the existing Linux runner. Each release gets a `.zip` (Windows) or
`.tar.gz` per platform plus `.sha256` files, on Forgejo and the GitHub mirror, through the existing
release workflow. The version is set at build time (`-ldflags "-X main.version=..."`). The program
can check the latest release and say when a newer one exists.

Windows shows "Windows protected your PC" for programs that aren't code-signed; signing can come
later. macOS needs right-click → Open (or `xattr -d com.apple.quarantine`) for the same reason.

A host's SFTP details map one to one onto the setup questions: address, port, user name,
password, and the folder to sync to (often `/` is the server's own folder). The README will have a
short "Use your host's SFTP details" section with examples for common panels. Hosts that only
allow SFTP usually don't allow installing a key, so the password is kept in the system's
credential store (Windows Credential Manager, macOS Keychain, Linux Secret Service), never in the
settings file.

## Tests

Go's own tests, run in CI next to the bash ones until the bash version is gone. The end-to-end
tests run against `internal/testserver`: a real SSH and SFTP server on a temporary folder inside
the test, so they need no network, no `ssh` program and no fake shell, and also run on Windows.
The bash tests' scenarios (sync, backups, excludes, server-managed files, configs, world download)
are ported one by one.

## Building it yourself

`go build -o dist/game-mod-sync ./cmd/game-mod-sync` (add `.exe` on Windows). Always give `-o`: a
plain `go build` in the repo's root would write the program over the bash `game-mod-sync` file
while both versions live side by side. `dist/` is ignored by git. `go test ./...` runs the tests.

## Phases

Each phase is its own pull request and leaves the bash version untouched.

1. **Core transfer:** module, SSH, the testserver, hashing (commands or SFTP only, with the
   manifest), compare, apply with backups; a bare `game-mod-sync sync` for one folder. CI builds
   all six platforms.
2. **Profiles and servers:** Gale, BepInEx, game files, AMP and SSH server types, first-time setup,
   targets, server-managed files, the text interface for a full mod sync, settings migration.
3. **Configs:** comparison, owners from DLLs, client/server tags (Thunderstore), the checklist.
4. **Full screen:** Bubble Tea screens for everything above, Edit menu, help.
5. **World download.**
6. **Release v1.0.0:** packaging, update check, README and screenshots for the Go version; the bash
   files are removed.
