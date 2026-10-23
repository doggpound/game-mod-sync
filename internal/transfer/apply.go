package transfer

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/pkg/sftp"
)

// Apply carries out a plan on the server over SFTP.
//
// Files that are replaced or deleted are first moved (renamed: instant, same
// disk) into Backup, keeping their relative path, so nothing is lost. New
// content goes to a temporary name next to its target and is renamed into
// place when complete, so a dropped connection never leaves half a file.
type Apply struct {
	SFTP     *sftp.Client
	Local    string // local folder (OS path)
	Server   string // server folder (absolute, / separators)
	Backup   string // server folder for replaced and deleted files; "" = no backups
	Progress func(done, total int64)
	Hashes   Tree      // the local tree the plan was made from (hashes for the manifest)
	Manifest *Manifest // kept up to date with what's uploaded and removed; may be nil
}

const tmpPrefix = ".gms-tmp-"

// Run applies the plan: deletions, then folders, then uploads.
func (a *Apply) Run(p Plan) error {
	c := a.SFTP
	if err := c.MkdirAll(a.Server); err != nil {
		return fmt.Errorf("creating %s: %w", a.Server, err)
	}
	for _, rel := range p.Delete {
		if err := a.retire(rel); err != nil {
			return err
		}
	}
	for _, rel := range p.RmDirs {
		// Its files were retired above; anything left is moved to the backup as a whole.
		if err := c.RemoveDirectory(a.server(rel)); err != nil {
			if err := a.retire(rel); err != nil {
				return err
			}
		}
	}
	for _, rel := range p.MkDirs {
		dir := a.server(rel)
		if err := c.Mkdir(dir); err != nil {
			if fi, serr := c.Stat(dir); serr != nil || !fi.IsDir() {
				return fmt.Errorf("creating %s: %w", dir, err)
			}
		}
		_ = c.Chmod(dir, 0o755)
	}
	var done int64
	for _, rel := range p.Send {
		if !p.New[rel] {
			if err := a.retire(rel); err != nil {
				return err
			}
		}
		n, err := a.upload(rel, func(n int64) {
			if a.Progress != nil {
				a.Progress(done+n, p.Bytes)
			}
		})
		if err != nil {
			return err
		}
		done += n
	}
	if a.Progress != nil {
		a.Progress(p.Bytes, p.Bytes)
	}
	return nil
}

func (a *Apply) server(rel string) string { return path.Join(a.Server, rel) }

// retire moves a server file or folder into the backup, or removes it when
// backups are off.
func (a *Apply) retire(rel string) error {
	c, src := a.SFTP, a.server(rel)
	a.Manifest.forget(a.Manifest.key(src))
	if a.Backup == "" {
		if err := removeAll(c, src); err != nil {
			return fmt.Errorf("removing %s: %w", src, err)
		}
		return nil
	}
	dst := path.Join(a.Backup, rel)
	if err := c.MkdirAll(path.Dir(dst)); err != nil {
		return fmt.Errorf("creating %s: %w", path.Dir(dst), err)
	}
	if err := c.Rename(src, dst); err != nil {
		return fmt.Errorf("moving %s to the backups: %w", src, err)
	}
	return nil
}

func (a *Apply) upload(rel string, progress func(int64)) (int64, error) {
	c := a.SFTP
	dst := a.server(rel)
	tmp := path.Join(path.Dir(dst), tmpPrefix+path.Base(dst))
	in, err := os.Open(filepath.Join(a.Local, filepath.FromSlash(rel)))
	if err != nil {
		return 0, err
	}
	defer in.Close()
	out, err := c.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return 0, fmt.Errorf("writing %s: %w", tmp, err)
	}
	n, err := io.Copy(out, &countingReader{r: in, f: progress})
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		c.Remove(tmp)
		return n, fmt.Errorf("uploading %s: %w", rel, err)
	}
	_ = c.Chmod(tmp, 0o644)
	// posix-rename replaces the target in one step (OpenSSH supports it).
	if err := c.PosixRename(tmp, dst); err != nil {
		c.Remove(dst)
		if err := c.Rename(tmp, dst); err != nil {
			c.Remove(tmp)
			return n, fmt.Errorf("putting %s in place: %w", rel, err)
		}
	}
	if fi, err := c.Stat(dst); err == nil {
		a.Manifest.set(a.Manifest.key(dst), a.Hashes[rel].Hash, fi.Size(), fi.ModTime().Unix())
	}
	return n, nil
}

type countingReader struct {
	r io.Reader
	n int64
	f func(int64)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	if c.f != nil && n > 0 {
		c.f(c.n)
	}
	return n, err
}

// removeAll deletes a server file or folder with everything in it.
func removeAll(c *sftp.Client, p string) error {
	fi, err := c.Lstat(p)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return c.Remove(p)
	}
	entries, err := c.ReadDir(p)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := removeAll(c, path.Join(p, e.Name())); err != nil {
			return err
		}
	}
	return c.RemoveDirectory(p)
}

// PruneBackups keeps the newest keep folders in dir (named by time, so newest
// sorts last) and removes the rest.
func PruneBackups(c *sftp.Client, dir string, keep int) error {
	entries, err := c.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for i, n := range names {
		if i >= keep {
			if err := removeAll(c, path.Join(dir, n)); err != nil {
				return err
			}
		}
	}
	return nil
}
