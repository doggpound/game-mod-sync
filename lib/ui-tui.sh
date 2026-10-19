# shellcheck shell=bash
# TUI: whiptail (or dialog) boxes. Used by `game-mod-sync --tui`.
# Same functions as lib/ui-text.sh - see the list there.
#
# Lines from say/ok/warn are collected and shown at the top of the next box,
# so the preview appears inside the "Apply?" box and results in a final box.
# info shows a short non-blocking status box while work runs.
#
# One screen for the whole run: whiptail and dialog normally switch the
# terminal to its alternate screen on start and back on exit, so between two
# boxes the shell flashes through. Instead we switch once (_tui_screen_on) and
# run the tools with a terminal description that has those switches removed
# (_tui_term), then switch back once at the end.

B='' D='' RED='' GRN='' YEL='' CYN='' N=''   # no ANSI colours inside boxes

TUI_BIN=""
for _t in ${GMS_TUI_BIN:-whiptail dialog}; do
  if command -v "$_t" >/dev/null 2>&1; then TUI_BIN=$_t; break; fi
done
TUI_TITLE="game-mod-sync"
# Top-left of the screen behind the boxes, and each box's own title.
TUI_BACKTITLE="game-mod-sync ${VERSION:-}"
UI_TITLE=""
# _tui_title DEFAULT -> TUI_TITLE = the title set with ui_title, else DEFAULT.
# Called in the main shell (boxes may run in subshells).
_tui_title() { TUI_TITLE=${UI_TITLE:-$1}; UI_TITLE=""; }
# Keys always come from the terminal, even inside loops that read from a pipe.
TUI_TTY=${GMS_TTY:-/dev/tty}
NOTES=""
TUI_SCREEN=""     # set while we hold the alternate screen
TUI_ENV=()        # env for the tool: TERM/TERMINFO without screen switching

# Build (once, cached) a copy of $TERM's description without smcup/rmcup.
# Needs infocmp + tic (ncurses); without them the tools still work, they just
# flash the shell between boxes.
_tui_term() {
  local base=${TERM:-xterm} name dir
  name="${base}-gms"
  dir="${XDG_CACHE_HOME:-$HOME/.cache}/game-mod-sync/terminfo"
  command -v infocmp >/dev/null 2>&1 && command -v tic >/dev/null 2>&1 || return 0
  if ! TERMINFO="$dir" infocmp "$name" >/dev/null 2>&1; then
    mkdir -p "$dir" || return 0
    infocmp -x "$base" 2>/dev/null \
      | sed -e "s/^$base|[^,]*,/$name|$base without alternate screen,/" -e 's/smcup=[^,]*,//; s/rmcup=[^,]*,//' \
      | tic -x -o "$dir" - >/dev/null 2>&1 || return 0
  fi
  TERMINFO="$dir" infocmp "$name" >/dev/null 2>&1 && TUI_ENV=(TERM="$name" TERMINFO="$dir")
  return 0
}

_tui_screen_on() {
  [ -n "$TUI_SCREEN" ] && return 0
  # Only the main shell may switch screens: a subshell's EXIT trap would
  # switch back early. ui_start does this up front for the main shell.
  [ "${BASHPID:-$$}" = "$$" ] || return 0
  TUI_SCREEN=1
  _tui_term
  tput smcup >&2 2>/dev/null || true
  tput clear >&2 2>/dev/null || true
  trap '_tui_screen_off' EXIT
}
_tui_screen_off() {
  [ -n "$TUI_SCREEN" ] || return 0
  TUI_SCREEN=""
  tput clear >&2 2>/dev/null || true
  tput rmcup >&2 2>/dev/null || true
}

ui_start() { if [ -n "$TUI_BIN" ] && [ -t 2 ]; then _tui_screen_on; fi; return 0; }

_note() { NOTES="${NOTES:+$NOTES
}$*"; }
info() { [ -n "$TUI_BIN" ] && [ -t 2 ] && { _tui_title "Please wait"; _tui_infobox "$*"; }; return 0; }
say()  { _note "$*"; }
ok()   { _note "$*"; }
warn() { _note "Warning: $*"; }
die() {
  local msg="Error: $*"
  if [ -n "$TUI_BIN" ] && [ -t 2 ]; then
    _tui_title "Error"
    _tui_box --msgbox "${NOTES:+$NOTES

}$msg" 0 >/dev/null || true
  fi
  _tui_screen_off
  printf '%s%s\n' "${NOTES:+$NOTES
}" "$msg" >&2
  exit 1
}

