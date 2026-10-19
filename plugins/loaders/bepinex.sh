# shellcheck shell=bash
# Mod loader: BepInEx (https://github.com/BepInEx/BepInEx)
#
# A loader plugin knows where a mod loader keeps mods and configs, so games
# using the same loader share it. It provides:
#   <name>_label              human name
#   <name>_detect PROFILE_DIR succeeds if the profile uses this loader
#   <name>_defaults           sets the loader's defaults (a game file can override them):
#                               SYNC_DIRS    mod folders, mirrored with --delete
#                               EXCLUDES     never sent or deleted inside SYNC_DIRS
#                               CONFIG_DIRS  config folders, offered per file, never deleted
#                               CONFIG_SKIP  never offered from CONFIG_DIRS (rsync patterns)
#   <name>_config_owners DIR  optional: print "plugin id<TAB>mod folder" for the
#                             mods in DIR (a SYNC_DIRS folder), used to tell which
#                             mod a config file belongs to

bepinex_label() { printf 'BepInEx'; }

bepinex_detect() { [ -d "$1/BepInEx/core" ] || [ -d "$1/BepInEx/plugins" ]; }

bepinex_defaults() {
  SYNC_DIRS="BepInEx/plugins"
  # MMHOOK/ is generated on the server by HookGenPatcher at startup.
  EXCLUDES="MMHOOK/"
  CONFIG_DIRS="BepInEx/config"
  # BepInEx.cfg is the loader's own settings (console, logging), which often
  # differ on a server on purpose; *.dat files are per-player data, not settings.
  CONFIG_SKIP="/BepInEx.cfg *.dat"
}

# Every BepInEx plugin declares [BepInPlugin("id", "Name", "1.2.3")]. In the
# DLL that is a custom-attribute blob: 01 00, then three length-prefixed UTF-8
# strings. Config files are named after that id (e.g. Azumatt.FastLink.cfg).
# Reading the declaration (rather than any mention of the id) avoids matching
# mods that only list another mod as a dependency.
bepinex_config_owners() { # PLUGINS_DIR
  command -v perl >/dev/null 2>&1 || return 0
  (cd "$1" && find . -name '*.dll' -not -path './MMHOOK/*' -print0 2>/dev/null |
    xargs -0 perl -e '
      local $/;
      for my $f (@ARGV) {
        open(my $fh, "<:raw", $f) or next; my $d = <$fh>; close $fh;
        (my $mod = $f) =~ s{^\./}{}; $mod =~ s{/.*}{};
        while ($d =~ /\x01\x00/g) {
          my $p = pos($d); my @s;
          for (1 .. 3) {
            last if $p >= length $d;
            my $n = ord substr($d, $p, 1);
            last if $n == 0 || $n > 0x7f;
            push @s, substr($d, $p + 1, $n); $p += 1 + $n;
          }
          next unless @s == 3;
          my ($id, $name, $v) = @s;
          next unless $v =~ /^\d+(\.\d+){1,3}$/ && $id =~ /^[\x21-\x7e]+$/ && $name =~ /^[\x20-\x7e]+$/;
          print "$id\t$mod\n";
        }
      }' 2>/dev/null) | sort -u
}
