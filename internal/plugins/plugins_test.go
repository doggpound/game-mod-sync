package plugins

import (
	"testing"

	"github.com/doggpound/game-mod-sync/internal/state"
)

type srv struct {
	name     string
	fallback bool
}

func (s srv) Name() string                                         { return s.name }
func (s srv) Label() string                                        { return s.name }
func (s srv) Help() string                                         { return "" }
func (s srv) DefaultUser() string                                  { return "" }
func (s srv) SearchFrom(*state.Config, string, bool) (string, int) { return "", 0 }
func (s srv) ShortName(_ *state.Config, root string) string        { return root }
func (s srv) Fallback() bool                                       { return s.fallback }

func TestServersListTheFallbackLast(t *testing.T) {
	RegisterServer(srv{name: "aaa-other", fallback: true})
	RegisterServer(srv{name: "bbb"})
	RegisterServer(srv{name: "ccc"})
	var names []string
	for _, s := range Servers() {
		names = append(names, s.Name())
	}
	if len(names) < 3 || names[len(names)-1] != "aaa-other" || names[0] != "bbb" {
		t.Errorf("order: %v", names)
	}
	if _, ok := ServerNamed("ccc"); !ok {
		t.Error("ServerNamed")
	}
}

func TestTwoPluginsWithOneNameAreRefused(t *testing.T) {
	RegisterServer(srv{name: "dup"})
	defer func() {
		if recover() == nil {
			t.Error("a second plugin named dup was accepted")
		}
	}()
	RegisterServer(srv{name: "dup"})
}

type ldr struct{ name, marker string }

func (l ldr) Name() string           { return l.name }
func (l ldr) Label() string          { return l.name }
func (l ldr) Detect(dir string) bool { return dir == l.marker }
func (l ldr) Layout() Layout         { return Layout{SyncDirs: []string{l.name}} }

func TestDetectLoader(t *testing.T) {
	RegisterLoader(ldr{"aloader", "a-profile"})
	RegisterLoader(ldr{"bloader", "b-profile"})
	if l, _ := DetectLoader("b-profile"); l.Name() != "bloader" {
		t.Errorf("detected %s", l.Name())
	}
	if l, _ := DetectLoader(""); l.Name() != "aloader" {
		t.Errorf("without a profile: %s, want the first", l.Name())
	}
	if _, err := WorldFormatNamed("nope", nil); err == nil {
		t.Error("an unknown world format was found")
	}
}
