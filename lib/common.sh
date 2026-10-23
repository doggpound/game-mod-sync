# shellcheck shell=bash
# Shared helpers: ssh, config and targets file. UI-independent.
# Output and prompts live in lib/ui-text.sh and lib/ui-tui.sh.

# -------------------------------------------------------------------- ssh --
SSH_OPTS=()
case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*) GMS_NO_SSH_MUX=1 ;;  # Windows ssh builds lack ControlMaster
esac
if [ -z "${GMS_NO_SSH_MUX:-}" ]; then
  # Reuse one SSH connection for listing, preview and sync, so a password
  # (if you don't use keys) is typed once per run.
  SSH_CTL_DIR="/tmp/gms-ssh-$(id -u)"
  mkdir -p "$SSH_CTL_DIR" && chmod 700 "$SSH_CTL_DIR"
  SSH_OPTS=(-o ControlMaster=auto -o "ControlPath=$SSH_CTL_DIR/%C" -o ControlPersist=120)
fi
# shellcheck disable=SC2029  # remote commands are built and quoted on purpose
rssh() { ssh ${SSH_OPTS[@]+"${SSH_OPTS[@]}"} "$@"; }
rsh_cmd() {
  local s="ssh" o
  for o in ${SSH_OPTS[@]+"${SSH_OPTS[@]}"}; do s="$s $o"; done
  printf '%s' "$s"
}
# single-quote a string for the remote shell
shq() { printf "'%s'" "$(printf '%s' "$1" | sed "s/'/'\\\\''/g")"; }

# Windows user folders as WSL sees them (tests point this into their sandbox).
WIN_USERS_GLOB=${GMS_WIN_USERS:-/mnt/*/Users/*}

# ----------------------------------------------------------------- config --
CONFIG_KEYS="GALE_DATA_DIR LAST_BACKEND LAST_HOST LAST_USER AMP_INSTANCES_DIR EXTRA_EXCLUDES KEEP_BACKUPS DEFAULT_UI LOCAL_WORLDS"
save_config() {
  local k tmp
  mkdir -p "$CONFIG_DIR"
  tmp="$CONFIG_FILE.tmp.$$"
  {
    echo "# game-mod-sync config - shell syntax, read on every run."
    for k in $CONFIG_KEYS; do printf '%s=%q\n' "$k" "${!k-}"; done
  } >"$tmp"
  mv "$tmp" "$CONFIG_FILE"
}

# targets.tsv - one line per synced profile, tab separated:
#   source  game  profile  backend  user@host  remote game root  [kept]  [worlds]
# "kept" records answers about server-managed files: ",+pattern" = leave it
# alone, ",-pattern" = sync over it. "worlds" is the server's worlds folder,
# when it had to be typed in (lib/worlds.sh).
target_get() { # SOURCE GAME PROFILE -> "backend<TAB>target<TAB>root<TAB>kept"
  [ -f "$TARGETS_FILE" ] || return 1
  awk -F'\t' -v s="$1" -v g="$2" -v p="$3" \
    '$1 == s && $2 == g && $3 == p { print $4 "\t" $5 "\t" $6 "\t" $7; f = 1; exit } END { exit !f }' "$TARGETS_FILE"
}
target_set_kept() { # SOURCE GAME PROFILE KEPT
  local tmp="$TARGETS_FILE.tmp.$$"
  awk -F'\t' -v OFS='\t' -v s="$1" -v g="$2" -v p="$3" -v k="$4" \
    '$1 == s && $2 == g && $3 == p { $7 = k } { print }' "$TARGETS_FILE" >"$tmp"
  mv "$tmp" "$TARGETS_FILE"
}
target_worlds_get() { # SOURCE GAME PROFILE -> worlds folder (empty if none)
  [ -f "$TARGETS_FILE" ] || return 0
  awk -F'\t' -v s="$1" -v g="$2" -v p="$3" '$1 == s && $2 == g && $3 == p { print $8; exit }' "$TARGETS_FILE"
}
target_worlds_set() { # SOURCE GAME PROFILE FOLDER
  local tmp="$TARGETS_FILE.tmp.$$"
  awk -F'\t' -v OFS='\t' -v s="$1" -v g="$2" -v p="$3" -v w="$4" \
    '$1 == s && $2 == g && $3 == p { $7 = $7; $8 = w } { print }' "$TARGETS_FILE" >"$tmp"
  mv "$tmp" "$TARGETS_FILE"
}
target_set() { # SOURCE GAME PROFILE BACKEND TARGET ROOT
  local tmp="$TARGETS_FILE.tmp.$$"
  mkdir -p "$CONFIG_DIR"
  if [ -f "$TARGETS_FILE" ]; then
    awk -F'\t' -v s="$1" -v g="$2" -v p="$3" '!($1 == s && $2 == g && $3 == p)' "$TARGETS_FILE" >"$tmp"
  else
    : >"$tmp"
  fi
  printf '%s\t%s\t%s\t%s\t%s\t%s\n' "$@" >>"$tmp"
  mv "$tmp" "$TARGETS_FILE"
}
target_remove() { # SOURCE GAME PROFILE
  local tmp="$TARGETS_FILE.tmp.$$"
  [ -f "$TARGETS_FILE" ] || return 0
  awk -F'\t' -v s="$1" -v g="$2" -v p="$3" '!($1 == s && $2 == g && $3 == p)' "$TARGETS_FILE" >"$tmp"
  mv "$tmp" "$TARGETS_FILE"
}
