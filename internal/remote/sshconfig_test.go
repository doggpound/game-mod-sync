package remote

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestResolve(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if r := Resolve("game.example.com", 0); r.Host != "game.example.com" || r.Port != 22 {
		t.Errorf("no config: %+v", r)
	}
	os.MkdirAll(filepath.Join(home, ".ssh"), 0o700)
	os.WriteFile(filepath.Join(home, ".ssh", "config"), []byte(`Host game.example.com
  HostName 192.0.2.10
  Port 2222
  IdentityFile ~/.ssh/game_key

Host *.lan
  Port 2200
`), 0o600)
	want := Resolved{Host: "192.0.2.10", Port: 2222, Keys: []string{filepath.Join(home, ".ssh", "game_key")}}
	if r := Resolve("game.example.com", 22); !reflect.DeepEqual(r, want) {
		t.Errorf("got %+v, want %+v", r, want)
	}
	if r := Resolve("game.example.com", 2022); r.Port != 2022 {
		t.Errorf("the setup's own port lost: %+v", r)
	}
	if r := Resolve("box.lan", 22); r.Host != "box.lan" || r.Port != 2200 {
		t.Errorf("wildcard: %+v", r)
	}
	if r := Resolve("other.example.com", 22); r.Host != "other.example.com" || r.Port != 22 || len(r.Keys) != 0 {
		t.Errorf("no match: %+v", r)
	}
}
