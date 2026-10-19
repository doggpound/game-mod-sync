# Server type: `ssh` (any SSH server)

The catch-all server type. Use it for any server you can SSH into where you know the game folder:
LinuxGSM, a plain SteamCMD install, a Docker bind mount, a Pterodactyl volume and so on. It works
until that platform gets its own auto-discovering server type.

## Usage

On the first sync of a profile, pick **Other (SSH, enter the game folder)**. Then:

1. **Host** and **SSH user**. There is no default user, so enter the one that owns the game files.
2. **Game folder:** the folder that contains the game's marker (e.g. the one with `BepInEx/` in it).
   - `~/serverfiles` and other paths relative to the SSH user's home work.
   - Absolute paths work too, e.g. `/var/lib/pterodactyl/volumes/<uuid>`.

The folder must already exist. If the marker folder inside it doesn't exist yet (say, `BepInEx/plugins`),
you get a warning and it's created on the first sync.

Because you typed the folder, you aren't asked to pick it again from a one-item list.

Requirements on the server: SSH key login for that user, `rsync`, and write access to the game folder.

## Design

`ssh_candidates` asks for the folder, then resolves it on the server:

```
cd && cd -- '<folder>' && pwd
```

This turns `~` and relative paths into an absolute path, and fails clearly if the folder doesn't
exist. A leading `~/` is stripped before quoting, so it isn't left as a literal `~`. The resolved path
is stored in `targets.tsv`, so later runs don't depend on the home directory or the current
directory.

`ssh_manual` is defined (it does nothing). The core uses it as a flag: the single candidate
came from the user, so it's accepted without a menu and isn't marked `(other game?)`.

There's no `ssh_managed`: a plain server has no settings to inspect. Server-side files you want left
alone go in the game's `EXCLUDES` or in `EXTRA_EXCLUDES`.

## Functions

| Function | Returns |
|---|---|
| `ssh_label` | `Other (SSH, enter the game folder)` |
| `ssh_default_user` | empty (always ask) |
| `ssh_manual` | nothing; its existence is the flag |
| `ssh_candidates TARGET MARKER` | one `path<TAB>path` line for the folder you typed |
