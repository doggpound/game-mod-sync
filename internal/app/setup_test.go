package app

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/doggpound/game-mod-sync/internal/remote"
	"github.com/doggpound/game-mod-sync/internal/secrets"
	"github.com/doggpound/game-mod-sync/internal/state"
	"github.com/doggpound/game-mod-sync/internal/testserver"
	"github.com/doggpound/game-mod-sync/internal/ui/text"
)

func TestParseServer(t *testing.T) {
	cases := []struct {
		in           string
		host         string
		port         int
		user, folder string
	}{
		{"game.example.com", "game.example.com", 0, "", ""},
		{"amp@game.example.com", "game.example.com", 0, "amp", ""},
		{"game.example.com:2022", "game.example.com", 2022, "", ""},
		{"sftp://me.a1b2c3@game.example.com:2022/", "game.example.com", 2022, "me.a1b2c3", ""},
		{"sftp://me@game.example.com:2022/home/container", "game.example.com", 2022, "me", "/home/container"},
		{"  ssh://game.example.com  ", "game.example.com", 0, "", ""},
	}
	for _, c := range cases {
		h, p, u, f := parseServer(c.in)
		if h != c.host || p != c.port || u != c.user || f != c.folder {
			t.Errorf("parseServer(%q) = %q %d %q %q", c.in, h, p, u, f)
		}
	}
}

// newSetupCase: a Gale profile and a test server, nothing set up yet.
func newSetupCase(t *testing.T, opts ...testserver.Option) *appSetup {
	srv := testserver.Start(t, opts...)
	s := &appSetup{srv: srv, cfg: t.TempDir()}
	gale := filepath.Join(t.TempDir(), "com.kesomannen.gale")
	s.pdir = filepath.Join(gale, "valheim", "profiles", "MyProfile")
	write(t, filepath.Join(s.pdir, "BepInEx", "plugins", "Azumatt-AzuClock", "AzuClock.dll"), "clock")
	t.Setenv("GMS_CONFIG_DIR", s.cfg)
	st, _ := state.Load(s.cfg)
	st.Config.GaleDataDir = gale
	st.Save()
	return s
}

func (s *appSetup) instance(t *testing.T, name, game string) string {
	r := filepath.ToSlash(filepath.Join(s.srv.Root, ".ampdata", "instances", name, game, "1"))
	write(t, filepath.Join(r, "BepInEx", "plugins", "x.dll"), "x")
	return r
}

func (s *appSetup) target(t *testing.T) state.Target {
	st, _ := state.Load(s.cfg)
	tg, ok := st.Target("gale", "valheim", "MyProfile")
	if !ok {
		t.Fatal("nothing saved")
	}
	return tg
}

func TestSetupAMPFindsTheInstance(t *testing.T) {
	s := newSetupCase(t)
	root := s.instance(t, "Valheim01", "Valheim")
	answers := fmt.Sprintf("1\n%s\n%d\n\ny\n", s.srv.Host, s.srv.Port) // AMP, host, port, user (default amp), apply
	out, err := s.run(t, answers, "-p", "MyProfile")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	tg := s.target(t)
	if tg.Backend != "amp" || tg.User != "amp" || tg.Port != s.srv.Port || tg.Root != root {
		t.Errorf("saved %+v", tg)
	}
	if !strings.Contains(out, "Saved: Valheim / MyProfile -> Valheim01") || !strings.Contains(out, "Synced.") {
		t.Errorf("output:\n%s", out)
	}
	if s.dials != 1 {
		t.Errorf("logged in %d times for setup + sync, want 1", s.dials)
	}
	if !exists(filepath.Join(root, "BepInEx", "plugins", "Azumatt-AzuClock", "AzuClock.dll")) {
		t.Error("not synced after setup")
	}
}

func TestSetupAMPOffersAChoiceWithTheGameFirst(t *testing.T) {
	s := newSetupCase(t)
	s.instance(t, "LethalCompany01", "LethalCompany")
	root := s.instance(t, "Valheim01", "Valheim")
	answers := fmt.Sprintf("1\n%s\n%d\namp\n1\nn\n", s.srv.Host, s.srv.Port) // ..., pick 1, don't apply
	out, err := s.run(t, answers, "-p", "MyProfile")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "1) Valheim01") || !strings.Contains(out, "2) LethalCompany01") || !strings.Contains(out, "(other game?)") {
		t.Errorf("menu:\n%s", out)
	}
	if s.target(t).Root != root {
		t.Errorf("saved %+v", s.target(t))
	}
}

func TestSetupOtherOnAnSFTPOnlyHost(t *testing.T) {
	s := newSetupCase(t, testserver.SFTPOnly())
	root := filepath.ToSlash(filepath.Join(s.srv.Root, "valheim-server"))
	write(t, filepath.Join(root, "BepInEx", "plugins", "x.dll"), "x")
	answers := fmt.Sprintf("2\n%s\n%d\nme.a1b2c3\ny\n", s.srv.Host, s.srv.Port)
	out, err := s.run(t, answers, "-p", "MyProfile")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if tg := s.target(t); tg.Backend != "ssh" || tg.User != "me.a1b2c3" || tg.Root != root {
		t.Errorf("saved %+v\n%s", tg, out)
	}
	if !exists(filepath.Join(root, "BepInEx", "plugins", "Azumatt-AzuClock")) {
		t.Errorf("not synced:\n%s", out)
	}
}

