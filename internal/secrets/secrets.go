// Package secrets keeps login passwords in the system's credential store
// (Windows Credential Manager, macOS Keychain, Linux Secret Service), never
// in the settings files. Where there's no store (e.g. WSL without a keyring
// running), nothing is saved and the password is asked each time.
package secrets

import (
	"errors"

	"github.com/zalando/go-keyring"
)

const service = "game-mod-sync"

// Store keeps passwords by account (user@host:port).
type Store interface {
	Get(account string) (string, error) // ErrNotFound when there's none
	Set(account, password string) error
	Delete(account string) error
}

// ErrNotFound means no password is saved for the account.
var ErrNotFound = errors.New("no saved password")

// System is the system's credential store.
type System struct{}

func (System) Get(account string) (string, error) {
	p, err := keyring.Get(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return p, err
}

func (System) Set(account, password string) error { return keyring.Set(service, account, password) }

func (System) Delete(account string) error {
	err := keyring.Delete(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// Memory is a store for tests.
type Memory map[string]string

func (m Memory) Get(a string) (string, error) {
	if p, ok := m[a]; ok {
		return p, nil
	}
	return "", ErrNotFound
}
func (m Memory) Set(a, p string) error { m[a] = p; return nil }
func (m Memory) Delete(a string) error { delete(m, a); return nil }
