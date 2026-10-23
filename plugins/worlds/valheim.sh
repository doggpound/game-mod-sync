# shellcheck shell=bash
# World format: Valheim's chunked saves (newer Valheim versions).
#
# Each world is a folder in worlds_local/ named after the world:
#   <world>/_main.<N>.fwl2   world metadata   \
#   <world>/_main.<N>.db2    world data        > save number N
#   <world>/_main.<N>.chunks chunk index      /
#   <world>/_main.<N>.ok     written last: save N is complete
#   <world>/*.chunk          terrain and objects, one file per area
# A save writes the changed .chunk files and _main.<N+1>.*, then the .ok, and
# only then removes what save N used, so save N stays whole until N+1 is done.
# (Worked out from the folders of a live server; the format isn't documented.)
# The game's own backups are worlds too (<world>_backup_auto-<time>/); they're
# listed after the others, newest first. A backup is never written to, so it's
# the safest copy to take.
# Older worlds (<world>.fwl + <world>.db) are converted by the game the first
# time it loads them, so they aren't offered.
#
# A world format provides:
#   <name>_label            human name
#   <name>_list_sh          shell code, run in a worlds folder (sh, on the server):
#                           print "world<TAB>details" for every complete world,
#                           in the order to offer them
#   <name>_paths WORLD      the names in the worlds folder that make up WORLD
#   <name>_stamp_sh WORLD   shell code, run in a worlds folder (the server's or the
#                           downloaded copy): print something that changes with
#                           every save, nothing if there's no complete save

valheim_label() { printf 'Valheim (chunked saves)'; }

valheim_list_sh() {
  cat <<'SH'
for d in */; do
  d=${d%/}
  n=$(ls "$d" 2>/dev/null | sed -n 's/^_main\.\([0-9][0-9]*\)\.ok$/\1/p' | sort -n | tail -n 1)
  [ -n "$n" ] || continue
  printf '%s\tsave %s, %s, %s\n' "$d" "$n" "$(date -r "$d/_main.$n.ok" '+%Y-%m-%d %H:%M')" "$(du -sh "$d" | cut -f1)"
done | awk -F'\t' '$1 !~ /_backup/ { print; next } { b[++n] = $0 } END { while (n) print b[n--] }'
SH
}

valheim_paths() { printf '%s\n' "$1"; }

valheim_stamp_sh() {
  printf 'cd %s 2>/dev/null || exit 0\n' "$(shq "$1")"
  cat <<'SH'
n=$(ls | sed -n 's/^_main\.\([0-9][0-9]*\)\.ok$/\1/p' | sort -n | tail -n 1)
[ -n "$n" ] && [ -f "_main.$n.fwl2" ] && [ -f "_main.$n.db2" ] && [ -f "_main.$n.chunks" ] && echo "save $n"
exit 0
SH
}
