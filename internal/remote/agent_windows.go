//go:build windows

package remote

import (
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

// dialAgent connects to the Windows OpenSSH agent's pipe, which password
// managers with an SSH agent (e.g. 1Password) also serve.
func dialAgent() (net.Conn, error) {
	timeout := 2 * time.Second
	return winio.DialPipe(`\\.\pipe\openssh-ssh-agent`, &timeout)
}
