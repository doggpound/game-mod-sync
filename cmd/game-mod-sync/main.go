// game-mod-sync (Go version, in progress - see docs/DESIGN-GO.md).
//
// Without a command it's the program: the profile menu, or one profile with
// -p, with the bash version's options. Profiles are set up with the bash
// version for now; config files, the full-screen interface and world
// download come in later phases.
//
// "sync" syncs any folders to a server, without profiles or settings:
//
//	game-mod-sync sync --host game.example.com --user amp \
//	    --local /path/to/profile --root /path/to/game/on/server \
//	    [--dir BepInEx/plugins] [--exclude MMHOOK/] [-n] [-y]
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/doggpound/game-mod-sync/internal/app"
	"github.com/doggpound/game-mod-sync/internal/remote"
	"github.com/doggpound/game-mod-sync/internal/transfer"
	"github.com/doggpound/game-mod-sync/internal/ui/text"
)

// version is set when releases are built: -ldflags "-X main.version=1.0.0".
var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string, in io.Reader, out io.Writer) error {
	if len(args) > 0 {
		switch args[0] {
		case "sync":
			return runSync(args[1:], in, out)
		case "version", "--version", "-V":
			fmt.Fprintln(out, "game-mod-sync", version)
			return nil
		case "help", "--help", "-h":
			usage(out)
			return app.Run([]string{"-h"}, in, out, version)
		}
	}
	return app.Run(args, in, out, version)
}

func usage(out io.Writer) {
	fmt.Fprintf(out, `game-mod-sync %s (Go version, in progress) - sync mod-manager profiles to game servers

Usage:
  game-mod-sync [options]        the profile menu, or one profile with -p
  game-mod-sync sync [options]   sync folders without profiles (see sync -h)
  game-mod-sync version

Options:
`, version)
}

type list []string

func (l *list) String() string     { return strings.Join(*l, " ") }
func (l *list) Set(v string) error { *l = append(*l, v); return nil }

func runSync(args []string, in io.Reader, out io.Writer) error {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fs.SetOutput(out)
	host := fs.String("host", "", "server `name` or address")
	port := fs.Int("port", 22, "SSH port")
	user := fs.String("user", "", "SSH user")
	local := fs.String("local", "", "local `folder` to sync from (e.g. a Gale profile)")
	root := fs.String("root", "", "game `folder` on the server")
	keep := fs.Int("keep-backups", 5, "backups to keep on the server (0 = none)")
	dry := fs.Bool("n", false, "show what would change, change nothing")
	yes := fs.Bool("y", false, "don't ask before applying")
	knownHosts := fs.String("known-hosts", remote.DefaultKnownHosts(), "known_hosts `file`")
	var dirs, excludes, keys list
	fs.Var(&dirs, "dir", "`folder` to sync, relative to --local and --root (repeatable; default BepInEx/plugins)")
	fs.Var(&excludes, "exclude", "`pattern` never sent or deleted (repeatable), e.g. MMHOOK/")
	fs.Var(&keys, "key", "private key `file` to log in with (repeatable; default ~/.ssh/id_*)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	for name, v := range map[string]string{"--host": *host, "--user": *user, "--local": *local, "--root": *root} {
		if v == "" {
			return fmt.Errorf("%s is required (see game-mod-sync sync -h)", name)
		}
	}
	if len(dirs) == 0 {
		dirs = list{"BepInEx/plugins"}
	}
	if len(keys) == 0 {
		keys = remote.DefaultKeyFiles()
	}

	u := text.New(in, out)
	hk, err := remote.HostKeys(*knownHosts, func(host, fp string) bool {
		u.Say(fmt.Sprintf("First connection to %s. Its key fingerprint is\n  %s", host, fp))
		return u.Confirm("Trust this server?", false)
	})
	if err != nil {
		return err
	}
	auth, closeAgent := remote.Auth(keys, func() (string, error) {
		return u.Password(fmt.Sprintf("Password for %s@%s", *user, *host))
	})
	defer closeAgent()
	c, err := remote.Dial(remote.Options{
		Host: *host, Port: *port, User: *user, Auth: auth,
		HostKey: hk, HostKeyAlgorithms: remote.KnownAlgorithms(*knownHosts, *host, *port),
	})
	if err != nil {
		return err
	}
	defer c.Close()

	return (&app.FolderSync{
		Client:      c,
		Local:       *local,
		Root:        *root,
		Dirs:        dirs,
		Excludes:    transfer.Excludes(excludes),
		KeepBackups: *keep,
		DryRun:      *dry,
		Yes:         *yes,
		UI:          u,
	}).Run()
}
