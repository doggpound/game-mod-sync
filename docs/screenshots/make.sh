#!/usr/bin/env bash
# Takes the README's screenshots: runs game-mod-sync (text and full-screen) in
# a tmux terminal against a made-up demo setup (a Gale profile "MyProfile", an
# AMP server "Valheim01" at game.example.com - really a local folder behind
# the tests' stand-in ssh, see tests/lib.sh), presses the keys a person would,
# and saves each screen as docs/screenshots/<name>.svg.
#
#   docs/screenshots/make.sh
#
# Run it after changing anything you can see in either interface, and commit
# the new pictures. Needs tmux, whiptail, python3, rsync and perl.
set -euo pipefail

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO=$(cd "$HERE/../.." && pwd)
for t in tmux whiptail python3 rsync perl; do
  command -v "$t" >/dev/null || { echo "make.sh needs $t" >&2; exit 1; }
done

# ---- demo setup ------------------------------------------------------------------
# shellcheck source=/dev/null
. "$REPO/tests/lib.sh"
sandbox_init
unset GMS_TTY                        # a real terminal this time (tmux)
trap 'tmux -L gms-shots kill-server 2>/dev/null || true; sandbox_done' EXIT

P=$(gale_profile valheim MyProfile)
mod() { # FOLDER ID NAME - a mod in the profile
  fake_plugin_dll "$P/BepInEx/plugins/$1/${3}.dll" "$2" "$3" 1.0.0
}
mod Azumatt-AzuClock Azumatt.AzuClock AzuClock
mod ArgusMagnus-ServersideQoL argusmagnus.ServersideQoL ServersideQoL
mod ArgusMagnus-ServersideQoL_AutoStore argusmagnus.ServersideQoL.AutoStore ServersideQoL_AutoStore
mod Azumatt-FastLink Azumatt.FastLink FastLink
printf '[General]\nShow = true\n' >"$P/BepInEx/config/Azumatt.AzuClock.cfg"
printf '[General]\nEnabled = true\n\n[AutoStore]\nRange = 8\n' >"$P/BepInEx/config/argusmagnus.ServersideQoL.cfg"
printf '[General]\nRange = 6\n' >"$P/BepInEx/config/argusmagnus.ServersideQoL.AutoStore.cfg"
printf '[General]\nServers = 2\n' >"$P/BepInEx/config/Azumatt.FastLink.cfg"
# Thunderstore tags as Gale caches them (client / server side, no network needed)
cat >"$(GALE)/valheim/thunderstore_cache.json" <<'JSON'
[{"full_name":"Azumatt-AzuClock","categories":["Mods","Client-side","Server-side"]},
 {"full_name":"ArgusMagnus-ServersideQoL","categories":["Mods","Server-side"]},
 {"full_name":"ArgusMagnus-ServersideQoL_AutoStore","categories":["Mods","Server-side"]},
 {"full_name":"Azumatt-FastLink","categories":["Mods","Client-side"]}]
JSON

S=$(amp_instance Valheim01)
cp -r "$P/BepInEx/plugins/ArgusMagnus-ServersideQoL_AutoStore" "$P/BepInEx/plugins/Azumatt-FastLink" "$S/BepInEx/plugins/"
mkdir -p "$S/BepInEx/plugins/ArgusMagnus-ServersideQoL" "$S/BepInEx/plugins/Someone-OldMod"
echo old >"$S/BepInEx/plugins/ArgusMagnus-ServersideQoL/ServersideQoL.dll"
echo old >"$S/BepInEx/plugins/Someone-OldMod/OldMod.dll"
printf '[General]\nEnabled = true\n\n[AutoStore]\nRange = 4\n' >"$S/BepInEx/config/argusmagnus.ServersideQoL.cfg"
cp "$P/BepInEx/config/argusmagnus.ServersideQoL.AutoStore.cfg" "$S/BepInEx/config/"
mkdir -p "$GMS_CONFIG_DIR"
printf 'gale\tvalheim\tMyProfile\tamp\tamp@game.example.com\t%s\t\n' "$S" >"$GMS_CONFIG_DIR/targets.tsv"

# ---- terminal --------------------------------------------------------------------
# Fixed size and colours, so the pictures are the same on every machine:
# whiptail's Ubuntu-style purple theme, and the drawn lists' matching colours.
printf 'root=,magenta\ncheckbox=,magenta\nentry=,magenta\nlabel=magenta,\nactlistbox=,magenta\nhelpline=,magenta\nroottext=,magenta\nemptyscale=magenta\ndisabledentry=magenta,\n' >"$SANDBOX/palette"
COLS=100 ROWS=30
T() { tmux -L gms-shots "$@"; }
start() { # ARGS... - start game-mod-sync in a fresh terminal
  T kill-server 2>/dev/null || true
  T -f /dev/null new-session -d -x "$COLS" -y "$ROWS" \
    "env PATH='$PATH' HOME='$HOME' XDG_DATA_HOME='$XDG_DATA_HOME' XDG_CACHE_HOME='$XDG_CACHE_HOME' \
     GMS_CONFIG_DIR='$GMS_CONFIG_DIR' GMS_NO_SSH_MUX=1 TERM=xterm-256color LANG=C.UTF-8 \
     NEWT_COLORS_FILE='$SANDBOX/palette' GMS_TREE_ACCENT=5 GMS_TREE_HDR_BG='48;5;54' \
     '$REPO/game-mod-sync' $*; sleep 600"
  T set-option -g status off >/dev/null
}
wait_for() { # TEXT - until the screen shows TEXT (max 20 s)
  local i
  for i in $(seq 1 100); do
    T capture-pane -p | grep -qF -- "$1" && { sleep 0.4; return 0; }
    sleep 0.2
  done
  echo "timed out waiting for: $1" >&2; T capture-pane -p >&2; exit 1
}
keys() { T send-keys "$@"; sleep 0.4; }
shot() { # NAME TITLE - save the screen as NAME.svg
  T capture-pane -e -N -p | python3 "$HERE/ansi2svg.py" "$2" >"$HERE/$1.svg"
  echo "  $1.svg"
}

echo "Screenshots:"
# Text interface: menu, preview and config checklist; review
start --cli
wait_for "Which profile do you want to sync?"
keys 1 Enter
wait_for "Numbers to tick/untick"
shot cli-configs "game-mod-sync"
keys Enter
wait_for "Apply these changes to the server?"
shot cli-review "game-mod-sync"

# Full screen: menu, the mods preview, config files with a row opened, review
start --tui
wait_for "Which profile do you want to sync?"
shot tui-menu "game-mod-sync --tui"
keys Enter
wait_for "OK continues to"
shot tui-preview "game-mod-sync --tui"
keys Enter
wait_for "Ticked files are copied"
keys Down Right
shot tui-configs "game-mod-sync --tui"
keys Enter
wait_for "Apply these changes"
shot tui-review "game-mod-sync --tui"
