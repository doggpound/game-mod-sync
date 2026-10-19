# shellcheck shell=bash
# Text UI: numbered menus and line prompts. Used by `game-mod-sync`.
#
# Every UI file provides the same functions; lib/core.sh only calls these:
#   info MSG                  transient progress ("Connecting ...")
#   say MSG                   a content line (preview, results)
#   ok MSG / warn MSG         success / warning line
#   die MSG                   show an error and exit 1
#   ui_ask PROMPT [DEFAULT]   free-text answer in ANSWER (TUI: 1 = Back)
#   ui_choose [--no-cancel] QUESTION LABEL...  pick one; sets CHOICE (1-based); 1 = back, q exits
#   ui_checklist Q LABEL on|off ...  tick boxes; sets CHECKED ("1 3"), 1 = back
#   ui_confirm QUESTION       yes/no, default no
#   ui_confirm_yes QUESTION   yes/no, default yes
#   ui_wait MSG CMD...        run CMD while showing MSG + activity; CMD's stdout
#                             -> WAIT_OUT, stderr -> WAIT_ERR, returns CMD's status
#   ui_gauge MSG CMD...       like ui_wait, but CMD prints percentages (0-100),
#                             one per line, shown as a progress bar
#   ui_suspend MSG / ui_resume  hand the terminal to a command that may prompt
#                             (e.g. ssh asking for a password), then take it back
#   ui_view TEXT              show a long text (TUI: scrollable box)
#   UI_EXTRA=LABEL            next ui_checklist offers one more action (returns 3)
#   UI_LIST_HEADER=TEXT       column titles shown above the next ui_checklist's rows
#   UI_DETAILS=(TEXT...)      per-row details for the next ui_checklist (TUI: rows
#                             expand with Right / collapse with Left; text: ignored)
#   ui_help TEXT              help for the next menu/checklist (TUI: H; text: h)
#   UI_BUTTONS="Edit Ok Exit" buttons of the next ui_choose (TUI: buttons; text: a
#                             letter each); one other than Ok/Back returns 4, UI_PRESSED
#   ui_title TITLE            title of the next question (TUI: box title; text: a heading
#                             above menus and checklists)
#   ui_start                  start of an interactive run
#   ui_report                 show what happened and wait for OK (TUI only)
#   ui_finish                 end of a run (flush anything still pending)

if [ -t 2 ]; then
  B=$'\033[1m' D=$'\033[2m' RED=$'\033[31m' GRN=$'\033[32m' YEL=$'\033[33m' CYN=$'\033[36m' N=$'\033[0m'
else
  B='' D='' RED='' GRN='' YEL='' CYN='' N=''
fi

info() { printf '%s==>%s %s\n' "$CYN" "$N" "$*" >&2; }
say()  { printf '%s\n' "$*" >&2; }
ok()   { printf '%s==>%s %s\n' "$GRN" "$N" "$*" >&2; }
warn() { printf '%swarning:%s %s\n' "$YEL" "$N" "$*" >&2; }
die()  { printf '%serror:%s %s\n' "$RED" "$N" "$*" >&2; exit 1; }

# Prompts read from the terminal on fd 3, so stdin/stdout stay free.
TTY_OPEN=""
tty_open() {
  [ -n "$TTY_OPEN" ] && return 0
  local t="${GMS_TTY:-/dev/tty}"
  if ! { exec 3<"$t"; } 2>/dev/null; then
    die "no terminal to ask questions on (pass -g GAME -p PROFILE -y to run unattended)"
  fi
  TTY_OPEN=1
}

ANSWER=""
ui_ask() {
  local ans
  UI_TITLE=""
  tty_open
  if [ -n "${2:-}" ]; then printf '%s [%s]: ' "$1" "$2" >&2; else printf '%s: ' "$1" >&2; fi
  IFS= read -r ans <&3 || die "input closed"
  ANSWER=${ans:-${2:-}}
}

ui_confirm() {
  local a
  UI_TITLE=""
  tty_open
  printf '%s [y/N]: ' "$1" >&2
  IFS= read -r a <&3 || return 1
  case $a in y|Y|yes|YES|Yes) return 0 ;; esac
  return 1
}

ui_confirm_yes() {
  local a
  UI_TITLE=""
  tty_open
  printf '%s [Y/n]: ' "$1" >&2
  IFS= read -r a <&3 || return 0
  case $a in n|N|no|NO|No) return 1 ;; esac
  return 0
}

