#!/usr/bin/env python3
"""Draws the README's status badges from the Go tests' results.

    go test -json ./... > results.json
    python3 docs/badges/make.py results.json docs/badges

Writes panel.svg (what the README shows: every badge in one box) and the
single badges it's made of: version.svg, tests.svg, and one badge per group
of packages (group-*.svg), each part showing its passed tests (or how many
failed). CI makes them again and checks they match the committed ones.
"""
import collections
import json
import os
import re
import sys

MODULE = "github.com/doggpound/game-mod-sync/"

# The panel's rows: (row label, [(badge file, badge label, [(part label, package)])]).
GROUPS = [
    ("Core", [
        ("group-core.svg", "core", [
            ("copy", "internal/transfer"), ("SSH", "internal/remote"), ("program", "internal/app"),
            ("configs", "internal/configs"), ("settings", "internal/state"), ("plugins", "internal/plugins"),
            ("updates", "internal/update"),
        ]),
        ("group-interface.svg", "interface", [("full screen", "internal/ui/tui")]),
    ]),
    ("Plugins", [
        ("group-sources.svg", "source", [("Gale", "internal/plugins/sources/gale")]),
        ("group-loaders.svg", "loader", [("BepInEx", "internal/plugins/loaders/bepinex")]),
        ("group-backends.svg", "server", [("AMP", "internal/plugins/servers/amp")]),
        ("group-games.svg", "games", [("files", "internal/plugins/games")]),
        ("group-worlds.svg", "worlds", [("Valheim", "internal/plugins/worlds/valheim"), ("files", "internal/plugins/worlds/files")]),
    ]),
]
GREEN, RED, PURPLE = "#4c1", "#e05d44", "#8e2de2"


def width(text):
    # ~6.5 px per character at 11px Verdana: close enough for short labels
    return len(text) * 65 // 10 + 12


def badge(path, label, value, colour):
    lw, vw = width(label), width(value)
    w = lw + vw
    with open(path, "w") as f:
        f.write(f'''<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="20" role="img" aria-label="{label}: {value}">
  <title>{label}: {value}</title>
  <linearGradient id="s" x2="0" y2="100%"><stop offset="0" stop-color="#bbb" stop-opacity=".1"/><stop offset="1" stop-opacity=".1"/></linearGradient>
  <clipPath id="r"><rect width="{w}" height="20" rx="3" fill="#fff"/></clipPath>
  <g clip-path="url(#r)"><rect width="{lw}" height="20" fill="#555"/><rect x="{lw}" width="{vw}" height="20" fill="{colour}"/><rect width="{w}" height="20" fill="url(#s)"/></g>
  <g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" font-size="11">
    <text x="{lw // 2}" y="15" fill="#010101" fill-opacity=".3">{label}</text><text x="{lw // 2}" y="14">{label}</text>
    <text x="{lw + vw // 2}" y="15" fill="#010101" fill-opacity=".3">{value}</text><text x="{lw + vw // 2}" y="14">{value}</text>
  </g>
</svg>
''')


def group_badge(path, label, segs):
    lw = width(label)
    x, rects, title = lw, "", label
    texts = (f'<text x="{lw // 2}" y="15" fill="#010101" fill-opacity=".3">{label}</text>'
             f'<text x="{lw // 2}" y="14">{label}</text>')
    for text, colour in segs:
        w = width(text)
        rects += f'<rect x="{x}" width="{w}" height="20" fill="{colour}"/>'
        if x != lw:
            rects += f'<rect x="{x}" width="1" height="20" fill="#fff" fill-opacity=".35"/>'
        texts += (f'<text x="{x + w // 2}" y="15" fill="#010101" fill-opacity=".3">{text}</text>'
                  f'<text x="{x + w // 2}" y="14">{text}</text>')
        title += ", " + text
        x += w
    with open(path, "w") as f:
        f.write(f'''<svg xmlns="http://www.w3.org/2000/svg" width="{x}" height="20" role="img" aria-label="{title}">
  <title>{title}</title>
  <linearGradient id="s" x2="0" y2="100%"><stop offset="0" stop-color="#bbb" stop-opacity=".1"/><stop offset="1" stop-opacity=".1"/></linearGradient>
  <clipPath id="r"><rect width="{x}" height="20" rx="3" fill="#fff"/></clipPath>
  <g clip-path="url(#r)"><rect width="{lw}" height="20" fill="#555"/>{rects}<rect width="{x}" height="20" fill="url(#s)"/></g>
  <g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" font-size="11">{texts}</g>
</svg>
''')


