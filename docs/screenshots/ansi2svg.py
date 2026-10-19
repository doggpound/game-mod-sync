#!/usr/bin/env python3
"""Turn a terminal screen captured by `tmux capture-pane -e -p` into an SVG
picture of a terminal window. Used by docs/screenshots/make.sh.

    ansi2svg.py [--cols N] [--rows N] [--fill COLOUR] TITLE < capture.txt > screen.svg

--cols / --rows give the screen's size, so the picture doesn't depend on how
much of the blank right and bottom edges tmux happens to include (3.4 pads
lines, 3.3 doesn't). --fill is the background of cells with no colour of
their own: tmux 3.3 drops the colour of cells a program cleared, e.g.
whiptail's backdrop, so the TUI shots pass that colour (a 0-255 number).

Colours: the 16 base colours of Windows Terminal's default theme (Campbell),
the xterm 256-colour cube and grey ramp, and 24-bit colours. Each run of text
is drawn with a fixed width (textLength), so columns line up whatever
monospace font the viewer has.
"""
import html
import re
import sys

BASE16 = ["#0c0c0c", "#c50f1f", "#13a10e", "#c19c00", "#0037da", "#881798", "#3a96dd", "#cccccc",
          "#767676", "#e74856", "#16c60c", "#f9f1a5", "#3b78ff", "#b4009e", "#61d6d6", "#f2f2f2"]
FG, BG = "#cccccc", "#0c0c0c"
CW, CH, FS = 8.4, 17, 14            # cell width, cell height, font size (px)
PAD, BAR = 14, 30                   # padding around the screen, title bar height


def colour256(n):
    if n < 16:
        return BASE16[n]
    if n < 232:
        n -= 16
        steps = [0, 95, 135, 175, 215, 255]
        return "#%02x%02x%02x" % (steps[n // 36], steps[(n // 6) % 6], steps[n % 6])
    g = 8 + (n - 232) * 10
    return "#%02x%02x%02x" % (g, g, g)


def parse(lines):
    """-> rows of cells (char, fg, bg, bold)"""
    rows = []
    fg = bg = None
    bold = rev = False
    for line in lines:
        row = []
        for m in re.finditer(r"\x1b\[([0-9;]*)m|([^\x1b])", line):
            if m.group(2) is not None:
                f, b = (bg, fg) if rev else (fg, bg)
                row.append((m.group(2), f, b, bold))
                continue
            codes = [int(c) if c else 0 for c in m.group(1).split(";")]
            i = 0
            while i < len(codes):
                c = codes[i]
                if c == 0:
                    fg = bg = None; bold = rev = False
                elif c == 1:
                    bold = True
                elif c == 22:
                    bold = False
                elif c == 7:
                    rev = True
                elif c == 27:
                    rev = False
                elif 30 <= c <= 37:
                    fg = BASE16[c - 30]
                elif 90 <= c <= 97:
                    fg = BASE16[c - 90 + 8]
                elif 40 <= c <= 47:
                    bg = BASE16[c - 40]
                elif 100 <= c <= 107:
                    bg = BASE16[c - 100 + 8]
                elif c == 39:
                    fg = None
                elif c == 49:
                    bg = None
                elif c in (38, 48) and i + 1 < len(codes):
                    if codes[i + 1] == 5 and i + 2 < len(codes):
                        col = colour256(codes[i + 2]); i += 2
                    elif codes[i + 1] == 2 and i + 4 < len(codes):
                        col = "#%02x%02x%02x" % tuple(codes[i + 2:i + 5]); i += 4
                    else:
                        col = None
                    if c == 38:
                        fg = col
                    else:
                        bg = col
                i += 1
        rows.append(row)
    return rows


def main():
    args, opts = sys.argv[1:], {}
    while len(args) > 1 and args[0] in ("--cols", "--rows", "--fill"):
        opts[args[0][2:]] = int(args[1]); args = args[2:]
    title = args[0] if args else "game-mod-sync"
    lines = sys.stdin.read().split("\n")
    if "rows" in opts:
        lines = (lines + [""] * opts["rows"])[:opts["rows"]]
    else:
        while lines and not lines[-1].strip():
            lines.pop()
    rows = parse(lines)
    fill = colour256(opts["fill"]) if "fill" in opts else None
    cols = opts.get("cols") or max([len(r) for r in rows] + [80])
    rows = [[(c, f, b or fill, bo) for c, f, b, bo in r[:cols]] + [(" ", None, fill, False)] * (cols - len(r)) for r in rows]
    w = int(PAD * 2 + cols * CW)
    h = int(BAR + PAD * 2 + len(rows) * CH)
    out = [f'<svg xmlns="http://www.w3.org/2000/svg" width="{w}" height="{h}" viewBox="0 0 {w} {h}" role="img" aria-label="{html.escape(title)}">',
           f'<title>{html.escape(title)}</title>',
           f'<rect width="{w}" height="{h}" rx="10" fill="{BG}"/>',
           f'<rect width="{w}" height="{BAR}" rx="10" fill="#2b2b2b"/><rect y="{BAR - 10}" width="{w}" height="10" fill="#2b2b2b"/>',
           '<circle cx="18" cy="15" r="6" fill="#ff5f57"/><circle cx="38" cy="15" r="6" fill="#febc2e"/><circle cx="58" cy="15" r="6" fill="#28c840"/>',
           f'<text x="{w / 2:.0f}" y="20" fill="#a0a0a0" font-family="Segoe UI,Helvetica,Arial,sans-serif" font-size="13" text-anchor="middle">{html.escape(title)}</text>',
           f'<g font-family="Cascadia Mono,Consolas,DejaVu Sans Mono,Menlo,monospace" font-size="{FS}">']
    for y, row in enumerate(rows):
        top = BAR + PAD + y * CH
        # backgrounds, merged per run of the same colour
        x = 0
        while x < len(row):
            b = row[x][2]
            e = x
            while e < len(row) and row[e][2] == b:
                e += 1
            if b and b != BG:
                out.append(f'<rect x="{PAD + x * CW:.1f}" y="{top}" width="{(e - x) * CW + 0.6:.1f}" height="{CH + 0.6}" fill="{b}" shape-rendering="crispEdges"/>')
            x = e
        # text, per run of the same colour and weight
        x = 0
        while x < len(row):
            f, bo = row[x][1], row[x][3]
            e = x
            while e < len(row) and row[e][1] == f and row[e][3] == bo:
                e += 1
            text = "".join(c[0] for c in row[x:e])
            if text.strip():
                lead = len(text) - len(text.lstrip(" "))
                body = text.strip(" ")
                tx = PAD + (x + lead) * CW
                attrs = f' font-weight="bold"' if bo else ""
                out.append(f'<text x="{tx:.1f}" y="{top + CH - 4}" fill="{f or FG}"{attrs} textLength="{len(body) * CW:.1f}" '
                           f'lengthAdjust="spacingAndGlyphs" xml:space="preserve">{html.escape(body)}</text>')
            x = e
    out.append("</g></svg>")
    print("\n".join(out))


if __name__ == "__main__":
    main()
