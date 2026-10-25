// Package app is game-mod-sync's flow: the main menu, first-time setup, a
// profile's sync (mods, then config files, one review), world download and
// the Edit menu. It reaches plugins only through internal/plugins, and the
// person only through internal/ui. See docs/DESIGN.md.
package app

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/doggpound/game-mod-sync/internal/remote"
	"github.com/doggpound/game-mod-sync/internal/transfer"
	"github.com/doggpound/game-mod-sync/internal/ui"
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
	Verbose     bool // list every file, not just a per-mod summary
	UI          ui.UI
	Now         func() time.Time // for tests; nil = time.Now
	Extra       Extra            // more to offer and copy in the same sync (configs); may be nil
	Title       string           // heading of the review, e.g. "Valheim / MyProfile  →  Valheim01 on host"
	Applied     bool             // set when changes were made on the server
}

// Extra is more to compare, offer and copy in the same sync: the config
// files. The sync calls it in this order: Scan, then Preview (dry runs and -y),
// Choose, Review, Apply.
type Extra interface {
	Scan(m *transfer.Manifest) error
	Offered() int                                    // how many could be copied
	Preview(yes bool)                                // what would be copied (yes: -y copies only new ones)
	Choose(yes bool) (bool, error)                   // pick; false = Back
	Picked() int                                     // how many were picked
	Review()                                         // one line per picked file
	Apply(backup string, m *transfer.Manifest) error // backup: this sync's backup folder ("" = none)
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
	var summary []string
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
			f.UI.Warn(d + " is empty here - skipping it so the server's copy isn't wiped")
			continue
		}
		f.UI.Info("Comparing with the server (checksums): " + d)
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
			f.UI.Say("  " + d + "/")
		}
		if f.Verbose {
			for _, x := range p.Send {
				f.UI.Say("     send    " + x)
			}
			for _, x := range p.Delete {
				f.UI.Say("     delete  " + x)
			}
		}
		for _, c := range p.Summary() {
			mark := map[string]string{"new": "+ new    ", "updated": "~ updated", "removed": "- removed"}[c.Kind]
			f.UI.Say("   " + mark + "  " + c.Mod)
			summary = append(summary, "   "+mark+"  "+c.Mod)
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
	if f.Extra != nil {
		if err := f.Extra.Scan(manifest); err != nil {
			return err
		}
	}
	offered := 0
	if f.Extra != nil {
		offered = f.Extra.Offered()
	}
	if !changes && offered == 0 {
		f.UI.OK("Already in sync - nothing to do.")
		return nil
	}
	mods := fmt.Sprintf("%d file(s) to send, %d to delete", send, del)
	if changes {
		f.UI.Say(mods + ".")
	} else {
		f.UI.Say("Mods are in sync.")
	}
	if f.DryRun {
		if f.Extra != nil {
			f.Extra.Preview(false)
		}
		f.UI.OK("Dry run - nothing changed.")
		return nil
	}
	picked := 0
	if f.Extra != nil {
		if f.Yes {
			f.Extra.Preview(true)
		}
		ok, err := f.Extra.Choose(f.Yes)
		if err != nil {
			return err
		}
		if !ok {
			f.UI.Say("Cancelled - nothing changed.")
			return nil
		}
		picked = f.Extra.Picked()
	}
	if !changes && picked == 0 {
		f.UI.OK("No configs picked - nothing changed.")
		return nil
	}
	var what []string
	if changes {
		what = append(what, mods)
	}
	if picked > 0 {
		what = append(what, fmt.Sprintf("%d config file(s) to copy", picked))
	}
	if !f.Yes {
		if f.Title != "" {
			f.UI.Say("")
			f.UI.Say(f.Title)
		}
		f.UI.Say("")
		if changes {
			f.UI.Say("Mods: " + mods)
			for _, l := range summary {
				f.UI.Say(l)
			}
		} else {
			f.UI.Say("Mods: already in sync")
		}
		if f.Extra != nil {
			f.UI.Say("")
			if picked > 0 {
				f.UI.Say(fmt.Sprintf("Config files to copy: %d", picked))
				f.Extra.Review()
			} else {
				f.UI.Say("Config files to copy: none")
			}
		}
		f.UI.Say("")
		f.UI.Say("Replaced and deleted files are kept in " + BackupsDir + " on the server.")
		if !f.UI.Confirm("Apply these changes to the server? ("+strings.Join(what, ", ")+")", false) {
			f.UI.Say("Cancelled - nothing changed.")
			return nil
		}
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
		f.UI.Info("Syncing " + fp.dir)
		if err := a.Run(fp.plan); err != nil {
			return fmt.Errorf("syncing %s: %w", fp.dir, err)
		}
	}
	if picked > 0 {
		bk := ""
		if f.KeepBackups > 0 {
			bk = path.Join(backups, ts)
		}
		f.UI.Info(fmt.Sprintf("Copying %d config file(s) ...", picked))
		if err := f.Extra.Apply(bk, manifest); err != nil {
			return err
		}
	}
	if f.KeepBackups > 0 {
		if err := transfer.PruneBackups(f.Client.SFTP(), backups, f.KeepBackups); err != nil {
			f.UI.Warn(fmt.Sprintf("tidying old backups: %v", err))
		}
	}
	f.Applied = true
	f.UI.OK("Synced.")
	if picked > 0 {
		f.UI.Say(fmt.Sprintf("Copied %d config file(s).", picked))
	}
	if f.KeepBackups > 0 {
		f.UI.Say("Previous files: " + path.Join(backups, ts))
	}
	return nil
}
