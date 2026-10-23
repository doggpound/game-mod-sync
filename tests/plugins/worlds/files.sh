# shellcheck shell=bash
# World format plugin: one file per part (plugins/worlds/files.sh)
# badge: world: Files

t_lists_worlds_by_their_first_file() {
  local d="$SANDBOX/worlds"
  mkdir -p "$d"
  touch "$d/Alpha.fwl" "$d/Alpha.db" "$d/Beta.fwl" "$d/Alpha.fwl.old" "$d/notes.txt"
  load_app
  WORLD_FILES="fwl db"
  assert_eq "$(printf 'Alpha\nBeta')" "$(cd "$d" && sh -c "$(files_list_sh)" | cut -f1 | sort)" "worlds"
}

t_a_world_is_one_file_per_extension() {
  load_app
  WORLD_FILES="fwl db"
  assert_eq "$(printf 'My World.fwl\nMy World.db')" "$(files_paths "My World")" "paths"
}

t_the_stamp_follows_the_contents() {
  local d="$SANDBOX/worlds" a b
  mkdir -p "$d"
  echo one >"$d/W.fwl"; echo data >"$d/W.db"
  load_app
  WORLD_FILES="fwl db"
  a=$(cd "$d" && sh -c "$(files_stamp_sh W)")
  touch -d '2001-01-01' "$d/W.db"
  assert_eq "$a" "$(cd "$d" && sh -c "$(files_stamp_sh W)")" "stamp after a time change"
  echo more >>"$d/W.db"
  b=$(cd "$d" && sh -c "$(files_stamp_sh W)")
  if [ -z "$a" ] || [ "$a" = "$b" ]; then fail "stamp should change with the contents: [$a] [$b]"; fi
}

t_no_stamp_when_a_part_is_missing() {
  local d="$SANDBOX/worlds"
  mkdir -p "$d"
  echo one >"$d/W.fwl"
  load_app
  WORLD_FILES="fwl db"
  assert_eq "" "$(cd "$d" && sh -c "$(files_stamp_sh W)")" "stamp"
}
