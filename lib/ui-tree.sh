# shellcheck shell=bash
# Menus and checklists drawn directly with ANSI escapes, used by lib/ui-tui.sh
# for ui_choose and ui_checklist. whiptail and dialog can't do what these
# need: rows that expand to show details, a header bar for columns, and a
# help overlay on a key.
#
#   _tree_list MODE TITLE TEXT HEADER BUTTONS LABEL [on|off] ...
#     MODE     menu (pick one row) or check (tick rows)
#     BUTTONS  the buttons left to right, e.g. "Ok Back" or "Edit Ok Exit";
#              Ok is focused first and Tab goes right from it (wrapping)
#     details per row in TREE_DETAIL[] (check mode), help text in TREE_HELP,
#     extra keys in TREE_KEYS ("e:Edit ..."): shown as "E - Edit" above the
#     help line; pressing one returns 4 with TREE_PRESSED=Edit and CHOICE set
#   -> menu: CHOICE (1-based); check: CHECKED="1 3".
#      Returns 0 (Ok), 1 (Back or Esc), 4 (another button: TREE_PRESSED=its name).
#
# Keys: Up/Down/PgUp/PgDn/Home/End move, 1-9 jump to a row, Tab / Shift+Tab
# move between the buttons (menus: also Left / Right), Enter presses the
# focused button, Esc = Back, H = help. Check mode: Space ticks, Right opens
# a row's details, Left closes them, + / - open / close all.

TREE_DETAIL=()
TREE_HELP=""
TREE_PRESSED="" TREE_KEYS=""

# Colours follow the dialog tool's look: newt's Ubuntu palette is magenta,
# plain newt and dialog are blue. The table's header bar is the table colour
# 20% darker: the terminal is asked for that colour's RGB (OSC 4); terminals
# that don't answer get a fixed dark shade from the 256-colour palette.
TREE_ACCENT="" TREE_HDR_BG=""
_tree_colors() {
  [ -z "$TREE_ACCENT" ] || return 0
  if [ "${TUI_BIN:-}" = whiptail ] && grep -qs '^root=,magenta' /etc/newt/palette; then TREE_ACCENT=5; else TREE_ACCENT=4; fi
  local reply="" r g b
  { exec 6<>"$TUI_TTY"; } 2>/dev/null || { _tree_colors_fallback; return 0; }
  printf '\e]4;%s;?\a' "$TREE_ACCENT" >&6
  IFS= read -rs -d $'\a' -t 0.3 -u 6 reply || true
  # drain the rest of an answer ended with ESC \ instead of BEL
  while IFS= read -rsn1 -t 0.02 -u 6 _; do :; done
  exec 6>&-
  if [[ $reply =~ rgb:([0-9a-fA-F]{2})[0-9a-fA-F]*/([0-9a-fA-F]{2})[0-9a-fA-F]*/([0-9a-fA-F]{2}) ]]; then
    r=$((16#${BASH_REMATCH[1]} * 8 / 10)); g=$((16#${BASH_REMATCH[2]} * 8 / 10)); b=$((16#${BASH_REMATCH[3]} * 8 / 10))
    TREE_HDR_BG="48;2;$r;$g;$b"
  else
    _tree_colors_fallback
  fi
}
_tree_colors_fallback() { if [ "$TREE_ACCENT" = 5 ]; then TREE_HDR_BG="48;5;54"; else TREE_HDR_BG="48;5;18"; fi; }

# Line-drawing characters: Unicode when the terminal speaks UTF-8, else ASCII.
_tree_glyphs() {
  case "${LC_ALL:-${LC_CTYPE:-${LANG:-}}}" in
    *[Uu][Tt][Ff]-8*|*[Uu][Tt][Ff]8*)
      TG_H='─' TG_V='│' TG_TL='┌' TG_TR='┐' TG_BL='└' TG_BR='┘' TG_LT='┤' TG_RT='├'
      TG_OPEN='▾' TG_SHUT='▸' TG_UP='▲' TG_DN='▼' ;;
    *)
      TG_H='-' TG_V='|' TG_TL='+' TG_TR='+' TG_BL='+' TG_BR='+' TG_LT='|' TG_RT='|'
      TG_OPEN='v' TG_SHUT='>' TG_UP='^' TG_DN='v' ;;
  esac
}

