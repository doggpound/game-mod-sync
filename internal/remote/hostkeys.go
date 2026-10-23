package remote

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// DefaultKnownHosts is ~/.ssh/known_hosts, shared with the ssh command.
func DefaultKnownHosts() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".ssh", "known_hosts")
}

// KnownAlgorithms lists the key algorithms for the keys saved for host:port in
// the known_hosts file at path (none if it isn't known). Asking the server
// only for these is what the ssh command does too: a server usually has
// several keys (ed25519, ecdsa, rsa) and only some of them are saved.
func KnownAlgorithms(path, host string, port int) []string {
	check, err := knownhosts.New(path)
	if err != nil {
		return nil
	}
	// Check a key no server has; the error lists the saved ones.
	pub, _, _ := ed25519.GenerateKey(nil)
	probe, _ := ssh.NewPublicKey(pub)
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	var ke *knownhosts.KeyError
	if !errors.As(check(addr, &net.TCPAddr{}, probe), &ke) {
		return nil
	}
	var algos []string
	seen := map[string]bool{}
	for _, k := range ke.Want {
		for _, a := range algorithmsFor(k.Key.Type()) {
			if !seen[a] {
				seen[a] = true
				algos = append(algos, a)
			}
		}
	}
	return algos
}

// algorithmsFor: an RSA key can be shown with any of three signature algorithms.
func algorithmsFor(keyType string) []string {
	if keyType == ssh.KeyAlgoRSA {
		return []string{ssh.KeyAlgoRSASHA512, ssh.KeyAlgoRSASHA256, ssh.KeyAlgoRSA}
	}
	return []string{keyType}
}

// ErrHostKeyChanged means the server's key isn't the one saved for it: either
// the server was reinstalled, or something is in the way. Never accepted.
var ErrHostKeyChanged = errors.New("the server's identity changed since the last connection")

// HostKeys checks the server's key against the known_hosts file at path. A
// server seen for the first time is shown to trust (with its fingerprint); a
// yes saves it, so it's not asked again.
func HostKeys(path string, trust func(host, fingerprint string) bool) (ssh.HostKeyCallback, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0o600)
	if err != nil {
		return nil, err
	}
	f.Close()
	return func(hostname string, addr net.Addr, key ssh.PublicKey) error {
		check, err := knownhosts.New(path) // read again: another connection may have added a line
		if err != nil {
			return err
		}
		err = check(hostname, addr, key)
		var ke *knownhosts.KeyError
		if !errors.As(err, &ke) {
			return err // nil when known
		}
		if len(ke.Want) > 0 {
			return fmt.Errorf("%w (%s, now %s) - if it was reinstalled, remove its line from %s",
				ErrHostKeyChanged, hostname, ssh.FingerprintSHA256(key), path)
		}
		if trust == nil || !trust(hostname, ssh.FingerprintSHA256(key)) {
			return fmt.Errorf("%s isn't a known server and wasn't trusted", hostname)
		}
		line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
		w, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer w.Close()
		_, err = fmt.Fprintln(w, line)
		return err
	}, nil
}
