# shellcheck shell=bash
# Source plugin: Gale (plugins/sources/gale.sh)
# badge: source: Gale

t_finds_the_data_folder_in_the_usual_place() {
  gale_profile valheim Default >/dev/null
  load_app
  gale_init
  assert_eq "$(GALE)" "$GALE_ROOT" "GALE_ROOT"
}

t_GALE_DATA_DIR_overrides_the_search() {
  mkdir -p "$SANDBOX/elsewhere/valheim/profiles/P"
  load_app
  GALE_DATA_DIR="$SANDBOX/elsewhere"
  gale_init
  assert_eq "$SANDBOX/elsewhere" "$GALE_ROOT" "GALE_ROOT"
}

t_no_data_folder_means_not_found() {
  load_app
  if gale_init; then fail "gale_init should fail without a data folder"; fi
}

t_lists_every_game_and_profile() {
  local a b c out
  a=$(gale_profile valheim MyProfile); b=$(gale_profile valheim Vanilla); c=$(gale_profile lethal-company Fun)
  load_app
  gale_init
  out=$(gale_list | sort)
  assert_eq "$(printf 'lethal-company\tFun\t%s\nvalheim\tMyProfile\t%s\nvalheim\tVanilla\t%s' "$c" "$a" "$b")" "$out" "gale_list"
}

t_ignores_files_where_profiles_should_be() {
  gale_profile valheim Real >/dev/null
  touch "$(GALE)/valheim/profiles/stray.txt"
  load_app
  gale_init
  assert_eq 1 "$(gale_list | wc -l | tr -d ' ')" "profiles listed"
}

t_reads_mod_tags_from_the_thunderstore_cache() {
  command -v perl >/dev/null || { echo "no perl"; exit 77; }
  gale_profile valheim P >/dev/null
  cat >"$(GALE)/valheim/thunderstore_cache.json" <<'JSON'
[{"full_name":"Azumatt-FastLink","categories":["Mods","Client-side"]},
 {"full_name":"ArgusMagnus-ServersideQoL","categories":["Server-side"]},
 {"name":"no full name"}]
JSON
  load_app
  gale_init
  assert_eq "$(printf 'Azumatt-FastLink\tMods|Client-side\nArgusMagnus-ServersideQoL\tServer-side')" "$(gale_mod_tags valheim)" "gale_mod_tags"
}

t_a_broken_thunderstore_cache_is_ignored() {
  gale_profile valheim P >/dev/null
  echo '{not json' >"$(GALE)/valheim/thunderstore_cache.json"
  load_app
  gale_init
  assert_eq "" "$(gale_mod_tags valheim)" "gale_mod_tags"
}
