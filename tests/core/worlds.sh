# shellcheck shell=bash
# End to end: a world downloaded from an AMP server into the game's local
# badge: world download
# saves (lib/worlds.sh), through the fake ssh and real rsync.

setup_world_download() {
  P=$(gale_profile valheim MyProfile)
  fake_plugin_dll "$P/BepInEx/plugins/Azumatt-AzuClock/AzuClock.dll" Azumatt.AzuClock AzuClock 1.0.0
  S=$(amp_instance Valheim01)
  printf '%s\n' 'App.CommandLineArgs=-name "{{ServerName}}" -world "{{WorldName}}" -savedir "Saves" {{crossplay}}' \
    'App.AppSettings={"WorldName":"Midgard","saveinterval":"1800"}' >"$REMOTE/.ampdata/instances/Valheim01/GenericModule.kvp"
  SW="$S/Saves/worlds_local"
  valheim_world "$SW" Midgard 1940 >/dev/null
  valheim_world "$SW" Ashlands 55 >/dev/null
  LW="$LOCAL/mnt/c/Users/me/AppData/LocalLow/IronGate/Valheim/worlds_local"
  mkdir -p "$LW"
  mkdir -p "$GMS_CONFIG_DIR"
  printf 'gale\tvalheim\tMyProfile\tamp\tamp@server\t%s\t\n' "$S" >"$GMS_CONFIG_DIR/targets.tsv"
}
download() { "$REPO/game-mod-sync" -p MyProfile --download-world -y "$@" 2>&1; }
need_rsync() { command -v rsync >/dev/null || { echo "no rsync"; exit 77; }; }

t_downloads_the_world_the_server_runs() {
  need_rsync
  setup_world_download
  download >"$SANDBOX/out" || { cat "$SANDBOX/out"; fail "download failed"; }
  assert_file "$LW/Midgard/_main.1940.db2"
  assert_file "$LW/Midgard/_main.1940.ok"
  assert_file "$LW/Midgard/10_1a__1_1940.chunk"
  assert_no_file "$LW/Ashlands"
  assert_contains "$(cat "$SANDBOX/out")" "Downloaded Midgard (save 1940)" "output"
}

t_world_picks_another_one() {
  need_rsync
  setup_world_download
  download --world Ashlands >/dev/null
  assert_file "$LW/Ashlands/_main.55.ok"
  assert_no_file "$LW/Midgard"
}

t_the_menu_offers_every_world_with_the_running_one_first() {
  need_rsync
  setup_world_download
  answers 2 y                                  # Ashlands, then yes to the review
  "$REPO/game-mod-sync" -p MyProfile --download-world >"$SANDBOX/out" 2>&1 || { cat "$SANDBOX/out"; fail "download failed"; }
  assert_contains "$(cat "$SANDBOX/out")" "1) Midgard" "menu"
  assert_contains "$(cat "$SANDBOX/out")" "the server runs this one" "menu"
  assert_file "$LW/Ashlands/_main.55.ok"
}

t_an_unknown_world_is_refused() {
  need_rsync
  setup_world_download
  local out
  out=$(download --world Gondor) && fail "should fail"
  assert_contains "$out" "no world named 'Gondor'" "output"
}

