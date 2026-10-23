# shellcheck shell=bash
# World download: copy a world from the server into this computer's saves for
# the game, to try mod updates on a copy of it before syncing them. One way
# only: nothing on the server is changed.
#
# How a game stores a world comes from its world format plugin
# (plugins/worlds/<WORLD_FORMAT>.sh); where the worlds are comes from the game
# file and the server type. This file does the rest:
#   1. the worlds folder on the server (world_server_dir)
#   2. the worlds folder on this computer (world_local_dir)
#   3. a copy into a cache folder, repeated until no save happened during it
#      (world_fetch)
#   4. this computer's copy of the world moved to a backup, the new one put in
#      its place (world_install)

WORLDS_CACHE="${XDG_CACHE_HOME:-$HOME/.cache}/game-mod-sync/worlds"

# Does this game have a world download?
world_supported() { [ -n "$WORLD_FORMAT" ]; }

# SERVER_WORLDS_DIR: the server's worlds folder; RUNNING_WORLD: the world the
# server runs, if the server type knows it. Returns 1 on Back.
SERVER_WORLDS_DIR="" RUNNING_WORLD=""
world_server_dir() { # SOURCE GAME PROFILE BACKEND TARGET ROOT
  local src=$1 game=$2 profile=$3 backend=$4 target=$5 root=$6 specvar="SAVES_$4" res="" saves dir try
  SERVER_WORLDS_DIR="" RUNNING_WORLD=""
  if declare -F "${backend}_saves" >/dev/null && [ -n "${!specvar-}" ]; then
    res=$("${backend}_saves" "$target" "$root" "${!specvar}")
  fi
  saves=${res%%$'\t'*}
  case $res in *$'\t'*) RUNNING_WORLD=${res#*$'\t'} ;; esac
  # A folder typed in for this profile, then the server type's, then the
  # game's usual one.
  dir=$(target_worlds_get "$src" "$game" "$profile")
  if [ -z "$dir" ] && [ -n "$saves" ]; then
    case $saves in /*) ;; *) saves="$root/$saves" ;; esac
    dir="$saves/$SERVER_WORLDS"
  fi
  if [ -z "$dir" ] && [ -n "$SERVER_SAVES" ]; then
    try=$(rssh "$target" "cd && cd -- $(shq "$SERVER_SAVES/$SERVER_WORLDS") 2>/dev/null && pwd") || try=""
    dir=$try
  fi
  if [ -n "$dir" ] && rssh "$target" "test -d $(shq "$dir")"; then
    SERVER_WORLDS_DIR=$dir
    return 0
  fi
  [ -z "$dir" ] || warn "$dir doesn't exist on the server."
  ui_title "Server worlds folder"
  ui_help "Server worlds folder

The folder on the server where $GAME_NAME keeps its worlds. It's asked
once per profile and remembered (Edit > Change a profile's server asks
again). A path without a leading / is relative to the SSH user's home."
  ui_ask "$GAME_NAME's worlds folder on the server" || return 1
  [ -n "$ANSWER" ] || die "a folder is required"
  try=$ANSWER
  case $try in "~") try="." ;; "~/"*) try=${try#"~/"} ;; esac
  dir=$(rssh "$target" "cd && cd -- $(shq "$try") 2>/dev/null && pwd") \
    || die "$ANSWER doesn't exist on the server"
  target_worlds_set "$src" "$game" "$profile" "$dir"
  SERVER_WORLDS_DIR=$dir
}

# LOCAL_WORLDS (config): "game=folder|game=folder", the worlds folder picked
# for each game when more than one was found.
local_worlds_get() { # GAME
  local e rest=$LOCAL_WORLDS
  while [ -n "$rest" ]; do
    e=${rest%%|*}
    [ "${e%%=*}" = "$1" ] && { printf '%s' "${e#*=}"; return 0; }
    [ "$rest" = "${rest#*|}" ] && break
    rest=${rest#*|}
  done
  return 1
}
local_worlds_set() { # GAME FOLDER
  local e rest=$LOCAL_WORLDS out=""
  while [ -n "$rest" ]; do
    e=${rest%%|*}
    [ "${e%%=*}" = "$1" ] || out="${out:+$out|}$e"
    [ "$rest" = "${rest#*|}" ] && break
    rest=${rest#*|}
  done
  LOCAL_WORLDS="${out:+$out|}$1=$2"
  save_config
}

# LOCAL_WORLDS_DIR: this computer's worlds folder for the game. Searched for
# under the Windows user folders (WSL, Git Bash) and $HOME; asks which one
# when there are several, and remembers it. Returns 1 on Back.
LOCAL_WORLDS_DIR=""
world_local_dir() { # GAME
  local c
  local -a cands=()
  LOCAL_WORLDS_DIR=""
  if c=$(local_worlds_get "$1") && [ -d "$c" ]; then LOCAL_WORLDS_DIR=$c; return 0; fi
  if [ -n "$LOCAL_WORLDS_WIN" ]; then
    for c in $WIN_USERS_GLOB; do
      [ -d "$c/$LOCAL_WORLDS_WIN" ] && cands+=("$c/$LOCAL_WORLDS_WIN")
    done
    if [ -n "${USERPROFILE:-}" ] && command -v cygpath >/dev/null 2>&1; then
      c="$(cygpath -u "$USERPROFILE")/$LOCAL_WORLDS_WIN"
      [ -d "$c" ] && cands+=("$c")
    fi
  fi
  if [ -n "$LOCAL_WORLDS_HOME" ] && [ -d "$HOME/$LOCAL_WORLDS_HOME" ]; then cands+=("$HOME/$LOCAL_WORLDS_HOME"); fi
  case ${#cands[@]} in
    0) die "can't find $GAME_NAME's worlds folder on this computer${LOCAL_WORLDS_WIN:+ (looked for $LOCAL_WORLDS_WIN)}.
       Start $GAME_NAME once so it exists, or set it in $CONFIG_FILE:
         LOCAL_WORLDS='$1=/path/to/worlds'" ;;
    1) LOCAL_WORLDS_DIR=${cands[0]} ;;
    *) ui_title "Local worlds folder"
       ui_choose "Which of these is your $GAME_NAME worlds folder?" "${cands[@]}" || return 1
       LOCAL_WORLDS_DIR=${cands[$((CHOICE - 1))]}
       local_worlds_set "$1" "$LOCAL_WORLDS_DIR"
       say "Saved to $CONFIG_FILE" ;;
  esac
}

# rsync filter for one world: only the names that make it up.
WORLD_FILTER=()
world_filter() { # WORLD
  local p
  WORLD_FILTER=()
  while IFS= read -r p; do
    [ -n "$p" ] && WORLD_FILTER+=("--include=/$p" "--include=/$p/***")
  done < <("${WORLD_FORMAT}_paths" "$1")
  WORLD_FILTER+=("--exclude=*")
}

# The world's save stamp in a local folder (the downloaded copy, or ours).
world_local_stamp() { # FOLDER WORLD
  (cd "$1" 2>/dev/null && sh -c "$("${WORLD_FORMAT}_stamp_sh" "$2")") || true
}

# Copy WORLD from the server into the cache. The server may save while it's
# copied, so the save stamp is read before and after; the copy is good when
# both match the copy's own stamp. Tried 3 times. Sets WORLD_STAMP.
WORLD_STAMP=""
world_fetch() { # TARGET SERVER_DIR WORLD STAGE
  local target=$1 sdir=$2 world=$3 stage=$4 stamp_sh before after mine rc try
  stamp_sh=$("${WORLD_FORMAT}_stamp_sh" "$world")
  world_filter "$world"
  mkdir -p "$stage"
  for try in 1 2 3; do
    ui_wait "Checking $world's last save on the server ..." \
      rssh "$target" "cd $(shq "$sdir") && $stamp_sh" || die "can't read $sdir on the server"
    before=$WAIT_OUT
    [ -n "$before" ] || die "$world has no complete save on the server"
    rc=0
    ui_title "Downloading $world"
    ui_gauge "Downloading $world ($before) from the server" \
      rsync_percent -rlt -s --delete -e "$(rsh_cmd)" "${WORLD_FILTER[@]}" "$target:$sdir/" "$stage/" || rc=$?
    # 24: files vanished during the copy - a save; the stamps below tell.
    [ "$rc" = 0 ] || [ "$rc" = 24 ] || die "downloading $world failed
$(printf '%s\n' "$WAIT_ERR" | tail -n 3)"
    ui_wait "Checking $world's last save on the server ..." \
      rssh "$target" "cd $(shq "$sdir") && $stamp_sh" || die "can't read $sdir on the server"
    after=$WAIT_OUT
    mine=$(world_local_stamp "$stage" "$world")
    if [ "$before" = "$after" ] && [ "$after" = "$mine" ]; then WORLD_STAMP=$mine; return 0; fi
    warn "The server saved $world while it was being copied - copying again ($try of 3)."
  done
  die "the server kept saving $world during the copy - try again in a minute"
}

# Move this computer's copy of WORLD (if any) to a backup, then put the
# downloaded one in its place. Sets WORLD_BACKUP (empty: nothing to back up).
WORLD_BACKUP=""
world_install() { # STAGE WORLD LOCAL_DIR TIMESTAMP
  local stage=$1 world=$2 ldir=$3 ts=$4 p broot
  local -a moved=()
  broot="${ldir%/*}/game-mod-sync-backups"
  WORLD_BACKUP=""
  while IFS= read -r p; do
    if [ -z "$p" ] || [ ! -e "$ldir/$p" ]; then continue; fi
    WORLD_BACKUP="$broot/$ts"
    mkdir -p "$WORLD_BACKUP"
    mv -- "$ldir/$p" "$WORLD_BACKUP/" || die "can't move $ldir/$p to $WORLD_BACKUP"
    moved+=("$p")
  done < <("${WORLD_FORMAT}_paths" "$world")
  world_filter "$world"
  if ! ui_gauge "Putting $world in $ldir" rsync_percent -rlt "${WORLD_FILTER[@]}" "$stage/" "$ldir/"; then
    # Put ours back as it was.
    for p in ${moved[@]+"${moved[@]}"}; do rm -rf -- "${ldir:?}/$p"; mv -- "$WORLD_BACKUP/$p" "$ldir/"; done
    die "copying $world into $ldir failed - your copy was left as it was
$(printf '%s\n' "$WAIT_ERR" | tail -n 3)"
  fi
  if [ "${KEEP_BACKUPS:-0}" -gt 0 ] && [ -d "$broot" ]; then
    (cd "$broot" && ls -1 | sort -r | tail -n +$((KEEP_BACKUPS + 1)) | xargs -r rm -rf --)
  fi
  return 0
}

# The whole download, for one profile's server. WORLD_ARG picks the world
# (else the one the server runs with -y, else a menu).
world_download() { # SOURCE GAME PROFILE
  local src=$1 game=$2 profile=$3 t backend target root kept name detail i ts mine stage note
  local -a names=() details=() labels=()
  t=$(target_get "$src" "$game" "$profile") || die "no server set up for '$profile'"
  IFS=$'\t' read -r backend target root kept <<<"$t"
  declare -F "${WORLD_FORMAT}_list_sh" >/dev/null \
    || die "$GAME_NAME's world format '$WORLD_FORMAT' has no plugin (plugins/worlds/$WORLD_FORMAT.sh)"

  world_server_dir "$src" "$game" "$profile" "$backend" "$target" "$root" || return 0
  ui_wait "Looking for worlds in $SERVER_WORLDS_DIR ..." \
    rssh "$target" "cd $(shq "$SERVER_WORLDS_DIR") && $("${WORLD_FORMAT}_list_sh")" \
    || die "can't list $SERVER_WORLDS_DIR on the server
$(printf '%s\n' "$WAIT_ERR" | tail -n 3)"
  # The world the server runs first, then the rest in the format's order.
  while IFS=$'\t' read -r name detail; do
    [ -n "$name" ] || continue
    if [ "$name" = "$RUNNING_WORLD" ]; then
      names=("$name" ${names[@]+"${names[@]}"}); details=("$detail" ${details[@]+"${details[@]}"})
    else
      names+=("$name"); details+=("$detail")
    fi
  done <<<"$WAIT_OUT"
  [ ${#names[@]} -gt 0 ] || die "no $GAME_NAME worlds in $SERVER_WORLDS_DIR on the server"

  if [ -n "$WORLD_ARG" ]; then
    CHOICE=0
    for i in "${!names[@]}"; do
      if [ "${names[$i]}" = "$WORLD_ARG" ]; then CHOICE=$((i + 1)); fi
    done
    [ "$CHOICE" -gt 0 ] || die "no world named '$WORLD_ARG' on the server (there: ${names[*]})"
  elif [ "$ASSUME_YES" = 1 ] || [ ${#names[@]} -eq 1 ]; then
    if [ ${#names[@]} -gt 1 ] && [ "${names[0]}" != "$RUNNING_WORLD" ]; then
      die "several worlds on the server (${names[*]}) - pick one with --world NAME"
    fi
    CHOICE=1
  else
    for i in "${!names[@]}"; do
      note=""
      if [ "${names[$i]}" = "$RUNNING_WORLD" ]; then note=", the server runs this one"; fi
      labels+=("${names[$i]}  ${D}${details[$i]}$note${N}")
    done
    ui_title "Download a world: $GAME_NAME / $profile"
    ui_help "Download a world

The worlds on the server ($(where_of "$backend" "$root")). The one you pick
is copied into $GAME_NAME's worlds on this computer, so you can play a
copy of it with your updated mods before syncing them. Your own copy of
that world (if any) is moved to a backup first. Nothing on the server is
changed."
    ui_choose "Which world do you want a copy of?" "${labels[@]}" || return 0
  fi
  name=${names[$((CHOICE - 1))]} detail=${details[$((CHOICE - 1))]}

  world_local_dir "$game" || return 0
  mine=$(world_local_stamp "$LOCAL_WORLDS_DIR" "$name")
  say ""
  say "${B}World $name${N} ($detail)"
  say "  from  $(where_of "$backend" "$root") on ${target#*@}: $SERVER_WORLDS_DIR"
  say "  to    $LOCAL_WORLDS_DIR"
  if [ -n "$mine" ]; then
    say "  Your copy ($mine) is moved to ${LOCAL_WORLDS_DIR%/*}/game-mod-sync-backups first."
  fi
  say "Nothing on the server is changed. Close $GAME_NAME on this computer first."
  [ -z "$WORLD_NOTE" ] || say "${YEL}$WORLD_NOTE${N}"
  if [ "$DRY_RUN" = 1 ]; then ok "Dry run - nothing downloaded."; return 0; fi
  if [ "$ASSUME_YES" != 1 ]; then
    ui_title "Download $name"
    ui_confirm "Download $name${mine:+ and replace your copy}?" || { say "Cancelled - nothing changed."; return 0; }
  fi

  stage="$WORLDS_CACHE/$game"
  world_fetch "$target" "$SERVER_WORLDS_DIR" "$name" "$stage"
  ts=$(date +%Y%m%d-%H%M%S)
  world_install "$stage" "$name" "$LOCAL_WORLDS_DIR" "$ts"
  ok "Downloaded $name ($WORLD_STAMP) to $LOCAL_WORLDS_DIR."
  [ -z "$WORLD_BACKUP" ] || say "Your previous copy: $WORLD_BACKUP"
  say "Start $GAME_NAME with this profile and pick $name to play the copy."
}
