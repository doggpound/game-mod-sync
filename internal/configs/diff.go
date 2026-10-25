// Package configs compares a profile's config folders with the server's, works
// out which mod each file belongs to and whether that mod is client-only, and
// describes what differs. Config files are offered per file and never deleted
// on the server (see docs/DESIGN.md, "Config files").
package configs

import (
	"fmt"
	"strings"
)

// setting is one key or line of a config file, in the order first seen.
type parsed struct {
	order []string          // keys in file order
	vals  map[string]string // "[Section]\tKey" -> value; "\tline\t<text>" -> count
}

// clean drops line endings, indentation and trailing spaces; ok is false for
// lines that don't count (blank, or comments starting #, ; or //).
func clean(line string) (string, bool) {
	line = strings.TrimRight(line, "\r \t")
	line = strings.TrimLeft(line, " \t")
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//") {
		return "", false
	}
	return line, true
}

// parse reads a config. BepInEx .cfg files (cfg = true) are read per
// [Section] Key = value; anything else per line (counting repeats).
func parse(text string, cfg bool) parsed {
	p := parsed{vals: map[string]string{}}
	sec := ""
	counts := map[string]int{}
	for _, raw := range strings.Split(text, "\n") {
		line, ok := clean(raw)
		if !ok {
			continue
		}
		if cfg && strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			sec = line
			continue
		}
		if cfg {
			if k, v, ok := strings.Cut(line, "="); ok {
				key := sec + "\t" + strings.TrimSpace(k)
				if _, seen := p.vals[key]; !seen {
					p.order = append(p.order, key)
				}
				p.vals[key] = strings.TrimSpace(v)
				continue
			}
		}
		key := "\tline\t" + line
		if counts[key] == 0 {
			p.order = append(p.order, key)
		}
		counts[key]++
		p.vals[key] = fmt.Sprint(counts[key])
	}
	return p
}

func isCfg(path string) bool { return strings.HasSuffix(strings.ToLower(path), ".cfg") }

// Detail is the short description of what differs between yours (local) and
// the server's: up to 4 settings ("Key: server -> yours"), then "+N more
// settings" and "N lines differ". Empty when they only differ in comments,
// blank lines, spacing or line endings.
func Detail(path, local, server string) []string {
	cfg := isCfg(path)
	l, s := parse(local, cfg), parse(server, cfg)
	keys := append(append([]string{}, l.order...), s.order...)
	seen := map[string]bool{}
	var settings []string
	lines, n := 0, 0
	for _, k := range keys {
		if seen[k] {
			continue
		}
		seen[k] = true
		lv, inL := l.vals[k]
		sv, inS := s.vals[k]
		if inL && inS && lv == sv {
			continue
		}
		if strings.HasPrefix(k, "\tline\t") {
			lines++
			continue
		}
		n++
		if n <= 4 {
			name := k[strings.Index(k, "\t")+1:]
			settings = append(settings, fmt.Sprintf("%s: %s -> %s", name, show(sv, inS), show(lv, inL)))
		}
	}
	if n > 4 {
		settings = append(settings, fmt.Sprintf("+%d more setting%s", n-4, plural(n-4)))
	}
	if lines > 0 {
		settings = append(settings, fmt.Sprintf("%d line%s differ", lines, plural(lines)))
	}
	return settings
}

func show(v string, ok bool) string {
	switch {
	case !ok:
		return "(none)"
	case v == "":
		return `""`
	case len(v) > 24:
		return v[:21] + "..."
	}
	return v
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// Full is every difference, for "show the full differences": .cfg files per
// setting with both values in full; other files as a line diff (comments and
// blank lines ignored; at most 200 lines).
func Full(path, local, server string) string {
	var b strings.Builder
	if isCfg(path) {
		l, s := parse(local, true), parse(server, true)
		seen := map[string]bool{}
		for _, k := range append(append([]string{}, l.order...), s.order...) {
			if seen[k] || strings.HasPrefix(k, "\tline\t") {
				continue
			}
			seen[k] = true
			lv, inL := l.vals[k]
			sv, inS := s.vals[k]
			if inL && inS && lv == sv {
				continue
			}
			sec, key, _ := strings.Cut(k, "\t")
			if sec == "" || sec == "[]" {
				fmt.Fprintf(&b, "    %s\n", key)
			} else {
				fmt.Fprintf(&b, "    %s %s\n", sec, key)
			}
			fmt.Fprintf(&b, "        server: %s\n", orNotSet(sv, inS))
			fmt.Fprintf(&b, "        yours:  %s\n", orNotSet(lv, inL))
		}
		return strings.TrimRight(b.String(), "\n")
	}
	n := 0
	for _, d := range lineDiff(meaningful(server), meaningful(local)) {
		if n == 200 {
			break
		}
		if d.removed {
			fmt.Fprintf(&b, "    server: %s\n", d.text)
		} else {
			fmt.Fprintf(&b, "    yours:  %s\n", d.text)
		}
		n++
	}
	return strings.TrimRight(b.String(), "\n")
}

func orNotSet(v string, ok bool) string {
	if !ok {
		return "(not set)"
	}
	return v
}

// meaningful: the lines a line diff looks at (no blank or # comment lines).
func meaningful(text string) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimRight(l, "\r")
		if t := strings.TrimSpace(l); t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		out = append(out, l)
	}
	return out
}

type diffLine struct {
	removed bool // in a (the server's) but not b (yours)
	text    string
}

// lineDiff lists the lines only in a (removed) or only in b (added), in
// order, from a longest-common-subsequence match. Config files are small;
// past ~4 million line pairs it falls back to comparing them as sets.
func lineDiff(a, b []string) []diffLine {
	var out []diffLine
	if len(a)*len(b) > 4_000_000 {
		inA, inB := map[string]bool{}, map[string]bool{}
		for _, l := range a {
			inA[l] = true
		}
		for _, l := range b {
			inB[l] = true
		}
		for _, l := range a {
			if !inB[l] {
				out = append(out, diffLine{true, l})
			}
		}
		for _, l := range b {
			if !inA[l] {
				out = append(out, diffLine{false, l})
			}
		}
		return out
	}
	// lcs[i][j] = length of the common subsequence of a[i:] and b[j:]
	lcs := make([][]int, len(a)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, diffLine{true, a[i]})
			i++
		default:
			out = append(out, diffLine{false, b[j]})
			j++
		}
	}
	for ; i < len(a); i++ {
		out = append(out, diffLine{true, a[i]})
	}
	for ; j < len(b); j++ {
		out = append(out, diffLine{false, b[j]})
	}
	return out
}
