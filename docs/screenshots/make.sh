#!/usr/bin/env bash
# Takes the README's screenshots: builds game-mod-sync, starts the made-up demo
# setup (demo/: a Gale profile and an AMP-style server that shows up as
# game.example.com), runs the program (full screen and text) in a tmux
# terminal, presses the keys a person would, and saves each screen as
# docs/screenshots/<name>.svg (ansi2svg.py).
#
#   docs/screenshots/make.sh
#
# Run it after changing anything visible in either interface, and commit the
# new pictures (CI's are the reference: see DESIGN.md). Needs go, tmux and python3.
set -euo pipefail

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
REPO=$(cd "$HERE/../.." && pwd)
for t in go tmux python3; do
  command -v "$t" >/dev/null || { echo "make.sh needs $t" >&2; exit 1; }
done
WORK=$(mktemp -d)
SOCK="gms-shots-$$"
COLS=100 ROWS=30
cleanup() { tmux -L "$SOCK" kill-server 2>/dev/null || true; [ -n "${DEMO:-}" ] && kill "$DEMO" 2>/dev/null; rm -rf "$WORK"; }
trap cleanup EXIT

(cd "$REPO" && go build -ldflags "-X main.version=$(cat VERSION)" -o "$WORK/game-mod-sync" . &&
  go build -o "$WORK/demo" ./docs/screenshots/demo)
"$WORK/demo" -dir "$WORK/d" >/dev/null &
DEMO=$!
for _ in $(seq 1 100); do [ -s "$WORK/d/env" ] && break; sleep 0.1; done
[ -s "$WORK/d/env" ] || { echo "the demo didn't start" >&2; exit 1; }
envs=()
while IFS= read -r l; do envs+=(-e "$l"); done <"$WORK/d/env"

T() { tmux -L "$SOCK" "$@"; }
start() { # COMMAND...
  T kill-server 2>/dev/null || true
  T -f /dev/null new-session -d -x "$COLS" -y "$ROWS" -e TERM=xterm-256color -e COLORTERM=truecolor \
    "${envs[@]}" "$*; sleep 60"
}
waitfor() { # TEXT
  for _ in $(seq 1 150); do
    T capture-pane -p | grep -qF -- "$1" && { sleep 0.4; return 0; }
    sleep 0.2
  done
  echo "timed out waiting for: $1" >&2; T capture-pane -p >&2; exit 1
}
shot() { # NAME TITLE
  T capture-pane -e -N -p | python3 "$HERE/ansi2svg.py" --cols "$COLS" --rows "$ROWS" "$2" >"$HERE/$1.svg"
}

# Full screen: main menu, config files with one opened, review, world download.
start "$WORK/game-mod-sync --tui"
waitfor "Which profile do you want to sync?";      shot tui-menu "game-mod-sync --tui"
T send-keys Enter
waitfor "Ticked files are copied";                 T send-keys Down Right
waitfor "server: Off";                             shot tui-configs "game-mod-sync --tui"
T send-keys Left Enter
waitfor "Apply these changes";                     shot tui-review "game-mod-sync --tui"
T send-keys n
waitfor "Which profile do you want to sync?";      T send-keys d
waitfor "Which world do you want a copy of?";      shot tui-worlds "game-mod-sync --tui"
T send-keys Escape q

# Text: one profile, the config checklist, the review.
start "$WORK/game-mod-sync --cli -p MyProfile"
waitfor "numbers to tick";                         shot cli-configs "game-mod-sync --cli -p MyProfile"
T send-keys Enter
waitfor "Apply these changes";                     shot cli-review "game-mod-sync --cli -p MyProfile"
T send-keys n Enter

echo "Screenshots:"; for f in tui-menu tui-configs tui-review tui-worlds cli-configs cli-review; do echo "  $f.svg"; done
