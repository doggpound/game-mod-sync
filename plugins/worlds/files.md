# World format: `files` (`<world>.<ext>`)

For games that save a world as a few files with the world's name, side by side in one folder,
e.g. older Valheim worlds (`MyWorld.fwl` + `MyWorld.db`). No plugin needed per game: the game file
sets

```bash
WORLD_FORMAT="files"
WORLD_FILES="fwl db"    # the extensions; every world has a file with the first one
```

plus the folders (`SERVER_SAVES`, `SERVER_WORLDS`, `LOCAL_WORLDS_WIN`, `LOCAL_WORLDS_HOME`; see
`plugins/games/_default.conf`).

## Design

- **List:** every `*.<first extension>` file in the worlds folder, with its time and size.
- **Paths:** `<world>.<ext>` for each extension.
- **Stamp:** `cksum` of those files, so it changes with their contents but not their times. Empty
  when one is missing.

Files a mod saves next to a world under other names (e.g. `MyWorld.fwl.BetterContinents`) are only
copied if their extension is listed (`WORLD_FILES="fwl db fwl.BetterContinents"`); then every world
must have them.

## Functions

| Function | Returns |
|---|---|
| `files_label` | `Files (<world>.<ext>)` |
| `files_list_sh` | sh code: `world<TAB>time, size` per world |
| `files_paths WORLD` | `WORLD.<ext>` for each of `WORLD_FILES` |
| `files_stamp_sh WORLD` | sh code: the files' checksums, or nothing |
