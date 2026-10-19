# Design

`game-mod-sync` connects four kinds of plugins. Each is a small file, so adding support for
something new means adding a file, not changing the core.

```
plugins/sources/<name>.sh    where profiles come from        gale
plugins/loaders/<name>.sh    where a mod loader keeps mods   bepinex
                             and configs
plugins/games/<id>.conf      game-specific settings          _default, valheim
plugins/backends/<name>.sh   how to find the game folder     amp, ssh
                             on a type of server
```

Plugins are found automatically: every `*.sh` in `plugins/sources/`, `plugins/loaders/` and
`plugins/backends/` is loaded. Server types are listed alphabetically, with `ssh` (the catch-all) always last.

**Your own plugins** go in `~/.config/game-mod-sync/plugins/` with the same folders. They
are loaded together with the repo's. A file with the same name as a repo plugin replaces it
(e.g. your own `backends/amp.sh`). For games, both are loaded in order, so your
`games/valheim.conf` only needs the lines it changes. `--config` shows which file each plugin
was loaded from. Plugin names must be valid shell identifiers (letters, digits, `_`), because
they become function prefixes; other files are ignored.

The flow for a run:

1. Every source lists its `game id / profile / folder`. Profiles whose game has nothing to sync
   (no `SYNC_DIRS` folder) are hidden.
2. You pick a profile. Its mod loader is detected from the profile folder (`<loader>_detect`), the
   loader's defaults are set, then `plugins/games/<id>.conf` is loaded on top.
3. If the profile has no saved target, a backend lists candidate folders on the server. You pick
   one, and it is saved to `targets.tsv`.
