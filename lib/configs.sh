# shellcheck shell=bash
# Config files: compare the profile's CONFIG_DIRS with the server's, work out
# which mod each file belongs to and whether that mod is client-only, and copy
# the files you pick. UI-independent; sourced by lib/core.sh.
#
# Rules: configs are offered per file and never deleted on the server. A file
# the server doesn't have yet is ticked (unless its mod is client-only); a file
# that differs is shown with the settings that differ, unticked.

# CFG_ITEMS: "dir path state mod side detail", separated by $US (unit
# separator, not a tab: `read` would merge the tabs around an empty field)
#   state  new | differs
#   mod    mod folder in the profile (e.g. Azumatt-FastLink), empty if unknown
#   side   client | server | both | (empty = unknown)
#   detail for "differs": which settings differ
US=$'\x1f'
CFG_ITEMS=()
CFG_HIDDEN=0   # files whose mod isn't in the profile (left over from removed mods)

# ------------------------------------------------------------ which mod --
# cfg_owners CONFIG_FILES MODS OWNERS -> "path<TAB>mod folder" (empty if unknown)
#   CONFIG_FILES  file with config paths relative to the config folder
#   MODS          file with the mod folder names
#   OWNERS        file with "plugin id<TAB>mod folder" (may be empty)
# Order: the plugin id the file is named after (longest wins), then a mod
# folder with the same name (ignoring case and punctuation), then a mod whose
# name matches the last part of the file name.
cfg_owners() {
  awk -F'\t' -v OFS='\t' '
    function norm(s) { s = tolower(s); gsub(/[^a-z0-9]/, "", s); return s }
    FILENAME == ARGV[1] { owner[tolower($1)] = $2; next }
    FILENAME == ARGV[2] {
      byname[norm($1)] = $1
      n = $1; sub(/^[^-]*-/, "", n); n = norm(n)
      short[n] = (n in short) ? "" : $1          # ambiguous short names are dropped
      next
    }
    {
      f = $0; k = f
      if (index(k, "/")) k = substr(k, 1, index(k, "/") - 1)
      else sub(/\.[^.]*$/, "", k)
      lk = tolower(k); best = ""; bl = 0
      for (id in owner)
        if ((lk == id || (index(lk, id) == 1 && substr(lk, length(id) + 1, 1) ~ /[._ -]/)) && length(id) > bl) {
          best = owner[id]; bl = length(id)
        }
      if (best == "") {
        p = k
        while (best == "") {
          if (norm(p) in byname) { best = byname[norm(p)]; break }
          if ((norm(p) in short) && short[norm(p)] != "") { best = short[norm(p)]; break }
          q = p; sub(/[._-][^._-]*$/, "", q)
          if (q == p || length(q) < 4) break
          p = q
        }
      }
      if (best == "") {
        t = k; sub(/^.*[.]/, "", t)
        if ((norm(t) in short) && short[norm(t)] != "") best = short[norm(t)]
      }
      print f, best
    }' "$3" "$2" "$1"
}

# ------------------------------------------------- client / server tags --
# Thunderstore categories say whether a mod runs on clients, servers or both
# (Valheim: "Client-side" / "Server-side"). Looked up per Author-Name mod
# folder and cached for good in TAGS_CACHE ("mod<TAB>time<TAB>cat|cat"; a mod
# Thunderstore doesn't know is cached with no categories). Only mods not in
# the cache are looked up; Edit -> "Refresh mod tags" (or --refresh-tags)
# looks them all up again. Before going online, the source's own cache is
# used (Gale keeps one).
TAGS_CACHE="${XDG_CACHE_HOME:-$HOME/.cache}/game-mod-sync/thunderstore-tags.tsv"

# tags_known SOURCE GAME -> "mod<TAB>cat|cat" from our cache, then the source's
tags_known() {
  [ ! -s "$TAGS_CACHE" ] || awk -F'\t' -v OFS='\t' '{ print $1, $3 }' "$TAGS_CACHE"
  if declare -F "${1}_mod_tags" >/dev/null; then "${1}_mod_tags" "$2" 2>/dev/null || true; fi
}

