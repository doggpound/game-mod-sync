# shellcheck shell=bash
# Server type plugin: Other (SSH, enter the game folder) (plugins/backends/ssh.sh)
# badge: server: SSH

t_resolves_a_folder_under_home() {
  mkdir -p "$REMOTE/servers/valheim/BepInEx/plugins"
  answers "~/servers/valheim"
  load_app
  assert_eq "$(printf '%s\t%s' "$REMOTE/servers/valheim" "$REMOTE/servers/valheim")" \
    "$(ssh_candidates me@server BepInEx/plugins 2>/dev/null)" "candidate"
}

t_accepts_an_absolute_folder() {
  mkdir -p "$REMOTE/abs/game/BepInEx/plugins"
  answers "$REMOTE/abs/game"
  load_app
  assert_eq "$REMOTE/abs/game" "$(ssh_candidates me@server BepInEx/plugins 2>/dev/null | cut -f2)" "root"
}

t_warns_when_the_game_folder_has_no_mods_yet() {
  mkdir -p "$REMOTE/fresh"
  answers "fresh"
  load_app
  assert_contains "$(ssh_candidates me@server BepInEx/plugins 2>&1 >/dev/null)" "will be created on the first sync" "warning"
}

t_refuses_a_folder_that_does_not_exist() {
  answers "~/nope"
  load_app
  if (ssh_candidates me@server BepInEx/plugins) >/dev/null 2>&1; then fail "a missing folder was accepted"; fi
}

t_is_listed_last() {
  load_app
  assert_eq ssh "${BACKENDS##* }" "last server type"
}