# Size a box to its text, within the terminal. Sets TUI_H / TUI_W.
_tui_size() { # TEXT EXTRA_ROWS
  local lines cols tl tc
  cols=$(printf '%s\n' "$1" | awk '{ if (length > m) m = length } END { print m + 0 }')
  tl=$(tput lines 2>/dev/null || echo 24); tc=$(tput cols 2>/dev/null || echo 80)
  TUI_W=$((cols + 6)); [ "$TUI_W" -lt 50 ] && TUI_W=50; [ "$TUI_W" -gt $((tc - 4)) ] && TUI_W=$((tc - 4))
  # Count wrapped lines too, so long paths don't push the buttons off the box.
  lines=$(printf '%s\n' "$1" | awk -v w=$((TUI_W - 10)) '{ n += (length > w) ? int((length + w - 1) / w) : 1 } END { print n + 0 }')
  TUI_H=$((lines + $2 + 8)); [ "$TUI_H" -gt $((tl - 2)) ] && TUI_H=$((tl - 2))
  return 0
}

# Menus and checklists: wide enough for the longest label, tall enough for
# the question plus ROWS list lines (labels don't wrap, they're cut).
_tui_list_size() { # ROWS LABEL...
  local rows=$1 w
  shift
  _tui_size "$TUI_TEXT
$(printf '      %s\n' "$@")" 0
  w=$TUI_W
  # dialog draws a frame around the list, whiptail does not
  if [ "$TUI_BIN" = dialog ]; then _tui_size "$TUI_TEXT" $((rows - 1)); else _tui_size "$TUI_TEXT" $((rows - 3)); fi
  TUI_W=$w
}

# _tui_box TYPE TEXT EXTRA_ROWS [type args...] -> result on stdout, exit code of the tool
# Set TUI_OPTS (array) for options that must come before the box type (e.g. --defaultno).
TUI_OPTS=()
_tui_box() {
  local type=$1 text=$2 extra=$3 rc=0
  local -a pre=(--backtitle "$TUI_BACKTITLE" --title "$TUI_TITLE")
  shift 3
  _tui_size "$text" "$extra"
  pre+=(${TUI_OPTS[@]+"${TUI_OPTS[@]}"})
  TUI_OPTS=()
  _tui_screen_on
  env ${TUI_ENV[@]+"${TUI_ENV[@]}"} "$TUI_BIN" "${pre[@]}" "$type" "$text" "$TUI_H" "$TUI_W" "$@" <"$TUI_TTY" 3>&1 1>&2 2>&3 || rc=$?
  return "$rc"
}