# tags_fetch GAME MOD... -> looks the mods up online and stores them in the
# cache (replacing older entries). Returns 1 if nothing could be looked up.
tags_fetch() {
  local game=$1 now tmp
  shift
  [ $# -gt 0 ] || return 0
  command -v curl >/dev/null 2>&1 && command -v perl >/dev/null 2>&1 || return 1
  ui_wait "Looking up $# mod(s) on Thunderstore (client / server) ..." \
    ts_fetch_tags "${THUNDERSTORE_COMMUNITY:-$game}" "$@" || true
  [ -n "$WAIT_OUT" ] || return 1
  now=$(date +%s)
  mkdir -p "${TAGS_CACHE%/*}"
  tmp="$TAGS_CACHE.tmp.$$"
  { printf '%s\n' "$WAIT_OUT" | awk -F'\t' -v OFS='\t' -v now="$now" '$1 != "" { print $1, now, $2 }'
    [ ! -f "$TAGS_CACHE" ] || awk -F'\t' 'NR == FNR { new[$1] = 1; next } !($1 in new)' \
      <(printf '%s\n' "$WAIT_OUT") "$TAGS_CACHE"
  } | sort >"$tmp" && mv "$tmp" "$TAGS_CACHE"
}

# mod_sides SOURCE GAME MOD... -> "mod<TAB>client|server|both|" per mod
mod_sides() {
  local src=$1 game=$2 m tmp
  local -a need=()
  shift 2
  tmp=$(mktemp "${TMPDIR:-/tmp}/gms.XXXXXX")
  tags_known "$src" "$game" >"$tmp"
  for m in "$@"; do
    case $m in *-*) ;; *) continue ;; esac            # not a Thunderstore Author-Name folder
    awk -F'\t' -v m="$m" '$1 == m { f = 1; exit } END { exit !f }' "$tmp" || need+=("$m")
  done
  if [ ${#need[@]} -gt 0 ] && tags_fetch "$game" "${need[@]}"; then tags_known "$src" "$game" >"$tmp"; fi
  printf '%s\n' "$@" | awk -F'\t' -v OFS='\t' -v c="${TAG_CLIENT:-}" -v s="${TAG_SERVER:-}" '
    FILENAME == ARGV[1] { if (!($1 in cats)) cats[$1] = $2; next }
    {
      n = split(cats[$1], a, "|"); cl = sv = 0
      for (i = 1; i <= n; i++) { if (c != "" && a[i] ~ c) cl = 1; if (s != "" && a[i] ~ s) sv = 1 }
      print $1, (cl && sv) ? "both" : cl ? "client" : sv ? "server" : ""
    }' "$tmp" -
  rm -f -- "$tmp"
}

# ts_fetch_tags COMMUNITY Author-Name... -> "Author-Name<TAB>cat|cat" (6 at a
# time). A package Thunderstore doesn't have (HTTP error) gives an empty
# category list, so it isn't asked for again; a network error gives nothing.
ts_fetch_tags() {
  local community=$1
  shift
  printf '%s\n' "$@" | xargs -P 6 -I{} sh -c '
    m=$1; c=$2
    j=$(curl -fsS -m 10 "https://thunderstore.io/api/experimental/package/${m%%-*}/${m#*-}/" 2>/dev/null); rc=$?
    if [ "$rc" = 22 ]; then printf "%s\t\n" "$m"; exit 0; fi
    [ "$rc" = 0 ] || exit 0
    printf "%s" "$j" | perl -MJSON::PP -e '"'"'
        local $/; my $j = eval { decode_json(<STDIN>) } or exit 0;
        my @l = @{ $j->{community_listings} || [] };
        my @mine = grep { ($_->{community} // "") eq $ARGV[1] } @l;
        @mine = @l unless @mine;
        my %c; $c{$_} = 1 for map { @{ $_->{categories} || [] } } @mine;
        print "$ARGV[0]\t", join("|", sort keys %c), "\n";
      '"'"' "$m" "$c"
  ' _ {} "$community"
}

# tags_refresh -> look up every mod of every listed profile again (Edit menu)
tags_refresh() {
  local e s g p d sd games="" game n client=0 server=0 both=0 none=0 side
  local -a mods=()
  for e in "${ENTRIES[@]}"; do
    IFS=$'\t' read -r s g p d <<<"$e"
    load_game "$g" "$d"
    for sd in $SYNC_DIRS; do
      [ -d "$d/$sd" ] || continue
      while IFS= read -r n; do
        case $n in *-*) mods+=("$g/$n") ;; esac
      done < <(cd "$d/$sd" && ls -1)
    done
    case " $games " in *" $g "*) ;; *) games="$games $g" ;; esac
  done
  for game in $games; do
    load_game "$game"
    # shellcheck disable=SC2046  # one argument per mod folder name (no spaces)
    set -- $(printf '%s\n' "${mods[@]}" | awk -F/ -v g="$game" '$1 == g { print $2 }' | sort -u)
    [ $# -gt 0 ] || continue
    if ! tags_fetch "$game" "$@"; then
      warn "Couldn't reach Thunderstore - mod tags not refreshed."
      return 0
    fi
    while IFS=$'\t' read -r n side; do
      case $side in client) client=$((client + 1)) ;; server) server=$((server + 1)) ;;
                    both) both=$((both + 1)) ;; *) none=$((none + 1)) ;; esac
    done < <(mod_sides "" "$game" "$@")
  done
  ok "Mod tags refreshed: $client client-only, $server server-only, $both both, $none without a tag."
}