# _tree_rep CHAR N -> CHAR repeated N times
_tree_rep() { local r="" i; for ((i = 0; i < $2; i++)); do r+=$1; done; printf '%s' "$r"; }

# _tree_fit TEXT WIDTH -> TEXT cut or padded to exactly WIDTH characters
_tree_fit() {
  local t=$1 w=$2
  [ "$w" -gt 0 ] || return 0
  if [ ${#t} -gt "$w" ]; then printf '%s' "${t:0:$((w - 1))}~"; else printf '%-*s' "$w" "$t"; fi
}

# _tree_key -> KEY: one key press from fd 5, escape sequences named
# ESCA (up) ESCB (down) ESCC (right) ESCD (left) ESC5~ ESC6~ ESCH ESCF ..., ESC alone.
_tree_key() {
  local k k2="" k3="" k4="" e=$'\e'
  IFS= read -rsn1 -u 5 k || { KEY=ESC; return; }
  if [ "$k" = "$e" ]; then
    IFS= read -rsn1 -t 0.05 -u 5 k2 || true
    if [ "$k2" = "[" ] || [ "$k2" = "O" ]; then
      IFS= read -rsn1 -t 0.05 -u 5 k3 || true
      case $k3 in [0-9]) IFS= read -rsn1 -t 0.05 -u 5 k4 || true ;; esac
      KEY="ESC$k3$k4"
    elif [ -z "$k2" ]; then KEY=ESC
    else KEY="ESC?"
    fi
  else
    KEY=$k
  fi
}

# Box drawing into $out: _tree_frame X Y W H TITLE draws border, title and
# shadow; _tree_line X Y W TEXT writes one inside line (TEXT may hold colours).
_tree_frame() {
  local x=$1 y=$2 w=$3 h=$4 t=" $5 " pad i e=$'\e'
  local bg="${e}[0;30;47m"
  pad=$(((w - 2 - ${#t} - 2) / 2)); [ "$pad" -lt 0 ] && pad=0
  out+="${e}[${y};${x}H${bg}${TG_TL}$(_tree_rep "$TG_H" "$pad")${TG_LT}${e}[1;31;47m${t}${bg}${TG_RT}"
  out+="$(_tree_rep "$TG_H" $((w - 4 - pad - ${#t})))${TG_TR}"
  for ((i = 1; i < h - 1; i++)); do
    out+="${e}[$((y + i));${x}H${bg}${TG_V}${e}[$((y + i));$((x + w - 1))H${bg}${TG_V}${e}[0;30;40m "
  done
  out+="${e}[$((y + h - 1));${x}H${bg}${TG_BL}$(_tree_rep "$TG_H" $((w - 2)))${TG_BR}${e}[0;30;40m "
  out+="${e}[$((y + h));$((x + 1))H${e}[0;30;40m$(printf '%*s' "$w" '')"
}
_tree_line() { out+=$'\e'"[$2;$(($1 + 1))H"$'\e'"[0;30;47m $4"$'\e'"[0;30;47m "; }

# Help overlay: scrollable text, one button (Close).
_tree_help() {
  local text=$1 rows cols W H X Y top=0 n i lines=() e=$'\e' out
  while :; do
    rows=$(tput lines 2>/dev/null || echo 24); cols=$(tput cols 2>/dev/null || echo 80)
    W=$((cols - 16)); [ "$W" -gt 100 ] && W=100; [ "$W" -lt 40 ] && W=$((cols - 2))
    mapfile -t lines < <(printf '%s\n' "$text" | fold -s -w $((W - 4)))
    n=${#lines[@]}
    H=$((n + 6)); [ "$H" -gt $((rows - 4)) ] && H=$((rows - 4))
    X=$(((cols - W) / 2 + 1)); Y=$(((rows - H) / 2 + 1))
    local LH=$((H - 5))
    [ "$top" -gt $((n - LH)) ] && top=$((n - LH)); [ "$top" -lt 0 ] && top=0
    out=""
    _tree_frame "$X" "$Y" "$W" "$H" "Help"
    for ((i = 0; i < LH; i++)); do
      _tree_line "$X" $((Y + 1 + i)) "$W" "$(_tree_fit "${lines[$((top + i))]:-}" $((W - 4)))"
    done
    local more=""
    [ "$top" -gt 0 ] && more+="$TG_UP"
    [ $((top + LH)) -lt "$n" ] && more+="$TG_DN"
    [ -z "$more" ] || more="$more scroll"
    _tree_line "$X" $((Y + H - 3)) "$W" "$(_tree_fit "$more" $((W - 4)))"
    local g=$(((W - 4 - 7) / 2))
    _tree_line "$X" $((Y + H - 2)) "$W" "$(printf '%*s' "$g" '')${e}[1;37;41m<Close>${e}[0;30;47m$(printf '%*s' $((W - 4 - g - 7)) '')"
    printf '%s' "$out" >&2
    _tree_key
    case $KEY in
      ESCA|k) top=$((top - 1)) ;;
      ESCB|j) top=$((top + 1)) ;;
      ESC5~) top=$((top - LH)) ;;
      ESC6~|' ') top=$((top + LH)) ;;
      ESCH|ESC1~) top=0 ;;
      ESCF|ESC4~) top=$n ;;
      ''|ESC|h|H|q|c|C) return 0 ;;
    esac
  done
}

