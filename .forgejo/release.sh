#!/usr/bin/env bash
# Builds a version's downloads and publishes them as a Forgejo release
# (creating the tag when it doesn't exist), and, when GH_TOKEN and GH_REPO are
# set, as a release on the GitHub mirror too. Safe to run again: an existing
# release gets its notes and files replaced.
#
# Versions with go.mod (1.0.0 and later): the program for every platform,
#   game-mod-sync-X.Y.Z-<os>-<arch>.zip (Windows) or .tar.gz, and SHA256SUMS.
# Older versions (the shell scripts): game-mod-sync-X.Y.Z.tar.gz of the source.
# Runs in .forgejo/workflows/release.yml; needs git, curl, node, python3 and
# (for Go versions) go on the PATH.
set -euo pipefail

api="${GITHUB_SERVER_URL%/}/api/v1/repos/$GITHUB_REPOSITORY"
web="${GITHUB_SERVER_URL%/}/$GITHUB_REPOSITORY"
auth="Authorization: token $TOKEN"
git fetch -q --tags --force origin

# The version at a commit: the VERSION file; for 0.x, VERSION="..." in
# lib/core.sh, or in any file for the oldest versions
version_at() {
  if git cat-file -e "$1:VERSION" 2>/dev/null; then
    git show "$1:VERSION" | tr -d ' \r' | sed -n 1p
    return
  fi
  { git show "$1:lib/core.sh" 2>/dev/null || git grep -h '^VERSION=' "$1" -- 2>/dev/null || true; } |
    sed -n 's/^VERSION="\(.*\)"/\1/p' | sed -n 1p
}

# Which tag, and which commit
if [ -n "${INPUT_TAG:-}" ]; then
  tag=$INPUT_TAG
elif [ "${GITHUB_REF_TYPE:-}" = tag ]; then
  tag=$GITHUB_REF_NAME
else
  v=$(version_at HEAD); [ -n "$v" ] || { echo "no VERSION in lib/core.sh"; exit 1; }
  tag=v$v
fi
[[ $tag =~ ^v[0-9]+(\.[0-9]+)*$ ]] || { echo "not a version tag: $tag"; exit 1; }
if [ -z "${INPUT_TAG:-}" ] && [ "${GITHUB_REF_TYPE:-}" != tag ] && git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
  echo "$tag is already released - nothing to do"; exit 0
fi
if git rev-parse -q --verify "refs/tags/$tag" >/dev/null; then
  sha=$(git rev-list -n 1 "$tag")
elif [ -z "${INPUT_TAG:-}" ]; then
  sha=$(git rev-parse HEAD)            # new version on main: tag this commit
else
  echo "tag $tag doesn't exist"; exit 1
