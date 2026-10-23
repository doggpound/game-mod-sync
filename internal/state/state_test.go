package state

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func write(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMigratesTheBashFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "config"), `# game-mod-sync config - shell syntax, read on every run.
GALE_DATA_DIR=/data/Gale\ Data
LAST_BACKEND=amp
LAST_HOST=game.example.com
LAST_USER=amp
AMP_INSTANCES_DIR=.ampdata/instances
EXTRA_EXCLUDES='a.dll b.dll'
KEEP_BACKUPS=3
DEFAULT_UI=tui
LOCAL_WORLDS=valheim=/mnt/c/w
`)
	write(t, filepath.Join(dir, "targets.tsv"),
		"gale\tvalheim\tMyProfile\tamp\tamp@game.example.com\t/srv/v1\t\n"+
			"gale\tvalheim\tOther\tamp\tamp@game.example.com\t/srv/v2\t,+ValheimPlus.dll,-X.dll\t/w\n")
	s, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Migrated {
		t.Error("not marked as migrated")
	}
	want := Config{GaleDataDir: "/data/Gale Data", LastBackend: "amp", LastHost: "game.example.com", LastUser: "amp",
		AMPInstancesDir: ".ampdata/instances", ExtraExcludes: []string{"a.dll", "b.dll"}, KeepBackups: 3,
		DefaultUI: "tui", LocalWorlds: map[string]string{"valheim": "/mnt/c/w"}}
	if !reflect.DeepEqual(s.Config, want) {
		t.Errorf("config:\n got  %+v\n want %+v", s.Config, want)
	}
	o, ok := s.Target("gale", "valheim", "Other")
	if !ok || o.Host != "game.example.com" || o.User != "amp" || o.Root != "/srv/v2" ||
		!reflect.DeepEqual(o.Kept, []string{"+ValheimPlus.dll", "-X.dll"}) || o.Worlds != "/w" {
		t.Errorf("target: %+v", o)
	}
	// Saved as TOML, and read back the same, without migrating again.
	s2, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s2.Migrated || !reflect.DeepEqual(s2.Config, s.Config) || len(s2.Targets) != 2 {
		t.Errorf("reloaded: migrated=%v %+v %d targets", s2.Migrated, s2.Config, len(s2.Targets))
	}
	if _, err := os.Stat(filepath.Join(dir, "config")); err != nil {
		t.Error("the bash config was removed")
	}
}

func TestPicksUpProfilesSetUpWithBashLater(t *testing.T) {
	dir := t.TempDir()
	s, _ := Load(dir)
	s.SetTarget(Target{Source: "gale", Game: "valheim", Profile: "A", Backend: "ssh", Host: "h", User: "u", Root: "/a"})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "targets.tsv"),
		"gale\tvalheim\tA\tssh\tu@old\t/old\t\ngale\tvalheim\tB\tssh\tu@h\t/b\t\n")
	s, _ = Load(dir)
	a, _ := s.Target("gale", "valheim", "A")
	b, ok := s.Target("gale", "valheim", "B")
	if a.Root != "/a" || !ok || b.Root != "/b" {
		t.Errorf("A=%+v B=%+v", a, b)
	}
}

func TestNothingSavedIsTheDefaults(t *testing.T) {
	s, err := Load(t.TempDir())
	if err != nil || s.Migrated || s.Config.KeepBackups != 5 || len(s.Targets) != 0 {
		t.Errorf("%+v %v", s, err)
	}
}

func TestRemoveTarget(t *testing.T) {
	s := &State{}
	s.SetTarget(Target{Source: "gale", Game: "g", Profile: "a"})
	s.SetTarget(Target{Source: "gale", Game: "g", Profile: "b"})
	s.SetTarget(Target{Source: "gale", Game: "g", Profile: "a", Root: "/new"})
	s.RemoveTarget("gale", "g", "b")
	if len(s.Targets) != 1 || s.Targets[0].Root != "/new" {
		t.Errorf("%+v", s.Targets)
	}
}
