// Package testserver runs a real SSH + SFTP server for tests, on 127.0.0.1
// and a free port. Commands run with sh in the test machine's own shell (like
// a Linux game server), with HOME set to the server's folder.
package testserver

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"testing"

	gliderssh "github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
	gossh "golang.org/x/crypto/ssh"
)

// Password is the password every user has.
const Password = "test"

// Server is a running test server. It stops when the test ends.
type Server struct {
	Host      string
	Port      int
	Root      string // the server's files (also HOME for commands)
	HostKey   gossh.PublicKey
	mu        sync.Mutex
	cmds      []string
	sftpOnly  bool
	extraKey  bool
	acceptAny bool
	close     func()
}

// Option changes how the server behaves.
type Option func(*Server)

// SFTPOnly makes the server refuse commands, like most game hosts' SFTP logins.
func SFTPOnly() Option { return func(s *Server) { s.sftpOnly = true } }

// ExtraHostKey gives the server a second key (ECDSA), offered before its
// ed25519 key, like real servers that have several.
func ExtraHostKey() Option { return func(s *Server) { s.extraKey = true } }

// AcceptAny lets every login in, with any password or key (the screenshot demo).
func AcceptAny() Option { return func(s *Server) { s.acceptAny = true } }

// Start starts a server whose files live in a new temporary folder. Tests
// that run commands skip on systems without sh, find, xargs and sha256sum.
func Start(t testing.TB, opts ...Option) *Server {
	t.Helper()
	probe := &Server{}
	for _, o := range opts {
		o(probe)
	}
	for _, tool := range []string{"sh", "find", "xargs", "sha256sum"} {
		if _, err := exec.LookPath(tool); err != nil && !probe.sftpOnly {
			t.Skipf("no %s on this system", tool)
		}
	}
	s, err := New(t.TempDir(), opts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

// New starts a server on root (its files, and HOME for commands) outside a
// test. Close stops it.
func New(root string, opts ...Option) (*Server, error) {
	s := &Server{Root: root}
	for _, o := range opts {
		o(s)
	}
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		return nil, err
	}
	s.HostKey = signer.PublicKey()
	srv := &gliderssh.Server{
		Handler: s.exec,
		SubsystemHandlers: map[string]gliderssh.SubsystemHandler{
			"sftp": func(sess gliderssh.Session) {
				// Logins land in the server's folder, like a host's SFTP.
				fs, err := sftp.NewServer(sess, sftp.WithServerWorkingDirectory(s.Root))
				if err != nil {
					return
				}
				_ = fs.Serve()
				fs.Close()
			},
		},
		PasswordHandler: func(_ gliderssh.Context, pw string) bool { return s.acceptAny || pw == Password },
	}
	if s.acceptAny {
		srv.PublicKeyHandler = func(gliderssh.Context, gliderssh.PublicKey) bool { return true }
	}
	if s.extraKey {
		ek, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		es, err := gossh.NewSignerFromKey(ek)
		if err != nil {
			return nil, err
		}
		srv.AddHostKey(es)
	}
	srv.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s.Host = "127.0.0.1"
	s.Port = ln.Addr().(*net.TCPAddr).Port
	s.close = func() { srv.Close() }
	go func() { _ = srv.Serve(ln) }()
	return s, nil
}

// Close stops the server.
func (s *Server) Close() {
	if s.close != nil {
		s.close()
	}
}

// Addr is host:port.
func (s *Server) Addr() string { return net.JoinHostPort(s.Host, strconv.Itoa(s.Port)) }

// Commands returns the commands run so far.
func (s *Server) Commands() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.cmds...)
}

func (s *Server) exec(sess gliderssh.Session) {
	cmd := sess.RawCommand()
	s.mu.Lock()
	s.cmds = append(s.cmds, cmd)
	s.mu.Unlock()
	if s.sftpOnly {
		_, _ = io.WriteString(sess.Stderr(), "This service allows sftp connections only.\n")
		_ = sess.Exit(1)
		return
	}
	c := exec.Command("sh", "-c", cmd)
	c.Dir = s.Root
	c.Env = append(os.Environ(), "HOME="+s.Root)
	c.Stdout = sess
	c.Stderr = sess.Stderr()
	err := c.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		_ = sess.Exit(0)
	case errors.As(err, &ee):
		_ = sess.Exit(ee.ExitCode())
	default:
		_, _ = io.WriteString(sess.Stderr(), err.Error()+"\n")
		_ = sess.Exit(127)
	}
}
