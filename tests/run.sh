#!/usr/bin/env bash
# Runs the tests: one file per plugin under tests/plugins/<kind>/<name>.sh,
# plus tests/core/*.sh. Every function named t_* in a file is a test; each runs
# in its own subshell and sandbox (tests/lib.sh). A "# badge: LABEL" line in a
# file names its badge.
#
#   tests/run.sh                 run everything
#   tests/run.sh gale amp        only files with these names
#   tests/run.sh --badges DIR    also write the README's status badges (SVG) to
#                                DIR: panel.svg (what the README shows: all of
#                                them in one box) and the single badges it's
#                                made of: tests, version, core-<name>, group-<kind>
#
# The README's badges are files in docs/badges/, so they show on Forgejo, on
# GitHub (mirror) and in any clone. After changing tests, plugins or VERSION:
#   tests/run.sh --badges docs/badges
# CI checks that they're up to date.
#
# Needs bash, rsync and perl (the BepInEx and Gale readers use perl).
set -uo pipefail
# Same output everywhere (badge widths count characters, sorting): UTF-8 C locale.
if locale -a 2>/dev/null | grep -qix 'c.utf-\?8'; then export LC_ALL=C.UTF-8; fi

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
badges="" only=()
while [ $# -gt 0 ]; do
  case $1 in
    --badges) badges=$2; shift 2 ;;
    -h|--help) sed -n '2,12p' "$0"; exit 0 ;;
    *) only+=("$1"); shift ;;
  esac
done

