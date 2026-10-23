# shellcheck shell=bash
# game-mod-sync core: everything except how questions are asked and messages
# shown. The launcher (game-mod-sync) picks the interface - GMS_UI=text
# (lib/ui-text.sh) or tui (lib/ui-tui.sh) - sets APP_DIR, sources this file,
# then calls gms_main "$@".

set -euo pipefail

VERSION="0.1.0"

# Defaults, overridable in the config file.
GALE_DATA_DIR=""
LAST_BACKEND="" LAST_HOST="" LAST_USER=""
AMP_INSTANCES_DIR=".ampdata/instances"
EXTRA_EXCLUDES=""    # extra excludes for every game, space separated
KEEP_BACKUPS=5       # server-side backups to keep, 0 = off
DEFAULT_UI=""        # cli or tui: the interface without --cli / --tui (empty = cli)
LOCAL_WORLDS=""      # "game=folder|...": local worlds folder per game, when several were found

CONFIG_DIR="${GMS_CONFIG_DIR:-${XDG_CONFIG_HOME:-$HOME/.config}/game-mod-sync}"
CONFIG_FILE="$CONFIG_DIR/config"
TARGETS_FILE="$CONFIG_DIR/targets.tsv"

PROG=${PROG:-game-mod-sync}

# shellcheck source=lib/common.sh
. "$APP_DIR/lib/common.sh"
# shellcheck source=lib/ui-text.sh
. "$APP_DIR/lib/ui-${GMS_UI:-text}.sh"
# shellcheck source=lib/configs.sh
. "$APP_DIR/lib/configs.sh"
# shellcheck source=lib/worlds.sh
. "$APP_DIR/lib/worlds.sh"

# ---------------------------------------------------------------- plugins --
# Plugins live in the repo (plugins/) and, optionally, in your config folder
# (~/.config/game-mod-sync/plugins/). A file there with the same name as a
# repo plugin replaces it; a new name adds one.
#   plugins/sources/<name>.sh   where profiles come from
#   plugins/loaders/<name>.sh   mod loaders (where mods and configs live)
#   plugins/backends/<name>.sh  server types
#   plugins/worlds/<name>.sh    world formats (how a game saves a world)
#   plugins/games/<id>.conf     per-game settings (repo first, then yours)
PLUGIN_DIR="$APP_DIR/plugins"
USER_PLUGIN_DIR="$CONFIG_DIR/plugins"

# plugin_names KIND [LAST...] -> plugin names (sorted, deduplicated), with the
# LAST names moved to the end of the list (e.g. the catch-all "ssh" type).
plugin_names() {
  local kind=$1 f n
  shift
  for f in "$PLUGIN_DIR/$kind"/*.sh "$USER_PLUGIN_DIR/$kind"/*.sh; do
    [ -f "$f" ] || continue
    n=${f##*/}; printf '%s\n' "${n%.sh}"
  done | sort -u | awk -v last=" $* " '
    !/^[A-Za-z_][A-Za-z0-9_]*$/ { next }        # names become function prefixes
    index(last, " " $0 " ") { tail = tail $0 "\n"; next }
    { print } END { printf "%s", tail }'
}
plugin_file() { # KIND NAME -> your copy if you have one, else the repo's
  if [ -f "$USER_PLUGIN_DIR/$1/$2.sh" ]; then printf '%s' "$USER_PLUGIN_DIR/$1/$2.sh"
  else printf '%s' "$PLUGIN_DIR/$1/$2.sh"; fi
}
SOURCES=$(plugin_names sources | paste -sd' ' -)        # menu order
BACKENDS=$(plugin_names backends ssh | paste -sd' ' -)  # "Other (SSH)" always last
LOADERS=$(plugin_names loaders | paste -sd' ' -)
WORLD_FORMATS=$(plugin_names worlds | paste -sd' ' -)
for _p in $SOURCES;  do . "$(plugin_file sources "$_p")";  done
for _p in $LOADERS;  do . "$(plugin_file loaders "$_p")";  done
for _p in $BACKENDS; do . "$(plugin_file backends "$_p")"; done
for _p in $WORLD_FORMATS; do . "$(plugin_file worlds "$_p")"; done
# shellcheck source=/dev/null
[ -f "$CONFIG_FILE" ] && . "$CONFIG_FILE"