t_your_copy_goes_to_a_backup_first() {
  need_rsync
  setup_world_download
  valheim_world "$LW" Midgard 2 >/dev/null
  valheim_world "$LW" WildThings 9 >/dev/null
  download >/dev/null
  assert_file "$LW/Midgard/_main.1940.ok"
  assert_no_file "$LW/Midgard/_main.2.ok"      # not a mix of both
  assert_eq 1 "$(find "${LW%/*}/game-mod-sync-backups" -name _main.2.ok | wc -l | tr -d ' ')" "backed-up save 2"
  assert_file "$LW/WildThings/_main.9.ok"          # other worlds stay
}

t_the_server_is_not_changed() {
  need_rsync
  setup_world_download
  local before
  before=$(cd "$REMOTE" && find . -type f -exec cksum {} + | sort)
  download >/dev/null
  assert_eq "$before" "$(cd "$REMOTE" && find . -type f -exec cksum {} + | sort)" "server files"
}

t_copies_again_when_the_server_saves_during_the_copy() {
  need_rsync
  setup_world_download
  local real
  real=$(PATH=${PATH#*:} command -v rsync)
  # The first download is followed by a save on the server (save 1941).
  cat >"$SANDBOX/bin/rsync" <<SH
#!/usr/bin/env bash
"$real" "\$@"; rc=\$?
case " \$* " in *" amp@server:"*)
  if [ ! -e "$SANDBOX/saved" ]; then
    touch "$SANDBOX/saved"
    . "$REPO/tests/lib.sh"; valheim_world "$SW" Midgard 1941 >/dev/null
  fi ;;
esac
exit \$rc
SH
  chmod +x "$SANDBOX/bin/rsync"
  download >"$SANDBOX/out" || { cat "$SANDBOX/out"; fail "download failed"; }
  assert_contains "$(cat "$SANDBOX/out")" "copying again" "output"
  assert_file "$LW/Midgard/_main.1941.ok"
  assert_no_file "$LW/Midgard/_main.1940.ok"
}

t_dry_run_downloads_nothing() {
  need_rsync
  setup_world_download
  assert_contains "$(download -n)" "Dry run" "output"
  assert_no_file "$LW/Midgard"
}

t_a_plain_ssh_server_uses_the_games_usual_save_folder() {
  need_rsync
  setup_world_download
  local root="$REMOTE/valheim-server"
  mkdir -p "$root/BepInEx/plugins"
  valheim_world "$REMOTE/.config/unity3d/IronGate/Valheim/worlds_local" Solo 4 >/dev/null
  printf 'gale\tvalheim\tMyProfile\tssh\tme@server\t%s\t\n' "$root" >"$GMS_CONFIG_DIR/targets.tsv"
  download >"$SANDBOX/out" || { cat "$SANDBOX/out"; fail "download failed"; }
  assert_file "$LW/Solo/_main.4.ok"
}

t_a_typed_in_worlds_folder_is_remembered() {
  need_rsync
  setup_world_download
  local root="$REMOTE/srv"
  mkdir -p "$root/BepInEx/plugins"
  valheim_world "$REMOTE/elsewhere/worlds" Solo 4 >/dev/null
  printf 'gale\tvalheim\tMyProfile\tssh\tme@server\t%s\t\n' "$root" >"$GMS_CONFIG_DIR/targets.tsv"
  answers "elsewhere/worlds"
  download >"$SANDBOX/out" || { cat "$SANDBOX/out"; fail "download failed"; }
  assert_file "$LW/Solo/_main.4.ok"
  assert_eq "$REMOTE/elsewhere/worlds" "$(cut -f8 "$GMS_CONFIG_DIR/targets.tsv")" "saved worlds folder"
  rm -rf "$LW/Solo"
  download >/dev/null                                # no answer left: must not ask
  assert_file "$LW/Solo/_main.4.ok"
}

t_asks_which_local_folder_when_there_are_several() {
  need_rsync
  setup_world_download
  local other="$LOCAL/mnt/d/Users/me/AppData/LocalLow/IronGate/Valheim/worlds_local"
  mkdir -p "$other"
  answers 2
  download >/dev/null
  assert_file "$other/Midgard/_main.1940.ok"
  assert_contains "$(cat "$GMS_CONFIG_DIR/config")" "LOCAL_WORLDS=valheim=" "config"
}

t_a_game_without_a_world_format_says_so() {
  need_rsync
  gale_profile lethal-company Fun >/dev/null
  touch "$(GALE)/lethal-company/profiles/Fun/BepInEx/plugins/x.dll"
  mkdir -p "$GMS_CONFIG_DIR"
  printf 'gale\tlethal-company\tFun\tssh\tme@server\t/srv\t\n' >"$GMS_CONFIG_DIR/targets.tsv"
  assert_contains "$("$REPO/game-mod-sync" -p Fun --download-world -y 2>&1)" "has no world download" "output"
}
