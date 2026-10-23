package transfer

import "testing"

func TestExcludes(t *testing.T) {
	cases := []struct {
		pats []string
		rel  string
		dir  bool
		want bool
	}{
		{[]string{"MMHOOK/"}, "MMHOOK", true, true},
		{[]string{"MMHOOK/"}, "MMHOOK/MMHOOK_assembly.dll", false, true}, // inside an excluded folder
		{[]string{"MMHOOK/"}, "MMHOOK", false, false},                    // a file named MMHOOK isn't a folder
		{[]string{"MMHOOK/"}, "Some-Mod/MMHOOK", true, true},             // anywhere
		{[]string{"/ValheimPlus.dll"}, "ValheimPlus.dll", false, true},
		{[]string{"/ValheimPlus.dll"}, "Grantapher-ValheimPlus/ValheimPlus.dll", false, false}, // anchored
		{[]string{"Valheim.DisplayBepInExInfo.dll"}, "Valheim.DisplayBepInExInfo.dll", false, true},
		{[]string{"Valheim.DisplayBepInExInfo.dll"}, "x/Valheim.DisplayBepInExInfo.dll", false, true},
		{[]string{"*.dat"}, "a/b/players.dat", false, true},
		{[]string{"*.dat"}, "a/b/players.data", false, false},
		{[]string{"a/b.cfg"}, "a/b.cfg", false, true},
		{[]string{"a/b.cfg"}, "x/a/b.cfg", false, true},
		{[]string{"a/b.cfg"}, "xa/b.cfg", false, false},
		{[]string{"/a/*.cfg"}, "a/b.cfg", false, true},
		{[]string{"/a/*.cfg"}, "a/c/b.cfg", false, false}, // * stops at /
		{[]string{"/a/**.cfg"}, "a/c/b.cfg", false, true}, // ** doesn't
		{[]string{"?.txt"}, "a.txt", false, true},
		{[]string{"?.txt"}, "ab.txt", false, false},
		{nil, "anything", false, false},
	}
	for _, c := range cases {
		if got := Excludes(c.pats).Match(c.rel, c.dir); got != c.want {
			t.Errorf("%v.Match(%q, dir=%v) = %v, want %v", c.pats, c.rel, c.dir, got, c.want)
		}
	}
}