_tui_infobox() {
  _tui_size "$1" 0
  _tui_screen_on
  # An infobox exits immediately; with screen switching it would vanish, which
  # the screen-switch-free TERM from _tui_term prevents (TERM=ansi as fallback).
  local -a envs=(${TUI_ENV[@]+"${TUI_ENV[@]}"})
  [ ${#envs[@]} -gt 0 ] || envs=(TERM=ansi)
  if [ "$TUI_BIN" = whiptail ]; then
    env "${envs[@]}" whiptail --backtitle "$TUI_BACKTITLE" --title "$TUI_TITLE" --infobox "$1" 7 "$TUI_W" >&2
  else
    env "${envs[@]}" dialog --backtitle "$TUI_BACKTITLE" --title "$TUI_TITLE" --infobox "$1" 5 "$TUI_W" >&2
  fi
}

# Notes too tall for one box (e.g. a long preview) are shown first in a
# scrollable text box, so the question box below stays small and keyboard-friendly.
_tui_flush_long_notes() { # [EXTRA_ROWS the box below needs besides the notes]
  local tl f
  [ -n "$NOTES" ] || return 0
  tl=$(tput lines 2>/dev/null || echo 24)
  [ "$(printf '%s\n' "$NOTES" | wc -l)" -gt $((tl - 12 - ${1:-0})) ] || return 0
  local next=$TUI_TITLE
  f=$(mktemp "${TMPDIR:-/tmp}/gms-tui.XXXXXX")
  # Say where OK leads, so this box doesn't read like a question.
  printf '%s\n\n(OK continues to: %s)\n' "$NOTES" "$next" >"$f"
  _tui_size "$NOTES

(OK continues to: $next)" 0
  _tui_screen_on
  TUI_TITLE="Details"
  env ${TUI_ENV[@]+"${TUI_ENV[@]}"} "$TUI_BIN" --backtitle "$TUI_BACKTITLE" --title "$TUI_TITLE" --textbox "$f" "$TUI_H" "$TUI_W" <"$TUI_TTY" 3>&1 1>&2 2>&3 || true
  TUI_TITLE=$next
  rm -f -- "$f"
  NOTES=""
}

_tui_take_notes() { # QUESTION [EXTRA_ROWS] -> TUI_TEXT = pending notes + question; clears notes
  _tui_flush_long_notes "${2:-0}"
  NOTES=$(printf '%s\n' "$NOTES" | sed '/./,$!d')   # drop leading blank lines
  TUI_TEXT=$1
  [ -n "$NOTES" ] && TUI_TEXT="$NOTES

$1"
  NOTES=""
  return 0
}

# ---- waits: a progress box while a command runs ------------------------------
# The command runs in the background; its exit status goes to a file so the
# feeder (which runs in a pipeline subshell and can't reap it) sees it finish.
_bg_start() {
  _BG_OUT=$(mktemp "${TMPDIR:-/tmp}/gms.XXXXXX"); _BG_ERR=$(mktemp "${TMPDIR:-/tmp}/gms.XXXXXX")
  _BG_RC=$(mktemp "${TMPDIR:-/tmp}/gms.XXXXXX")
  { "$@" </dev/null >"$_BG_OUT" 2>"$_BG_ERR"; echo $? >"$_BG_RC"; } &
  _BG_PID=$!
}
_bg_done() { [ -s "$_BG_RC" ]; }
_bg_finish() {
  local rc
  wait "$_BG_PID" 2>/dev/null || true
  rc=$(cat "$_BG_RC" 2>/dev/null); rc=${rc:-1}
  WAIT_OUT=$(cat "$_BG_OUT"); WAIT_ERR=$(cat "$_BG_ERR")
  rm -f -- "$_BG_OUT" "$_BG_ERR" "$_BG_RC"
  return "$rc"
}

# One gauge window fed from stdin (lines "PCT" or "XXX/PCT/text/XXX" blocks).
# It stays up and is updated in place - no redrawing, no flicker.
_tui_gauge_box() { # MSG
  _tui_size "$1  (999s)" 0
  TUI_H=$((TUI_H - 2))
  _tui_screen_on
  env ${TUI_ENV[@]+"${TUI_ENV[@]}"} "$TUI_BIN" --backtitle "$TUI_BACKTITLE" --title "$TUI_TITLE" --gauge "$1" "$TUI_H" "$TUI_W" 0 >&2 2>/dev/null || true
}

# Drive the bar while the background command runs. The length is unknown, so
# the bar eases towards 95% (halfway after ~7s) and fills when it's done.
_tui_wait_feed() { # MSG
  local msg=$1 start=$SECONDS q=0 p=-1 np s=-1 ns
  while ! _bg_done; do
    q=$((q + (9500 - q) / 40))
    np=$((q / 100)) ns=$((SECONDS - start))
    if [ "$ns" != "$s" ]; then
      printf 'XXX\n%d\n%s  (%ds)\nXXX\n' "$np" "$msg" "$ns"; s=$ns p=$np
    elif [ "$np" != "$p" ]; then
      printf '%d\n' "$np"; p=$np
    fi
    sleep 0.25
  done
  printf '100\n'
  sleep 0.15
}

WAIT_OUT="" WAIT_ERR=""
ui_wait() {
  local msg=$1 rc=0
  shift
  _tui_title "Please wait"
  _bg_start "$@"
  if [ -n "$TUI_BIN" ] && [ -t 2 ]; then
    _tui_screen_on
    _tui_wait_feed "$msg" | _tui_gauge_box "$msg"
  fi
  _bg_finish || rc=$?
  return "$rc"
}

# Pass percentages through, relabelling the box once copying really starts.
# While rsync is still checking files (no percentages yet), tick the elapsed
# time so the box visibly moves. (bash read, not awk: awk buffers piped input.)
_tui_gauge_label() { # MSG
  local msg=$1 p started="" start=$SECONDS s=-1 rc
  while :; do
    rc=0; IFS= read -r -t 1 p || rc=$?
    if [ "$rc" -gt 128 ]; then           # timeout: no new progress this second
      if [ -z "$started" ] && [ "$((SECONDS - start))" != "$s" ]; then
        s=$((SECONDS - start))
        printf 'XXX\n0\n%s - checking files  (%ds)\nXXX\n' "$msg" "$s"
      fi
      continue
    fi
    [ "$rc" -eq 0 ] || break             # end of input
    if [ -z "$started" ] && [ "$p" -gt 0 ] 2>/dev/null; then
      printf 'XXX\n%s\n%s - copying\nXXX\n' "$p" "$msg"; started=1
    elif [ -n "$started" ]; then
      printf '%s\n' "$p"
    fi
  done
}

# Known progress: CMD prints percentages, one per line.
ui_gauge() {
  local msg=$1 rc=0 errf
  shift
  _tui_title "Syncing"
  errf=$(mktemp "${TMPDIR:-/tmp}/gms.XXXXXX")
  if [ -n "$TUI_BIN" ] && [ -t 2 ]; then
    _tui_screen_on
    # Until real progress starts (rsync checksums everything first), say so.
    if "$@" </dev/null 2>"$errf" | _tui_gauge_label "$msg" | _tui_gauge_box "$msg - checking files  (0s)"
    then rc=0; else rc=${PIPESTATUS[0]}; fi
  else
    "$@" </dev/null 2>"$errf" >/dev/null || rc=$?
  fi
  WAIT_OUT=""; WAIT_ERR=$(cat "$errf"); rm -f -- "$errf"
  return "$rc"
}

# Give the plain terminal to a command that may prompt (ssh password, host key).
ui_suspend() {
  _tui_screen_off
  printf '%s\n' "$1" >&2
}
ui_resume() { ui_start; }


_tui_need() {
  [ -n "$TUI_BIN" ] || { printf 'error: game-mod-sync --tui needs whiptail or dialog (apt install whiptail) - or use game-mod-sync --cli\n' >&2; exit 1; }
  if ! [ -t 2 ] || ! { : <"$TUI_TTY"; } 2>/dev/null; then
    printf 'error: game-mod-sync --tui needs a terminal - use game-mod-sync --cli -g GAME -p PROFILE -y for scripts\n' >&2
    exit 1
  fi
  _tui_screen_on   # here, in the main shell - boxes may run in subshells
}

# ui_view TEXT: a long text in a scrollable box (title from ui_title).
ui_view() {
  local f
  _tui_title "Details"
  if [ -z "$TUI_BIN" ] || [ ! -t 2 ]; then printf '%s\n' "$1" >&2; return 0; fi
  f=$(mktemp "${TMPDIR:-/tmp}/gms-tui.XXXXXX")
  printf '%s\n' "$1" >"$f"
  _tui_size "$1" 0
  _tui_screen_on
  env ${TUI_ENV[@]+"${TUI_ENV[@]}"} "$TUI_BIN" --backtitle "$TUI_BACKTITLE" --title "$TUI_TITLE" --textbox "$f" "$TUI_H" "$TUI_W" <"$TUI_TTY" 3>&1 1>&2 2>&3 || true
  rm -f -- "$f"
}
UI_EXTRA="" UI_LIST_HEADER="" UI_DETAILS=()
# shellcheck source=lib/ui-tree.sh
. "$APP_DIR/lib/ui-tree.sh"

# ui_title TITLE: the title of the next box (else a generic one per box type).
ui_title() { UI_TITLE=$1; }
# ui_help TEXT: what H shows on the next menu or checklist (keys are added).
# UI_BUTTONS="Ok Exit": the next ui_choose's buttons, left to right;
# pressing one other than Ok / Back returns 4 with UI_PRESSED set to it.
# UI_KEYS="e:Edit": extra keys for the next ui_choose, acting on the highlighted
# row (CHOICE); they return 4 with UI_PRESSED too.
UI_HELP="" UI_BUTTONS="" UI_PRESSED="" UI_KEYS=""
ui_help() { UI_HELP=$1; }

# Our own lists (lib/ui-tree.sh) need a terminal of at least this size;
# smaller ones get the plain whiptail/dialog version.
_tui_tree_ok() {
  [ -n "$TUI_BIN" ] && [ -t 2 ] && [ "$(tput cols 2>/dev/null || echo 0)" -ge 60 ] \
    && [ "$(tput lines 2>/dev/null || echo 0)" -ge 16 ]
}
_tui_help_text() { # MODE [noback] -> TREE_HELP = UI_HELP + the keys for MODE
  local keys
  keys="Keys
  Up / Down, PgUp / PgDn, Home / End   move
  1-9                                  jump to that row"
  if [ "$1" = check ]; then keys="$keys
  Space                                tick / untick
  Right / Left                         show / hide a row's details (rows with an arrow)
  + / -                                show / hide all details"; fi
  keys="$keys
  Tab / Shift+Tab                      move between the buttons
  Enter                                press the highlighted button"
  [ "${2:-}" = noback ] || keys="$keys
  Esc                                  Back"
  keys="$keys
  H                                    this help"
  TREE_HELP="${UI_HELP:+$UI_HELP

}$keys"
  UI_HELP=""
}

# The Cancel button is labelled "Back" everywhere: it returns 1 and the
# caller goes back one step.
_tui_back_opts() {
  if [ "$TUI_BIN" = dialog ]; then TUI_BACK=(--cancel-label Back); else TUI_BACK=(--cancel-button Back); fi
}
TUI_BACK=()

ANSWER=""
ui_ask() {
  local out
  _tui_title "Input"
  _tui_need
  _tui_take_notes "$1"
  _tui_back_opts
  TUI_OPTS=("${TUI_BACK[@]}")
  out=$(_tui_box --inputbox "$TUI_TEXT" 2 "${2:-}") || return 1
  ANSWER=${out:-${2:-}}
}

ui_confirm() {
  _tui_title "Confirm"
  _tui_need
  _tui_take_notes "$1"
  TUI_OPTS=(--defaultno)
  _tui_box --yesno "$TUI_TEXT" 0 >/dev/null
}

ui_confirm_yes() {
  _tui_title "Confirm"
  _tui_need
  _tui_take_notes "$1"
  _tui_box --yesno "$TUI_TEXT" 0 >/dev/null
}

CHOICE=0
ui_choose() { # [--no-cancel] QUESTION LABEL...  (returns 1 on Back)
  local prompt i=1 label out n rows
  local -a items=() opt=()
  _tui_back_opts
  if [ "$1" = --no-cancel ]; then
    shift
    if [ "$TUI_BIN" = dialog ]; then opt=(--no-cancel); else opt=(--nocancel); fi
  else
    opt=("${TUI_BACK[@]}")
  fi
  prompt=$1
  shift
  n=$#
  [ "$n" -gt 0 ] || die "nothing to choose from"
  _tui_title "Choose"
  _tui_need
  for label in "$@"; do items+=("$i" "$label"); i=$((i + 1)); done
  _tui_take_notes "$prompt" $((n + 10))
  local b="Ok Back" xb=${UI_BUTTONS:-} xk=${UI_KEYS:-} rc=0
  UI_BUTTONS="" UI_KEYS=""
  [ ${#opt[@]} -gt 0 ] && [ "${opt[0]}" != "${TUI_BACK[0]:-}" ] && b=Ok
  [ -z "$xb" ] || b=$xb
  if _tui_tree_ok; then
    case " $b " in *" Back "*) _tui_help_text menu ;; *) _tui_help_text menu noback ;; esac
    TREE_KEYS=$xk
    _tree_list menu "$TUI_TITLE" "$TUI_TEXT" "" "$b" "$@" || rc=$?
    UI_PRESSED=$TREE_PRESSED
    return "$rc"
  fi
  UI_HELP=""
  # Small terminal: extra buttons (other than Ok / Back) and keys become list rows.
  local -a xbtn=()
  for i in $b; do case $i in Ok|Back) ;; *) xbtn+=("$i") ;; esac; done
  for i in $xk; do xbtn+=("${i#*:}"); done
  if [ ${#xbtn[@]} -gt 0 ]; then
    i=$((n + 1))
    for label in "${xbtn[@]}"; do items+=("$i" "$label"); i=$((i + 1)); done
    set -- "$@" "${xbtn[@]}"
  fi
  rows=$n; [ "$rows" -gt 15 ] && rows=15
  # Width must fit the longest label too.
  _tui_list_size "$rows" "$@"
  _tui_screen_on
  out=$(env ${TUI_ENV[@]+"${TUI_ENV[@]}"} "$TUI_BIN" --backtitle "$TUI_BACKTITLE" --title "$TUI_TITLE" ${opt[@]+"${opt[@]}"} --menu "$TUI_TEXT" "$TUI_H" "$TUI_W" "$rows" "${items[@]}" <"$TUI_TTY" 3>&1 1>&2 2>&3) || return 1
  CHOICE=$out
  if [ ${#xbtn[@]} -gt 0 ] && [ "$CHOICE" -gt "$n" ]; then UI_PRESSED=${xbtn[$((CHOICE - n - 1))]}; return 4; fi
  return 0
}

# Tick boxes: ui_checklist QUESTION LABEL on|off ... -> CHECKED="1 3" (1-based).
# Returns 1 on Back (nothing changes).
CHECKED=""
# With UI_EXTRA set, a first row "> UI_EXTRA (tick, then OK)" is added
# (whiptail has no third button); ticking it returns 3 with the other ticks
# in CHECKED, so the caller can show something and come back.
# With UI_DETAILS (one text per row) set, the list is drawn by lib/ui-tree.sh
# instead: rows expand with Right / collapse with Left to show their details.
ui_checklist() {
  local q=$1 i=1 out rows extra=${UI_EXTRA:-} hdr=${UI_LIST_HEADER:-}
  local -a items=() labs=()
  UI_EXTRA="" UI_LIST_HEADER=""
  if _tui_tree_ok; then
    TREE_DETAIL=(${UI_DETAILS[@]+"${UI_DETAILS[@]}"}); UI_DETAILS=()
    _tui_title "Choose"
    _tui_need
    _tui_take_notes "$q" $(($# / 2 + 12))
    _tui_help_text check
    _tree_list check "$TUI_TITLE" "$TUI_TEXT" "$hdr" "Ok Back" "${@:2}"
    return
  fi
  UI_DETAILS=() UI_HELP=""
  # Column titles: the last line of the text, indented like the rows' text
  # (box margin + "[ ] ").
  if [ -n "$hdr" ]; then
    if [ "$TUI_BIN" = dialog ]; then q="$q

       $hdr"; else q="$q

     $hdr"; fi
  fi
  shift
  [ -z "$extra" ] || { items+=(X "> $extra (tick, then OK)" off); labs+=("> $extra (tick, then OK)"); }
  while [ $# -ge 2 ]; do items+=("$i" "$1" "$2"); labs+=("$1"); i=$((i + 1)); shift 2; done
  _tui_title "Choose"
  _tui_need
  rows=${#labs[@]}; [ "$rows" -gt 15 ] && rows=15
  # If notes + question + list don't fit, the notes go to a scrollable box first.
  _tui_take_notes "$q" $(($(printf '%s\n' "$q" | wc -l) + rows + 2))
  _tui_list_size "$rows" "${labs[@]/#/     }"
  # A checklist needs two more rows than a menu for the same text.
  TUI_H=$((TUI_H + 2)); [ "$TUI_H" -gt $(($(tput lines 2>/dev/null || echo 24) - 2)) ] && TUI_H=$(($(tput lines 2>/dev/null || echo 24) - 2))
  _tui_screen_on
  _tui_back_opts
  # No tag column: rows are pre-aligned text, and numbers would only shift them.
  # (dialog also needs --no-collapse to keep the columns' spaces.)
  local -a notags=(--notags); [ "$TUI_BIN" = dialog ] && notags=(--no-tags --no-collapse)
  out=$(env ${TUI_ENV[@]+"${TUI_ENV[@]}"} "$TUI_BIN" --backtitle "$TUI_BACKTITLE" --title "$TUI_TITLE" --separate-output "${notags[@]}" "${TUI_BACK[@]}" \
        --checklist "$TUI_TEXT" "$TUI_H" "$TUI_W" "$rows" "${items[@]}" <"$TUI_TTY" 3>&1 1>&2 2>&3) || return 1
  CHECKED=$(printf '%s\n' "$out" | tr -d '"' | grep -v '^X$' | paste -sd' ' -)
  if printf '%s\n' "$out" | tr -d '"' | grep -qx X; then return 3; fi
  return 0
}

# Show what happened (results, "already in sync", cancelled) and wait for OK.
ui_report() {
  _tui_title "Result"
  NOTES=$(printf '%s\n' "$NOTES" | sed '/./,$!d')
  [ -n "$NOTES" ] || return 0
  if [ -n "$TUI_BIN" ] && [ -t 2 ]; then
    _tui_flush_long_notes
    if [ -n "$NOTES" ]; then _tui_box --msgbox "$NOTES" 0 >/dev/null || true; fi
  fi
  NOTES=""
}

ui_finish() {
  _tui_title "Done"
  [ -n "$NOTES" ] || return 0
  local t
  NOTES=$(printf '%s\n' "$NOTES" | sed '/./,$!d')
  t=$NOTES
  _tui_flush_long_notes
  if [ -n "$NOTES" ] && [ -n "$TUI_BIN" ] && [ -t 2 ]; then _tui_box --msgbox "$NOTES" 0 >/dev/null || true; fi
  NOTES=""
  _tui_screen_off
  printf '%s\n' "$t" >&2   # keep a copy in the terminal's scrollback
}
