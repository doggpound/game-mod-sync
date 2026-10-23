package remote

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
)

func newKey(t *testing.T) ssh.PublicKey {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	k, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestHostKeysAsksOnceThenRemembers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ssh", "known_hosts")
	asked := 0
	cb, err := HostKeys(path, func(host, fp string) bool { asked++; return true })
	if err != nil {
		t.Fatal(err)
	}
	key := newKey(t)
	addr := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 2022}
	for i := 0; i < 2; i++ {
		if err := cb("game.example.com:2022", addr, key); err != nil {
			t.Fatalf("connection %d: %v", i+1, err)
		}
	}
	if asked != 1 {
		t.Errorf("asked %d times, want 1", asked)
	}
}

func TestHostKeysRefusesAChangedKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	cb, _ := HostKeys(path, func(string, string) bool { return true })
	addr := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 22}
	if err := cb("game.example.com:22", addr, newKey(t)); err != nil {
		t.Fatal(err)
	}
	err := cb("game.example.com:22", addr, newKey(t))
	if !errors.Is(err, ErrHostKeyChanged) {
		t.Errorf("err = %v, want ErrHostKeyChanged", err)
	}
}

func TestHostKeysNotTrusted(t *testing.T) {
	cb, _ := HostKeys(filepath.Join(t.TempDir(), "known_hosts"), func(string, string) bool { return false })
	if err := cb("game.example.com:22", &net.TCPAddr{}, newKey(t)); err == nil {
		t.Error("an untrusted server was accepted")
	}
}

func TestQuote(t *testing.T) {
	if got := Quote("it's here"); got != `'it'\''s here'` {
		t.Errorf("Quote = %s", got)
	}
}
