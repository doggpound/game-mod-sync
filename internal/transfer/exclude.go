package transfer

import (
	"path"
	"strings"
)

// Excludes are paths that are never sent and never deleted on the server,
// written like rsync's --exclude patterns (the parts the game files use):
//
//	MMHOOK/           a folder named MMHOOK anywhere (not a file)
//	/ValheimPlus.dll  only at the top of the synced folder
//	Some.dll          a file or folder with that name anywhere
//	a/b.cfg           that path, at the top or below any folder
//	* ? **            any name / one character / anything including /
//
// Anything inside an excluded folder is excluded too.
type Excludes []string

// Match reports whether rel (a file, or a folder when dir is true) is excluded,
// itself or through one of its parent folders.
func (ex Excludes) Match(rel string, dir bool) bool {
	parts := strings.Split(rel, "/")
	for i := range parts {
		sub := strings.Join(parts[:i+1], "/")
		isDir := dir || i < len(parts)-1
		for _, p := range ex {
			if matchOne(p, sub, isDir) {
				return true
			}
		}
	}
	return false
}

func matchOne(pat, rel string, dir bool) bool {
	if strings.HasSuffix(pat, "/") {
		if !dir {
			return false
		}
		pat = strings.TrimSuffix(pat, "/")
	}
	if strings.HasPrefix(pat, "/") {
		return glob(strings.TrimPrefix(pat, "/"), rel)
	}
	if !strings.Contains(pat, "/") {
		return glob(pat, path.Base(rel))
	}
	// Unanchored with a slash: the whole path, or its end after any folder.
	if glob(pat, rel) {
		return true
	}
	for i := 0; i < len(rel); i++ {
		if rel[i] == '/' && glob(pat, rel[i+1:]) {
			return true
		}
	}
	return false
}

// glob matches name against pat: * and ? never cross a /, ** does.
func glob(pat, name string) bool {
	for len(pat) > 0 {
		switch {
		case strings.HasPrefix(pat, "**"):
			rest := strings.TrimLeft(pat, "*")
			for i := 0; i <= len(name); i++ {
				if glob(rest, name[i:]) {
					return true
				}
			}
			return false
		case pat[0] == '*':
			for i := 0; i <= len(name); i++ {
				if glob(pat[1:], name[i:]) {
					return true
				}
				if i < len(name) && name[i] == '/' {
					return false
				}
			}
			return false
		case pat[0] == '?':
			if name == "" || name[0] == '/' {
				return false
			}
		default:
			if name == "" || name[0] != pat[0] {
				return false
			}
		}
		pat, name = pat[1:], name[1:]
	}
	return name == ""
}
