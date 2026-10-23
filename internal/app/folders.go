// Package app is game-mod-sync's flow. In phase 1 that's syncing a set of
// folders from this computer to a game folder on the server; profiles,
// servers, configs and worlds come in later phases (docs/DESIGN-GO.md).
package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/doggpound/game-mod-sync/internal/remote"
	"github.com/doggpound/game-mod-sync/internal/transfer"
)

// BackupsDir is where replaced and deleted files go, inside the game folder.
const BackupsDir = ".mod-sync-backups"

// FolderSync makes folders on the server match the same folders locally.
type FolderSync struct {
	Client      *remote.Client
	Local       string   // e.g. the profile folder
	Root        string   // the game folder on the server
	Dirs        []string // folders to sync, relative to both (e.g. BepInEx/plugins)
	Excludes    transfer.Excludes
	KeepBackups int  // backups to keep on the server; 0 = none made
	DryRun      bool // show what would change, change nothing
	Yes         bool // don't ask before applying
	Out         io.Writer
	Confirm     func(question string) bool
	Now         func() time.Time // for tests; nil = time.Now
}

// ErrNothingToSync is returned when none of the folders exist locally with
// something in them: syncing an empty folder would wipe the server's.
var ErrNothingToSync = errors.New("nothing to sync - refusing (it would wipe the server's folders)")

type folderPlan struct {
	dir   string
	local transfer.Tree
	plan  transfer.Plan
}

// Run compares, shows the changes, asks, and applies them.
func (f *FolderSync) Run() error {
	var plans []folderPlan
	send, del := 0, 0
	manifestFile := path.Join(f.Root, BackupsDir, "manifest.json")
	manifest := transfer.LoadManifest(f.Client.SFTP(), manifestFile, f.Root)
	// Hashes read over SFTP are worth keeping even if nothing is applied.
	defer func() { _ = manifest.Save(f.Client.SFTP(), manifestFile) }()
	for _, d := range f.Dirs {
		local := filepath.Join(f.Local, filepath.FromSlash(d))
		entries, err := os.ReadDir(local)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Fprintf(f.Out, "warning: %s is empty here - skipping it so the server's copy isn't wiped\n", d)
			continue
		}
		fmt.Fprintf(f.Out, "==> Comparing with the server (checksums): %s\n", d)
		lt, err := transfer.LocalTree(local)
		if err != nil {
			return err
		}
		st, err := transfer.ServerTree(f.Client, path.Join(f.Root, d), manifest)
		if err != nil {
			return err
		}
		p := transfer.Compare(lt, st, f.Excludes)
		if len(f.Dirs) > 1 {
			fmt.Fprintf(f.Out, "  %s/\n", d)
		}
		for _, c := range p.Summary() {
			mark := map[string]string{"new": "+ new    ", "updated": "~ updated", "removed": "- removed"}[c.Kind]
			fmt.Fprintf(f.Out, "   %s  %s\n", mark, c.Mod)
		}
		send += len(p.Send)
		del += len(p.Delete)
		plans = append(plans, folderPlan{d, lt, p})
	}
	if len(plans) == 0 {
		return ErrNothingToSync
	}
	changes := false
	for _, fp := range plans {
		if !fp.plan.Empty() {
			changes = true
		}
	}
	if !changes {
		fmt.Fprintln(f.Out, "Already in sync - nothing to do.")
		return nil
	}
	what := fmt.Sprintf("%d file(s) to send, %d to delete", send, del)
	fmt.Fprintln(f.Out, what+".")
	if f.DryRun {
		fmt.Fprintln(f.Out, "Dry run - nothing changed.")
		return nil
	}
	if !f.Yes && (f.Confirm == nil || !f.Confirm("Apply these changes to the server? ("+what+")")) {
		fmt.Fprintln(f.Out, "Cancelled - nothing changed.")
		return nil
	}

	now := time.Now
	if f.Now != nil {
		now = f.Now
	}
	ts := now().Format("20060102-150405")
	backups := path.Join(f.Root, BackupsDir)
	for _, fp := range plans {
		if fp.plan.Empty() {
			continue
		}
		a := &transfer.Apply{
			SFTP:     f.Client.SFTP(),
			Local:    filepath.Join(f.Local, filepath.FromSlash(fp.dir)),
			Server:   path.Join(f.Root, fp.dir),
			Hashes:   fp.local,
			Manifest: manifest,
		}
		if f.KeepBackups > 0 {
			a.Backup = path.Join(backups, ts, fp.dir)
		}
		fmt.Fprintf(f.Out, "==> Syncing %s\n", fp.dir)
		if err := a.Run(fp.plan); err != nil {
			return fmt.Errorf("syncing %s: %w", fp.dir, err)
		}
	}
	if f.KeepBackups > 0 {
		if err := transfer.PruneBackups(f.Client.SFTP(), backups, f.KeepBackups); err != nil {
			fmt.Fprintf(f.Out, "warning: tidying old backups: %v\n", err)
		}
	}
	fmt.Fprintln(f.Out, "Synced.")
	if f.KeepBackups > 0 {
		fmt.Fprintf(f.Out, "Previous files: %s\n", path.Join(backups, ts))
	}
	return nil
}
