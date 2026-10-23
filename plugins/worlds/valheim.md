# World format: `valheim` (Valheim's chunked saves)

Lets the world download (**D** in the main menu, `--download-world`) copy Valheim worlds. See
the [README](../../README.md#download-a-world) for how to use it.

## Layout

Newer Valheim versions save each world as a folder in `worlds_local/`, named after the world:

```
worlds_local/
  Midgard/
    _main.1940.fwl2      world metadata    \
    _main.1940.db2       world data         > save 1940
    _main.1940.chunks    chunk index       /
    _main.1940.ok        written last: save 1940 is complete
    10_1a__1_1.chunk     terrain and objects, one file per area
    ...
  Midgard_backup_auto-20261022-200802/   the game's own backups, same layout
```

Older worlds (`Midgard.fwl` + `Midgard.db`) are converted the first time the game loads
them, and aren't offered. A game that still saves that way can use the [`files`](files.md) format
with `WORLD_FILES="fwl db"`.

| Where | Folder | From |
|---|---|---|
| AMP server | `<game root>/Saves/worlds_local` | `-savedir "Saves"` in the instance's `App.CommandLineArgs` (`SAVES_amp`) |
| Other server | `~/.config/unity3d/IronGate/Valheim/worlds_local` | the dedicated server's default (`SERVER_SAVES`) |
| Windows (WSL) | `C:\Users\<you>\AppData\LocalLow\IronGate\Valheim\worlds_local` | `LOCAL_WORLDS_WIN` |
| Linux | `~/.config/unity3d/IronGate/Valheim/worlds_local` | `LOCAL_WORLDS_HOME` |

## Design

- **List:** every folder with a `_main.<N>.ok`, with its newest save number, the time of that
  save and its size. The game's backups (`*_backup*`) come after the other worlds, newest first.
  A backup is never written to, so it's the safest copy to take.
- **Paths:** the world's folder.
- **Stamp:** `save <N>`, the newest `N` that has its `.ok` and all of `.fwl2`, `.db2` and `.chunks`.
  A save in progress (no `.ok` yet) doesn't count. The download compares the stamp on the server
  before and after the copy, and the copy's own, to know that no save landed in between.

This was worked out from the folders of a live server; Iron Gate doesn't document the format.
What the stamp relies on: the `.ok` is written last, and a save's files are only removed after
the next save's `.ok`.

## Functions

| Function | Returns |
|---|---|
| `valheim_label` | `Valheim (chunked saves)` |
| `valheim_list_sh` | sh code: `world<TAB>save N, time, size` per world |
| `valheim_paths WORLD` | `WORLD` (the folder) |
| `valheim_stamp_sh WORLD` | sh code: `save N`, or nothing |
