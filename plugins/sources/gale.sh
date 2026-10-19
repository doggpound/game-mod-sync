# shellcheck shell=bash
# Source: Gale mod manager (https://github.com/Kesomannen/gale)
#
# Gale keeps one folder per game, each with a profiles/ folder:
#   <data dir>/com.kesomannen.gale/<game id>/profiles/<profile>/...
# The game id (e.g. "valheim", "lethal-company") is used to find the
# matching games/<game id>.conf.
#
# A source provides:
#   <name>_label            human name
#   <name>_init             locate data (may prompt once and save config)
#   <name>_list             print "game id<TAB>profile<TAB>profile folder" lines
#   <name>_mod_tags GAME    optional: "Author-Name<TAB>cat|cat" for cached mods

gale_label() { printf 'Gale'; }

GALE_ROOT=""
gale_init() {
  local c cands=()
  if [ -n "${GALE_DATA_DIR:-}" ]; then
    [ -d "$GALE_DATA_DIR" ] || die "GALE_DATA_DIR does not exist: $GALE_DATA_DIR"
    GALE_ROOT=$GALE_DATA_DIR
    return
  fi
  # %APPDATA% on Windows, ~/.local/share on Linux, ~/Library/Application Support on macOS.
  for c in /mnt/*/Users/*/AppData/Roaming/com.kesomannen.gale; do               # WSL
    [ -d "$c" ] && cands+=("$c")
  done
  if [ -n "${APPDATA:-}" ] && command -v cygpath >/dev/null 2>&1; then        # Git Bash / Cygwin
    c="$(cygpath -u "$APPDATA")/com.kesomannen.gale"
    [ -d "$c" ] && cands+=("$c")
  fi
  for c in "${XDG_DATA_HOME:-$HOME/.local/share}/com.kesomannen.gale" \
           "$HOME/.var/app/com.kesomannen.gale/data/com.kesomannen.gale" \
           "$HOME/Library/Application Support/com.kesomannen.gale"; do       # Linux, Flatpak, macOS
    [ -d "$c" ] && cands+=("$c")
  done

  case ${#cands[@]} in
    0) return 1 ;;
    1) GALE_ROOT=${cands[0]} ;;
    *) say "Found more than one Gale data folder:"
       ui_title "Gale data folder"
       ui_choose "Which one is yours?" "${cands[@]}" || exit 0
       GALE_ROOT=${cands[$((CHOICE - 1))]}
       GALE_DATA_DIR=$GALE_ROOT
       save_config
       say "Saved to $CONFIG_FILE" ;;
  esac
}

gale_list() {
  local g p game
  for g in "$GALE_ROOT"/*/profiles; do
    [ -d "$g" ] || continue
    game=${g%/profiles}; game=${game##*/}
    for p in "$g"/*/; do
      [ -d "$p" ] || continue
      p=${p%/}
      printf '%s\t%s\t%s\n' "$game" "${p##*/}" "$p"
    done
  done
}

# Optional: "Author-Name<TAB>category|category" for mods Gale has cached from
# Thunderstore (lets client/server-only checks work offline).
gale_mod_tags() { # GAME
  local f="$GALE_ROOT/$1/thunderstore_cache.json"
  [ -f "$f" ] && command -v perl >/dev/null 2>&1 || return 0
  perl -MJSON::PP -e '
    local $/; open(my $fh, "<", $ARGV[0]) or exit 0; my $j = eval { decode_json(<$fh>) } or exit 0;
    for my $p (ref $j eq "ARRAY" ? @$j : ()) {
      next unless $p->{full_name};
      print $p->{full_name}, "\t", join("|", @{ $p->{categories} || [] }), "\n";
    }' "$f" 2>/dev/null
}
