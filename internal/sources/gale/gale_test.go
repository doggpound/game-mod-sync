package gale

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestList(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"valheim/profiles/MyProfile", "valheim/profiles/Vanilla", "lethal-company/profiles/Fun", "no-profiles/other"} {
		os.MkdirAll(filepath.Join(root, p), 0o755)
	}
	os.WriteFile(filepath.Join(root, "valheim", "profiles", "stray.txt"), nil, 0o644)
	got, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range got {
		names = append(names, p.Game+"/"+p.Name)
	}
	if want := []string{"lethal-company/Fun", "valheim/MyProfile", "valheim/Vanilla"}; !reflect.DeepEqual(names, want) {
		t.Errorf("got %v, want %v", names, want)
	}
}

func TestCandidatesUnderWindowsUsersSeenFromWSL(t *testing.T) {
	root := t.TempDir()
	d := filepath.Join(root, "mnt", "c", "Users", "me", "AppData", "Roaming", folder)
	os.MkdirAll(d, 0o755)
	t.Setenv("GMS_WIN_USERS", filepath.Join(root, "mnt", "*", "Users", "*"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", "")
	if got := Candidates(); !reflect.DeepEqual(got, []string{d}) {
		t.Errorf("got %v", got)
	}
}
