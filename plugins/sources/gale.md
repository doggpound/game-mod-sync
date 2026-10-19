# Profile source: `gale` (Gale mod manager)

Reads games and profiles from [Gale](https://github.com/Kesomannen/gale). Nothing in Gale is
changed; the profile folders are only read and synced from.

## Usage

Nothing to set up: Gale's data folder is found automatically. Every Gale game that has profiles
shows up in the menu as `Game / Profile`. Profiles whose game has nothing to sync (none of the
game's `SYNC_DIRS`, e.g. no `BepInEx/plugins`) are hidden.

Where it looks, in order:

| Setup | Folder |
|---|---|
| WSL on Windows | `/mnt/<drive>/Users/<user>/AppData/Roaming/com.kesomannen.gale` |
| Git Bash / Cygwin | `$APPDATA/com.kesomannen.gale` |
| Linux | `${XDG_DATA_HOME:-~/.local/share}/com.kesomannen.gale` |
| Linux (Flatpak) | `~/.var/app/com.kesomannen.gale/data/com.kesomannen.gale` |
| macOS | `~/Library/Application Support/com.kesomannen.gale` |

If more than one exists (for example, Gale installed in both Windows and WSL), you pick one once and
it's saved. If Gale's data folder is somewhere else, set it once:

```bash
echo 'GALE_DATA_DIR="/path/to/com.kesomannen.gale"' >> ~/.config/game-mod-sync/config
```

| Setting | Default | Purpose |
|---|---|---|
| `GALE_DATA_DIR` | empty (auto-detect) | Gale's data folder, the one containing one folder per game |

## Design

Gale stores one folder per game, each with its profiles:

```
com.kesomannen.gale/
  valheim/
    profiles/
      MyProfile/
        BepInEx/plugins/...
  lethal-company/
    profiles/...
```

- The **game id** is Gale's folder name (`valheim`, `lethal-company`). game-mod-sync uses it to load
  `plugins/games/<id>.conf`, so a game file must be named after Gale's folder name.
- The **profile folder** has the same layout as the game's install folder (`BepInEx/plugins`, ...),
  which is why `SYNC_DIRS` paths work on both sides unchanged.
- Any folder without a `profiles/` folder inside it is ignored.

`gale_init` returns failure when no Gale data folder is found. That lets the core support
several sources: it uses whichever ones are installed and only errors when none are.

Gale unpacks mods itself on each computer, so file dates differ between computers. That's why the
sync compares checksums instead of dates (see `docs/DESIGN.md`).

## Functions

| Function | Returns |
|---|---|
| `gale_label` | `Gale` |
| `gale_init` | finds the data folder (may ask once and save it); returns 1 if Gale isn't installed |
| `gale_list` | `game id<TAB>profile<TAB>profile folder` for every profile of every game |

**Mod tags.** `gale_mod_tags GAME` reads Gale's `<game>/thunderstore_cache.json` (Thunderstore
packages Gale has looked up, with their categories) so client-only checks for configs work
without asking Thunderstore. It only covers the mods Gale happened to cache; the rest are
looked up online (see [docs/DESIGN.md](../../docs/DESIGN.md#configs)).