CHOICE=0
ui_choose() { # [--no-cancel] QUESTION LABEL...  (returns 1 on b = back)
  local prompt n i=1 ans label back="b = back, " help=${UI_HELP:-} xb=${UI_BUTTONS:-} xk=${UI_KEYS:-} k keys="" bl
  local -A bkey=() rkey=()
  UI_HELP="" UI_BUTTONS="" UI_KEYS=""
  # Row keys (UI_KEYS "e:Edit"): typed after a number, e.g. "1e" edits row 1.
  for bl in $xk; do rkey[${bl%%:*}]=${bl#*:}; keys+="<number>${bl%%:*} = ${bl#*:}, "; done

  # Extra buttons (UI_BUTTONS, other than Ok / Back) get a letter each:
  # their first one not already taken (Edit = e, Exit = x).
  for bl in $xb; do
    case $bl in Ok|Back) continue ;; esac
    for ((i = 0; i < ${#bl}; i++)); do
      k=${bl:$i:1}; k=${k,}
      case "$k" in [bhqd]) continue ;; esac
      [ -z "${rkey[$k]:-}" ] || continue
      [ -z "${bkey[$k]:-}" ] && { bkey[$k]=$bl; keys+="$k = ${bl,}, "; break; }
    done
  done
  i=1
  if [ "$1" = --no-cancel ]; then back=""; shift; fi   # the main menu has its own Exit
  prompt=$1
  shift
  n=$#
  [ "$n" -gt 0 ] || die "nothing to choose from"
  tty_open
  _text_heading
  for label in "$@"; do
    printf '  %s%2d)%s %s\n' "$B" "$i" "$N" "$label" >&2
    i=$((i + 1))
  done
  while :; do
    printf '%s [1-%d, %s%s%sq to quit]: ' "$prompt" "$n" "$keys" "${help:+h = help, }" "$back" >&2
    IFS= read -r ans <&3 || die "input closed"
    if [ -n "$ans" ] && [ -n "${bkey[${ans,}]:-}" ]; then UI_PRESSED=${bkey[${ans,}]}; return 4; fi
    if [[ $ans =~ ^([0-9]+)([a-zA-Z])$ ]] && [ -n "${rkey[${BASH_REMATCH[2],}]:-}" ] \
       && [ "${BASH_REMATCH[1]}" -ge 1 ] && [ "${BASH_REMATCH[1]}" -le "$n" ]; then
      CHOICE=${BASH_REMATCH[1]}; UI_PRESSED=${rkey[${BASH_REMATCH[2],}]}; return 4
    fi
    case $ans in
      q|Q) exit 0 ;;
      h|H) if [ -n "$help" ]; then
             printf '\n%s\n\n' "$help" >&2
             i=1; for label in "$@"; do printf '  %s%2d)%s %s\n' "$B" "$i" "$N" "$label" >&2; i=$((i + 1)); done
             continue
           fi ;;
      b|B) [ -n "$back" ] && return 1 ;;
      ''|*[!0-9]*) ;;
      *) if [ "$ans" -ge 1 ] && [ "$ans" -le "$n" ]; then CHOICE=$ans; return 0; fi ;;
    esac
    warn "pick a number between 1 and $n"
  done
}

