# shellcheck shell=bash
# Test helpers, sourced by tests/run.sh for every test (each runs in its own
# subshell and sandbox, so tests can't affect each other). Tests run with
# errexit: the first failing assert (or command) fails the test. Exit 77 = skip.
#
#   SANDBOX   empty folder for this test
#   LOCAL     $SANDBOX/local  - this computer (HOME, Gale data, config)
#   REMOTE    $SANDBOX/remote - the "server": the fake ssh runs commands here
#             with HOME=$REMOTE, so rsync over ssh copies into it
#   load_app  sources lib/core.sh (text UI) like the launcher does
#   answers LINE...   what the text UI reads as typed input, in order
#   assert_eq / assert_contains / assert_not_contains / assert_file / assert_no_file

REPO=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

sandbox_init() {
  SANDBOX=$(mktemp -d "${TMPDIR:-/tmp}/gms-test.XXXXXX")
  LOCAL=$SANDBOX/local REMOTE=$SANDBOX/remote
  mkdir -p "$LOCAL/home" "$REMOTE" "$SANDBOX/bin"
  # ssh stand-in: drop the options and the host, run the command "on the server"
  cat >"$SANDBOX/bin/ssh" <<SH
#!/usr/bin/env bash
while [ \$# -gt 0 ]; do case \$1 in -o|-l|-p|-i) shift 2 ;; -*) shift ;; *) break ;; esac; done
shift
cd "$REMOTE" && HOME="$REMOTE" exec bash -c "\$*"
SH
  chmod +x "$SANDBOX/bin/ssh"
  # no network in tests: Thunderstore lookups find nothing
  printf '#!/bin/sh\nexit 22\n' >"$SANDBOX/bin/curl"; chmod +x "$SANDBOX/bin/curl"
  export PATH="$SANDBOX/bin:$PATH" HOME="$LOCAL/home" XDG_DATA_HOME="$LOCAL/home/.local/share"
  export XDG_CACHE_HOME="$LOCAL/cache" GMS_CONFIG_DIR="$LOCAL/config" GMS_NO_SSH_MUX=1
  export GMS_WIN_USERS="$LOCAL/mnt/*/Users/*"   # Windows user folders, as WSL sees them
  unset GALE_DATA_DIR APPDATA
  : >"$SANDBOX/answers"
  export GMS_TTY="$SANDBOX/answers"   # the text UI reads typed answers from here
}
sandbox_done() { [ -n "${SANDBOX:-}" ] && rm -rf -- "$SANDBOX"; }

load_app() {
  APP_DIR=$REPO GMS_UI=text PROG=game-mod-sync
  # shellcheck source=/dev/null
  . "$REPO/lib/core.sh"
}

answers() { printf '%s\n' "$@" >>"$SANDBOX/answers"; }

# ---- fixtures -----------------------------------------------------------------
GALE() { printf '%s' "$XDG_DATA_HOME/com.kesomannen.gale"; }

# gale_profile GAME PROFILE -> a BepInEx profile folder (prints its path)
gale_profile() {
  local p
  p="$(GALE)/$1/profiles/$2"
  mkdir -p "$p/BepInEx/plugins" "$p/BepInEx/config" "$p/BepInEx/core"
  printf '%s' "$p"
}

# fake_plugin_dll FILE ID NAME VERSION -> a file holding a [BepInPlugin] blob,
# the way the compiler stores it: 01 00, then three length-prefixed strings
fake_plugin_dll() {
  mkdir -p "$(dirname "$1")"
  { printf 'MZ\x90\x00junk'
    printf '\x01\x00'
    printf "\\x$(printf %02x ${#2})%s" "$2"
    printf "\\x$(printf %02x ${#3})%s" "$3"
    printf "\\x$(printf %02x ${#4})%s" "$4"
    printf '\x00\x00more junk'
  } >"$1"
}

# amp_instance NAME -> a server game folder the way AMP lays it out (prints it)
amp_instance() {
  local r="$REMOTE/.ampdata/instances/$1/Valheim/896660"
  mkdir -p "$r/BepInEx/plugins" "$r/BepInEx/config" "$r/BepInEx/core"
  printf '%s' "$r"
}

# valheim_world FOLDER WORLD SAVE -> a chunked Valheim world (save number SAVE)
# in the worlds folder FOLDER (prints the world's folder)
valheim_world() {
  local w="$1/$2"
  mkdir -p "$w"
  rm -f "$w"/_main.*
  echo "fwl $3" >"$w/_main.$3.fwl2"; echo "db $3" >"$w/_main.$3.db2"; echo "index $3" >"$w/_main.$3.chunks"
  echo "terrain" >"$w/00_00__0_1.chunk"; echo "base $3" >"$w/10_1a__1_$3.chunk"
  echo ok >"$w/_main.$3.ok"
  printf '%s' "$w"
}

# ---- asserts --------------------------------------------------------------------
fail() { printf '    %s\n' "$@" >&2; return 1; }
assert_eq() { # EXPECTED ACTUAL [WHAT]
  [ "$1" = "$2" ] || fail "${3:-value}: expected [$1]" "${3:-value}: got      [$2]"
}
assert_contains() { # HAYSTACK NEEDLE [WHAT]
  case $1 in *"$2"*) ;; *) fail "${3:-output} should contain [$2]" "got: [$1]" ;; esac
}
assert_not_contains() { # HAYSTACK NEEDLE [WHAT]
  case $1 in *"$2"*) fail "${3:-output} should not contain [$2]" "got: [$1]" ;; *) ;; esac
}
assert_file()    { [ -e "$1" ] || fail "missing: ${1#"$SANDBOX"/}"; }
assert_no_file() { [ ! -e "$1" ] || fail "should not exist: ${1#"$SANDBOX"/}"; }