fi
ver=${tag#v}
echo "Releasing $tag at ${sha:0:7}"

# The tagged commit should carry that version (the first ones were tagged by hand)
v=$(version_at "$sha")
[ "$v" = "$ver" ] || echo "note: lib/core.sh at ${sha:0:7} says '${v:-none}'"

# Previous version: the highest tag below this one
prev=$(git tag -l 'v*' | grep -vx "$tag" | { cat; echo "$tag"; } | sort -V | grep -B1 -x "$tag" | grep -vx "$tag" || true)
if [ -n "$prev" ]; then range="$prev..$sha"; else range=$sha; fi
# (merge commits and an empty "Initial commit" are left out)
commits=()
while read -r c; do
  [ "$(git log -1 --format=%s "$c")" = "Initial commit" ] && [ -z "$(git log -1 --format=%b "$c")" ] && continue
  commits+=("$c")
done < <(git rev-list --reverse --no-merges "$range")
[ ${#commits[@]} -gt 0 ] || commits=("$sha")
# The first release only describes its own commit, not the whole history.
[ -n "$prev" ] || commits=("$(git rev-list -1 --no-merges "$sha")")

# The release is named just the tag (vX.Y.Z, same on Forgejo and GitHub). Its
# one-line summary, at the top of the notes: the commit that names this version,
# else the last one that set VERSION, else the oldest in the range.
title=$tag summary=""
for c in "${commits[@]}"; do
  s=$(git log -1 --format=%s "$c")
  case $s in "$tag:"*|"$tag "*) summary=${s#"$tag"}; summary=${summary#:}; summary=${summary# } ;; esac
done
if [ -z "$summary" ]; then
  c=$(git log -1 --format=%H "$range" -- VERSION || true)
  [ -n "$c" ] || c=$(git log -1 --format=%H -G'^VERSION=' "$range" -- || true)
  summary=$(git log -1 --format=%s "${c:-${commits[0]}}")
fi

strip() { grep -Ev '^(Co-Authored-By|Claude-Session|Signed-off-by):' | sed -e :a -e '/^\n*$/{$d;N;ba' -e '}'; }
notes=$(
  printf '**%s**\n\n' "$summary"
  if [ ${#commits[@]} -eq 1 ]; then
    git log -1 --format=%b "${commits[0]}" | strip
  else
    for c in "${commits[@]}"; do
      printf '### %s\n\n' "$(git log -1 --format=%s "$c")"
      git log -1 --format=%b "$c" | strip
      printf '\n\n'
    done
  fi
  printf '\n---\n'
  printf 'Committed %s. ' "$(git log -1 --format=%ad --date=short "$sha")"
  # @@WEB@@ becomes the repo's address on each site
  if [ -n "$prev" ]; then printf 'Changes since %s: @@WEB@@/compare/%s...%s\n\n' "$prev" "$prev" "$tag"; else printf '\n\n'; fi
  if git cat-file -e "$sha:go.mod" 2>/dev/null; then
    printf '**Download** the file for your computer, extract it and run `game-mod-sync` (nothing else to install):\n\n'
    printf -- '- Windows: `game-mod-sync-%s-windows-amd64.zip` (`-arm64` for ARM PCs); run `game-mod-sync.exe` in Windows Terminal\n' "$ver"
    printf -- '- macOS: `game-mod-sync-%s-macos-arm64.tar.gz` (Apple silicon) or `-macos-amd64` (Intel)\n' "$ver"
    printf -- '- Linux: `game-mod-sync-%s-linux-amd64.tar.gz` (or `-linux-arm64`)\n\n' "$ver"
    printf '`SHA256SUMS` has the files'"'"' checksums. See the README for setup.\n'
  else
    printf '**Install:** download `game-mod-sync-%s.tar.gz`, extract it and run `./game-mod-sync` (or `./game-mod-sync --tui`). See the README for setup.\n' "$ver"
  fi
)
notes=$(printf '%s\n' "$notes" | cat -s)    # no runs of blank lines

# Files
out=$(mktemp -d)
name="game-mod-sync-$ver"
files=()
if git cat-file -e "$sha:go.mod" 2>/dev/null; then
  # The program for every platform, from that commit's source.
  src="$out/src"
  git worktree add -q --detach "$src" "$sha"
  for p in windows/amd64 windows/arm64 linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
    os=${p%/*} arch=${p#*/} label=${p%/*}
    [ "$os" = darwin ] && label=macos
    dir="$out/$name-$label-$arch"
    mkdir -p "$dir"
    exe=game-mod-sync
    [ "$os" = windows ] && exe=game-mod-sync.exe
    (cd "$src" && CGO_ENABLED=0 GOOS=$os GOARCH=$arch GOTOOLCHAIN=local \
      go build -trimpath -ldflags "-s -w -X main.version=$ver" -o "$dir/$exe" .)
    cp "$src/README.md" "$dir/"
    if [ "$os" = windows ]; then
      (cd "$out" && python3 -m zipfile -c "$name-$label-$arch.zip" "$name-$label-$arch/")
      files+=("$name-$label-$arch.zip")
    else
      tar -czf "$out/$name-$label-$arch.tar.gz" -C "$out" "$name-$label-$arch"
      files+=("$name-$label-$arch.tar.gz")
    fi
    echo "  built $label/$arch"
  done
  git worktree remove --force "$src"
  (cd "$out" && sha256sum "${files[@]}" >SHA256SUMS)
  files+=(SHA256SUMS)
else
  git archive --format=tar.gz --prefix="$name/" -o "$out/$name.tar.gz" "$sha"
  (cd "$out" && sha256sum "$name.tar.gz" >"$name.tar.gz.sha256")
  files=("$name.tar.gz" "$name.tar.gz.sha256")
fi

json() { node -e 'const [t,n,b,s]=process.argv.slice(1);process.stdout.write(JSON.stringify({tag_name:t,name:n,body:b,target_commitish:s,draft:false,prerelease:false}))' "$@"; }
field() { node -e 'let d="";process.stdin.on("data",c=>d+=c).on("end",()=>{const j=JSON.parse(d||"{}");const v=process.argv[1].split(".").reduce((o,k)=>o==null?o:o[k],j);if(Array.isArray(v))console.log(v.map(a=>a.id+" "+a.name).join("\n"));else if(v!=null)console.log(v)})' "$1"; }

body=$(json "$tag" "$title" "${notes//@@WEB@@/$web}" "$sha")
code=$(curl -sk -o "$out/r.json" -w '%{http_code}' -H "$auth" "$api/releases/tags/$tag")
if [ "$code" = 200 ]; then
  id=$(field id <"$out/r.json")
  echo "Updating release $id"
  curl -skf -X PATCH -H "$auth" -H 'Content-Type: application/json' -d "$body" "$api/releases/$id" >/dev/null
  field assets <"$out/r.json" | while read -r aid aname; do
    [ -n "$aid" ] && curl -skf -X DELETE -H "$auth" "$api/releases/$id/assets/$aid" >/dev/null && echo "  removed old $aname"
  done
else
  echo "Creating release"
  id=$(curl -skf -X POST -H "$auth" -H 'Content-Type: application/json' -d "$body" "$api/releases" | field id)
fi
[ -n "$id" ] || { echo "no release id"; exit 1; }
for f in "${files[@]}"; do
  curl -skf -X POST -H "$auth" -F "attachment=@$out/$f" "$api/releases/$id/assets?name=$f" >/dev/null
  echo "  uploaded $f"
done
echo "Done: $web/releases/tag/$tag"

# ---- GitHub mirror ----------------------------------------------------------
# The push mirror carries branches and tags, not releases. The same release is
# made there with the GitHub API (token: Contents read/write on GH_REPO).
[ -n "${GH_TOKEN:-}" ] && [ -n "${GH_REPO:-}" ] || exit 0
gapi="https://api.github.com/repos/$GH_REPO"
gweb="https://github.com/$GH_REPO"
gh() { curl -sS -H "Authorization: Bearer $GH_TOKEN" -H "Accept: application/vnd.github+json" -H "X-GitHub-Api-Version: 2022-11-28" "$@"; }
# A new commit reaches GitHub through the mirror shortly after it lands; wait for it.
for i in $(seq 1 30); do
  [ "$(gh -o /dev/null -w '%{http_code}' "$gapi/commits/$sha")" = 200 ] && break
  [ "$i" = 30 ] && { echo "GitHub: commit ${sha:0:7} not mirrored after 5 minutes"; exit 1; }
  sleep 10
done
gbody=$(node -e 'const [t,n,b,s]=process.argv.slice(1);process.stdout.write(JSON.stringify({tag_name:t,name:n,body:b,target_commitish:s,draft:false,prerelease:false,make_latest:"legacy"}))' \
  "$tag" "$title" "${notes//@@WEB@@/$gweb}" "$sha")
code=$(gh -o "$out/g.json" -w '%{http_code}' "$gapi/releases/tags/$tag")
if [ "$code" = 200 ]; then
  gid=$(field id <"$out/g.json")
  echo "GitHub: updating release $gid"
  gh -f -X PATCH -d "$gbody" "$gapi/releases/$gid" >/dev/null
  field assets <"$out/g.json" | while read -r aid aname; do
    [ -n "$aid" ] && gh -f -X DELETE "$gapi/releases/assets/$aid" >/dev/null && echo "  removed old $aname"
  done
else
  echo "GitHub: creating release"
  gid=$(gh -f -X POST -d "$gbody" "$gapi/releases" | field id)
fi
[ -n "$gid" ] || { echo "GitHub: no release id"; exit 1; }
for f in "${files[@]}"; do
  gh -f -X POST -H "Content-Type: application/octet-stream" --data-binary "@$out/$f" \
    "https://uploads.github.com/repos/$GH_REPO/releases/$gid/assets?name=$f" >/dev/null
  echo "  uploaded $f"
done
echo "Done: $gweb/releases/tag/$tag"
