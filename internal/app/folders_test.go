package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/doggpound/game-mod-sync/internal/remote"
	"github.com/doggpound/game-mod-sync/internal/testserver"
	"github.com/doggpound/game-mod-sync/internal/transfer"
)

// Both kinds of server: one that runs commands, and an SFTP-only game host.
var modes = map[string][]testserver.Option{"shell": nil, "sftp-only": {testserver.SFTPOnly()}}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(path string) bool { _, err := os.Stat(path); return err == nil }

type setup struct {
	srv    *testserver.Server
	client *remote.Client
	local  string // profile
	root   string // game folder on the server
	out    *bytes.Buffer
}

// newSetup: a profile with two mods, and a server game folder with an older
// Jotunn, a mod that's gone from the profile and server-generated MMHOOK.
func newSetup(t *testing.T, opts []testserver.Option) *setup {
	srv := testserver.Start(t, opts...)
	c, err := remote.Dial(remote.Options{
		Host: srv.Host, Port: srv.Port, User: "amp",
		Auth:    []ssh.AuthMethod{ssh.Password(testserver.Password)},
		HostKey: ssh.FixedHostKey(srv.HostKey),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	s := &setup{srv: srv, client: c, local: t.TempDir(), root: filepath.ToSlash(filepath.Join(srv.Root, "Valheim01")), out: &bytes.Buffer{}}
	pl := filepath.Join(s.local, "BepInEx", "plugins")
	write(t, filepath.Join(pl, "Azumatt-AzuClock", "AzuClock.dll"), "clock 1.0")
	write(t, filepath.Join(pl, "ValheimModding-Jotunn", "Jotunn.dll"), "jotunn 2.0")
	sp := filepath.Join(s.root, "BepInEx", "plugins")
	write(t, filepath.Join(sp, "ValheimModding-Jotunn", "Jotunn.dll"), "jotunn 1.0")
	write(t, filepath.Join(sp, "Old-RemovedMod", "Old.dll"), "old")
	write(t, filepath.Join(sp, "MMHOOK", "MMHOOK_assembly.dll"), "generated")
	return s
}

func (s *setup) sync(t *testing.T, change func(*FolderSync)) error {
	t.Helper()
	f := &FolderSync{
		Client: s.client, Local: s.local, Root: s.root,
		Dirs:        []string{"BepInEx/plugins"},
		Excludes:    transfer.Excludes{"MMHOOK/"},
		KeepBackups: 5, Yes: true, Out: s.out,
		Now: func() time.Time { return time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC) },
	}
	if change != nil {
		change(f)
	}
	return f.Run()
}

func TestSync(t *testing.T) {
	for name, opts := range modes {
		t.Run(name, func(t *testing.T) {
			s := newSetup(t, opts)
			if err := s.sync(t, nil); err != nil {
				t.Fatalf("%v\n%s", err, s.out)
			}
			sp := filepath.Join(s.root, "BepInEx", "plugins")
			if got := read(t, filepath.Join(sp, "Azumatt-AzuClock", "AzuClock.dll")); got != "clock 1.0" {
				t.Errorf("new mod: %q", got)
			}
			if got := read(t, filepath.Join(sp, "ValheimModding-Jotunn", "Jotunn.dll")); got != "jotunn 2.0" {
				t.Errorf("updated mod: %q", got)
			}
			if exists(filepath.Join(sp, "Old-RemovedMod")) {
				t.Error("removed mod still on the server")
			}
			if !exists(filepath.Join(sp, "MMHOOK", "MMHOOK_assembly.dll")) {
				t.Error("excluded MMHOOK was touched")
			}
			bk := filepath.Join(s.root, BackupsDir, "20261023-120000", "BepInEx", "plugins")
			if got := read(t, filepath.Join(bk, "ValheimModding-Jotunn", "Jotunn.dll")); got != "jotunn 1.0" {
				t.Errorf("backup of the replaced file: %q", got)
			}
			if !exists(filepath.Join(bk, "Old-RemovedMod", "Old.dll")) {
				t.Error("no backup of the removed file")
			}
			for _, want := range []string{"+ new      Azumatt-AzuClock", "~ updated  ValheimModding-Jotunn", "- removed  Old-RemovedMod", "2 file(s) to send, 1 to delete"} {
				if !strings.Contains(s.out.String(), want) {
					t.Errorf("output lacks %q:\n%s", want, s.out)
				}
			}
			if fi, err := os.Stat(filepath.Join(sp, "Azumatt-AzuClock", "AzuClock.dll")); err == nil && fi.Mode().Perm() != 0o644 {
				t.Errorf("permissions %v, want 644", fi.Mode().Perm())
			}
			matches, _ := filepath.Glob(filepath.Join(sp, "*", ".gms-tmp-*"))
			if len(matches) > 0 {
				t.Errorf("temporary files left: %v", matches)
			}

			s.out.Reset()
			if err := s.sync(t, nil); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(s.out.String(), "Already in sync") {
				t.Errorf("second run:\n%s", s.out)
			}
		})
	}
}

func TestSyncDryRunChangesNothing(t *testing.T) {
	for name, opts := range modes {
		t.Run(name, func(t *testing.T) {
			s := newSetup(t, opts)
			if err := s.sync(t, func(f *FolderSync) { f.DryRun = true }); err != nil {
				t.Fatal(err)
			}
			sp := filepath.Join(s.root, "BepInEx", "plugins")
			if !exists(filepath.Join(sp, "Old-RemovedMod", "Old.dll")) || exists(filepath.Join(sp, "Azumatt-AzuClock")) {
				t.Error("dry run changed the server")
			}
			if !strings.Contains(s.out.String(), "Dry run") {
				t.Errorf("output:\n%s", s.out)
			}
		})
	}
}

func TestSyncAsksAndCanBeCancelled(t *testing.T) {
	s := newSetup(t, nil)
	var asked string
	err := s.sync(t, func(f *FolderSync) {
		f.Yes = false
		f.Confirm = func(q string) bool { asked = q; return false }
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(asked, "2 file(s) to send, 1 to delete") {
		t.Errorf("question: %q", asked)
	}
	if exists(filepath.Join(s.root, "BepInEx", "plugins", "Azumatt-AzuClock")) {
		t.Error("cancelled, but the server changed")
	}
}

func TestSyncRefusesAnEmptyProfileFolder(t *testing.T) {
	s := newSetup(t, nil)
	os.RemoveAll(filepath.Join(s.local, "BepInEx", "plugins"))
	os.MkdirAll(filepath.Join(s.local, "BepInEx", "plugins"), 0o755)
	err := s.sync(t, nil)
	if !errors.Is(err, ErrNothingToSync) {
		t.Fatalf("err = %v, want ErrNothingToSync", err)
	}
	if !exists(filepath.Join(s.root, "BepInEx", "plugins", "Old-RemovedMod", "Old.dll")) {
		t.Error("the server's mods were wiped")
	}
	if !strings.Contains(s.out.String(), "is empty here") {
		t.Errorf("output:\n%s", s.out)
	}
}

func TestSyncKeepsOnlyTheNewestBackups(t *testing.T) {
	s := newSetup(t, nil)
	bk := filepath.Join(s.root, BackupsDir)
	for _, ts := range []string{"20260101-000000", "20260102-000000", "20260103-000000"} {
		os.MkdirAll(filepath.Join(bk, ts), 0o755)
	}
	if err := s.sync(t, func(f *FolderSync) { f.KeepBackups = 2 }); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(bk)
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	if strings.Join(names, " ") != "20260103-000000 20261023-120000" {
		t.Errorf("backups kept: %v", names)
	}
}

func TestSFTPOnlyUsesTheManifest(t *testing.T) {
	s := newSetup(t, []testserver.Option{testserver.SFTPOnly()})
	if err := s.sync(t, nil); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(s.root, BackupsDir, "manifest.json")) {
		t.Fatal("no manifest on the server")
	}
	// Change a server file's contents but keep its size and time: the
	// manifest's hash is trusted, so it isn't read (and isn't seen as changed).
	f := filepath.Join(s.root, "BepInEx", "plugins", "Azumatt-AzuClock", "AzuClock.dll")
	fi, _ := os.Stat(f)
	write(t, f, "CLOCK 1.0")
	os.Chtimes(f, fi.ModTime(), fi.ModTime())
	s.out.Reset()
	if err := s.sync(t, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.out.String(), "Already in sync") {
		t.Errorf("the file was read again despite the manifest:\n%s", s.out)
	}
	// A new time means it changed: read again, and synced back.
	later := fi.ModTime().Add(time.Minute)
	os.Chtimes(f, later, later)
	s.out.Reset()
	if err := s.sync(t, nil); err != nil {
		t.Fatal(err)
	}
	if got := read(t, f); got != "clock 1.0" {
		t.Errorf("after a real change: %q\n%s", got, s.out)
	}
}

func TestSFTPOnlyServerRunsNothing(t *testing.T) {
	s := newSetup(t, []testserver.Option{testserver.SFTPOnly()})
	if s.client.CanRun() {
		t.Fatal("CanRun on an SFTP-only server")
	}
	if err := s.sync(t, nil); err != nil {
		t.Fatal(err)
	}
	if n := len(s.srv.Commands()); n != 1 { // just the check
		t.Errorf("%d commands tried: %v", n, s.srv.Commands())
	}
}

// A server with several host keys, of which known_hosts has only the ed25519
// one: asking for the saved kind connects; not asking reports a changed key.
func TestKnownHostsWithSeveralServerKeys(t *testing.T) {
	srv := testserver.Start(t, testserver.ExtraHostKey())
	kh := filepath.Join(t.TempDir(), "known_hosts")
	write(t, kh, knownhosts.Line([]string{knownhosts.Normalize(srv.Addr())}, srv.HostKey)+"\n")
	cb, err := remote.HostKeys(kh, func(string, string) bool { t.Error("asked to trust a known server"); return false })
	if err != nil {
		t.Fatal(err)
	}
	opts := remote.Options{Host: srv.Host, Port: srv.Port, User: "amp",
		Auth: []ssh.AuthMethod{ssh.Password(testserver.Password)}, HostKey: cb}

	opts.HostKeyAlgorithms = remote.KnownAlgorithms(kh, srv.Host, srv.Port)
	if want := []string{ssh.KeyAlgoED25519}; strings.Join(opts.HostKeyAlgorithms, ",") != strings.Join(want, ",") {
		t.Fatalf("KnownAlgorithms = %v, want %v", opts.HostKeyAlgorithms, want)
	}
	c, err := remote.Dial(opts)
	if err != nil {
		t.Fatalf("with the saved key's algorithm: %v", err)
	}
	c.Close()

	opts.HostKeyAlgorithms = nil
	if _, err := remote.Dial(opts); !errors.Is(err, remote.ErrHostKeyChanged) {
		t.Errorf("without it: err = %v (the server shows its ECDSA key first)", err)
	}
}
