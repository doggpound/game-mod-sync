package configs

import (
	"regexp"
	"strings"
)

var nonAlnum = regexp.MustCompile(`[^a-z0-9]`)

func norm(s string) string { return nonAlnum.ReplaceAllString(strings.ToLower(s), "") }

// Owners works out which mod folder each config file belongs to ("" when
// unknown). files are paths relative to the config folder, mods the mod
// folder names, ids the mod loader's "plugin id -> mod folder" pairs. In
// order: the plugin id the file is named after (the longest that fits), a mod
// folder with the same name (ignoring case and punctuation, also with parts
// cut off the end, e.g. Azumatt.FastLink_servers), then a unique mod whose
// name (without its author) is the last part of the file name.
func Owners(files, mods []string, ids map[string]string) map[string]string {
	owner := map[string]string{}
	for id, mod := range ids {
		owner[strings.ToLower(id)] = mod
	}
	byName := map[string]string{}
	short := map[string]string{} // name without "Author-"; "" when ambiguous
	for _, m := range mods {
		byName[norm(m)] = m
		n := m
		if i := strings.Index(n, "-"); i >= 0 {
			n = n[i+1:]
		}
		n = norm(n)
		if _, dup := short[n]; dup {
			short[n] = ""
		} else {
			short[n] = m
		}
	}
	out := map[string]string{}
	for _, f := range files {
		k := f
		if i := strings.Index(k, "/"); i >= 0 {
			k = k[:i]
		} else if i := strings.LastIndex(k, "."); i >= 0 {
			k = k[:i]
		}
		lk := strings.ToLower(k)
		best, bl := "", 0
		for id, mod := range owner {
			fits := lk == id || (strings.HasPrefix(lk, id) && len(lk) > len(id) && strings.ContainsRune("._ -", rune(lk[len(id)])))
			if fits && (len(id) > bl || (len(id) == bl && mod < best)) {
				best, bl = mod, len(id)
			}
		}
		for p := k; best == ""; {
			if m, ok := byName[norm(p)]; ok {
				best = m
				break
			}
			if m := short[norm(p)]; m != "" {
				best = m
				break
			}
			q := p
			if i := strings.LastIndexAny(q, "._-"); i >= 0 {
				q = q[:i]
			}
			if q == p || len(q) < 4 {
				break
			}
			p = q
		}
		if best == "" {
			t := k
			if i := strings.LastIndex(t, "."); i >= 0 {
				t = t[i+1:]
			}
			best = short[norm(t)]
		}
		out[f] = best
	}
	return out
}

// ModName is a mod folder's name for labels: Azumatt-FastLink -> FastLink.
func ModName(mod string) string {
	if i := strings.Index(mod, "-"); i >= 0 && i < len(mod)-1 {
		return mod[i+1:]
	}
	return mod
}