usage() {
  cat <<EOF
$PROG $VERSION - sync mod-manager profiles to game servers

Usage: $PROG [options]

      --cli            text menus and prompts (default)
      --tui            full-screen dialog boxes (needs whiptail or dialog)
                       (the default can be changed: Edit > Default interface)

  -g, --game ID        only this game (Gale's folder name, e.g. valheim)
  -p, --profile NAME   this profile (skips the menu)
  -n, --dry-run        show what would change, change nothing
  -y, --yes            don't ask for confirmation
  -v, --verbose        list every file, not just a per-mod summary
      --setup          (re)pick the server folder for the chosen profile
      --refresh-tags   look up every mod's client/server tags on Thunderstore again
      --download-world copy a world from the profile's server into this
                       computer's saves (with -p; nothing on the server changes)
      --world NAME     the world to download (default: the one the server runs)
      --list           show saved profile -> server mappings and exit
      --games          show detected games and what gets synced for each
      --config         print config file paths and exit
  -h, --help           this help
  -V, --version        print version

The first sync of a profile asks for the server type, host and user, then
lists the matching game folders on the server; the choice is remembered.

Server types: $(for b in $BACKENDS; do printf '%s (%s)  ' "$b" "$("${b}_label")"; done)
Config:       $CONFIG_FILE
Targets:      $TARGETS_FILE
EOF
}

# ------------------------------------------------------------------ games --
# Load the settings for GAME_ID: built-in default, then the game's own file,
# then a user override in the config folder.
# The mod loader is detected from the profile folder when one is given (else
# the first loader, BepInEx); its defaults come first so a game file can
# override them.
load_game() { # GAME_ID [PROFILE_DIR]
  GAME_ID=$1
  GAME_NAME="" SYNC_DIRS="" EXCLUDES="" CONFIG_DIRS="" CONFIG_SKIP="" GAME_MARKER="" SERVER_MATCH="" AFTER_SYNC=""
  WORLD_FORMAT="" WORLD_FILES="" SERVER_SAVES="" SERVER_WORLDS="" LOCAL_WORLDS_WIN="" LOCAL_WORLDS_HOME="" WORLD_NOTE=""
  local _b; for _b in $BACKENDS; do eval "MANAGED_$_b='' SAVES_$_b=''"; done
  LOADER=""
  for _b in $LOADERS; do
    if [ -n "${2:-}" ] && "${_b}_detect" "$2"; then LOADER=$_b; break; fi
  done
  [ -n "$LOADER" ] || LOADER=${LOADERS%% *}
  [ -z "$LOADER" ] || "${LOADER}_defaults"
  local _f
  for _f in "$PLUGIN_DIR/games/_default.conf" "$USER_PLUGIN_DIR/games/_default.conf" \
            "$PLUGIN_DIR/games/$GAME_ID.conf" "$USER_PLUGIN_DIR/games/$GAME_ID.conf"; do
    # shellcheck source=/dev/null
    [ -f "$_f" ] && . "$_f"
  done
  [ -n "$GAME_MARKER" ] || GAME_MARKER=${SYNC_DIRS%% *}
  return 0
}

# Short name for where a profile syncs to: the instance for AMP, else the path.
where_of() { # BACKEND ROOT
  local w=$2
  if [ "$1" = amp ]; then w=${w#*/"$AMP_INSTANCES_DIR"/}; w=${w%%/*}; fi
  printf '%s' "$w"
}

# Does this profile folder have at least one of the game's sync folders?
profile_has_sync_dirs() {
  local d
  for d in $SYNC_DIRS; do [ -d "$1/$d" ] && return 0; done
  return 1
}

# ---------------------------------------------------------------- entries --
# ENTRIES: "source<TAB>game<TAB>profile<TAB>profile folder", syncable only.
ENTRIES=()
collect_entries() {
  local s line game rest found=0
  for s in $SOURCES; do
    if ! "${s}_init"; then continue; fi
    found=1
    while IFS= read -r line; do
      [ -n "$line" ] || continue
      game=${line%%$'\t'*}
      [ -z "$GAME_FILTER" ] || [ "$game" = "$GAME_FILTER" ] || continue
      rest=${line#*$'\t'}
      load_game "$game" "${rest#*$'\t'}"
      profile_has_sync_dirs "${rest#*$'\t'}" || continue
      ENTRIES+=("$s"$'\t'"$line")
    done < <("${s}_list")
  done
  [ "$found" = 1 ] || die "couldn't find a mod manager's data folder ($SOURCES).
       If Gale's data folder is somewhere unusual, set it once with:
         echo 'GALE_DATA_DIR=\"/path/to/com.kesomannen.gale\"' >> $CONFIG_FILE"
}

# --------------------------------------------------------------- setup ----
setup_target() { # SOURCE GAME PROFILE
  local src=$1 game=$2 profile=$3 b backend host user def_user target line
  local -a backends=() labels=() roots=() rlabels=() other_roots=() other_labels=()
  say ""
  say "One-time setup for ${B}$GAME_NAME / $profile${N}"

  for b in $BACKENDS; do backends+=("$b"); labels+=("$("${b}_label")"); done
  ui_title "Setup: server type"
  ui_help "Setup: server type

How the game server is run. AMP finds the game folders of your AMP
instances by itself; Other (SSH) asks for the folder. Either way it only
needs SSH access to the server."
  ui_choose "What kind of server is it?" "${labels[@]}" || return 1
  backend=${backends[$((CHOICE - 1))]}

  ui_title "Setup: server"
  ui_ask "Server hostname" "$LAST_HOST" || return 1; host=$ANSWER
  [ -n "$host" ] || die "a hostname is required"
  def_user=$("${backend}_default_user")
  [ "$backend" = "$LAST_BACKEND" ] && [ -n "$LAST_USER" ] && def_user=$LAST_USER
  ui_title "Setup: SSH user"
  ui_ask "SSH user" "$def_user" || return 1; user=$ANSWER
  [ -n "$user" ] || die "an SSH user is required"
  LAST_BACKEND=$backend LAST_HOST=$host LAST_USER=$user
  save_config
  target="$user@$host"

  # Try without prompts first; if ssh needs a password or a new host key
  # confirmed, hand it the plain terminal once (the shared connection is
  # then reused, so later steps don't ask again).
  if ! ui_wait "Connecting to $target ..." rssh -o BatchMode=yes "$target" true; then
    ui_suspend "ssh needs your input for $target:"
    rssh "$target" true || die "can't ssh to $target (tip: ssh-copy-id $target)"
    ui_resume
  fi

  # Candidates matching this game go first; other games' folders after.
  while IFS=$'\t' read -r b line; do
    [ -n "$line" ] || continue
    if declare -F "${backend}_manual" >/dev/null || printf '%s\n' "$line" | grep -Eiq -- "$SERVER_MATCH"; then
      rlabels+=("$b"); roots+=("$line")
    else
      other_labels+=("$b  ${D}(other game?)${N}"); other_roots+=("$line")
    fi
  done < <("${backend}_candidates" "$target" "$GAME_MARKER")
  rlabels+=(${other_labels[@]+"${other_labels[@]}"})
  roots+=(${other_roots[@]+"${other_roots[@]}"})
  # A typed-in folder that came back empty means Back was pressed.
  if [ ${#roots[@]} -eq 0 ] && declare -F "${backend}_manual" >/dev/null; then return 1; fi
  [ ${#roots[@]} -gt 0 ] || die "no $GAME_MARKER folders found on $host.
       Install the mod loader on the server (and start it once so its folders exist),
       or run --setup again and pick '$(ssh_label)'."

  if [ ${#roots[@]} -eq 1 ] && { [ ${#other_roots[@]} -eq 0 ] || declare -F "${backend}_manual" >/dev/null; }; then
    CHOICE=1
  else
    ui_title "Setup: game folder"
    ui_help "Setup: game folder

The game folders found on the server. Ones that look like this game are
listed first; '(other game?)' marks folders of other games that use the
same mod loader. The choice is saved, so later syncs don't ask."
    ui_choose "Where should '$profile' sync to?" "${rlabels[@]}" || return 1
  fi
  target_set "$src" "$game" "$profile" "$backend" "$target" "${roots[$((CHOICE - 1))]}"
  ok "Saved: $GAME_NAME / $profile -> $(where_of "$backend" "${roots[$((CHOICE - 1))]}") on $host"
}

# --------------------------------------------------------- server-managed --
# Files the server installs itself (e.g. AMP's "Install ValheimPlus"). Asks
# once per file (default: leave it alone) and remembers the answer.
# Sets KEEP_PATTERNS to the patterns to exclude for this sync.
KEEP_PATTERNS=()
check_server_managed() { # SOURCE GAME PROFILE PROFILE_DIR BACKEND TARGET ROOT KEPT
  local src=$1 game=$2 profile=$3 pdir=$4 backend=$5 target=$6 root=$7 kept=$8
  local specvar="MANAGED_$backend" pat why dup d changed=0
  KEEP_PATTERNS=()
  declare -F "${backend}_managed" >/dev/null || return 0
  [ -n "${!specvar-}" ] || return 0
  while IFS=$'\t' read -r pat why; do
    [ -n "$pat" ] || continue
    case ",$kept," in
      *",+$pat,"*) KEEP_PATTERNS+=("$pat")
                   say "${D}Leaving $pat alone - the server installs it ($why)${N}"
                   continue ;;
      *",-$pat,"*) continue ;;
    esac
    say ""
    warn "The server installs $pat itself ($why)."
    dup=""
    for d in $SYNC_DIRS; do
      [ -d "$pdir/$d" ] || continue
      dup=$(cd "$pdir/$d" && find . -mindepth 2 -name "${pat##*/}" 2>/dev/null | sed 's|^\./||' | head -n1)
      [ -n "$dup" ] && break
    done
    if [ -n "$dup" ]; then
      warn "Your profile also has its own copy ($dup), so the server ends up with two.
         Cleanest fix: turn the option off on the server and let the profile provide it."
    fi
    ui_title "Server-installed file"
    if [ "$ASSUME_YES" = 1 ] || ui_confirm_yes "Leave the server's $pat alone when syncing?"; then
      kept="$kept,+$pat"; KEEP_PATTERNS+=("$pat")
    else
      kept="$kept,-$pat"
    fi
    changed=1
  done < <("${backend}_managed" "$target" "$root" "${!specvar}")
  [ "$changed" = 1 ] && target_set_kept "$src" "$game" "$profile" "$kept"
  return 0
}

# ------------------------------------------------------------------- sync --
# Per-mod summary of rsync's itemized output; adds to N_SEND / N_DEL.
N_SEND=0 N_DEL=0 SYNC_REVIEW=""
summarize() {
  local summary s d
  summary=$(printf '%s\n' "$1" | awk '
    function modof(p,  s) { s = index(p, "/"); return s ? substr(p, 1, s - 1) : p }
    {
      code = $1; path = $0; sub(/^[^ ]+ +/, "", path)
      if (path == "" || path == "./") next
      m = modof(path); top = (path == m || path == m "/")
      if (code == "*deleting")  { if (path !~ /\/$/) del++; seen[m] = 1; if (top) gone[m] = 1 }
      else if (code ~ /^cd\+/)  { if (top) added[m] = 1 }
      else if (code ~ /^[<>]f/) { send++; seen[m] = 1; if (top && code ~ /^.f\+/) added[m] = 1 }
    }
    END {
      for (m in seen) print (gone[m] ? "3removed" : added[m] ? "1new" : "2updated") "\t" m
      printf "#\t%d\t%d\n", send, del
    }' | sort)
  s=$(printf '%s\n' "$summary" | awk -F'\t' '$1 == "#" { print $2 }')
  d=$(printf '%s\n' "$summary" | awk -F'\t' '$1 == "#" { print $3 }')
  N_SEND=$((N_SEND + ${s:-0})) N_DEL=$((N_DEL + ${d:-0}))
  local kind mod
  while IFS=$'\t' read -r kind mod; do
    case $kind in
      1new)     say "   ${GRN}+ new    ${N}  $mod"; SYNC_REVIEW+="   + new      $mod"$'\n' ;;
      2updated) say "   ${YEL}~ updated${N}  $mod"; SYNC_REVIEW+="   ~ updated  $mod"$'\n' ;;
      3removed) say "   ${RED}- removed${N}  $mod"; SYNC_REVIEW+="   - removed  $mod"$'\n' ;;
    esac
  done <<<"$summary"
}

# rsync with whole-transfer progress, printed as bare percentages for ui_gauge.
# Returns rsync's exit status.
rsync_percent() {
  # Progress lines end in \r. Parsed with bash `read` because awk/tr buffer
  # piped input and would deliver every update at the very end.
  local chunk p last=-1
  rsync --info=progress2 --no-inc-recursive --outbuf=N "$@" |
  while IFS= read -r -d $'\r' chunk || [ -n "$chunk" ]; do
    if [[ $chunk =~ [[:space:]]([0-9]{1,3})%[[:space:]] ]]; then
      p=${BASH_REMATCH[1]}
      [ "$p" = "$last" ] || { printf '%s\n' "$p"; last=$p; }
    fi
  done
  return "${PIPESTATUS[0]}"
}

# The review shown before anything is applied: mods, then the picked configs.
sync_review() { # PROFILE BACKEND ROOT TARGET
  say "$GAME_NAME / $1  ->  $(where_of "$2" "$3") on ${4#*@}"
  say ""
  if [ "$N_SEND" -gt 0 ] || [ "$N_DEL" -gt 0 ]; then
    say "Mods: $N_SEND file(s) to send, $N_DEL to delete"
    say "${SYNC_REVIEW%$'\n'}"
  else
    say "Mods: already in sync"
  fi
  say ""
  if [ ${#CFG_SELECTED[@]} -gt 0 ]; then
    say "Config files to copy: ${#CFG_SELECTED[@]}"
    cfg_review
  else
    say "Config files to copy: none"
  fi
  say ""
  say "Replaced and deleted files are kept in .mod-sync-backups on the server."
}

do_sync() { # SOURCE GAME PROFILE PROFILE_DIR
  local src=$1 game=$2 profile=$3 pdir=$4 t backend target root kept d e out ts
  local -a base=() dirs=() args=()
  t=$(target_get "$src" "$game" "$profile") || die "no server set up for '$profile'"
  IFS=$'\t' read -r backend target root kept <<<"$t"

  check_server_managed "$src" "$game" "$profile" "$pdir" "$backend" "$target" "$root" "$kept"

  # shellcheck disable=SC2054  # the commas are part of --chmod's value
  base=(-rlc -s --delete --chmod=D755,F644 --out-format='%i %n' -e "$(rsh_cmd)")
  for e in $EXCLUDES $EXTRA_EXCLUDES; do base+=("--exclude=$e"); done
  # Server-managed patterns are anchored to the top of the sync folder, so a
  # same-named file inside a mod's own folder is still synced.
  for e in ${KEEP_PATTERNS[@]+"${KEEP_PATTERNS[@]}"}; do base+=("--exclude=/${e#/}"); done

  # Only sync folders the profile has; an empty one would wipe the server's.
  for d in $SYNC_DIRS; do
    [ -d "$pdir/$d" ] || continue
    if [ -z "$(ls -A "$pdir/$d" 2>/dev/null)" ]; then
      warn "$d is empty in this profile - skipping it so the server's copy isn't wiped"
      continue
    fi
    dirs+=("$d")
  done
  [ ${#dirs[@]} -gt 0 ] || die "'$profile' has nothing to sync - refusing (it would wipe the server's mods)"

  say ""
  say "${B}$GAME_NAME / $profile${N}  ->  ${B}$(where_of "$backend" "$root")${N} on ${target#*@}"
  N_SEND=0 N_DEL=0 SYNC_REVIEW=""
  for d in "${dirs[@]}"; do
    ui_wait "Comparing $GAME_NAME / $profile with the server (checksums): $d" \
        rsync "${base[@]}" -n "$pdir/$d/" "$target:$root/$d/" \
      || die "rsync preview of $d failed (is $root still there? re-run with --setup)
$(printf '%s\n' "$WAIT_ERR" | tail -n 3)"
    out=$WAIT_OUT
    [ ${#dirs[@]} -gt 1 ] && say "  ${B}$d/${N}"
    if [ "$VERBOSE" = 1 ] && [ -n "$out" ]; then
      while IFS= read -r e; do say "   $e"; done <<<"$out"
      say ""
    fi
    summarize "$out"
  done

  # Config files: offered per file, never deleted (lib/configs.sh).
  cfg_scan "$src" "$pdir" "$target" "$root"
  cfg_count
  if [ "$N_SEND" -eq 0 ] && [ "$N_DEL" -eq 0 ] && [ "$CFG_OFFERED" -eq 0 ]; then
    ok "Already in sync - nothing to do."
    return 0
  fi
  say ""
  if [ "$N_SEND" -gt 0 ] || [ "$N_DEL" -gt 0 ]; then say "$N_SEND file(s) to send, $N_DEL to delete."
  else say "Mods are in sync."; fi
  if [ "$DRY_RUN" = 1 ]; then
    [ ${#CFG_ITEMS[@]} -eq 0 ] || { say ""; say "  ${B}Configs${N}"; cfg_preview; }
    ok "Dry run - nothing changed."; return 0
  fi
  [ "$ASSUME_YES" = 1 ] && [ ${#CFG_ITEMS[@]} -gt 0 ] && { say "  ${B}Configs${N} (new ones are copied)"; cfg_preview; }
  cfg_choose || { say "Cancelled - nothing changed."; return 0; }
  if [ "$N_SEND" -eq 0 ] && [ "$N_DEL" -eq 0 ] && [ ${#CFG_SELECTED[@]} -eq 0 ]; then
    ok "No configs picked - nothing changed."
    return 0
  fi
  if [ "$ASSUME_YES" != 1 ]; then
    local what=""
    if [ "$N_SEND" -gt 0 ] || [ "$N_DEL" -gt 0 ]; then what="$N_SEND file(s) to send, $N_DEL to delete"; fi
    [ ${#CFG_SELECTED[@]} -gt 0 ] && what="${what:+$what, }${#CFG_SELECTED[@]} config file(s) to copy"
    sync_review "$profile" "$backend" "$root" "$target"
    ui_title "Review"
    ui_confirm "Apply these changes to the server? ($what)" || { say "Cancelled - nothing changed."; return 0; }
  fi

  # Replaced or deleted files are moved to <game root>/.mod-sync-backups/<time>/,
  # outside the folders the mod loader scans.
  ts=$(date +%Y%m%d-%H%M%S)
  if [ "$N_SEND" -gt 0 ] || [ "$N_DEL" -gt 0 ]; then
    for d in "${dirs[@]}"; do
      args=("${base[@]}")
      [ "${KEEP_BACKUPS:-0}" -gt 0 ] && args+=(--backup "--backup-dir=$root/.mod-sync-backups/$ts/$d")
      ui_wait "Preparing $d on the server ..." rssh "$target" "mkdir -p $(shq "$root/$d")" \
        || die "can't create $root/$d on the server"
      ui_title "Syncing mods"
      ui_gauge "Syncing $GAME_NAME / $profile: $d" rsync_percent "${args[@]}" "$pdir/$d/" "$target:$root/$d/" \
        || die "rsync of $d failed
$(printf '%s\n' "$WAIT_ERR" | tail -n 3)"
    done
  fi
  cfg_copy "$pdir" "$target" "$root" "$ts"
  if [ "${KEEP_BACKUPS:-0}" -gt 0 ]; then
    ui_wait "Tidying old backups ..." rssh "$target" "cd $(shq "$root/.mod-sync-backups") 2>/dev/null && \
      ls -1 | sort -r | tail -n +$((KEEP_BACKUPS + 1)) | xargs -r rm -rf --" || true
  fi

  ok "Synced $GAME_NAME / $profile."
  [ ${#CFG_SELECTED[@]} -eq 0 ] || say "Copied ${#CFG_SELECTED[@]} config file(s)."
  [ "${KEEP_BACKUPS:-0}" -gt 0 ] && say "Previous files: $root/.mod-sync-backups/$ts"
  say "$AFTER_SYNC"
}

# Download a world from the entry's server (lib/worlds.sh).
run_download() { # "SOURCE<TAB>GAME<TAB>PROFILE<TAB>DIR"
  local src game profile pdir
  IFS=$'\t' read -r src game profile pdir <<<"$1"
  load_game "$game" "$pdir"
  if ! world_supported; then
    say "$GAME_NAME has no world download (its game file sets no WORLD_FORMAT)."
    return 0
  fi
  if [ "$DO_SETUP" = 1 ] || ! target_get "$src" "$game" "$profile" >/dev/null; then
    setup_target "$src" "$game" "$profile" || { NOTES=""; return 0; }
  fi
  world_download "$src" "$game" "$profile"
}

run_entry() { # "SOURCE<TAB>GAME<TAB>PROFILE<TAB>DIR"
  local src game profile pdir
  IFS=$'\t' read -r src game profile pdir <<<"$1"
  load_game "$game" "$pdir"
  if [ "$DO_SETUP" = 1 ] || ! target_get "$src" "$game" "$profile" >/dev/null; then
    # Back: nothing to sync, and drop the half-shown setup messages.
    setup_target "$src" "$game" "$profile" || { NOTES=""; return 0; }
  fi
  do_sync "$src" "$game" "$profile" "$pdir"
}

# ------------------------------------------------------------------- edit --
config_paths() {
  local s
  printf 'interface:    %s (%s)\n' "$([ "${GMS_UI:-text}" = tui ] && echo tui || echo cli)" "${GMS_UI_FROM:-default}"
  printf 'config:       %s\ntargets:      %s\nplugins:      %s\nyour plugins: %s\n' \
    "$CONFIG_FILE" "$TARGETS_FILE" "$PLUGIN_DIR" "$USER_PLUGIN_DIR"
  for s in $SOURCES;  do printf '  source   %-8s %s\n' "$s" "$(plugin_file sources "$s")"; done
  for s in $LOADERS;  do printf '  loader   %-8s %s\n' "$s" "$(plugin_file loaders "$s")"; done
  for s in $BACKENDS; do printf '  backend  %-8s %s\n' "$s" "$(plugin_file backends "$s")"; done
  for s in $WORLD_FORMATS; do printf '  world    %-8s %s\n' "$s" "$(plugin_file worlds "$s")"; done
}

# LABELS: one menu line per entry in ENTRIES ("Game / profile -> where").
LABELS=()
entry_labels() {
  local e s g p d t tb tt tr
  LABELS=()
  for e in "${ENTRIES[@]}"; do
    IFS=$'\t' read -r s g p d <<<"$e"
    load_game "$g"
    if t=$(target_get "$s" "$g" "$p"); then
      IFS=$'\t' read -r tb tt tr _ <<<"$t"
      LABELS+=("$GAME_NAME / $p  ${D}-> $(where_of "$tb" "$tr") on ${tt#*@}${N}")
    else
      LABELS+=("$GAME_NAME / $p  ${D}(not set up yet)${N}")
    fi
  done
}

# Pick a profile for an edit; EDIT_ENTRY is empty after Back.
# With "setup-only", only profiles that have a server are listed.
# When the Edit menu was opened on a profile (EDIT_PRESET), that one is used.
EDIT_ENTRY="" EDIT_PRESET=""
edit_pick_profile() { # QUESTION [setup-only]
  local -a ents=() labs=()
  local i s g p d
  if [ -n "$EDIT_PRESET" ]; then
    IFS=$'\t' read -r s g p d <<<"$EDIT_PRESET"
    EDIT_ENTRY=$EDIT_PRESET
    if [ "${2:-}" = setup-only ] && ! target_get "$s" "$g" "$p" >/dev/null; then
      load_game "$g"
      say "$GAME_NAME / $p has no server set up yet."
      EDIT_ENTRY=""
    fi
    return 0
  fi
  entry_labels
  for i in "${!ENTRIES[@]}"; do
    IFS=$'\t' read -r s g p d <<<"${ENTRIES[$i]}"
    if [ "${2:-}" = setup-only ] && ! target_get "$s" "$g" "$p" >/dev/null; then continue; fi
    ents+=("${ENTRIES[$i]}"); labs+=("${LABELS[$i]}")
  done
  EDIT_ENTRY=""
  if [ ${#ents[@]} -eq 0 ]; then say "No profile has a server set up yet."; return 0; fi
  ui_choose "$1" "${labs[@]}" || return 0
  EDIT_ENTRY=${ents[$((CHOICE - 1))]}
}

# Checklist of server-managed files: ticked = leave the server's copy alone.
edit_managed() {
  local s g p d t backend target root kept specvar spec pat why i state
  local -a pats=() items=() whys=()
  ui_title "Server-managed files"
  edit_pick_profile "Server-managed files for which profile?" setup-only
  [ -n "$EDIT_ENTRY" ] || return 0
  IFS=$'\t' read -r s g p d <<<"$EDIT_ENTRY"
  load_game "$g"
  t=$(target_get "$s" "$g" "$p")
  IFS=$'\t' read -r backend target root kept <<<"$t"
  # Files the game config names for this server type, plus any answered before.
  specvar="MANAGED_$backend"
  for spec in ${!specvar-}; do
    pat=${spec#*=}; pats+=("$pat"); whys+=("${backend} setting \"${spec%%=*}\"")
  done
  for pat in $(printf '%s' "$kept" | tr ',' '\n' | sed -n 's/^[+-]//p'); do
    case " ${pats[*]-} " in *" $pat "*) ;; *) pats+=("$pat"); whys+=("answered before") ;; esac
  done
  if [ ${#pats[@]} -eq 0 ]; then
    say "Nothing on the server is managed for $GAME_NAME / $p - every file is synced."
    return 0
  fi
  for i in "${!pats[@]}"; do
    case ",$kept," in *",-${pats[$i]},"*) state=off ;; *) state=on ;; esac
    items+=("${pats[$i]} - leave the server's copy alone (${whys[$i]})" "$state")
  done
  ui_title "Server-managed files: $GAME_NAME / $p"
  ui_help "Server-managed files

Files the server installs by itself when one of its settings is on (for
example AMP's 'Install ValheimPlus'). Ticked: the server's copy is left
alone when syncing. Unticked: your profile's version is synced over it."
  ui_checklist "$GAME_NAME / $p: ticked files are left alone; unticked ones are synced over." "${items[@]}" || return 0
  kept=""
  for i in "${!pats[@]}"; do
    case " $CHECKED " in *" $((i + 1)) "*) kept="$kept,+${pats[$i]}" ;; *) kept="$kept,-${pats[$i]}" ;; esac
  done
  target_set_kept "$s" "$g" "$p" "$kept"
  ok "Saved server-managed files for $GAME_NAME / $p."
}

edit_change_server() {
  local s g p d
  ui_title "Change a profile's server"
  edit_pick_profile "Change the server for which profile?"
  [ -n "$EDIT_ENTRY" ] || return 0
  IFS=$'\t' read -r s g p d <<<"$EDIT_ENTRY"
  load_game "$g"
  setup_target "$s" "$g" "$p" || NOTES=""
}

edit_forget() {
  local s g p d
  ui_title "Forget a profile's server"
  edit_pick_profile "Forget the server for which profile?" setup-only
  [ -n "$EDIT_ENTRY" ] || return 0
  IFS=$'\t' read -r s g p d <<<"$EDIT_ENTRY"
  load_game "$g"
  ui_title "Forget a profile's server"
  if ui_confirm "Forget where $GAME_NAME / $p syncs to? The next sync will ask again."; then
    target_remove "$s" "$g" "$p"
    ok "Forgot the server for $GAME_NAME / $p."
  fi
}

edit_config_file() {
  local ed=${VISUAL:-${EDITOR:-}} tty=${GMS_TTY:-/dev/tty}
  if [ -z "$ed" ]; then
    for ed in nano vi; do command -v "$ed" >/dev/null 2>&1 && break; ed=""; done
  fi
  [ -n "$ed" ] || { warn "No editor found - set EDITOR, or edit $CONFIG_FILE by hand."; return 0; }
  [ -f "$CONFIG_FILE" ] || save_config
  ui_suspend "Opening $CONFIG_FILE in $ed ..."
  # shellcheck disable=SC2094,SC2086  # the terminal, not a file; EDITOR may have args
  $ed "$CONFIG_FILE" <"$tty" >"$tty" 2>&1 || true
  ui_resume
  if bash -n "$CONFIG_FILE" 2>/dev/null; then
    # shellcheck source=/dev/null
    . "$CONFIG_FILE"
    ok "Config saved and reloaded."
  else
    warn "$CONFIG_FILE has a syntax error - fix it before the next run:
$(bash -n "$CONFIG_FILE" 2>&1 | head -n 3)"
  fi
}

# Which interface game-mod-sync starts in without --cli / --tui.
edit_default_ui() {
  ui_title "Edit: default interface"
  ui_help "Default interface

game-mod-sync starts in this interface when run without --cli or --tui.
  cli   text menus and prompts in the terminal
  tui   full-screen dialog boxes with progress bars (needs whiptail or dialog)

Saved as DEFAULT_UI in the config file. Without a terminal (scripts, cron)
it always uses the text interface."
  ui_choose "Start game-mod-sync in which interface? (now: ${DEFAULT_UI:-cli})" \
    "cli - text menus and prompts" \
    "tui - full-screen dialog boxes" || return 0
  case $CHOICE in 1) DEFAULT_UI=cli ;; 2) DEFAULT_UI=tui ;; *) return 0 ;; esac
  save_config
  say "Default interface: $DEFAULT_UI (from the next start). --cli / --tui still choose for one run."
}

edit_menu() { # [ENTRY] - the profile to edit (else each action asks)
  local s g p d what="" rc
  EDIT_PRESET=${1:-}
  if [ -n "$EDIT_PRESET" ]; then
    IFS=$'\t' read -r s g p d <<<"$EDIT_PRESET"
    load_game "$g"
    what="$GAME_NAME / $p"
  fi
  while :; do
    ui_title "Edit${what:+: $what}"
    ui_help "Edit

Opened with E on a profile, the first three act on that profile.

Change a profile's server     run the first-time setup again for a profile
Server-managed files          files the server installs itself (e.g. AMP's
                              ValheimPlus): tick = leave the server's copy alone
Forget a profile's server     remove the setup; the next sync asks again
Edit the config file          open it in your editor (\$EDITOR, else nano or vi)
Show file locations           where settings, targets and plugins are
Refresh mod tags              look up every mod's client/server tags on
                              Thunderstore again (they're cached otherwise)
Default interface             text (cli) or full screen (tui) when started
                              without --cli / --tui"
    rc=0
    ui_choose "Edit what?${what:+ (server settings are for $what)}" \
      "Change a profile's server" \
      "Server-managed files (leave alone or sync over)" \
      "Forget a profile's server" \
      "Edit the config file" \
      "Show file locations" \
      "Refresh mod tags (client / server, from Thunderstore)" \
      "Default interface (now: ${DEFAULT_UI:-cli})" || rc=$?
    [ "$rc" = 0 ] || { EDIT_PRESET=""; return 0; }
    case $CHOICE in
      1) edit_change_server ;;
      2) edit_managed ;;
      3) edit_forget ;;
      4) edit_config_file ;;
      5) say "$(config_paths)" ;;
      6) tags_refresh ;;
      7) edit_default_ui ;;
      *) EDIT_PRESET=""; return 0 ;;
    esac
    ui_title "Edit: result"
    ui_report
  done
}

# ------------------------------------------------------------------- main --
gms_main() {
GAME_FILTER="" PROFILE_ARG="" DRY_RUN=0 ASSUME_YES=0 VERBOSE=0 DO_SETUP=0 REFRESH_TAGS=0 DO_WORLD=0 WORLD_ARG=""
while [ $# -gt 0 ]; do
  case $1 in
    -g|--game)    [ $# -ge 2 ] || die "$1 needs a game id"; GAME_FILTER=$2; shift ;;
    --game=*)     GAME_FILTER=${1#*=} ;;
    -p|--profile) [ $# -ge 2 ] || die "$1 needs a profile name"; PROFILE_ARG=$2; shift ;;
    --profile=*)  PROFILE_ARG=${1#*=} ;;
    -n|--dry-run) DRY_RUN=1 ;;
    -y|--yes)     ASSUME_YES=1 ;;
    -v|--verbose) VERBOSE=1 ;;
    --setup)      DO_SETUP=1 ;;
    --refresh-tags) REFRESH_TAGS=1 ;;
    --download-world) DO_WORLD=1 ;;
    --world)      [ $# -ge 2 ] || die "$1 needs a world name"; WORLD_ARG=$2; DO_WORLD=1; shift ;;
    --world=*)    WORLD_ARG=${1#*=}; DO_WORLD=1 ;;
    --cli|--tui)  ;;   # handled by the launcher
    --list)
      [ -s "$TARGETS_FILE" ] || { echo "No profiles set up yet."; exit 0; }
      while IFS=$'\t' read -r s g p b t r k; do
        load_game "$g"
        printf '%s%s / %s%s -> %s on %s  %s%s (%s)%s\n' "$B" "$GAME_NAME" "$p" "$N" "$(where_of "$b" "$r")" "${t#*@}" "$D" "$r" "$b" "$N"
        k=$(printf '%s' "$k" | tr ',' '\n' | sed -n 's/^+//p' | paste -sd' ' -)
        [ -z "$k" ] || printf '    leaves alone: %s\n' "$k"
      done <"$TARGETS_FILE"
      exit 0 ;;
    --games)
      for s in $SOURCES; do
        "${s}_init" || continue
        "${s}_list" | cut -f1 | sort -u | while IFS= read -r g; do
          load_game "$g"
          printf '%s%s%s (%s, %s, %s)\n    sync: %s\n    keep on server: %s\n    configs: %s%s\n    worlds: %s\n' \
            "$B" "$GAME_NAME" "$N" "$g" "$("${s}_label")" "$("${LOADER}_label" 2>/dev/null || echo "no loader")" \
            "$SYNC_DIRS" "${EXCLUDES:-nothing} $EXTRA_EXCLUDES" "${CONFIG_DIRS:-none}" \
            "${CONFIG_SKIP:+ (never copied: $CONFIG_SKIP)}" \
            "$(if [ -n "$WORLD_FORMAT" ]; then "${WORLD_FORMAT}_label" 2>/dev/null || echo "$WORLD_FORMAT (no plugin)"; else echo "no download"; fi)"
        done
      done
      exit 0 ;;
    --config)     config_paths; exit 0 ;;
    -h|--help)    usage; exit 0 ;;
    -V|--version) echo "$PROG $VERSION"; exit 0 ;;
    *) die "unknown option: $1 (see --help)" ;;
  esac
  shift
