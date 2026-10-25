package games_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	_ "github.com/doggpound/game-mod-sync/internal/plugins/all"
	. "github.com/doggpound/game-mod-sync/internal/plugins/games"
)

// Every game file must load, so a typo fails here and not for users.
func TestEveryGameFileLoads(t *testing.T) {
	ids := Builtin()
	if len(ids) < 1 {
		t.Fatalf("found %v", ids)
	}
	for _, id := range append(ids, "_default") {
		if _, err := Load(id, "", ""); err != nil {
			t.Errorf("%s.toml: %v", id, err)
		}
	}
}

func TestValheim(t *testing.T) {
	g, err := Load("valheim", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if g.Name != "Valheim" || g.Loader != "bepinex" {
		t.Errorf("name %q loader %q", g.Name, g.Loader)
	}
	if !reflect.DeepEqual(g.SyncDirs, []string{"BepInEx/plugins"}) || !reflect.DeepEqual(g.ConfigDirs, []string{"BepInEx/config"}) {
		t.Errorf("dirs %v %v", g.SyncDirs, g.ConfigDirs)
	}
	if !reflect.DeepEqual(g.Excludes, []string{"MMHOOK/", "Valheim.DisplayBepInExInfo.dll"}) {
		t.Errorf("excludes %v", g.Excludes)
	}
	if !reflect.DeepEqual(g.Managed["amp"], []string{"valheim_plus_install=ValheimPlus.dll"}) {
		t.Errorf("managed %v", g.Managed)
	}
	if !g.MatchesServer("/home/amp/.ampdata/instances/Valheim01/x") || !g.MatchesServer("/srv/896660") || g.MatchesServer("/srv/LethalCompany01") {
		t.Errorf("server match %q", g.ServerMatch)
	}
	if g.Worlds.Format != "valheim" || g.Worlds.Saves["amp"] != "-savedir WorldName" || g.GameMarker != "BepInEx/plugins" {
		t.Errorf("worlds %+v marker %q", g.Worlds, g.GameMarker)
	}
}

func TestAGameWithoutAFileGetsTheDefaults(t *testing.T) {
	g, err := Load("lethal-company", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if g.Name != "Lethal Company" || !g.MatchesServer("/srv/LethalCompany01") || g.Thunderstore != "lethal-company" {
		t.Errorf("%+v", g)
	}
	if g.Worlds.Format != "" || g.AfterSync == "" {
		t.Errorf("worlds %q after %q", g.Worlds.Format, g.AfterSync)
	}
}

func TestYourOwnFileChangesOnlyWhatItLists(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "valheim.toml"), []byte("after_sync = \"Restart it in the panel.\"\n"), 0o644)
	g, err := Load("valheim", "", dir)
	if err != nil {
		t.Fatal(err)
	}
	if g.AfterSync != "Restart it in the panel." || g.Name != "Valheim" || len(g.Excludes) != 2 {
		t.Errorf("%+v", g)
	}
	os.WriteFile(filepath.Join(dir, "valheim.toml"), []byte("excludes = [\n"), 0o644)
	if _, err := Load("valheim", "", dir); err == nil || !strings.Contains(err.Error(), "valheim.toml") {
		t.Errorf("a broken file: err = %v", err)
	}
}
