package transfer

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"

	"github.com/pkg/sftp"

	"github.com/doggpound/game-mod-sync/internal/remote"
)

// ServerTree lists and hashes everything under dir on the server. When the
// server runs commands, it hashes there (fast). On SFTP-only hosts files are
// read over SFTP, except those the manifest already knows with the same size
// and time. Paths in the manifest are relative to its base folder (dir must
// be inside it).
func ServerTree(c *remote.Client, dir string, m *Manifest) (Tree, error) {
	if c.CanRun() {
		t, err := RemoteTree(c, dir)
		if err == nil {
			return t, nil
		}
		// e.g. no sha256sum on the server: fall back to SFTP
	}
	return SFTPTree(c.SFTP(), dir, m)
}

// SFTPTree builds the tree over SFTP alone, using and updating m (may be nil).
func SFTPTree(c *sftp.Client, dir string, m *Manifest) (Tree, error) {
	t := Tree{}
	if _, err := c.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return t, nil
	}
	w := c.Walk(dir)
	for w.Step() {
		if err := w.Err(); err != nil {
			return nil, err
		}
		p := w.Path()
		if p == dir {
			continue
		}
		rel := strings.TrimPrefix(p, dir+"/")
		fi := w.Stat()
		switch {
		case fi.IsDir():
			t[rel] = Entry{Dir: true}
		case fi.Mode().IsRegular():
			key := m.key(p)
			if h, ok := m.lookup(key, fi.Size(), fi.ModTime().Unix()); ok {
				t[rel] = Entry{Hash: h}
				continue
			}
			h, err := hashRemote(c, p)
			if err != nil {
				return nil, err
			}
			m.set(key, h, fi.Size(), fi.ModTime().Unix())
			t[rel] = Entry{Hash: h}
		}
	}
	return t, nil
}

func hashRemote(c *sftp.Client, p string) (string, error) {
	f, err := c.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("reading %s: %w", p, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Manifest remembers the hash of each server file with the size and time it
// had, so SFTP-only syncs don't download unchanged files to compare them. It's
// stored on the server (shared by everyone who syncs there); a file changed by
// anything else gets a new size or time and is read again.
type Manifest struct {
	Base  string                   `json:"-"` // server folder the paths are relative to
	Files map[string]ManifestEntry `json:"files"`
	dirty bool
}

// ManifestEntry is what's known about one file.
type ManifestEntry struct {
	Hash  string `json:"sha256"`
	Size  int64  `json:"size"`
	MTime int64  `json:"mtime"`
}

// LoadManifest reads the manifest at file (a missing or unreadable one is empty).
func LoadManifest(c *sftp.Client, file, base string) *Manifest {
	m := &Manifest{Base: base, Files: map[string]ManifestEntry{}}
	f, err := c.Open(file)
	if err != nil {
		return m
	}
	defer f.Close()
	if err := json.NewDecoder(f).Decode(m); err != nil || m.Files == nil {
		m.Files = map[string]ManifestEntry{}
	}
	return m
}

// Save writes the manifest to file if anything changed.
func (m *Manifest) Save(c *sftp.Client, file string) error {
	if m == nil || !m.dirty {
		return nil
	}
	if err := c.MkdirAll(path.Dir(file)); err != nil {
		return err
	}
	tmp := path.Join(path.Dir(file), tmpPrefix+path.Base(file))
	f, err := c.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return err
	}
	err = json.NewEncoder(f).Encode(m)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		c.Remove(tmp)
		return err
	}
	if err := c.PosixRename(tmp, file); err != nil {
		c.Remove(file)
		return c.Rename(tmp, file)
	}
	m.dirty = false
	return nil
}

func (m *Manifest) key(p string) string {
	if m == nil {
		return p
	}
	return strings.TrimPrefix(p, m.Base+"/")
}

func (m *Manifest) lookup(key string, size, mtime int64) (string, bool) {
	if m == nil {
		return "", false
	}
	e, ok := m.Files[key]
	if !ok || e.Size != size || e.MTime != mtime {
		return "", false
	}
	return e.Hash, true
}

func (m *Manifest) set(key, hash string, size, mtime int64) {
	if m == nil {
		return
	}
	m.Files[key] = ManifestEntry{Hash: hash, Size: size, MTime: mtime}
	m.dirty = true
}

// forget drops a path and everything under it.
func (m *Manifest) forget(key string) {
	if m == nil {
		return
	}
	for k := range m.Files {
		if k == key || strings.HasPrefix(k, key+"/") {
			delete(m.Files, k)
			m.dirty = true
		}
	}
}