4. Each folder in `SYNC_DIRS` is previewed. `CONFIG_DIRS` are compared file by file and offered in
   a checklist (see [Configs](#configs)). Then a review (`sync_review`: the mod changes and the picked
   config files) and one confirmation; mods are mirrored and the picked configs copied.

## Why content comparison

rsync's default "changed" test is size plus modification time. Each computer's Gale downloads
and unpacks its own copy of each mod, so timestamps differ between computers. Syncing from a
different computer would re-send everything. The script uses `rsync -c` (checksums) and doesn't
preserve times (`-rl`, no `-t`), so only real content changes are sent. The cost is reading every
file on both sides, which is negligible at mod-folder sizes (about 160 MB).

## Safety rules

- `--delete` is used, so only folders the mod manager fully owns belong in `SYNC_DIRS`.
  Config folders are never mirrored: they're `CONFIG_DIRS`, copied only per picked file and
  never deleted, because server configs often differ on purpose (many mods push the server's
  config to clients).
- `EXCLUDES` are server-side files that must survive: they're never sent and never deleted
  (rsync protects excluded paths from `--delete`).
- If a sync folder is empty in the profile, it's skipped. If nothing is left to sync, the run is refused.
- Replaced or deleted files go to `<game root>/.mod-sync-backups/<timestamp>/<sync dir>/` (rsync
  `--backup-dir`). That folder sits outside the loader's folders, so it's never loaded.
  `KEEP_BACKUPS` (default 5) sets how many are kept. AMP discovery ignores this folder.
- New files get `755/644` permissions (`--chmod`), so Windows (WSL) permissions don't leak onto the server.
- Every run previews first and asks before changing anything, unless `-y` is given.

## Server-managed files

Some server types install mods themselves. For example, AMP's Valheim template has an
"Install ValheimPlus" option. With that option on, every AMP update unpacks `ValheimPlus.dll` into
`BepInEx/plugins/`. A plain mirror would delete that file on every sync.

- A game file lists what each server type can install: `MANAGED_amp="valheim_plus_install=ValheimPlus.dll"`
  (`setting=pattern[,pattern]`, with patterns relative to the sync folder).
- The backend's optional `<name>_managed` hook reports which of those settings are on for the chosen
  instance. AMP reads `App.AppSettings` from `<instance>/*.kvp`.
- The first time a file is reported, you're asked "Leave it alone?" (default yes; `-y` answers yes).
  The answer is stored in column 7 of `targets.tsv` (`+pattern` keeps the file, `-pattern` syncs over it),
  and `--setup` clears it.
- Kept patterns are anchored (`--exclude=/ValheimPlus.dll`), so a copy inside a mod's own folder
  (e.g. Gale's `Grantapher-ValheimPlus.../ValheimPlus.dll`) still syncs.
- If the profile also contains the file, you're warned that the server will have two copies. The
  cleanest fix is to turn the server option off and let the profile provide the mod.
- If the server option is later turned off, the file is no longer reported. The next sync then
  proposes removing the leftover copy.

## Configs

`lib/configs.sh`, used for every `CONFIG_DIRS` folder:

1. **Compare.** `rsync -n -c` without `--delete` itemizes files that are new (`<f+++++++++`) or
   differ. `CONFIG_SKIP` patterns are left out (BepInEx: `/BepInEx.cfg`, the loader's own settings,
   and `*.dat`, per-player data).
2. **What differs.** The server's copies of differing files are fetched to a temp folder and
   compared after dropping comments, blank lines, indentation and line endings. `.cfg` files are
   compared per `[Section] Key = value`, so the checklist can say `Range: 10 -> 20`. Files that
   only differ in formatting (BepInEx rewrites comments when versions change) are dropped.
3. **Which mod.** BepInEx names config files after the plugin's id (`Azumatt.FastLink.cfg`,
   `ArgusMagnus.ServersideQoL/` for a folder). `bepinex_config_owners` reads each DLL's
   `[BepInPlugin(id, name, version)]` attribute blob, which gives exact `id -> mod folder` pairs.
   Reading the declaration matters: a plain search for the id also hits mods that only list it
   as a dependency. A file belongs to the mod with the longest id it starts with. Fallbacks: a
   mod folder with the same name ignoring case and punctuation (also for shortened names like
   `Azumatt.FastLink_servers` and `XRayVision_ModeratorList`), then a unique match on the last
   part of the name. Tested on a real 55-mod Valheim profile: every config whose mod is
   installed was matched; the unmatched ones were all left over from removed mods, and those
   are hidden unless `-v`.
4. **Client or server.** Thunderstore categories (`Client-side`, `Server-side` for Valheim;
   regexes `TAG_CLIENT`/`TAG_SERVER` in the game file). Cached for good in
   `~/.cache/game-mod-sync/thunderstore-tags.tsv`; only mods not in it are looked up. Before
   going online, the source's own cache is used (Gale's `thunderstore_cache.json` via
   `gale_mod_tags`). Lookups use
   `https://thunderstore.io/api/experimental/package/<author>/<name>/` (6 in parallel, curl +
   perl's JSON::PP). A package Thunderstore doesn't have is cached with no tags, so it isn't
   asked for on every sync; a network error caches nothing. Edit → Refresh mod tags
   (`tags_refresh`, `--refresh-tags`) looks up every mod of every profile again.
   Client-side without Server-side = client only: not offered. No tag means the file is offered
   normally.
5. **Choose.** `ui_checklist`: new files ticked (if the mod is known), differing ones unticked.
   Rows are aligned columns built by `cfg_layout` (status, side, mod, file; long paths keep their
   end) with column titles (`UI_LIST_HEADER`), sorted new first, then by mod. Each differing
   row carries its details (`UI_DETAILS`, from `cfg_diff_full`: every differing setting with
   both values, or a line diff for non-`.cfg` files), which the TUI list expands in place. The
   text version, and a terminal too small for the drawn list, offer "Show the full differences"
   (`UI_EXTRA`) instead. whiptail/dialog
   can't disable a checklist row, so client-only mods are named above the list instead. `-y`
   takes the defaults (new files only).
6. **Copy.** `rsync --files-from` with the same `--backup-dir` as the mods, no `--delete`.

Items are kept as fields separated by `\x1f`, not tabs: `read` merges consecutive tabs, which
shifts fields when one is empty.

## Drawn menus and checklists (`lib/ui-tree.sh`)

whiptail and dialog have no expandable rows, no header bar for columns and no way to catch a
key like H in a menu. So in the TUI, `ui_choose` and `ui_checklist` draw their own box with
ANSI escapes (`_tree_list`): same colours as the dialog tool (newt's Ubuntu palette is magenta,
otherwise blue), the header bar in the table colour 20% darker (the terminal is asked for that
colour's RGB with OSC 4; without an answer, a fixed dark shade from the 256-colour palette), rows
in bright white with the cursor row in red, open details in soft grey, key lines like "E - Edit"
and "H - Help Menu", and buttons (`Ok Back`, or `Ok Exit` on the main menu) that Tab / Shift+Tab
move between, starting at Ok. Row keys (`UI_KEYS="e:Edit"`) act on the highlighted row and
return 4 with `UI_PRESSED`; the Edit menu opened that way works on that profile (`EDIT_PRESET`). H opens a help overlay (`_tree_help`, scrollable, one Close button) with
the screen's `ui_help` text plus its keys. Keys are read one at a time from the terminal
(`read -rsn1`), escape sequences decoded for arrows and PgUp/PgDn/Home/End. The box sizes itself
to its content and redraws in place; a resize or a change in size (rows opened/closed) redraws
the background too. Input boxes, yes/no, messages and progress bars stay whiptail/dialog.
Below 60x16 the plain whiptail/dialog menu and checklist are used.

## Text and TUI: one core, two interfaces

`game-mod-sync` is a small launcher. It picks the interface first, because the core loads
`lib/ui-<interface>.sh` as it starts: `--tui` / `--cli` on the command line, else `DEFAULT_UI`
from the config file (`cli` or `tui`, set in Edit → Default interface; read with `sed`, before
the config is loaded), else text. A saved `tui` falls back to text without a terminal (scripts,
cron). It sets `GMS_UI` (`text` or `tui`) and `GMS_UI_FROM` (shown by `--config`), loads
`lib/core.sh` and calls `gms_main`, which ignores `--tui` / `--cli`. `game-mod-sync-tui` only runs
`game-mod-sync --tui`, so old links keep working. All behaviour lives in the core. The core never prints or
reads directly; it only calls the UI functions below, which `lib/ui-text.sh` and `lib/ui-tui.sh`
both implement:

| Function | Text UI | TUI |
|---|---|---|
| `info MSG` | `==> MSG` line | short status box while work runs |
| `say MSG` / `ok MSG` / `warn MSG` | printed right away | collected and shown at the top of the next box |
| `die MSG` | `error:` line, exit 1 | error box, exit 1 |
| `ui_ask PROMPT [DEFAULT]` | `PROMPT [DEFAULT]:` (answer in `ANSWER`) | input box; Back returns 1 |
| `ui_choose [--no-cancel] QUESTION LABEL...` | numbered list (sets `CHOICE`), `b` returns 1 | menu; the Cancel button is labelled Back and returns 1 (`--no-cancel` hides it; the main menu has Exit) |
| `ui_checklist Q LABEL on\|off ...` | numbered list with `[x]`, type numbers to toggle (sets `CHECKED`) | checklist |
| `ui_confirm Q` / `ui_confirm_yes Q` | `[y/N]` / `[Y/n]` | yes/no box, No / Yes preselected |
| `ui_wait MSG CMD...` | `==> MSG` + spinner and seconds | progress bar with elapsed seconds |
| `ui_gauge MSG CMD...` | `[#####   ] 42%` bar | progress bar (CMD prints percentages) |
| `ui_help TEXT` | `h` prints it | H opens it as an overlay (keys are added) |
| `UI_DETAILS=(TEXT...)` before `ui_checklist` | ignored | rows expand with Right / collapse with Left |
| `ui_title TITLE` | heading above the next menu/checklist | title of the next box (the screen behind shows `game-mod-sync <version>`) |
| `ui_view TEXT` | printed | scrollable box |
| `UI_EXTRA=LABEL` before `ui_checklist` | `d` returns 3 | first row "> LABEL (tick, then OK)" returns 3 (whiptail has no third button) |
| `UI_LIST_HEADER=TEXT` before `ui_checklist` | column titles above the rows | same, indented to line up with the rows |
| `ui_suspend MSG` / `ui_resume` | prints MSG | leaves the full screen so a command can prompt, then returns |
| `ui_report` | nothing (already printed) | box with the results, waits for OK |
| `ui_start` / `ui_finish` | nothing | enter the full screen / final results box, leave the screen, echo results |

**No flicker.** Each wait is one gauge window that stays open and is updated through its input
(the `XXX` protocol), never a box relaunched in a loop. A wait's length is unknown, so its bar eases
towards 95% (about halfway after 7 seconds), shows the elapsed seconds and fills when the command
ends. The command's exit status is written to a file, because the feeder runs in a pipeline
subshell that can't reap it. While rsync is still checksumming before a copy, the copy bar ticks
"checking files (Ns)" once a second.

**Main menu.** Without `-p`, the profile menu has an Exit entry and comes back after each sync,
"already in sync" or cancelled confirmation, once you've pressed OK on the result. **Edit** opens a
second menu (`edit_menu` in `lib/core.sh`) built only from the shared UI calls, so both commands
get it: re-run setup, a checklist over the `kept` column of `targets.tsv` (patterns from the game's
`MANAGED_<backend>` plus any answered before), remove a target, open the config in `$EDITOR`
(through `ui_suspend`/`ui_resume`, then `bash -n` and reload), and show file locations. Back
returns 1 from any prompt; `setup_target` passes that up, so backing out of a setup saves nothing. `-p` (or a single profile with `-y`) syncs once.

Every wait on the network goes through `ui_wait` (connecting, finding instances, reading AMP
settings, comparing, preparing folders, tidying backups); the copy itself goes through `ui_gauge`.
`ui_wait` runs the command in the background with its output in temp files (`WAIT_OUT`,
`WAIT_ERR`), so errors can be shown in the error message. For the copy, `rsync_percent` turns
rsync's `--info=progress2` into percentages. rsync checksums every file before it starts sending
(`-c`), so the bar says "checking files" until real progress begins, then "copying". The stream is
parsed with bash `read`: `awk` and `tr` buffer piped input and would deliver every update at the end.

**One screen.** whiptail and dialog normally switch the terminal to its alternate screen when they
start and back when they exit, so the shell would flash between every box. The TUI switches once
at the start (`ui_start`) and once at the end, and runs the tools with a copy of the terminal
description that has those switches removed. The copy is built with `infocmp`/`tic` and cached in
`~/.cache/game-mod-sync/terminfo`; without those tools it still works, it just flashes. Only the
main shell may switch screens, because a subshell's exit would switch back early.

**SSH prompts.** Connecting is tried with `BatchMode=yes` first. If ssh needs a password or a new
host key confirmed, the UI steps aside (`ui_suspend`), ssh asks in the plain terminal, and the
shared SSH connection is reused afterwards, so nothing else prompts.

Because the preview is built with `say`, the TUI shows it inside the boxes that follow (the config
checklist, then the "Review" yes/no box).
When the preview is taller than the terminal, it's shown first in a scrollable text box, so the
question box stays small. Keystrokes are read from `/dev/tty`, which keeps the boxes working even
inside loops that read from a pipe. The TUI uses `whiptail` if present, else `dialog`
(`GMS_TUI_BIN` overrides).

Plugins may call the same functions (e.g. the `ssh` server type uses `ui_ask`), so they work in
both interfaces without changes.

## Files

| Path | Purpose |
|---|---|
| `game-mod-sync` | Launcher: picks the interface, loads the core (`game-mod-sync-tui` = `--tui`) |
| `lib/core.sh` | Everything else: options, plugin loading, menus' content, setup, preview, sync |
| `lib/ui-text.sh` / `lib/ui-tui.sh` | The two interfaces (same functions) |
| `lib/common.sh` | SSH connection reuse, config file and targets file |
| `plugins/sources/gale.sh` | Finds Gale's data folder and lists its games and profiles ([gale.md](../plugins/sources/gale.md)) |
| `plugins/backends/amp.sh` | Finds game folders inside AMP instances ([amp.md](../plugins/backends/amp.md)) |
| `plugins/backends/ssh.sh` | Any SSH server: you type the game folder ([ssh.md](../plugins/backends/ssh.md)) |
| `plugins/games/_default.conf` | Defaults for every game (BepInEx layout) |
| `plugins/games/valheim.conf` | Valheim specifics |

Per-computer state lives in `~/.config/game-mod-sync/` (or `$GMS_CONFIG_DIR`):

| File | Contents |
|---|---|
| `config` | Gale folder, last server type, host and user, `EXTRA_EXCLUDES`, `KEEP_BACKUPS`, `AMP_INSTANCES_DIR` |
| `targets.tsv` | One line per profile: `source  game  profile  backend  user@host  game root  kept` (tab-separated) |
| `plugins/{sources,backends,games}/` | Optional: your own plugins and overrides (see the top of this page) |

## Adding a game

Most BepInEx games work without a game file: the BepInEx loader syncs `BepInEx/plugins`, keeps
`MMHOOK/` and offers `BepInEx/config` files. Add `plugins/games/<id>.conf` (where `<id>` is Gale's folder name for the game) only to
change something. Put it in the repo to share it, or in `~/.config/game-mod-sync/plugins/games/`
to keep it local:

```bash
GAME_NAME="Lethal Company"
SYNC_DIRS="BepInEx/plugins BepInEx/patchers"   # mirrored folders
EXCLUDES="MMHOOK/"                              # never sent or deleted on the server
GAME_MARKER="BepInEx/plugins"                   # how to recognise the game folder on a server
SERVER_MATCH="lethal|1966720"                   # regex: matching server folders are listed first
MANAGED_amp="some_setting=SomeMod.dll"          # files AMP installs itself when its setting is on
AFTER_SYNC="Restart the server in AMP."
```

`CONFIG_DIRS`, `CONFIG_SKIP`, `THUNDERSTORE_COMMUNITY` (default: the game id) and
`TAG_CLIENT`/`TAG_SERVER` can be set here too. A game using a loader that has no plugin yet can
set the folders directly, e.g. MelonLoader: `SYNC_DIRS="Mods Plugins UserLibs"`,
`CONFIG_DIRS="UserData"`.

## Adding a mod loader

Add `plugins/loaders/<name>.sh` with `<name>_label`, `<name>_detect PROFILE_DIR` (succeed if the
profile uses it), `<name>_defaults` (set `SYNC_DIRS`, `EXCLUDES`, `CONFIG_DIRS`, `CONFIG_SKIP`) and,
optionally, `<name>_config_owners PLUGINS_DIR` (print `id<TAB>mod folder`). Loaders are tried in
alphabetical order; without a profile folder (e.g. `--games`) the first one is used. See
[plugins/loaders/bepinex.md](../plugins/loaders/bepinex.md).

## Adding a server type

Create `plugins/backends/<name>.sh` with these functions (it's picked up automatically), and write
`plugins/backends/<name>.md` describing its usage and design (see [amp.md](../plugins/backends/amp.md)
as a model). Use `ui_ask` / `ui_choose` / `info` / `warn` / `die` for any interaction, so it works in
both interfaces:

```bash
<name>_label()          # menu text, e.g. "Pterodactyl"
<name>_default_user()   # default SSH user, or "" to ask with no default
<name>_candidates TARGET MARKER
    # Print one "label<TAB>absolute game root" line per game folder on the server that
    # contains MARKER. Use rssh "$TARGET" "<command>" to run commands there.
<name>_manual()         # optional: define it if candidates come from what the user typed,
                        # so the single result isn't offered as a menu
<name>_managed TARGET ROOT SPECS
                        # optional: SPECS is the game's MANAGED_<name>. Print
                        # "pattern<TAB>reason" for each file the server installs itself
                        # (see "Server-managed files")
```

Ideas for later:

- **LinuxGSM:** `~/serverfiles` for each game user.
- **Pterodactyl:** `/var/lib/pterodactyl/volumes/<uuid>`, with names from the panel API.
- **Docker:** bind-mount paths from `docker inspect`.

Until one of these exists, the `ssh` type covers it: you type the folder.

The menu label in `where_of()` shortens AMP paths to the instance name. Add a case there if a new
backend has a nicer short name.

## Adding a profile source

Create `plugins/sources/<name>.sh` (it's picked up automatically) and write `plugins/sources/<name>.md`
describing its usage and design (see [gale.md](../plugins/sources/gale.md) as a model):

```bash
<name>_label()   # e.g. "r2modman"
<name>_init()    # find the data folder; return 1 if this manager isn't installed
<name>_list()    # print "game id<TAB>profile<TAB>profile folder" lines
```

r2modman would be a close copy of `gale.sh`. Its profiles are in
`r2modmanPlus-local/<Game>/profiles/<profile>/` with the same `BepInEx/` layout. Its game folder
names differ from Gale's (`Valheim` vs `valheim`), so map them to the same game ids.

## Alternatives considered

- **Gale's CLI:** only `--game`, `--profile` and `--launch`. There's no headless install or export.
- **r2modman-headless:** applies an exported `.r2z` to a server by downloading the mods from
  Thunderstore. It's an early release (v0.0.x), and it needs an export and a file copy for every
  change. Pushing the profile's own files guarantees the server runs exactly what you tested.
- **AMP:** no Thunderstore integration for Valheim.

## Testing

The scripts were tested locally against a fake Gale folder (two games), a fake AMP tree
(Valheim and Lethal Company instances) and an `ssh` shim that runs commands locally. The
tests covered first-run setup, ranking, preview, apply, idempotent re-runs, backup pruning,
the empty-profile guard, ambiguous `-p`, `--setup`, WSL detection, nested folder creation, and
AMP-managed ValheimPlus detection (keep, decline, and option turned off), user plugin drop-ins and
overrides. The TUI was driven with real `whiptail` and `dialog` in a terminal session: setup,
managed-file prompt (yes and no), long preview, apply, cancel, and the missing-tool error. The
terminal's alternate-screen flag was sampled during whole runs to confirm a single enter/leave, and
the copy bar was checked to move from "checking files" through rising percentages. Error output from
a failing rsync and the ssh password fallback were tested too.
Configs were tested with real files from a 55-mod Valheim profile (Gale on Windows): owner
matching against the real DLLs, plus an end-to-end run with a new server-side mod
(ServersideQoL_AutoStore: mod and both its configs copied, one inside a sub-folder), a differing
client+server config (ticked and replaced, old one in the backups), a client-only one (listed, not
copied), comment- and CRLF-only differences (hidden), a removed mod's leftover (hidden), `-y` (new
only), Back (nothing changed), and no network (tags unknown, files offered).
Lint: `shellcheck game-mod-sync game-mod-sync-tui lib/*.sh plugins/*/*.sh` (settings in `.shellcheckrc`).
