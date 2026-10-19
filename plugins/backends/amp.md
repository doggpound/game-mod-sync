# Server type: `amp` (CubeCoders AMP)

Finds game folders inside [CubeCoders AMP](https://cubecoders.com/AMP) instances and detects
add-ons that AMP installs itself.

## Usage

On the first sync of a profile, pick **CubeCoders AMP** as the server type. Then:

1. **Host:** the AMP server, e.g. `game.example.com`.
2. **SSH user:** defaults to `amp`, the user AMP runs as. It owns `~/.ampdata`.
3. **Instance:** a list of every instance that has the game's marker folder (e.g. `BepInEx/plugins`).
   Instances matching the game are listed first. Others are marked `(other game?)`.

```
   1) Valheim01         Valheim01/Valheim/896660
   2) Valheim02         Valheim02/Valheim/896660
   3) LethalCompany01   LethalCompany01/LethalCompany/1966720  (other game?)
```

Requirements on the server: SSH key login for the `amp` user (`ssh-copy-id amp@host`) and `rsync`.

After a sync, restart the instance in AMP so the server loads the new mods.

### Settings

| Setting (in `~/.config/game-mod-sync/config`) | Default | Purpose |
|---|---|---|
| `AMP_INSTANCES_DIR` | `.ampdata/instances` | Where instances live, relative to the SSH user's home |

### Files AMP installs itself

Some AMP game templates install mods on every update. For example, the Valheim template's
**Install ValheimPlus** option unpacks `ValheimPlus.dll` into `BepInEx/plugins/`. When a game file
lists such an option (`MANAGED_amp=...`) and it's on for the chosen instance, the sync shows:

```
warning: The server installs ValheimPlus.dll itself (AMP setting "valheim_plus_install" is on for Valheim01).
Leave the server's ValheimPlus.dll alone when syncing? [Y/n]
```

Enter keeps AMP's file. If your profile also contains the mod, you're warned that the server will
have two copies; the cleanest fix is to turn the AMP option off. `--setup` asks again.

## Design

**Layout.** AMP keeps each instance at `~/.ampdata/instances/<Instance>/`. The game itself sits
somewhere below that, and the path differs per template (Valheim: `Valheim/896660/`). So instead of
hard-coding paths per game, `amp_candidates` searches each instance (up to depth 8) for the game's
marker folder and treats the folder above it as the game root:

```
find ~/.ampdata/instances -mindepth 2 -maxdepth 8 -type d -path '*/BepInEx/plugins' \
     -not -path '*/.mod-sync-backups/*'
```

Backups that game-mod-sync leaves on the server are skipped, so they're never offered as instances.
The instance name (first folder under `instances/`) is used as the label and as the short name in
menus (`where_of` in `lib/core.sh`).

**Managed files.** `amp_managed` reads the instance's settings from `<instance>/*.kvp`, the line
`App.AppSettings={json}` (in practice `GenericModule.kvp`). For each `setting=pattern[,pattern]` in
the game's `MANAGED_amp`, if `"setting":"true"` (or `true`) appears, it prints
`pattern<TAB>reason`. The core asks once, stores the answer, and excludes kept patterns
anchored to the top of the sync folder. Anchoring means a same-named file inside a mod's own folder
still syncs.

Only a substring check is used on the JSON, so there's no `jq` dependency on the server.

**What it does not do.** It never talks to the AMP API or panel and never starts, stops or updates
instances. Everything is `find` and `grep` over SSH, plus rsync into the chosen folder.

## Functions

| Function | Returns |
|---|---|
| `amp_label` | `CubeCoders AMP` |
| `amp_default_user` | `amp` |
| `amp_candidates TARGET MARKER` | `label<TAB>absolute game root` per instance containing `MARKER` |
| `amp_managed TARGET ROOT SPECS` | `pattern<TAB>reason` per managed file whose AMP setting is on |

## Known limits

- Assumes the default AMP data folder. If AMP was installed with a different data path, set
  `AMP_INSTANCES_DIR`.
- Detecting managed files needs the setting name from the AMP template. Look for it in the
  instance's `GenericModule.kvp` (`App.AppSettings`) and add it to the game's `MANAGED_amp`.
