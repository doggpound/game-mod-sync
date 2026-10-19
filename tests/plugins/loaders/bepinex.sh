# shellcheck shell=bash
# Mod loader plugin: BepInEx (plugins/loaders/bepinex.sh)
# badge: loader: BepInEx

t_detects_a_BepInEx_profile() {
  local p
  p=$(gale_profile valheim P)
  load_app
  bepinex_detect "$p"
}

t_does_not_detect_other_folders() {
  mkdir -p "$SANDBOX/plain/mods"
  load_app
  if bepinex_detect "$SANDBOX/plain"; then fail "a folder without BepInEx/ was detected"; fi
}

t_sets_the_folders_to_sync() {
  load_app
  bepinex_defaults
  assert_eq "BepInEx/plugins" "$SYNC_DIRS" SYNC_DIRS
  assert_eq "BepInEx/config" "$CONFIG_DIRS" CONFIG_DIRS
  assert_contains "$EXCLUDES" "MMHOOK/" EXCLUDES
  assert_contains "$CONFIG_SKIP" "/BepInEx.cfg" CONFIG_SKIP
}

t_maps_config_files_to_mods_from_the_plugin_declaration() {
  command -v perl >/dev/null || { echo "no perl"; exit 77; }
  local d=$SANDBOX/plugins
  fake_plugin_dll "$d/Azumatt-FastLink/FastLink.dll" Azumatt.FastLink FastLink 1.4.2
  fake_plugin_dll "$d/ArgusMagnus-ServersideQoL/ServersideQoL.dll" argusmagnus.ServersideQoL ServersideQoL 1.0.10
  fake_plugin_dll "$d/MMHOOK/MMHOOK_x.dll" mmhook.id MMHOOK 1.0.0
  load_app
  assert_eq "$(printf 'Azumatt.FastLink\tAzumatt-FastLink\nargusmagnus.ServersideQoL\tArgusMagnus-ServersideQoL' | sort)" \
    "$(bepinex_config_owners "$d")" "owners (MMHOOK left out)"
}

t_a_mod_that_only_mentions_another_is_not_its_owner() {
  command -v perl >/dev/null || { echo "no perl"; exit 77; }
  local d=$SANDBOX/plugins
  mkdir -p "$d/Someone-Addon"
  # a dependency string, not a declaration: no 01 00 prefix, no version after it
  printf 'MZ..Azumatt.FastLink..dependency..' >"$d/Someone-Addon/Addon.dll"
  load_app
  assert_eq "" "$(bepinex_config_owners "$d")" "owners"
}

t_is_picked_by_load_game_for_a_BepInEx_profile() {
  local p
  p=$(gale_profile valheim P)
  load_app
  load_game valheim "$p"
  assert_eq bepinex "$LOADER" LOADER
  assert_eq "BepInEx/plugins" "$GAME_MARKER" GAME_MARKER
}
