# shellcheck shell=bash
# Game plugin: Valheim (plugins/games/valheim.conf)
# badge: game: Valheim

t_has_its_name_and_server_match() {
  load_app
  load_game valheim
  assert_eq Valheim "$GAME_NAME" GAME_NAME
  # matched case-insensitively, like setup does
  echo Valheim01 | grep -Eiq "$SERVER_MATCH" || fail "SERVER_MATCH [$SERVER_MATCH] should match Valheim01"
  echo /home/amp/x/896660 | grep -Eiq "$SERVER_MATCH" || fail "SERVER_MATCH should match the app id 896660"
  if echo LethalCompany01 | grep -Eiq "$SERVER_MATCH"; then fail "SERVER_MATCH should not match other games"; fi
}

t_never_touches_server_only_files() {
  load_app
  load_game valheim
  assert_contains "$EXCLUDES" "MMHOOK/" EXCLUDES
  assert_contains "$EXCLUDES" "Valheim.DisplayBepInExInfo.dll" EXCLUDES
}

t_knows_what_AMP_installs_itself() {
  load_app
  load_game valheim
  # shellcheck disable=SC2154  # set by valheim.conf
  assert_eq "valheim_plus_install=ValheimPlus.dll" "$MANAGED_amp" MANAGED_amp
}

t_gets_its_folders_from_the_loader() {
  local p
  p=$(gale_profile valheim P)
  load_app
  load_game valheim "$p"
  assert_eq "BepInEx/plugins" "$SYNC_DIRS" SYNC_DIRS
  assert_eq "BepInEx/config" "$CONFIG_DIRS" CONFIG_DIRS
}
