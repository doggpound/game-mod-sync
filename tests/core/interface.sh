# shellcheck shell=bash
# One command, two interfaces: --cli / --tui, else DEFAULT_UI from the config.
# badge: interface choice

iface() { "$REPO/game-mod-sync" "$@" --config 2>&1 | sed -n 's/^interface: *//p'; }
set_default() { mkdir -p "$GMS_CONFIG_DIR"; printf 'DEFAULT_UI=%s\n' "$1" >"$GMS_CONFIG_DIR/config"; }

t_text_by_default() {
  assert_eq "cli (default)" "$(iface)" "interface"
}

t_tui_flag_picks_full_screen() {
  assert_eq "tui (--tui)" "$(iface --tui)" "interface"
}

t_cli_flag_wins_over_the_saved_default() {
  set_default tui
  assert_eq "cli (--cli)" "$(iface --cli)" "interface"
}

t_saved_default_cli_is_used() {
  set_default cli
  assert_eq "cli (DEFAULT_UI=cli)" "$(iface)" "interface"
}

t_saved_tui_falls_back_to_text_without_a_terminal() {
  set_default tui
  assert_eq "cli (DEFAULT_UI=tui, but no terminal)" "$(iface)" "interface"
}

t_the_default_is_saved_in_the_config_file() {
  load_app
  DEFAULT_UI=tui
  save_config
  assert_contains "$(cat "$GMS_CONFIG_DIR/config")" "DEFAULT_UI=tui" "config file"
}