_tree_list() {
  local mode=$1 title=$2 text=$3 header=$4 buttons=$5
  shift 5
  local -a lab=() on=() open=() vis_row=() vis_det=() textl=() det=()
  local n=0 i j cur=0 top=0 rc=0 accent e=$'\e' rows cols geom="" fb=0 nb
  local W H X Y LH LW tl vis out full=1 more y s
  local -a btn=() hk=() hint=()
  read -r -a btn <<<"$buttons"
  read -r -a hk <<<"$TREE_KEYS"
  for i in "${hk[@]}"; do hint+=("${i%%:*}"); done
  nb=${#btn[@]}
  for i in "${!btn[@]}"; do [ "${btn[$i],,}" = ok ] && fb=$i; done
  while [ $# -gt 0 ]; do
    lab+=("$1"); open+=(0); n=$((n + 1))
    if [ "$mode" = check ]; then on+=("${2:-off}"); shift 2; else on+=(off); shift; fi
  done
  _tree_colors
  accent=$TREE_ACCENT
  _tree_glyphs
  exec 5<"$TUI_TTY"
  printf '%s' "${e}[?25l" >&2                         # hide the cursor

  while :; do
    rows=$(tput lines 2>/dev/null || echo 24); cols=$(tput cols 2>/dev/null || echo 80)
    # The box fits its content (text, header, rows, open details), up to the screen.
    local wmax=$((${#title} + 10)) tline
    for ((i = 0; i < n; i++)); do [ $((${#lab[$i]} + 10)) -gt "$wmax" ] && wmax=$((${#lab[$i]} + 10)); done
    [ $((${#header} + 11)) -gt "$wmax" ] && wmax=$((${#header} + 11))
    while IFS= read -r tline; do [ $((${#tline} + 4)) -gt "$wmax" ] && wmax=$((${#tline} + 4)); done <<<"$text"
    for ((i = 0; i < n; i++)); do
      [ "${open[$i]}" = 1 ] || continue
      while IFS= read -r tline; do [ $((${#tline} + 14)) -gt "$wmax" ] && wmax=$((${#tline} + 14)); done <<<"${TREE_DETAIL[$i]:-}"
    done
    W=$((wmax + 2)); [ "$W" -lt 60 ] && W=60; [ "$W" -gt $((cols - 4)) ] && W=$((cols - 4))
    LW=$((W - 4))
    textl=()
    [ -z "$text" ] || mapfile -t textl < <(printf '%s\n' "$text" | fold -s -w "$LW")
    tl=${#textl[@]}
    # rows: border, text, blank, [header], list, blank, key hints, help hint, buttons, border
    local fixed=$((tl + 5 + ${#hk[@]} + (tl > 0 ? 1 : 0) + (${#header} > 0 ? 1 : 0)))
    # visible lines: each row, then its details when open
    vis_row=() vis_det=()
    for ((i = 0; i < n; i++)); do
      vis_row+=("$i"); vis_det+=(-1)
      if [ "${open[$i]}" = 1 ] && [ -n "${TREE_DETAIL[$i]:-}" ]; then
        mapfile -t det <<<"${TREE_DETAIL[$i]}"
        for ((j = 0; j < ${#det[@]}; j++)); do vis_row+=("$i"); vis_det+=("$j"); done
      fi
    done
    vis=${#vis_row[@]}
    LH=$vis
    [ "$LH" -gt $((rows - 3 - fixed)) ] && LH=$((rows - 3 - fixed))
    if [ "$mode" = menu ]; then [ "$LH" -lt 1 ] && LH=1; else [ "$LH" -lt 3 ] && LH=3; fi
    H=$((fixed + LH))
    X=$(((cols - W) / 2 + 1)); Y=$(((rows - H) / 2 + 1)); [ "$Y" -lt 2 ] && Y=2
    # a box that moved or changed size leaves pieces behind: redraw the background
    [ "$geom" = "$X $Y $W $H $rows $cols" ] || full=1
    geom="$X $Y $W $H $rows $cols"
    # keep the cursor row, and as much of its open details as fit, in view
    local cl=0 last
    for ((i = 0; i < vis; i++)); do
      if [ "${vis_row[$i]}" = "$cur" ] && [ "${vis_det[$i]}" = -1 ]; then cl=$i; break; fi
    done
    [ "$cl" -lt "$top" ] && top=$cl
    [ "$cl" -ge $((top + LH)) ] && top=$((cl - LH + 1))
    last=$cl
    while [ $((last + 1)) -lt "$vis" ] && [ "${vis_row[$((last + 1))]}" = "$cur" ]; do last=$((last + 1)); done
    while [ "$last" -ge $((top + LH)) ] && [ "$top" -lt "$cl" ]; do top=$((top + 1)); done
    [ "$top" -gt $((vis - LH)) ] && top=$((vis - LH)); [ "$top" -lt 0 ] && top=0

    out=""
    if [ "$full" = 1 ]; then          # background + backtitle when (re)drawing everything
      out+="${e}[0;37;4${accent}m${e}[2J${e}[1;1H ${TUI_BACKTITLE}"
      full=0
    fi
    _tree_frame "$X" "$Y" "$W" "$H" "$title"
    y=$((Y + 1))
    for ((i = 0; i < tl; i++)); do _tree_line "$X" "$y" "$W" "$(_tree_fit "${textl[$i]}" "$LW")"; y=$((y + 1)); done
    if [ "$tl" -gt 0 ]; then _tree_line "$X" "$y" "$W" "$(_tree_fit "" "$LW")"; y=$((y + 1)); fi
    if [ -n "$header" ]; then
      _tree_line "$X" "$y" "$W" "${e}[1;97;${TREE_HDR_BG}m$(_tree_fit "       $header" "$LW")"; y=$((y + 1))
    fi
    for ((i = 0; i < LH; i++)); do
      local v=$((top + i))
      if [ "$v" -ge "$vis" ]; then
        _tree_line "$X" "$y" "$W" "${e}[0;37;4${accent}m$(_tree_fit "" "$LW")"; y=$((y + 1)); continue
      fi
      local r=${vis_row[$v]} d=${vis_det[$v]}
      if [ "$d" = -1 ]; then
        if [ "$mode" = check ]; then
          local arrow=' ' box='[ ]'
          if [ -n "${TREE_DETAIL[$r]:-}" ]; then
            if [ "${open[$r]}" = 1 ]; then arrow=$TG_OPEN; else arrow=$TG_SHUT; fi
          fi
          [ "${on[$r]}" = on ] && box='[*]'
          s=$(_tree_fit " $arrow $box ${lab[$r]}" "$LW")
        else
          s=$(_tree_fit "  $(printf '%2d' $((r + 1)))  ${lab[$r]}" "$LW")
        fi
        if [ "$r" = "$cur" ]; then s="${e}[1;97;41m$s"; else s="${e}[0;97;4${accent}m$s"; fi
      else
        # details: a soft grey, so the rows themselves stand out
        mapfile -t det <<<"${TREE_DETAIL[$r]}"
        s="${e}[0;38;5;250;4${accent}m$(_tree_fit "         ${det[$d]}" "$LW")"
      fi
      _tree_line "$X" "$y" "$W" "$s"; y=$((y + 1))
    done
    more=""
    [ "$top" -gt 0 ] && more+="$TG_UP"
    [ $((top + LH)) -lt "$vis" ] && more+="$TG_DN"
    [ -z "$more" ] || more="$more more "
    _tree_line "$X" "$y" "$W" "$(_tree_fit "" "$LW")"; y=$((y + 1))
    for i in "${hk[@]}"; do
      local kk=${i%%:*}
      _tree_line "$X" "$y" "$W" "${e}[0;30;47m$(_tree_fit "${kk^} - ${i#*:}" "$LW")"; y=$((y + 1))
    done
    _tree_line "$X" "$y" "$W" "${e}[0;30;47m$(_tree_fit "H - Help Menu" "$LW")"
    [ -z "$more" ] || out+="${e}[${y};$((X + W - 3 - ${#more}))H${e}[0;30;47m${more% }"
    y=$((y + 1))
    # buttons, spread evenly; the focused one in red
    local blen=0 gap bl=""
    for i in "${!btn[@]}"; do blen=$((blen + ${#btn[$i]} + 2)); done
    gap=$(((LW - blen) / (nb + 1)))
    for i in "${!btn[@]}"; do
      bl+="$(printf '%*s' "$gap" '')"
      if [ "$i" = "$fb" ]; then bl+="${e}[1;97;41m<${btn[$i]}>${e}[0;30;47m"; else bl+="<${btn[$i]}>"; fi
    done
    _tree_line "$X" "$y" "$W" "$bl$(printf '%*s' $((LW - blen - gap * nb)) '')"
    out+="${e}[0m"
    printf '%s' "$out" >&2

    _tree_key
    case $KEY in
      ESCA|k) [ "$cur" -gt 0 ] && cur=$((cur - 1)) ;;
      ESCB|j) [ "$cur" -lt $((n - 1)) ] && cur=$((cur + 1)) ;;
      ESC5~) cur=$((cur - LH)); [ "$cur" -lt 0 ] && cur=0 ;;
      ESC6~) cur=$((cur + LH)); [ "$cur" -gt $((n - 1)) ] && cur=$((n - 1)) ;;
      ESCH|ESC1~) cur=0 ;;
      ESCF|ESC4~) cur=$((n - 1)) ;;
      [1-9]) [ "$KEY" -le "$n" ] && cur=$((KEY - 1)) ;;
      h|H|'?') _tree_help "${TREE_HELP:-No help for this screen.}"; full=1 ;;
      $'\t') fb=$(((fb + 1) % nb)) ;;
      ESCZ) fb=$(((fb + nb - 1) % nb)) ;;                 # Shift+Tab
      '') case ${btn[$fb],,} in
            ok) rc=0 ;;
            back) rc=1 ;;
            *) rc=4; TREE_PRESSED=${btn[$fb]} ;;
          esac
          break ;;
      ESC) case " ${buttons,,} " in *" back "*) rc=1; break ;; esac ;;
    esac
    # extra keys (e.g. E = Edit the highlighted row)
    for i in "${hk[@]}"; do
      if [ "${KEY,}" = "${i%%:*}" ]; then rc=4; TREE_PRESSED=${i#*:}; break 2; fi
    done
    # menus: Left / Right also move between the buttons
    if [ "$mode" = menu ]; then
      case $KEY in
        ESCC) [ "$fb" -lt $((nb - 1)) ] && fb=$((fb + 1)) ;;
        ESCD) [ "$fb" -gt 0 ] && fb=$((fb - 1)) ;;
      esac
    fi
    if [ "$mode" = check ]; then
      case $KEY in
        ESCC|l) [ -n "${TREE_DETAIL[$cur]:-}" ] && open[cur]=1 ;;
        ESCD) open[cur]=0 ;;
        +|=) for ((i = 0; i < n; i++)); do open[i]=1; done ;;
        -|_) for ((i = 0; i < n; i++)); do open[i]=0; done ;;
        ' ') if [ "${on[$cur]}" = on ]; then on[cur]=off; else on[cur]=on; fi ;;
      esac
    fi
    # a resize redraws everything
    if [ "$rows" != "$(tput lines 2>/dev/null || echo 24)" ] || [ "$cols" != "$(tput cols 2>/dev/null || echo 80)" ]; then full=1; fi
  done
  exec 5<&-
  printf '%s' "${e}[?25h${e}[0m" >&2                 # cursor back
  TREE_DETAIL=() TREE_HELP="" TREE_KEYS=""
  CHOICE=$((cur + 1))
  CHECKED=""
  for ((i = 0; i < n; i++)); do [ "${on[$i]}" = on ] && CHECKED="$CHECKED $((i + 1))"; done
  CHECKED=${CHECKED# }
  return "$rc"
}
