# shellcheck shell=bash
# World format plugin: Valheim's chunked saves (plugins/worlds/valheim.sh)
# badge: world: Valheim

t_lists_worlds_with_their_last_save() {
  local d="$SANDBOX/worlds" out
  valheim_world "$d" Midgard 1940 >/dev/null
  valheim_world "$d" Ashlands 7 >/dev/null
  load_app
  out=$(cd "$d" && sh -c "$(valheim_list_sh)" | sort)
  assert_eq "$(printf 'Ashlands\nMidgard')" "$(printf '%s\n' "$out" | cut -f1)" "worlds"
  assert_contains "$out" "save 1940" "details"
}

t_skips_folders_without_a_finished_save() {
  local d="$SANDBOX/worlds"
  valheim_world "$d" Done 3 >/dev/null
  mkdir -p "$d/Half" "$d/Junk"
  echo x >"$d/Half/_main.1.fwl2"            # a save that never finished
  touch "$d/OldStyle.fwl" "$d/OldStyle.db"   # converted by the game on load
  load_app
  assert_eq Done "$(cd "$d" && sh -c "$(valheim_list_sh)" | cut -f1)" "worlds"
}

t_a_world_is_its_folder() {
  load_app
  assert_eq "My World" "$(valheim_paths "My World")" "paths"
}

t_the_stamp_is_the_last_finished_save() {
  local d="$SANDBOX/worlds" w
  w=$(valheim_world "$d" W 12)
  echo x >"$w/_main.13.fwl2"                 # save 13 in progress
  load_app
  assert_eq "save 12" "$(cd "$d" && sh -c "$(valheim_stamp_sh W)")" "stamp"
}

t_no_stamp_when_a_finished_save_is_missing_a_part() {
  local d="$SANDBOX/worlds" w
  w=$(valheim_world "$d" W 5)
  rm "$w/_main.5.db2"
  load_app
  assert_eq "" "$(cd "$d" && sh -c "$(valheim_stamp_sh W)")" "stamp"
}

t_no_stamp_for_a_missing_world() {
  load_app
  mkdir -p "$SANDBOX/worlds"
  assert_eq "" "$(cd "$SANDBOX/worlds" && sh -c "$(valheim_stamp_sh "Nope")")" "stamp"
}

t_lists_the_games_own_backups_last_newest_first() {
  local d="$SANDBOX/worlds"
  valheim_world "$d" Ashlands 9 >/dev/null
  valheim_world "$d" Ashlands_backup_auto-20261022-200802 8 >/dev/null
  valheim_world "$d" Ashlands_backup_auto-20261022-220802 9 >/dev/null
  valheim_world "$d" Zebra 1 >/dev/null
  load_app
  assert_eq "$(printf 'Ashlands\nZebra\nAshlands_backup_auto-20261022-220802\nAshlands_backup_auto-20261022-200802')" \
    "$(cd "$d" && sh -c "$(valheim_list_sh)" | cut -f1)" "order"
}