CHECKED=""
# With UI_EXTRA set (e.g. "Show the full differences"), "d" returns 3 with the
# current ticks in CHECKED, so the caller can show something and come back.
ui_checklist() { # QUESTION LABEL on|off ... -> CHECKED="1 3"; 1 = went back
  local q=$1 i ans x extra=${UI_EXTRA:-} hdr=${UI_LIST_HEADER:-} help=${UI_HELP:-}
  UI_EXTRA="" UI_LIST_HEADER="" UI_DETAILS=() UI_HELP=""
  local -a labs=() on=()
  shift
  while [ $# -ge 2 ]; do
    labs+=("$1"); if [ "$2" = on ]; then on+=(1); else on+=(0); fi; shift 2
  done
  tty_open
  _text_heading
  while :; do
    printf '%s\n' "$q" >&2
    [ -z "$hdr" ] || printf '          %s%s%s\n' "$D" "$hdr" "$N" >&2
    for i in "${!labs[@]}"; do
      x=' '; [ "${on[$i]}" = 1 ] && x=x
      printf '  %s%2d)%s [%s] %s\n' "$B" $((i + 1)) "$N" "$x" "${labs[$i]}" >&2
    done
    printf 'Numbers to tick/untick, Enter to save, %s%sb to go back: ' "${extra:+d = ${extra,}, }" "${help:+h = help, }" >&2
    IFS= read -r ans <&3 || return 1
    case $ans in
      h|H) if [ -n "$help" ]; then printf '\n%s\n\n' "$help" >&2; continue; fi ;;
      b|B) return 1 ;;
      '') break ;;
      d|D) if [ -n "$extra" ]; then
             CHECKED=""
             for i in "${!labs[@]}"; do [ "${on[$i]}" = 1 ] && CHECKED="$CHECKED $((i + 1))"; done
             CHECKED=${CHECKED# }
             return 3
           fi ;;
    esac
    for x in $ans; do
      case $x in *[!0-9]*) continue ;; esac
      if [ "$x" -ge 1 ] && [ "$x" -le ${#labs[@]} ]; then on[x - 1]=$((1 - on[x - 1])); fi
    done
  done
  CHECKED=""
  for i in "${!labs[@]}"; do [ "${on[$i]}" = 1 ] && CHECKED="$CHECKED $((i + 1))"; done
  CHECKED=${CHECKED# }
  return 0
}

# Run "$@" in the background with output in temp files; the caller polls.
_bg_start() {
  _BG_OUT=$(mktemp "${TMPDIR:-/tmp}/gms.XXXXXX"); _BG_ERR=$(mktemp "${TMPDIR:-/tmp}/gms.XXXXXX")
  "$@" </dev/null >"$_BG_OUT" 2>"$_BG_ERR" &
  _BG_PID=$!
}
_bg_finish() {
  local rc=0
  wait "$_BG_PID" || rc=$?
  WAIT_OUT=$(cat "$_BG_OUT"); WAIT_ERR=$(cat "$_BG_ERR")
  rm -f -- "$_BG_OUT" "$_BG_ERR"
  return "$rc"
}

WAIT_OUT="" WAIT_ERR=""
ui_wait() {
  local msg=$1 i=0 start=$SECONDS spin='\|/-'
  shift
  info "$msg"
  _bg_start "$@"
  if [ -t 2 ]; then
    while kill -0 "$_BG_PID" 2>/dev/null; do
      printf '\r   %s %ds ' "${spin:i++%4:1}" $((SECONDS - start)) >&2
      sleep 0.2
    done
    printf '\r\033[K' >&2
  fi
  _bg_finish
}

ui_gauge() {
  local msg=$1 p rc=0 errf
  shift
  info "$msg"
  [ -t 2 ] && printf '   checking files...' >&2
  errf=$(mktemp "${TMPDIR:-/tmp}/gms.XXXXXX")
  if "$@" </dev/null 2>"$errf" | while IFS= read -r p; do
       [ -t 2 ] && [ "$p" -gt 0 ] && printf '\r   [%-30s] %3d%%' "$(printf '%*s' $((p * 30 / 100)) '' | tr ' ' '#')" "$p" >&2
     done; then rc=0; else rc=${PIPESTATUS[0]}; fi
  [ -t 2 ] && printf '\r\033[K' >&2
  WAIT_OUT=""; WAIT_ERR=$(cat "$errf"); rm -f -- "$errf"
  return "$rc"
}

ui_suspend() { say "$1"; }
ui_resume() { :; }

ui_report() { say ""; }   # output is already on screen; just space it out
UI_TITLE="" UI_EXTRA="" UI_LIST_HEADER="" UI_DETAILS=() UI_HELP="" UI_BUTTONS="" UI_PRESSED="" UI_KEYS=""
ui_help() { UI_HELP=$1; }
ui_title() { UI_TITLE=$1; }
# ui_view TEXT: show a long text (TUI: scrollable box) and wait.
ui_view() { _text_heading; printf '%s\n' "$1" >&2; }
# Print the pending title as a heading (menus and checklists), then forget it.
_text_heading() { [ -z "$UI_TITLE" ] || printf '\n%s-- %s --%s\n' "$B" "$UI_TITLE" "$N" >&2; UI_TITLE=""; }
ui_start() { :; }
ui_finish() { :; }
