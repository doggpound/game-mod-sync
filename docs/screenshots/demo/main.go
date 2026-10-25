// The screenshots' made-up setup: a Gale profile, an AMP-style game server
// (the tests' SSH/SFTP server, on 127.0.0.1) and the settings that point one
// at the other, all under -dir. ~/.ssh/config makes the server show up as
// game.example.com. It writes <dir>/env (the environment to run
// game-mod-sync in) and serves until it's stopped. Used by make.sh.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"github.com/doggpound/game-mod-sync/internal/testserver"
)

func write(path, content string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		log.Fatal(err)
	}
}

// dll is a plugin DLL with its [BepInPlugin(id, name, version)] attribute.
func dll(path, id, name, version string) {
	b := []byte("MZ\x90\x00demo\x01\x00")
	for _, s := range []string{id, name, version} {
		b = append(b, byte(len(s)))
		b = append(b, s...)
	}
	write(path, string(append(b, 0, 0)))
}

func world(dir, name string, save int) {
	w := filepath.Join(dir, name)
	for _, ext := range []string{"fwl2", "db2", "chunks"} {
		write(filepath.Join(w, fmt.Sprintf("_main.%d.%s", save, ext)), fmt.Sprint(ext, save))
	}
	for i := 0; i < 12+save%7; i++ { // about a megabyte each, so sizes look real
		write(filepath.Join(w, fmt.Sprintf("%02x_1a__1_1.chunk", i)), strings.Repeat(fmt.Sprintf("chunk %d %d ", save, i), 80000))
	}
	write(filepath.Join(w, fmt.Sprintf("_main.%d.ok", save)), "ok")
	// Fixed times, so the pictures are the same on every run.
	at := time.Date(2026, 10, 20, 18, 55, 0, 0, time.Local).Add(time.Duration(save) * time.Minute)
	entries, _ := os.ReadDir(w)
	for _, e := range entries {
		os.Chtimes(filepath.Join(w, e.Name()), at, at)
	}
}

