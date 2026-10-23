// Package remote is the connection to a game server: one SSH connection that
// runs commands and carries an SFTP session for file transfers.
package remote

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Options says where to connect and how to log in.
type Options struct {
	Host    string
	Port    int // 0 = 22
	User    string
	Auth    []ssh.AuthMethod
	HostKey ssh.HostKeyCallback
	// HostKeyAlgorithms limits which kinds of server key to ask for: the ones
	// saved for this server (see KnownAlgorithms), so a server with several
	// keys shows the one we know. Empty = any.
	HostKeyAlgorithms []string
	Timeout           time.Duration // 0 = 15s
}

// Client is an open connection. Close it when done.
type Client struct {
	Target string // user@host, for messages
	ssh    *ssh.Client
	sftp   *sftp.Client
	shell  *bool // can run commands; nil = not checked yet
}

// CanRun reports whether the server runs commands for this login. Many game
// hosts only offer SFTP; then everything is done over SFTP alone.
func (c *Client) CanRun() bool {
	if c.shell == nil {
		_, err := c.Run("true")
		ok := err == nil
		c.shell = &ok
	}
	return *c.shell
}

// Dial connects and logs in, and opens the SFTP session.
func Dial(o Options) (*Client, error) {
	port := o.Port
	if port == 0 {
		port = 22
	}
	timeout := o.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	cfg := &ssh.ClientConfig{
		User:              o.User,
		Auth:              o.Auth,
		HostKeyCallback:   o.HostKey,
		HostKeyAlgorithms: o.HostKeyAlgorithms,
		Timeout:           timeout,
	}
	addr := net.JoinHostPort(o.Host, strconv.Itoa(port))
	sc, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		return nil, fmt.Errorf("can't connect to %s@%s: %w", o.User, o.Host, err)
	}
	fc, err := sftp.NewClient(sc)
	if err != nil {
		sc.Close()
		return nil, fmt.Errorf("%s@%s has no SFTP (needed to copy files): %w", o.User, o.Host, err)
	}
	return &Client{Target: o.User + "@" + o.Host, ssh: sc, sftp: fc}, nil
}

// SFTP is the file-transfer session.
func (c *Client) SFTP() *sftp.Client { return c.sftp }

// Close ends the connection.
func (c *Client) Close() error {
	c.sftp.Close()
	return c.ssh.Close()
}

// ExitError is a command that ran but failed; Stderr holds what it printed.
type ExitError struct {
	Cmd    string
	Status int
	Stderr string
}

func (e *ExitError) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = "no message"
	}
	return fmt.Sprintf("command on the server failed (exit %d): %s", e.Status, msg)
}

// Run runs a shell command on the server and returns what it printed.
func (c *Client) Run(cmd string) ([]byte, error) {
	s, err := c.ssh.NewSession()
	if err != nil {
		return nil, err
	}
	defer s.Close()
	var out, errb bytes.Buffer
	s.Stdout, s.Stderr = &out, &errb
	if err := s.Run(cmd); err != nil {
		var ee *ssh.ExitError
		if errors.As(err, &ee) {
			return out.Bytes(), &ExitError{Cmd: cmd, Status: ee.ExitStatus(), Stderr: errb.String()}
		}
		return out.Bytes(), err
	}
	return out.Bytes(), nil
}

// Quote makes s one word for the server's shell.
func Quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
