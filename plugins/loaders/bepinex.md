# Mod loader: `bepinex` (BepInEx)

Used for every profile that has `BepInEx/core` or `BepInEx/plugins`, which covers Valheim,
Lethal Company and most other Thunderstore games.

## Usage

Nothing to set up. Per profile:

| What | Folder | How it's synced |
|---|---|---|
| Mods | `BepInEx/plugins` | Mirrored (new, updated and removed mods), except `MMHOOK/` |
| Configs | `BepInEx/config` | Per file, from a checklist; never deleted on the server |

Never offered from `BepInEx/config`: `BepInEx.cfg` (the loader's own console and logging
settings, which are often different on a server on purpose) and `*.dat` (per-player data some
mods keep there).

A game file can override any of these (`SYNC_DIRS`, `EXCLUDES`, `CONFIG_DIRS`, `CONFIG_SKIP`), for
example to add `BepInEx/patchers`:

```bash
# ~/.config/game-mod-sync/plugins/games/lethal-company.conf
SYNC_DIRS="BepInEx/plugins BepInEx/patchers"
```

## Design

**Detection.** `bepinex_detect` checks the profile folder, so the loader doesn't need to be
named per game.

**Which mod owns a config.** BepInEx writes each plugin's settings to `BepInEx/config/<plugin
id>.cfg`, and some mods add more files or a folder named after the same id
(`Azumatt.FastLink_servers.yml`, `ArgusMagnus.ServersideQoL/`). The id is declared in the DLL as
`[BepInPlugin("id", "Name", "1.2.3")]`. In the compiled DLL, that's a custom-attribute blob:
the bytes `01 00`, then three length-prefixed UTF-8 strings. `bepinex_config_owners` scans every DLL
under `BepInEx/plugins` (except `MMHOOK/`) for blobs whose third string is a version number,
which yields exact `id -> mod folder` pairs without loading .NET. It needs `perl`; without it,
files are matched by name only. Mods that only mention another mod's id (as a dependency) are
not matched, because a dependency attribute doesn't carry a name and version.

Mod folders are Thunderstore's `Author-Name`, which is what the client/server tag lookup uses
(see [docs/DESIGN.md](../../docs/DESIGN.md#configs)).