func main() {
	dir := flag.String("dir", "", "folder for the demo (made empty first)")
	flag.Parse()
	if *dir == "" {
		log.Fatal("-dir is required")
	}
	d, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatal(err)
	}
	os.RemoveAll(d)

	// The profile: two updated or new mods, configs that are new, differ,
	// or belong to a client-only mod.
	gale := filepath.Join(d, "gale")
	p := filepath.Join(gale, "valheim", "profiles", "MyProfile", "BepInEx")
	dll(filepath.Join(p, "plugins", "Azumatt-AzuClock", "AzuClock.dll"), "Azumatt.AzuClock", "AzuClock", "1.0.4")
	dll(filepath.Join(p, "plugins", "ArgusMagnus-ServersideQoL", "ServersideQoL.dll"), "argusmagnus.ServersideQoL", "ServersideQoL", "1.0.10")
	dll(filepath.Join(p, "plugins", "Azumatt-AzuAutoStore", "AzuAutoStore.dll"), "Azumatt.AzuAutoStore", "AzuAutoStore", "3.0.6")
	dll(filepath.Join(p, "plugins", "ValheimModding-Jotunn", "Jotunn.dll"), "com.jotunn.jotunn", "Jotunn", "2.24.0")
	write(filepath.Join(p, "core", "BepInEx.dll"), "core")
	write(filepath.Join(p, "config", "Azumatt.AzuClock.cfg"), "[General]\nShow = true\n")
	write(filepath.Join(p, "config", "argusmagnus.ServersideQoL.cfg"), "[General]\nAutoStore = true\nAutoPickup = true\n")
	write(filepath.Join(p, "config", "Azumatt.AzuAutoStore.cfg"), "[General]\nLock Configuration = On\nRange = 20\n\n[Chests]\nAllowed = Chest,Barrel\n")

	// The server: an older Jotunn, a mod that's gone from the profile, the
	// server's own MMHOOK, AzuAutoStore's config with other settings, worlds.
	srvRoot := filepath.Join(d, "server")
	root := filepath.Join(srvRoot, ".ampdata", "instances", "Valheim01", "Valheim", "896660")
	s := filepath.Join(root, "BepInEx")
	dll(filepath.Join(s, "plugins", "ValheimModding-Jotunn", "Jotunn.dll"), "com.jotunn.jotunn", "Jotunn", "2.23.1")
	dll(filepath.Join(s, "plugins", "Azumatt-AzuAutoStore", "AzuAutoStore.dll"), "Azumatt.AzuAutoStore", "AzuAutoStore", "3.0.6")
	dll(filepath.Join(s, "plugins", "Old-RemovedMod", "Old.dll"), "old.removed", "Old", "1.0.0")
	write(filepath.Join(s, "plugins", "MMHOOK", "MMHOOK_assembly_valheim.dll"), "generated")
	write(filepath.Join(s, "config", "Azumatt.AzuAutoStore.cfg"), "[General]\nLock Configuration = Off\nRange = 10\n\n[Chests]\nAllowed = Chest,Barrel\n")
	write(filepath.Join(srvRoot, ".ampdata", "instances", "Valheim01", "GenericModule.kvp"),
		"App.CommandLineArgs=-name \"{{ServerName}}\" -world \"{{WorldName}}\" -savedir \"Saves\"\n"+
			"App.AppSettings={\"WorldName\":\"Midgard\"}\n")
	world(filepath.Join(root, "Saves", "worlds_local"), "Midgard", 1940)
	world(filepath.Join(root, "Saves", "worlds_local"), "Ashlands", 55)
	world(filepath.Join(root, "Saves", "worlds_local"), "Midgard_backup_auto-20261024-120000", 1931)

	srv, err := testserver.New(srvRoot, testserver.AcceptAny())
	if err != nil {
		log.Fatal(err)
	}
	defer srv.Close()

	// This computer: a key, known_hosts with the server's key, and
	// ~/.ssh/config so it's game.example.com.
	home := filepath.Join(d, "home")
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		log.Fatal(err)
	}
	write(filepath.Join(home, ".ssh", "id_ed25519"), string(pem.EncodeToMemory(block)))
	write(filepath.Join(home, ".ssh", "known_hosts"), knownhosts.Line([]string{knownhosts.Normalize(srv.Addr())}, srv.HostKey)+"\n")
	write(filepath.Join(home, ".ssh", "config"), fmt.Sprintf("Host game.example.com\n  HostName %s\n  Port %d\n", srv.Host, srv.Port))
	world(filepath.Join(d, "local", "worlds_local"), "Midgard", 1902)

	cfg := filepath.Join(d, "config")
	write(filepath.Join(cfg, "config.toml"), fmt.Sprintf("keep_backups = 5\nupdate_check = false\n\n[source_dirs]\n  gale = %q\n\n[local_worlds]\n  valheim = %q\n",
		filepath.Join(gale), filepath.Join(d, "local", "worlds_local")))
	write(filepath.Join(cfg, "targets.toml"), fmt.Sprintf("[[target]]\n  source = \"gale\"\n  game = \"valheim\"\n  profile = \"MyProfile\"\n"+
		"  backend = \"amp\"\n  host = \"game.example.com\"\n  port = 22\n  user = \"amp\"\n  root = %q\n", filepath.ToSlash(root)))
	// The mods' client/server tags, so nothing is looked up online.
	write(filepath.Join(d, "cache", "game-mod-sync", "thunderstore-tags.tsv"),
		"ArgusMagnus-ServersideQoL\t0\tServer-side\nAzumatt-AzuAutoStore\t0\tClient-side|Server-side\n"+
			"Azumatt-AzuClock\t0\tClient-side\nValheimModding-Jotunn\t0\tClient-side|Server-side\n")

	write(filepath.Join(d, "env"), fmt.Sprintf("HOME=%s\nGMS_CONFIG_DIR=%s\nXDG_CACHE_HOME=%s\nSSH_AUTH_SOCK=\n",
		home, cfg, filepath.Join(d, "cache")))
	fmt.Println("demo server on", srv.Addr())
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}
