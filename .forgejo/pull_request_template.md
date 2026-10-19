## Summary

<!-- One or two sentences: what this changes and why. -->

## Changes

-

## Testing

- [ ] `tests/run.sh` passes
- [ ] `shellcheck -x -S warning game-mod-sync lib/*.sh plugins/*/*.sh tests/*.sh tests/*/*.sh tests/plugins/*/*.sh` is clean
- [ ] Badges regenerated if tests, plugins or `VERSION` changed (`tests/run.sh --badges docs/badges`)
- [ ] Screenshots regenerated if anything visible in either interface changed (`docs/screenshots/make.sh`)
- [ ] Tried by hand (say how: `--cli`, `--tui`, server type, game):

## Release

- [ ] No release
- [ ] Release: `VERSION` in `lib/core.sh` bumped to `X.Y.Z` (merging publishes `vX.Y.Z`)

## Checklist

- [ ] README / `docs/DESIGN.md` / plugin docs updated if behaviour changed
- [ ] No personal or environment details anywhere (hostnames, IPs, user names, server or profile names, URLs of private services) - in code, docs, commit messages or this description
