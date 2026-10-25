## Summary

<!-- One or two sentences: what this changes and why. -->

## Changes

-

## Testing

- [ ] `go test ./...` passes and `gofmt -l .` / `go vet ./...` are clean
- [ ] Badges regenerated if tests or `VERSION` changed (`go test -json ./... > results.json && python3 docs/badges/make.py results.json docs/badges`)
- [ ] Screenshots regenerated if anything visible in either interface changed (`docs/screenshots/make.sh`; CI's `screenshots` artifact is the reference)
- [ ] Tried by hand (say how: `--cli` / `--tui`, OS, server type, game):

## Release

- [ ] No release
- [ ] Release: `VERSION` bumped to `X.Y.Z` (merging publishes `vX.Y.Z`)

## Checklist

- [ ] README / `docs/DESIGN.md` / `plugins/README.md` updated if behaviour changed
- [ ] No personal or environment details anywhere (hostnames, IPs, user names, server or profile names, URLs of private services) - in code, docs, commit messages or this description
