package worlds

import (
	"errors"
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pkg/sftp"
)

// ErrChangedDuringCopy means files vanished while they were being copied: the
// server saved the world. The copy is tried again.
var ErrChangedDuringCopy = errors.New("the world changed during the copy")

type entry struct {
	dir   bool
	size  int64
	mtime time.Time
}

// serverEntries lists the world's paths below dir on the server (relative,
// / separators). Paths that don't exist are left out.
func serverEntries(c *sftp.Client, dir string, paths []string) (map[string]entry, error) {
	out := map[string]entry{}
	for _, p := range paths {
		top := path.Join(dir, p)
		fi, err := c.Stat(top)
		if errors.Is(err, iofs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !fi.IsDir() {
			out[p] = entry{size: fi.Size(), mtime: fi.ModTime()}
			continue
		}
		w := c.Walk(top)
		for w.Step() {
			if err := w.Err(); err != nil {
				if errors.Is(err, iofs.ErrNotExist) {
					continue // removed by a save while listing
				}
				return nil, err
			}
			rel := strings.TrimPrefix(strings.TrimPrefix(w.Path(), dir), "/")
			st := w.Stat()
			switch {
			case st.IsDir():
				out[rel] = entry{dir: true}
			case st.Mode().IsRegular():
				out[rel] = entry{size: st.Size(), mtime: st.ModTime()}
			}
		}
	}
	return out, nil
}

// localEntries lists the same paths below dir on this computer.
func localEntries(dir string, paths []string) map[string]entry {
	out := map[string]entry{}
	for _, p := range paths {
		top := filepath.Join(dir, filepath.FromSlash(p))
		_ = filepath.WalkDir(top, func(fp string, d iofs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(dir, fp)
			rel = filepath.ToSlash(rel)
			if d.IsDir() {
				out[rel] = entry{dir: true}
				return nil
			}
			if fi, err := d.Info(); err == nil && fi.Mode().IsRegular() {
				out[rel] = entry{size: fi.Size(), mtime: fi.ModTime()}
			}
			return nil
		})
	}
	return out
}

// Mirror makes dst's copy of the world's paths match the server's dir: files
// that are new or differ in size or time are downloaded (and given the
// server's time, so the next copy skips them), files the server doesn't have
// are removed. Returns ErrChangedDuringCopy when a file vanished meanwhile.
func Mirror(c *sftp.Client, dir, dst string, paths []string, progress func(done, total int64)) error {
	srv, err := serverEntries(c, dir, paths)
	if err != nil {
		return err
	}
	loc := localEntries(dst, paths)

	// Remove what the server doesn't have (deepest first), and files that
	// became folders or the other way round.
	var gone []string
	for rel, l := range loc {
		if s, ok := srv[rel]; !ok || s.dir != l.dir {
			gone = append(gone, rel)
		}
	}
	sort.Slice(gone, func(i, j int) bool { return strings.Count(gone[i], "/") > strings.Count(gone[j], "/") })
	for _, rel := range gone {
		if err := os.RemoveAll(filepath.Join(dst, filepath.FromSlash(rel))); err != nil {
			return err
		}
		delete(loc, rel)
	}

	var dirs, files []string
	var total int64
	for rel, s := range srv {
		if s.dir {
			dirs = append(dirs, rel)
			continue
		}
		l, ok := loc[rel]
		if ok && l.size == s.size && l.mtime.Unix() == s.mtime.Unix() {
			continue
		}
		files = append(files, rel)
		total += s.size
	}
	sort.Slice(dirs, func(i, j int) bool { return strings.Count(dirs[i], "/") < strings.Count(dirs[j], "/") })
	for _, rel := range dirs {
		if err := os.MkdirAll(filepath.Join(dst, filepath.FromSlash(rel)), 0o755); err != nil {
			return err
		}
	}
	sort.Strings(files)
	var done int64
	for _, rel := range files {
		n, err := download(c, path.Join(dir, rel), filepath.Join(dst, filepath.FromSlash(rel)), srv[rel].mtime, func(n int64) {
			if progress != nil {
				progress(done+n, total)
			}
		})
		if err != nil {
			return err
		}
		done += n
	}
	if progress != nil {
		progress(total, total)
	}
	return nil
}

func download(c *sftp.Client, src, dst string, mtime time.Time, progress func(int64)) (int64, error) {
	in, err := c.Open(src)
	if errors.Is(err, iofs.ErrNotExist) {
		return 0, ErrChangedDuringCopy
	}
	if err != nil {
		return 0, fmt.Errorf("reading %s: %w", src, err)
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return 0, err
	}
	tmp := dst + ".gms-tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(out, &counter{r: in, f: progress})
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return n, fmt.Errorf("downloading %s: %w", src, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return n, err
	}
	_ = os.Chtimes(dst, mtime, mtime)
	return n, nil
}

type counter struct {
	r io.Reader
	n int64
	f func(int64)
}

func (c *counter) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	if c.f != nil && n > 0 {
		c.f(c.n)
	}
	return n, err
}

// CopyTree copies src (a file or folder) to dst on this computer.
func CopyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d iofs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}
