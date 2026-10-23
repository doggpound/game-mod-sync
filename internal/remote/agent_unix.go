//go:build !windows

package remote

import (
	"errors"
	"net"
	"os"
)

// dialAgent connects to the SSH agent named by SSH_AUTH_SOCK.
func dialAgent() (net.Conn, error) {
	sock := os.Getenv("SSH_AUTH_SOCK")
	if sock == "" {
		return nil, errors.New("no SSH agent")
	}
	return net.Dial("unix", sock)
}
