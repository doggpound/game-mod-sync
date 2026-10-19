# shellcheck shell=bash
# Game plugin: defaults for any game without its own file (plugins/games/_default.conf)
# badge: game: any other

t_names_a_game_from_its_id() {
  load_app
  load_game lethal-company
  assert_eq "Lethal Company" "$GAME_NAME" GAME_NAME
}

t_matches_server_folders_with_or_without_dashes() {
  load_app
  load_game lethal-company
  local f
  for f in LethalCompany01 lethal-company lethal_company "Lethal Company"; do
    echo "$f" | grep -Eiq "$SERVER_MATCH" || fail "SERVER_MATCH [$SERVER_MATCH] should match [$f]"
  done
}

t_uses_the_game_id_as_the_Thunderstore_community() {
  load_app
  load_game lethal-company
  assert_eq lethal-company "$THUNDERSTORE_COMMUNITY" THUNDERSTORE_COMMUNITY
  echo Client-side | grep -Eq "$TAG_CLIENT" || fail "TAG_CLIENT should match Client-side"
  echo Server-side | grep -Eq "$TAG_SERVER" || fail "TAG_SERVER should match Server-side"
}

t_a_user_override_wins() {
  mkdir -p "$GMS_CONFIG_DIR/plugins/games"
  echo 'GAME_NAME="My Game"' >"$GMS_CONFIG_DIR/plugins/games/lethal-company.conf"
  load_app
  load_game lethal-company
  assert_eq "My Game" "$GAME_NAME" GAME_NAME
}
