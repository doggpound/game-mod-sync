package remote

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

// DefaultKeyFiles are the usual private keys in ~/.ssh, in the order ssh tries them.
func DefaultKeyFiles() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var out []string
	for _, n := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
		out = append(out, filepath.Join(home, ".ssh", n))
	}
	return out
}

// Auth lists the ways to log in, tried in order: keys from the SSH agent and
// from keyFiles (one publickey attempt), then the password, if password isn't
// nil. Keys that are missing or protected by a passphrase are skipped. The
// returned func closes the agent connection.
func Auth(keyFiles []string, password func() (string, error)) ([]ssh.AuthMethod, func()) {
	var signers []ssh.Signer
	done := func() {}
	if conn, err := dialAgent(); err == nil {
		if s, err := agent.NewClient(conn).Signers(); err == nil {
			signers = append(signers, s...)
		}
		done = func() { conn.Close() }
	}
	for _, f := range keyFiles {
		if s, err := loadKey(f); err == nil {
			signers = append(signers, s)
		}
	}
	var methods []ssh.AuthMethod
	if len(signers) > 0 {
		methods = append(methods, ssh.PublicKeys(signers...))
	}
	if password != nil {
		// Retried, so a saved password that stopped working is followed by
		// asking (password returns the saved one first, then asks).
		methods = append(methods,
			ssh.RetryableAuthMethod(ssh.PasswordCallback(password), 3),
			ssh.KeyboardInteractive(func(_, _ string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range questions {
					p, err := password()
					if err != nil {
						return nil, err
					}
					answers[i] = p
				}
				return answers, nil
			}))
	}
	return methods, done
}

func loadKey(path string) (ssh.Signer, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s, err := ssh.ParsePrivateKey(b)
	var pm *ssh.PassphraseMissingError
	if errors.As(err, &pm) {
		return nil, err // skipped; the agent usually has it unlocked
	}
	return s, err
}
