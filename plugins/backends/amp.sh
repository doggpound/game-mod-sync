# shellcheck shell=bash
# Server type: CubeCoders AMP (https://cubecoders.com/AMP)
#
# AMP keeps every instance under the AMP user's home:
#   ~/.ampdata/instances/<Instance>/<Game>/<AppId>/...
# The game root differs per game, so instead of hard-coding it we look for
# the game's marker folder (e.g. BepInEx/plugins) anywhere inside each
# instance and treat its parent as the game root.
#
# A server type provides:
#   <name>_label                  human name for the menu
#   <name>_default_user           default SSH user ("" = ask with no default)
#   <name>_candidates TARGET MARKER
#       print "label<TAB>absolute game root" lines for every place on the
#       server where MARKER exists (relative to a game root)
#   <name>_managed TARGET ROOT SPECS        (optional)
#       SPECS is the game's MANAGED_<name> value. Print "pattern<TAB>reason"
#       for each file the server installs itself, so it can be left alone.

amp_label()        { printf 'CubeCoders AMP'; }
amp_default_user() { printf 'amp'; }

amp_candidates() {
  local target=$1 marker=$2 base=${AMP_INSTANCES_DIR:-.ampdata/instances} p root rel
  ui_wait "Looking for AMP instances with $marker under ~/$base ..." \
    rssh "$target" "find \"\$HOME\"/$(shq "$base") -mindepth 2 -maxdepth 8 -type d \
                    -path $(shq "*/$marker") -not -path '*/.mod-sync-backups/*' 2>/dev/null | sort" || true
  printf '%s\n' "$WAIT_OUT" |
  while IFS= read -r p; do
    [ -n "$p" ] || continue
    root=${p%/"$marker"}
    rel=${root#*/"$base"/}
    printf '%s  %s%s%s\t%s\n' "${rel%%/*}" "$D" "$rel" "$N" "$root"
  done
}

# AMP game templates can install add-ons themselves on every update (e.g. the
# Valheim template's "Install ValheimPlus" option). The instance's settings
# live in <instance>/GenericModule.kvp as App.AppSettings={json}.
# SPECS: "setting=pattern[,pattern] ..." - patterns are relative to a sync folder.
amp_managed() {
  local target=$1 root=$2 specs=$3 base=${AMP_INSTANCES_DIR:-.ampdata/instances}
  local rest inst settings spec setting p
  case $root in */"$base"/*) ;; *) return 0 ;; esac
  rest=${root#*/"$base"/}
  inst="${root%%/"$base"/*}/$base/${rest%%/*}"
  ui_wait "Checking ${rest%%/*}'s AMP settings ..." \
    rssh "$target" "grep -h '^App.AppSettings=' $(shq "$inst")/*.kvp 2>/dev/null" || true
  settings=$WAIT_OUT
  [ -n "$settings" ] || return 0
  for spec in $specs; do
    setting=${spec%%=*}
    case $settings in
      *"\"$setting\":\"true\""*|*"\"$setting\":true"*)
        for p in $(printf '%s' "${spec#*=}" | tr ',' ' '); do
          printf '%s\tAMP setting "%s" is on for %s\n' "$p" "$setting" "${rest%%/*}"
        done ;;
    esac
  done
}
