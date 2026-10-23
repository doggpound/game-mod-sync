# shellcheck shell=bash
# World format: one file per part, <world>.<ext>, side by side in the worlds
# folder. The game file lists the extensions in WORLD_FILES, the first one
# being the file every world has, e.g. WORLD_FILES="fwl db" for Valheim's
# older saves (MyWorld.fwl + MyWorld.db). See plugins/worlds/valheim.sh for
# what a world format provides.

files_label() { printf 'Files (<world>.<ext>)'; }

files_list_sh() {
  local first=${WORLD_FILES%% *}
  printf 'x=%s\n' "$(shq "$first")"
  cat <<'SH'
for f in *."$x"; do
  [ -f "$f" ] || continue
  printf '%s\t%s, %s\n' "${f%."$x"}" "$(date -r "$f" '+%Y-%m-%d %H:%M')" "$(du -sh "$f" | cut -f1)"
done
SH
}

files_paths() {
  local e
  for e in $WORLD_FILES; do printf '%s.%s\n' "$1" "$e"; done
}

# Content checksums: a copy's times may differ from the server's.
files_stamp_sh() {
  local p list=""
  while IFS= read -r p; do list="$list $(shq "$p")"; done < <(files_paths "$1")
  printf 'for f in%s; do [ -f "$f" ] || exit 0; done\ncksum%s\n' "$list" "$list"
}