func TestSetupFromAPastedLink(t *testing.T) {
	s := newSetupCase(t, testserver.SFTPOnly())
	root := filepath.ToSlash(filepath.Join(s.srv.Root, "srv", "valheim"))
	write(t, filepath.Join(root, "BepInEx", "plugins", "x.dll"), "x")
	answers := fmt.Sprintf("2\nsftp://me@%s:%d%s\nn\n", s.srv.Host, s.srv.Port, root) // no port or user questions
	out, err := s.run(t, answers, "-p", "MyProfile")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if tg := s.target(t); tg.User != "me" || tg.Port != s.srv.Port || tg.Root != root {
		t.Errorf("saved %+v\n%s", tg, out)
	}
	if strings.Contains(out, "Port") || strings.Contains(out, "User name") {
		t.Errorf("asked what the link already said:\n%s", out)
	}
}

func TestSetupTypeTheFolder(t *testing.T) {
	s := newSetupCase(t, testserver.SFTPOnly())
	answers := fmt.Sprintf("2\n%s\n%d\nme\n1\nnew-server\nn\n", s.srv.Host, s.srv.Port) // nothing found: Type the folder
	write(t, filepath.Join(s.srv.Root, "new-server", "readme.txt"), "")
	out, err := s.run(t, answers, "-p", "MyProfile")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "No BepInEx/plugins folders found") || !strings.Contains(out, "will be created on the first sync") {
		t.Errorf("output:\n%s", out)
	}
	if tg := s.target(t); tg.Root != filepath.ToSlash(filepath.Join(s.srv.Root, "new-server")) {
		t.Errorf("saved %+v", tg)
	}
}

func TestSetupBackSavesNothing(t *testing.T) {
	s := newSetupCase(t)
	out, err := s.run(t, "b\n", "-p", "MyProfile")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	st, _ := state.Load(s.cfg)
	if len(st.Targets) != 0 {
		t.Errorf("saved %+v", st.Targets)
	}
}

func TestSetupNeedsAnswersSoNotWithY(t *testing.T) {
	s := newSetupCase(t)
	if _, err := s.run(t, "", "-p", "MyProfile", "-y"); err == nil || !strings.Contains(err.Error(), "without -y") {
		t.Errorf("err = %v", err)
	}
}

// The real login path: known_hosts, no keys, a typed password that's saved
// and used the next time without asking.
func TestPasswordIsSavedAndUsed(t *testing.T) {
	s := newSetupCase(t, testserver.SFTPOnly())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SSH_AUTH_SOCK", "")
	store := secrets.Memory{}
	tg := state.Target{Host: s.srv.Host, Port: s.srv.Port, User: "me"}
	dial := func(answers string) (string, error) {
		var out bytes.Buffer
		st, _ := state.Load(s.cfg)
		a := &App{UI: text.New(strings.NewReader(answers), &out), State: st, Secrets: store}
		c, err := a.dial(tg)
		if err == nil {
			c.Close()
		}
		return out.String(), err
	}
	out, err := dial("y\n" + testserver.Password + "\ny\n") // trust, password, save
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, "First connection to") || !strings.Contains(out, "Saved - it won't be asked again") {
		t.Errorf("first login:\n%s", out)
	}
	if store[fmt.Sprintf("me@%s:%d", s.srv.Host, s.srv.Port)] != testserver.Password {
		t.Errorf("store: %v", store)
	}
	out, err = dial("")
	if err != nil {
		t.Fatalf("second login: %v\n%s", err, out)
	}
	if strings.Contains(out, "Password") || strings.Contains(out, "First connection") {
		t.Errorf("asked again:\n%s", out)
	}
	// A saved password that stopped working is asked for again.
	store[fmt.Sprintf("me@%s:%d", s.srv.Host, s.srv.Port)] = "old"
	out, err = dial(testserver.Password + "\nn\n")
	if err != nil || !strings.Contains(out, "Password for me@") {
		t.Errorf("changed password: %v\n%s", err, out)
	}
}

func TestEditForgetsServerAndPassword(t *testing.T) {
	s := newAppSetup(t)
	store := secrets.Memory{fmt.Sprintf("amp@%s:%d", s.srv.Host, s.srv.Port): "pw"}
	var out bytes.Buffer
	st, _ := state.Load(s.cfg)
	a := &App{UI: text.New(strings.NewReader("1e\n3\n2\ny\nb\nx\n"), &out), State: st, Secrets: store,
		Dial: func(tg state.Target) (*remote.Client, error) {
			return remote.Dial(remote.Options{Host: tg.Host, Port: tg.Port, User: tg.User,
				Auth: []ssh.AuthMethod{ssh.Password(testserver.Password)}, HostKey: ssh.FixedHostKey(s.srv.HostKey)})
		}}
	if err := a.Main(); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if len(store) != 0 {
		t.Errorf("password still saved: %v", store)
	}
	st, _ = state.Load(s.cfg)
	if len(st.Targets) != 0 || !strings.Contains(out.String(), "(not set up yet)") {
		t.Errorf("targets %v\n%s", st.Targets, out.String())
	}
}

func TestEditDefaultInterface(t *testing.T) {
	s := newAppSetup(t)
	out, err := s.run(t, "1e\n6\n2\nb\nx\n") // Edit, Default interface, tui, back, exit
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	st, _ := state.Load(s.cfg)
	if st.Config.DefaultUI != "tui" || !strings.Contains(out, "Default interface: tui (from the next start)") {
		t.Errorf("default_ui %q\n%s", st.Config.DefaultUI, out)
	}
}
