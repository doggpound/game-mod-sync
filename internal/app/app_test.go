package app

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/doggpound/game-mod-sync/internal/remote"
	"github.com/doggpound/game-mod-sync/internal/state"
	"github.com/doggpound/game-mod-sync/internal/testserver"
	"github.com/doggpound/game-mod-sync/internal/ui/text"
)

type appSetup struct {
	srv   *testserver.Server
	cfg   string // config folder
	root  string // game folder on the server (AMP layout)
	pdir  string // the Gale profile
	dials int    // logins so far
}

// newAppSetup: a Gale Valheim profile MyProfile, an AMP-style instance on the
// test server, and a saved target between them.
func newAppSetup(t *testing.T) *appSetup {
	srv := testserver.Start(t)
	s := &appSetup{srv: srv, cfg: t.TempDir()}
	gale := filepath.Join(t.TempDir(), "com.kesomannen.gale")
	s.pdir = filepath.Join(gale, "valheim", "profiles", "MyProfile")
	write(t, filepath.Join(s.pdir, "BepInEx", "plugins", "Azumatt-AzuClock", "AzuClock.dll"), "clock")
	s.root = filepath.ToSlash(filepath.Join(srv.Root, ".ampdata", "instances", "Valheim01", "Valheim", "896660"))
	write(t, filepath.Join(s.root, "BepInEx", "plugins", "Old", "Old.dll"), "old")
	t.Setenv("GMS_CONFIG_DIR", s.cfg)
	st, _ := state.Load(s.cfg)
	st.Config.GaleDataDir = gale
	st.Config.AMPInstancesDir = filepath.ToSlash(filepath.Join(srv.Root, ".ampdata", "instances"))
	st.SetTarget(state.Target{Source: "gale", Game: "valheim", Profile: "MyProfile", Backend: "amp",
		Host: srv.Host, Port: srv.Port, User: "amp", Root: s.root})
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	return s
}

// run runs the program with args and typed answers; returns its output.
func (s *appSetup) run(t *testing.T, answers string, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	st, err := state.Load(s.cfg)
	if err != nil {
		t.Fatal(err)
	}
	a := &App{UI: text.New(strings.NewReader(answers), &out), State: st, Opts: parse(t, args),
		Dial: func(tg state.Target) (*remote.Client, error) {
			s.dials++
			return remote.Dial(remote.Options{Host: tg.Host, Port: tg.Port, User: tg.User,
				Auth: []ssh.AuthMethod{ssh.Password(testserver.Password)}, HostKey: ssh.FixedHostKey(s.srv.HostKey)})
		}}
	err = a.Main()
	return out.String(), err
}

func parse(t *testing.T, args []string) Options {
	var o Options
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-p":
			i++
			o.Profile = args[i]
		case "-y":
			o.Yes = true
		case "-n":
			o.DryRun = true
		default:
			t.Fatalf("test helper: unknown %s", args[i])
		}
	}
	return o
}

func TestSyncOneProfile(t *testing.T) {
	s := newAppSetup(t)
	out, err := s.run(t, "", "-p", "MyProfile", "-y")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !exists(filepath.Join(s.root, "BepInEx", "plugins", "Azumatt-AzuClock", "AzuClock.dll")) ||
		exists(filepath.Join(s.root, "BepInEx", "plugins", "Old")) {
		t.Errorf("server not synced:\n%s", out)
	}
	for _, want := range []string{"Valheim / MyProfile  ->  Valheim01 on 127.0.0.1", "+ new      Azumatt-AzuClock", "Restart the server"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestMainMenu(t *testing.T) {
	s := newAppSetup(t)
	out, err := s.run(t, "1e\nb\n1\ny\nx\n") // Edit, Back, sync it, yes, exit
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	for _, want := range []string{"1) Valheim / MyProfile  -> Valheim01 on 127.0.0.1", "-- Edit: Valheim / MyProfile --", "Synced."} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Count(out, "-- Main menu --") != 3 {
		t.Errorf("expected the menu 3 times (start, after Edit, after the sync):\n%s", out)
	}
}

func TestAProfileWithoutAServer(t *testing.T) {
	s := newAppSetup(t)
	st, _ := state.Load(s.cfg)
	st.RemoveTarget("gale", "valheim", "MyProfile")
	st.Save()
	_, err := s.run(t, "", "-p", "MyProfile", "-y")
	if err == nil || !strings.Contains(err.Error(), "no server set up yet") {
		t.Errorf("err = %v", err)
	}
}

func TestUnknownProfile(t *testing.T) {
	s := newAppSetup(t)
	if _, err := s.run(t, "", "-p", "Nope"); err == nil || !strings.Contains(err.Error(), "no profile named 'Nope'") {
		t.Errorf("err = %v", err)
	}
}

func TestServerInstalledFileIsAskedOnceAndLeftAlone(t *testing.T) {
	s := newAppSetup(t)
	write(t, filepath.Join(s.srv.Root, ".ampdata", "instances", "Valheim01", "GenericModule.kvp"),
		`App.AppSettings={"valheim_plus_install":"true","other":"x"}`+"\n")
	vp := filepath.Join(s.root, "BepInEx", "plugins", "ValheimPlus.dll")
	write(t, vp, "installed by AMP")

	out, err := s.run(t, "y\ny\n", "-p", "MyProfile") // leave it alone: yes; apply: yes
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if !strings.Contains(out, `The server installs ValheimPlus.dll itself (AMP setting "valheim_plus_install" is on for Valheim01)`) {
		t.Errorf("not asked:\n%s", out)
	}
	if read(t, vp) != "installed by AMP" {
		t.Error("the server's ValheimPlus.dll was synced over")
	}
	st, _ := state.Load(s.cfg)
	if tg, _ := st.Target("gale", "valheim", "MyProfile"); strings.Join(tg.Kept, ",") != "+ValheimPlus.dll" {
		t.Errorf("answer not saved: %v", tg.Kept)
	}

	out, err = s.run(t, "", "-p", "MyProfile", "-y")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "The server installs") || !strings.Contains(out, "Leaving ValheimPlus.dll alone") {
		t.Errorf("second run:\n%s", out)
	}
	if read(t, vp) != "installed by AMP" {
		t.Error("synced over on the second run")
	}
}

func TestListShowsTargets(t *testing.T) {
	newAppSetup(t) // sets GMS_CONFIG_DIR
	var out bytes.Buffer
	if err := Run([]string{"--list"}, strings.NewReader(""), &out, "test"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Valheim / MyProfile -> Valheim01 on 127.0.0.1") {
		t.Errorf("%s", out.String())
	}
}
