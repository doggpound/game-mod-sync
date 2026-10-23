// game-mod-sync (Go version, in progress - see docs/DESIGN-GO.md).
//
// Phase 1 has one command that syncs folders to a server:
//
//	game-mod-sync sync --host game.example.com --user amp \
//	    --local /path/to/profile --root /path/to/game/on/server \
//	    [--dir BepInEx/plugins] [--exclude MMHOOK/] [-n] [-y]
//
// The full program (profiles, menus, configs, worlds) is still the bash
// version in this repo.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/doggpound/game-mod-sync/internal/app"
	"github.com/doggpound/game-mod-sync/internal/remote"
	"github.com/doggpound/game-mod-sync/internal/transfer"
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
	if len(args) == 0 {
		usage(out)
		return nil
	}
	switch args[0] {
	case "sync":
		return runSync(args[1:], in, out)
	case "version", "--version", "-V":
		fmt.Fprintln(out, "game-mod-sync", version)
		return nil
	case "help", "--help", "-h":
		usage(out)
		return nil
	}
	return fmt.Errorf("unknown command %q (see game-mod-sync help)", args[0])
}

func usage(out io.Writer) {
	fmt.Fprintf(out, `game-mod-sync %s (Go version, in progress)

Usage:
  game-mod-sync sync --host HOST --user USER --local FOLDER --root FOLDER [options]
  game-mod-sync version

sync makes folders on the server match the same folders here, by content.
Replaced and deleted files are kept in <root>/.mod-sync-backups/<time>/.
Run "game-mod-sync sync -h" for its options.
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

	p := &prompter{in: bufio.NewReader(in), out: out}
	hk, err := remote.HostKeys(*knownHosts, p.trust)
	if err != nil {
		return err
	}
	auth, closeAgent := remote.Auth(keys, p.password(*user, *host))
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
		Out:         out,
		Confirm:     p.confirm,
	}).Run()
}

// prompter asks questions on the terminal (or reads answers from in).
type prompter struct {
	in  *bufio.Reader
	out io.Writer
}

func (p *prompter) line(q string) string {
	fmt.Fprint(p.out, q)
	s, _ := p.in.ReadString('\n')
	return strings.TrimSpace(s)
}

func (p *prompter) confirm(q string) bool {
	a := strings.ToLower(p.line(q + " [y/N] "))
	return a == "y" || a == "yes"
}

func (p *prompter) trust(host, fingerprint string) bool {
	fmt.Fprintf(p.out, "First connection to %s. Its key fingerprint is\n  %s\n", host, fingerprint)
	return p.confirm("Trust this server?")
}

func (p *prompter) password(user, host string) func() (string, error) {
	return func() (string, error) {
		q := fmt.Sprintf("Password for %s@%s: ", user, host)
		if f, ok := os.Stdin.Fd(), true; ok && term.IsTerminal(int(f)) {
			fmt.Fprint(p.out, q)
			b, err := term.ReadPassword(int(f))
			fmt.Fprintln(p.out)
			return string(b), err
		}
		return p.line(q), nil
	}
}