done

ui_start
command -v rsync >/dev/null 2>&1 || die "rsync is not installed"
command -v ssh   >/dev/null 2>&1 || die "ssh is not installed"
case "$(rsync --version 2>/dev/null)" in
  *"version 3."*|*"version 4."*) ;;
  *) warn "rsync 3.x is recommended (macOS: brew install rsync)" ;;
esac

collect_entries
[ ${#ENTRIES[@]} -gt 0 ] || die "no profiles with mods found${GAME_FILTER:+ for game '$GAME_FILTER'}"
if [ "$REFRESH_TAGS" = 1 ]; then tags_refresh; ui_finish; return 0; fi

# -p (or a single profile with -y): sync once and stop.
if [ -n "$PROFILE_ARG" ]; then
  pick=() games=""
  for e in "${ENTRIES[@]}"; do
    IFS=$'\t' read -r s g p d <<<"$e"
    if [ "$p" = "$PROFILE_ARG" ]; then pick+=("$e"); games="$games $g"; fi
  done
  [ ${#pick[@]} -gt 0 ] || die "no profile named '$PROFILE_ARG'${GAME_FILTER:+ for game '$GAME_FILTER'}"
  [ ${#pick[@]} -eq 1 ] || die "'$PROFILE_ARG' exists for several games ($games ) - add -g GAME"
  if [ "$DO_WORLD" = 1 ]; then run_download "${pick[0]}"; else run_entry "${pick[0]}"; fi
elif [ ${#ENTRIES[@]} -eq 1 ] && [ "$ASSUME_YES" = 1 ]; then
  if [ "$DO_WORLD" = 1 ]; then run_download "${ENTRIES[0]}"; else run_entry "${ENTRIES[0]}"; fi
elif [ "$DO_WORLD" = 1 ]; then
  die "--download-world needs a profile: add -p PROFILE (or press D on it in the menu)"
else
  # Main menu: after each sync (or "already in sync") come back here.
  while :; do
    entry_labels
    labels=("${LABELS[@]}")
    ui_title "Main menu"
    ui_help "Main menu

Pick a profile to sync it to its server. Each line shows the game and
profile, then where it syncs to (or \"not set up yet\": the first sync
asks for the server once).

A sync previews what changes, lets you pick config files, and asks
before changing anything. Afterwards you come back here.

Ok (Enter)  sync the highlighted profile
E           edit the highlighted profile: its server, server-managed files;
            also the config file and the mods' client/server tags
D           download a world from the highlighted profile's server into
            this computer's saves, to test mod updates on a copy of it
            (nothing on the server is changed)
Exit        quit (Tab moves to it)"
    UI_BUTTONS="Ok Exit" UI_KEYS="e:Edit d:Download"
    rc=0; ui_choose --no-cancel "Which profile do you want to sync?" "${labels[@]}" || rc=$?
    case $rc in
      0) ;;
      4) if [ "$UI_PRESSED" = Edit ]; then
           if [ "$CHOICE" -ge 1 ] && [ "$CHOICE" -le ${#ENTRIES[@]} ]; then edit_menu "${ENTRIES[$((CHOICE - 1))]}"
           else edit_menu; fi
           continue
         fi
         if [ "$UI_PRESSED" = Download ] && [ "$CHOICE" -ge 1 ] && [ "$CHOICE" -le ${#ENTRIES[@]} ]; then
           run_download "${ENTRIES[$((CHOICE - 1))]}"
           ui_title "Download result"
           ui_report
           DO_SETUP=0
           continue
         fi
         break ;;
      *) break ;;
    esac
    run_entry "${ENTRIES[$((CHOICE - 1))]}"
    ui_title "Sync result"
    ui_report
    DO_SETUP=0
  done
fi
ui_finish
}
