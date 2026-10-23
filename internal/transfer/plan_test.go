package transfer

import (
	"reflect"
	"testing"
)

func file(h string) Entry { return Entry{Hash: h, Size: int64(len(h))} }

var dir = Entry{Dir: true}

func TestCompare(t *testing.T) {
	local := Tree{
		"Azumatt-AzuClock":              dir,
		"Azumatt-AzuClock/AzuClock.dll": file("new"),
		"Jotunn":                        dir,
		"Jotunn/Jotunn.dll":             file("v2"),
		"Same":                          dir,
		"Same/Same.dll":                 file("s"),
	}
	server := Tree{
		"Jotunn":                     dir,
		"Jotunn/Jotunn.dll":          file("v1"),
		"Same":                       dir,
		"Same/Same.dll":              file("s"),
		"Old-RemovedMod":             dir,
		"Old-RemovedMod/sub":         dir,
		"Old-RemovedMod/sub/Old.dll": file("old"),
		"MMHOOK":                     dir,
		"MMHOOK/MMHOOK_x.dll":        file("gen"),
		"ValheimPlus.dll":            file("vp"),
	}
	p := Compare(local, server, Excludes{"MMHOOK/", "/ValheimPlus.dll"})
	if want := []string{"Azumatt-AzuClock/AzuClock.dll", "Jotunn/Jotunn.dll"}; !reflect.DeepEqual(p.Send, want) {
		t.Errorf("Send = %v, want %v", p.Send, want)
	}
	if !p.New["Azumatt-AzuClock/AzuClock.dll"] || p.New["Jotunn/Jotunn.dll"] {
		t.Errorf("New = %v", p.New)
	}
	if want := []string{"Old-RemovedMod/sub/Old.dll"}; !reflect.DeepEqual(p.Delete, want) {
		t.Errorf("Delete = %v, want %v", p.Delete, want)
	}
	if want := []string{"Old-RemovedMod/sub", "Old-RemovedMod"}; !reflect.DeepEqual(p.RmDirs, want) {
		t.Errorf("RmDirs = %v, want %v (deepest first)", p.RmDirs, want)
	}
	if want := []string{"Azumatt-AzuClock"}; !reflect.DeepEqual(p.MkDirs, want) {
		t.Errorf("MkDirs = %v, want %v", p.MkDirs, want)
	}
	if p.Bytes != 3+2 {
		t.Errorf("Bytes = %d", p.Bytes)
	}
	want := []Change{{"new", "Azumatt-AzuClock"}, {"updated", "Jotunn"}, {"removed", "Old-RemovedMod"}}
	if got := p.Summary(); !reflect.DeepEqual(got, want) {
		t.Errorf("Summary = %v, want %v", got, want)
	}
}

func TestCompareKeepsFoldersThatHoldExcludedFiles(t *testing.T) {
	server := Tree{"Gone": dir, "Gone/keep.dat": file("x"), "Gone/drop.dll": file("y")}
	p := Compare(Tree{}, server, Excludes{"*.dat"})
	if !reflect.DeepEqual(p.Delete, []string{"Gone/drop.dll"}) || len(p.RmDirs) != 0 {
		t.Errorf("Delete = %v, RmDirs = %v", p.Delete, p.RmDirs)
	}
}

func TestCompareFileBecomesFolder(t *testing.T) {
	p := Compare(Tree{"x": dir, "x/a": file("a")}, Tree{"x": file("old")}, nil)
	if !reflect.DeepEqual(p.Delete, []string{"x"}) || !reflect.DeepEqual(p.MkDirs, []string{"x"}) ||
		!reflect.DeepEqual(p.Send, []string{"x/a"}) {
		t.Errorf("plan = %+v", p)
	}
}

func TestCompareSameIsEmpty(t *testing.T) {
	tr := Tree{"a": dir, "a/b": file("h")}
	if p := Compare(tr, tr, nil); !p.Empty() {
		t.Errorf("plan = %+v", p)
	}
}
