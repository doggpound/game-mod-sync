// Package transfer compares a local folder with one on the server by content
// and makes the server's match it: what rsync -rlc --delete --backup did in
// the bash version, over SSH and SFTP only.
package transfer

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/doggpound/game-mod-sync/internal/remote"
)

// Entry is one file or folder. Paths are relative, with / separators.
type Entry struct {
	Dir  bool
	Hash string // sha256, hex; files only
	Size int64  // local files only
}

// Tree is a folder's contents by relative path.
type Tree map[string]Entry

// LocalTree hashes every file under root. Symlinks are skipped (mod managers
// don't make them). A missing root is an empty tree.
func LocalTree(root string) (Tree, error) {
	t := Tree{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root && errors.Is(err, fs.ErrNotExist) {
				return filepath.SkipAll
			}
			return err
		}
		if p == root {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		switch {
		case d.IsDir():
			t[rel] = Entry{Dir: true}
		case d.Type().IsRegular():
			h, n, err := hashFile(p)
			if err != nil {
				return err
			}
			t[rel] = Entry{Hash: h, Size: n}
		}
		return nil
	})
	return t, err
}

func hashFile(p string) (string, int64, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// Runner runs a shell command on the server (remote.Client does).
type Runner interface {
	Run(cmd string) ([]byte, error)
}

// RemoteTree lists and hashes everything under root on the server with
// find and sha256sum (GNU coreutils). A missing root is an empty tree.
func RemoteTree(r Runner, root string) (Tree, error) {
	t := Tree{}
	q := remote.Quote(root)
	out, err := r.Run("cd " + q + " 2>/dev/null || exit 0; find . -mindepth 1 -type d -print0")
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", root, err)
	}
	for _, p := range splitNul(out) {
		t[strings.TrimPrefix(p, "./")] = Entry{Dir: true}
	}
	// sha256sum -z ends each line with NUL, so any file name works.
	out, err = r.Run("cd " + q + " 2>/dev/null || exit 0; find . -type f -print0 | xargs -0 -r sha256sum -z")
	if err != nil {
		return nil, fmt.Errorf("hashing %s: %w", root, err)
	}
	for _, line := range splitNul(out) {
		// "<64 hex>  ./path" (or " *./path" in binary mode)
		if len(line) < 67 {
			return nil, fmt.Errorf("unexpected sha256sum output: %q", line)
		}
		t[strings.TrimPrefix(line[66:], "./")] = Entry{Hash: line[:64]}
	}
	return t, nil
}

func splitNul(b []byte) []string {
	var out []string
	for _, p := range bytes.Split(b, []byte{0}) {
		if len(p) > 0 {
			out = append(out, string(p))
		}
	}
	return out
}
