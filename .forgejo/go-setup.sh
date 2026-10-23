#!/usr/bin/env bash
# Puts the Go version CI uses on the PATH of the job's next steps. It's kept in
# the Go cache (see the cache keys in tests.yml and go-cache.yml, which hash this
# file), so it's only downloaded again when this version changes.
set -euo pipefail
version=1.26.10   # the oldest Go the code needs (go.mod)
dir="$HOME/.cache/gms-go/go$version"
if [ ! -x "$dir/bin/go" ]; then
  mkdir -p "$dir"
  curl -fsSL "https://go.dev/dl/go$version.linux-amd64.tar.gz" | tar xz -C "$dir" --strip-components=1
fi
echo "$dir/bin" >>"$GITHUB_PATH"
