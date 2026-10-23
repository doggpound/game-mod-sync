# shellcheck shell=bash
# Server type plugin: CubeCoders AMP (plugins/backends/amp.sh)
# badge: server: AMP

t_finds_instances_that_have_the_game_folder() {
  local r1 r2 out
  r1=$(amp_instance Valheim01); r2=$(amp_instance Valheim02)
  mkdir -p "$REMOTE/.ampdata/instances/Minecraft01/Minecraft/world"     # no BepInEx
  load_app
  out=$(amp_candidates amp@server BepInEx/plugins | awk -F'\t' '{ print $2 }')
  assert_eq "$(printf '%s\n%s' "$r1" "$r2")" "$out" "game roots"   # sorted
}

t_labels_candidates_with_the_instance_name() {
  amp_instance Valheim01 >/dev/null
  load_app
  assert_contains "$(amp_candidates amp@server BepInEx/plugins | cut -f1)" "Valheim01" "label"
}

t_skips_backup_copies() {
  local r
  r=$(amp_instance V1)
  mkdir -p "$r/.mod-sync-backups/20260101-000000/BepInEx/plugins"
  load_app
  assert_eq 1 "$(amp_candidates amp@server BepInEx/plugins | wc -l | tr -d ' ')" "candidates"
}

t_no_instances_is_an_empty_list() {
  load_app
  assert_eq "" "$(amp_candidates amp@server BepInEx/plugins)" "candidates"
}

t_reports_files_the_server_installs_itself() {
  local r
  r=$(amp_instance V1)
  printf 'App.AppSettings={"valheim_plus_install":"true","other":"x"}\n' >"$REMOTE/.ampdata/instances/V1/GenericModule.kvp"
  load_app
  assert_eq "$(printf 'ValheimPlus.dll\tAMP setting "valheim_plus_install" is on for V1')" \
    "$(amp_managed amp@server "$r" "valheim_plus_install=ValheimPlus.dll")" "managed"
}

t_reports_nothing_when_the_setting_is_off() {
  local r
  r=$(amp_instance V1)
  printf 'App.AppSettings={"valheim_plus_install":"false"}\n' >"$REMOTE/.ampdata/instances/V1/GenericModule.kvp"
  load_app
  assert_eq "" "$(amp_managed amp@server "$r" "valheim_plus_install=ValheimPlus.dll")" "managed"
}

t_ignores_folders_outside_AMP() {
  load_app
  assert_eq "" "$(amp_managed amp@server /srv/valheim "valheim_plus_install=ValheimPlus.dll")" "managed"
}

t_short_name_is_the_instance() {
  load_app
  assert_eq Valheim01 "$(where_of amp "/home/amp/.ampdata/instances/Valheim01/Valheim/896660")" "where_of"
}

t_reads_the_save_folder_and_world_from_the_settings() {
  local r
  r=$(amp_instance V1)
  printf '%s\n' 'App.CommandLineArgs=-name "{{ServerName}}" -world "{{WorldName}}" -savedir "Saves" {{crossplay}}' \
    'App.AppSettings={"ServerName":"x","WorldName":"Midgard"}' >"$REMOTE/.ampdata/instances/V1/GenericModule.kvp"
  load_app
  assert_eq "$(printf 'Saves\tMidgard')" "$(amp_saves amp@server "$r" "-savedir WorldName")" "saves"
}

t_save_folder_with_spaces() {
  local r
  r=$(amp_instance V1)
  printf '%s\n' 'App.CommandLineArgs=-savedir "My Saves" -port 1' >"$REMOTE/.ampdata/instances/V1/GenericModule.kvp"
  load_app
  assert_eq "$(printf 'My Saves\t')" "$(amp_saves amp@server "$r" "-savedir WorldName")" "saves"
}

t_template_values_are_not_used() {
  local r
  r=$(amp_instance V1)
  printf '%s\n' 'App.CommandLineArgs=-savedir "{{SaveDir}}"' 'App.AppSettings={"WorldName":"{{x}}"}' \
    >"$REMOTE/.ampdata/instances/V1/GenericModule.kvp"
  load_app
  assert_eq "" "$(amp_saves amp@server "$r" "-savedir WorldName")" "saves"
}
