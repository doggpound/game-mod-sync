// Package worlds is the shared part of world download: reading worlds on
// the server (over SFTP) or on this computer through one small FS, and
// copying a world's files (Mirror). How a game saves a world is a world
// format plugin (plugins/worlds/).
package worlds

import (
	"fmt"
	"io"
	iofs "io/fs"
	"os"
	"path"
	"path/filepath"

	"github.com/pkg/sftp"
)

// FS is a place worlds are read from.
type FS interface {
	ReadDir(dir string) ([]iofs.FileInfo, error)
	Stat(p string) (iofs.FileInfo, error)
	Open(p string) (io.ReadCloser, error)
	Join(elem ...string) string
}

// ServerFS reads the server over SFTP.
type ServerFS struct{ C *sftp.Client }

func (s ServerFS) ReadDir(d string) ([]iofs.FileInfo, error) { return s.C.ReadDir(d) }
func (s ServerFS) Stat(p string) (iofs.FileInfo, error)      { return s.C.Stat(p) }
func (s ServerFS) Open(p string) (io.ReadCloser, error)      { return s.C.Open(p) }
func (s ServerFS) Join(e ...string) string                   { return path.Join(e...) }

// LocalFS reads this computer.
type LocalFS struct{}

func (LocalFS) ReadDir(d string) ([]iofs.FileInfo, error) {
	entries, err := os.ReadDir(d)
	if err != nil {
		return nil, err
	}
	var out []iofs.FileInfo
	for _, e := range entries {
		if fi, err := e.Info(); err == nil {
			out = append(out, fi)
		}
	}
	return out, nil
}
func (LocalFS) Stat(p string) (iofs.FileInfo, error) { return os.Stat(p) }
func (LocalFS) Open(p string) (io.ReadCloser, error) { return os.Open(p) }
func (LocalFS) Join(e ...string) string              { return filepath.Join(e...) }

// World is one world on offer.
type World struct {
	Name   string
	Detail string // e.g. "save 1940, 2026-10-22 18:55, 65M"
}

// HumanSize is a size the way du -h writes it: 512, 4.0K, 65M, 1.5G.
func HumanSize(n int64) string {
	f := float64(n)
	for _, u := range []string{"", "K", "M", "G", "T"} {
		if f < 1024 || u == "T" {
			switch {
			case u == "":
				return fmt.Sprintf("%d", n)
			case f < 10:
				return fmt.Sprintf("%.1f%s", f, u)
			default:
				return fmt.Sprintf("%.0f%s", f, u)
			}
		}
		f /= 1024
	}
	return ""
}
