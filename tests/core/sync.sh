# shellcheck shell=bash
# End to end: a Gale Valheim profile synced to an AMP server with the text UI
# badge: end-to-end sync
# and -y (no questions), through the fake ssh and real rsync.

setup_profile_and_server() {
  P=$(gale_profile valheim MyProfile)
  fake_plugin_dll "$P/BepInEx/plugins/Azumatt-AzuClock/AzuClock.dll" Azumatt.AzuClock AzuClock 1.0.0
  fake_plugin_dll "$P/BepInEx/plugins/ArgusMagnus-ServersideQoL/ServersideQoL.dll" argusmagnus.ServersideQoL ServersideQoL 1.0.10
  printf '[General]\nShow = true\n' >"$P/BepInEx/config/Azumatt.AzuClock.cfg"
  printf '[General]\nOn = true\n' >"$P/BepInEx/config/argusmagnus.ServersideQoL.cfg"
  S=$(amp_instance Valheim01)
  mkdir -p "$S/BepInEx/plugins/Old-RemovedMod" "$S/BepInEx/plugins/MMHOOK"
  echo old >"$S/BepInEx/plugins/Old-RemovedMod/Old.dll"
  echo generated >"$S/BepInEx/plugins/MMHOOK/MMHOOK_assembly.dll"
  printf '[General]\nOn = false\n' >"$S/BepInEx/config/argusmagnus.ServersideQoL.cfg"
  mkdir -p "$GMS_CONFIG_DIR"
  printf 'gale\tvalheim\tMyProfile\tamp\tamp@server\t%s\t\n' "$S" >"$GMS_CONFIG_DIR/targets.tsv"
}
sync() { "$REPO/game-mod-sync" -p MyProfile -y "$@" 2>&1; }

t_sends_new_mods_and_removes_old_ones() {
  command -v rsync >/dev/null || { echo "no rsync"; exit 77; }
  setup_profile_and_server
  sync >"$SANDBOX/out" || { cat "$SANDBOX/out"; fail "sync failed"; }
  assert_file "$S/BepInEx/plugins/Azumatt-AzuClock/AzuClock.dll"
  assert_file "$S/BepInEx/plugins/ArgusMagnus-ServersideQoL/ServersideQoL.dll"
  assert_no_file "$S/BepInEx/plugins/Old-RemovedMod"
}

t_keeps_a_backup_of_removed_files() {
  command -v rsync >/dev/null || { echo "no rsync"; exit 77; }
  setup_profile_and_server
  sync >/dev/null
  assert_eq 1 "$(find "$S/.mod-sync-backups" -name Old.dll | wc -l | tr -d ' ')" "backed-up Old.dll"
}

t_leaves_server_generated_files_alone() {
  command -v rsync >/dev/null || { echo "no rsync"; exit 77; }
  setup_profile_and_server
  sync >/dev/null
  assert_file "$S/BepInEx/plugins/MMHOOK/MMHOOK_assembly.dll"
}

t_copies_new_configs_but_not_differing_ones_by_default() {
  command -v rsync >/dev/null || { echo "no rsync"; exit 77; }
  command -v perl >/dev/null || { echo "no perl"; exit 77; }
  setup_profile_and_server
  sync >/dev/null
  assert_file "$S/BepInEx/config/Azumatt.AzuClock.cfg"
  assert_contains "$(cat "$S/BepInEx/config/argusmagnus.ServersideQoL.cfg")" "On = false" "server's own setting"
}

t_a_second_run_sends_nothing() {
  command -v rsync >/dev/null || { echo "no rsync"; exit 77; }
  setup_profile_and_server
  sync >/dev/null
  local out
  out=$(sync)
  assert_contains "$out" "Mods are in sync" "second run"
  assert_contains "$out" "nothing changed" "second run"   # the differing config is left unticked
}

t_already_in_sync_when_nothing_differs() {
  command -v rsync >/dev/null || { echo "no rsync"; exit 77; }
  setup_profile_and_server
  cp "$P/BepInEx/config/argusmagnus.ServersideQoL.cfg" "$S/BepInEx/config/"
  sync >/dev/null
  assert_contains "$(sync)" "Already in sync" "second run"
}

t_dry_run_changes_nothing() {
  command -v rsync >/dev/null || { echo "no rsync"; exit 77; }
  setup_profile_and_server
  assert_contains "$(sync -n)" "Dry run" "output"
  assert_file "$S/BepInEx/plugins/Old-RemovedMod/Old.dll"
  assert_no_file "$S/BepInEx/plugins/Azumatt-AzuClock"
}

t_a_differing_config_that_is_not_a_cfg_file_is_shown() {
  command -v rsync >/dev/null || { echo "no rsync"; exit 77; }
  command -v perl >/dev/null || { echo "no perl"; exit 77; }
  setup_profile_and_server
  mkdir -p "$P/BepInEx/config/argusmagnus.ServersideQoL" "$S/BepInEx/config/argusmagnus.ServersideQoL"
  printf 'Throttle:\n  Max: 100\n' >"$P/BepInEx/config/argusmagnus.ServersideQoL/Advanced.yml"
  printf 'Throttle:\n  Max: 50\n' >"$S/BepInEx/config/argusmagnus.ServersideQoL/Advanced.yml"
  local out
  out=$(sync -n -v) || { printf '%s\n' "$out"; fail "dry run failed"; }
  assert_contains "$out" "Advanced.yml" "output"
  assert_contains "$out" "Dry run" "output"
}
