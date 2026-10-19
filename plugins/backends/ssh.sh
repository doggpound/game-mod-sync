# shellcheck shell=bash
# Server type: any server you can SSH into, where you type the game folder.
# Covers LinuxGSM, plain SteamCMD installs, Docker bind mounts, Pterodactyl
# volumes and anything else until it gets its own auto-discovering type.

ssh_label()        { printf 'Other (SSH, enter the game folder)'; }
ssh_default_user() { printf ''; }
ssh_manual()       { :; }   # the user typed the folder: don't ask them to pick it again

ssh_candidates() {
  local target=$1 marker=$2 root
  ui_title "Setup: game folder"
  ui_ask "Game server folder on the server (the one that contains $marker)" || return 0  # Back
  root=$ANSWER
  [ -n "$root" ] || die "a folder is required"
  # Resolve ~ and relative paths (relative to the SSH user's home) on the server.
  case $root in
    "~")   root="." ;;
    "~/"*) root=${root#"~/"} ;;
  esac
  root=$(rssh "$target" "cd && cd -- $(shq "$root") 2>/dev/null && pwd") \
    || die "that folder doesn't exist on the server"
  rssh "$target" "test -d $(shq "$root/$marker")" \
    || warn "$root/$marker doesn't exist yet - it will be created on the first sync"
  printf '%s\t%s\n' "$root" "$root"
}