# ------------------------------------------------------ what differs --
# cfg_diff_detail LOCAL SERVER -> the differences ("Key: server -> yours", up
# to 4, separated by \036), empty
# if they only differ in comments, blank lines, spacing or line endings.
# .cfg files (BepInEx format) are compared per setting.
cfg_diff_detail() {
  awk -v lf="$1" -v sf="$2" '
    function load(file, arr, isc,   line, sec, k, v, n) {
      while ((getline line < file) > 0) {
        sub(/[\r \t]+$/, "", line); sub(/^[ \t]+/, "", line)
        if (line == "" || line ~ /^(#|;|\/\/)/) continue
        if (isc && line ~ /^\[.*\]$/) { sec = line; continue }
        if (isc && index(line, "=")) {
          k = substr(line, 1, index(line, "=") - 1); v = substr(line, index(line, "=") + 1)
          gsub(/^[ \t]+|[ \t]+$/, "", k); gsub(/^[ \t]+|[ \t]+$/, "", v)
          arr[sec "\t" k] = v; continue
        }
        arr["\tline\t" line]++
      }
      close(file)
    }
    function show(v) { return (length(v) > 24) ? substr(v, 1, 21) "..." : (v == "" ? "\"\"" : v) }
    BEGIN {
      isc = (lf ~ /\.cfg$/)
      load(lf, L, isc); load(sf, S, isc)
      for (k in L) if (!(k in S) || S[k] != L[k]) d[k] = 1
      for (k in S) if (!(k in L)) d[k] = 1
      n = 0; out = ""; lines = 0
      for (k in d) {
        if (k ~ /^\tline\t/) { lines++; continue }
        n++
        if (n <= 4) {
          split(k, p, "\t")
          out = out (out == "" ? "" : "\036") p[2] ": " ((k in S) ? show(S[k]) : "(none)") " -> " ((k in L) ? show(L[k]) : "(none)")
        }
      }
      if (n > 4) out = out "\036+" (n - 4) " more setting" (n > 5 ? "s" : "")
      if (lines) out = out (out == "" ? "" : "\036") lines " line" (lines > 1 ? "s" : "") " differ"
      print out
    }'
}

# cfg_diff_full LOCAL SERVER -> every difference, for "Show the full
# differences": .cfg files per setting with both values in full, other files
# as a line diff (comments and blank lines ignored).
cfg_diff_full() {
  if [[ $1 == *.cfg ]]; then
    awk -v lf="$1" -v sf="$2" '
      function load(file, arr, ord,   line, sec, k, v) {
        while ((getline line < file) > 0) {
          sub(/[\r \t]+$/, "", line); sub(/^[ \t]+/, "", line)
          if (line == "" || line ~ /^(#|;|\/\/)/) continue
          if (line ~ /^\[.*\]$/) { sec = line; continue }
          if (!index(line, "=")) continue
          k = substr(line, 1, index(line, "=") - 1); v = substr(line, index(line, "=") + 1)
          gsub(/^[ \t]+|[ \t]+$/, "", k); gsub(/^[ \t]+|[ \t]+$/, "", v)
          if (!((sec "\t" k) in seen)) { seen[sec "\t" k] = 1; order[++no] = sec "\t" k }
          arr[sec "\t" k] = v
        }
        close(file)
      }
      BEGIN {
        load(lf, L); load(sf, S)
        for (i = 1; i <= no; i++) {
          k = order[i]
          if ((k in L) && (k in S) && L[k] == S[k]) continue
          split(k, p, "\t")
          printf "    %s%s\n", ((p[1] == "" || p[1] == "[]") ? "" : p[1] " "), p[2]
          printf "        server: %s\n", (k in S) ? S[k] : "(not set)"
          printf "        yours:  %s\n", (k in L) ? L[k] : "(not set)"
        }
      }'
  else
    # diff exits 1 when the files differ, and head stops reading after 200
    # lines; under pipefail either would fail the caller, so it's ignored.
    diff <(sed -e 's/\r$//' -e '/^[[:space:]]*#/d' -e '/^[[:space:]]*$/d' "$2") \
         <(sed -e 's/\r$//' -e '/^[[:space:]]*#/d' -e '/^[[:space:]]*$/d' "$1") |
      sed -n -e 's/^< /    server: /p' -e 's/^> /    yours:  /p' | head -n 200 || true
  fi
}
CFG_FULL=""   # all differences of this scan, ready for ui_view
declare -A CFG_FULL_OF=()   # the same per file ("dir/path")

# ------------------------------------------------------------- compare --
# cfg_scan SOURCE PROFILE_DIR TARGET ROOT -> fills CFG_ITEMS
cfg_scan() {
  local src=$1 pdir=$2 target=$3 root=$4 d cdir e path mod side detail tmp code
  local -a args=()
  CFG_ITEMS=() CFG_HIDDEN=0 CFG_FULL="" CFG_FULL_OF=()
  [ -n "${CONFIG_DIRS:-}" ] || return 0
  tmp=$(mktemp -d "${TMPDIR:-/tmp}/gms.XXXXXX")
  for cdir in $CONFIG_DIRS; do
    [ -d "$pdir/$cdir" ] || continue
    args=(-rlc -s -n --out-format='%i %n' -e "$(rsh_cmd)")
    for e in $CONFIG_SKIP; do args+=("--exclude=$e"); done
    if ! ui_wait "Comparing configs ($cdir) ..." rsync "${args[@]}" "$pdir/$cdir/" "$target:$root/$cdir/"; then
      warn "Couldn't compare $cdir: $(printf '%s\n' "$WAIT_ERR" | tail -n 1)"
      continue
    fi
    # <f+++++++++ = not on the server; <f(anything else) = content differs
    printf '%s\n' "$WAIT_OUT" | awk '$1 ~ /^[<>]f/ { c = $1; sub(/^[^ ]+ /, ""); print (c ~ /^.f\+\+\+/ ? "new" : "differs") "\t" $0 }' >"$tmp/items"
    [ -s "$tmp/items" ] || continue

    # Server copies of the differing files, to show what differs.
    awk -F'\t' '$1 == "differs" { print $2 }' "$tmp/items" >"$tmp/differs"
    mkdir -p "$tmp/server"
    if [ -s "$tmp/differs" ]; then
      ui_wait "Reading the server's versions of ${cdir##*/} files ..." \
        rsync -rlc -s -e "$(rsh_cmd)" --files-from="$tmp/differs" "$target:$root/$cdir/" "$tmp/server/" || true
    fi

    # Which mod each file belongs to, and that mod's side.
    cut -f2 "$tmp/items" >"$tmp/files"
    : >"$tmp/owners"; : >"$tmp/mods"
    for d in $SYNC_DIRS; do
      [ -d "$pdir/$d" ] || continue
      (cd "$pdir/$d" && ls -1) >>"$tmp/mods"
      if declare -F "${LOADER}_config_owners" >/dev/null; then "${LOADER}_config_owners" "$pdir/$d" >>"$tmp/owners"; fi
    done
    cfg_owners "$tmp/files" "$tmp/mods" "$tmp/owners" >"$tmp/map"
    # shellcheck disable=SC2046  # one argument per mod folder name (no spaces)
    mod_sides "$src" "$GAME_ID" $(cut -f2 "$tmp/map" | awk 'NF' | sort -u) >"$tmp/sides"

    while IFS=$'\t' read -r code path; do
      mod=$(awk -F'\t' -v p="$path" '$1 == p { print $2; exit }' "$tmp/map")
      # Left over from a mod that's no longer in the profile: not worth
      # offering every time (shown with -v).
      if [ -z "$mod" ] && [ "$VERBOSE" != 1 ]; then CFG_HIDDEN=$((CFG_HIDDEN + 1)); continue; fi
      side=$(awk -F'\t' -v m="$mod" '$1 == m { print $2; exit }' "$tmp/sides")
      detail=""
      if [ "$code" = differs ]; then
        if [ -f "$tmp/server/$path" ]; then
          detail=$(cfg_diff_detail "$pdir/$cdir/$path" "$tmp/server/$path")
          [ -n "$detail" ] || continue          # only comments/spacing differ
          CFG_FULL_OF["$cdir/$path"]=$(cfg_diff_full "$pdir/$cdir/$path" "$tmp/server/$path")
          CFG_FULL="${CFG_FULL:+$CFG_FULL

}$path${mod:+  ($(cfg_mod_name "$mod"))}
${CFG_FULL_OF["$cdir/$path"]}"
        else
          detail="couldn't read the server's copy"
        fi
      fi
      CFG_ITEMS+=("$cdir$US$path$US$code$US$mod$US$side$US$detail")
    done <"$tmp/items"
  done
  rm -rf -- "$tmp"
  # New ones first, then by mod and file.
  if [ ${#CFG_ITEMS[@]} -gt 1 ]; then
    mapfile -t CFG_ITEMS < <(printf '%s\n' "${CFG_ITEMS[@]}" | sort -t "$US" -k3,3r -k4,4f -k2,2f)
  fi
  return 0
}

# Short mod name for labels: Azumatt-FastLink -> FastLink
cfg_mod_name() { local m=${1#*-}; printf '%s' "${m:-$1}"; }

# cut_start TEXT MAX -> "..." + the last MAX-3 chars (keeps a file name's end)
cut_start() { if [ ${#1} -le "$2" ]; then printf '%s' "$1"; else printf '...%s' "${1: -$(($2 - 3))}"; fi; }
# cut_end TEXT MAX -> TEXT cut at MAX chars with "..."
cut_end() { if [ ${#1} -le "$2" ]; then printf '%s' "$1"; else printf '%s...' "${1:0:$(($2 - 3))}"; fi; }

# cfg_layout -> CFG_ROWS (aligned checklist rows, one per offered item),
# CFG_ROW_IDX (their CFG_ITEMS index), CFG_ROW_ON (on/off), CFG_DIFFS (aligned
# "file  what differs" lines), CFG_CLIENT (client-only mods, comma separated).
CFG_ROWS=() CFG_ROW_IDX=() CFG_ROW_ON=() CFG_DIFFS=() CFG_CLIENT="" CFG_HEADER=""
cfg_layout() {
  local i dir path state mod side detail name wm=3 cols fmax dmax shown d
  CFG_ROWS=() CFG_ROW_IDX=() CFG_ROW_ON=() CFG_DIFFS=() CFG_CLIENT=""
  cols=$(tput cols 2>/dev/null || echo 100); [ "$cols" -gt 40 ] || cols=100
  # Column widths. A row is "status side mod file"; the file column gets what
  # is left of the terminal after the box, the checkbox and the other columns.
  for i in "${!CFG_ITEMS[@]}"; do
    IFS=$US read -r dir path state mod side detail <<<"${CFG_ITEMS[$i]}"
    [ "$side" = client ] && continue
    name=$(cfg_mod_name "${mod:-?}"); [ ${#name} -gt 26 ] && name=${name:0:26}
    [ ${#name} -gt "$wm" ] && wm=${#name}
  done
  fmax=$((cols - wm - 36)); [ "$fmax" -gt 70 ] && fmax=70; [ "$fmax" -lt 20 ] && fmax=20
  dmax=$((cols - 16)); [ "$dmax" -lt 30 ] && dmax=30
  CFG_HEADER=$(printf '%-8s %-7s %-*s  %s' "STATUS" "SIDE" "$wm" "MOD" "FILE")
  for i in "${!CFG_ITEMS[@]}"; do
    IFS=$US read -r dir path state mod side detail <<<"${CFG_ITEMS[$i]}"
    if [ "$side" = client ]; then
      CFG_CLIENT="${CFG_CLIENT:+$CFG_CLIENT, }$(cfg_mod_name "$mod")"
      continue
    fi
    name=$(cfg_mod_name "${mod:-?}"); [ ${#name} -gt 26 ] && name=${name:0:26}
    shown=$(cut_start "$path" "$fmax")
    CFG_ROWS+=("$(printf '%-8s %-7s %-*s  %s' "$state" "${side:-?}" "$wm" "$name" "$shown")")
    CFG_ROW_IDX+=("$i")
    if [ "$state" = new ] && [ -n "$mod" ]; then CFG_ROW_ON+=(on); else CFG_ROW_ON+=(off); fi
    if [ "$state" = differs ]; then
      CFG_DIFFS+=("  $(cut_start "$path" "$((cols - 12))")")
      while IFS= read -r -d $'\036' d || [ -n "$d" ]; do
        CFG_DIFFS+=("      $(cut_end "$d" "$dmax")")
      done <<<"$detail"
    fi
  done
  return 0
}

# ------------------------------------------------------------- choose --
# cfg_choose -> CFG_SELECTED (indexes into CFG_ITEMS); returns 1 on Back.
# Client-only mods' configs aren't offered; they're named above the list.
CFG_SELECTED=()
cfg_choose() {
  local i q rc
  local -a items=() on=()
  CFG_SELECTED=()
  cfg_layout
  [ ${#CFG_ROWS[@]} -gt 0 ] || return 0
  if [ "$ASSUME_YES" = 1 ]; then
    for i in "${!CFG_ROWS[@]}"; do [ "${CFG_ROW_ON[$i]}" = on ] && CFG_SELECTED+=("${CFG_ROW_IDX[$i]}"); done
    return 0
  fi
  q="Ticked files are copied to the server."
  [ -n "$CFG_CLIENT" ] && q="$q
Not copied (client mods only): $CFG_CLIENT"
  [ "$CFG_HIDDEN" -gt 0 ] && q="$q
Not shown: $CFG_HIDDEN file(s) from mods that aren't in this profile (-v shows them)"
  on=("${CFG_ROW_ON[@]}")
  local dir path rest
  local -a details=()
  for i in "${!CFG_ROWS[@]}"; do
    IFS=$US read -r dir path rest <<<"${CFG_ITEMS[${CFG_ROW_IDX[$i]}]}"
    details+=("${CFG_FULL_OF["$dir/$path"]:-}")
  done
  while :; do
    items=()
    for i in "${!CFG_ROWS[@]}"; do items+=("${CFG_ROWS[$i]}" "${on[$i]}"); done
    # Per-row details: the TUI expands rows in place (Right / Left). The text
    # version (and a terminal too small for that) offers "Show the full
    # differences" instead, which comes back here with the ticks kept.
    UI_DETAILS=("${details[@]}")
    ui_help "Config files

The mods' config files from your profile, compared with the server's.
Ticked files are copied to the server when you apply. Nothing is ever
deleted on the server, and a file that gets replaced is kept in the
backups (.mod-sync-backups in the game folder).

STATUS
  new      the server doesn't have this file yet. Ticked, so a mod you
           just added starts with your settings instead of its defaults.
  differs  the server has the file with other settings. Not ticked:
           copying it replaces the server's settings with yours.
           Rows with an arrow can be opened (Right) to see each setting
           that differs: the server's value, then yours.

SIDE (from the mod's Thunderstore tags)
  server   runs on the server
  both     runs on the server and on players' machines
  client   runs only on players' machines - these files are never
           copied and are listed above the table instead
  ?        no tags known (Edit > Refresh mod tags looks them up again)

Files that differ only in comments, spacing or line endings are not
listed. Files from mods that are no longer in this profile are hidden;
run with -v to see them."
    [ -z "$CFG_FULL" ] || UI_EXTRA="Show the full differences"
    ui_title "Config files"
    UI_LIST_HEADER=$CFG_HEADER
    rc=0; ui_checklist "$q" "${items[@]}" || rc=$?
    case $rc in
      0) break ;;
      3) for i in "${!on[@]}"; do on[i]=off; done
         for i in $CHECKED; do on[i - 1]=on; done
         ui_title "Differences (server -> yours)"
         ui_view "$CFG_FULL" ;;
      *) return 1 ;;
    esac
  done
  for i in $CHECKED; do CFG_SELECTED+=("${CFG_ROW_IDX[$((i - 1))]}"); done
  return 0
}

# Preview lines for the config part (dry runs and -y), same columns.
cfg_preview() {
  local i
  cfg_layout
  [ "$CFG_HIDDEN" -eq 0 ] || say "   ${D}$CFG_HIDDEN file(s) from mods that aren't in this profile not shown (-v shows them)${N}"
  [ -z "$CFG_CLIENT" ] || say "   ${D}Not copied (client mods only): $CFG_CLIENT${N}"
  [ ${#CFG_ROWS[@]} -eq 0 ] || say "     ${D}$CFG_HEADER${N}"
  for i in "${!CFG_ROWS[@]}"; do
    if [ "${CFG_ROW_ON[$i]}" = on ]; then say "   ${GRN}+${N} ${CFG_ROWS[$i]}"
    else say "   ${YEL}~${N} ${CFG_ROWS[$i]}"; fi
  done
  if [ "$VERBOSE" = 1 ] && [ -n "$CFG_FULL" ]; then
    say "   What differs (server -> yours), in full:"
    while IFS= read -r i; do say "   $i"; done <<<"$CFG_FULL"
  elif [ ${#CFG_DIFFS[@]} -gt 0 ]; then
    say "   What differs (server -> yours; -v for all of it):"
    for i in "${CFG_DIFFS[@]}"; do say "   $i"; done
  fi
}
CFG_OFFERED=0

# cfg_review: the picked config files (CFG_SELECTED), one line each
cfg_review() {
  local i j
  for i in "${CFG_SELECTED[@]}"; do
    for j in "${!CFG_ROW_IDX[@]}"; do
      [ "${CFG_ROW_IDX[$j]}" = "$i" ] && say "   ${CFG_ROWS[$j]}"
    done
  done
  return 0
}

# cfg_copy PROFILE_DIR TARGET ROOT TIMESTAMP -> copies CFG_SELECTED, with backups like the mods
cfg_copy() {
  local pdir=$1 target=$2 root=$3 ts=$4 i dir path rest list cdir
  local -a args=()
  [ ${#CFG_SELECTED[@]} -gt 0 ] || return 0
  list=$(mktemp "${TMPDIR:-/tmp}/gms.XXXXXX")
  for cdir in $CONFIG_DIRS; do
    : >"$list"
    for i in "${CFG_SELECTED[@]}"; do
      IFS=$US read -r dir path rest <<<"${CFG_ITEMS[$i]}"
      [ "$dir" = "$cdir" ] && printf '%s\n' "$path" >>"$list"
    done
    [ -s "$list" ] || continue
    # shellcheck disable=SC2054  # the commas are part of --chmod's value
    args=(-rlc -s --chmod=D755,F644 -e "$(rsh_cmd)" --files-from="$list")
    [ "${KEEP_BACKUPS:-0}" -gt 0 ] && args+=(--backup "--backup-dir=$root/.mod-sync-backups/$ts/$cdir")
    ui_wait "Preparing $cdir on the server ..." \
      rssh "$target" "mkdir -p $(shq "$root/$cdir")" || die "can't create $root/$cdir on the server"
    ui_wait "Copying $(wc -l <"$list" | tr -d ' ') config file(s) ..." \
      rsync "${args[@]}" "$pdir/$cdir/" "$target:$root/$cdir/" \
      || { rm -f -- "$list"; die "copying configs failed
$(printf '%s\n' "$WAIT_ERR" | tail -n 3)"; }
  done
  rm -f -- "$list"
}

# CFG_OFFERED: configs that can be copied (not client-only).
cfg_count() {
  local i dir path state mod side rest
  CFG_OFFERED=0
  for i in "${!CFG_ITEMS[@]}"; do
    IFS=$US read -r dir path state mod side rest <<<"${CFG_ITEMS[$i]}"
    [ "$side" = client ] || CFG_OFFERED=$((CFG_OFFERED + 1))
  done
}