def panel(path, rows):
    """One image: a rounded light-grey box, a label before each row, and the
    badges (the files above, nested, their ids made unique). A README can't
    style HTML (no CSS on GitHub or Forgejo), hence an image."""
    d = os.path.dirname(path)
    pad, lw, gap, rh, rg = 14, 74, 6, 20, 10
    y, i, maxw, body, labels = pad, 0, 0, "", ""
    for label, files in rows:
        rw = 0
        labels += f'<text x="{pad + lw - 14}" y="{y + 14}" text-anchor="end">{label}</text>'
        for b in files:
            i += 1
            svg = open(os.path.join(d, b)).read().strip().split("\n")
            w = int(re.search(r'width="(\d+)"', svg[0]).group(1))
            inner = "\n".join(l for l in svg[1:-1] if "<title>" not in l)
            inner = (inner.replace('id="s"', f'id="s{i}"').replace("url(#s)", f"url(#s{i})")
                     .replace('id="r"', f'id="r{i}"').replace("url(#r)", f"url(#r{i})"))
            body += f'<svg x="{pad + lw + rw}" y="{y}" width="{w}" height="20">{inner}</svg>'
            rw += w + gap
        maxw = max(maxw, rw - gap)
        y += rh + rg
    W, H = pad + lw + maxw + pad, y - rg + pad
    with open(path, "w") as f:
        f.write(f'''<svg xmlns="http://www.w3.org/2000/svg" width="{W}" height="{H}" role="img" aria-label="Status and test badges">
  <title>game-mod-sync: status and tests</title>
  <rect x="0.5" y="0.5" width="{W - 1}" height="{H - 1}" rx="12" fill="#f6f8fa" stroke="#d0d7de"/>
  <line x1="{pad + lw - 6}" y1="{pad - 4}" x2="{pad + lw - 6}" y2="{H - pad + 4}" stroke="#d0d7de"/>
  <g fill="#57606a" font-family="-apple-system,Segoe UI,Helvetica,Arial,DejaVu Sans,sans-serif" font-size="12" font-weight="600">{labels}</g>
  {body}
</svg>
''')


def main():
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    results, out = sys.argv[1], sys.argv[2]
    passed, failed = collections.Counter(), collections.Counter()
    for line in open(results):
        try:
            e = json.loads(line)
        except ValueError:
            continue
        test = e.get("Test", "")
        if not test or "/" in test:  # top-level tests only
            continue
        pkg = e.get("Package", "").removeprefix(MODULE)
        if e.get("Action") == "pass":
            passed[pkg] += 1
        elif e.get("Action") == "fail":
            failed[pkg] += 1
    os.makedirs(out, exist_ok=True)
    repo = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
    version = open(os.path.join(repo, "VERSION")).read().strip()
    badge(os.path.join(out, "version.svg"), "version", "v" + version, PURPLE)
    total, bad = sum(passed.values()) + sum(failed.values()), sum(failed.values())
    if bad:
        badge(os.path.join(out, "tests.svg"), "tests", f"{bad} of {total} failed", RED)
    else:
        badge(os.path.join(out, "tests.svg"), "tests", f"{total} passed", GREEN)
    rows = [("Status", ["version.svg", "tests.svg"])]
    for row, groups in GROUPS:
        files = []
        for file, label, parts in groups:
            segs = []
            for name, pkg in parts:
                if failed[pkg]:
                    segs.append((f"{name} {failed[pkg]} failed", RED))
                else:
                    segs.append((f"{name} {passed[pkg]}", GREEN))
            group_badge(os.path.join(out, file), label, segs)
            files.append(file)
        rows.append((row, files))
    panel(os.path.join(out, "panel.svg"), rows)
    print(f"badges written to {out}: {total} tests, {bad} failed")


if __name__ == "__main__":
    main()