if [ -t 1 ]; then G=$'\e[32m' R=$'\e[31m' Y=$'\e[33m' N=$'\e[0m'; else G="" R="" Y="" N=""; fi
total=0 failed=0
declare -A f_pass=() f_fail=() label=()
files=()
for f in "$HERE"/plugins/*/*.sh "$HERE"/core/*.sh; do
  [ -f "$f" ] || continue
  n=${f##*/}; n=${n%.sh}
  if [ ${#only[@]} -gt 0 ] && [[ " ${only[*]} " != *" $n "* ]]; then continue; fi
  files+=("$f")
done

for _tf in "${files[@]}"; do
  rel=${_tf#"$HERE"/}; kind=${rel%/*}; kind=${kind#plugins/}; name=${rel##*/}; name=${name%.sh}
  key="$kind/$name"
  label[$key]=$(sed -n 's/^# badge: //p' "$_tf" | head -n 1); [ -n "${label[$key]}" ] || label[$key]=$name
  f_pass[$key]=0 f_fail[$key]=0
  printf '%s\n' "$key"
  mapfile -t tests < <(bash -c ". '$HERE/lib.sh';  . '$_tf'; declare -F" | awk '$3 ~ /^t_/ { print $3 }')
  for _tt in "${tests[@]}"; do
    total=$((total + 1))
    out=$( (
      # shellcheck source=/dev/null
      . "$HERE/lib.sh"
      # shellcheck source=/dev/null
      . "$_tf"
      sandbox_init
      trap sandbox_done EXIT
      set -e
      "$_tt"
    ) 2>&1 )
    rc=$?
    desc=${_tt#t_}; desc=${desc//_/ }
    if [ "$rc" = 0 ]; then
      f_pass[$key]=$((f_pass[$key] + 1))
      printf '  %sok%s    %s\n' "$G" "$N" "$desc"
    elif [ "$rc" = 77 ]; then
      printf '  %sskip%s  %s (%s)\n' "$Y" "$N" "$desc" "$(printf '%s' "$out" | tail -n 1)"
    else
      f_fail[$key]=$((f_fail[$key] + 1)); failed=$((failed + 1))
      printf '  %sFAIL%s  %s\n' "$R" "$N" "$desc"
      printf '%s\n' "$out" | tail -n 20 | sed 's/^/        /'
    fi
  done
done

printf '\n%d tests, %d failed\n' "$total" "$failed"

# ---- badges ---------------------------------------------------------------------
# A flat badge: grey label, coloured value. Text widths are estimated (~6.5px
# per character at 11px Verdana), which is close enough for short labels.
badge() { # FILE LABEL VALUE COLOUR
  local lw vw w
  lw=$(( ${#2} * 65 / 10 + 12 )); vw=$(( ${#3} * 65 / 10 + 12 )); w=$((lw + vw))
  cat >"$1" <<SVG
<svg xmlns="http://www.w3.org/2000/svg" width="$w" height="20" role="img" aria-label="$2: $3">
  <title>$2: $3</title>
  <linearGradient id="s" x2="0" y2="100%"><stop offset="0" stop-color="#bbb" stop-opacity=".1"/><stop offset="1" stop-opacity=".1"/></linearGradient>
  <clipPath id="r"><rect width="$w" height="20" rx="3" fill="#fff"/></clipPath>
  <g clip-path="url(#r)"><rect width="$lw" height="20" fill="#555"/><rect x="$lw" width="$vw" height="20" fill="$4"/><rect width="$w" height="20" fill="url(#s)"/></g>
  <g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" font-size="11">
    <text x="$((lw / 2))" y="15" fill="#010101" fill-opacity=".3">$2</text><text x="$((lw / 2))" y="14">$2</text>
    <text x="$((lw + vw / 2))" y="15" fill="#010101" fill-opacity=".3">$3</text><text x="$((lw + vw / 2))" y="14">$3</text>
  </g>
</svg>
SVG
}
# A grouped badge: grey label, then one coloured segment per item.
group_badge() { # FILE LABEL "TEXT|COLOUR"...
  local f=$1 l=$2 lw x seg t c w rects="" texts="" title
  shift 2
  lw=$(( ${#l} * 65 / 10 + 12 )); x=$lw; title=$l
  texts="<text x=\"$((lw / 2))\" y=\"15\" fill=\"#010101\" fill-opacity=\".3\">$l</text><text x=\"$((lw / 2))\" y=\"14\">$l</text>"
  for seg in "$@"; do
    t=${seg%|*} c=${seg##*|}
    w=$(( ${#t} * 65 / 10 + 12 ))
    rects+="<rect x=\"$x\" width=\"$w\" height=\"20\" fill=\"$c\"/>"
    [ "$x" = "$lw" ] || rects+="<rect x=\"$x\" width=\"1\" height=\"20\" fill=\"#fff\" fill-opacity=\".35\"/>"
    texts+="<text x=\"$((x + w / 2))\" y=\"15\" fill=\"#010101\" fill-opacity=\".3\">$t</text><text x=\"$((x + w / 2))\" y=\"14\">$t</text>"
    title+=", $t"
    x=$((x + w))
  done
  cat >"$f" <<SVG
<svg xmlns="http://www.w3.org/2000/svg" width="$x" height="20" role="img" aria-label="$title">
  <title>$title</title>
  <linearGradient id="s" x2="0" y2="100%"><stop offset="0" stop-color="#bbb" stop-opacity=".1"/><stop offset="1" stop-opacity=".1"/></linearGradient>
  <clipPath id="r"><rect width="$x" height="20" rx="3" fill="#fff"/></clipPath>
  <g clip-path="url(#r)"><rect width="$lw" height="20" fill="#555"/>$rects<rect width="$x" height="20" fill="url(#s)"/></g>
  <g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" font-size="11">$texts</g>
</svg>
SVG
}
# The README's badge panel: one image, a rounded light-grey box with a label in
# front of each row ("Status", "Plugins") and the badges (the files above,
# nested as <svg> elements, their ids made unique) drawn in it. A README can't
# style HTML (no CSS on GitHub or Forgejo), hence an image.
panel() { # FILE "LABEL|badge.svg badge.svg ..."...
  local f=$1 dir row label b i=0 y w rw maxw=0 bw body="" labels="" pad=14 lw=74 gap=6 rh=20 rg=10 n=0
  dir=$(dirname "$f")
  shift
  y=$pad
  for row in "$@"; do
    label=${row%%|*} rw=0
    labels+="<text x=\"$((pad + lw - 14))\" y=\"$((y + 14))\" text-anchor=\"end\">$label</text>"
    for b in ${row#*|}; do
      i=$((i + 1))
      w=$(sed -n 's/^<svg [^>]* width="\([0-9]*\)".*/\1/p' "$dir/$b" | head -n 1)
      bw=$((pad + lw + rw))
      body+="<svg x=\"$bw\" y=\"$y\" width=\"$w\" height=\"20\">$(sed -e '1d' -e '$d' -e "s/id=\"s\"/id=\"s$i\"/; s/url(#s)/url(#s$i)/; s/id=\"r\"/id=\"r$i\"/; s/url(#r)/url(#r$i)/; /<title>/d" "$dir/$b")</svg>"
      rw=$((rw + w + gap))
    done
    rw=$((rw - gap)); [ "$rw" -gt "$maxw" ] && maxw=$rw
    y=$((y + rh + rg)); n=$((n + 1))
  done
  local W=$((pad + lw + maxw + pad)) H=$((y - rg + pad))
  cat >"$f" <<SVG
<svg xmlns="http://www.w3.org/2000/svg" width="$W" height="$H" role="img" aria-label="Status and plugin test badges">
  <title>game-mod-sync: status and plugin tests</title>
  <rect x="0.5" y="0.5" width="$((W - 1))" height="$((H - 1))" rx="12" fill="#f6f8fa" stroke="#d0d7de"/>
  <line x1="$((pad + lw - 6))" y1="$((pad - 4))" x2="$((pad + lw - 6))" y2="$((H - pad + 4))" stroke="#d0d7de"/>
  <g fill="#57606a" font-family="-apple-system,Segoe UI,Helvetica,Arial,DejaVu Sans,sans-serif" font-size="12" font-weight="600">$labels</g>
  $body
</svg>
SVG
}
if [ -n "$badges" ]; then
  mkdir -p "$badges"
  # one badge per kind of plugin: "server | AMP 8 | SSH 5" (catch-alls like
  # games/_default last); the README shows these, so new plugins appear by themselves
  for kind in sources loaders backends games; do
    segs=()
    while read -r key; do
      [ -n "$key" ] || continue
      p=${f_pass[$key]} x=${f_fail[$key]} name=${label[$key]#*: }
      if [ "$x" = 0 ]; then segs+=("$name $p|#4c1"); else segs+=("$name $x failed|#e05d44"); fi
    done < <(printf '%s\n' "${!f_pass[@]}" | grep "^$kind/" | awk -F/ '{ print ($2 ~ /^_/ ? 1 : 0) "\t" $0 }' | sort | cut -f2)
    [ ${#segs[@]} -gt 0 ] || continue
    case $kind in sources) g=source ;; loaders) g=loader ;; backends) g=server ;; games) g=game ;; esac
    group_badge "$badges/group-$kind.svg" "$g" "${segs[@]}"
  done
  for key in "${!f_pass[@]}"; do
    [ "${key%%/*}" = core ] || continue       # plugins are in the group badges
    p=${f_pass[$key]} x=${f_fail[$key]}
    if [ "$x" = 0 ]; then badge "$badges/${key//\//-}.svg" "${label[$key]}" "$p passed" "#4c1"
    else badge "$badges/${key//\//-}.svg" "${label[$key]}" "$x failed" "#e05d44"; fi
  done
  v=$(sed -n 's/^VERSION="\(.*\)"/\1/p' "$HERE/../lib/core.sh" | head -n 1)
  badge "$badges/version.svg" version "v$v" "#8e2de2"
  if [ "$failed" = 0 ]; then badge "$badges/tests.svg" tests "$total passed" "#4c1"
  else badge "$badges/tests.svg" tests "$failed of $total failed" "#e05d44"; fi
  core=""; for b in "$badges"/core-*.svg; do [ -f "$b" ] && core="${b##*/} $core"; done   # core-sync, then core-interface
  groups=""; for kind in sources loaders backends games; do [ -f "$badges/group-$kind.svg" ] && groups+="group-$kind.svg "; done
  panel "$badges/panel.svg" "Status|version.svg tests.svg $core" "Plugins|$groups"
  echo "badges written to $badges"
fi
[ "$failed" = 0 ]
